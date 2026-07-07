package getdaemontasks

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type input struct {
	IDs                []uint
	DedicatedServerIDs []uint
	ServerIDs          []uint
	Tasks              []domain.DaemonTaskType
	Statuses           []domain.DaemonTaskStatus
	Sort               string
	PageNumber         int
	PageSize           int
}

func readInput(r *http.Request) (*input, error) {
	queryReader := api.NewQueryReader(r)

	result := &input{}

	// Parse filter parameters following JSON API spec: filter[field]=value
	ids, err := queryReader.ReadUintList("filter[id]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[id] list")
	}
	result.IDs = append(result.IDs, ids...)

	dedicatedServerIDs, err := queryReader.ReadUintList("filter[dedicated_server_id]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[dedicated_server_id] list")
	}
	result.DedicatedServerIDs = append(result.DedicatedServerIDs, dedicatedServerIDs...)

	serverIDs, err := queryReader.ReadUintList("filter[server_id]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[server_id] list")
	}
	result.ServerIDs = append(result.ServerIDs, serverIDs...)

	taskStrings, err := queryReader.ReadList("filter[task]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[task] list")
	}
	for _, taskStr := range taskStrings {
		result.Tasks = append(result.Tasks, domain.DaemonTaskType(taskStr))
	}

	statusStrings, err := queryReader.ReadList("filter[status]")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read filter[status] list")
	}
	for _, statusStr := range statusStrings {
		result.Statuses = append(result.Statuses, domain.DaemonTaskStatus(statusStr))
	}

	// Parse sort parameter: sort=field or sort=-field
	sortStr, err := queryReader.ReadString("sort")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read sort")
	}
	result.Sort = sortStr

	// Parse pagination following JSON API spec: page[number] and page[size],
	// with page[size] clamped to base.MaxPageSize.
	pageNumber, pageSize, err := base.ReadPage(queryReader)
	if err != nil {
		return nil, err
	}
	result.PageNumber = pageNumber
	result.PageSize = pageSize

	return result, nil
}
