package daemon

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	capabilityArchive = "archive"

	// archiveDefaultTimeout and archiveMaxTimeout mirror the daemon's own
	// defaults so its timeout fires first and yields a clean final response.
	archiveDefaultTimeout = time.Hour
	archiveMaxTimeout     = 24 * time.Hour
	// archiveCompletionGrace extends the panel-side wait past the daemon
	// timeout: a daemon-reported "timeout exceeded" beats a local ctx error.
	archiveCompletionGrace = 5 * time.Minute
	archiveAckTimeout      = 30 * time.Second
	archiveOpRetention     = 10 * time.Minute
	archiveJanitorInterval = time.Minute
	// archiveEventSkippedCap bounds the skipped-entries list inside pub/sub
	// events; the daemon itself caps the full list at 1000.
	archiveEventSkippedCap = 100

	archiveActionStart  = "start"
	archiveActionCancel = "cancel"
)

type archiveNotSupportedError struct{}

func (e *archiveNotSupportedError) Error() string   { return "node does not support archive operations" }
func (e *archiveNotSupportedError) HTTPStatus() int { return http.StatusBadGateway }

var ErrArchiveNotSupported error = &archiveNotSupportedError{}

var ErrArchiveOperationNotFound = errors.New("archive operation not found")

type ArchiveKind string

const (
	ArchiveKindCreate  ArchiveKind = "create"
	ArchiveKindExtract ArchiveKind = "extract"
)

type ArchiveOpStatus string

const (
	ArchiveOpRunning ArchiveOpStatus = "running"
	ArchiveOpDone    ArchiveOpStatus = "done"
	ArchiveOpError   ArchiveOpStatus = "error"
)

type ArchiveStartOptions struct {
	// ServerID scopes the operation's events to a server channel for the
	// file-manager WebSocket; 0 marks a node-scoped (plugin) operation.
	ServerID         uint
	Initiator        string
	ProgressInterval time.Duration
	Timeout          time.Duration
}

// CreateArchiveParams carries absolute panel-side paths; the service strips
// node.WorkPath before anything goes on the wire.
type CreateArchiveParams struct {
	ArchivePath      string
	BasePath         string
	Sources          []string
	Format           proto.ArchiveFormat
	CompressionLevel *int32
	Overwrite        bool
	MaxTotalBytes    uint64
	MaxFiles         uint32
	Owner            OwnerOptions
	Options          ArchiveStartOptions
}

type ExtractArchiveParams struct {
	ArchivePath         string
	Destination         string
	Format              proto.ArchiveFormat
	ConflictPolicy      proto.ArchiveConflictPolicy
	CreateDestination   bool
	PreservePermissions bool
	MaxTotalBytes       uint64
	MaxFiles            uint32
	Owner               OwnerOptions
	Options             ArchiveStartOptions
}

type ArchiveLimits struct {
	MaxTotalBytes uint64
	MaxFiles      uint32
}

type ArchiveOpSnapshot struct {
	OperationID string
	NodeID      uint64
	ServerID    uint
	Kind        ArchiveKind
	Initiator   string
	Status      ArchiveOpStatus
	Progress    messages.ArchiveProgressEventPayload
	Result      *messages.ArchiveCompleteEventPayload
	StartedAt   time.Time
	UpdatedAt   time.Time
}

type archiveOpEntry struct {
	snapshot  ArchiveOpSnapshot
	expiresAt time.Time
	waiters   []chan *messages.ArchiveCompleteEventPayload
}

// archiveRelayMeta is the session-owning instance's context for one in-flight
// operation: enough to enrich daemon pushes into pub/sub events.
type archiveRelayMeta struct {
	nodeID    uint64
	serverID  uint
	kind      ArchiveKind
	initiator string
}

// ArchiveService starts, cancels and observes daemon-side archive operations.
// The initiating instance keeps an in-memory registry entry per operation;
// the session-owning instance relays daemon pushes into pub/sub, so both
// roles work whether they are the same instance or not.
type ArchiveService struct {
	ps         pubsub.PubSub
	gateway    ArchiveGateway
	registry   ConnectionChecker
	instanceID string
	limits     ArchiveLimits
	logger     *slog.Logger

	mu     sync.Mutex
	ops    map[string]*archiveOpEntry
	relays map[string]archiveRelayMeta
	acks   map[string]chan string
}

func NewArchiveService(
	ps pubsub.PubSub,
	gateway ArchiveGateway,
	registry ConnectionChecker,
	instanceID string,
	limits ArchiveLimits,
	logger *slog.Logger,
) *ArchiveService {
	if logger == nil {
		logger = slog.Default()
	}

	return &ArchiveService{
		ps:         ps,
		gateway:    gateway,
		registry:   registry,
		instanceID: instanceID,
		limits:     limits,
		logger:     logger,
		ops:        make(map[string]*archiveOpEntry),
		relays:     make(map[string]archiveRelayMeta),
		acks:       make(map[string]chan string),
	}
}

func (s *ArchiveService) Start(ctx context.Context) error {
	if err := s.ps.Subscribe(ctx, channels.DaemonArchiveRequestAll, s.handleDispatchRequest); err != nil {
		return errors.Wrap(err, "subscribe to archive request dispatch")
	}

	ackChannel := channels.BuildDaemonArchiveResponseChannel(s.instanceID)
	if err := s.ps.Subscribe(ctx, ackChannel, s.handleDispatchAck); err != nil {
		return errors.Wrap(err, "subscribe to archive dispatch acks")
	}

	if err := s.ps.Subscribe(ctx, channels.RealtimeArchiveOpAll, s.handleOpEvent); err != nil {
		return errors.Wrap(err, "subscribe to archive operation events")
	}

	go s.janitor(ctx)

	s.logger.Info("archive service started", "instance_id", s.instanceID)

	return nil
}

func (s *ArchiveService) StartCreate(
	ctx context.Context, node *domain.Node, p CreateArchiveParams,
) (string, error) {
	baseRel := stripWorkPath(node.WorkPath, p.BasePath)

	// The daemon resolves sources relative to base_path and stores the source
	// string as the archive entry name, so absolute panel-side paths must be
	// rebased, not just stripped of the work path.
	sources := make([]string, 0, len(p.Sources))
	for _, src := range p.Sources {
		srcRel, err := sourceRelToBase(baseRel, stripWorkPath(node.WorkPath, src))
		if err != nil {
			return "", err
		}
		sources = append(sources, srcRel)
	}

	maxBytes, maxFiles := s.effectiveLimits(p.MaxTotalBytes, p.MaxFiles)

	create := &proto.CreateArchiveParams{
		ArchivePath:      stripWorkPath(node.WorkPath, p.ArchivePath),
		Format:           p.Format,
		BasePath:         baseRel,
		Sources:          sources,
		CompressionLevel: p.CompressionLevel,
		Overwrite:        p.Overwrite,
		OwnerUser:        p.Owner.User,
		OwnerUid:         p.Owner.UID,
		OwnerGid:         p.Owner.GID,
		MaxTotalBytes:    maxBytes,
		MaxFiles:         maxFiles,
	}

	return s.startOp(ctx, node, ArchiveKindCreate, &proto.ArchiveRequest{
		Operation: &proto.ArchiveRequest_Create{Create: create},
	}, p.Options)
}

// sourceRelToBase rebases a work-directory-relative source path onto the
// base directory. A source equal to the base contributes the base's children
// ("." to the daemon); one outside the base has no representable entry name
// and is rejected.
func sourceRelToBase(baseRel, srcRel string) (string, error) {
	if baseRel == "." {
		return srcRel, nil
	}

	if srcRel == baseRel {
		return ".", nil
	}

	if rel, ok := strings.CutPrefix(srcRel, baseRel+"/"); ok {
		return rel, nil
	}

	return "", errors.Errorf("source %q is outside the base path %q", srcRel, baseRel)
}

func (s *ArchiveService) StartExtract(
	ctx context.Context, node *domain.Node, p ExtractArchiveParams,
) (string, error) {
	maxBytes, maxFiles := s.effectiveLimits(p.MaxTotalBytes, p.MaxFiles)

	extract := &proto.ExtractArchiveParams{
		ArchivePath:         stripWorkPath(node.WorkPath, p.ArchivePath),
		Destination:         stripWorkPath(node.WorkPath, p.Destination),
		Format:              p.Format,
		CreateDestination:   p.CreateDestination,
		ConflictPolicy:      p.ConflictPolicy,
		PreservePermissions: p.PreservePermissions,
		OwnerUser:           p.Owner.User,
		OwnerUid:            p.Owner.UID,
		OwnerGid:            p.Owner.GID,
		MaxTotalBytes:       maxBytes,
		MaxFiles:            maxFiles,
	}

	return s.startOp(ctx, node, ArchiveKindExtract, &proto.ArchiveRequest{
		Operation: &proto.ArchiveRequest_Extract{Extract: extract},
	}, p.Options)
}

func (s *ArchiveService) effectiveLimits(maxBytes uint64, maxFiles uint32) (uint64, uint32) {
	if maxBytes == 0 || (s.limits.MaxTotalBytes > 0 && maxBytes > s.limits.MaxTotalBytes) {
		maxBytes = s.limits.MaxTotalBytes
	}
	if maxFiles == 0 || (s.limits.MaxFiles > 0 && maxFiles > s.limits.MaxFiles) {
		maxFiles = s.limits.MaxFiles
	}

	return maxBytes, maxFiles
}

// normalizeArchiveTimeout applies the daemon-mirroring default for
// non-positive values and caps the rest, for both the local and the
// dispatched start paths.
func normalizeArchiveTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return archiveDefaultTimeout
	}
	if timeout > archiveMaxTimeout {
		return archiveMaxTimeout
	}

	return timeout
}

func (s *ArchiveService) startOp(
	ctx context.Context,
	node *domain.Node,
	kind ArchiveKind,
	req *proto.ArchiveRequest,
	opts ArchiveStartOptions,
) (string, error) {
	nodeID := uint64(node.ID)
	opID := idgen.New()

	timeout := normalizeArchiveTimeout(opts.Timeout)

	req.RequestId = opID
	req.Timeout = durationpb.New(timeout)
	if opts.ProgressInterval > 0 {
		req.ProgressInterval = durationpb.New(opts.ProgressInterval)
	}

	s.registerOp(opID, nodeID, kind, opts, timeout)

	if s.registry.IsConnected(nodeID) {
		if !s.registry.HasCapability(nodeID, capabilityArchive) {
			s.dropOp(opID)

			return "", ErrArchiveNotSupported
		}

		meta := archiveRelayMeta{nodeID: nodeID, serverID: opts.ServerID, kind: kind, initiator: opts.Initiator}
		s.addRelay(opID, meta)
		go s.runOwned(opID, meta, req, timeout) //nolint:gosec // intentionally outlives the request context

		return opID, nil
	}

	if !s.registry.IsConnectedAnywhere(nodeID) {
		s.dropOp(opID)

		return "", ErrDaemonNotConnected
	}

	data, err := req.MarshalVT()
	if err != nil {
		s.dropOp(opID)

		return "", errors.Wrap(err, "marshal archive request")
	}

	err = s.dispatchAndWaitAck(ctx, nodeID, messages.DaemonArchiveRequestPayload{
		NodeID:     nodeID,
		RequestID:  opID,
		InstanceID: s.instanceID,
		Action:     archiveActionStart,
		ServerID:   opts.ServerID,
		Kind:       string(kind),
		Initiator:  opts.Initiator,
		Data:       data,
	})
	if err != nil {
		s.dropOp(opID)
		if err.Error() == ErrArchiveNotSupported.Error() {
			return "", ErrArchiveNotSupported
		}

		return "", errors.WithMessage(err, "dispatch archive start")
	}

	return opID, nil
}

// Cancel is fire-and-forget: the outcome arrives as the operation's final
// event. Canceling an unknown or finished operation is a daemon-side no-op.
func (s *ArchiveService) Cancel(ctx context.Context, node *domain.Node, operationID, reason string) error {
	nodeID := uint64(node.ID)

	if s.registry.IsConnected(nodeID) {
		if err := s.gateway.RequestArchiveCancel(nodeID, operationID, reason); err != nil {
			return errors.WithMessage(err, "archive cancel")
		}

		return nil
	}

	if !s.registry.IsConnectedAnywhere(nodeID) {
		return ErrDaemonNotConnected
	}

	err := s.dispatchAndWaitAck(ctx, nodeID, messages.DaemonArchiveRequestPayload{
		NodeID:     nodeID,
		RequestID:  operationID,
		InstanceID: s.instanceID,
		Action:     archiveActionCancel,
		Reason:     reason,
	})
	if err != nil {
		return errors.WithMessage(err, "dispatch archive cancel")
	}

	return nil
}

// GetSnapshot reports an operation known to THIS instance: only the
// initiating instance holds registry entries, and they expire after
// archiveOpRetention past completion.
func (s *ArchiveService) GetSnapshot(operationID string) (ArchiveOpSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.ops[operationID]
	if !ok {
		return ArchiveOpSnapshot{}, false
	}

	snapshot := entry.snapshot
	if snapshot.Result != nil {
		result := *snapshot.Result
		snapshot.Result = &result
	}

	return snapshot, true
}

// WaitCompletion blocks until the operation publishes its final event, the
// ctx expires, or the registry entry is unknown. An already-completed
// operation resolves immediately from the stored snapshot.
func (s *ArchiveService) WaitCompletion(
	ctx context.Context, operationID string,
) (*messages.ArchiveCompleteEventPayload, error) {
	s.mu.Lock()
	entry, ok := s.ops[operationID]
	if !ok {
		s.mu.Unlock()

		return nil, ErrArchiveOperationNotFound
	}
	if entry.snapshot.Result != nil {
		result := *entry.snapshot.Result
		s.mu.Unlock()

		return &result, nil
	}

	waiter := make(chan *messages.ArchiveCompleteEventPayload, 1)
	entry.waiters = append(entry.waiters, waiter)
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-waiter:
		return result, nil
	}
}

// HandleArchiveProgress implements gateway.ArchiveProgressHandler on the
// session-owning instance.
func (s *ArchiveService) HandleArchiveProgress(
	_ context.Context, nodeID uint64, progress *proto.ArchiveProgress,
) error {
	meta, ok := s.relayMeta(progress.GetRequestId())
	if !ok {
		s.logger.Debug("archive progress for unknown operation",
			"node_id", nodeID,
			"request_id", progress.GetRequestId(),
		)

		return nil
	}

	s.publishEvent(messages.TypeArchiveProgress, progress.GetRequestId(), meta.serverID,
		messages.ArchiveProgressEventPayload{
			OperationID:    progress.GetRequestId(),
			NodeID:         meta.nodeID,
			ServerID:       meta.serverID,
			Kind:           string(meta.kind),
			FilesProcessed: progress.GetFilesProcessed(),
			FilesTotal:     progress.GetFilesTotal(),
			BytesProcessed: progress.GetBytesProcessed(),
			BytesTotal:     progress.GetBytesTotal(),
			CurrentEntry:   progress.GetCurrentEntry(),
		})

	return nil
}

// runOwned drives one operation on the session-owning instance: it blocks on
// the gateway until the daemon's single final response, then publishes the
// completion event for every interested instance.
func (s *ArchiveService) runOwned(
	opID string, meta archiveRelayMeta, req *proto.ArchiveRequest, timeout time.Duration,
) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout+archiveCompletionGrace)
	defer cancel()

	resp, err := s.gateway.RequestArchive(ctx, meta.nodeID, req)

	complete := messages.ArchiveCompleteEventPayload{
		OperationID: opID,
		NodeID:      meta.nodeID,
		ServerID:    meta.serverID,
		Kind:        string(meta.kind),
	}

	switch {
	case err != nil:
		complete.Error = err.Error()
	default:
		complete.Success = resp.GetSuccess()
		complete.Error = resp.GetError()
		complete.FilesProcessed = resp.GetFilesProcessed()
		complete.BytesProcessed = resp.GetBytesProcessed()
		complete.ArchiveSize = resp.GetArchiveSize()
		complete.SkippedCount = resp.GetSkippedCount()
		complete.Format = ArchiveFormatToAPIName(resp.GetFormat())

		skipped := resp.GetSkipped()
		if len(skipped) > archiveEventSkippedCap {
			skipped = skipped[:archiveEventSkippedCap]
		}
		complete.Skipped = skipped
	}

	s.removeRelay(opID)
	s.publishEvent(messages.TypeArchiveComplete, opID, meta.serverID, complete)
}

func (s *ArchiveService) handleDispatchRequest(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.DaemonArchiveRequestPayload](msg)
	if err != nil {
		s.logger.Warn("failed to parse archive request payload", "error", err)

		return nil
	}

	if !s.registry.IsConnected(payload.NodeID) {
		return nil
	}

	switch payload.Action {
	case archiveActionStart:
		s.ack(payload.InstanceID, payload.RequestID, s.startDispatched(payload))
	case archiveActionCancel:
		var ackErr string
		if err := s.gateway.RequestArchiveCancel(payload.NodeID, payload.RequestID, payload.Reason); err != nil {
			ackErr = err.Error()
		}
		s.ack(payload.InstanceID, payload.RequestID, ackErr)
	default:
		s.ack(payload.InstanceID, payload.RequestID, "unsupported archive action: "+payload.Action)
	}

	return nil
}

// startDispatched validates and launches a remotely-requested operation,
// returning the ack error text ("" = accepted). The launch happens in a
// goroutine so the ack stays within the dispatch cycle.
func (s *ArchiveService) startDispatched(payload messages.DaemonArchiveRequestPayload) string {
	if !s.registry.HasCapability(payload.NodeID, capabilityArchive) {
		return ErrArchiveNotSupported.Error()
	}

	req := &proto.ArchiveRequest{}
	if err := req.UnmarshalVT(payload.Data); err != nil {
		return "unmarshal archive request: " + err.Error()
	}

	timeout := normalizeArchiveTimeout(req.GetTimeout().AsDuration())

	meta := archiveRelayMeta{
		nodeID:    payload.NodeID,
		serverID:  payload.ServerID,
		kind:      ArchiveKind(payload.Kind),
		initiator: payload.Initiator,
	}
	if !s.tryAddRelay(payload.RequestID, meta) {
		// A redelivered start request: the operation is already running
		// here, launching it twice would collide on the daemon's request id.
		return ""
	}
	go s.runOwned(payload.RequestID, meta, req, timeout)

	return ""
}

func (s *ArchiveService) handleDispatchAck(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.DaemonArchiveResponsePayload](msg)
	if err != nil {
		s.logger.Warn("failed to parse archive ack payload", "error", err)

		return nil
	}

	s.mu.Lock()
	ackCh, ok := s.acks[payload.RequestID]
	s.mu.Unlock()

	if ok {
		select {
		case ackCh <- payload.Error:
		default:
		}
	}

	return nil
}

// handleOpEvent maintains the initiating instance's registry from the
// per-operation event stream; events of operations started elsewhere are
// ignored.
func (s *ArchiveService) handleOpEvent(_ context.Context, msg *pubsub.Message) error {
	switch msg.Type {
	case messages.TypeArchiveProgress:
		payload, err := messages.ParsePayload[messages.ArchiveProgressEventPayload](msg)
		if err != nil {
			return nil //nolint:nilerr // a malformed event must not kill the subscription
		}

		s.mu.Lock()
		if entry, ok := s.ops[payload.OperationID]; ok {
			entry.snapshot.Progress = payload
			entry.snapshot.UpdatedAt = time.Now()
		}
		s.mu.Unlock()

	case messages.TypeArchiveComplete:
		payload, err := messages.ParsePayload[messages.ArchiveCompleteEventPayload](msg)
		if err != nil {
			return nil //nolint:nilerr // a malformed event must not kill the subscription
		}

		var waiters []chan *messages.ArchiveCompleteEventPayload

		s.mu.Lock()
		if entry, ok := s.ops[payload.OperationID]; ok {
			result := payload
			entry.snapshot.Result = &result
			entry.snapshot.Status = ArchiveOpDone
			if !payload.Success {
				entry.snapshot.Status = ArchiveOpError
			}
			entry.snapshot.UpdatedAt = time.Now()
			entry.expiresAt = time.Now().Add(archiveOpRetention)
			waiters = entry.waiters
			entry.waiters = nil
		}
		s.mu.Unlock()

		for _, waiter := range waiters {
			result := payload
			select {
			case waiter <- &result:
			default:
			}
		}
	}

	return nil
}

func (s *ArchiveService) dispatchAndWaitAck(
	ctx context.Context, nodeID uint64, payload messages.DaemonArchiveRequestPayload,
) error {
	ackCh := make(chan string, 1)

	s.mu.Lock()
	s.acks[payload.RequestID] = ackCh
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.acks, payload.RequestID)
		s.mu.Unlock()
	}()

	channel := channels.BuildDaemonArchiveRequestChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonArchiveRequest, payload)
	if err != nil {
		return errors.WithMessage(err, "create archive request message")
	}

	if err := s.ps.Publish(ctx, channel, msg); err != nil {
		return errors.Wrap(err, "publish archive request")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(archiveAckTimeout):
		return errors.New("archive dispatch timed out")
	case ackErr := <-ackCh:
		if ackErr != "" {
			return errors.New(ackErr)
		}

		return nil
	}
}

func (s *ArchiveService) ack(instanceID, requestID, ackErr string) {
	channel := channels.BuildDaemonArchiveResponseChannel(instanceID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonArchiveResponse,
		messages.DaemonArchiveResponsePayload{RequestID: requestID, Error: ackErr})
	if err != nil {
		s.logger.Error("failed to create archive ack message", "error", err)

		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.ps.Publish(ctx, channel, msg); err != nil {
		s.logger.Error("failed to publish archive ack",
			"request_id", requestID,
			"error", err,
		)
	}
}

func (s *ArchiveService) publishEvent(msgType, opID string, serverID uint, payload any) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	targets := []string{channels.BuildRealtimeArchiveOpChannel(opID)}
	if serverID != 0 {
		targets = append(targets, channels.BuildRealtimeFMArchiveChannel(uint64(serverID)))
	}

	for _, channel := range targets {
		msg, err := messages.NewMessage(channel, msgType, payload)
		if err != nil {
			s.logger.Error("failed to create archive event message", "error", err)

			continue
		}

		if err := s.ps.Publish(ctx, channel, msg); err != nil {
			s.logger.Error("failed to publish archive event",
				"channel", channel,
				"operation_id", opID,
				"error", err,
			)
		}
	}
}

func (s *ArchiveService) registerOp(
	opID string, nodeID uint64, kind ArchiveKind, opts ArchiveStartOptions, timeout time.Duration,
) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ops[opID] = &archiveOpEntry{
		snapshot: ArchiveOpSnapshot{
			OperationID: opID,
			NodeID:      nodeID,
			ServerID:    opts.ServerID,
			Kind:        kind,
			Initiator:   opts.Initiator,
			Status:      ArchiveOpRunning,
			StartedAt:   now,
			UpdatedAt:   now,
		},
		expiresAt: now.Add(timeout + archiveCompletionGrace + archiveOpRetention),
	}
}

func (s *ArchiveService) dropOp(opID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.ops, opID)
}

func (s *ArchiveService) addRelay(opID string, meta archiveRelayMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.relays[opID] = meta
}

// tryAddRelay inserts the relay entry unless one already exists, making
// duplicate dispatch deliveries a no-op.
func (s *ArchiveService) tryAddRelay(opID string, meta archiveRelayMeta) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.relays[opID]; exists {
		return false
	}
	s.relays[opID] = meta

	return true
}

func (s *ArchiveService) removeRelay(opID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.relays, opID)
}

func (s *ArchiveService) relayMeta(opID string) (archiveRelayMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, ok := s.relays[opID]

	return meta, ok
}

func (s *ArchiveService) janitor(ctx context.Context) {
	ticker := time.NewTicker(archiveJanitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepExpired(time.Now())
		}
	}
}

func (s *ArchiveService) sweepExpired(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for opID, entry := range s.ops {
		if now.After(entry.expiresAt) {
			delete(s.ops, opID)
		}
	}
}

var archiveFormatAPINames = map[proto.ArchiveFormat]string{
	proto.ArchiveFormat_ARCHIVE_FORMAT_ZIP:      "zip",
	proto.ArchiveFormat_ARCHIVE_FORMAT_TAR:      "tar",
	proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ:   "tar_gz",
	proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_BZ2:  "tar_bz2",
	proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_XZ:   "tar_xz",
	proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_ZSTD: "tar_zstd",
	proto.ArchiveFormat_ARCHIVE_FORMAT_GZ:       "gz",
	proto.ArchiveFormat_ARCHIVE_FORMAT_BZ2:      "bz2",
	proto.ArchiveFormat_ARCHIVE_FORMAT_XZ:       "xz",
	proto.ArchiveFormat_ARCHIVE_FORMAT_ZSTD:     "zstd",
	proto.ArchiveFormat_ARCHIVE_FORMAT_7Z:       "7z",
	proto.ArchiveFormat_ARCHIVE_FORMAT_RAR:      "rar",
}

var archiveFormatsByAPIName = func() map[string]proto.ArchiveFormat {
	m := make(map[string]proto.ArchiveFormat, len(archiveFormatAPINames))
	for format, name := range archiveFormatAPINames {
		m[name] = format
	}

	return m
}()

// ArchiveFormatToAPIName maps a proto format onto its lower_snake API name;
// unspecified/unknown formats map to "".
func ArchiveFormatToAPIName(format proto.ArchiveFormat) string {
	return archiveFormatAPINames[format]
}

// ArchiveFormatFromAPIName resolves a lower_snake API name; ok is false for
// unknown names.
func ArchiveFormatFromAPIName(name string) (proto.ArchiveFormat, bool) {
	format, ok := archiveFormatsByAPIName[name]

	return format, ok
}
