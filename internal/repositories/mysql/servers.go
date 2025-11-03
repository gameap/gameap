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

type ServerRepository struct {
	db base.DB
	tm base.TransactionManager
}

func NewServerRepository(
	db base.DB,
	tm base.TransactionManager,
) *ServerRepository {
	return &ServerRepository{
		db: db,
		tm: tm,
	}
}

func (r *ServerRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Server, error) {
	builder := sq.Select(base.ServerFields...).
		From(base.ServersTable).
		Where("deleted_at IS NULL")

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerRepository) Find(
	ctx context.Context,
	filter *filters.FindServer,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Server, error) {
	builder := sq.Select(base.ServerFields...).
		From(base.ServersTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerRepository) FindUserServers(
	ctx context.Context,
	userID uint,
	filter *filters.FindServer,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Server, error) {
	builder := sq.Select(base.ServerFields...).
		From(base.ServersTable).
		Join("server_user ON servers.id = server_user.server_id").
		Where(sq.Eq{"server_user.user_id": userID}).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Server, error) {
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
		return nil, errors.WithMessage(err, "failed to build query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck // closed in defer
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", query, "err", err)
		}
	}(rows)

	var servers []domain.Server

	for rows.Next() {
		var server *domain.Server
		server, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		servers = append(servers, *server)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return servers, nil
}

func (r *ServerRepository) Save(ctx context.Context, server *domain.Server) error {
	if server.UpdatedAt == nil || server.UpdatedAt.IsZero() {
		server.UpdatedAt = lo.ToPtr(time.Now())
	}

	if server.ID == 0 && (server.CreatedAt == nil || server.CreatedAt.IsZero()) {
		server.CreatedAt = lo.ToPtr(time.Now())
	}

	query, args, err := sq.Insert(base.ServersTable).
		Columns(base.ServerFields...).
		Values(
			server.ID,
			server.UUID,
			server.UUIDShort,
			server.Enabled,
			server.Installed,
			server.Blocked,
			server.Name,
			server.GameID,
			server.DSID,
			server.GameModID,
			server.Expires,
			server.ServerIP,
			server.ServerPort,
			server.QueryPort,
			server.RconPort,
			server.Rcon,
			server.Dir,
			server.SuUser,
			server.CPULimit,
			server.RAMLimit,
			server.NetLimit,
			server.StartCommand,
			server.StopCommand,
			server.ForceStopCommand,
			server.RestartCommand,
			server.ProcessActive,
			server.LastProcessCheck,
			server.Vars,
			server.CreatedAt,
			server.UpdatedAt,
			server.DeletedAt,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"uuid=VALUES(uuid)," +
			"uuid_short=VALUES(uuid_short)," +
			"enabled=VALUES(enabled)," +
			"installed=VALUES(installed)," +
			"blocked=VALUES(blocked)," +
			"name=VALUES(name)," +
			"game_id=VALUES(game_id)," +
			"ds_id=VALUES(ds_id)," +
			"game_mod_id=VALUES(game_mod_id)," +
			"expires=VALUES(expires)," +
			"server_ip=VALUES(server_ip)," +
			"server_port=VALUES(server_port)," +
			"query_port=VALUES(query_port)," +
			"rcon_port=VALUES(rcon_port)," +
			"rcon=VALUES(rcon)," +
			"dir=VALUES(dir)," +
			"su_user=VALUES(su_user)," +
			"cpu_limit=VALUES(cpu_limit)," +
			"ram_limit=VALUES(ram_limit)," +
			"net_limit=VALUES(net_limit)," +
			"start_command=VALUES(start_command)," +
			"stop_command=VALUES(stop_command)," +
			"force_stop_command=VALUES(force_stop_command)," +
			"restart_command=VALUES(restart_command)," +
			"process_active=VALUES(process_active)," +
			"last_process_check=VALUES(last_process_check)," +
			"vars=VALUES(vars)," +
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

	// If this is a new server (ID is 0), get the inserted ID
	if server.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		server.ID = uint(lastID)
	}

	return nil
}

func (r *ServerRepository) SaveBulk(ctx context.Context, servers []*domain.Server) error {
	if len(servers) == 0 {
		return nil
	}

	builder := sq.Insert(base.ServersTable).
		Columns(base.ServerFields...)

	for _, server := range servers {
		builder = builder.Values(
			server.ID,
			server.UUID,
			server.UUIDShort,
			server.Enabled,
			server.Installed,
			server.Blocked,
			server.Name,
			server.GameID,
			server.DSID,
			server.GameModID,
			server.Expires,
			server.ServerIP,
			server.ServerPort,
			server.QueryPort,
			server.RconPort,
			server.Rcon,
			server.Dir,
			server.SuUser,
			server.CPULimit,
			server.RAMLimit,
			server.NetLimit,
			server.StartCommand,
			server.StopCommand,
			server.ForceStopCommand,
			server.RestartCommand,
			server.ProcessActive,
			server.LastProcessCheck,
			server.Vars,
			server.CreatedAt,
			server.UpdatedAt,
			server.DeletedAt,
		)
	}

	query, args, err := builder.
		Suffix("ON DUPLICATE KEY UPDATE " +
			"uuid=VALUES(uuid)," +
			"uuid_short=VALUES(uuid_short)," +
			"enabled=VALUES(enabled)," +
			"installed=VALUES(installed)," +
			"blocked=VALUES(blocked)," +
			"name=VALUES(name)," +
			"game_id=VALUES(game_id)," +
			"ds_id=VALUES(ds_id)," +
			"game_mod_id=VALUES(game_mod_id)," +
			"expires=VALUES(expires)," +
			"server_ip=VALUES(server_ip)," +
			"server_port=VALUES(server_port)," +
			"query_port=VALUES(query_port)," +
			"rcon_port=VALUES(rcon_port)," +
			"rcon=VALUES(rcon)," +
			"dir=VALUES(dir)," +
			"su_user=VALUES(su_user)," +
			"cpu_limit=VALUES(cpu_limit)," +
			"ram_limit=VALUES(ram_limit)," +
			"net_limit=VALUES(net_limit)," +
			"start_command=VALUES(start_command)," +
			"stop_command=VALUES(stop_command)," +
			"force_stop_command=VALUES(force_stop_command)," +
			"restart_command=VALUES(restart_command)," +
			"process_active=VALUES(process_active)," +
			"last_process_check=VALUES(last_process_check)," +
			"vars=VALUES(vars)," +
			"updated_at=VALUES(updated_at)," +
			"deleted_at=VALUES(deleted_at)").
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

func (r *ServerRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.ServersTable).
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

func (r *ServerRepository) SetUserServers(ctx context.Context, userID uint, serverIDs []uint) error {
	return r.tm.Do(ctx, func(ctx context.Context) error {
		deleteQuery, deleteArgs, err := sq.Delete("server_user").
			Where(sq.Eq{"user_id": userID}).
			PlaceholderFormat(sq.Question).
			ToSql()
		if err != nil {
			return errors.WithMessage(err, "failed to build delete query")
		}

		_, err = r.db.ExecContext(ctx, deleteQuery, deleteArgs...)
		if err != nil {
			return errors.WithMessage(err, "failed to delete existing relationships")
		}

		// Insert new relationships if there are any
		if len(serverIDs) > 0 {
			insertBuilder := sq.Insert("server_user").
				Columns("user_id", "server_id").
				PlaceholderFormat(sq.Question)

			for _, serverID := range serverIDs {
				insertBuilder = insertBuilder.Values(userID, serverID)
			}

			insertQuery, insertArgs, err := insertBuilder.ToSql()
			if err != nil {
				return errors.WithMessage(err, "failed to build insert query")
			}

			_, err = r.db.ExecContext(ctx, insertQuery, insertArgs...)
			if err != nil {
				return errors.WithMessage(err, "failed to insert new relationships")
			}
		}

		return nil
	})
}

func (r *ServerRepository) Exists(ctx context.Context, filter *filters.FindServer) (bool, error) {
	if filter == nil {
		return false, nil
	}

	innerQuery, args, err := sq.Select("1").
		From(base.ServersTable).
		Where(r.filterToSq(filter)).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return false, errors.WithMessage(err, "failed to build query")
	}

	// Wrap with EXISTS
	query := "SELECT EXISTS(" + innerQuery + ")"

	var exists bool
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, errors.WithMessage(err, "failed to execute query")
	}

	return exists, nil
}

func (r *ServerRepository) Search(ctx context.Context, query string) ([]*domain.Server, error) {
	// Select only the specific fields requested
	searchFields := []string{
		"id",
		"name",
		"server_ip",
		"server_port",
		"game_id",
		"game_mod_id",
	}

	builder := sq.Select(searchFields...).
		From(base.ServersTable).
		Where("deleted_at IS NULL")

	// If query length is less than 3 characters, return first 10 servers
	if len(query) < 3 {
		builder = builder.Limit(10)
	} else {
		// Search for servers where name, server_ip, or server_port matches the query
		searchPattern := "%" + query + "%"
		builder = builder.Where(
			sq.Or{
				sq.Like{"name": searchPattern},
				sq.Like{"server_ip": searchPattern},
				sq.Like{"server_port": searchPattern},
			},
		)
	}

	sqlQuery, args, err := builder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build search query")
	}

	rows, err := r.db.QueryContext(ctx, sqlQuery, args...) //nolint:sqlclosecheck // closed in defer
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute search query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", sqlQuery, "err", err)
		}
	}(rows)

	var servers []*domain.Server

	for rows.Next() {
		var server domain.Server
		err = rows.Scan(
			&server.ID,
			&server.Name,
			&server.ServerIP,
			&server.ServerPort,
			&server.GameID,
			&server.GameModID,
		)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan search result")
		}

		servers = append(servers, &server)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "search rows iteration error")
	}

	return servers, nil
}

func (r *ServerRepository) scan(row base.Scanner) (*domain.Server, error) {
	var server domain.Server

	err := row.Scan(
		&server.ID,
		&server.UUID,
		&server.UUIDShort,
		&server.Enabled,
		&server.Installed,
		&server.Blocked,
		&server.Name,
		&server.GameID,
		&server.DSID,
		&server.GameModID,
		&server.Expires,
		&server.ServerIP,
		&server.ServerPort,
		&server.QueryPort,
		&server.RconPort,
		&server.Rcon,
		&server.Dir,
		&server.SuUser,
		&server.CPULimit,
		&server.RAMLimit,
		&server.NetLimit,
		&server.StartCommand,
		&server.StopCommand,
		&server.ForceStopCommand,
		&server.RestartCommand,
		&server.ProcessActive,
		&server.LastProcessCheck,
		&server.Vars,
		&server.CreatedAt,
		&server.UpdatedAt,
		&server.DeletedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &server, nil
}

func (r *ServerRepository) filterToSq(filter *filters.FindServer) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 9)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.UUIDs) > 0 {
		and = append(and, sq.Eq{"uuid": filter.UUIDs})
	}

	if len(filter.UserIDs) > 0 {
		// For user filter, we need to join with server_user table
		subQuery := sq.Select("server_id").
			From("server_user").
			Where(sq.Eq{"user_id": filter.UserIDs})
		and = append(and, sq.Expr("id IN (?)", subQuery))
	}

	if filter.Enabled != nil {
		and = append(and, sq.Eq{"enabled": *filter.Enabled})
	}

	if filter.Blocked != nil {
		and = append(and, sq.Eq{"blocked": *filter.Blocked})
	}

	if len(filter.GameIDs) > 0 {
		and = append(and, sq.Eq{"game_id": filter.GameIDs})
	}

	if len(filter.DSIDs) > 0 {
		and = append(and, sq.Eq{"ds_id": filter.DSIDs})
	}

	if len(filter.Names) > 0 {
		and = append(and, sq.Eq{"name": filter.Names})
	}

	if len(filter.GameModIDs) > 0 {
		and = append(and, sq.Eq{"game_mod_id": filter.GameModIDs})
	}

	if !filter.WithDeleted {
		and = append(and, sq.Expr("deleted_at IS NULL"))
	}

	return and
}
