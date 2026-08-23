package domain_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestPermissionSatisfied(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		permission domain.PluginPermission
		granted    []domain.PluginPermission
		want       bool
	}{
		{name: "exact_grant", permission: domain.PluginPermissionFilesRead,
			granted: []domain.PluginPermission{domain.PluginPermissionFilesRead}, want: true},
		{name: "files_includes_files_read", permission: domain.PluginPermissionFilesRead,
			granted: []domain.PluginPermission{domain.PluginPermissionFiles}, want: true},
		{name: "files_read_does_not_include_files", permission: domain.PluginPermissionFiles,
			granted: []domain.PluginPermission{domain.PluginPermissionFilesRead}, want: false},
		{name: "unrelated_grant", permission: domain.PluginPermissionSecrets,
			granted: []domain.PluginPermission{domain.PluginPermissionFiles}, want: false},
		{name: "nothing_granted", permission: domain.PluginPermissionFilesRead, granted: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, domain.PermissionSatisfied(tt.permission, tt.granted))
		})
	}
}

func TestPlugin_HasPermission_honours_supersets(t *testing.T) {
	t.Parallel()

	plugin := &domain.Plugin{AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionFiles}}

	assert.True(t, plugin.HasPermission(domain.PluginPermissionFiles))
	assert.True(t, plugin.HasPermission(domain.PluginPermissionFilesRead))
	assert.False(t, plugin.HasPermission(domain.PluginPermissionNodeCommands))
}

func TestPluginPermissionSupersets(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []domain.PluginPermission{domain.PluginPermissionFiles},
		domain.PluginPermissionSupersets(domain.PluginPermissionFilesRead))
	assert.Nil(t, domain.PluginPermissionSupersets(domain.PluginPermissionFiles))
}

func TestParsePluginPermissions_accepts_files_read(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]domain.PluginPermission{domain.PluginPermissionFilesRead},
		domain.ParsePluginPermissions([]string{"files_read", "made_up"}))
}

func TestPlugin_LoadState(t *testing.T) {
	t.Parallel()

	plugin := &domain.Plugin{Generation: 4, ConfigSchema: new(`{"type":"object"}`)}
	plugin.MarkError("boom", time.Now())

	state := plugin.LoadState()
	assert.Equal(t, domain.PluginStatusError, state.Status)
	assert.Equal(t, "boom", *state.LastError)
	assert.Equal(t, 4, state.Generation)
	assert.Equal(t, `{"type":"object"}`, *state.ConfigSchema)
	assert.True(t, plugin.HasConfigSchema())
}
