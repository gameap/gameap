package hostlibrary

import (
	"context"
	"os"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
)

// TaskScheduler is the scheduler surface the gameap-scheduler host library
// delegates to. Implementations must tolerate being called while the plugin
// manager lock is held: host functions run during guest Initialize/Shutdown.
type TaskScheduler interface {
	// AddTask registers or replaces (by PluginID+Name) a task definition.
	AddTask(ctx context.Context, task domain.PluginScheduledTask) error
	RemoveTask(ctx context.Context, pluginID domain.Uint64ID, name string) error
	ListTasks(ctx context.Context, pluginID domain.Uint64ID) ([]domain.PluginScheduledTask, error)
}

// NodeFileService is the remote-node filesystem surface the gameap-nodefs host
// library delegates to. Satisfied by *daemon.FileService.
type NodeFileService interface {
	ReadDir(ctx context.Context, node *domain.Node, directory string) ([]*daemon.FileInfo, error)
	MkDir(ctx context.Context, node *domain.Node, directory string, owner daemon.OwnerOptions) error
	Copy(ctx context.Context, node *domain.Node, source, destination string) error
	Move(ctx context.Context, node *domain.Node, source, destination string) error
	Download(ctx context.Context, node *domain.Node, filePath string) ([]byte, error)
	Upload(
		ctx context.Context,
		node *domain.Node,
		filePath string,
		content []byte,
		perms os.FileMode,
		owner daemon.OwnerOptions,
	) error
	Remove(ctx context.Context, node *domain.Node, path string, recursive bool) error
	GetFileInfo(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
	Chmod(ctx context.Context, node *domain.Node, path string, perm uint32) error
}

// NodeCommandService is the remote-node command execution surface the
// gameap-nodecmd host library delegates to. Satisfied by *daemon.CommandService.
type NodeCommandService interface {
	ExecuteCommand(
		ctx context.Context,
		node *domain.Node,
		command string,
		opts ...daemon.CommandServiceOption,
	) (*daemon.CommandResult, error)
}
