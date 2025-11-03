package filters

import "time"

type FindServerTaskFail struct {
	IDs           []uint
	ServerTaskIDs []uint
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}

func FindServerTaskFailByServerTaskIDs(serverTaskIDs ...uint) *FindServerTaskFail {
	return &FindServerTaskFail{
		ServerTaskIDs: serverTaskIDs,
	}
}
