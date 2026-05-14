package daemon

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
)

type OwnerOptions struct {
	User string
	UID  int32
	GID  int32
}

func (o OwnerOptions) IsZero() bool {
	return o.User == "" && o.UID == 0 && o.GID == 0
}

func OwnerFromServer(server *domain.Server) OwnerOptions {
	if server == nil || server.SuUser == nil {
		return OwnerOptions{}
	}

	return OwnerOptions{User: *server.SuUser}
}

type FileGateway interface {
	RequestFileList(
		ctx context.Context, nodeID uint64, path string, recursive bool, pattern string,
	) (*proto.FileListResponse, error)
	RequestFileRead(
		ctx context.Context, nodeID uint64, path string, offset int64, length int64,
	) (*proto.FileReadResponse, error)
	RequestFileWrite(
		ctx context.Context, nodeID uint64, path string,
		content []byte, mode int32, createDirs bool, owner OwnerOptions,
	) error
	RequestFileOperation(
		ctx context.Context, nodeID uint64, req *proto.FileOperationRequest,
	) (*proto.FileOperationResponse, error)
	RequestFileUploadTask(
		ctx context.Context, nodeID uint64, transferID, destPath, checksum string,
		totalSize int64, mode int32, owner OwnerOptions,
	) error
	RequestFileDownloadTask(
		ctx context.Context, nodeID uint64, transferID, srcPath string,
	) error
}

type ConnectionChecker interface {
	IsConnected(nodeID uint64) bool
	IsConnectedAnywhere(nodeID uint64) bool
	HasCapability(nodeID uint64, capability string) bool
}

type FileDispatcher interface {
	Start(ctx context.Context) error

	DispatchFileList(
		ctx context.Context, nodeID uint64, path string, recursive bool, pattern string,
	) (*proto.FileListResponse, error)
	DispatchFileOperation(
		ctx context.Context, nodeID uint64, req *proto.FileOperationRequest,
	) (*proto.FileOperationResponse, error)
	DispatchFileRead(
		ctx context.Context, nodeID uint64, path string, offset int64, length int64,
	) (*FileReadResult, error)
	DispatchFileWrite(
		ctx context.Context, nodeID uint64, path string,
		content []byte, mode int32, createDirs bool, owner OwnerOptions,
	) error
	DispatchUploadTask(
		ctx context.Context, nodeID uint64, transferID string, destPath string,
		mode int32, owner OwnerOptions,
	) error
	DispatchDownloadTask(ctx context.Context, nodeID uint64, transferID string, srcPath string) error
}

type FileReadResult struct {
	Content     []byte
	StoragePath string
}

type CommandGateway interface {
	RequestCommand(
		ctx context.Context, nodeID uint64, req *proto.CommandRequest,
	) (*proto.CommandResult, error)
}

type StatusGateway interface {
	RequestStatus(ctx context.Context, nodeID uint64) (*proto.StatusResponse, error)
}

type ConsoleLogGateway interface {
	RequestConsoleLog(
		ctx context.Context, nodeID uint64, serverID uint64, maxBytes int64,
	) (*proto.ConsoleLogResponse, error)
}

type CommandDispatcher interface {
	Start(ctx context.Context) error
	DispatchCommand(
		ctx context.Context, nodeID uint64, req *proto.CommandRequest,
	) (*proto.CommandResult, error)
}

type StatusDispatcher interface {
	Start(ctx context.Context) error
	DispatchStatus(ctx context.Context, nodeID uint64) (*proto.StatusResponse, error)
}

type ConsoleLogDispatcher interface {
	Start(ctx context.Context) error
	DispatchConsoleLog(
		ctx context.Context, nodeID uint64, serverID uint64, maxBytes int64,
	) (*proto.ConsoleLogResponse, error)
}

type HTTPProxyGateway interface {
	RequestHTTPProxy(
		ctx context.Context, nodeID uint64, req *proto.HTTPProxyRequest,
	) (*proto.HTTPProxyResponse, error)
}

type HTTPProxyDispatcher interface {
	Start(ctx context.Context) error
	DispatchHTTPProxy(
		ctx context.Context, nodeID uint64, req *proto.HTTPProxyRequest,
	) (*proto.HTTPProxyResponse, error)
}
