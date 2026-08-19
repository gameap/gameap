package pluginssh

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

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

	snapshot := runToCompletion(t, sessions, ExecParams{Handle: result.Handle, Command: "echo hello"})

	assert.Equal(t, StatusCompleted, snapshot.Status)
	assert.True(t, snapshot.Succeeded())
	assert.Equal(t, "hello", string(snapshot.Stdout))
	assert.Empty(t, snapshot.Stderr)
}

func TestSessions_ExecOutcomes(t *testing.T) {
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
				assert.Len(t, snapshot.Stdout, 16, "only the head is kept")
				assert.True(t, snapshot.StdoutTruncated)
				assert.Equal(t, uint64(4096), snapshot.StdoutTotal,
					"the plugin still learns how much the command produced")
			},
		},
		{
			name:       "missing_exit_status_is_a_failure",
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
			snapshot := runToCompletion(t, sessions, tt.params)

			assert.Equal(t, tt.wantStatus, snapshot.Status)
			tt.assert(t, snapshot)
		})
	}
}

func TestSessions_ExecValidation(t *testing.T) {
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

// TestSessions_ConnectionLossFailsOperations: a machine that goes away mid
// bootstrap must produce a failed operation, not a stuck one.
func TestSessions_ConnectionLossFailsOperations(t *testing.T) {
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
	assert.NotEqual(t, StatusCompleted, snapshot.Status)
}

func TestSessions_SnapshotOffsets(t *testing.T) {
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
	server := newTestSSHServer(t)
	host, port := server.addr()

	t.Run("connections_per_plugin", func(t *testing.T) {
		sessions := newTestSessions(t, Config{MaxConnections: 1})
		connectToTestServer(t, sessions, server)

		_, err := sessions.Connect(context.Background(), ConnectParams{
			Host: host, Port: port, User: "gameap", Password: testPassword,
			HostKey: HostKeyPolicy{AcceptAny: true},
		})

		assert.ErrorIs(t, err, ErrTooManyConnections)
	})

	t.Run("running_operations_per_plugin", func(t *testing.T) {
		sessions := newTestSessions(t, Config{MaxOperations: 1})
		handle := connectToTestServer(t, sessions, server)

		_, err := sessions.StartExec(context.Background(), ExecParams{Handle: handle, Command: "sleep 30000"})
		require.NoError(t, err)

		_, err = sessions.StartExec(context.Background(), ExecParams{Handle: handle, Command: "echo hi"})
		assert.ErrorIs(t, err, ErrTooManyOperations)
	})
}

func TestSessions_RetentionAndEviction(t *testing.T) {
	server := newTestSSHServer(t)

	t.Run("finished_operations_expire", func(t *testing.T) {
		sessions := newTestSessions(t, Config{OperationRetention: 100 * time.Millisecond})
		handle := connectToTestServer(t, sessions, server)

		snapshot := runToCompletion(t, sessions, ExecParams{Handle: handle, Command: "echo hi"})

		assert.Eventually(t, func() bool {
			_, ok := sessions.Snapshot(snapshot.OperationID, 0, 0)

			return !ok
		}, 3*time.Second, 20*time.Millisecond)
	})

	t.Run("oldest_finished_operations_are_evicted", func(t *testing.T) {
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

func TestSessions_ClosedSetRefusesEverything(t *testing.T) {
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

func TestSessions_ConnectValidation(t *testing.T) {
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
			_, err := sessions.Connect(context.Background(), tt.params)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantError)
		})
	}
}

func TestSessions_PublicKeyAuthentication(t *testing.T) {
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
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{KeepaliveInterval: 30 * time.Millisecond})
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

	assert.Eventually(t, func() bool {
		_, err := sessions.connection(result.Handle)

		return err != nil
	}, 5*time.Second, 25*time.Millisecond, "an unused connection must not stay open forever")
}

func TestGenerateKeyPair(t *testing.T) {
	tests := []struct {
		name        string
		keyType     KeyType
		wantKeyType string
	}{
		{name: "ed25519", keyType: KeyTypeED25519, wantKeyType: "ssh-ed25519"},
		{name: "ecdsa_p256", keyType: KeyTypeECDSAP256, wantKeyType: "ecdsa-sha2-nistp256"},
		{name: "unspecified_defaults_to_ed25519", keyType: "", wantKeyType: "ssh-ed25519"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
