package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/samber/lo"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

type PersonalAccessTokenRepository struct {
	db base.DB
}

func NewPersonalAccessTokenRepository(db base.DB) *PersonalAccessTokenRepository {
	return &PersonalAccessTokenRepository{
		db: db,
	}
}

func (r *PersonalAccessTokenRepository) Find(
	ctx context.Context,
	filter *filters.FindPersonalAccessToken,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PersonalAccessToken, error) {
	builder := sq.Select(base.PersonalAccessTokenFields...).
		From(base.PersonalAccessTokensTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *PersonalAccessTokenRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PersonalAccessToken, error) {
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

	var tokens []domain.PersonalAccessToken

	for rows.Next() {
		var token *domain.PersonalAccessToken
		token, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		tokens = append(tokens, *token)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return tokens, nil
}

func (r *PersonalAccessTokenRepository) Save(ctx context.Context, token *domain.PersonalAccessToken) error {
	if token.UpdatedAt == nil || token.UpdatedAt.IsZero() {
		token.UpdatedAt = lo.ToPtr(time.Now())
	}

	if token.ID == 0 && (token.CreatedAt == nil || token.CreatedAt.IsZero()) {
		token.CreatedAt = lo.ToPtr(time.Now())
	}

	var abilitiesJSON []byte
	if token.Abilities != nil && len(*token.Abilities) > 0 {
		var err error
		abilitiesJSON, err = json.Marshal(token.Abilities)
		if err != nil {
			return errors.WithMessage(err, "failed to marshal abilities")
		}
	}

	var lastUsedAtStr, createdAtStr, updatedAtStr *string
	if token.LastUsedAt != nil {
		lastUsedAtStr = lo.ToPtr(token.LastUsedAt.Format(time.RFC3339))
	}
	if token.CreatedAt != nil {
		createdAtStr = lo.ToPtr(token.CreatedAt.Format(time.RFC3339))
	}
	if token.UpdatedAt != nil {
		updatedAtStr = lo.ToPtr(token.UpdatedAt.Format(time.RFC3339))
	}

	query, args, err := sq.Insert(base.PersonalAccessTokensTable).
		Columns(base.PersonalAccessTokenFields...).
		Values(
			lo.EmptyableToPtr(token.ID),
			token.TokenableType,
			token.TokenableID,
			token.Name,
			token.Token,
			abilitiesJSON,
			lastUsedAtStr,
			createdAtStr,
			updatedAtStr,
		).
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"tokenable_type=excluded.tokenable_type," +
			"tokenable_id=excluded.tokenable_id," +
			"name=excluded.name," +
			"token=excluded.token," +
			"abilities=excluded.abilities," +
			"last_used_at=excluded.last_used_at," +
			"updated_at=excluded.updated_at " +
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

	if token.ID == 0 {
		token.ID = returnedID
	}

	return nil
}

func (r *PersonalAccessTokenRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.PersonalAccessTokensTable).
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

func (r *PersonalAccessTokenRepository) UpdateLastUsedAt(ctx context.Context, id uint, lastUsedAt time.Time) error {
	lastUsedAtStr := lastUsedAt.Format(time.RFC3339)

	query, args, err := sq.Update(base.PersonalAccessTokensTable).
		Set("last_used_at", lastUsedAtStr).
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

func (r *PersonalAccessTokenRepository) scan(row base.Scanner) (*domain.PersonalAccessToken, error) {
	var token domain.PersonalAccessToken
	var abilitiesJSON []byte
	var lastUsedAtStr, createdAtStr, updatedAtStr *string

	err := row.Scan(
		&token.ID,
		&token.TokenableType,
		&token.TokenableID,
		&token.Name,
		&token.Token,
		&abilitiesJSON,
		&lastUsedAtStr,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	if len(abilitiesJSON) > 0 {
		var abilities []domain.PATAbility
		err = json.Unmarshal(abilitiesJSON, &abilities)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to unmarshal abilities")
		}
		token.Abilities = &abilities
	}

	if lastUsedAtStr != nil && *lastUsedAtStr != "" {
		lastUsedAt, err := base.ParseTime(*lastUsedAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse last_used_at time")
		}
		token.LastUsedAt = &lastUsedAt
	}

	if createdAtStr != nil && *createdAtStr != "" {
		createdAt, err := base.ParseTime(*createdAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse created_at time")
		}
		token.CreatedAt = &createdAt
	}

	if updatedAtStr != nil && *updatedAtStr != "" {
		updatedAt, err := base.ParseTime(*updatedAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse updated_at time")
		}
		token.UpdatedAt = &updatedAt
	}

	return &token, nil
}

func (r *PersonalAccessTokenRepository) filterToSq(filter *filters.FindPersonalAccessToken) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 4)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.Tokens) > 0 {
		and = append(and, sq.Eq{"token": filter.Tokens})
	}

	if len(filter.TokenableTypes) > 0 {
		and = append(and, sq.Eq{"tokenable_type": filter.TokenableTypes})
	}

	if len(filter.TokenableIDs) > 0 {
		and = append(and, sq.Eq{"tokenable_id": filter.TokenableIDs})
	}

	return and
}
