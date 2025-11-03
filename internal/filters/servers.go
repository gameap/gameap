package filters

import "github.com/google/uuid"

type FindServer struct {
	IDs        []uint
	UUIDs      []uuid.UUID
	UserIDs    []uint
	Enabled    *bool
	Blocked    *bool
	GameIDs    []string
	DSIDs      []uint
	GameModIDs []uint
	Names      []string

	WithDeleted bool
}

func FindServerByIDs(ids ...uint) *FindServer {
	return &FindServer{
		IDs: ids,
	}
}

func FindServerByNodeIDs(nodeIDs ...uint) *FindServer {
	return &FindServer{
		DSIDs: nodeIDs,
	}
}

func FindServerByUUIDs(uuids []uuid.UUID) *FindServer {
	return &FindServer{
		UUIDs: uuids,
	}
}
