package plugin

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const observedPluginDBID uint64 = 42

// newObservedRuntime builds a runtime whose host modules report to observer.
// Creation goes through newWazeroRuntime because wazero's version cache is
// unsynchronised (see runtimeconfig.go).
func newObservedRuntime(t *testing.T, observer Observer) wazero.Runtime {
	t.Helper()

	ctx := context.Background()
	runtime := newWazeroRuntime(ctx, wazero.NewRuntimeConfig())
	t.Cleanup(func() { _ = runtime.Close(ctx) })

	return interceptHostCalls(runtime, observer, observedPluginDBID)
}

// TestInterceptHostCalls_decorates_for_every_observer: the deadline margin
// needs the seam whether or not there is something to report to, so even a
// nil observer gets the intercepting runtime.
func TestInterceptHostCalls_decorates_for_every_observer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		observer Observer
	}{
		{name: "nil_observer"},
		{name: "nop_observer", observer: NopObserver{}},
		{name: "reporting_observer", observer: &observerRecorder{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := context.Background()
			runtime := newWazeroRuntime(ctx, wazero.NewRuntimeConfig())
			t.Cleanup(func() { _ = runtime.Close(ctx) })

			// ACT
			got := interceptHostCalls(runtime, tt.observer, observedPluginDBID)

			// ASSERT
			assert.NotSame(t, runtime, got, "host calls must be routed through the interceptor")
		})
	}
}

func TestHostCallContext(t *testing.T) {
	t.Parallel()

	t.Run("keeps_the_margin_back_from_the_guest_deadline", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		deadline := time.Now().Add(10 * time.Second)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		// ACT
		got, cancelGot := hostCallContext(ctx)
		defer cancelGot()

		// ASSERT
		gotDeadline, ok := got.Deadline()
		require.True(t, ok)
		assert.Equal(t, deadline.Add(-hostCallDeadlineMargin), gotDeadline)
	})

	t.Run("passes_a_context_without_deadline_through", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()

		// ACT
		got, cancelGot := hostCallContext(ctx)
		defer cancelGot()

		// ASSERT
		_, ok := got.Deadline()
		assert.False(t, ok)
		assert.Equal(t, ctx, got)
	})

	t.Run("expires_at_once_when_less_than_the_margin_is_left", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx, cancel := context.WithTimeout(context.Background(), hostCallDeadlineMargin/2)
		defer cancel()

		// ACT
		got, cancelGot := hostCallContext(ctx)
		defer cancelGot()

		// ASSERT
		require.ErrorIs(t, got.Err(), context.DeadlineExceeded,
			"a host call that cannot finish in time must not start")
	})
}

// TestObservedFunctionBuilder_options_keep_the_chain guards the decorator's
// fluent contract: every option has to hand back the observed builder, or the
// remaining calls in a host library's chain would bypass the interceptor.
func TestObservedFunctionBuilder_options_keep_the_chain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		option func(wazero.HostFunctionBuilder) wazero.HostFunctionBuilder
	}{
		{
			name: "WithGoModuleFunction",
			option: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder {
				return b.WithGoModuleFunction(
					api.GoModuleFunc(func(context.Context, api.Module, []uint64) {}), nil, nil)
			},
		},
		{
			name: "WithGoFunction",
			option: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder {
				return b.WithGoFunction(api.GoFunc(func(context.Context, []uint64) {}), nil, nil)
			},
		},
		{
			name:   "WithFunc",
			option: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder { return b.WithFunc(func() {}) },
		},
		{
			name:   "WithName",
			option: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder { return b.WithName("env_fn") },
		},
		{
			name: "WithParameterNames",
			option: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder {
				return b.WithParameterNames("a", "b")
			},
		},
		{
			name: "WithResultNames",
			option: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder {
				return b.WithResultNames("out")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			// A builder is single-goroutine by contract, so every subtest owns one.
			runtime := newObservedRuntime(t, &observerRecorder{})
			builder := runtime.NewHostModuleBuilder("env").NewFunctionBuilder()

			// ACT
			got := tt.option(builder)

			// ASSERT
			assert.Same(t, builder, got, "the option must return the observed builder, not the wazero one")
		})
	}
}

// TestObservedFunctionBuilder_Export asserts that every registration flavour
// still lands in the underlying wazero module with its signature and names
// intact — the interceptor defers the real registration to Export, so a
// dropped option would silently change the module a host library exports.
// wazero forbids calling an exported host function from Go, so the guest-side
// timing itself is covered by the WASM-backed tests in hostimports_test.go.
func TestObservedFunctionBuilder_Export(t *testing.T) {
	t.Parallel()

	doubleParams := []api.ValueType{api.ValueTypeI32}
	doubleResults := []api.ValueType{api.ValueTypeI64}

	tests := []struct {
		name string
		// register wires one host function under the name "double".
		register        func(wazero.HostFunctionBuilder) wazero.HostFunctionBuilder
		wantParams      []api.ValueType
		wantResults     []api.ValueType
		wantParamNames  []string
		wantResultNames []string
	}{
		{
			name: "go_module_function_keeps_its_signature",
			register: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder {
				return b.WithGoModuleFunction(
					api.GoModuleFunc(func(context.Context, api.Module, []uint64) {}),
					doubleParams, doubleResults,
				)
			},
			wantParams:  doubleParams,
			wantResults: doubleResults,
		},
		{
			name: "go_function_keeps_its_signature_and_parameter_names",
			register: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder {
				return b.WithGoFunction(
					api.GoFunc(func(context.Context, []uint64) {}),
					doubleParams, doubleResults,
				).WithName("double_impl").WithParameterNames("value").WithResultNames("doubled")
			},
			wantParams:      doubleParams,
			wantResults:     doubleResults,
			wantParamNames:  []string{"value"},
			wantResultNames: []string{"doubled"},
		},
		{
			name: "reflective_function_signature_is_derived_from_the_go_types",
			register: func(b wazero.HostFunctionBuilder) wazero.HostFunctionBuilder {
				return b.WithFunc(func(_ context.Context, _ api.Module, value uint32) uint64 {
					return uint64(value) * 2
				}).WithParameterNames("value")
			},
			wantParams:     doubleParams,
			wantResults:    doubleResults,
			wantParamNames: []string{"value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := context.Background()
			observer := &observerRecorder{}
			runtime := newObservedRuntime(t, observer)
			builder := runtime.NewHostModuleBuilder("env")

			// ACT
			module, err := tt.register(builder.NewFunctionBuilder()).Export("double").Instantiate(ctx)

			// ASSERT
			require.NoError(t, err, "the module must still instantiate through the interceptor")

			definitions := module.ExportedFunctionDefinitions()
			definition, ok := definitions["double"]
			require.True(t, ok, "Export must reach the underlying wazero builder")

			assert.Equal(t, "env", definition.ModuleName())
			assert.Equal(t, tt.wantParams, definition.ParamTypes(), "parameter types")
			assert.Equal(t, tt.wantResults, definition.ResultTypes(), "result types")
			assert.Equal(t, tt.wantParamNames, definition.ParamNames(), "parameter names")
			assert.Equal(t, tt.wantResultNames, definition.ResultNames(), "result names")

			_, host, _ := observer.snapshot()
			assert.Empty(t, host, "registration alone reports nothing; only a guest call does")
		})
	}
}

func TestNopObserver_ignores_every_signal(t *testing.T) {
	t.Parallel()

	// ARRANGE
	observer := observerOrNop(nil)

	// ACT + ASSERT
	require.IsType(t, NopObserver{}, observer, "a nil Observer is replaced, not stored")
	assert.NotPanics(t, func() {
		observer.GuestCall(1, "handle_event", time.Millisecond, GuestCallResultOK)
		observer.HostCall(1, "gameap-log", "log", time.Millisecond, true)
		observer.EventDispatched(proto.EventType_EVENT_TYPE_SERVER_POST_START, EventResultHandled)
		observer.HTTPRequest(1, HTTPResultOK)
	})
}

// deadlineRecordingNodeCmd keeps the deadline of the context the host call
// ran with.
type deadlineRecordingNodeCmd struct {
	mu          sync.Mutex
	deadline    time.Time
	hasDeadline bool
}

func (s *deadlineRecordingNodeCmd) ExecuteCommand(
	ctx context.Context,
	_ *nodecmd.ExecuteCommandRequest,
) (*nodecmd.ExecuteCommandResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deadline, s.hasDeadline = ctx.Deadline()

	return &nodecmd.ExecuteCommandResponse{Output: "ok"}, nil
}

func (s *deadlineRecordingNodeCmd) recorded() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deadline, s.hasDeadline
}

// TestInterceptedRuntime_host_calls_run_with_the_deadline_margin drives a
// real guest (importing.wasm calls gameap-nodecmd from its event handler)
// and checks the host function saw the guest deadline less the margin, so a
// blocking host operation returns before the runtime closes the module.
func TestInterceptedRuntime_host_calls_run_with_the_deadline_margin(t *testing.T) {
	t.Parallel()

	load := func(t *testing.T, cmd *deadlineRecordingNodeCmd) *LoadedPlugin {
		t.Helper()

		recording := hostStubLibrary{func(ctx context.Context, r wazero.Runtime) error {
			return nodecmd.Instantiate(ctx, r, cmd)
		}}

		manager := NewManager(ManagerConfig{
			Libraries: append([]HostLibrary{recording}, importingLibraries(&stubNodeCmd{})[1:]...),
		})

		loaded, err := manager.Load(context.Background(), importingWASM, nil, 42)
		require.NoError(t, err)
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		return loaded
	}

	t.Run("caller_deadline_less_the_margin", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		cmd := &deadlineRecordingNodeCmd{}
		loaded := load(t, cmd)

		deadline := time.Now().Add(10 * time.Second)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		// ACT
		_, err := loaded.Instance.HandleEvent(ctx, &proto.Event{
			Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
		})

		// ASSERT
		require.NoError(t, err)

		got, ok := cmd.recorded()
		require.True(t, ok, "the host call must carry a deadline")
		assert.WithinDuration(t, deadline.Add(-hostCallDeadlineMargin), got, time.Millisecond)
	})

	t.Run("default_call_timeout_less_the_margin", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		cmd := &deadlineRecordingNodeCmd{}
		loaded := load(t, cmd)

		// ACT
		before := time.Now()
		_, err := loaded.Instance.HandleEvent(context.Background(), &proto.Event{
			Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
		})
		after := time.Now()

		// ASSERT
		require.NoError(t, err)

		got, ok := cmd.recorded()
		require.True(t, ok, "a call without a deadline gets the default call timeout")
		assert.False(t, got.Before(before.Add(defaultCallTimeout-hostCallDeadlineMargin)))
		assert.False(t, got.After(after.Add(defaultCallTimeout-hostCallDeadlineMargin)))
	})
}
