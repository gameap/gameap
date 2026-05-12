package filters

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type FindServerTaskExecution struct {
	IDs           []uint
	ExecutionIDs  []string
	ServerTaskIDs []uint
	ServerIDs     []uint
	NodeIDs       []uint
	Statuses      []domain.ServerTaskExecutionStatus
	NotStatuses   []domain.ServerTaskExecutionStatus
	StartedAfter  *time.Time
	StartedBefore *time.Time
}
