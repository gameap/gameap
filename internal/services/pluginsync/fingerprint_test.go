package pluginsync_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/pluginsync"
	"github.com/stretchr/testify/assert"
)

func TestFingerprint(t *testing.T) {
	base := func() *domain.Plugin {
		return &domain.Plugin{
			ID:       1,
			Name:     "test-plugin",
			Version:  "1.0.0",
			Filename: new("1.wasm"),
			Checksum: new("916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"),
			Status:   domain.PluginStatusActive,
			Priority: 10,
		}
	}

	tests := []struct {
		name       string
		mutate     func(*domain.Plugin)
		wantDiffer bool
	}{
		{
			name:       "unchanged_row_hashes_the_same",
			mutate:     func(_ *domain.Plugin) {},
			wantDiffer: false,
		},
		{
			name:       "version_change_differs",
			mutate:     func(p *domain.Plugin) { p.Version = "2.0.0" },
			wantDiffer: true,
		},
		{
			name:       "filename_change_differs",
			mutate:     func(p *domain.Plugin) { p.Filename = new("2.wasm") },
			wantDiffer: true,
		},
		{
			name:       "checksum_change_differs",
			mutate:     func(p *domain.Plugin) { p.Checksum = new("deadbeef") },
			wantDiffer: true,
		},
		{
			name:       "checksum_cleared_differs",
			mutate:     func(p *domain.Plugin) { p.Checksum = nil },
			wantDiffer: true,
		},
		{
			// Grants are re-read from the database on every host library call,
			// so changing them must not cost a restart.
			name: "allowed_permissions_change_hashes_the_same",
			mutate: func(p *domain.Plugin) {
				p.AllowedPermissions = []domain.PluginPermission{domain.PluginPermissionManageRBAC}
			},
			wantDiffer: false,
		},
		{
			// Priority only orders repository queries; nothing in the runtime
			// reads it.
			name:       "priority_change_hashes_the_same",
			mutate:     func(p *domain.Plugin) { p.Priority = 99 },
			wantDiffer: false,
		},
		{
			// Status drives load and unload, not a rebuild in place.
			name:       "status_change_hashes_the_same",
			mutate:     func(p *domain.Plugin) { p.Status = domain.PluginStatusDisabled },
			wantDiffer: false,
		},
		{
			name:       "last_loaded_at_change_hashes_the_same",
			mutate:     func(p *domain.Plugin) { p.LastLoadedAt = new(time.Now()) },
			wantDiffer: false,
		},
		{
			name:       "updated_at_change_hashes_the_same",
			mutate:     func(p *domain.Plugin) { p.UpdatedAt = new(time.Now()) },
			wantDiffer: false,
		},
		{
			name:       "name_change_hashes_the_same",
			mutate:     func(p *domain.Plugin) { p.Name = "renamed" },
			wantDiffer: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := pluginsync.Fingerprint(base())

			mutated := base()
			tt.mutate(mutated)
			got := pluginsync.Fingerprint(mutated)

			if tt.wantDiffer {
				assert.NotEqual(t, original, got)

				return
			}

			assert.Equal(t, original, got)
		})
	}
}

func TestFingerprint_field_boundaries_do_not_collide(t *testing.T) {
	// Without length prefixing these two rows would hash the same.
	first := pluginsync.Fingerprint(&domain.Plugin{Version: "ab", Filename: new("c")})
	second := pluginsync.Fingerprint(&domain.Plugin{Version: "a", Filename: new("bc")})

	assert.NotEqual(t, first, second)
}
