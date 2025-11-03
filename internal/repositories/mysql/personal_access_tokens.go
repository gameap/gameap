package mysql

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

	// Serialize abilities to JSON
	var abilitiesJSON []byte
	if token.Abilities != nil && len(*token.Abilities) > 0 {
		var err error
		abilitiesJSON, err = json.Marshal(token.Abilities)
		if err != nil {
			return errors.WithMessage(err, "failed to marshal abilities")
		}
	}

	query, args, err := sq.Insert(base.PersonalAccessTokensTable).
		Columns(base.PersonalAccessTokenFields...).
		Values(
			token.ID,
			token.TokenableType,
			token.TokenableID,
			token.Name,
			token.Token,
			abilitiesJSON,
			token.LastUsedAt,
			token.CreatedAt,
			token.UpdatedAt,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"tokenable_type=VALUES(tokenable_type)," +
			"tokenable_id=VALUES(tokenable_id)," +
			"name=VALUES(name)," +
			"token=VALUES(token)," +
			"abilities=VALUES(abilities)," +
			"last_used_at=VALUES(last_used_at)," +
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

	// If this is a new token (ID is 0), get the inserted ID
	if token.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		token.ID = uint(lastID)
	}

	return nil
}

func (r *PersonalAccessTokenRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.PersonalAccessTokensTable).
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

func (r *PersonalAccessTokenRepository) UpdateLastUsedAt(ctx context.Context, id uint, lastUsedAt time.Time) error {
	query, args, err := sq.Update(base.PersonalAccessTokensTable).
		Set("last_used_at", lastUsedAt).
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

func (r *PersonalAccessTokenRepository) scan(row base.Scanner) (*domain.PersonalAccessToken, error) {
	var token domain.PersonalAccessToken
	var abilitiesJSON []byte

	err := row.Scan(
		&token.ID,
		&token.TokenableType,
		&token.TokenableID,
		&token.Name,
		&token.Token,
		&abilitiesJSON,
		&token.LastUsedAt,
		&token.CreatedAt,
		&token.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	// Deserialize abilities from JSON
	if len(abilitiesJSON) > 0 {
		var abilities []domain.PATAbility
		err = json.Unmarshal(abilitiesJSON, &abilities)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to unmarshal abilities")
		}
		token.Abilities = &abilities
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
