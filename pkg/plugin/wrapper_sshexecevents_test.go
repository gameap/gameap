package plugin

import (
	"context"
	"testing"

	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPluginServiceWrapper_HandleExecCompleted_NilFunctionReturnsError: unlike
// the optional load-time queries, a missing export here is an error — silently
// succeeding would swallow the result of a command the plugin asked about.
func TestPluginServiceWrapper_HandleExecCompleted_NilFunctionReturnsError(t *testing.T) {
	// ARRANGE
	w := &pluginServiceWrapper{handleexeccompleted: nil}

	// ACT
	resp, err := w.HandleExecCompleted(context.Background(), &sshsdk.HandleExecCompletedRequest{
		OperationId: "op-1",
	})

	// ASSERT
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrExportNotFound))
	assert.Contains(t, err.Error(), "ssh_exec_events_handler_handle_exec_completed")
	assert.Nil(t, resp)
}

func TestPluginServiceWrapper_HasSSHExecEventsHandler(t *testing.T) {
	tests := []struct {
		name    string
		wrapper *pluginServiceWrapper
		want    bool
	}{
		{
			name:    "export_absent",
			wrapper: &pluginServiceWrapper{},
			want:    false,
		},
		{
			name:    "export_present",
			wrapper: &pluginServiceWrapper{handleexeccompleted: unusedExport{}},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.wrapper.HasSSHExecEventsHandler())
		})
	}
}

// sshCapablePluginService mimics a wrapper that does expose the capability.
type sshCapablePluginService struct {
	mockPluginService

	hasHandler bool
}

func (s *sshCapablePluginService) HasSSHExecEventsHandler() bool { return s.hasHandler }

func TestLoadedPlugin_HasSSHExecEventsHandler(t *testing.T) {
	tests := []struct {
		name   string
		plugin *LoadedPlugin
		want   bool
	}{
		{
			name:   "instance_without_the_capability",
			plugin: &LoadedPlugin{Instance: &mockPluginService{}},
			want:   false,
		},
		{
			name:   "instance_reporting_no_handler",
			plugin: &LoadedPlugin{Instance: &sshCapablePluginService{hasHandler: false}},
			want:   false,
		},
		{
			name:   "instance_reporting_a_handler",
			plugin: &LoadedPlugin{Instance: &sshCapablePluginService{hasHandler: true}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.plugin.HasSSHExecEventsHandler())
		})
	}
}

// TestSharedFixtureHasNoSSHExecEventsHandler pins backward compatibility: the
// example plugin predates the ssh module, and it must still load with the
// capability simply reported as absent.
func TestSharedFixtureHasNoSSHExecEventsHandler(t *testing.T) {
	plugin := loadSharedServerLoggerWASM(t)

	assert.False(t, plugin.HasSSHExecEventsHandler())
}
