package pluginssh

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Causes and transport errors classifyExec has to map. The engine wraps a
// lost connection in a cancelCauseError carrying errConnectionLost as its
// cause; the bare-sentinel and plain-error entries below keep the fallback
// branches covered, driven directly.
var (
	errExecTestCause     = errors.New("panel is shutting down")
	errExecTestTransport = errors.New("broken pipe")
)

// execTestDialClient opens a raw client to the test server. classifyExec is fed
// the errors a real session reports, because ssh.ExitError keeps its status and
// signal in unexported fields and cannot be built by hand.
func execTestDialClient(t *testing.T, server *testSSHServer) *ssh.Client {
	t.Helper()

	host, port := server.addr()

	client, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))), &ssh.ClientConfig{
		User:            "gameap",
		Auth:            []ssh.AuthMethod{ssh.Password(testPassword)},
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error { return nil },
		Timeout:         10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// execTestLiveContext is the operation context of a command nothing
// interrupted, so classifyExec has only the wait error to go on.
func execTestLiveContext(t *testing.T) context.Context {
	t.Helper()

	return context.Background()
}

func execTestWaitError(t *testing.T, client *ssh.Client, command string) error {
	t.Helper()

	session, err := client.NewSession()
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	return session.Run(command)
}

// execTestOperation builds a running operation without the session machinery,
// so the bookkeeping around it can be driven directly. The caller may replace
// cancelFn to observe cancellation.
func execTestOperation(handle uint64, notify bool) *operation {
	return &operation{
		id:        xid.New().String(),
		handle:    handle,
		stdout:    newCapturedStream(16),
		stderr:    newCapturedStream(16),
		cancelFn:  func(error) {},
		done:      make(chan struct{}),
		status:    StatusRunning,
		exitCode:  -1,
		notify:    notify,
		startedAt: time.Now(),
	}
}

func execTestEnv(count int) map[string]string {
	env := make(map[string]string, count)
	for i := range count {
		env["GAMEAP_VAR"+strconv.Itoa(i)] = "value"
	}

	return env
}

// TestClassifyExec pins the whole "why did the command stop" mapping: it is the
// only thing a plugin sees about an outcome, and every branch here reaches the
// guest as a different status.
func TestClassifyExec(t *testing.T) {
	t.Parallel()

	server := newTestSSHServer(t)
	client := execTestDialClient(t, server)

	exitCodeErr := execTestWaitError(t, client, "exit 3")
	var exitErr *ssh.ExitError
	require.ErrorAs(t, exitCodeErr, &exitErr)
	require.Equal(t, 3, exitErr.ExitStatus(), "the fixture must be a plain non-zero exit")

	signalErr := execTestWaitError(t, client, "die-signal KILL")
	var signalExitErr *ssh.ExitError
	require.ErrorAs(t, signalErr, &signalExitErr)
	require.Equal(t, "KILL", signalExitErr.Signal(), "the fixture must be a signalled exit")

	missingErr := execTestWaitError(t, client, "no-status")
	require.ErrorAs(t, missingErr, new(*ssh.ExitMissingError), "the fixture must be a missing exit status")

	tests := []struct {
		name        string
		opCtx       func(t *testing.T) context.Context
		waitErr     error
		wantStatus  Status
		wantCode    int32
		wantSignal  string
		wantMessage string
	}{
		{
			name: "expired_exec_timeout_is_reported_as_timed_out",
			opCtx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithTimeoutCause(context.Background(), time.Nanosecond, errExecTimeout)
				t.Cleanup(cancel)
				<-ctx.Done()

				return ctx
			},
			waitErr:     missingErr,
			wantStatus:  StatusTimedOut,
			wantCode:    -1,
			wantMessage: errExecTimeout.Error(),
		},
		{
			name: "lost_connection_cause_is_reported_as_errored",
			opCtx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(errConnectionLost)

				return ctx
			},
			waitErr:     missingErr,
			wantStatus:  StatusFailed,
			wantCode:    -1,
			wantMessage: errConnectionLost.Error(),
		},
		{
			name: "engine_wrapped_connection_loss_is_reported_as_errored",
			opCtx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(&cancelCauseError{reason: "connection closed: EOF", cause: errConnectionLost})

				return ctx
			},
			waitErr:     missingErr,
			wantStatus:  StatusFailed,
			wantCode:    -1,
			wantMessage: "connection closed: EOF",
		},
		{
			name: "explicit_cancellation_carries_its_reason",
			opCtx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(&cancelCauseError{reason: "scale down"})

				return ctx
			},
			waitErr:     missingErr,
			wantStatus:  StatusCanceled,
			wantCode:    -1,
			wantMessage: "scale down",
		},
		{
			name: "unrecognised_cause_is_reported_as_canceled",
			opCtx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(errExecTestCause)

				return ctx
			},
			waitErr:     errExecTestTransport,
			wantStatus:  StatusCanceled,
			wantCode:    -1,
			wantMessage: errExecTestCause.Error(),
		},
		{
			name: "clean_exit_wins_over_a_late_cancellation",
			opCtx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(&cancelCauseError{reason: "plugin unloaded"})

				return ctx
			},
			wantStatus: StatusCompleted,
			wantCode:   0,
		},
		{
			name:       "clean_exit_is_completed_with_code_zero",
			opCtx:      execTestLiveContext,
			wantStatus: StatusCompleted,
			wantCode:   0,
		},
		{
			name:       "non_zero_exit_keeps_the_remote_code",
			opCtx:      execTestLiveContext,
			waitErr:    exitCodeErr,
			wantStatus: StatusCompleted,
			wantCode:   3,
		},
		{
			name:        "signalled_exit_reports_the_signal_instead_of_a_code",
			opCtx:       execTestLiveContext,
			waitErr:     signalErr,
			wantStatus:  StatusCompleted,
			wantCode:    -1,
			wantSignal:  "KILL",
			wantMessage: "killed by signal KILL",
		},
		{
			name:        "missing_exit_status_is_reported_as_errored",
			opCtx:       execTestLiveContext,
			waitErr:     missingErr,
			wantStatus:  StatusFailed,
			wantCode:    -1,
			wantMessage: "without an exit status",
		},
		{
			name:        "transport_error_is_reported_as_errored",
			opCtx:       execTestLiveContext,
			waitErr:     errExecTestTransport,
			wantStatus:  StatusFailed,
			wantCode:    -1,
			wantMessage: errExecTestTransport.Error(),
		},
		{
			name: "cancellation_wins_over_the_exit_status",
			opCtx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(&cancelCauseError{reason: "plugin unloaded"})

				return ctx
			},
			waitErr:     exitCodeErr,
			wantStatus:  StatusCanceled,
			wantCode:    -1,
			wantMessage: "plugin unloaded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			opCtx := tt.opCtx(t)

			// ACT
			outcome := classifyExec(opCtx, tt.waitErr)

			// ASSERT
			assert.Equal(t, tt.wantStatus, outcome.status, "the status is what the plugin branches on")
			assert.Equal(t, tt.wantCode, outcome.exitCode, "exit code mismatch")
			assert.Equal(t, tt.wantSignal, outcome.exitSignal, "exit signal mismatch")

			if tt.wantMessage == "" {
				assert.Empty(t, outcome.message, "a clean outcome must not carry an error message")

				return
			}

			assert.Contains(t, outcome.message, tt.wantMessage, "the message is what the plugin reads")
		})
	}
}

// TestOperation_FinishPublishesTheOutcomeOnce: the completion callback is
// delivered once and the first terminal state is the one that sticks, so a late
// transport error cannot rewrite a command the plugin already saw succeed.
func TestOperation_FinishPublishesTheOutcomeOnce(t *testing.T) {
	t.Parallel()

	// ARRANGE
	op := execTestOperation(1, true)

	// ACT
	first := op.finish(execOutcome{status: StatusCompleted, exitCode: 0})
	second := op.finish(execOutcome{status: StatusFailed, exitCode: 9, exitSignal: "KILL", message: "late"})

	// ASSERT
	assert.True(t, first, "the first outcome asks for the completion callback")
	assert.False(t, second, "a second outcome must not fire the callback again")

	snapshot := op.snapshot(0, 0)
	assert.Equal(t, StatusCompleted, snapshot.Status, "the first outcome wins")
	assert.Equal(t, int32(0), snapshot.ExitCode)
	assert.Empty(t, snapshot.ExitSignal)
	assert.Empty(t, snapshot.Error)
	assert.False(t, snapshot.FinishedAt.IsZero(), "a finished operation must carry a finish time")
}

// TestOperation_SubscribeArmsTheCompletionCallback: a plugin that asks for the
// callback after starting the command must still get it.
func TestOperation_SubscribeArmsTheCompletionCallback(t *testing.T) {
	t.Parallel()

	// ARRANGE
	op := execTestOperation(1, false)

	// ACT
	replay := op.subscribe()

	// ASSERT
	assert.False(t, replay, "a running operation has nothing to replay")
	assert.True(t, op.finish(execOutcome{status: StatusCompleted}),
		"the callback must fire once the subscribed operation ends")
}

// TestOperation_SubscribeOnFinishedOperationAsksForReplay: a command that ended
// while the guest was still deciding must not lose its event.
func TestOperation_SubscribeOnFinishedOperationAsksForReplay(t *testing.T) {
	t.Parallel()

	// ARRANGE
	op := execTestOperation(1, false)
	op.finish(execOutcome{status: StatusCompleted})

	// ACT
	replay := op.subscribe()

	// ASSERT
	assert.True(t, replay, "a finished operation must be delivered immediately")
}

// TestOperation_DropNotificationSilencesTheCallback: a reloaded module instance
// must not receive completions belonging to the previous one.
func TestOperation_DropNotificationSilencesTheCallback(t *testing.T) {
	t.Parallel()

	// ARRANGE
	op := execTestOperation(1, true)

	// ACT
	op.dropNotification()

	// ASSERT
	assert.False(t, op.finish(execOutcome{status: StatusCanceled, exitCode: -1}),
		"a dropped subscription must not deliver anything")
}

// TestSessions_ExecSizeLimits guards the two caps that keep a plugin from
// pushing an unbounded request through the SSH channel. Both are checked before
// the handle is resolved, so an unknown handle marks the accepted boundary.
func TestSessions_ExecSizeLimits(t *testing.T) {
	t.Parallel()

	sessions := newTestSessions(t, Config{})

	tests := []struct {
		name      string
		params    ExecParams
		wantError error
	}{
		{
			name:      "command_above_the_byte_cap",
			params:    ExecParams{Command: strings.Repeat("a", maxCommandBytes+1)},
			wantError: ErrCommandTooLong,
		},
		{
			name:      "command_at_the_byte_cap_passes_validation",
			params:    ExecParams{Command: strings.Repeat("a", maxCommandBytes)},
			wantError: ErrConnectionNotFound,
		},
		{
			name:      "more_environment_variables_than_allowed",
			params:    ExecParams{Command: "env", Env: execTestEnv(maxEnvVars + 1)},
			wantError: ErrTooManyEnvVars,
		},
		{
			name:      "environment_variables_at_the_cap_pass_validation",
			params:    ExecParams{Command: "env", Env: execTestEnv(maxEnvVars)},
			wantError: ErrConnectionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			operationID, err := sessions.StartExec(context.Background(), tt.params)

			// ASSERT
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantError)
			assert.Empty(t, operationID, "a rejected request must not hand out an operation id")
		})
	}
}

// TestSessions_ReserveOperationSlotOnClosedSet: an unloaded plugin must not be
// able to book capacity it can no longer use.
func TestSessions_ReserveOperationSlotOnClosedSet(t *testing.T) {
	t.Parallel()

	// ARRANGE
	sessions := newTestSessions(t, Config{})
	sessions.Close()

	// ACT
	err := sessions.reserveOperationSlot()

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionsClosed)
}

// TestSessions_CancelWithoutReasonReportsTheDefault: the reason is the only
// explanation the plugin gets, so an empty one must still say who stopped it.
func TestSessions_CancelWithoutReasonReportsTheDefault(t *testing.T) {
	t.Parallel()

	// ARRANGE
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	operationID, err := sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "sleep 30000",
	})
	require.NoError(t, err)

	// ACT
	require.NoError(t, sessions.Cancel(operationID, ""))

	// ASSERT
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sessions.WaitCompletion(ctx, operationID))

	snapshot, ok := sessions.Snapshot(operationID, 0, 0)
	require.True(t, ok)
	assert.Equal(t, StatusCanceled, snapshot.Status)
	assert.Contains(t, snapshot.Error, "canceled by plugin", "an unexplained cancel still names its origin")
}

func TestSessions_WaitCompletionUnknownOperation(t *testing.T) {
	t.Parallel()

	// ARRANGE
	sessions := newTestSessions(t, Config{})

	// ACT
	err := sessions.WaitCompletion(context.Background(), "no-such-operation")

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOperationNotFound)
}

func TestSessions_SubscribeCompletionUnknownOperation(t *testing.T) {
	t.Parallel()

	// ARRANGE
	sessions := newTestSessions(t, Config{})

	// ACT
	err := sessions.SubscribeCompletion("no-such-operation")

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOperationNotFound)
}

// TestSessions_WaitCompletionBudgetExpiredLeavesTheCommandRunning: the command
// is detached from the guest call on purpose, so a caller that runs out of
// budget gets its context error back and the command keeps going.
func TestSessions_WaitCompletionBudgetExpiredLeavesTheCommandRunning(t *testing.T) {
	t.Parallel()

	// ARRANGE
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	operationID, err := sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "sleep 30000",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// ACT
	waitErr := sessions.WaitCompletion(ctx, operationID)

	// ASSERT
	require.Error(t, waitErr)
	assert.ErrorIs(t, waitErr, context.DeadlineExceeded)

	snapshot, ok := sessions.Snapshot(operationID, 0, 0)
	require.True(t, ok)
	assert.Equal(t, StatusRunning, snapshot.Status, "giving up on the wait must not stop the command")
}

// TestSessions_WaitCompletionDuringCloseReportsTheClosedSet: unloading a plugin
// must not leave a waiter parked on an operation that can no longer finish.
func TestSessions_WaitCompletionDuringCloseReportsTheClosedSet(t *testing.T) {
	t.Parallel()

	// ARRANGE
	sessions := newTestSessions(t, Config{})

	// An operation nothing will ever finish isolates the branch under test:
	// closing the set is then the only thing that can wake the waiter.
	op := execTestOperation(1, false)
	sessions.mu.Lock()
	sessions.ops[op.id] = op
	sessions.mu.Unlock()

	waitResult := make(chan error, 1)
	go func() { waitResult <- sessions.WaitCompletion(context.Background(), op.id) }()

	// ACT
	// The waiter needs a moment to park; closing sooner is answered with the
	// same error by the lookup, so the assertion holds either way.
	time.Sleep(50 * time.Millisecond)
	sessions.Close()

	// ASSERT
	select {
	case err := <-waitResult:
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionsClosed)
	case <-time.After(10 * time.Second):
		t.Fatal("WaitCompletion must not outlive the session set")
	}
}

// TestSessions_StartExecReleasesTheSlotWhenNothingRan: a command that never
// started must give its operation slot back, otherwise a plugin retrying an
// unreachable host quietly locks itself out of the engine.
func TestSessions_StartExecReleasesTheSlotWhenNothingRan(t *testing.T) {
	t.Parallel()

	t.Run("client_refuses_to_open_a_session", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		server := newTestSSHServer(t)
		sessions := newTestSessions(t, Config{MaxOperations: 1})

		// Registered by hand and without the watchdog goroutine: a running
		// connectionLost would drop the handle and the test would see
		// ErrConnectionNotFound instead of the start failure it is after.
		broken := execTestDialClient(t, server)
		require.NoError(t, broken.Close())

		const brokenHandle = 4242

		sessions.mu.Lock()
		sessions.conns[brokenHandle] = &connection{
			handle: brokenHandle,
			client: broken,
			host:   "127.0.0.1",
			closed: make(chan struct{}),
		}
		sessions.mu.Unlock()

		// ACT
		for range 3 {
			_, err := sessions.StartExec(context.Background(), ExecParams{
				Handle:  brokenHandle,
				Command: "echo hi",
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrStartFailed)
		}

		// ASSERT
		handle := connectToTestServer(t, sessions, server)
		snapshot := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo alive"})
		assert.Equal(t, "alive", string(snapshot.Stdout),
			"the only operation slot must still be free after failed starts")
	})

	t.Run("server_refuses_the_exec_request", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		server := newTestSSHServer(t)
		sessions := newTestSessions(t, Config{MaxOperations: 1})
		handle := connectToTestServer(t, sessions, server)

		// ACT
		for range 3 {
			_, err := sessions.StartExec(context.Background(), ExecParams{
				Handle:  handle,
				Command: rejectExecCommand,
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrStartFailed)
		}

		// ASSERT
		snapshot := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo alive"})
		assert.Equal(t, "alive", string(snapshot.Stdout),
			"the only operation slot must still be free after refused execs")
	})
}
