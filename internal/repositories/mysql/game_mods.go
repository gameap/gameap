package mysql

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/base"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

type GameModRepository struct {
	db base.DB
}

func NewGameModRepository(db base.DB) *GameModRepository {
	return &GameModRepository{
		db: db,
	}
}

func (r *GameModRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	builder := sq.Select(base.GameModFields...).
		From(base.GameModsTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *GameModRepository) Find(
	ctx context.Context,
	filter *filters.FindGameMod,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	builder := sq.Select(base.GameModFields...).
		From(base.GameModsTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *GameModRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy(o.String())
		}
	} else {
		builder = builder.OrderBy("name ASC")
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

	var gameMods []domain.GameMod

	for rows.Next() {
		var gameMod *domain.GameMod
		gameMod, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		gameMods = append(gameMods, *gameMod)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return gameMods, nil
}

func (r *GameModRepository) Save(ctx context.Context, gameMod *domain.GameMod) error {
	query, args, err := sq.Insert(base.GameModsTable).
		Columns(base.GameModFields...).
		Values(
			gameMod.ID,
			gameMod.GameCode,
			gameMod.Name,
			gameMod.FastRcon,
			gameMod.Vars,
			gameMod.RemoteRepositoryLinux,
			gameMod.RemoteRepositoryWindows,
			gameMod.LocalRepositoryLinux,
			gameMod.LocalRepositoryWindows,
			gameMod.StartCmdLinux,
			gameMod.StartCmdWindows,
			gameMod.KickCmd,
			gameMod.BanCmd,
			gameMod.ChnameCmd,
			gameMod.SrestartCmd,
			gameMod.ChmapCmd,
			gameMod.SendmsgCmd,
			gameMod.PasswdCmd,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"game_code=VALUES(game_code)," +
			"name=VALUES(name)," +
			"fast_rcon=VALUES(fast_rcon)," +
			"vars=VALUES(vars)," +
			"remote_repository_linux=VALUES(remote_repository_linux)," +
			"remote_repository_windows=VALUES(remote_repository_windows)," +
			"local_repository_linux=VALUES(local_repository_linux)," +
			"local_repository_windows=VALUES(local_repository_windows)," +
			"start_cmd_linux=VALUES(start_cmd_linux)," +
			"start_cmd_windows=VALUES(start_cmd_windows)," +
			"kick_cmd=VALUES(kick_cmd)," +
			"ban_cmd=VALUES(ban_cmd)," +
			"chname_cmd=VALUES(chname_cmd)," +
			"srestart_cmd=VALUES(srestart_cmd)," +
			"chmap_cmd=VALUES(chmap_cmd)," +
			"sendmsg_cmd=VALUES(sendmsg_cmd)," +
			"passwd_cmd=VALUES(passwd_cmd)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	if gameMod.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		gameMod.ID = uint(lastID)
	}

	return nil
}

func (r *GameModRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.GameModsTable).
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

func (r *GameModRepository) scan(row base.Scanner) (*domain.GameMod, error) {
	var gameMod domain.GameMod

	err := row.Scan(
		&gameMod.ID,
		&gameMod.GameCode,
		&gameMod.Name,
		&gameMod.FastRcon,
		&gameMod.Vars,
		&gameMod.RemoteRepositoryLinux,
		&gameMod.RemoteRepositoryWindows,
		&gameMod.LocalRepositoryLinux,
		&gameMod.LocalRepositoryWindows,
		&gameMod.StartCmdLinux,
		&gameMod.StartCmdWindows,
		&gameMod.KickCmd,
		&gameMod.BanCmd,
		&gameMod.ChnameCmd,
		&gameMod.SrestartCmd,
		&gameMod.ChmapCmd,
		&gameMod.SendmsgCmd,
		&gameMod.PasswdCmd,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &gameMod, nil
}

func (r *GameModRepository) filterToSq(filter *filters.FindGameMod) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 3)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.GameCodes) > 0 {
		and = append(and, sq.Eq{"game_code": filter.GameCodes})
	}

	if len(filter.Names) > 0 {
		and = append(and, sq.Eq{"name": filter.Names})
	}

	return and
}
