// Package pluginarchive delivers archive operation events into wasm plugins.
// Registration happens from nodefs host functions, which may run while the
// plugin manager lock is held (guest Initialize/Shutdown), so this path never
// touches PluginProvider — the plugin is resolved at delivery time, on a
// dedicated goroutine per operation.
package pluginarchive

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/pkg/errors"
)

const (
	defaultProgressCallTimeout   = 10 * time.Second
	defaultCompletionCallTimeout = 30 * time.Second
	defaultBusyRetryDelay        = 2 * time.Second
	defaultBusyRetries           = 5
)

type Options struct {
	ProgressCallTimeout   time.Duration
	CompletionCallTimeout time.Duration
	BusyRetryDelay        time.Duration
	BusyRetries           int
}

func (o Options) withDefaults() Options {
	if o.ProgressCallTimeout <= 0 {
		o.ProgressCallTimeout = defaultProgressCallTimeout
	}
	if o.CompletionCallTimeout <= 0 {
		o.CompletionCallTimeout = defaultCompletionCallTimeout
	}
	if o.BusyRetryDelay <= 0 {
		o.BusyRetryDelay = defaultBusyRetryDelay
	}
	if o.BusyRetries <= 0 {
		o.BusyRetries = defaultBusyRetries
	}

	return o
}

// registration tracks one plugin-initiated operation. The progress channel
// keeps only the latest event (drop-and-replace): a slow guest sees fewer,
// fresher updates. The completion channel is capacity 1 and never dropped.
type registration struct {
	pluginID       uint64
	nodeID         uint64
	reportProgress bool
	progressCh     chan messages.ArchiveProgressEventPayload
	completeCh     chan messages.ArchiveCompleteEventPayload
	stopCh         chan struct{}
	stopOnce       sync.Once
}

func (r *registration) stop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

type Service struct {
	plugins  PluginProvider
	resolver ManagerIDResolver
	sub      pubsub.Subscriber
	opts     Options
	logger   *slog.Logger

	mu      sync.Mutex
	regs    map[string]*registration
	baseCtx context.Context
}

func New(
	plugins PluginProvider,
	resolver ManagerIDResolver,
	sub pubsub.Subscriber,
	opts Options,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	return &Service{
		plugins:  plugins,
		resolver: resolver,
		sub:      sub,
		opts:     opts.withDefaults(),
		logger:   logger,
		regs:     make(map[string]*registration),
		baseCtx:  context.Background(),
	}
}

func (s *Service) Start(ctx context.Context) error {
	s.baseCtx = ctx

	if err := s.sub.Subscribe(ctx, channels.RealtimeArchiveOpAll, s.onMessage); err != nil {
		return errors.Wrap(err, "subscribe to archive operation events")
	}

	s.logger.Info("plugin archive event dispatcher started")

	return nil
}

// Register records the plugin's interest in an operation and spawns its
// delivery goroutine. Called from host functions, possibly under the plugin
// manager lock — it must never resolve plugins.
func (s *Service) Register(pluginID uint64, operationID string, nodeID uint64, reportProgress bool) {
	if pluginID == 0 || operationID == "" {
		return
	}

	reg := &registration{
		pluginID:       pluginID,
		nodeID:         nodeID,
		reportProgress: reportProgress,
		progressCh:     make(chan messages.ArchiveProgressEventPayload, 1),
		completeCh:     make(chan messages.ArchiveCompleteEventPayload, 1),
		stopCh:         make(chan struct{}),
	}

	s.mu.Lock()
	s.regs[operationID] = reg
	s.mu.Unlock()

	go s.deliverLoop(operationID, reg)
}

// RemovePlugin drops every registration of the plugin; used on uninstall so
// stale deliveries cannot reach a freshly reinstalled instance.
func (s *Service) RemovePlugin(pluginID uint64) {
	s.mu.Lock()
	for operationID, reg := range s.regs {
		if reg.pluginID == pluginID {
			reg.stop()
			delete(s.regs, operationID)
		}
	}
	s.mu.Unlock()
}

func (s *Service) lookup(operationID string) (*registration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reg, ok := s.regs[operationID]

	return reg, ok
}

func (s *Service) drop(operationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.regs, operationID)
}

// onMessage feeds the per-operation channels; per the pubsub.Handler
// contract it never blocks.
func (s *Service) onMessage(_ context.Context, msg *pubsub.Message) error {
	switch msg.Type {
	case messages.TypeArchiveProgress:
		payload, err := messages.ParsePayload[messages.ArchiveProgressEventPayload](msg)
		if err != nil {
			return nil //nolint:nilerr // a malformed event must not kill the subscription
		}

		reg, ok := s.lookup(payload.OperationID)
		if !ok || !reg.reportProgress {
			return nil
		}

		for {
			select {
			case reg.progressCh <- payload:
				return nil
			default:
			}
			select {
			case <-reg.progressCh:
			default:
			}
		}

	case messages.TypeArchiveComplete:
		payload, err := messages.ParsePayload[messages.ArchiveCompleteEventPayload](msg)
		if err != nil {
			return nil //nolint:nilerr // a malformed event must not kill the subscription
		}

		reg, ok := s.lookup(payload.OperationID)
		if !ok {
			return nil
		}

		select {
		case reg.completeCh <- payload:
		default:
		}
	}

	return nil
}

func (s *Service) deliverLoop(operationID string, reg *registration) {
	defer s.drop(operationID)

	for {
		select {
		case <-reg.stopCh:
			return
		case <-s.baseCtx.Done():
			return
		case payload := <-reg.progressCh:
			s.deliverProgress(reg, payload)
		case payload := <-reg.completeCh:
			s.deliverComplete(reg, payload)

			return
		}
	}
}

func (s *Service) deliverProgress(reg *registration, payload messages.ArchiveProgressEventPayload) {
	lp, handler, ok := s.resolveHandler(reg.pluginID)
	if !ok {
		return
	}

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(s.baseCtx), s.opts.ProgressCallTimeout)
	defer cancel()

	_, err := handler.HandleArchiveProgress(callCtx, &nodefs.HandleArchiveProgressRequest{
		OperationId:    payload.OperationID,
		NodeId:         payload.NodeID,
		FilesProcessed: payload.FilesProcessed,
		FilesTotal:     payload.FilesTotal,
		BytesProcessed: payload.BytesProcessed,
		BytesTotal:     payload.BytesTotal,
		CurrentEntry:   payload.CurrentEntry,
	})
	if err == nil {
		return
	}

	if errors.Is(err, pkgplugin.ErrPluginBusy) {
		// The guest was never entered; the next progress event supersedes
		// this one anyway.
		return
	}

	s.handleGuestCallError(callCtx, reg, lp, err, "archive progress")
}

func (s *Service) deliverComplete(reg *registration, payload messages.ArchiveCompleteEventPayload) {
	lp, handler, ok := s.resolveHandler(reg.pluginID)
	if !ok {
		s.logger.Debug("archive completion has no reachable handler",
			slog.Uint64("plugin_id", reg.pluginID),
			slog.String("operation_id", payload.OperationID))

		return
	}

	request := &nodefs.HandleArchiveCompletedRequest{
		OperationId:    payload.OperationID,
		NodeId:         payload.NodeID,
		Success:        payload.Success,
		FilesProcessed: payload.FilesProcessed,
		BytesProcessed: payload.BytesProcessed,
		ArchiveSize:    payload.ArchiveSize,
		SkippedCount:   payload.SkippedCount,
		Format:         nodefs.ArchiveFormatFromAPIName(payload.Format),
	}
	if payload.Error != "" {
		request.Error = new(payload.Error)
	}

	attempts := 1 + s.opts.BusyRetries
	for attempt := range attempts {
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(s.baseCtx), s.opts.CompletionCallTimeout)
		_, err := handler.HandleArchiveCompleted(callCtx, request)
		cancel()

		if err == nil {
			return
		}

		if !errors.Is(err, pkgplugin.ErrPluginBusy) {
			s.handleGuestCallError(callCtx, reg, lp, err, "archive completion")

			return
		}

		if attempt == attempts-1 {
			// The completion stays observable through GetArchiveOperation.
			s.logger.Error("plugin stayed busy, archive completion callback dropped",
				slog.Uint64("plugin_id", reg.pluginID),
				slog.String("operation_id", payload.OperationID))

			return
		}

		select {
		case <-reg.stopCh:
			return
		case <-s.baseCtx.Done():
			return
		case <-time.After(s.opts.BusyRetryDelay):
		}
	}
}

// handleGuestCallError mirrors the scheduler policy: a deadline hit inside
// the guest closes the module (WithCloseOnContextDone), so the plugin is
// disabled until reload; any other guest error is only logged.
func (s *Service) handleGuestCallError(
	callCtx context.Context, reg *registration, lp *pkgplugin.LoadedPlugin, err error, kind string,
) {
	if callCtx.Err() != nil && s.baseCtx.Err() == nil {
		lp.Disable()
		s.logger.Error(kind+" callback timed out, plugin disabled until reload",
			slog.Uint64("plugin_id", reg.pluginID),
			slog.String("error", err.Error()))

		return
	}

	s.logger.Warn(kind+" callback failed",
		slog.Uint64("plugin_id", reg.pluginID),
		slog.String("error", err.Error()))
}

func (s *Service) resolveHandler(pluginID uint64) (*pkgplugin.LoadedPlugin, nodefs.ArchiveEventsHandler, bool) {
	managerID, ok := s.resolver.GetPluginManagerID(domain.Uint64ID(pluginID))
	if !ok {
		managerID = pkgplugin.CompactPluginID(domain.Uint64ID(pluginID))
	}

	lp, ok := s.plugins.GetPlugin(managerID)
	if !ok || !lp.IsEnabled() || !lp.HasArchiveEventsHandler() {
		return nil, nil, false
	}

	handler, ok := lp.Instance.(nodefs.ArchiveEventsHandler)
	if !ok {
		return nil, nil, false
	}

	return lp, handler, true
}
