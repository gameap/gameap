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

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck
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

	formatTime := func(t *time.Time) *string {
		if t != nil {
			return lo.ToPtr(t.Format(time.RFC3339))
		}

		return nil
	}

	query, args, err := sq.Insert(base.ServersTable).
		Columns(base.ServerFields...).
		Values(
			lo.EmptyableToPtr(server.ID),
			server.UUID,
			server.UUIDShort,
			server.Enabled,
			server.Installed,
			server.Blocked,
			server.Name,
			server.GameID,
			server.DSID,
			server.GameModID,
			formatTime(server.Expires),
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
			formatTime(server.LastProcessCheck),
			server.Vars,
			formatTime(server.CreatedAt),
			formatTime(server.UpdatedAt),
			formatTime(server.DeletedAt),
		).
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"uuid=excluded.uuid," +
			"uuid_short=excluded.uuid_short," +
			"enabled=excluded.enabled," +
			"installed=excluded.installed," +
			"blocked=excluded.blocked," +
			"name=excluded.name," +
			"game_id=excluded.game_id," +
			"ds_id=excluded.ds_id," +
			"game_mod_id=excluded.game_mod_id," +
			"expires=excluded.expires," +
			"server_ip=excluded.server_ip," +
			"server_port=excluded.server_port," +
			"query_port=excluded.query_port," +
			"rcon_port=excluded.rcon_port," +
			"rcon=excluded.rcon," +
			"dir=excluded.dir," +
			"su_user=excluded.su_user," +
			"cpu_limit=excluded.cpu_limit," +
			"ram_limit=excluded.ram_limit," +
			"net_limit=excluded.net_limit," +
			"start_command=excluded.start_command," +
			"stop_command=excluded.stop_command," +
			"force_stop_command=excluded.force_stop_command," +
			"restart_command=excluded.restart_command," +
			"process_active=excluded.process_active," +
			"last_process_check=excluded.last_process_check," +
			"vars=excluded.vars," +
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

	if server.ID == 0 {
		server.ID = returnedID
	}

	return nil
}

func (r *ServerRepository) SaveBulk(ctx context.Context, servers []*domain.Server) error {
	if len(servers) == 0 {
		return nil
	}

	formatTime := func(t *time.Time) *string {
		if t != nil {
			return lo.ToPtr(t.Format(time.RFC3339))
		}

		return nil
	}

	builder := sq.Insert(base.ServersTable).
		Columns(base.ServerFields...)

	for _, server := range servers {
		builder = builder.Values(
			lo.EmptyableToPtr(server.ID),
			server.UUID,
			server.UUIDShort,
			server.Enabled,
			server.Installed,
			server.Blocked,
			server.Name,
			server.GameID,
			server.DSID,
			server.GameModID,
			formatTime(server.Expires),
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
			formatTime(server.LastProcessCheck),
			server.Vars,
			formatTime(server.CreatedAt),
			formatTime(server.UpdatedAt),
			formatTime(server.DeletedAt),
		)
	}

	query, args, err := builder.
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"uuid=excluded.uuid," +
			"uuid_short=excluded.uuid_short," +
			"enabled=excluded.enabled," +
			"installed=excluded.installed," +
			"blocked=excluded.blocked," +
			"name=excluded.name," +
			"game_id=excluded.game_id," +
			"ds_id=excluded.ds_id," +
			"game_mod_id=excluded.game_mod_id," +
			"expires=excluded.expires," +
			"server_ip=excluded.server_ip," +
			"server_port=excluded.server_port," +
			"query_port=excluded.query_port," +
			"rcon_port=excluded.rcon_port," +
			"rcon=excluded.rcon," +
			"dir=excluded.dir," +
			"su_user=excluded.su_user," +
			"cpu_limit=excluded.cpu_limit," +
			"ram_limit=excluded.ram_limit," +
			"net_limit=excluded.net_limit," +
			"start_command=excluded.start_command," +
			"stop_command=excluded.stop_command," +
			"force_stop_command=excluded.force_stop_command," +
			"restart_command=excluded.restart_command," +
			"process_active=excluded.process_active," +
			"last_process_check=excluded.last_process_check," +
			"vars=excluded.vars," +
			"updated_at=excluded.updated_at," +
			"deleted_at=excluded.deleted_at").
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
			ToSql()
		if err != nil {
			return errors.WithMessage(err, "failed to build delete query")
		}

		_, err = r.db.ExecContext(ctx, deleteQuery, deleteArgs...)
		if err != nil {
			return errors.WithMessage(err, "failed to delete existing relationships")
		}

		if len(serverIDs) > 0 {
			insertBuilder := sq.Insert("server_user").
				Columns("user_id", "server_id")

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
		ToSql()
	if err != nil {
		return false, errors.WithMessage(err, "failed to build query")
	}

	query := "SELECT EXISTS(" + innerQuery + ")"

	var exists bool
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, errors.WithMessage(err, "failed to execute query")
	}

	return exists, nil
}

func (r *ServerRepository) Search(ctx context.Context, query string) ([]*domain.Server, error) {
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

	if len(query) < 3 {
		builder = builder.Limit(10)
	} else {
		searchPattern := "%" + query + "%"
		builder = builder.Where(
			sq.Or{
				sq.Like{"name": searchPattern},
				sq.Like{"server_ip": searchPattern},
				sq.Like{"server_port": searchPattern},
			},
		)
	}

	sqlQuery, args, err := builder.ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build search query")
	}

	rows, err := r.db.QueryContext(ctx, sqlQuery, args...) //nolint:sqlclosecheck
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
	var expiresStr, lastProcessCheckStr, createdAtStr, updatedAtStr, deletedAtStr *string

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
		&expiresStr,
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
		&lastProcessCheckStr,
		&server.Vars,
		&createdAtStr,
		&updatedAtStr,
		&deletedAtStr,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	if expiresStr != nil && *expiresStr != "" {
		expires, err := base.ParseTime(*expiresStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse expires time")
		}
		server.Expires = &expires
	}

	if lastProcessCheckStr != nil && *lastProcessCheckStr != "" {
		lastProcessCheck, err := base.ParseTime(*lastProcessCheckStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse last_process_check time")
		}
		server.LastProcessCheck = &lastProcessCheck
	}

	if createdAtStr != nil && *createdAtStr != "" {
		createdAt, err := base.ParseTime(*createdAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse created_at time")
		}
		server.CreatedAt = &createdAt
	}

	if updatedAtStr != nil && *updatedAtStr != "" {
		updatedAt, err := base.ParseTime(*updatedAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse updated_at time")
		}
		server.UpdatedAt = &updatedAt
	}

	if deletedAtStr != nil && *deletedAtStr != "" {
		deletedAt, err := base.ParseTime(*deletedAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse deleted_at time")
		}
		server.DeletedAt = &deletedAt
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
