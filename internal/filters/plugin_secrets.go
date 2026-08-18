package filters

import "github.com/gameap/gameap/internal/domain"

type FindPluginSecret struct {
	PluginIDs []domain.Uint64ID
	Keys      []string
}
