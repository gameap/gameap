package sqlite

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

type GameRepository struct {
	db base.DB
}

func NewGameRepository(db base.DB) *GameRepository {
	return &GameRepository{
		db: db,
	}
}

func (r *GameRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Game, error) {
	builder := sq.Select(base.GameFields...).
		From(base.GamesTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *GameRepository) Find(
	ctx context.Context,
	filter *filters.FindGame,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Game, error) {
	builder := sq.Select(base.GameFields...).
		From(base.GamesTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *GameRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Game, error) {
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

	var games []domain.Game

	for rows.Next() {
		var game *domain.Game
		game, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		games = append(games, *game)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return games, nil
}

func (r *GameRepository) Save(ctx context.Context, game *domain.Game) error {
	query, args, err := sq.Insert(base.GamesTable).
		Columns(base.GameFields...).
		Values(
			game.Code,
			game.Name,
			game.Engine,
			game.EngineVersion,
			game.SteamAppIDLinux,
			game.SteamAppIDWindows,
			game.SteamAppSetConfig,
			game.RemoteRepositoryLinux,
			game.RemoteRepositoryWindows,
			game.LocalRepositoryLinux,
			game.LocalRepositoryWindows,
			game.Enabled,
		).
		Suffix("ON CONFLICT(code) DO UPDATE SET " +
			"name=excluded.name," +
			"engine=excluded.engine," +
			"engine_version=excluded.engine_version," +
			"steam_app_id_linux=excluded.steam_app_id_linux," +
			"steam_app_id_windows=excluded.steam_app_id_windows," +
			"steam_app_set_config=excluded.steam_app_set_config," +
			"remote_repository_linux=excluded.remote_repository_linux," +
			"remote_repository_windows=excluded.remote_repository_windows," +
			"local_repository_linux=excluded.local_repository_linux," +
			"local_repository_windows=excluded.local_repository_windows," +
			"enabled=excluded.enabled").
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

func (r *GameRepository) Delete(ctx context.Context, code string) error {
	query, args, err := sq.Delete(base.GamesTable).
		Where(sq.Eq{"code": code}).
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

func (r *GameRepository) scan(row base.Scanner) (*domain.Game, error) {
	var game domain.Game

	err := row.Scan(
		&game.Code,
		&game.Name,
		&game.Engine,
		&game.EngineVersion,
		&game.SteamAppIDLinux,
		&game.SteamAppIDWindows,
		&game.SteamAppSetConfig,
		&game.RemoteRepositoryLinux,
		&game.RemoteRepositoryWindows,
		&game.LocalRepositoryLinux,
		&game.LocalRepositoryWindows,
		&game.Enabled,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &game, nil
}

func (r *GameRepository) filterToSq(filter *filters.FindGame) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 1)

	if len(filter.Codes) > 0 {
		and = append(and, sq.Eq{"code": filter.Codes})
	}

	return and
}
