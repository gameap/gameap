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

type ServerSettingRepository struct {
	db base.DB
}

func NewServerSettingRepository(db base.DB) *ServerSettingRepository {
	return &ServerSettingRepository{
		db: db,
	}
}

func (r *ServerSettingRepository) Find(
	ctx context.Context,
	filter *filters.FindServerSetting,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerSetting, error) {
	builder := sq.Select(base.ServerSettingFields...).
		From(base.ServerSettingsTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerSettingRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerSetting, error) {
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

	var settings []domain.ServerSetting

	for rows.Next() {
		var setting *domain.ServerSetting
		setting, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		settings = append(settings, *setting)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return settings, nil
}

func (r *ServerSettingRepository) Save(ctx context.Context, setting *domain.ServerSetting) error {
	query, args, err := sq.Insert(base.ServerSettingsTable).
		Columns(base.ServerSettingFields...).
		Values(
			setting.ID,
			setting.Name,
			setting.ServerID,
			setting.Value,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"name=VALUES(name)," +
			"server_id=VALUES(server_id)," +
			"value=VALUES(value)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	// If this is a new setting (ID is 0), get the inserted ID
	if setting.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		setting.ID = uint(lastID)
	}

	return nil
}

func (r *ServerSettingRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.ServerSettingsTable).
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

func (r *ServerSettingRepository) scan(row base.Scanner) (*domain.ServerSetting, error) {
	var setting domain.ServerSetting

	err := row.Scan(
		&setting.ID,
		&setting.Name,
		&setting.ServerID,
		&setting.Value,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &setting, nil
}

func (r *ServerSettingRepository) filterToSq(filter *filters.FindServerSetting) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 3)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.ServerIDs) > 0 {
		and = append(and, sq.Eq{"server_id": filter.ServerIDs})
	}

	if len(filter.Names) > 0 {
		and = append(and, sq.Eq{"name": filter.Names})
	}

	return and
}
