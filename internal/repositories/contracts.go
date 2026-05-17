package repositories

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/pubsub/dlq"
)

type GameRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Game, error)

	Find(
		ctx context.Context,
		filter *filters.FindGame,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Game, error)

	Save(ctx context.Context, game *domain.Game) error

	Delete(ctx context.Context, code string) error
}

type GameModRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.GameMod, error)

	Find(
		ctx context.Context,
		filter *filters.FindGameMod,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.GameMod, error)

	Save(ctx context.Context, gameMod *domain.GameMod) error

	Delete(ctx context.Context, id uint) error
}

type UserRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.User, error)

	Find(
		ctx context.Context,
		filter *filters.FindUser,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.User, error)

	Save(ctx context.Context, user *domain.User) error

	Delete(ctx context.Context, id uint) error
}

type ServerStatusUpdate struct {
	ID               uint
	ProcessActive    bool
	LastProcessCheck time.Time
}

type ServerRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Server, error)

	Find(
		ctx context.Context, filter *filters.FindServer, order []filters.Sorting, pagination *filters.Pagination,
	) ([]domain.Server, error)

	Count(ctx context.Context, filter *filters.FindServer) (int, error)

	FindUserServers(
		ctx context.Context,
		userID uint,
		filter *filters.FindServer,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Server, error)

	Save(ctx context.Context, server *domain.Server) error

	SaveBulk(ctx context.Context, servers []*domain.Server) error

	UpdateServerStatuses(ctx context.Context, nodeID uint, statuses []ServerStatusUpdate) error

	Delete(ctx context.Context, id uint) error

	SoftDelete(ctx context.Context, id uint) error

	SetUserServers(ctx context.Context, userID uint, serverIDs []uint) error

	Exists(ctx context.Context, filter *filters.FindServer) (bool, error)

	Search(ctx context.Context, query string) ([]*domain.Server, error)
}

type RBACRepository interface {
	GetRoles(context.Context) ([]domain.Role, error)
	SaveRole(context.Context, *domain.Role) error
	GetPermissions(context.Context, uint, domain.EntityType) ([]domain.Permission, error)
	GetRolesForEntity(context.Context, uint, domain.EntityType) ([]domain.RestrictedRole, error)
	AssignRolesForEntity(context.Context, uint, domain.EntityType, []domain.RestrictedRole) error
	ClearRolesForEntity(context.Context, uint, domain.EntityType) error
	Allow(context.Context, uint, domain.EntityType, []domain.Ability) error
	Forbid(context.Context, uint, domain.EntityType, []domain.Ability) error
	Revoke(context.Context, uint, domain.EntityType, []domain.Ability) error
}

type PersonalAccessTokenRepository interface {
	Find(
		ctx context.Context,
		filter *filters.FindPersonalAccessToken,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.PersonalAccessToken, error)

	Save(ctx context.Context, token *domain.PersonalAccessToken) error
	Delete(ctx context.Context, id uint) error

	UpdateLastUsedAt(ctx context.Context, id uint, lastUsedAt time.Time) error
}

type DaemonTaskRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.DaemonTask, error)

	Find(
		ctx context.Context,
		filter *filters.FindDaemonTask,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.DaemonTask, error)

	FindWithOutput(
		ctx context.Context,
		filter *filters.FindDaemonTask,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.DaemonTask, error)

	Count(ctx context.Context, filter *filters.FindDaemonTask) (int, error)

	Save(ctx context.Context, task *domain.DaemonTask) error

	Delete(ctx context.Context, id uint) error

	Exists(ctx context.Context, filter *filters.FindDaemonTask) (bool, error)

	AppendOutput(ctx context.Context, id uint, output string) error
}

type ServerTaskRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.ServerTask, error)

	Find(
		ctx context.Context,
		filter *filters.FindServerTask,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.ServerTask, error)

	Save(ctx context.Context, task *domain.ServerTask) error

	Delete(ctx context.Context, id uint) error
}

type ServerTaskFailRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.ServerTaskFail, error)

	Find(
		ctx context.Context,
		filter *filters.FindServerTaskFail,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.ServerTaskFail, error)

	Save(ctx context.Context, fail *domain.ServerTaskFail) error
}

type ServerSettingRepository interface {
	Find(
		ctx context.Context,
		filter *filters.FindServerSetting,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.ServerSetting, error)

	Save(ctx context.Context, setting *domain.ServerSetting) error

	Delete(ctx context.Context, id uint) error
}

type NodeRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Node, error)

	Find(
		ctx context.Context,
		filter *filters.FindNode,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Node, error)

	Save(ctx context.Context, node *domain.Node) error

	// UpdateGDaemonAPIToken atomically rotates only the daemon API token of a
	// single node, without the full-row read-modify-write of Save (which races
	// concurrent updates to unrelated columns). hashedToken must already be the
	// stored (hashed) representation.
	UpdateGDaemonAPIToken(ctx context.Context, nodeID uint, hashedToken string, updatedAt time.Time) error

	Delete(ctx context.Context, id uint) error
}

type ClientCertificateRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.ClientCertificate, error)

	Find(
		ctx context.Context,
		filter *filters.FindClientCertificate,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.ClientCertificate, error)

	Save(ctx context.Context, certificate *domain.ClientCertificate) error

	Delete(ctx context.Context, id uint) error
}

type PluginStorageRepository interface {
	Find(
		ctx context.Context,
		filter *filters.FindPluginStorage,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.PluginStorageEntry, error)

	Save(ctx context.Context, entry *domain.PluginStorageEntry) error

	Delete(ctx context.Context, id uint64) error

	DeleteByPlugin(ctx context.Context, pluginID uint64) error

	DeleteByFilter(ctx context.Context, filter *filters.FindPluginStorage) error
}

type PluginRepository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Plugin, error)

	Find(
		ctx context.Context,
		filter *filters.FindPlugin,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Plugin, error)

	Save(ctx context.Context, plugin *domain.Plugin) error

	Delete(ctx context.Context, id domain.Uint64ID) error

	Exists(ctx context.Context, filter *filters.FindPlugin) (bool, error)
}

type DLQRepository interface {
	Push(ctx context.Context, msg *dlq.FailedMessage) error
	Pop(ctx context.Context) (*dlq.FailedMessage, error)
	List(ctx context.Context, limit, offset int) ([]dlq.FailedMessage, error)
	Count(ctx context.Context) (int, error)
	MarkProcessed(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	Purge(ctx context.Context) error
}
