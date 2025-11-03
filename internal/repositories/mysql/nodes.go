package mysql

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

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck // closed in defer
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

	query, args, err := sq.Insert(base.NodesTable).
		Columns(base.NodeFields...).
		Values(
			node.ID,
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
			node.CreatedAt,
			node.UpdatedAt,
			node.DeletedAt,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"enabled=VALUES(enabled)," +
			"name=VALUES(name)," +
			"os=VALUES(os)," +
			"location=VALUES(location)," +
			"provider=VALUES(provider)," +
			"ip=VALUES(ip)," +
			"ram=VALUES(ram)," +
			"cpu=VALUES(cpu)," +
			"work_path=VALUES(work_path)," +
			"steamcmd_path=VALUES(steamcmd_path)," +
			"gdaemon_host=VALUES(gdaemon_host)," +
			"gdaemon_port=VALUES(gdaemon_port)," +
			"gdaemon_api_key=VALUES(gdaemon_api_key)," +
			"gdaemon_api_token=VALUES(gdaemon_api_token)," +
			"gdaemon_login=VALUES(gdaemon_login)," +
			"gdaemon_password=VALUES(gdaemon_password)," +
			"gdaemon_server_cert=VALUES(gdaemon_server_cert)," +
			"client_certificate_id=VALUES(client_certificate_id)," +
			"prefer_install_method=VALUES(prefer_install_method)," +
			"script_install=VALUES(script_install)," +
			"script_reinstall=VALUES(script_reinstall)," +
			"script_update=VALUES(script_update)," +
			"script_start=VALUES(script_start)," +
			"script_pause=VALUES(script_pause)," +
			"script_unpause=VALUES(script_unpause)," +
			"script_stop=VALUES(script_stop)," +
			"script_kill=VALUES(script_kill)," +
			"script_restart=VALUES(script_restart)," +
			"script_status=VALUES(script_status)," +
			"script_stats=VALUES(script_stats)," +
			"script_get_console=VALUES(script_get_console)," +
			"script_send_command=VALUES(script_send_command)," +
			"script_delete=VALUES(script_delete)," +
			"updated_at=VALUES(updated_at)," +
			"deleted_at=VALUES(deleted_at)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	// If this is a new node (ID is 0), get the inserted ID
	if node.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		node.ID = uint(lastID)
	}

	return nil
}

func (r *NodeRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.NodesTable).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Question).
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
		&node.CreatedAt,
		&node.UpdatedAt,
		&node.DeletedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
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
