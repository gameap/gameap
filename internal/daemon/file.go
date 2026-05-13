package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/transfers"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/gameap/gameap/pkg/validation"
	"github.com/pkg/errors"
)

const (
	smallFileThreshold     = 1 * 1024 * 1024 // 1MB
	chunkSize              = 64 * 1024       // 64KB
	transferPrefix         = "transfers/"
	capabilityFileTransfer = "file_transfer"
	s3PollInterval         = 200 * time.Millisecond
	initialPartTimeout     = 2 * time.Minute

	legacyUploadDefaultPerms  os.FileMode = 0o644
	legacyUploadMaxBufferSize             = 256 * 1024 * 1024 // 256 MiB
	// legacyUploadOverallTimeout caps the legacy upload independent of the
	// caller's ctx deadline (which is typically DaemonDispatchTimeout=2m and
	// would kill slow networks before they finish). Idle-timeout inside
	// legacy.UploadStream is the primary stall detector; this is a hard
	// safety cap for runaway uploads.
	legacyUploadOverallTimeout = 2 * time.Hour
)

type daemonNotConnectedError struct{}

func (e *daemonNotConnectedError) Error() string   { return "daemon not connected" }
func (e *daemonNotConnectedError) HTTPStatus() int { return http.StatusBadGateway }

var ErrDaemonNotConnected error = &daemonNotConnectedError{}

type FileService struct {
	gateway     FileGateway
	registry    ConnectionChecker
	dispatcher  FileDispatcher
	storage     files.StreamFileManager
	transferReg *transfers.Registry
	legacy      *FileBINNService
	logger      *slog.Logger
}

func NewFileService(
	gateway FileGateway,
	registry ConnectionChecker,
	dispatcher FileDispatcher,
	storage files.StreamFileManager,
	transferReg *transfers.Registry,
	legacy *FileBINNService,
	logger *slog.Logger,
) *FileService {
	if logger == nil {
		logger = slog.Default()
	}

	return &FileService{
		gateway:     gateway,
		registry:    registry,
		dispatcher:  dispatcher,
		storage:     storage,
		transferReg: transferReg,
		legacy:      legacy,
		logger:      logger,
	}
}

func (s *FileService) resolveRoute(nodeID uint64) (local bool, err error) {
	if s.registry.IsConnected(nodeID) {
		return true, nil
	}

	if !s.registry.IsConnectedAnywhere(nodeID) {
		return false, ErrDaemonNotConnected
	}

	return false, nil
}

func (s *FileService) ReadDir(
	ctx context.Context,
	node *domain.Node,
	directory string,
) ([]*FileInfo, error) {
	return s.readDir(ctx, node, directory, false)
}

func (s *FileService) ReadDirRecursive(
	ctx context.Context,
	node *domain.Node,
	directory string,
) ([]*FileInfo, error) {
	return s.readDir(ctx, node, directory, true)
}

func (s *FileService) readDir(
	ctx context.Context,
	node *domain.Node,
	directory string,
	recursive bool,
) ([]*FileInfo, error) {
	nodeID := uint64(node.ID)
	relDir := stripWorkPath(node.WorkPath, directory)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		if s.legacy != nil && !recursive {
			return s.legacy.ReadDir(ctx, node, directory)
		}
		if s.legacy != nil && recursive {
			return s.legacy.ReadDirRecursive(ctx, node, directory)
		}

		return nil, err
	}

	var resp *proto.FileListResponse
	if local {
		resp, err = s.gateway.RequestFileList(ctx, nodeID, relDir, recursive, "")
	} else {
		resp, err = s.dispatcher.DispatchFileList(ctx, nodeID, relDir, recursive, "")
	}
	if err != nil {
		return nil, errors.WithMessage(err, "file list request")
	}

	if !resp.Success {
		return nil, errors.Errorf("file list failed: %s", resp.Error)
	}

	result := make([]*FileInfo, 0, len(resp.Files))
	for _, f := range resp.Files {
		result = append(result, protoFileStatToFileInfo(f))
	}

	return result, nil
}

func (s *FileService) Download(ctx context.Context, node *domain.Node, filePath string) ([]byte, error) {
	nodeID := uint64(node.ID)
	relPath := stripWorkPath(node.WorkPath, filePath)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		if s.legacy != nil {
			return s.legacy.Download(ctx, node, filePath)
		}

		return nil, err
	}

	if local {
		resp, readErr := s.gateway.RequestFileRead(ctx, nodeID, relPath, 0, 0)
		if readErr != nil {
			return nil, errors.WithMessage(readErr, "file read request")
		}

		if !resp.Success {
			return nil, errors.Errorf("file read failed: %s", resp.Error)
		}

		return resp.Content, nil
	}

	result, err := s.dispatcher.DispatchFileRead(ctx, nodeID, relPath, 0, 0)
	if err != nil {
		return nil, errors.WithMessage(err, "dispatched file read")
	}

	if result.Content != nil {
		return result.Content, nil
	}

	data, err := s.storage.Read(ctx, result.StoragePath)
	if err != nil {
		return nil, errors.WithMessage(err, "reading transferred file from storage")
	}
	_ = s.storage.Delete(context.Background(), result.StoragePath)

	return data, nil
}

func (s *FileService) DownloadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
) (io.ReadCloser, error) {
	nodeID := uint64(node.ID)
	relPath := stripWorkPath(node.WorkPath, filePath)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		if s.legacy != nil {
			return s.legacy.DownloadStream(ctx, node, filePath)
		}

		return nil, err
	}

	if local {
		return s.downloadStreamLocal(ctx, nodeID, relPath)
	}

	return s.downloadStreamRemote(ctx, nodeID, relPath)
}

func (s *FileService) downloadStreamLocal(ctx context.Context, nodeID uint64, path string) (io.ReadCloser, error) {
	if !s.registry.HasCapability(nodeID, capabilityFileTransfer) {
		return s.downloadStreamLocalChunked(ctx, nodeID, path), nil
	}

	transferID := idgen.New()
	state := s.transferReg.Register(transferID)

	go func() {
		if err := s.gateway.RequestFileDownloadTask(ctx, nodeID, transferID, path); err != nil {
			state.SetError(errors.WithMessage(err, "daemon file download task"))

			return
		}
		state.Complete()
	}()

	waitCtx, waitCancel := context.WithTimeout(ctx, initialPartTimeout)
	defer waitCancel()

	available, err := state.WaitForPart(waitCtx, 0)
	if err != nil {
		s.transferReg.Unregister(transferID)

		return nil, errors.WithMessage(err, "gateway download task")
	}
	if !available {
		s.transferReg.Unregister(transferID)

		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		for partNum := 0; ; partNum++ {
			avail, waitErr := state.WaitForPart(ctx, partNum)
			if waitErr != nil {
				pw.CloseWithError(waitErr)

				return
			}
			if !avail {
				return
			}

			partPath := transfers.TransferPartPath(transferID, partNum)

			reader, readErr := s.storage.ReadStream(ctx, partPath)
			if readErr != nil {
				s.logger.Error("failed to read transfer part",
					"transfer_id", transferID, "part", partNum, "error", readErr)
				pw.CloseWithError(errors.Wrapf(readErr, "read part %d", partNum))

				return
			}

			if _, copyErr := io.Copy(pw, reader); copyErr != nil {
				_ = reader.Close()
				s.logger.Error("failed to stream part to pipe",
					"transfer_id", transferID, "part", partNum, "error", copyErr)
				pw.CloseWithError(errors.Wrapf(copyErr, "stream part %d", partNum))

				return
			}
			_ = reader.Close()

			if delErr := s.storage.Delete(ctx, partPath); delErr != nil {
				s.logger.Warn("failed to delete consumed part",
					"transfer_id", transferID, "part", partNum, "error", delErr)
			}
		}
	}()

	return &chunkedCleanupReadCloser{
		ReadCloser:  pr,
		transferID:  transferID,
		storage:     s.storage,
		transferReg: s.transferReg,
		logger:      s.logger,
	}, nil
}

func (s *FileService) downloadStreamLocalChunked(
	ctx context.Context, nodeID uint64, path string,
) io.ReadCloser {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		var offset int64

		for {
			select {
			case <-ctx.Done():
				pw.CloseWithError(ctx.Err())

				return
			default:
			}

			resp, err := s.gateway.RequestFileRead(ctx, nodeID, path, offset, int64(chunkSize))
			if err != nil {
				pw.CloseWithError(errors.WithMessage(err, "gateway chunked read"))

				return
			}

			if !resp.Success {
				pw.CloseWithError(errors.Errorf("gateway chunked read: %s", resp.Error))

				return
			}

			if len(resp.Content) == 0 {
				return
			}

			if _, err := pw.Write(resp.Content); err != nil {
				return
			}

			offset += int64(len(resp.Content))

			if int64(len(resp.Content)) < int64(chunkSize) {
				return
			}
		}
	}()

	return pr
}

func (s *FileService) downloadStreamRemote(ctx context.Context, nodeID uint64, path string) (io.ReadCloser, error) {
	transferID := idgen.New()

	errCh := make(chan error, 1)
	go func() {
		if err := s.dispatcher.DispatchDownloadTask(ctx, nodeID, transferID, path); err != nil {
			errCh <- errors.WithMessage(err, "dispatched download task")

			return
		}
		s.writeSentinelIfMissing(ctx, transferID)
	}()

	waitCtx, waitCancel := context.WithTimeout(ctx, initialPartTimeout)
	defer waitCancel()

	available, err := s.waitForPartS3(waitCtx, transferID, 0, errCh)
	if err != nil {
		return nil, errors.WithMessage(err, "remote download task")
	}
	if !available {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		for partNum := 0; ; partNum++ {
			avail, waitErr := s.waitForPartS3(ctx, transferID, partNum, errCh)
			if waitErr != nil {
				pw.CloseWithError(waitErr)

				return
			}
			if !avail {
				return
			}

			partPath := transfers.TransferPartPath(transferID, partNum)

			reader, readErr := s.storage.ReadStream(ctx, partPath)
			if readErr != nil {
				s.logger.Error("failed to read remote transfer part",
					"transfer_id", transferID, "part", partNum, "error", readErr)
				pw.CloseWithError(errors.Wrapf(readErr, "read part %d", partNum))

				return
			}

			if _, copyErr := io.Copy(pw, reader); copyErr != nil {
				_ = reader.Close()
				s.logger.Error("failed to stream remote part to pipe",
					"transfer_id", transferID, "part", partNum, "error", copyErr)
				pw.CloseWithError(errors.Wrapf(copyErr, "stream part %d", partNum))

				return
			}
			_ = reader.Close()

			if delErr := s.storage.Delete(ctx, partPath); delErr != nil {
				s.logger.Warn("failed to delete consumed remote part",
					"transfer_id", transferID, "part", partNum, "error", delErr)
			}
		}
	}()

	return &remoteCleanupReadCloser{
		ReadCloser: pr,
		transferID: transferID,
		storage:    s.storage,
		logger:     s.logger,
	}, nil
}

func (s *FileService) waitForPartS3(
	ctx context.Context,
	transferID string,
	partNum int,
	errCh <-chan error,
) (bool, error) {
	partPath := transfers.TransferPartPath(transferID, partNum)
	donePath := transfers.TransferDonePath(transferID)

	for {
		select {
		case err := <-errCh:
			return false, err
		default:
		}

		if s.storage.Exists(ctx, partPath) {
			return true, nil
		}

		if s.storage.Exists(ctx, donePath) {
			doneInfo, err := s.readSentinel(ctx, donePath)
			if err != nil {
				return false, errors.WithMessage(err, "reading transfer sentinel")
			}
			if !doneInfo.Success {
				return false, errors.Errorf("transfer failed on remote: %s", doneInfo.Error)
			}

			return false, nil
		}

		select {
		case <-time.After(s3PollInterval):
		case <-ctx.Done():
			return false, ctx.Err()
		case err := <-errCh:
			return false, err
		}
	}
}

func (s *FileService) writeSentinelIfMissing(ctx context.Context, transferID string) {
	donePath := transfers.TransferDonePath(transferID)
	if s.storage.Exists(ctx, donePath) {
		return
	}

	info := transfers.DoneInfo{Success: true, TotalParts: 0}

	data, err := json.Marshal(info)
	if err != nil {
		s.logger.Error("failed to marshal fallback sentinel", "transfer_id", transferID, "error", err)

		return
	}

	if writeErr := s.storage.Write(ctx, donePath, data); writeErr != nil {
		s.logger.Error("failed to write fallback sentinel", "transfer_id", transferID, "error", writeErr)
	}
}

func (s *FileService) readSentinel(ctx context.Context, path string) (*transfers.DoneInfo, error) {
	data, err := s.storage.Read(ctx, path)
	if err != nil {
		return nil, errors.Wrap(err, "read sentinel file")
	}

	var info transfers.DoneInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, errors.Wrap(err, "parse sentinel JSON")
	}

	return &info, nil
}

func (s *FileService) Upload(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	content []byte,
	perms os.FileMode,
) error {
	nodeID := uint64(node.ID)
	relPath := stripWorkPath(node.WorkPath, filePath)
	mode := int32(perms & 0x1FF)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		if s.legacy != nil {
			return s.legacy.Upload(ctx, node, filePath, content, perms)
		}

		return errors.WithMessage(err, "Upload: failed to resolve route")
	}

	if local {
		return s.gateway.RequestFileWrite(ctx, nodeID, relPath, content, mode, true)
	}

	return s.dispatcher.DispatchFileWrite(ctx, nodeID, relPath, content, mode, true)
}

func (s *FileService) UploadStreamPrepared(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	transferID string,
	checksum string,
	totalSize uint64,
) error {
	nodeID := uint64(node.ID)
	relPath := stripWorkPath(node.WorkPath, filePath)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		if s.legacy != nil {
			return s.uploadStreamPreparedLegacy(ctx, node, filePath, transferID, totalSize)
		}

		return errors.WithMessage(err, "UploadStreamPrepared: failed to resolve route")
	}

	if local && s.registry.HasCapability(nodeID, capabilityFileTransfer) {
		if reqErr := s.gateway.RequestFileUploadTask(
			ctx, nodeID, transferID, relPath, checksum, safeUint64ToInt64(totalSize),
		); reqErr != nil {
			return errors.WithMessage(reqErr, "upload task")
		}

		return nil
	}

	if dispatchErr := s.dispatcher.DispatchUploadTask(ctx, nodeID, transferID, relPath); dispatchErr != nil {
		return errors.WithMessage(dispatchErr, "dispatched upload task")
	}

	return nil
}

func (s *FileService) uploadStreamPreparedLegacy(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	transferID string,
	totalSize uint64,
) error {
	s.logger.Debug("uploadStreamPreparedLegacy: start",
		"node_id", node.ID, "transfer_id", transferID,
		"file_path", filePath, "total_size", totalSize)

	if totalSize > legacyUploadMaxBufferSize {
		s.logger.Debug("uploadStreamPreparedLegacy: size cap exceeded",
			"transfer_id", transferID, "total_size", totalSize, "cap", legacyUploadMaxBufferSize)

		return errors.Errorf(
			"legacy upload not supported for size %d (max %d): node requires gRPC daemon",
			totalSize, legacyUploadMaxBufferSize,
		)
	}

	storagePath := transfers.TransferDataPath(transferID)

	if !s.storage.Exists(ctx, storagePath) {
		s.logger.Debug("uploadStreamPreparedLegacy: data missing in storage",
			"transfer_id", transferID, "storage_path", storagePath)

		return errors.Errorf("upload data missing in storage: transfer %s", transferID)
	}

	readStart := time.Now()
	content, err := s.storage.Read(ctx, storagePath)
	if err != nil {
		s.logger.Debug("uploadStreamPreparedLegacy: storage read failed",
			"transfer_id", transferID, "storage_path", storagePath,
			"duration", time.Since(readStart), "error", err)

		return errors.Wrap(err, "read upload data for legacy upload")
	}
	s.logger.Debug("uploadStreamPreparedLegacy: data read from storage",
		"transfer_id", transferID, "bytes", len(content),
		"duration", time.Since(readStart))

	if uint64(len(content)) != totalSize {
		s.logger.Debug("uploadStreamPreparedLegacy: size mismatch",
			"transfer_id", transferID, "expected", totalSize, "got", len(content))

		return errors.Errorf(
			"upload data size mismatch for transfer %s: expected %d, got %d",
			transferID, totalSize, len(content),
		)
	}

	// Detach from caller's deadline (typically DaemonDispatchTimeout=2m, too
	// short for slow networks where 13MB legitimately takes 13+ min). Keep
	// caller cancellation propagating so an aborted HTTP request still stops
	// the upload.
	uploadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), legacyUploadOverallTimeout)
	defer cancel()
	stopWatcher := make(chan struct{})
	defer close(stopWatcher)
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-stopWatcher:
		}
	}()

	uploadStart := time.Now()
	uploadErr := s.legacy.UploadStream(
		uploadCtx, node, filePath, bytes.NewReader(content), totalSize, legacyUploadDefaultPerms,
	)
	if uploadErr != nil {
		s.logger.Debug("uploadStreamPreparedLegacy: legacy upload failed",
			"transfer_id", transferID, "node_id", node.ID,
			"duration", time.Since(uploadStart), "error", uploadErr)

		return errors.WithMessage(uploadErr, "legacy upload stream")
	}

	s.logger.Debug("uploadStreamPreparedLegacy: success",
		"transfer_id", transferID, "node_id", node.ID,
		"size", totalSize, "duration", time.Since(uploadStart))

	return nil
}

func (s *FileService) UploadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	r io.Reader,
	size uint64,
	perms os.FileMode,
) error {
	nodeID := uint64(node.ID)
	relPath := stripWorkPath(node.WorkPath, filePath)
	mode := int32(perms & 0x1FF)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		if s.legacy != nil {
			return s.legacy.UploadStream(ctx, node, filePath, r, size, perms)
		}

		return err
	}

	if local && size > 0 && size <= uint64(smallFileThreshold) {
		content, readErr := io.ReadAll(r)
		if readErr != nil {
			return errors.Wrap(readErr, "reading upload content")
		}

		return s.gateway.RequestFileWrite(ctx, nodeID, relPath, content, mode, true)
	}

	transferID := idgen.New()
	storagePath := transfers.TransferDataPath(transferID)

	hasher := sha256.New()
	teeReader := io.TeeReader(r, hasher)

	if err := s.storage.WriteStream(ctx, storagePath, teeReader); err != nil {
		return errors.Wrap(err, "writing upload to storage")
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	if local && s.registry.HasCapability(nodeID, capabilityFileTransfer) {
		err := s.gateway.RequestFileUploadTask(ctx, nodeID, transferID, relPath, checksum, safeUint64ToInt64(size))
		_ = s.storage.Delete(context.Background(), storagePath)

		if err != nil {
			return errors.WithMessage(err, "upload task")
		}

		return nil
	}

	if err := s.dispatcher.DispatchUploadTask(ctx, nodeID, transferID, relPath); err != nil {
		_ = s.storage.Delete(context.Background(), storagePath)

		return errors.WithMessage(err, "dispatched upload task")
	}

	return nil
}

func (s *FileService) MkDir(ctx context.Context, node *domain.Node, directory string) error {
	nodeID := uint64(node.ID)
	if s.legacy != nil && !s.registry.IsConnected(nodeID) && !s.registry.IsConnectedAnywhere(nodeID) {
		return s.legacy.MkDir(ctx, node, directory)
	}

	return s.doFileOperation(ctx, node, &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_MKDIR,
		Parameters: &proto.FileOperationRequest_MkdirParams{
			MkdirParams: &proto.MkdirParams{
				Path:      stripWorkPath(node.WorkPath, directory),
				Recursive: true,
			},
		},
	})
}

func (s *FileService) Copy(ctx context.Context, node *domain.Node, source, destination string) error {
	nodeID := uint64(node.ID)
	if s.legacy != nil && !s.registry.IsConnected(nodeID) && !s.registry.IsConnectedAnywhere(nodeID) {
		return s.legacy.Copy(ctx, node, source, destination)
	}

	return s.doFileOperation(ctx, node, &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_COPY,
		Parameters: &proto.FileOperationRequest_CopyParams{
			CopyParams: &proto.CopyParams{
				Source:      stripWorkPath(node.WorkPath, source),
				Destination: stripWorkPath(node.WorkPath, destination),
				Recursive:   true,
			},
		},
	})
}

func (s *FileService) Move(ctx context.Context, node *domain.Node, source, destination string) error {
	nodeID := uint64(node.ID)
	if s.legacy != nil && !s.registry.IsConnected(nodeID) && !s.registry.IsConnectedAnywhere(nodeID) {
		return s.legacy.Move(ctx, node, source, destination)
	}

	return s.doFileOperation(ctx, node, &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_MOVE,
		Parameters: &proto.FileOperationRequest_MoveParams{
			MoveParams: &proto.MoveParams{
				Source:      stripWorkPath(node.WorkPath, source),
				Destination: stripWorkPath(node.WorkPath, destination),
			},
		},
	})
}

func (s *FileService) Remove(ctx context.Context, node *domain.Node, path string, recursive bool) error {
	nodeID := uint64(node.ID)
	if s.legacy != nil && !s.registry.IsConnected(nodeID) && !s.registry.IsConnectedAnywhere(nodeID) {
		return s.legacy.Remove(ctx, node, path, recursive)
	}

	return s.doFileOperation(ctx, node, &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_DELETE,
		Parameters: &proto.FileOperationRequest_DeleteParams{
			DeleteParams: &proto.DeleteParams{
				Path:      stripWorkPath(node.WorkPath, path),
				Recursive: recursive,
			},
		},
	})
}

func (s *FileService) GetFileInfo(
	ctx context.Context,
	node *domain.Node,
	path string,
) (*FileDetails, error) {
	nodeID := uint64(node.ID)
	relPath := stripWorkPath(node.WorkPath, path)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		if s.legacy != nil {
			return s.legacy.GetFileInfo(ctx, node, path)
		}

		return nil, err
	}

	req := &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_STAT,
		Parameters: &proto.FileOperationRequest_StatParams{
			StatParams: &proto.StatParams{
				Path: relPath,
			},
		},
	}

	var resp *proto.FileOperationResponse
	if local {
		resp, err = s.gateway.RequestFileOperation(ctx, nodeID, req)
	} else {
		resp, err = s.dispatcher.DispatchFileOperation(ctx, nodeID, req)
	}
	if err != nil {
		return nil, errors.WithMessage(err, "stat request")
	}

	if !resp.Success {
		return nil, errors.Errorf("stat failed: %s", resp.Error)
	}

	statResult := resp.GetStatResult()
	if statResult == nil {
		return nil, errors.New("stat returned no result")
	}

	return protoFileStatToDetails(statResult.Stat), nil
}

func (s *FileService) Chmod(ctx context.Context, node *domain.Node, path string, perm uint32) error {
	nodeID := uint64(node.ID)
	if s.legacy != nil && !s.registry.IsConnected(nodeID) && !s.registry.IsConnectedAnywhere(nodeID) {
		return s.legacy.Chmod(ctx, node, path, perm)
	}

	return s.doFileOperation(ctx, node, &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_CHMOD,
		Parameters: &proto.FileOperationRequest_ChmodParams{
			ChmodParams: &proto.ChmodParams{
				Path: stripWorkPath(node.WorkPath, path),
				Mode: int32(perm & 0x1FF),
			},
		},
	})
}

func (s *FileService) doFileOperation(
	ctx context.Context,
	node *domain.Node,
	req *proto.FileOperationRequest,
) error {
	nodeID := uint64(node.ID)

	local, err := s.resolveRoute(nodeID)
	if err != nil {
		return err
	}

	var resp *proto.FileOperationResponse
	if local {
		resp, err = s.gateway.RequestFileOperation(ctx, nodeID, req)
	} else {
		resp, err = s.dispatcher.DispatchFileOperation(ctx, nodeID, req)
	}
	if err != nil {
		return errors.WithMessage(err, "file operation")
	}

	if !resp.Success {
		return errors.Errorf("file operation failed: %s", resp.Error)
	}

	return nil
}

type chunkedCleanupReadCloser struct {
	io.ReadCloser

	transferID  string
	storage     files.StreamFileManager
	transferReg *transfers.Registry
	logger      *slog.Logger
}

func (c *chunkedCleanupReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.transferReg.Unregister(c.transferID)

	prefix := transfers.TransferPrefix(c.transferID)
	if cleanErr := c.storage.DeleteByPrefix(context.Background(), prefix); cleanErr != nil {
		c.logger.Warn("failed to cleanup transfer",
			"transfer_id", c.transferID, "error", cleanErr)
	}

	return err
}

type remoteCleanupReadCloser struct {
	io.ReadCloser

	transferID string
	storage    files.StreamFileManager
	logger     *slog.Logger
}

func (c *remoteCleanupReadCloser) Close() error {
	err := c.ReadCloser.Close()

	prefix := transfers.TransferPrefix(c.transferID)
	if cleanErr := c.storage.DeleteByPrefix(context.Background(), prefix); cleanErr != nil {
		c.logger.Warn("failed to cleanup remote transfer",
			"transfer_id", c.transferID, "error", cleanErr)
	}

	return err
}

func protoFileStatToFileInfo(f *proto.FileStat) *FileInfo {
	if f == nil {
		return nil
	}

	var modTime uint64
	if f.ModifiedAt != nil {
		ts := f.ModifiedAt.AsTime().Unix()
		if ts > 0 {
			modTime = uint64(ts)
		}
	}

	return &FileInfo{
		Name:          f.Name,
		Path:          f.Path,
		Size:          f.Size,
		TimeModified:  modTime,
		Type:          protoFileTypeToFileType(f.Type),
		Perm:          f.Mode,
		SymlinkTarget: f.SymlinkTarget,
	}
}

func protoFileTypeToFileType(t proto.FileType) FileType {
	switch t {
	case proto.FileType_FILE_TYPE_DIRECTORY:
		return FileTypeDir
	case proto.FileType_FILE_TYPE_REGULAR:
		return FileTypeFile
	case proto.FileType_FILE_TYPE_SYMLINK:
		return FileTypeSymlink
	case proto.FileType_FILE_TYPE_SOCKET:
		return FileTypeSocket
	case proto.FileType_FILE_TYPE_FIFO:
		return FileTypeNamedPipe
	case proto.FileType_FILE_TYPE_BLOCK_DEVICE:
		return FileTypeBlockDevice
	case proto.FileType_FILE_TYPE_CHAR_DEVICE:
		return FileTypeDevice
	default:
		return FileTypeUnknown
	}
}

func protoFileStatToDetails(s *proto.FileStat) *FileDetails {
	if s == nil {
		return &FileDetails{}
	}

	var modTime, accessTime uint64
	if s.ModifiedAt != nil {
		ts := s.ModifiedAt.AsTime().Unix()
		if ts > 0 {
			modTime = uint64(ts)
		}
	}
	if s.AccessedAt != nil {
		ts := s.AccessedAt.AsTime().Unix()
		if ts > 0 {
			accessTime = uint64(ts)
		}
	}

	return &FileDetails{
		Name:             s.Name,
		Size:             s.Size,
		ModificationTime: modTime,
		AccessTime:       accessTime,
		Perm:             s.Mode,
		Type:             protoFileTypeToFileType(s.Type),
		SymlinkTarget:    s.SymlinkTarget,
	}
}

func safeUint64ToInt64(v uint64) int64 {
	if v > uint64(math.MaxInt64) {
		return math.MaxInt64
	}

	return int64(v)
}

// stripWorkPath returns fullPath relative to workPath, defensively handling Windows
// separators and drive letters. Both inputs may use any mix of "/" and "\" as separators;
// the result is forward-slash relative. If a Windows volume name survives the prefix trim
// (e.g. workPath was empty or stored with a mismatched shape), it is stripped so the
// daemon cannot receive an absolute path and produce a doubled-join (C:\gameap\C:\gameap\...).
func stripWorkPath(workPath, fullPath string) string {
	normWork := strings.ReplaceAll(workPath, "\\", "/")
	normFull := strings.ReplaceAll(fullPath, "\\", "/")

	rel := strings.TrimPrefix(normFull, normWork)

	if len(rel) >= 2 && rel[1] == ':' && validation.IsASCIILetter(rel[0]) {
		rel = rel[2:]
	}

	rel = strings.TrimLeft(rel, "/")

	if rel == "" {
		return "."
	}

	return rel
}
