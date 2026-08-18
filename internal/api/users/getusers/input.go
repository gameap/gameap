package getusers

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type input struct {
	IDs    []uint
	Logins []string
	Emails []string

	// Paginated stays opt-in. This endpoint has always answered with a bare
	// array and the panel's user list depends on that shape, so a page
	// envelope would be a breaking change. A caller that asks for a page
	// simply gets a shorter array.
	Paginated  bool
	PageNumber int
	PageSize   int
}

func readInput(r *http.Request) (*input, error) {
	queryReader := api.NewQueryReader(r)

	result := &input{}

	ids, err := queryReader.ReadUintList("filter[id]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[id] list")
	}
	result.IDs = ids

	logins, err := queryReader.ReadList("filter[login]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[login] list")
	}
	result.Logins = logins

	// Matching is exact. Pass every spelling you want to accept — the filter
	// turns a list into an IN (...), so a caller unsure about the stored
	// casing can send both the original and the lower-cased address.
	emails, err := queryReader.ReadList("filter[email]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[email] list")
	}
	result.Emails = emails

	pageRequested, err := isPageRequested(queryReader)
	if err != nil {
		return nil, err
	}

	if pageRequested {
		pageNumber, pageSize, err := base.ReadPage(queryReader)
		if err != nil {
			return nil, err
		}

		result.Paginated = true
		result.PageNumber = pageNumber
		result.PageSize = pageSize
	}

	return result, nil
}

func isPageRequested(queryReader *api.QueryReader) (bool, error) {
	numberStr, err := queryReader.ReadString("page[number]")
	if err != nil {
		return false, errors.WithMessage(err, "failed to read page[number]")
	}

	sizeStr, err := queryReader.ReadString("page[size]")
	if err != nil {
		return false, errors.WithMessage(err, "failed to read page[size]")
	}

	return numberStr != "" || sizeStr != "", nil
}

// buildFilter returns nil when nothing was requested, so the handler keeps
// using the cheaper FindAll path.
func buildFilter(in *input) *filters.FindUser {
	if len(in.IDs) == 0 && len(in.Logins) == 0 && len(in.Emails) == 0 {
		return nil
	}

	return &filters.FindUser{
		IDs:    in.IDs,
		Logins: in.Logins,
		Emails: in.Emails,
	}
}

func buildPagination(in *input) *filters.Pagination {
	if !in.Paginated {
		return nil
	}

	offset := uint64((in.PageNumber - 1) * in.PageSize) //nolint:gosec // validated by base.ReadPage

	return &filters.Pagination{
		Limit:  uint64(in.PageSize), //nolint:gosec // validated by base.ReadPage
		Offset: offset,
	}
}
