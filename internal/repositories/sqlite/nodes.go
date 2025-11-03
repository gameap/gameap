package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/samber/lo"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

type NodeRepository struct {
	db base.DB
}

func NewNodeRepository(db base.DB) *NodeRepository {
	return &NodeRepository{
		db: db,
	}
}

func (r *NodeRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	builder := sq.Select(base.NodeFields...).
		From(base.NodesTable).
		Where("deleted_at IS NULL")

	return r.find(ctx, builder, order, pagination)
}

func (r *NodeRepository) Find(
	ctx context.Context,
	filter *filters.FindNode,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	builder := sq.Select(base.NodeFields...).
		From(base.NodesTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *NodeRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy(o.String())
		}
	} else {
		builder = builder.OrderBy("id ASC")
	}

	if pagination != nil {
		if pagination.Limit <= 0 {
			pagination.Limit = filters.DefaultLimit
		}

		if pagination.Offset < 0 {
			pagination.Offset = 0
		}

		builder = builder.Limit(uint64(pagination.Limit)).Offset(uint64(pagination.Offset))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", query, "err", err)
		}
	}(rows)

	var nodes []domain.Node

	for rows.Next() {
		var node *domain.Node
		node, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		nodes = append(nodes, *node)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return nodes, nil
}

//nolint:funlen
func (r *NodeRepository) Save(ctx context.Context, node *domain.Node) error {
	if node.UpdatedAt == nil || node.UpdatedAt.IsZero() {
		node.UpdatedAt = lo.ToPtr(time.Now())
	}

	if node.ID == 0 && (node.CreatedAt == nil || node.CreatedAt.IsZero()) {
		node.CreatedAt = lo.ToPtr(time.Now())
	}

	var createdAtStr, updatedAtStr, deletedAtStr *string
	if node.CreatedAt != nil {
		createdAtStr = lo.ToPtr(node.CreatedAt.Format(time.RFC3339))
	}
	if node.UpdatedAt != nil {
		updatedAtStr = lo.ToPtr(node.UpdatedAt.Format(time.RFC3339))
	}
	if node.DeletedAt != nil {
		deletedAtStr = lo.ToPtr(node.DeletedAt.Format(time.RFC3339))
	}

	query, args, err := sq.Insert(base.NodesTable).
		Columns(base.NodeFields...).
		Values(
			lo.EmptyableToPtr(node.ID),
			node.Enabled,
			node.Name,
			node.OS,
			node.Location,
			node.Provider,
			node.IPs,
			node.RAM,
			node.CPU,
			node.WorkPath,
			node.SteamcmdPath,
			node.GdaemonHost,
			node.GdaemonPort,
			node.GdaemonAPIKey,
			node.GdaemonAPIToken,
			node.GdaemonLogin,
			node.GdaemonPassword,
			node.GdaemonServerCert,
			node.ClientCertificateID,
			node.PreferInstallMethod,
			node.ScriptInstall,
			node.ScriptReinstall,
			node.ScriptUpdate,
			node.ScriptStart,
			node.ScriptPause,
			node.ScriptUnpause,
			node.ScriptStop,
			node.ScriptKill,
			node.ScriptRestart,
			node.ScriptStatus,
			node.ScriptStats,
			node.ScriptGetConsole,
			node.ScriptSendCommand,
			node.ScriptDelete,
			createdAtStr,
			updatedAtStr,
			deletedAtStr,
		).
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"enabled=excluded.enabled," +
			"name=excluded.name," +
			"os=excluded.os," +
			"location=excluded.location," +
			"provider=excluded.provider," +
			"ip=excluded.ip," +
			"ram=excluded.ram," +
			"cpu=excluded.cpu," +
			"work_path=excluded.work_path," +
			"steamcmd_path=excluded.steamcmd_path," +
			"gdaemon_host=excluded.gdaemon_host," +
			"gdaemon_port=excluded.gdaemon_port," +
			"gdaemon_api_key=excluded.gdaemon_api_key," +
			"gdaemon_api_token=excluded.gdaemon_api_token," +
			"gdaemon_login=excluded.gdaemon_login," +
			"gdaemon_password=excluded.gdaemon_password," +
			"gdaemon_server_cert=excluded.gdaemon_server_cert," +
			"client_certificate_id=excluded.client_certificate_id," +
			"prefer_install_method=excluded.prefer_install_method," +
			"script_install=excluded.script_install," +
			"script_reinstall=excluded.script_reinstall," +
			"script_update=excluded.script_update," +
			"script_start=excluded.script_start," +
			"script_pause=excluded.script_pause," +
			"script_unpause=excluded.script_unpause," +
			"script_stop=excluded.script_stop," +
			"script_kill=excluded.script_kill," +
			"script_restart=excluded.script_restart," +
			"script_status=excluded.script_status," +
			"script_stats=excluded.script_stats," +
			"script_get_console=excluded.script_get_console," +
			"script_send_command=excluded.script_send_command," +
			"script_delete=excluded.script_delete," +
			"updated_at=excluded.updated_at," +
			"deleted_at=excluded.deleted_at " +
			"RETURNING id").
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	var returnedID uint
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&returnedID)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	if node.ID == 0 {
		node.ID = returnedID
	}

	return nil
}

func (r *NodeRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.NodesTable).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	return nil
}

func (r *NodeRepository) scan(row base.Scanner) (*domain.Node, error) {
	var node domain.Node
	var createdAtStr, updatedAtStr, deletedAtStr *string

	err := row.Scan(
		&node.ID,
		&node.Enabled,
		&node.Name,
		&node.OS,
		&node.Location,
		&node.Provider,
		&node.IPs,
		&node.RAM,
		&node.CPU,
		&node.WorkPath,
		&node.SteamcmdPath,
		&node.GdaemonHost,
		&node.GdaemonPort,
		&node.GdaemonAPIKey,
		&node.GdaemonAPIToken,
		&node.GdaemonLogin,
		&node.GdaemonPassword,
		&node.GdaemonServerCert,
		&node.ClientCertificateID,
		&node.PreferInstallMethod,
		&node.ScriptInstall,
		&node.ScriptReinstall,
		&node.ScriptUpdate,
		&node.ScriptStart,
		&node.ScriptPause,
		&node.ScriptUnpause,
		&node.ScriptStop,
		&node.ScriptKill,
		&node.ScriptRestart,
		&node.ScriptStatus,
		&node.ScriptStats,
		&node.ScriptGetConsole,
		&node.ScriptSendCommand,
		&node.ScriptDelete,
		&createdAtStr,
		&updatedAtStr,
		&deletedAtStr,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	if createdAtStr != nil && *createdAtStr != "" {
		createdAt, err := base.ParseTime(*createdAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse created_at time")
		}
		node.CreatedAt = &createdAt
	}

	if updatedAtStr != nil && *updatedAtStr != "" {
		updatedAt, err := base.ParseTime(*updatedAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse updated_at time")
		}
		node.UpdatedAt = &updatedAt
	}

	if deletedAtStr != nil && *deletedAtStr != "" {
		deletedAt, err := base.ParseTime(*deletedAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse deleted_at time")
		}
		node.DeletedAt = &deletedAt
	}

	return &node, nil
}

func (r *NodeRepository) filterToSq(filter *filters.FindNode) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 4)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if filter.GDaemonAPIKey != nil {
		and = append(and, sq.Eq{"gdaemon_api_key": *filter.GDaemonAPIKey})
	}

	if filter.GDaemonAPIToken != nil {
		and = append(and, sq.Eq{"gdaemon_api_token": *filter.GDaemonAPIToken})
	}

	if !filter.WithDeleted {
		and = append(and, sq.Expr("deleted_at IS NULL"))
	}

	return and
}
