package hostlibrary

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

// filesPermissionDeniedMessage is what a plugin without the grant sees. It
// names the missing permission so plugin authors know what to declare.
const filesPermissionDeniedMessage = "plugin permission " + string(domain.PluginPermissionFiles) + " required"

// syncWaitGuestDeadlineGrace keeps the blocking archive wait strictly inside
// the guest call deadline: hitting the call deadline itself would close the
// wasm module (WithCloseOnContextDone), while an expired wait merely answers
// completed=false.
const syncWaitGuestDeadlineGrace = 2 * time.Second

// defaultSyncWaitBudget applies when the request names no timeout and the
// context carries no deadline (the wrapper always sets one in production);
// without it such a call would answer completed=false immediately.
const defaultSyncWaitBudget = 30 * time.Second

type NodeFSServiceImpl struct {
	pluginID    uint64
	fileService NodeFileService
	nodeRepo    repositories.NodeRepository
	archive     NodeArchiveService
	events      ArchiveEventRegistrar
	checker     PluginPermissionChecker
}

func NewNodeFSService(
	pluginID uint64,
	fileService NodeFileService,
	nodeRepo repositories.NodeRepository,
	archive NodeArchiveService,
	events ArchiveEventRegistrar,
	checker PluginPermissionChecker,
) *NodeFSServiceImpl {
	return &NodeFSServiceImpl{
		pluginID:    pluginID,
		fileService: fileService,
		nodeRepo:    nodeRepo,
		archive:     archive,
		events:      events,
		checker:     checker,
	}
}

// authorize gates every method of this module on the "files" grant. Plugin
// ID 0 (transient dry-run loads) is never granted anything.
func (s *NodeFSServiceImpl) authorize(ctx context.Context) (bool, string) {
	allowed, err := s.checker.Has(ctx, s.pluginID, domain.PluginPermissionFiles)
	if err != nil {
		slog.ErrorContext(ctx, "failed to check plugin files permission",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("error", err.Error()))

		return false, "failed to check plugin permission: " + err.Error()
	}

	if !allowed {
		slog.WarnContext(ctx, "plugin denied access to the nodefs host library",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("permission", string(domain.PluginPermissionFiles)))

		return false, filesPermissionDeniedMessage
	}

	return true, ""
}

func (s *NodeFSServiceImpl) initiator() string {
	return "plugin:" + strconv.FormatUint(s.pluginID, 10)
}

func (s *NodeFSServiceImpl) getNode(ctx context.Context, nodeID uint64) (*domain.Node, error) {
	nodes, err := s.nodeRepo.Find(ctx, filters.FindNodeByIDs(uint(nodeID)), nil, nil)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, nil
	}

	return &nodes[0], nil
}

func (s *NodeFSServiceImpl) ReadDir(
	ctx context.Context,
	req *nodefs.ReadDirRequest,
) (*nodefs.ReadDirResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.ReadDirResponse{Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.ReadDirResponse{Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.ReadDirResponse{Error: new("node not found")}, nil
	}

	files, err := s.fileService.ReadDir(ctx, node, req.Path)
	if err != nil {
		return &nodefs.ReadDirResponse{Error: new(err.Error())}, nil
	}

	return &nodefs.ReadDirResponse{
		Files: convertFileInfosToProto(files),
	}, nil
}

func (s *NodeFSServiceImpl) MkDir(
	ctx context.Context,
	req *nodefs.MkDirRequest,
) (*nodefs.MkDirResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.MkDirResponse{Success: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.MkDirResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.MkDirResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.fileService.MkDir(ctx, node, req.Path, daemon.OwnerOptions{})
	if err != nil {
		return &nodefs.MkDirResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.MkDirResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Copy(
	ctx context.Context,
	req *nodefs.CopyRequest,
) (*nodefs.CopyResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.CopyResponse{Success: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.CopyResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.CopyResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.fileService.Copy(ctx, node, req.Source, req.Destination)
	if err != nil {
		return &nodefs.CopyResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.CopyResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Move(
	ctx context.Context,
	req *nodefs.MoveRequest,
) (*nodefs.MoveResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.MoveResponse{Success: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.MoveResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.MoveResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.fileService.Move(ctx, node, req.Source, req.Destination)
	if err != nil {
		return &nodefs.MoveResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.MoveResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Download(
	ctx context.Context,
	req *nodefs.DownloadRequest,
) (*nodefs.DownloadResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.DownloadResponse{Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.DownloadResponse{Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.DownloadResponse{Error: new("node not found")}, nil
	}

	content, err := s.fileService.Download(ctx, node, req.Path)
	if err != nil {
		return &nodefs.DownloadResponse{Error: new(err.Error())}, nil
	}

	return &nodefs.DownloadResponse{Content: content}, nil
}

func (s *NodeFSServiceImpl) Upload(
	ctx context.Context,
	req *nodefs.UploadRequest,
) (*nodefs.UploadResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.UploadResponse{Success: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.UploadResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.UploadResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.fileService.Upload(ctx, node, req.Path, req.Content, os.FileMode(req.Permissions), daemon.OwnerOptions{})
	if err != nil {
		return &nodefs.UploadResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.UploadResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Remove(
	ctx context.Context,
	req *nodefs.RemoveRequest,
) (*nodefs.RemoveResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.RemoveResponse{Success: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.RemoveResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.RemoveResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.fileService.Remove(ctx, node, req.Path, req.Recursive)
	if err != nil {
		return &nodefs.RemoveResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.RemoveResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) GetFileInfo(
	ctx context.Context,
	req *nodefs.GetFileInfoRequest,
) (*nodefs.GetFileInfoResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.GetFileInfoResponse{Found: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.GetFileInfoResponse{Found: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.GetFileInfoResponse{Found: false, Error: new("node not found")}, nil
	}

	details, err := s.fileService.GetFileInfo(ctx, node, req.Path)
	if err != nil {
		return &nodefs.GetFileInfoResponse{Found: false, Error: new(err.Error())}, nil
	}

	return &nodefs.GetFileInfoResponse{
		File:  convertFileDetailsToProto(details),
		Found: true,
	}, nil
}

func (s *NodeFSServiceImpl) Chmod(
	ctx context.Context,
	req *nodefs.ChmodRequest,
) (*nodefs.ChmodResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.ChmodResponse{Success: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.ChmodResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.ChmodResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.fileService.Chmod(ctx, node, req.Path, req.Permissions)
	if err != nil {
		return &nodefs.ChmodResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.ChmodResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Hash(
	ctx context.Context,
	req *nodefs.HashRequest,
) (*nodefs.HashResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.HashResponse{Success: false, Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.HashResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.HashResponse{Success: false, Error: new("node not found")}, nil
	}

	// Membership check instead of comparing against UNSPECIFIED: any value
	// outside the known set normalizes to SHA-256, and the response echoes
	// the algorithm that actually produced the digests.
	algorithm := req.Algorithm
	if _, known := hashAlgorithmsToProto[algorithm]; !known {
		algorithm = nodefs.HashAlgorithm_HASH_ALGORITHM_SHA256
	}

	result, err := s.fileService.Hash(ctx, node, req.Paths, hashAlgorithmToProto(algorithm))
	if err != nil {
		return &nodefs.HashResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.HashResponse{
		Success:   true,
		Algorithm: algorithm,
		Results: lo.Map(result.GetHashes(), func(h *proto.FileHash, _ int) *nodefs.FileHash {
			fileHash := &nodefs.FileHash{
				Path: h.GetPath(),
				Hash: h.GetHash(),
				Size: h.GetSize(),
			}
			if h.GetError() != "" {
				fileHash.Error = new(h.GetError())
			}

			return fileHash
		}),
	}, nil
}

func (s *NodeFSServiceImpl) CreateArchive(
	ctx context.Context,
	req *nodefs.CreateArchiveRequest,
) (*nodefs.ArchiveSyncResponse, error) {
	// Blocking calls never deliver progress callbacks: the guest is parked
	// inside this host function and cannot be re-entered.
	operationID, errMsg := s.startCreate(ctx, req, false)
	if errMsg != "" {
		return &nodefs.ArchiveSyncResponse{Success: false, Error: new(errMsg)}, nil
	}

	return s.waitSync(ctx, operationID, req.TimeoutSeconds), nil
}

func (s *NodeFSServiceImpl) ExtractArchive(
	ctx context.Context,
	req *nodefs.ExtractArchiveRequest,
) (*nodefs.ArchiveSyncResponse, error) {
	operationID, errMsg := s.startExtract(ctx, req, false)
	if errMsg != "" {
		return &nodefs.ArchiveSyncResponse{Success: false, Error: new(errMsg)}, nil
	}

	return s.waitSync(ctx, operationID, req.TimeoutSeconds), nil
}

func (s *NodeFSServiceImpl) StartCreateArchive(
	ctx context.Context,
	req *nodefs.CreateArchiveRequest,
) (*nodefs.StartArchiveResponse, error) {
	operationID, errMsg := s.startCreate(ctx, req, req.ReportProgress)
	if errMsg != "" {
		return &nodefs.StartArchiveResponse{Success: false, Error: new(errMsg)}, nil
	}

	return &nodefs.StartArchiveResponse{Success: true, OperationId: operationID}, nil
}

func (s *NodeFSServiceImpl) StartExtractArchive(
	ctx context.Context,
	req *nodefs.ExtractArchiveRequest,
) (*nodefs.StartArchiveResponse, error) {
	operationID, errMsg := s.startExtract(ctx, req, req.ReportProgress)
	if errMsg != "" {
		return &nodefs.StartArchiveResponse{Success: false, Error: new(errMsg)}, nil
	}

	return &nodefs.StartArchiveResponse{Success: true, OperationId: operationID}, nil
}

func (s *NodeFSServiceImpl) CancelArchive(
	ctx context.Context,
	req *nodefs.CancelArchiveRequest,
) (*nodefs.CancelArchiveResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.CancelArchiveResponse{Success: false, Error: new(msg)}, nil
	}

	snapshot, found := s.archive.GetSnapshot(req.OperationId)
	if !found || snapshot.Initiator != s.initiator() {
		// Never confirm the existence of another initiator's operation.
		return &nodefs.CancelArchiveResponse{Success: false, Error: new("operation not found")}, nil
	}

	node, err := s.getNode(ctx, snapshot.NodeID)
	if err != nil {
		return &nodefs.CancelArchiveResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.CancelArchiveResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.archive.Cancel(ctx, node, req.OperationId, req.Reason)
	if err != nil {
		return &nodefs.CancelArchiveResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.CancelArchiveResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) GetArchiveOperation(
	ctx context.Context,
	req *nodefs.GetArchiveOperationRequest,
) (*nodefs.GetArchiveOperationResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodefs.GetArchiveOperationResponse{Success: false, Error: new(msg)}, nil
	}

	snapshot, found := s.archive.GetSnapshot(req.OperationId)
	if !found || snapshot.Initiator != s.initiator() {
		return &nodefs.GetArchiveOperationResponse{Success: true, Found: false}, nil
	}

	response := &nodefs.GetArchiveOperationResponse{
		Success:        true,
		Found:          true,
		Status:         archiveStatusToSDK(snapshot.Status),
		FilesProcessed: snapshot.Progress.FilesProcessed,
		FilesTotal:     snapshot.Progress.FilesTotal,
		BytesProcessed: snapshot.Progress.BytesProcessed,
		BytesTotal:     snapshot.Progress.BytesTotal,
		CurrentEntry:   snapshot.Progress.CurrentEntry,
	}

	if snapshot.Result != nil {
		response.OpSuccess = snapshot.Result.Success
		if snapshot.Result.Error != "" {
			response.OpError = new(snapshot.Result.Error)
		}
		response.FilesProcessed = snapshot.Result.FilesProcessed
		response.BytesProcessed = snapshot.Result.BytesProcessed
		response.ArchiveSize = snapshot.Result.ArchiveSize
		response.SkippedCount = snapshot.Result.SkippedCount
		response.Format = archiveFormatFromAPIName(snapshot.Result.Format)
	}

	return response, nil
}

func (s *NodeFSServiceImpl) startCreate(
	ctx context.Context,
	req *nodefs.CreateArchiveRequest,
	reportProgress bool,
) (string, string) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return "", msg
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return "", err.Error()
	}

	if node == nil {
		return "", "node not found"
	}

	operationID, err := s.archive.StartCreate(ctx, node, daemon.CreateArchiveParams{
		ArchivePath:      req.ArchivePath,
		BasePath:         req.BasePath,
		Sources:          req.Sources,
		Format:           archiveFormatToProto(req.Format),
		CompressionLevel: req.CompressionLevel,
		Overwrite:        req.Overwrite,
		MaxTotalBytes:    req.MaxTotalBytes,
		MaxFiles:         req.MaxFiles,
		Options: daemon.ArchiveStartOptions{
			Initiator: s.initiator(),
			Timeout:   time.Duration(req.TimeoutSeconds) * time.Second,
		},
	})
	if err != nil {
		return "", err.Error()
	}

	s.registerEvents(operationID, req.NodeId, reportProgress)

	return operationID, ""
}

func (s *NodeFSServiceImpl) startExtract(
	ctx context.Context,
	req *nodefs.ExtractArchiveRequest,
	reportProgress bool,
) (string, string) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return "", msg
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return "", err.Error()
	}

	if node == nil {
		return "", "node not found"
	}

	operationID, err := s.archive.StartExtract(ctx, node, daemon.ExtractArchiveParams{
		ArchivePath:         req.ArchivePath,
		Destination:         req.Destination,
		Format:              archiveFormatToProto(req.Format),
		ConflictPolicy:      conflictPolicyToProto(req.ConflictPolicy),
		CreateDestination:   req.CreateDestination,
		PreservePermissions: req.PreservePermissions,
		MaxTotalBytes:       req.MaxTotalBytes,
		MaxFiles:            req.MaxFiles,
		Options: daemon.ArchiveStartOptions{
			Initiator: s.initiator(),
			Timeout:   time.Duration(req.TimeoutSeconds) * time.Second,
		},
	})
	if err != nil {
		return "", err.Error()
	}

	s.registerEvents(operationID, req.NodeId, reportProgress)

	return operationID, ""
}

// registerEvents records the callback interest and replays a completion that
// beat the registration: the operation runs concurrently with this host call
// and a small archive can publish its final event first.
func (s *NodeFSServiceImpl) registerEvents(operationID string, nodeID uint64, reportProgress bool) {
	s.events.Register(s.pluginID, operationID, nodeID, reportProgress)

	if snapshot, ok := s.archive.GetSnapshot(operationID); ok && snapshot.Result != nil {
		s.events.NotifyCompleted(operationID, *snapshot.Result)
	}
}

// waitSync blocks until the operation completes or the wait budget runs out.
// The budget is the requested timeout capped by the guest call deadline
// minus a grace, so the wait always answers before the runtime would kill
// the module.
func (s *NodeFSServiceImpl) waitSync(
	ctx context.Context, operationID string, timeoutSeconds uint32,
) *nodefs.ArchiveSyncResponse {
	response := &nodefs.ArchiveSyncResponse{
		Success:     true,
		OperationId: operationID,
	}

	budget := time.Duration(0)
	if timeoutSeconds > 0 {
		budget = time.Duration(timeoutSeconds) * time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline) - syncWaitGuestDeadlineGrace
		if budget <= 0 || remaining < budget {
			budget = remaining
		}
	} else if budget <= 0 {
		budget = defaultSyncWaitBudget
	}

	// A non-positive budget here means the guest call deadline is imminent:
	// answering now (completed=false) beats being killed mid-wait.
	if budget <= 0 {
		return response
	}

	waitCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	result, err := s.archive.WaitCompletion(waitCtx, operationID)
	if err != nil || result == nil {
		// Wait budget exhausted (or the registry entry vanished): the
		// operation keeps running, the plugin polls with the operation id.
		return response
	}

	response.Completed = true
	response.OpSuccess = result.Success
	if result.Error != "" {
		response.OpError = new(result.Error)
	}
	response.FilesProcessed = result.FilesProcessed
	response.BytesProcessed = result.BytesProcessed
	response.ArchiveSize = result.ArchiveSize
	response.SkippedCount = result.SkippedCount
	response.Format = archiveFormatFromAPIName(result.Format)

	return response
}

func convertFileInfosToProto(files []*daemon.FileInfo) []*nodefs.FileInfo {
	return lo.Map(files, func(f *daemon.FileInfo, _ int) *nodefs.FileInfo {
		return &nodefs.FileInfo{
			Name:         f.Name,
			Size:         f.Size,
			ModifiedTime: f.TimeModified,
			Type:         convertFileTypeToProto(f.Type),
			Permissions:  f.Perm,
		}
	})
}

func convertFileTypeToProto(t daemon.FileType) nodefs.FileType {
	switch t {
	case daemon.FileTypeDir:
		return nodefs.FileType_FILE_TYPE_DIR
	case daemon.FileTypeFile:
		return nodefs.FileType_FILE_TYPE_FILE
	case daemon.FileTypeDevice:
		return nodefs.FileType_FILE_TYPE_DEVICE
	case daemon.FileTypeBlockDevice:
		return nodefs.FileType_FILE_TYPE_BLOCK_DEVICE
	case daemon.FileTypeNamedPipe:
		return nodefs.FileType_FILE_TYPE_NAMED_PIPE
	case daemon.FileTypeSymlink:
		return nodefs.FileType_FILE_TYPE_SYMLINK
	case daemon.FileTypeSocket:
		return nodefs.FileType_FILE_TYPE_SOCKET
	default:
		return nodefs.FileType_FILE_TYPE_UNKNOWN
	}
}

func convertFileDetailsToProto(d *daemon.FileDetails) *nodefs.FileDetails {
	return &nodefs.FileDetails{
		Name:             d.Name,
		Mime:             d.Mime,
		Size:             d.Size,
		ModificationTime: d.ModificationTime,
		AccessTime:       d.AccessTime,
		CreateTime:       d.CreateTime,
		Permissions:      d.Perm,
		Type:             convertFileTypeToProto(d.Type),
	}
}

var hashAlgorithmsToProto = map[nodefs.HashAlgorithm]proto.HashAlgorithm{
	nodefs.HashAlgorithm_HASH_ALGORITHM_MD5:    proto.HashAlgorithm_HASH_ALGORITHM_MD5,
	nodefs.HashAlgorithm_HASH_ALGORITHM_SHA1:   proto.HashAlgorithm_HASH_ALGORITHM_SHA1,
	nodefs.HashAlgorithm_HASH_ALGORITHM_SHA256: proto.HashAlgorithm_HASH_ALGORITHM_SHA256,
	nodefs.HashAlgorithm_HASH_ALGORITHM_SHA512: proto.HashAlgorithm_HASH_ALGORITHM_SHA512,
	nodefs.HashAlgorithm_HASH_ALGORITHM_CRC32:  proto.HashAlgorithm_HASH_ALGORITHM_CRC32,
	nodefs.HashAlgorithm_HASH_ALGORITHM_CRC64:  proto.HashAlgorithm_HASH_ALGORITHM_CRC64,
}

func hashAlgorithmToProto(a nodefs.HashAlgorithm) proto.HashAlgorithm {
	return hashAlgorithmsToProto[a]
}

var archiveFormatsToProto = map[nodefs.ArchiveFormat]proto.ArchiveFormat{
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_UNSPECIFIED: proto.ArchiveFormat_ARCHIVE_FORMAT_UNSPECIFIED,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_ZIP:         proto.ArchiveFormat_ARCHIVE_FORMAT_ZIP,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_TAR:         proto.ArchiveFormat_ARCHIVE_FORMAT_TAR,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ:      proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_TAR_BZ2:     proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_BZ2,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_TAR_XZ:      proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_XZ,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_TAR_ZSTD:    proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_ZSTD,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_GZ:          proto.ArchiveFormat_ARCHIVE_FORMAT_GZ,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_BZ2:         proto.ArchiveFormat_ARCHIVE_FORMAT_BZ2,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_XZ:          proto.ArchiveFormat_ARCHIVE_FORMAT_XZ,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_ZSTD:        proto.ArchiveFormat_ARCHIVE_FORMAT_ZSTD,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_7Z:          proto.ArchiveFormat_ARCHIVE_FORMAT_7Z,
	nodefs.ArchiveFormat_ARCHIVE_FORMAT_RAR:         proto.ArchiveFormat_ARCHIVE_FORMAT_RAR,
}

func archiveFormatToProto(f nodefs.ArchiveFormat) proto.ArchiveFormat {
	return archiveFormatsToProto[f]
}

func archiveFormatFromAPIName(name string) nodefs.ArchiveFormat {
	return nodefs.ArchiveFormatFromAPIName(name)
}

func conflictPolicyToProto(p nodefs.ArchiveConflictPolicy) proto.ArchiveConflictPolicy {
	switch p {
	case nodefs.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_SKIP:
		return proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_SKIP
	case nodefs.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_OVERWRITE:
		return proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_OVERWRITE
	case nodefs.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_UNSPECIFIED,
		nodefs.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_ERROR:
		return proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_ERROR
	default:
		return proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_ERROR
	}
}

func archiveStatusToSDK(status daemon.ArchiveOpStatus) nodefs.ArchiveOperationStatus {
	switch status {
	case daemon.ArchiveOpRunning:
		return nodefs.ArchiveOperationStatus_ARCHIVE_OPERATION_STATUS_RUNNING
	case daemon.ArchiveOpDone:
		return nodefs.ArchiveOperationStatus_ARCHIVE_OPERATION_STATUS_COMPLETED
	case daemon.ArchiveOpError:
		return nodefs.ArchiveOperationStatus_ARCHIVE_OPERATION_STATUS_ERROR
	default:
		return nodefs.ArchiveOperationStatus_ARCHIVE_OPERATION_STATUS_UNSPECIFIED
	}
}

type NodeFSHostLibrary struct {
	impl *NodeFSServiceImpl
}

func (l *NodeFSHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return nodefs.Instantiate(ctx, r, l.impl)
}

// NodeFSHostLibraryFactory builds a per-plugin nodefs module: the plugin ID
// scopes the "files" permission check, archive-event registration and
// archive-operation ownership.
type NodeFSHostLibraryFactory struct {
	fileService NodeFileService
	nodeRepo    repositories.NodeRepository
	archive     NodeArchiveService
	events      ArchiveEventRegistrar
	checker     PluginPermissionChecker
}

func NewNodeFSHostLibraryFactory(
	fileService NodeFileService,
	nodeRepo repositories.NodeRepository,
	archive NodeArchiveService,
	events ArchiveEventRegistrar,
	checker PluginPermissionChecker,
) *NodeFSHostLibraryFactory {
	return &NodeFSHostLibraryFactory{
		fileService: fileService,
		nodeRepo:    nodeRepo,
		archive:     archive,
		events:      events,
		checker:     checker,
	}
}

func (f *NodeFSHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &NodeFSHostLibrary{
		impl: NewNodeFSService(pluginID, f.fileService, f.nodeRepo, f.archive, f.events, f.checker),
	}
}
