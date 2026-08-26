package filters

import "github.com/gameap/gameap/internal/domain"

type FindPluginStorage struct {
	IDs         []uint64
	PluginIDs   []uint64
	Keys        []string
	EntityPairs []domain.PluginStorageEntityPair
	// KeyPrefix keeps only entries whose key starts with the prefix
	// (case-sensitive where the column collation is).
	KeyPrefix *string
}
