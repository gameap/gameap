package base

import (
	"strconv"

	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

// MaxPageSize caps page[size] so a client cannot force an unbounded LIMIT and
// exhaust memory / DoS the database. Requests above it are clamped, not
// rejected, so a well-behaved client asking for "as many as possible" still
// succeeds with a bounded page.
const MaxPageSize = 100

// ReadPage parses JSON:API page[number] / page[size] query parameters. Missing
// values default to page 1 and DefaultPageSize; page[size] is clamped to
// MaxPageSize. A malformed integer or a non-positive value is an error.
//
// Use this instead of hand-rolling the parse in each list handler so the upper
// bound is enforced uniformly.
func ReadPage(q *api.QueryReader) (pageNumber, pageSize int, err error) {
	pageNumber = 1
	pageSize = DefaultPageSize

	numberStr, err := q.ReadString("page[number]")
	if err != nil {
		return 0, 0, errors.WithMessage(err, "failed to read page[number]")
	}
	if numberStr != "" {
		pageNumber, err = strconv.Atoi(numberStr)
		if err != nil {
			return 0, 0, errors.WithMessage(err, "invalid page[number] value")
		}
		if pageNumber < 1 {
			return 0, 0, errors.New("page[number] must be positive")
		}
	}

	sizeStr, err := q.ReadString("page[size]")
	if err != nil {
		return 0, 0, errors.WithMessage(err, "failed to read page[size]")
	}
	if sizeStr != "" {
		pageSize, err = strconv.Atoi(sizeStr)
		if err != nil {
			return 0, 0, errors.WithMessage(err, "invalid page[size] value")
		}
		if pageSize < 1 {
			return 0, 0, errors.New("page[size] must be positive")
		}
		if pageSize > MaxPageSize {
			pageSize = MaxPageSize
		}
	}

	return pageNumber, pageSize, nil
}
