package taskdispatcher

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturedTaskEvent struct {
	eventType pluginproto.EventType
	taskID    uint
	nodeID    uint
	serverID  *uint
	taskType  string
	status    string
}

type fakePluginEvents struct {
	events []capturedTaskEvent
}

func (f *fakePluginEvents) DispatchTaskEventAsync(
	_ context.Context,
	eventType pluginproto.EventType,
	taskID, nodeID uint,
	serverID *uint,
	taskType, status string,
	_ map[string]string,
) {
	f.events = append(f.events, capturedTaskEvent{
		eventType: eventType,
		taskID:    taskID,
		nodeID:    nodeID,
		serverID:  serverID,
		taskType:  taskType,
		status:    status,
	})
}

func TestDispatch_EmitsPluginTaskCreatedEvent(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	pluginEvents := &fakePluginEvents{}
	h.dispatcher.SetPluginEventDispatcher(pluginEvents)

	serverID := uint(9)
	task := &domain.DaemonTask{
		DedicatedServerID: 7,
		ServerID:          &serverID,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	// ACT
	err := h.dispatcher.Dispatch(context.Background(), task)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, pluginEvents.events, 1)
	event := pluginEvents.events[0]
	assert.Equal(t, pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED, event.eventType)
	assert.Equal(t, task.ID, event.taskID)
	assert.Equal(t, uint(7), event.nodeID)
	require.NotNil(t, event.serverID)
	assert.Equal(t, serverID, *event.serverID)
	assert.Equal(t, string(domain.DaemonTaskTypeServerStart), event.taskType)
}
