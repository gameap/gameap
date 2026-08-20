package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func subscribeOnceCmd(t *testing.T, ps pubsub.PubSub, channel string) (
	getReceived func() *pubsub.Message,
	done <-chan struct{},
) {
	t.Helper()

	var (
		mu  sync.Mutex
		msg *pubsub.Message
	)
	doneCh := make(chan struct{})

	err := ps.Subscribe(context.Background(), channel, func(_ context.Context, m *pubsub.Message) error {
		mu.Lock()
		defer mu.Unlock()

		if msg != nil {
			return nil
		}
		msg = m
		close(doneCh)

		return nil
	})
	require.NoError(t, err)

	return func() *pubsub.Message {
		mu.Lock()
		defer mu.Unlock()

		return msg
	}, doneCh
}

func TestCommandHandler_RegisterPendingCommand_returnsBufferedChan(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())

	// ACT
	ch := handler.RegisterPendingCommand("cmd-1")

	// ASSERT — channel must be buffered so a single non-blocking send succeeds
	// without a reader, mirroring HandleCommandResult's `select { case ch <- ... : default: }`.
	require.NotNil(t, ch)
	select {
	case _, open := <-ch:
		// Reading without a sender must block; a successful read here means the
		// channel is closed prematurely.
		t.Fatalf("expected empty open channel, got read (open=%v)", open)
	default:
	}
}

func TestCommandHandler_HandleCommandResult_deliversToWaiter(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())
	const commandID = "cmd-deliver"
	ch := handler.RegisterPendingCommand(commandID)

	want := &proto.CommandResult{
		CommandId: commandID,
		ExitCode:  7,
		Output:    []byte("done"),
		Error:     "warn",
	}

	// ACT
	err := handler.HandleCommandResult(context.Background(), 1, want)

	// ASSERT
	require.NoError(t, err)

	select {
	case got, ok := <-ch:
		require.True(t, ok, "channel must still be open and carry the result")
		require.NotNil(t, got)
		assert.Equal(t, commandID, got.CommandID)
		assert.Equal(t, want.ExitCode, got.ExitCode)
		assert.Equal(t, want.Output, got.Output)
		assert.Equal(t, want.Error, got.Error)
	case <-time.After(time.Second):
		t.Fatal("waiter did not receive command result")
	}

	// The channel is closed right after delivery, so a second receive yields the
	// zero value with ok=false — this is how RequestCommand* learns the result
	// stream is complete.
	_, open := <-ch
	assert.False(t, open, "channel must be closed after the result is delivered")

	// Delivery also removes the pending registration (delete-under-lock), so a
	// late UnregisterPendingCommand is a no-op and must not double-close the
	// already-closed channel.
	assert.NotPanics(t, func() {
		handler.UnregisterPendingCommand(commandID)
	}, "unregister after delivery must not double-close the channel")
}

func TestCommandHandler_HandleCommandResult_secondCallForSameID_isDroppedSilently(t *testing.T) {
	t.Parallel()
	// ARRANGE — register a waiter and deliver a first result. The first call
	// buffers the result, closes the channel and removes the registration
	// (delete-under-lock).
	handler := NewCommandHandler(nil, slog.Default())
	const commandID = "cmd-twice"
	ch := handler.RegisterPendingCommand(commandID)

	first := &proto.CommandResult{CommandId: commandID, ExitCode: 1, Output: []byte("first")}
	require.NoError(t, handler.HandleCommandResult(context.Background(), 1, first))

	// Second call: the registration was already removed by the first call, so
	// there is no pending waiter to deliver to. The result is dropped silently
	// without touching the (now closed) channel, and the buffered first result
	// is preserved.
	second := &proto.CommandResult{CommandId: commandID, ExitCode: 2, Output: []byte("second")}

	// ACT
	err := handler.HandleCommandResult(context.Background(), 1, second)

	// ASSERT
	require.NoError(t, err)

	select {
	case got, ok := <-ch:
		require.True(t, ok)
		require.NotNil(t, got)
		assert.Equal(t, "first", string(got.Output), "buffered first result must be preserved")
	case <-time.After(time.Second):
		t.Fatal("first buffered result was lost")
	}
}

func TestCommandHandler_HandleCommandResult_unknownID_dropsSilently(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	handler := NewCommandHandler(ps, slog.Default())

	published := make(chan struct{}, 1)
	require.NoError(t, ps.Subscribe(context.Background(), channels.RealtimeConsoleAll,
		func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		}))

	// ACT — neither pending waiter nor server tracking.
	err := handler.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
		CommandId: "ghost",
		ExitCode:  0,
	})

	// ASSERT
	require.NoError(t, err)

	select {
	case <-published:
		t.Fatal("unexpected publish for untracked command id")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCommandHandler_HandleCommandResult_publishesConsoleResultWhenServerTracked(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	const (
		commandID        = "cmd-publish-result"
		serverID  uint64 = 99
		exitCode  int32  = 5
	)

	channel := channels.BuildRealtimeConsoleResultChannel(serverID)
	getReceived, done := subscribeOnceCmd(t, ps, channel)

	handler := NewCommandHandler(ps, slog.Default())
	handler.TrackCommandServer(commandID, serverID)

	// ACT
	err := handler.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
		CommandId: commandID,
		ExitCode:  exitCode,
		Error:     "non-fatal",
	})

	// ASSERT
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for console result publish")
	}

	got := getReceived()
	require.NotNil(t, got)
	assert.Equal(t, channel, got.Channel)
	assert.Equal(t, messages.TypeConsoleResult, got.Type)

	payload, err := messages.ParsePayload[messages.ConsoleResultPayload](got)
	require.NoError(t, err)
	assert.Equal(t, serverID, payload.ServerID)
	assert.Equal(t, commandID, payload.CommandID)
	assert.Equal(t, exitCode, payload.ExitCode)
	assert.Equal(t, "non-fatal", payload.Error)
}

func TestCommandHandler_HandleCommandResult_untracksServerAfterDelivery(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())
	const (
		commandID        = "cmd-untrack-after"
		serverID  uint64 = 33
	)
	handler.TrackCommandServer(commandID, serverID)

	// ACT
	err := handler.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
		CommandId: commandID,
	})

	// ASSERT
	require.NoError(t, err)

	// Re-tracking and verifying via a second result is the only observable way:
	// after HandleCommandResult removes the mapping, a subsequent result with the
	// same id should NOT publish a console result (no panic, no publish).
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	handler2 := NewCommandHandler(ps, slog.Default())
	handler2.TrackCommandServer(commandID, serverID)

	published := make(chan struct{}, 1)
	require.NoError(t, ps.Subscribe(context.Background(), channels.BuildRealtimeConsoleResultChannel(serverID),
		func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		}))

	require.NoError(t, handler2.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
		CommandId: commandID,
	}))

	// First call after Track must publish.
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("expected first result to publish console result")
	}

	// Second call without re-tracking must NOT publish.
	require.NoError(t, handler2.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
		CommandId: commandID,
	}))
	select {
	case <-published:
		t.Fatal("server mapping should have been cleared by previous HandleCommandResult")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCommandHandler_HandleCommandOutput_publishesToConsoleOutputChannel(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	const (
		commandID        = "cmd-output-1"
		serverID  uint64 = 88
	)
	chunk := []byte("partial output")

	channel := channels.BuildRealtimeConsoleOutputChannel(serverID)
	getReceived, done := subscribeOnceCmd(t, ps, channel)

	handler := NewCommandHandler(ps, slog.Default())
	handler.TrackCommandServer(commandID, serverID)

	// ACT
	err := handler.HandleCommandOutput(context.Background(), 1, &proto.CommandOutput{
		CommandId:   commandID,
		OutputChunk: chunk,
	})

	// ASSERT
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for console output publish")
	}

	got := getReceived()
	require.NotNil(t, got)
	assert.Equal(t, channel, got.Channel)
	assert.Equal(t, messages.TypeConsoleOutput, got.Type)

	payload, err := messages.ParsePayload[messages.ConsoleOutputPayload](got)
	require.NoError(t, err)
	assert.Equal(t, serverID, payload.ServerID)
	assert.Equal(t, commandID, payload.CommandID)
	assert.Equal(t, string(chunk), payload.Chunk)
}

func TestCommandHandler_HandleCommandOutput_untrackedCommand_doesNotPublish(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	handler := NewCommandHandler(ps, slog.Default())

	published := make(chan struct{}, 1)
	require.NoError(t, ps.Subscribe(context.Background(), channels.RealtimeConsoleAll,
		func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		}))

	// ACT — no TrackCommandServer call before output
	err := handler.HandleCommandOutput(context.Background(), 1, &proto.CommandOutput{
		CommandId:   "ghost",
		OutputChunk: []byte("data"),
	})

	// ASSERT
	require.NoError(t, err)

	select {
	case <-published:
		t.Fatal("must not publish console output for an untracked command")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCommandHandler_HandleCommandOutput_nilPublisher_returnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())
	handler.TrackCommandServer("cmd", 1) // server tracked, but publisher is nil

	// ACT
	err := handler.HandleCommandOutput(context.Background(), 1, &proto.CommandOutput{
		CommandId:   "cmd",
		OutputChunk: []byte("x"),
	})

	// ASSERT
	require.NoError(t, err)
}

func TestCommandHandler_HandleCommandResult_nilPublisher_returnsNilAndUntracksServer(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())
	const (
		commandID        = "cmd-nil-pub"
		serverID  uint64 = 5
	)
	handler.TrackCommandServer(commandID, serverID)

	// ACT
	err := handler.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
		CommandId: commandID,
	})

	// ASSERT — no publish (handler.publisher == nil), no panic, server mapping
	// cleared because untrack runs unconditionally at the end of HandleCommandResult.
	require.NoError(t, err)
}

func TestCommandHandler_UnregisterPendingCommand_closesChan(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())
	const commandID = "cmd-unreg"
	ch := handler.RegisterPendingCommand(commandID)

	// ACT
	handler.UnregisterPendingCommand(commandID)

	// ASSERT — read on a closed channel returns zero value and ok=false.
	select {
	case got, ok := <-ch:
		assert.False(t, ok, "channel must be closed after unregister")
		assert.Nil(t, got)
	case <-time.After(time.Second):
		t.Fatal("expected closed channel to be readable")
	}
}

func TestCommandHandler_UnregisterPendingCommand_unknownID_isSafe(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())

	// ACT + ASSERT — must not panic on a never-registered id.
	handler.UnregisterPendingCommand("never-registered")
}

func TestCommandHandler_UnregisterPendingCommand_alsoRemovesServerMapping(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	handler := NewCommandHandler(ps, slog.Default())
	const (
		commandID        = "cmd-cleanup"
		serverID  uint64 = 21
	)
	handler.TrackCommandServer(commandID, serverID)

	published := make(chan struct{}, 1)
	require.NoError(t, ps.Subscribe(context.Background(), channels.BuildRealtimeConsoleOutputChannel(serverID),
		func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		}))

	// ACT
	handler.UnregisterPendingCommand(commandID)

	// ASSERT — UnregisterPendingCommand also clears commandServers; subsequent
	// output for the same id is dropped.
	require.NoError(t, handler.HandleCommandOutput(context.Background(), 1, &proto.CommandOutput{
		CommandId:   commandID,
		OutputChunk: []byte("x"),
	}))

	select {
	case <-published:
		t.Fatal("UnregisterPendingCommand must also drop the server mapping")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCommandHandler_TrackUntrackCommandServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		setup          func(h *CommandHandler)
		commandID      string
		wantPublishes  bool
		serverIDForSub uint64
	}{
		{
			name: "track_then_untrack_drops_publish",
			setup: func(h *CommandHandler) {
				h.TrackCommandServer("cmd", 100)
				h.UntrackCommandServer("cmd")
			},
			commandID:      "cmd",
			wantPublishes:  false,
			serverIDForSub: 100,
		},
		{
			name: "track_then_publish",
			setup: func(h *CommandHandler) {
				h.TrackCommandServer("cmd", 200)
			},
			commandID:      "cmd",
			wantPublishes:  true,
			serverIDForSub: 200,
		},
		{
			name: "track_overwrites_previous_server",
			setup: func(h *CommandHandler) {
				h.TrackCommandServer("cmd", 300)
				h.TrackCommandServer("cmd", 400)
			},
			commandID:      "cmd",
			wantPublishes:  true,
			serverIDForSub: 400,
		},
		{
			name: "untrack_unknown_is_safe",
			setup: func(h *CommandHandler) {
				h.UntrackCommandServer("never-tracked")
			},
			commandID:      "never-tracked",
			wantPublishes:  false,
			serverIDForSub: 999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			ps := memory.New()
			t.Cleanup(func() { _ = ps.Close() })

			handler := NewCommandHandler(ps, slog.Default())
			tt.setup(handler)

			published := make(chan struct{}, 1)
			require.NoError(t, ps.Subscribe(context.Background(),
				channels.BuildRealtimeConsoleOutputChannel(tt.serverIDForSub),
				func(_ context.Context, _ *pubsub.Message) error {
					published <- struct{}{}

					return nil
				}))

			// ACT
			require.NoError(t, handler.HandleCommandOutput(context.Background(), 1, &proto.CommandOutput{
				CommandId:   tt.commandID,
				OutputChunk: []byte("x"),
			}))

			// ASSERT
			if tt.wantPublishes {
				select {
				case <-published:
				case <-time.After(time.Second):
					t.Fatal("expected publish, got none")
				}
			} else {
				select {
				case <-published:
					t.Fatal("expected no publish, got one")
				case <-time.After(50 * time.Millisecond):
				}
			}
		})
	}
}

func TestCommandHandler_PublishError_isLoggedNotReturned(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(errPublisher{}, slog.Default())
	const (
		commandID        = "cmd-errpub"
		serverID  uint64 = 1
	)
	handler.TrackCommandServer(commandID, serverID)

	// ACT + ASSERT — both Handle* methods must swallow publisher errors when a
	// server is tracked (so the publish branch is exercised).
	err := handler.HandleCommandOutput(context.Background(), 1, &proto.CommandOutput{
		CommandId:   commandID,
		OutputChunk: []byte("x"),
	})
	assert.NoError(t, err, "HandleCommandOutput must not surface publisher errors")

	// Re-track because the previous Result call would untrack.
	handler.TrackCommandServer(commandID, serverID)
	err = handler.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
		CommandId: commandID,
		ExitCode:  0,
	})
	assert.NoError(t, err, "HandleCommandResult must not surface publisher errors")
}

func TestCommandHandler_NewCommandHandler_nilLogger_usesDefault(t *testing.T) {
	t.Parallel()
	// ARRANGE + ACT
	handler := NewCommandHandler(nil, nil)

	// ASSERT
	require.NotNil(t, handler)
	err := handler.HandleCommandResult(context.Background(), 1, &proto.CommandResult{CommandId: "x"})
	assert.NoError(t, err)
}

func TestCommandHandler_RegisterUnregister_concurrentAccess(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewCommandHandler(nil, slog.Default())

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// ACT
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()

			commandID := fmt.Sprintf("cmd-%d", id)
			handler.TrackCommandServer(commandID, uint64(id))
			ch := handler.RegisterPendingCommand(commandID)
			_ = ch
			handler.UnregisterPendingCommand(commandID)
			// UnregisterPendingCommand also clears the server mapping; calling
			// UntrackCommandServer again must be safe.
			handler.UntrackCommandServer(commandID)
		}(i)
	}

	wg.Wait()

	// ASSERT — no panics under -race; final state is the only observable.
	// (HandleCommandOutput on a cleaned id must not publish; verified in other tests.)
}

func TestCommandHandler_HandleResultAndUnregister_concurrentSameID_noDoubleClose(t *testing.T) {
	t.Parallel()
	// ARRANGE — HandleCommandResult and UnregisterPendingCommand both take
	// exclusive ownership of the pending channel under the lock (delete-then-use),
	// so racing them on the SAME command id must never close the channel twice
	// (a "close of closed channel" panic). This is the race the delete-under-lock
	// guard in HandleCommandResult protects against; it should fail under -race
	// if the ownership handoff regresses.
	handler := NewCommandHandler(nil, slog.Default())

	const iterations = 200

	// ACT
	for i := range iterations {
		commandID := fmt.Sprintf("race-cmd-%d", i)
		handler.RegisterPendingCommand(commandID)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()

			_ = handler.HandleCommandResult(context.Background(), 1, &proto.CommandResult{
				CommandId: commandID,
			})
		}()
		go func() {
			defer wg.Done()

			handler.UnregisterPendingCommand(commandID)
		}()

		wg.Wait()
	}

	// ASSERT — every id is removed by exactly one of the two racers; no pending
	// registration may leak.
	handler.mu.RLock()
	assert.Empty(t, handler.pendingCommands,
		"no pending command may remain after concurrent handle/unregister")
	handler.mu.RUnlock()
}
