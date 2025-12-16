package hostlibrary

import (
	"context"
	"os"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

type NodeFSServiceImpl struct {
	fileService *daemon.FileService
	nodeRepo    repositories.NodeRepository
}

func NewNodeFSService(
	fileService *daemon.FileService,
	nodeRepo repositories.NodeRepository,
) *NodeFSServiceImpl {
	return &NodeFSServiceImpl{
		fileService: fileService,
		nodeRepo:    nodeRepo,
	}
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
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.ReadDirResponse{Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.ReadDirResponse{Error: lo.ToPtr("node not found")}, nil
	}

	files, err := s.fileService.ReadDir(ctx, node, req.Path)
	if err != nil {
		return &nodefs.ReadDirResponse{Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.ReadDirResponse{
		Files: convertFileInfosToProto(files),
	}, nil
}

func (s *NodeFSServiceImpl) MkDir(
	ctx context.Context,
	req *nodefs.MkDirRequest,
) (*nodefs.MkDirResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.MkDirResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.MkDirResponse{Success: false, Error: lo.ToPtr("node not found")}, nil
	}

	err = s.fileService.MkDir(ctx, node, req.Path)
	if err != nil {
		return &nodefs.MkDirResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.MkDirResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Copy(
	ctx context.Context,
	req *nodefs.CopyRequest,
) (*nodefs.CopyResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.CopyResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.CopyResponse{Success: false, Error: lo.ToPtr("node not found")}, nil
	}

	err = s.fileService.Copy(ctx, node, req.Source, req.Destination)
	if err != nil {
		return &nodefs.CopyResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.CopyResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Move(
	ctx context.Context,
	req *nodefs.MoveRequest,
) (*nodefs.MoveResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.MoveResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.MoveResponse{Success: false, Error: lo.ToPtr("node not found")}, nil
	}

	err = s.fileService.Move(ctx, node, req.Source, req.Destination)
	if err != nil {
		return &nodefs.MoveResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.MoveResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Download(
	ctx context.Context,
	req *nodefs.DownloadRequest,
) (*nodefs.DownloadResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.DownloadResponse{Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.DownloadResponse{Error: lo.ToPtr("node not found")}, nil
	}

	content, err := s.fileService.Download(ctx, node, req.Path)
	if err != nil {
		return &nodefs.DownloadResponse{Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.DownloadResponse{Content: content}, nil
}

func (s *NodeFSServiceImpl) Upload(
	ctx context.Context,
	req *nodefs.UploadRequest,
) (*nodefs.UploadResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.UploadResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.UploadResponse{Success: false, Error: lo.ToPtr("node not found")}, nil
	}

	err = s.fileService.Upload(ctx, node, req.Path, req.Content, os.FileMode(req.Permissions))
	if err != nil {
		return &nodefs.UploadResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.UploadResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) Remove(
	ctx context.Context,
	req *nodefs.RemoveRequest,
) (*nodefs.RemoveResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.RemoveResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.RemoveResponse{Success: false, Error: lo.ToPtr("node not found")}, nil
	}

	err = s.fileService.Remove(ctx, node, req.Path, req.Recursive)
	if err != nil {
		return &nodefs.RemoveResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.RemoveResponse{Success: true}, nil
}

func (s *NodeFSServiceImpl) GetFileInfo(
	ctx context.Context,
	req *nodefs.GetFileInfoRequest,
) (*nodefs.GetFileInfoResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.GetFileInfoResponse{Found: false, Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.GetFileInfoResponse{Found: false, Error: lo.ToPtr("node not found")}, nil
	}

	details, err := s.fileService.GetFileInfo(ctx, node, req.Path)
	if err != nil {
		return &nodefs.GetFileInfoResponse{Found: false, Error: lo.ToPtr(err.Error())}, nil
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
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.ChmodResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.ChmodResponse{Success: false, Error: lo.ToPtr("node not found")}, nil
	}

	err = s.fileService.Chmod(ctx, node, req.Path, req.Permissions)
	if err != nil {
		return &nodefs.ChmodResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodefs.ChmodResponse{Success: true}, nil
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

type NodeFSHostLibrary struct {
	impl *NodeFSServiceImpl
}

func NewNodeFSHostLibrary(
	fileService *daemon.FileService,
	nodeRepo repositories.NodeRepository,
) *NodeFSHostLibrary {
	return &NodeFSHostLibrary{
		impl: NewNodeFSService(fileService, nodeRepo),
	}
}

func (l *NodeFSHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return nodefs.Instantiate(ctx, r, l.impl)
}
