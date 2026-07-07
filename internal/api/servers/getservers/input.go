package getservers

import (
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

//nolint:gochecknoglobals
var allowedSortFields = map[string]string{
	"id":          "id",
	"name":        "name",
	"server_ip":   "server_ip",
	"server_port": "server_port",
	"game_id":     "game_id",
	"ds_id":       "ds_id",
}

type input struct {
	DSIDs      []uint
	GameIDs    []string
	Enabled    *bool
	Sort       string
	PageNumber int
	PageSize   int
}

func readInput(r *http.Request) (*input, error) {
	queryReader := api.NewQueryReader(r)

	result := &input{}

	dsIDs, err := queryReader.ReadUintList("filter[ds_id]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[ds_id] list")
	}
	result.DSIDs = dsIDs

	gameIDs, err := queryReader.ReadList("filter[game_id]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[game_id] list")
	}
	result.GameIDs = gameIDs

	enabledStr, err := queryReader.ReadString("filter[enabled]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[enabled]")
	}
	if enabledStr != "" {
		enabled, err := strconv.ParseBool(enabledStr)
		if err != nil {
			return nil, errors.WithMessage(err, "invalid filter[enabled] value")
		}
		result.Enabled = &enabled
	}

	sortStr, err := queryReader.ReadString("sort")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read sort")
	}
	result.Sort = sortStr

	pageNumber, pageSize, err := base.ReadPage(queryReader)
	if err != nil {
		return nil, err
	}
	result.PageNumber = pageNumber
	result.PageSize = pageSize

	return result, nil
}

func buildFilter(input *input, userID uint, isAdmin bool) *filters.FindServer {
	filter := &filters.FindServer{}

	if !isAdmin {
		filter.UserIDs = []uint{userID}
	}

	if len(input.DSIDs) > 0 {
		filter.DSIDs = input.DSIDs
	}

	if len(input.GameIDs) > 0 {
		filter.GameIDs = input.GameIDs
	}

	if input.Enabled != nil {
		filter.Enabled = input.Enabled
	}

	return filter
}

func buildSorting(input *input) []filters.Sorting {
	defaultSorting := []filters.Sorting{
		{Field: "id", Direction: filters.SortDirectionAsc},
	}

	sort, err := filters.ParseUserSort(input.Sort, allowedSortFields)
	if err != nil || sort == nil {
		return defaultSorting
	}

	return []filters.Sorting{*sort}
}

func buildPagination(input *input) *filters.Pagination {
	offset := uint64((input.PageNumber - 1) * input.PageSize) //nolint:gosec // PageNumber and PageSize are validated

	return &filters.Pagination{
		Limit:  uint64(input.PageSize), //nolint:gosec // PageSize is validated
		Offset: offset,
	}
}
