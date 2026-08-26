package domain

import "time"

// PluginSecret is one credential a plugin stored through the gameap-secrets
// host module. Value holds the "enc:"-prefixed ciphertext produced by
// pkg/secret and bound to (PluginID, Key); it is decrypted only when the
// owning plugin reads it back.
type PluginSecret struct {
	ID        uint64     `db:"id"`
	PluginID  Uint64ID   `db:"plugin_id"`
	Key       string     `db:"key"`
	Value     string     `db:"value"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}
