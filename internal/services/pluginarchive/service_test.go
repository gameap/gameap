package pluginarchive

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPluginDBID  uint64 = 5
	testManagerID          = "fake-plugin"
	testOperationID        = "op-1"
	testNodeID      uint64 = 3
)

type fakeProvider struct {
	mu      sync.Mutex
	plugins map[string]*pkgplugin.LoadedPlugin
	calls   int
}

func (p *fakeProvider) GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++

	lp, ok := p.plugins[pluginID]

	return lp, ok
}

func (p *fakeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

type fakeResolver struct {
	ids map[domain.Uint64ID]string
}

func (r *fakeResolver) GetPluginManagerID(dbID domain.Uint64ID) (string, bool) {
	id, ok := r.ids[dbID]

	return id, ok
}

// fakePluginInstance implements proto.PluginService plus the archive events
// capability, the way the real wrapper does.
type fakePluginInstance struct {
	sdk.EmptyPluginService

	mu           sync.Mutex
	progress     []*nodefs.HandleArchiveProgressRequest
	completed    []*nodefs.HandleArchiveCompletedRequest
	progressFunc func(ctx context.Context, req *nodefs.HandleArchiveProgressRequest) error
	completeFunc func(ctx context.Context, req *nodefs.HandleArchiveCompletedRequest) error
	hasHandler   bool
}

func (f *fakePluginInstance) GetInfo(context.Context, *proto.GetInfoRequest) (*proto.PluginInfo, error) {
	return &proto.PluginInfo{Id: testManagerID}, nil
}

func (f *fakePluginInstance) HandleArchiveProgress(
	ctx context.Context,
	req *nodefs.HandleArchiveProgressRequest,
) (*nodefs.HandleArchiveProgressResponse, error) {
	f.mu.Lock()
	f.progress = append(f.progress, req)
	handle := f.progressFunc
	f.mu.Unlock()

	if handle != nil {
		if err := handle(ctx, req); err != nil {
			return nil, err
		}
	}

	return &nodefs.HandleArchiveProgressResponse{}, nil
}

func (f *fakePluginInstance) HandleArchiveCompleted(
	ctx context.Context,
	req *nodefs.HandleArchiveCompletedRequest,
) (*nodefs.HandleArchiveCompletedResponse, error) {
	f.mu.Lock()
	f.completed = append(f.completed, req)
	handle := f.completeFunc
	f.mu.Unlock()

	if handle != nil {
		if err := handle(ctx, req); err != nil {
			return nil, err
		}
	}

	return &nodefs.HandleArchiveCompletedResponse{}, nil
}

func (f *fakePluginInstance) HasArchiveEventsHandler() bool { return f.hasHandler }

func (f *fakePluginInstance) recordedProgress() []*nodefs.HandleArchiveProgressRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]*nodefs.HandleArchiveProgressRequest(nil), f.progress...)
}

func (f *fakePluginInstance) recordedCompleted() []*nodefs.HandleArchiveCompletedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]*nodefs.HandleArchiveCompletedRequest(nil), f.completed...)
}

type serviceEnv struct {
	service  *Service
	pubsub   pubsub.PubSub
	provider *fakeProvider
	instance *fakePluginInstance
	plugin   *pkgplugin.LoadedPlugin
}

func newServiceEnv(t *testing.T, mutate func(*Options)) *serviceEnv {
	t.Helper()

	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	instance := &fakePluginInstance{hasHandler: true}
	plugin := &pkgplugin.LoadedPlugin{
		Info:     &proto.PluginInfo{Id: testManagerID},
		Enabled:  true,
		Instance: instance,
	}
	provider := &fakeProvider{plugins: map[string]*pkgplugin.LoadedPlugin{testManagerID: plugin}}
	resolver := &fakeResolver{ids: map[domain.Uint64ID]string{domain.Uint64ID(testPluginDBID): testManagerID}}

	opts := Options{}
	if mutate != nil {
		mutate(&opts)
	}

	svc := New(provider, resolver, ps, opts, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, svc.Start(ctx))

	return &serviceEnv{
		service:  svc,
		pubsub:   ps,
		provider: provider,
		instance: instance,
		plugin:   plugin,
	}
}

func publishProgress(t *testing.T, ps pubsub.PubSub, payload messages.ArchiveProgressEventPayload) {
	t.Helper()

	channel := channels.BuildRealtimeArchiveOpChannel(payload.OperationID)
	msg, err := messages.NewMessage(channel, messages.TypeArchiveProgress, payload)
	require.NoError(t, err)
	require.NoError(t, ps.Publish(context.Background(), channel, msg))
}

func publishComplete(t *testing.T, ps pubsub.PubSub, payload messages.ArchiveCompleteEventPayload) {
	t.Helper()

	channel := channels.BuildRealtimeArchiveOpChannel(payload.OperationID)
	msg, err := messages.NewMessage(channel, messages.TypeArchiveComplete, payload)
	require.NoError(t, err)
	require.NoError(t, ps.Publish(context.Background(), channel, msg))
}

func TestService_Register_NeverResolvesPlugins(t *testing.T) {
	t.Parallel()

	// ARRANGE: Register runs from host functions, possibly under the plugin
	// manager write lock — resolving the plugin there would self-deadlock.
	env := newServiceEnv(t, nil)

	// ACT
	env.service.Register(testPluginDBID, testOperationID, testNodeID, true)

	// ASSERT
	assert.Equal(t, 0, env.provider.callCount(),
		"plugin resolution must happen at delivery time, never at registration")
}

func TestService_DeliversProgressAndCompletion(t *testing.T) {
	t.Parallel()

	// ARRANGE
	env := newServiceEnv(t, nil)
	env.service.Register(testPluginDBID, testOperationID, testNodeID, true)

	// ACT
	publishProgress(t, env.pubsub, messages.ArchiveProgressEventPayload{
		OperationID:    testOperationID,
		NodeID:         testNodeID,
		FilesProcessed: 2,
		FilesTotal:     5,
		CurrentEntry:   "maps/x.bsp",
	})

	require.Eventually(t, func() bool {
		return len(env.instance.recordedProgress()) == 1
	}, 2*time.Second, 10*time.Millisecond, "progress must reach the guest")

	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID:    testOperationID,
		NodeID:         testNodeID,
		Success:        true,
		FilesProcessed: 5,
		Format:         "zip",
	})

	// ASSERT
	require.Eventually(t, func() bool {
		return len(env.instance.recordedCompleted()) == 1
	}, 2*time.Second, 10*time.Millisecond, "completion must reach the guest")

	progress := env.instance.recordedProgress()[0]
	assert.Equal(t, testOperationID, progress.OperationId)
	assert.Equal(t, testNodeID, progress.NodeId)
	assert.Equal(t, uint32(2), progress.FilesProcessed)
	assert.Equal(t, "maps/x.bsp", progress.CurrentEntry)

	completed := env.instance.recordedCompleted()[0]
	assert.True(t, completed.Success)
	assert.Equal(t, uint32(5), completed.FilesProcessed)
	assert.Equal(t, nodefs.ArchiveFormat_ARCHIVE_FORMAT_ZIP, completed.Format)
}

func TestService_ProgressCoalescesToLatest(t *testing.T) {
	t.Parallel()

	// ARRANGE: a slow guest must see fewer, fresher progress updates.
	release := make(chan struct{})
	env := newServiceEnv(t, nil)
	env.instance.progressFunc = func(context.Context, *nodefs.HandleArchiveProgressRequest) error {
		<-release

		return nil
	}
	env.service.Register(testPluginDBID, testOperationID, testNodeID, true)

	// ACT: the first event occupies the guest before the burst goes out, so
	// events 2..5 provably pile up behind a busy handler and coalesce.
	publishProgress(t, env.pubsub, messages.ArchiveProgressEventPayload{
		OperationID:    testOperationID,
		FilesProcessed: 1,
	})
	require.Eventually(t, func() bool {
		return len(env.instance.recordedProgress()) == 1
	}, 2*time.Second, 10*time.Millisecond, "the first event must reach the guest")

	for i := uint32(2); i <= 5; i++ {
		publishProgress(t, env.pubsub, messages.ArchiveProgressEventPayload{
			OperationID:    testOperationID,
			FilesProcessed: i,
		})
	}
	close(release)

	// The coalesced survivor must be delivered before the completion is
	// published, otherwise the loop may pick the completion first.
	require.Eventually(t, func() bool {
		progress := env.instance.recordedProgress()

		return progress[len(progress)-1].FilesProcessed == 5
	}, 2*time.Second, 10*time.Millisecond, "the latest progress must win")

	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
	})

	require.Eventually(t, func() bool {
		return len(env.instance.recordedCompleted()) == 1
	}, 2*time.Second, 10*time.Millisecond)

	// ASSERT
	progress := env.instance.recordedProgress()
	assert.LessOrEqual(t, len(progress), 3, "intermediate progress must be dropped, not queued")
}

func TestService_CompletionRetriesWhenBusy(t *testing.T) {
	t.Parallel()

	// ARRANGE
	var attempts int
	var mu sync.Mutex
	env := newServiceEnv(t, func(o *Options) {
		o.BusyRetryDelay = 10 * time.Millisecond
		o.BusyRetries = 3
	})
	env.instance.completeFunc = func(context.Context, *nodefs.HandleArchiveCompletedRequest) error {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts < 3 {
			return errors.Wrap(pkgplugin.ErrPluginBusy, "gate occupied")
		}

		return nil
	}
	env.service.Register(testPluginDBID, testOperationID, testNodeID, false)

	// ACT
	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
	})

	// ASSERT
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return attempts == 3
	}, 2*time.Second, 10*time.Millisecond, "busy completions must be retried")
}

func TestService_ProgressSkippedWithoutOptIn(t *testing.T) {
	t.Parallel()

	// ARRANGE
	env := newServiceEnv(t, nil)
	env.service.Register(testPluginDBID, testOperationID, testNodeID, false)

	// ACT
	publishProgress(t, env.pubsub, messages.ArchiveProgressEventPayload{
		OperationID:    testOperationID,
		FilesProcessed: 1,
	})
	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
	})

	// ASSERT
	require.Eventually(t, func() bool {
		return len(env.instance.recordedCompleted()) == 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Empty(t, env.instance.recordedProgress(),
		"progress must not be delivered without report_progress")
}

func TestService_HandlerAbsentPluginStaysUntouched(t *testing.T) {
	t.Parallel()

	// ARRANGE: a plugin without the export polls GetArchiveOperation instead.
	env := newServiceEnv(t, nil)
	env.instance.hasHandler = false
	env.service.Register(testPluginDBID, testOperationID, testNodeID, true)

	// ACT
	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
	})

	// ASSERT: the delivery goroutine drains and exits without touching the
	// guest.
	require.Eventually(t, func() bool {
		return env.provider.callCount() >= 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Empty(t, env.instance.recordedCompleted())
	assert.True(t, env.plugin.IsEnabled(), "an absent handler must never disable the plugin")
}

func TestService_UnknownOperationEventsIgnored(t *testing.T) {
	t.Parallel()

	// ARRANGE
	env := newServiceEnv(t, nil)
	env.service.Register(testPluginDBID, testOperationID, testNodeID, true)

	// ACT
	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: "some-other-op",
		Success:     true,
	})

	// ASSERT
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, env.instance.recordedCompleted(),
		"events of unregistered operations must be ignored")
}

func TestService_RemovePluginStopsDelivery(t *testing.T) {
	t.Parallel()

	// ARRANGE
	env := newServiceEnv(t, nil)
	env.service.Register(testPluginDBID, testOperationID, testNodeID, true)

	// ACT
	env.service.RemovePlugin(testPluginDBID)
	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
	})

	// ASSERT
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, env.instance.recordedCompleted(),
		"deliveries must stop after the plugin is removed")
}

func TestService_NotifyCompletedReplaysMissedCompletion(t *testing.T) {
	t.Parallel()

	// ARRANGE: the completion event of a fast operation was published (and
	// dropped) before Register; the host library replays it from the
	// operation snapshot.
	env := newServiceEnv(t, nil)
	env.service.Register(testPluginDBID, testOperationID, testNodeID, false)

	// ACT
	env.service.NotifyCompleted(testOperationID, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
		Format:      "zip",
	})

	// ASSERT
	require.Eventually(t, func() bool {
		return len(env.instance.recordedCompleted()) == 1
	}, 2*time.Second, 10*time.Millisecond, "the replayed completion must reach the guest")
	assert.True(t, env.instance.recordedCompleted()[0].Success)
}

func TestService_StopWaitsForInFlightDelivery(t *testing.T) {
	t.Parallel()

	// ARRANGE: a completion callback is executing inside the guest when
	// Stop is called; Stop must wait for it instead of racing the runtime
	// teardown.
	entered := make(chan struct{})
	release := make(chan struct{})
	env := newServiceEnv(t, nil)
	env.instance.completeFunc = func(context.Context, *nodefs.HandleArchiveCompletedRequest) error {
		close(entered)
		<-release

		return nil
	}
	env.service.Register(testPluginDBID, testOperationID, testNodeID, false)

	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
	})
	<-entered

	stopDone := make(chan struct{})
	go func() {
		env.service.Stop()
		close(stopDone)
	}()

	// ASSERT: Stop blocks while the guest call runs...
	select {
	case <-stopDone:
		t.Fatal("Stop must wait for the in-flight delivery")
	case <-time.After(50 * time.Millisecond):
	}

	// ...and returns once it finishes.
	close(release)
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop must return after the delivery finishes")
	}

	// New registrations are rejected after Stop.
	env.service.Register(testPluginDBID, "op-after-stop", testNodeID, false)
	env.service.NotifyCompleted("op-after-stop", messages.ArchiveCompleteEventPayload{
		OperationID: "op-after-stop",
		Success:     true,
	})
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, env.instance.recordedCompleted(), 1,
		"registrations after Stop must not deliver")
}

func TestService_GuestDeadlineDisablesPlugin(t *testing.T) {
	t.Parallel()

	// ARRANGE: a deadline hit inside the guest closes the wasm module, so
	// the plugin must be disabled until reload (scheduler policy).
	env := newServiceEnv(t, func(o *Options) {
		o.CompletionCallTimeout = 20 * time.Millisecond
	})
	env.instance.completeFunc = func(ctx context.Context, _ *nodefs.HandleArchiveCompletedRequest) error {
		<-ctx.Done()

		return ctx.Err()
	}
	env.service.Register(testPluginDBID, testOperationID, testNodeID, false)

	// ACT
	publishComplete(t, env.pubsub, messages.ArchiveCompleteEventPayload{
		OperationID: testOperationID,
		Success:     true,
	})

	// ASSERT
	require.Eventually(t, func() bool {
		return !env.plugin.IsEnabled()
	}, 2*time.Second, 10*time.Millisecond, "a timed-out guest call must disable the plugin")
}
