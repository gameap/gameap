package pluginssh

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const testPluginID = 7

// realDialer and realResolver keep the engine's policy code in the path while
// the test talks to its own loopback server.
type realDialer struct{}

func (realDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer

	return dialer.DialContext(ctx, network, address)
}

type staticResolver struct {
	answers map[string][]netip.Addr
	err     error
}

func (r staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.answers[host], nil
}

func newTestSessions(t *testing.T, cfg Config) *Sessions {
	t.Helper()

	// Loopback is a private address by definition, so the functional tests run
	// with the block off; the policy itself is covered in ssh_security_test.go.
	cfg.BlockPrivateIPs = false

	svc := newService(nil, nil, cfg, nil, staticResolver{}, realDialer{})
	sessions := svc.NewSessions(testPluginID)
	t.Cleanup(func() {
		sessions.Close()
		svc.Stop()
	})

	return sessions
}

func connectToTestServer(t *testing.T, sessions *Sessions, server *testSSHServer) uint64 {
	t.Helper()

	host, port := server.addr()

	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     host,
		Port:     port,
		User:     "gameap",
		Password: testPassword,
		HostKey:  HostKeyPolicy{AcceptAny: true},
	})
	require.NoError(t, err)
	require.NotZero(t, result.Handle)

	return result.Handle
}

func runToCompletion(t *testing.T, sessions *Sessions, params ExecParams) ExecSnapshot {
	t.Helper()

	operationID, err := sessions.StartExec(context.Background(), params)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, sessions.WaitCompletion(ctx, operationID))

	snapshot, ok := sessions.Snapshot(operationID, 0, 0)
	require.True(t, ok)

	return snapshot
}

func TestSessions_ConnectAndExec(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	host, port := server.addr()

	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     host,
		Port:     port,
		User:     "gameap",
		Password: testPassword,
		HostKey:  HostKeyPolicy{AcceptAny: true},
	})

	require.NoError(t, err)
	assert.NotZero(t, result.Handle)
	assert.Equal(t, server.fingerprint, result.HostKeyFingerprintSHA256,
		"the observed key must be reported so a plugin can pin it")
	assert.Equal(t, "ssh-ed25519", result.HostKeyType)
	assert.Contains(t, result.ServerVersion, "SSH-2.0")
	assert.Equal(t, net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10)), result.Address,
		"the audit trail needs the address that was actually dialed")

	snapshot := runToCompletion(t, sessions, ExecParams{Handle: result.Handle, Command: "echo hello"})

	assert.Equal(t, StatusCompleted, snapshot.Status)
	assert.True(t, snapshot.Succeeded())
	assert.Equal(t, "hello", string(snapshot.Stdout))
	assert.Empty(t, snapshot.Stderr)
}

func TestSessions_ExecOutcomes(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	tests := []struct {
		name       string
		params     ExecParams
		wantStatus Status
		assert     func(t *testing.T, snapshot ExecSnapshot)
	}{
		{
			name:       "non_zero_exit_is_completed_but_not_successful",
			params:     ExecParams{Handle: handle, Command: "exit 3"},
			wantStatus: StatusCompleted,
			assert: func(t *testing.T, snapshot ExecSnapshot) {
				t.Helper()
				assert.Equal(t, int32(3), snapshot.ExitCode)
				assert.False(t, snapshot.Succeeded())
			},
		},
		{
			name:       "stderr_is_captured_separately",
			params:     ExecParams{Handle: handle, Command: "echo-stderr boom"},
			wantStatus: StatusCompleted,
			assert: func(t *testing.T, snapshot ExecSnapshot) {
				t.Helper()
				assert.Empty(t, snapshot.Stdout)
				assert.Equal(t, "boom", string(snapshot.Stderr))
			},
		},
		{
			name:       "stdin_is_piped_into_the_command",
			params:     ExecParams{Handle: handle, Command: "cat", Stdin: []byte("install script")},
			wantStatus: StatusCompleted,
			assert: func(t *testing.T, snapshot ExecSnapshot) {
				t.Helper()
				assert.Equal(t, "install script", string(snapshot.Stdout))
			},
		},
		{
			name:       "output_beyond_the_cap_is_truncated",
			params:     ExecParams{Handle: handle, Command: "spam 4096", MaxOutputBytes: 16},
			wantStatus: StatusCompleted,
			assert: func(t *testing.T, snapshot ExecSnapshot) {
				t.Helper()
				require.Len(t, snapshot.Stdout, 16, "only the head is kept")
				assert.True(t, snapshot.StdoutTruncated)
				assert.Equal(t, uint64(4096), snapshot.StdoutTotal,
					"the plugin still learns how much the command produced")
			},
		},
		{
			name:       "missing_exit_status_is_an_error",
			params:     ExecParams{Handle: handle, Command: "no-status"},
			wantStatus: StatusFailed,
			assert: func(t *testing.T, snapshot ExecSnapshot) {
				t.Helper()
				assert.Contains(t, snapshot.Error, "exit status")
			},
		},
		{
			name:       "timeout_kills_the_command",
			params:     ExecParams{Handle: handle, Command: "sleep 30000", Timeout: 300 * time.Millisecond},
			wantStatus: StatusTimedOut,
			assert: func(t *testing.T, snapshot ExecSnapshot) {
				t.Helper()
				assert.Contains(t, snapshot.Error, "timed out")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshot := runToCompletion(t, sessions, tt.params)

			assert.Equal(t, tt.wantStatus, snapshot.Status)
			tt.assert(t, snapshot)
		})
	}
}

func TestSessions_ExecValidation(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{MaxStdinBytes: 8})
	handle := connectToTestServer(t, sessions, server)

	tests := []struct {
		name      string
		params    ExecParams
		wantError error
	}{
		{
			name:      "empty_command",
			params:    ExecParams{Handle: handle, Command: "  "},
			wantError: ErrCommandRequired,
		},
		{
			name:      "stdin_above_the_cap",
			params:    ExecParams{Handle: handle, Command: "cat", Stdin: []byte("way too much")},
			wantError: ErrStdinTooLarge,
		},
		{
			name:      "invalid_env_name",
			params:    ExecParams{Handle: handle, Command: "env", Env: map[string]string{"A=B": "c"}},
			wantError: ErrInvalidEnvName,
		},
		{
			name:      "unknown_handle",
			params:    ExecParams{Handle: handle + 1, Command: "echo hi"},
			wantError: ErrConnectionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := sessions.StartExec(context.Background(), tt.params)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantError)
		})
	}
}

// TestSessions_EnvRejectedBySshd: sshd only accepts the names in AcceptEnv, so
// the failure must name the variable instead of surfacing as an opaque start
// error.
func TestSessions_EnvRejectedBySshd(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	_, err := sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "env",
		Env:     map[string]string{"GAMEAP_TOKEN": "x"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvRejected)
	assert.Contains(t, err.Error(), "GAMEAP_TOKEN")

	server.allowEnv("GAMEAP_TOKEN")

	snapshot := runToCompletion(t, sessions, ExecParams{
		Handle:  handle,
		Command: "env",
		Env:     map[string]string{"GAMEAP_TOKEN": "x"},
	})
	assert.Contains(t, string(snapshot.Stdout), "GAMEAP_TOKEN=x")
}

func TestSessions_CancelExec(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	operationID, err := sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "sleep 30000",
	})
	require.NoError(t, err)

	require.NoError(t, sessions.Cancel(operationID, "scale down"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sessions.WaitCompletion(ctx, operationID))

	snapshot, ok := sessions.Snapshot(operationID, 0, 0)
	require.True(t, ok)
	assert.Equal(t, StatusCanceled, snapshot.Status)
	assert.Contains(t, snapshot.Error, "scale down")

	err = sessions.Cancel(operationID, "again")
	assert.ErrorIs(t, err, ErrOperationFinished)

	assert.ErrorIs(t, sessions.Cancel("nope", ""), ErrOperationNotFound)
}

// TestSessions_DisconnectCancelsRunningOperations: closing a connection must
// not leave a plugin waiting forever on a command that can no longer finish.
func TestSessions_DisconnectCancelsRunningOperations(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	operationID, err := sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "sleep 30000",
	})
	require.NoError(t, err)

	require.NoError(t, sessions.Disconnect(handle))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sessions.WaitCompletion(ctx, operationID))

	snapshot, ok := sessions.Snapshot(operationID, 0, 0)
	require.True(t, ok)
	assert.Equal(t, StatusCanceled, snapshot.Status)
	assert.Contains(t, snapshot.Error, "connection closed by plugin")

	assert.ErrorIs(t, sessions.Disconnect(handle), ErrConnectionNotFound)
}

// TestSessions_ConnectionLossEndsOperations: a machine that goes away mid
// bootstrap must end the operation with an error, not leave it stuck.
func TestSessions_ConnectionLossEndsOperations(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{KeepaliveInterval: 50 * time.Millisecond})
	handle := connectToTestServer(t, sessions, server)

	operationID, err := sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "kill-conn",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sessions.WaitCompletion(ctx, operationID))

	snapshot, ok := sessions.Snapshot(operationID, 0, 0)
	require.True(t, ok)
	// The dying transport wakes session.Wait and the connection watchdog at
	// the same time. Both arms classify as StatusFailed — the watchdog through
	// the errConnectionLost cause, session.Wait through the channel that
	// closed without an exit status — matching the EXEC_STATUS_FAILED contract
	// in ssh.proto. TestSessions_LostConnectionIsReportedAsFailed pins the
	// watchdog path itself, without the race.
	assert.Equal(t, StatusFailed, snapshot.Status,
		"a machine that went away is a transport failure, not a cancellation")
	assert.False(t, snapshot.Succeeded())
	assert.NotEmpty(t, snapshot.Error, "the plugin has to be able to report why the command died")
}

// TestSessions_LostConnectionIsReportedAsFailed pins what a plugin is told
// when the engine gives up on a connection: connectionLost cancels the
// operations running on it with errConnectionLost as the cause, classifyExec
// unwraps to it and reports StatusFailed — the EXEC_STATUS_FAILED contract in
// ssh.proto ("Transport failure, connection closed"). A retry loop in a plugin
// keys off this status; a user's cancellation must stay distinguishable.
//
// The cancellation is driven directly instead of by killing the socket: a
// dying transport also wakes session.Wait, and whichever lands first decides
// the outcome (see TestSessions_ConnectionLossEndsOperations).
func TestSessions_LostConnectionIsReportedAsFailed(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	// A command that outlives the test keeps session.Wait blocked, so the
	// cancellation is the only thing that can finish the operation.
	operationID, err := sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "sleep 30000",
	})
	require.NoError(t, err)

	sessions.cancelConnectionOperations(handle, "connection closed: EOF", errConnectionLost)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sessions.WaitCompletion(ctx, operationID))

	snapshot, ok := sessions.Snapshot(operationID, 0, 0)
	require.True(t, ok)
	assert.Equal(t, StatusFailed, snapshot.Status)
	assert.Contains(t, snapshot.Error, "connection closed: EOF",
		"the transport error is what the plugin has to report to its operator")
	assert.Equal(t, int32(-1), snapshot.ExitCode)
	assert.False(t, snapshot.Succeeded())
}

func TestSessions_SnapshotOffsets(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	snapshot := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo abcdef"})
	require.Equal(t, "abcdef", string(snapshot.Stdout))
	assert.Equal(t, uint64(6), snapshot.StdoutNextOffset)

	tail, ok := sessions.Snapshot(snapshot.OperationID, 3, 0)
	require.True(t, ok)
	assert.Equal(t, "def", string(tail.Stdout), "offsets let a plugin stream output incrementally")

	past, ok := sessions.Snapshot(snapshot.OperationID, 99, 0)
	require.True(t, ok)
	assert.Empty(t, past.Stdout, "an offset past the end must not panic")
}

func TestSessions_Limits(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	host, port := server.addr()

	t.Run("connections_per_plugin", func(t *testing.T) {
		t.Parallel()
		sessions := newTestSessions(t, Config{MaxConnections: 1})
		connectToTestServer(t, sessions, server)

		_, err := sessions.Connect(context.Background(), ConnectParams{
			Host: host, Port: port, User: "gameap", Password: testPassword,
			HostKey: HostKeyPolicy{AcceptAny: true},
		})

		assert.ErrorIs(t, err, ErrTooManyConnections)
	})

	t.Run("running_operations_per_plugin", func(t *testing.T) {
		t.Parallel()
		sessions := newTestSessions(t, Config{MaxOperations: 1})
		handle := connectToTestServer(t, sessions, server)

		_, err := sessions.StartExec(context.Background(), ExecParams{Handle: handle, Command: "sleep 30000"})
		require.NoError(t, err)

		_, err = sessions.StartExec(context.Background(), ExecParams{Handle: handle, Command: "echo hi"})
		assert.ErrorIs(t, err, ErrTooManyOperations)
	})
}

func TestSessions_RetentionAndEviction(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)

	t.Run("finished_operations_expire", func(t *testing.T) {
		t.Parallel()
		sessions := newTestSessions(t, Config{OperationRetention: 100 * time.Millisecond})
		handle := connectToTestServer(t, sessions, server)

		snapshot := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo hi"})

		assert.Eventually(t, func() bool {
			_, ok := sessions.Snapshot(snapshot.OperationID, 0, 0)

			return !ok
		}, 3*time.Second, 20*time.Millisecond)
	})

	t.Run("oldest_finished_operations_are_evicted", func(t *testing.T) {
		t.Parallel()
		sessions := newTestSessions(t, Config{MaxRetainedOperations: 2, OperationRetention: time.Hour})
		handle := connectToTestServer(t, sessions, server)

		first := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo 1"})
		runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo 2"})
		last := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo 3"})

		_, ok := sessions.Snapshot(first.OperationID, 0, 0)
		assert.False(t, ok, "retained output must stay bounded")

		_, ok = sessions.Snapshot(last.OperationID, 0, 0)
		assert.True(t, ok)
	})
}

// TestSessions_CloseStopsRetentionTimers: an unloaded plugin must not stay
// reachable from the timer queue for the whole retention window.
func TestSessions_CloseStopsRetentionTimers(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{OperationRetention: time.Hour})
	handle := connectToTestServer(t, sessions, server)

	runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo hi"})

	sessions.mu.Lock()
	pending := len(sessions.timers)
	sessions.mu.Unlock()
	require.Equal(t, 1, pending, "a finished operation must be on a retention timer")

	sessions.Close()

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	assert.Empty(t, sessions.timers, "Close must stop and drop every retention timer")
}

// TestSessions_EvictionRemovesTheRetentionTimer: eviction is what bounds the
// retained output, so the timer of an evicted operation must go with it —
// otherwise the timer queue keeps one entry per finished command with no
// upper bound.
func TestSessions_EvictionRemovesTheRetentionTimer(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{MaxRetainedOperations: 2, OperationRetention: time.Hour})
	handle := connectToTestServer(t, sessions, server)

	runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo 1"})
	runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo 2"})
	runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo 3"})

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	assert.Len(t, sessions.timers, 2, "evicted operations must not keep their timers")
}

// TestSessions_FinishAfterCloseKeepsTheSetEmpty: a wait goroutine finishing
// after Close must not resurrect bookkeeping on the closed set — the finished
// list, the timer queue and the running counter all belong to an instance that
// no longer exists.
func TestSessions_FinishAfterCloseKeepsTheSetEmpty(t *testing.T) {
	t.Parallel()
	sessions := newTestSessions(t, Config{})
	op := execTestOperation(connTestHandle, true)

	sessions.mu.Lock()
	sessions.ops[op.id] = op
	sessions.running = 1
	sessions.mu.Unlock()

	sessions.Close()

	sessions.finishOperation(op, execOutcome{status: StatusCompleted, exitCode: 0})

	select {
	case <-op.done:
	default:
		t.Fatal("waiters must still be released by the late finish")
	}

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	assert.Empty(t, sessions.ops)
	assert.Empty(t, sessions.finished, "a late finish must not repopulate the finished list")
	assert.Empty(t, sessions.timers, "a late finish must not schedule a retention timer")
	assert.Zero(t, sessions.running)
}

func TestSessions_ClosedSetRefusesEverything(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	sessions.Close()

	_, err := sessions.Connect(context.Background(), ConnectParams{
		Host: "example.com", User: "gameap", Password: testPassword,
		HostKey: HostKeyPolicy{AcceptAny: true},
	})
	assert.ErrorIs(t, err, ErrSessionsClosed)

	_, err = sessions.StartExec(context.Background(), ExecParams{Handle: handle, Command: "echo hi"})
	assert.ErrorIs(t, err, ErrSessionsClosed)

	assert.ErrorIs(t, sessions.Disconnect(handle), ErrSessionsClosed)

	_, ok := sessions.Snapshot("whatever", 0, 0)
	assert.False(t, ok)
}

// TestSessions_CloseDuringStartExec: Close can land between session
// preparation and registration; the already-started command must be torn down
// and the slot released instead of surviving in a closed set.
func TestSessions_CloseDuringStartExec(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	conn, err := sessions.connection(handle)
	require.NoError(t, err)
	require.NoError(t, sessions.reserveOperationSlot())

	params := ExecParams{Handle: handle, Command: "sleep 30000"}
	session, err := sessions.prepareSession(conn, params)
	require.NoError(t, err)

	sessions.Close()

	_, err = sessions.registerOperation(conn, session, params)
	assert.ErrorIs(t, err, ErrSessionsClosed)

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	assert.Empty(t, sessions.ops, "no operation may survive in a closed set")
	assert.Zero(t, sessions.running, "the reserved slot must be released")
}

func TestSessions_ConnectValidation(t *testing.T) {
	t.Parallel()
	sessions := newTestSessions(t, Config{})

	tests := []struct {
		name      string
		params    ConnectParams
		wantError error
	}{
		{
			name:      "host_required",
			params:    ConnectParams{User: "gameap", Password: testPassword, HostKey: HostKeyPolicy{AcceptAny: true}},
			wantError: ErrHostRequired,
		},
		{
			name:      "user_required",
			params:    ConnectParams{Host: "example.com", Password: testPassword, HostKey: HostKeyPolicy{AcceptAny: true}},
			wantError: ErrUserRequired,
		},
		{
			name: "port_above_65535_is_refused",
			params: ConnectParams{
				Host: "example.com", Port: 70000, User: "gameap",
				Password: testPassword, HostKey: HostKeyPolicy{AcceptAny: true},
			},
			wantError: ErrInvalidPort,
		},
		{
			name:      "host_key_policy_required",
			params:    ConnectParams{Host: "example.com", User: "gameap", Password: testPassword},
			wantError: ErrHostKeyPolicyRequired,
		},
		{
			name:      "credentials_required",
			params:    ConnectParams{Host: "example.com", User: "gameap", HostKey: HostKeyPolicy{AcceptAny: true}},
			wantError: ErrAuthRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := sessions.Connect(context.Background(), tt.params)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantError)
		})
	}
}

func TestSessions_PublicKeyAuthentication(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	host, port := server.addr()

	pair, err := GenerateKeyPair(KeyTypeED25519, "gameap-autoscale")
	require.NoError(t, err)

	authorized := parseAuthorizedKey(t, pair.PublicKey)
	server.authorizeKey(authorized)

	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:          host,
		Port:          port,
		User:          "gameap",
		PrivateKeyPEM: pair.PrivateKeyPEM,
		HostKey:       HostKeyPolicy{AcceptAny: true},
	})

	require.NoError(t, err)
	assert.NotZero(t, result.Handle)
}

func TestSessions_IdleTimeoutClosesConnection(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	// The tiny IdleTimeout ceiling keeps the 10s request floor out of the way;
	// the sweep itself still ticks on the floored one-second interval.
	sessions := newTestSessions(t, Config{KeepaliveInterval: 30 * time.Millisecond, IdleTimeout: 100 * time.Millisecond})
	host, port := server.addr()

	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:        host,
		Port:        port,
		User:        "gameap",
		Password:    testPassword,
		HostKey:     HostKeyPolicy{AcceptAny: true},
		IdleTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	// Polling sessions.connection would count as use and keep the connection
	// alive forever, so the probe watches the connection itself.
	conn, err := sessions.connection(result.Handle)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		select {
		case <-conn.closed:
			return true
		default:
			return false
		}
	}, 5*time.Second, 25*time.Millisecond, "an unused connection must not stay open forever")
}

// TestSessions_ConnectionHost: the host by handle is what lets the audit
// records of commands name the machine they ran on.
func TestSessions_ConnectionHost(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)
	host, _ := server.addr()

	got, ok := sessions.ConnectionHost(handle)
	require.True(t, ok)
	assert.Equal(t, host, got)

	_, ok = sessions.ConnectionHost(handle + 1)
	assert.False(t, ok, "an unknown handle has no host")

	sessions.Close()

	_, ok = sessions.ConnectionHost(handle)
	assert.False(t, ok, "a closed set answers for no handles")
}

// TestSessions_TransferLimitCancelsARunawayCommand: capture stops at the
// output cap, but the remote side used to keep sending at line rate for the
// whole exec timeout; the transfer cap must end the operation instead.
func TestSessions_TransferLimitCancelsARunawayCommand(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{MaxOutputBytes: 1024})
	handle := connectToTestServer(t, sessions, server)

	snapshot := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "spam 33554432"})

	assert.Equal(t, StatusCanceled, snapshot.Status)
	assert.Contains(t, snapshot.Error, "transfer limit exceeded")
	assert.False(t, snapshot.Succeeded())
}

// TestSessions_ConnectionHandoutMarksItUsed: prepareSession can spend longer
// than the idle budget between the handle lookup and the first byte on the
// wire, so the handout itself must reset the idle clock — otherwise the sweep
// closes the connection under a command that is still being started.
func TestSessions_ConnectionHandoutMarksItUsed(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	conn, err := sessions.connection(handle)
	require.NoError(t, err)

	conn.mu.Lock()
	conn.lastUsed = time.Now().Add(-time.Hour)
	conn.mu.Unlock()

	_, err = sessions.connection(handle)
	require.NoError(t, err)

	assert.Less(t, conn.idleFor(), time.Minute, "the handout must count as use")
}

// TestSessions_TinyIdleTimeoutIsFlooredUnderALargeCeiling: the watchdog tick
// derives from the requested idle timeout, so a millisecond request would turn
// the sweep into a probe flood; under the default ceiling it is floored.
func TestSessions_TinyIdleTimeoutIsFlooredUnderALargeCeiling(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	host, port := server.addr()

	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:        host,
		Port:        port,
		User:        "gameap",
		Password:    testPassword,
		HostKey:     HostKeyPolicy{AcceptAny: true},
		IdleTimeout: 5 * time.Millisecond,
	})
	require.NoError(t, err)

	conn, err := sessions.connection(result.Handle)
	require.NoError(t, err)

	assert.Equal(t, minIdleTimeout, conn.idleTimeout)
}

// TestSessions_SignalledCommandReportsTheSignal: a command the remote OS
// killed never sends an exit status, so the signal name is the only thing a
// plugin can act on — an installer killed by the OOM killer must not look like
// a command that simply exited.
func TestSessions_SignalledCommandReportsTheSignal(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	handle := connectToTestServer(t, sessions, server)

	snapshot := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "die-signal KILL"})

	assert.Equal(t, StatusCompleted, snapshot.Status, "the command ran; it just did not exit on its own")
	assert.Equal(t, int32(-1), snapshot.ExitCode, "a signalled command has no exit code to report")
	assert.Equal(t, "KILL", snapshot.ExitSignal)
	assert.Contains(t, snapshot.Error, "killed by signal KILL")
	assert.False(t, snapshot.Succeeded())
}

// TestSessions_RegisterConnectionRechecksTheSlot: checkConnectionSlot decides
// before the dial, so two connects that raced through it both arrive here with
// an open client. The re-check under the lock is what actually enforces the
// limit, and what stops a connect that raced with an unload from parking a
// client in a closed set.
func TestSessions_RegisterConnectionRechecksTheSlot(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)

	t.Run("closed_set_refuses_the_registration", func(t *testing.T) {
		t.Parallel()
		sessions := newTestSessions(t, Config{})
		handle := connectToTestServer(t, sessions, server)
		conn, err := sessions.connection(handle)
		require.NoError(t, err)

		sessions.Close()

		registered, err := sessions.registerConnection(conn.client, ConnectParams{
			Host: "127.0.0.1", User: "gameap",
		}, server.fingerprint)

		assert.ErrorIs(t, err, ErrSessionsClosed)
		assert.Zero(t, registered, "a refused registration hands out no handle")
	})

	t.Run("connection_limit_refuses_the_registration", func(t *testing.T) {
		t.Parallel()
		sessions := newTestSessions(t, Config{MaxConnections: 1})
		handle := connectToTestServer(t, sessions, server)
		conn, err := sessions.connection(handle)
		require.NoError(t, err)

		registered, err := sessions.registerConnection(conn.client, ConnectParams{
			Host: "127.0.0.1", User: "gameap",
		}, server.fingerprint)

		assert.ErrorIs(t, err, ErrTooManyConnections)
		assert.Zero(t, registered)

		sessions.mu.Lock()
		defer sessions.mu.Unlock()
		require.Len(t, sessions.conns, 1, "the refused registration must not have taken a slot")
	})
}

func TestSessions_StatusToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status Status
		want   sshsdk.ExecStatus
	}{
		{name: "running", status: StatusRunning, want: sshsdk.ExecStatus_EXEC_STATUS_RUNNING},
		{name: "completed", status: StatusCompleted, want: sshsdk.ExecStatus_EXEC_STATUS_COMPLETED},
		{name: "errored", status: StatusFailed, want: sshsdk.ExecStatus_EXEC_STATUS_FAILED},
		{name: "timed_out", status: StatusTimedOut, want: sshsdk.ExecStatus_EXEC_STATUS_TIMED_OUT},
		{name: "canceled", status: StatusCanceled, want: sshsdk.ExecStatus_EXEC_STATUS_CANCELED},
		{
			name:   "unknown_status_stays_unspecified",
			status: Status("something_new"),
			want:   sshsdk.ExecStatus_EXEC_STATUS_UNSPECIFIED,
		},
		{name: "empty_status_stays_unspecified", status: "", want: sshsdk.ExecStatus_EXEC_STATUS_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, statusToProto(tt.status),
				"an unmapped status must not read as a different outcome on the wire")
		})
	}
}

const (
	sessionsTestOperationID     = "test-operation"
	sessionsTestOperationHandle = 42
)

// newSessionsTestOperation builds the record a started command produces.
// completionRequest only reads what finish and the captured streams put there,
// so the wire shape can be pinned outcome by outcome without a server.
func newSessionsTestOperation(outputCap int) *operation {
	return &operation{
		id:        sessionsTestOperationID,
		handle:    sessionsTestOperationHandle,
		stdout:    newCapturedStream(outputCap),
		stderr:    newCapturedStream(outputCap),
		cancelFn:  func(error) {},
		done:      make(chan struct{}),
		status:    StatusRunning,
		exitCode:  -1,
		startedAt: time.Now(),
	}
}

// TestSessions_CompletionRequest pins the completion callback: it is everything
// a plugin gets to decide whether its command worked and whether the output it
// is about to fetch is the whole of it.
func TestSessions_CompletionRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		prepare func() *operation
		check   func(t *testing.T, request *sshsdk.HandleExecCompletedRequest)
	}{
		{
			name: "running_operation_has_no_finish_timestamp",
			prepare: func() *operation {
				return newSessionsTestOperation(64)
			},
			check: func(t *testing.T, request *sshsdk.HandleExecCompletedRequest) {
				t.Helper()
				assert.Equal(t, sessionsTestOperationID, request.OperationId)
				assert.Equal(t, uint64(sessionsTestOperationHandle), request.Handle)
				assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_RUNNING, request.Status)
				assert.False(t, request.Success)
				assert.Positive(t, request.StartedAt)
				assert.Zero(t, request.FinishedAt, "an unfinished operation must not claim a finish time")
				assert.Nil(t, request.Error)
			},
		},
		{
			name: "errored_operation_carries_the_error_message",
			prepare: func() *operation {
				op := newSessionsTestOperation(64)
				op.finish(execOutcome{
					status:   StatusFailed,
					exitCode: -1,
					message:  "remote command exited without an exit status",
				})

				return op
			},
			check: func(t *testing.T, request *sshsdk.HandleExecCompletedRequest) {
				t.Helper()
				assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_FAILED, request.Status)
				assert.False(t, request.Success)
				require.NotNil(t, request.Error, "a plugin has nothing else to report to its operator")
				assert.Equal(t, "remote command exited without an exit status", *request.Error)
				assert.Positive(t, request.FinishedAt)
			},
		},
		{
			name: "zero_exit_is_a_success",
			prepare: func() *operation {
				op := newSessionsTestOperation(64)
				op.finish(execOutcome{status: StatusCompleted, exitCode: 0})

				return op
			},
			check: func(t *testing.T, request *sshsdk.HandleExecCompletedRequest) {
				t.Helper()
				assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_COMPLETED, request.Status)
				assert.True(t, request.Success)
				assert.Zero(t, request.ExitCode)
				assert.Nil(t, request.Error, "a clean run carries no error")
			},
		},
		{
			name: "non_zero_exit_is_not_a_success",
			prepare: func() *operation {
				op := newSessionsTestOperation(64)
				op.finish(execOutcome{status: StatusCompleted, exitCode: 3})

				return op
			},
			check: func(t *testing.T, request *sshsdk.HandleExecCompletedRequest) {
				t.Helper()
				assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_COMPLETED, request.Status)
				assert.False(t, request.Success, "the command ran, but it did not do what was asked")
				assert.Equal(t, int32(3), request.ExitCode)
			},
		},
		{
			name: "signalled_command_reports_the_signal",
			prepare: func() *operation {
				op := newSessionsTestOperation(64)
				op.finish(execOutcome{
					status:     StatusCompleted,
					exitCode:   -1,
					exitSignal: "KILL",
					message:    "killed by signal KILL",
				})

				return op
			},
			check: func(t *testing.T, request *sshsdk.HandleExecCompletedRequest) {
				t.Helper()
				assert.Equal(t, "KILL", request.ExitSignal)
				assert.Equal(t, int32(-1), request.ExitCode)
				assert.False(t, request.Success)
				require.NotNil(t, request.Error)
				assert.Equal(t, "killed by signal KILL", *request.Error)
			},
		},
		{
			name: "truncated_output_reports_the_full_size",
			prepare: func() *operation {
				op := newSessionsTestOperation(8)
				_, _ = op.stdout.Write([]byte("0123456789"))
				_, _ = op.stderr.Write(make([]byte, 4096))
				op.finish(execOutcome{status: StatusCompleted, exitCode: 0})

				return op
			},
			check: func(t *testing.T, request *sshsdk.HandleExecCompletedRequest) {
				t.Helper()
				assert.True(t, request.StdoutTruncated, "a plugin must know the output it fetches is a head")
				assert.True(t, request.StderrTruncated)
				assert.Equal(t, uint64(10), request.StdoutTotalBytes, "the full size is reported, not the kept one")
				assert.Equal(t, uint64(4096), request.StderrTotalBytes)
			},
		},
		{
			name: "output_within_the_cap_is_not_flagged",
			prepare: func() *operation {
				op := newSessionsTestOperation(8)
				_, _ = op.stdout.Write([]byte("ok"))
				op.finish(execOutcome{status: StatusCompleted, exitCode: 0})

				return op
			},
			check: func(t *testing.T, request *sshsdk.HandleExecCompletedRequest) {
				t.Helper()
				assert.False(t, request.StdoutTruncated, "a false truncation flag sends a plugin chasing lost output")
				assert.False(t, request.StderrTruncated)
				assert.Equal(t, uint64(2), request.StdoutTotalBytes)
				assert.Zero(t, request.StderrTotalBytes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := tt.prepare().completionRequest()

			require.NotNil(t, request)
			tt.check(t, request)
		})
	}
}

func TestGenerateKeyPair(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		keyType     KeyType
		wantKeyType string
		slow        bool
	}{
		{name: "ed25519", keyType: KeyTypeED25519, wantKeyType: "ssh-ed25519"},
		{name: "ecdsa_p256", keyType: KeyTypeECDSAP256, wantKeyType: "ecdsa-sha2-nistp256"},
		{name: "unspecified_defaults_to_ed25519", keyType: "", wantKeyType: "ssh-ed25519"},
		{name: "rsa4096", keyType: KeyTypeRSA4096, wantKeyType: "ssh-rsa", slow: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.slow && testing.Short() {
				t.Skip("rsa-4096 generation takes seconds; skipped in short mode")
			}

			pair, err := GenerateKeyPair(tt.keyType, "autoscale@panel")

			require.NoError(t, err)
			assert.Equal(t, tt.wantKeyType, pair.KeyType)
			assert.True(t, strings.HasPrefix(pair.FingerprintSHA256, "SHA256:"))
			assert.True(t, strings.HasSuffix(pair.PublicKey, " autoscale@panel"),
				"the comment identifies the key in authorized_keys")

			signer, err := ssh.ParsePrivateKey([]byte(pair.PrivateKeyPEM))
			require.NoError(t, err)
			assert.Equal(t, ssh.FingerprintSHA256(signer.PublicKey()), pair.FingerprintSHA256,
				"the returned fingerprint must match the private key")

			parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pair.PublicKey))
			require.NoError(t, err)
			assert.Equal(t, signer.PublicKey().Marshal(), parsed.Marshal())
		})
	}
}

func parseAuthorizedKey(t *testing.T, line string) ssh.PublicKey {
	t.Helper()

	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line)) //nolint:dogsled // fixed signature
	require.NoError(t, err)

	return key
}
