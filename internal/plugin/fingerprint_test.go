package plugin_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/stretchr/testify/assert"
)

func TestFingerprint(t *testing.T) {
	t.Parallel()

	base := func() *domain.Plugin {
		return &domain.Plugin{
			ID:         1,
			Name:       "test-plugin",
			Version:    "1.0.0",
			Filename:   new("1.wasm"),
			Checksum:   new("916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"),
			Status:     domain.PluginStatusActive,
			Priority:   10,
			Generation: 2,
			Config:     map[string]any{"api_key": "k", "limit": float64(5)},
		}
	}

	tests := []struct {
		name       string
		mutate     func(*domain.Plugin)
		wantDiffer bool
	}{
		{name: "unchanged_row_hashes_the_same", mutate: func(_ *domain.Plugin) {}},
		{name: "version_change_differs", mutate: func(p *domain.Plugin) { p.Version = "2.0.0" }, wantDiffer: true},
		{name: "filename_change_differs", mutate: func(p *domain.Plugin) { p.Filename = new("2.wasm") }, wantDiffer: true},
		{name: "checksum_change_differs", mutate: func(p *domain.Plugin) { p.Checksum = new("deadbeef") }, wantDiffer: true},
		{name: "checksum_cleared_differs", mutate: func(p *domain.Plugin) { p.Checksum = nil }, wantDiffer: true},
		{name: "config_value_change_differs", mutate: func(p *domain.Plugin) { p.Config["limit"] = float64(6) }, wantDiffer: true},
		{name: "config_key_added_differs", mutate: func(p *domain.Plugin) { p.Config["extra"] = "x" }, wantDiffer: true},
		{name: "generation_change_differs", mutate: func(p *domain.Plugin) { p.Generation = 3 }, wantDiffer: true},
		{
			name: "allowed_permissions_change_hashes_the_same",
			mutate: func(p *domain.Plugin) {
				p.AllowedPermissions = []domain.PluginPermission{domain.PluginPermissionManageRBAC}
			},
		},
		{name: "priority_change_hashes_the_same", mutate: func(p *domain.Plugin) { p.Priority = 99 }},
		{name: "status_change_hashes_the_same", mutate: func(p *domain.Plugin) { p.Status = domain.PluginStatusDisabled }},
		{name: "config_schema_change_hashes_the_same", mutate: func(p *domain.Plugin) { p.ConfigSchema = new(`{}`) }},
		{name: "last_loaded_at_change_hashes_the_same", mutate: func(p *domain.Plugin) { p.LastLoadedAt = new(time.Now()) }},
		{name: "updated_at_change_hashes_the_same", mutate: func(p *domain.Plugin) { p.UpdatedAt = new(time.Now()) }},
		{name: "name_change_hashes_the_same", mutate: func(p *domain.Plugin) { p.Name = "renamed" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := plugin.Fingerprint(base())

			mutated := base()
			tt.mutate(mutated)
			got := plugin.Fingerprint(mutated)

			if tt.wantDiffer {
				assert.NotEqual(t, original, got)

				return
			}

			assert.Equal(t, original, got)
		})
	}
}

func TestFingerprint_nil_and_empty_config_hash_the_same(t *testing.T) {
	t.Parallel()

	withNil := plugin.Fingerprint(&domain.Plugin{Version: "1"})
	withEmpty := plugin.Fingerprint(&domain.Plugin{Version: "1", Config: map[string]any{}})

	assert.Equal(t, withNil, withEmpty)
}

func TestFingerprint_field_boundaries_do_not_collide(t *testing.T) {
	t.Parallel()

	first := plugin.Fingerprint(&domain.Plugin{Version: "ab", Filename: new("c")})
	second := plugin.Fingerprint(&domain.Plugin{Version: "a", Filename: new("bc")})

	assert.NotEqual(t, first, second)
}

func TestFileChecksum(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		plugin.FileChecksum([]byte("abc")))
}

func TestResolveFilename(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "custom.wasm", plugin.ResolveFilename(&domain.Plugin{ID: 7, Filename: new("custom.wasm")}))
	assert.Equal(t, "7.wasm", plugin.ResolveFilename(&domain.Plugin{ID: 7, Filename: new("")}))
	assert.Equal(t, "7.wasm", plugin.ResolveFilename(&domain.Plugin{ID: 7}))
}
