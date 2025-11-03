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

type ClientCertificateRepository struct {
	db base.DB
}

func NewClientCertificateRepository(db base.DB) *ClientCertificateRepository {
	return &ClientCertificateRepository{
		db: db,
	}
}

func (r *ClientCertificateRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ClientCertificate, error) {
	builder := sq.Select(base.ClientCertificateFields...).
		From(base.ClientCertificatesTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *ClientCertificateRepository) Find(
	ctx context.Context,
	filter *filters.FindClientCertificate,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ClientCertificate, error) {
	builder := sq.Select(base.ClientCertificateFields...).
		From(base.ClientCertificatesTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *ClientCertificateRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ClientCertificate, error) {
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

	var certificates []domain.ClientCertificate

	for rows.Next() {
		var certificate *domain.ClientCertificate
		certificate, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		certificates = append(certificates, *certificate)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return certificates, nil
}

func (r *ClientCertificateRepository) Save(ctx context.Context, certificate *domain.ClientCertificate) error {
	query, args, err := sq.Insert(base.ClientCertificatesTable).
		Columns(base.ClientCertificateFields...).
		Values(
			certificate.ID,
			certificate.Fingerprint,
			certificate.Expires,
			certificate.Certificate,
			certificate.PrivateKey,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"fingerprint=VALUES(fingerprint)," +
			"expires=VALUES(expires)," +
			"certificate=VALUES(certificate)," +
			"private_key=VALUES(private_key)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	// If this is a new certificate (ID is 0), get the inserted ID
	if certificate.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		certificate.ID = uint(lastID)
	}

	return nil
}

func (r *ClientCertificateRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.ClientCertificatesTable).
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

func (r *ClientCertificateRepository) scan(row base.Scanner) (*domain.ClientCertificate, error) {
	var certificate domain.ClientCertificate

	err := row.Scan(
		&certificate.ID,
		&certificate.Fingerprint,
		&certificate.Expires,
		&certificate.Certificate,
		&certificate.PrivateKey,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &certificate, nil
}

func (r *ClientCertificateRepository) filterToSq(filter *filters.FindClientCertificate) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 1)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	return and
}
