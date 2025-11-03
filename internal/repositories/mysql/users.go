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

type UserRepository struct {
	db base.DB
}

func NewUserRepository(db base.DB) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

func (r *UserRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	builder := sq.Select(base.UserFields...).
		From(base.UsersTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *UserRepository) Find(
	ctx context.Context,
	filter *filters.FindUser,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	builder := sq.Select(base.UserFields...).
		From(base.UsersTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *UserRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
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

	var users []domain.User

	for rows.Next() {
		var user *domain.User
		user, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		users = append(users, *user)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return users, nil
}

func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	if user.UpdatedAt == nil || user.UpdatedAt.IsZero() {
		user.UpdatedAt = lo.ToPtr(time.Now())
	}

	if user.ID == 0 && (user.CreatedAt == nil || user.CreatedAt.IsZero()) {
		user.CreatedAt = lo.ToPtr(time.Now())
	}

	query, args, err := sq.Insert(base.UsersTable).
		Columns(base.UserFields...).
		Values(
			user.ID,
			user.Login,
			user.Email,
			user.Password,
			user.RememberToken,
			user.Name,
			user.CreatedAt,
			user.UpdatedAt,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"login=VALUES(login)," +
			"email=VALUES(email)," +
			"password=VALUES(password)," +
			"remember_token=VALUES(remember_token)," +
			"name=VALUES(name)," +
			"updated_at=VALUES(updated_at)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	// If this is a new user (ID is 0), get the inserted ID
	if user.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		user.ID = uint(lastID)
	}

	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.UsersTable).
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

func (r *UserRepository) scan(row base.Scanner) (*domain.User, error) {
	var user domain.User

	err := row.Scan(
		&user.ID,
		&user.Login,
		&user.Email,
		&user.Password,
		&user.RememberToken,
		&user.Name,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &user, nil
}

func (r *UserRepository) filterToSq(filter *filters.FindUser) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 3)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.Logins) > 0 {
		and = append(and, sq.Eq{"login": filter.Logins})
	}

	if len(filter.Emails) > 0 {
		and = append(and, sq.Eq{"email": filter.Emails})
	}

	return and
}
