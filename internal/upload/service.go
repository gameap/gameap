package upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/pkg/errors"
)

type Config struct {
	ChunkSize             uint64
	SessionTTL            time.Duration
	MaxChunks             uint
	DaemonDispatchTimeout time.Duration
}

type CreateParams struct {
	ServerID         uint
	NodeID           uint
	UserID           uint
	FullPath         string
	SuUser           string
	TotalSize        uint64
	ExpectedChecksum string
}

type SessionStatus struct {
	Session        *Session
	ReceivedChunks []uint
	MissingChunks  []uint
	UploadedBytes  uint64
	Completed      bool
}

type Service struct {
	storage Storage
	daemon  DaemonUploader
	clock   Clock
	logger  *slog.Logger
	cfg     Config
}

func NewService(storage Storage, daemon DaemonUploader, clock Clock, logger *slog.Logger, cfg Config) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = realClock{}
	}

	return &Service{
		storage: storage,
		daemon:  daemon,
		clock:   clock,
		logger:  logger,
		cfg:     cfg,
	}
}

func (s *Service) Create(ctx context.Context, p CreateParams) (*Session, error) {
	if p.TotalSize == 0 {
		return nil, ErrInvalidTotalSize
	}
	if !isHexSHA256(p.ExpectedChecksum) {
		return nil, ErrInvalidChecksum
	}

	chunkSize := s.cfg.ChunkSize
	if chunkSize == 0 {
		return nil, errors.New("chunk size is not configured")
	}

	totalChunks := uint((p.TotalSize + chunkSize - 1) / chunkSize)
	if totalChunks == 0 {
		totalChunks = 1
	}
	if s.cfg.MaxChunks > 0 && totalChunks > s.cfg.MaxChunks {
		return nil, ErrTooManyChunks
	}

	now := s.clock.Now().UTC()
	sess := &Session{
		UploadID:         idgen.New(),
		ServerID:         p.ServerID,
		NodeID:           p.NodeID,
		UserID:           p.UserID,
		FullPath:         p.FullPath,
		SuUser:           p.SuUser,
		TotalSize:        p.TotalSize,
		ChunkSize:        chunkSize,
		TotalChunks:      totalChunks,
		ExpectedChecksum: strings.ToLower(p.ExpectedChecksum),
		CreatedAt:        now,
		ExpiresAt:        now.Add(s.cfg.SessionTTL),
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return nil, errors.Wrap(err, "marshal session metadata")
	}
	if writeErr := s.storage.Write(ctx, metadataPath(sess.UploadID), data); writeErr != nil {
		return nil, errors.WithMessage(writeErr, "write session metadata")
	}

	return sess, nil
}

func (s *Service) WriteChunk(
	ctx context.Context,
	uploadID string,
	userID uint,
	index uint,
	body io.Reader,
) error {
	sess, err := s.load(ctx, uploadID)
	if err != nil {
		return err
	}
	if err := s.verifyAccess(sess, userID); err != nil {
		return err
	}
	if s.storage.Exists(ctx, donePath(uploadID)) {
		return ErrSessionAlreadyDone
	}
	if index >= sess.TotalChunks {
		return ErrInvalidIndex
	}

	expectedSize := sess.ChunkSizeFor(index)
	limited := io.LimitReader(body, safeChunkLimit(expectedSize))
	counter := &countingReader{reader: limited}

	cp := chunkPath(uploadID, index)
	if writeErr := s.storage.WriteStream(ctx, cp, counter); writeErr != nil {
		_ = s.storage.Delete(context.Background(), cp)

		return errors.WithMessage(writeErr, "write chunk")
	}

	if counter.count < 0 || uint64(counter.count) != expectedSize {
		_ = s.storage.Delete(context.Background(), cp)

		return ErrChunkSizeMismatch
	}

	return nil
}

func safeChunkLimit(expectedSize uint64) int64 {
	if expectedSize >= math.MaxInt64 {
		return math.MaxInt64
	}

	return int64(expectedSize) + 1
}

func (s *Service) Status(ctx context.Context, uploadID string, userID uint) (*SessionStatus, error) {
	sess, err := s.load(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if err := s.verifyAccess(sess, userID); err != nil {
		return nil, err
	}

	received, err := s.receivedChunks(ctx, sess.UploadID)
	if err != nil {
		return nil, err
	}

	receivedList := make([]uint, 0, len(received))
	missingList := make([]uint, 0, sess.TotalChunks)
	var uploadedBytes uint64
	for i := uint(0); i < sess.TotalChunks; i++ {
		if _, ok := received[i]; ok {
			receivedList = append(receivedList, i)
			uploadedBytes += sess.ChunkSizeFor(i)
		} else {
			missingList = append(missingList, i)
		}
	}
	slices.Sort(receivedList)

	return &SessionStatus{
		Session:        sess,
		ReceivedChunks: receivedList,
		MissingChunks:  missingList,
		UploadedBytes:  uploadedBytes,
		Completed:      s.storage.Exists(ctx, donePath(uploadID)),
	}, nil
}

func (s *Service) Complete(
	ctx context.Context,
	uploadID string,
	userID uint,
	node *domain.Node,
) error {
	sess, err := s.load(ctx, uploadID)
	if err != nil {
		return err
	}
	if err := s.verifyAccess(sess, userID); err != nil {
		return err
	}
	if node == nil || node.ID != sess.NodeID {
		return ErrNodeMismatch
	}

	if s.storage.Exists(ctx, donePath(uploadID)) {
		return nil
	}

	received, err := s.receivedChunks(ctx, uploadID)
	if err != nil {
		return err
	}
	if uint(len(received)) != sess.TotalChunks {
		return ErrIncompleteUpload
	}
	for i := uint(0); i < sess.TotalChunks; i++ {
		if _, ok := received[i]; !ok {
			return ErrIncompleteUpload
		}
	}

	readers := make([]io.Reader, sess.TotalChunks)
	for i := uint(0); i < sess.TotalChunks; i++ {
		readers[i] = newLazyChunkReader(ctx, s.storage, chunkPath(uploadID, i))
	}

	hasher := sha256.New()
	teeReader := io.TeeReader(io.MultiReader(readers...), hasher)

	dp := dataPath(uploadID)
	if writeErr := s.storage.WriteStream(ctx, dp, teeReader); writeErr != nil {
		_ = s.storage.Delete(context.Background(), dp)

		return errors.WithMessage(writeErr, "assemble upload data")
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != sess.ExpectedChecksum {
		s.logger.Error(
			"upload assemble checksum mismatch",
			"upload_id", uploadID,
			"expected_checksum", sess.ExpectedChecksum,
			"assembled_checksum", actualChecksum,
			"total_chunks", sess.TotalChunks,
		)

		_ = s.storage.Delete(context.Background(), dp)

		return ErrChecksumMismatch
	}

	dispatchCtx := ctx
	if s.cfg.DaemonDispatchTimeout > 0 {
		var cancel context.CancelFunc
		dispatchCtx, cancel = context.WithTimeout(ctx, s.cfg.DaemonDispatchTimeout)
		defer cancel()
	}

	if dispatchErr := s.daemon.UploadStreamPrepared(
		dispatchCtx, node, sess.FullPath, sess.UploadID, actualChecksum, sess.TotalSize,
		daemon.OwnerOptions{User: sess.SuUser},
	); dispatchErr != nil {
		s.logger.Error(
			"upload daemon dispatch failed",
			"upload_id", uploadID,
			"node_id", node.ID,
			"full_path", sess.FullPath,
			"expected_checksum", sess.ExpectedChecksum,
			"assembled_checksum", actualChecksum,
			"total_size", sess.TotalSize,
			"error", dispatchErr,
		)

		_ = s.storage.Delete(context.Background(), dp)

		return errors.WithMessage(dispatchErr, "dispatch upload to daemon")
	}

	doneInfo, marshalErr := json.Marshal(DoneInfo{Success: true, Checksum: actualChecksum})
	if marshalErr != nil {
		s.logger.Warn("failed to marshal done sentinel", "upload_id", uploadID, "error", marshalErr)
	} else if writeErr := s.storage.Write(ctx, donePath(uploadID), doneInfo); writeErr != nil {
		s.logger.Warn("failed to write done sentinel", "upload_id", uploadID, "error", writeErr)
	}

	// Cleanup runs in a detached goroutine using context.Background because the cleanup
	// must finish even if the request context is cancelled — the upload is already done.
	go s.cleanupAfterComplete(uploadID) //nolint:gosec

	return nil
}

func (s *Service) Abort(ctx context.Context, uploadID string, userID uint) error {
	sess, err := s.load(ctx, uploadID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}

		return err
	}
	if err := s.verifyAccess(sess, userID); err != nil {
		return err
	}
	if delErr := s.storage.DeleteByPrefix(ctx, transferRoot(uploadID)); delErr != nil {
		return errors.WithMessage(delErr, "delete upload session")
	}

	return nil
}

func (s *Service) load(ctx context.Context, uploadID string) (*Session, error) {
	raw, err := s.storage.Read(ctx, metadataPath(uploadID))
	if err != nil {
		return nil, ErrSessionNotFound
	}
	var sess Session
	if unmarshalErr := json.Unmarshal(raw, &sess); unmarshalErr != nil {
		return nil, errors.Wrap(unmarshalErr, "unmarshal session metadata")
	}

	return &sess, nil
}

func (s *Service) verifyAccess(sess *Session, userID uint) error {
	if sess.UserID != userID {
		return ErrSessionForbidden
	}
	if !sess.ExpiresAt.IsZero() && s.clock.Now().After(sess.ExpiresAt) {
		return ErrSessionExpired
	}

	return nil
}

func (s *Service) receivedChunks(ctx context.Context, uploadID string) (map[uint]struct{}, error) {
	paths, err := s.storage.List(ctx, chunksPrefix(uploadID))
	if err != nil {
		return nil, errors.WithMessage(err, "list uploaded chunks")
	}
	received := make(map[uint]struct{}, len(paths))
	for _, p := range paths {
		if idx, ok := indexFromChunkPath(uploadID, p); ok {
			received[idx] = struct{}{}
		}
	}

	return received, nil
}

func (s *Service) cleanupAfterComplete(uploadID string) {
	bg := context.Background()
	if err := s.storage.DeleteByPrefix(bg, chunksPrefix(uploadID)); err != nil {
		s.logger.Warn("failed to cleanup chunks", "upload_id", uploadID, "error", err)
	}
	if err := s.storage.Delete(bg, dataPath(uploadID)); err != nil {
		s.logger.Warn("failed to delete assembled data", "upload_id", uploadID, "error", err)
	}
}

func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}

	return true
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.count += int64(n)

	return n, err
}
