package domain

import "time"

type PluginStorageEntry struct {
	ID         uint64     `db:"id"`
	PluginID   uint64     `db:"plugin_id"`
	Key        string     `db:"key"`
	EntityType *string    `db:"entity_type"`
	EntityID   *uint      `db:"entity_id"`
	Payload    []byte     `db:"payload"`
	CreatedAt  *time.Time `db:"created_at"`
	UpdatedAt  *time.Time `db:"updated_at"`
}

type PluginStorageEntityPair struct {
	EntityType *string
	EntityID   *uint
}

// PluginStorageUsage is what one plugin keeps in gameap-storage; it backs
// the per-plugin quotas without reading payloads.
type PluginStorageUsage struct {
	Keys  int
	Bytes uint64
}
