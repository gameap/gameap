package filters

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/rs/xid"
)

type FindServerTaskExecution struct {
	IDs           []uint
	ExecutionIDs  []xid.ID
	ServerTaskIDs []uint
	ServerIDs     []uint
	NodeIDs       []uint
	Statuses      []domain.ServerTaskExecutionStatus
	NotStatuses   []domain.ServerTaskExecutionStatus
	StartedAfter  *time.Time
	StartedBefore *time.Time
}
