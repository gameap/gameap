package filters

import "github.com/gameap/gameap/internal/domain"

type FindServerTask struct {
	IDs        []uint
	ServersIDs []uint
	NodeIDs    []uint
	Commands   []domain.ServerTaskCommand
}
