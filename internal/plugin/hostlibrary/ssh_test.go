package hostlibrary

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/services/pluginssh"
	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sshTestPluginID = 7

// mockSSHSessions records what the adapter asked the engine to do and answers
// with whatever the test staged.
type mockSSHSessions struct {
	mu sync.Mutex

	connectParams []pluginssh.ConnectParams
	connectResult *pluginssh.ConnectResult
	connectErr    error

	execParams  []pluginssh.ExecParams
	operationID string
	startErr    error

	waitErr        error
	waitCalls      int
	snapshot       pluginssh.ExecSnapshot
	snapshotFound  bool
	snapshotOffset [2]uint64

	subscribed []string
	canceled   []string
	cancelErr  error

	disconnected []uint64
	disconnErr   error

	hosts map[uint64]string

	closed bool
}

func (m *mockSSHSessions) Connect(_ context.Context, params pluginssh.ConnectParams) (*pluginssh.ConnectResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connectParams = append(m.connectParams, params)

	return m.connectResult, m.connectErr
}

func (m *mockSSHSessions) Disconnect(handle uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.disconnected = append(m.disconnected, handle)

	return m.disconnErr
}

func (m *mockSSHSessions) StartExec(_ context.Context, params pluginssh.ExecParams) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.execParams = append(m.execParams, params)

	return m.operationID, m.startErr
}

func (m *mockSSHSessions) Cancel(operationID, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.canceled = append(m.canceled, operationID)

	return m.cancelErr
}

func (m *mockSSHSessions) Snapshot(_ string, stdoutOffset, stderrOffset uint64) (pluginssh.ExecSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snapshotOffset = [2]uint64{stdoutOffset, stderrOffset}

	return m.snapshot, m.snapshotFound
}

func (m *mockSSHSessions) WaitCompletion(ctx context.Context, _ string) error {
	m.mu.Lock()
	m.waitCalls++
	err := m.waitErr
	m.mu.Unlock()

	if err != nil {
		// Mirror the engine: an exhausted budget blocks until the caller's
		// context ends rather than failing instantly.
		<-ctx.Done()

		return ctx.Err()
	}

	return nil
}

func (m *mockSSHSessions) SubscribeCompletion(operationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.subscribed = append(m.subscribed, operationID)

	return nil
}

func (m *mockSSHSessions) ConnectionHost(handle uint64) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	host, ok := m.hosts[handle]

	return host, ok
}

func (m *mockSSHSessions) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
}

func (m *mockSSHSessions) recordedExec() []pluginssh.ExecParams {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]pluginssh.ExecParams(nil), m.execParams...)
}

func (m *mockSSHSessions) recordedSubscriptions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]string(nil), m.subscribed...)
}

func newSSHService(t *testing.T, allowed bool, sessions *mockSSHSessions) *SSHServiceImpl {
	t.Helper()

	return NewSSHService(sessions, NewGuard(stubPermissionChecker{allowed: allowed}).For(sshTestPluginID))
}

// TestSSHService_EveryMethodRequiresTheGrant: the ssh grant is the plugin-side
// half of the gate (PLUGIN_SSH_ENABLED is the operator's half), so no entry
// point may skip it — not even key generation, which leaks entropy budget and
// signals intent.
func TestSSHService_EveryMethodRequiresTheGrant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sessions := &mockSSHSessions{}
	svc := newSSHService(t, false, sessions)

	checks := []struct {
		name string
		call func() (bool, *string)
	}{
		{"generate_key_pair", func() (bool, *string) {
			resp, _ := svc.GenerateKeyPair(ctx, &sshsdk.GenerateKeyPairRequest{})

			return resp.Success, resp.Error
		}},
		{"connect", func() (bool, *string) {
			resp, _ := svc.Connect(ctx, &sshsdk.ConnectRequest{Host: "example.com", User: "root"})

			return resp.Success, resp.Error
		}},
		{"disconnect", func() (bool, *string) {
			resp, _ := svc.Disconnect(ctx, &sshsdk.DisconnectRequest{Handle: 1})

			return resp.Success, resp.Error
		}},
		{"exec", func() (bool, *string) {
			resp, _ := svc.Exec(ctx, &sshsdk.ExecRequest{Handle: 1, Command: "id"})

			return resp.Success, resp.Error
		}},
		{"start_exec", func() (bool, *string) {
			resp, _ := svc.StartExec(ctx, &sshsdk.ExecRequest{Handle: 1, Command: "id"})

			return resp.Success, resp.Error
		}},
		{"get_exec_operation", func() (bool, *string) {
			resp, _ := svc.GetExecOperation(ctx, &sshsdk.GetExecOperationRequest{OperationId: "x"})

			return resp.Success, resp.Error
		}},
		{"cancel_exec", func() (bool, *string) {
			resp, _ := svc.CancelExec(ctx, &sshsdk.CancelExecRequest{OperationId: "x"})

			return resp.Success, resp.Error
		}},
		{"write_file", func() (bool, *string) {
			resp, _ := svc.WriteFile(ctx, &sshsdk.WriteFileRequest{Handle: 1, Path: "/tmp/x"})

			return resp.Success, resp.Error
		}},
		{"read_file", func() (bool, *string) {
			resp, _ := svc.ReadFile(ctx, &sshsdk.ReadFileRequest{Handle: 1, Path: "/tmp/x"})

			return resp.Success, resp.Error
		}},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()
			success, errMsg := check.call()

			assert.False(t, success)
			require.NotNil(t, errMsg)
			assert.Contains(t, *errMsg, "plugin permission ssh required")
		})
	}

	assert.Empty(t, sessions.connectParams, "a denied call must not reach the engine")
	assert.Empty(t, sessions.recordedExec())
}

// TestSSHService_RateLimitStopsTheCallBeforeTheEngine covers OWASP
// API4:2023 Unrestricted Resource Consumption: gameap-ssh opens real TCP
// connections to hosts the plugin picks, so a plugin looping on Connect must
// be throttled by the panel, not by the remote side.
func TestSSHService_RateLimitStopsTheCallBeforeTheEngine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sessions := &mockSSHSessions{connectResult: &pluginssh.ConnectResult{Handle: 1}}
	guard := NewGuard(
		stubPermissionChecker{allowed: true},
		WithGuardRateLimits(map[RateClass]RateLimit{RateClassSSH: {RPS: 1, Burst: 1}}),
	)
	svc := NewSSHService(sessions, guard.For(sshTestPluginID))

	first, err := svc.Connect(ctx, &sshsdk.ConnectRequest{Host: "example.com", User: "root"})
	require.NoError(t, err)
	require.True(t, first.Success, first.Error)

	second, err := svc.Connect(ctx, &sshsdk.ConnectRequest{Host: "example.com", User: "root"})

	require.NoError(t, err)
	assert.False(t, second.Success)
	require.NotNil(t, second.Error)
	assert.Contains(t, *second.Error, "rate limited: gameap-ssh")
	assert.Len(t, sessions.connectParams, 1, "a throttled call must not reach the engine")
}

// TestSSHService_AuditsTheOperationWithoutTheSecrets covers OWASP
// API9:2023 Improper Inventory Management: reaching a host outside the node
// inventory has to leave a trail, and that trail must not become the place
// the bootstrap credentials end up.
func TestSSHService_AuditsTheOperationWithoutTheSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	recorder := &auditRecorder{}
	sessions := &mockSSHSessions{
		connectResult: &pluginssh.ConnectResult{Handle: 4, HostKeyType: "ssh-ed25519"},
		operationID:   "op-1",
	}
	svc := NewSSHService(
		sessions,
		NewGuard(stubPermissionChecker{allowed: true}, WithGuardAudit(recorder)).For(sshTestPluginID),
	)

	_, err := svc.Connect(ctx, &sshsdk.ConnectRequest{Host: "example.com", Port: 2222, User: "root"})
	require.NoError(t, err)

	_, err = svc.StartExec(ctx, &sshsdk.ExecRequest{
		Handle:  4,
		Command: "echo hunter2 | passwd --stdin root",
		Stdin:   []byte("hunter2"),
	})
	require.NoError(t, err)

	events := recorder.all()
	require.Len(t, events, 2)

	assert.Equal(t, audit.EventPluginSSHConnect, events[0].Type)
	assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	assert.Equal(t, audit.AuthMethodPlugin, events[0].AuthMethod)
	assert.Equal(t, "ssh_host", events[0].ResourceType)
	assert.Equal(t, "example.com:2222", events[0].ResourceID)

	assert.Equal(t, audit.EventPluginSSHExec, events[1].Type)
	assert.Equal(t, "start_exec", events[1].Action)
	assert.Equal(t, "ssh_session", events[1].ResourceType)
	assert.Equal(t, "4", events[1].ResourceID)

	for _, event := range events {
		for _, attr := range event.Extra {
			assert.NotContains(t, attr.Value.String(), "hunter2",
				"neither the command nor its stdin may reach the audit stream")
		}
	}
}

// TestSSHService_UsesTheDefaultPortInTheAuditRecord: an operator reading the
// trail needs the endpoint that was actually dialed, not the zero the plugin
// left in the request.
func TestSSHService_UsesTheDefaultPortInTheAuditRecord(t *testing.T) {
	t.Parallel()
	recorder := &auditRecorder{}
	sessions := &mockSSHSessions{connectResult: &pluginssh.ConnectResult{Handle: 1}}
	svc := NewSSHService(
		sessions,
		NewGuard(stubPermissionChecker{allowed: true}, WithGuardAudit(recorder)).For(sshTestPluginID),
	)

	_, err := svc.Connect(context.Background(), &sshsdk.ConnectRequest{Host: "example.com", User: "root"})

	require.NoError(t, err)
	events := recorder.all()
	require.Len(t, events, 1)
	assert.Equal(t, "example.com:22", events[0].ResourceID)
}

// TestSSHService_PermissionCheckFailureIsReported keeps a database hiccup from
// silently granting access.
func TestSSHService_PermissionCheckFailureIsReported(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{}
	svc := NewSSHService(sessions,
		NewGuard(stubPermissionChecker{allowed: true, err: assert.AnError}).For(sshTestPluginID))

	resp, err := svc.Connect(context.Background(), &sshsdk.ConnectRequest{Host: "example.com", User: "root"})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "failed to check plugin permission")
}

func TestSSHService_ConnectMapsRequestAndResult(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{connectResult: &pluginssh.ConnectResult{
		Handle:                   42,
		HostKeyFingerprintSHA256: "SHA256:abc",
		HostKeyType:              "ssh-ed25519",
		ServerVersion:            "SSH-2.0-OpenSSH_9.6",
	}}
	svc := newSSHService(t, true, sessions)

	resp, err := svc.Connect(context.Background(), &sshsdk.ConnectRequest{
		Host:             "203.0.113.10",
		Port:             2222,
		User:             "root",
		Auth:             &sshsdk.SSHAuth{PrivateKeyPem: "PEM", Passphrase: "pp"},
		HostKey:          &sshsdk.HostKeyPolicy{AcceptAny: true},
		ConnectTimeoutMs: 5000,
		IdleTimeoutMs:    60000,
	})

	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)
	assert.Equal(t, uint64(42), resp.Handle)
	assert.Equal(t, "SHA256:abc", resp.HostKeyFingerprintSha256)
	assert.Equal(t, "SSH-2.0-OpenSSH_9.6", resp.ServerVersion)

	require.Len(t, sessions.connectParams, 1)
	params := sessions.connectParams[0]
	assert.Equal(t, "203.0.113.10", params.Host)
	assert.Equal(t, uint32(2222), params.Port)
	assert.Equal(t, "PEM", params.PrivateKeyPEM)
	assert.Equal(t, "pp", params.Passphrase)
	assert.True(t, params.HostKey.AcceptAny)
	assert.Equal(t, 5*time.Second, params.ConnectTimeout)
	assert.Equal(t, time.Minute, params.IdleTimeout)
}

// TestSSHService_ConnectReportsRejectedHostKey: after a mismatch the plugin
// needs the observed key to decide whether to pin it or raise an alarm.
func TestSSHService_ConnectReportsRejectedHostKey(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{connectErr: &pluginssh.HostKeyRejectedError{
		KeyType:           "ssh-ed25519",
		FingerprintSHA256: "SHA256:observed",
	}}
	svc := newSSHService(t, true, sessions)

	resp, err := svc.Connect(context.Background(), &sshsdk.ConnectRequest{Host: "h", User: "root"})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "SHA256:observed", resp.HostKeyFingerprintSha256)
	assert.Equal(t, "ssh-ed25519", resp.HostKeyType)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "host key verification failed")
}

func TestSSHService_ExecCompletesWithinBudget(t *testing.T) {
	t.Parallel()
	started := time.Now()
	sessions := &mockSSHSessions{
		operationID:   "op-1",
		snapshotFound: true,
		snapshot: pluginssh.ExecSnapshot{
			OperationID: "op-1",
			Handle:      9,
			Status:      pluginssh.StatusCompleted,
			ExitCode:    0,
			Stdout:      []byte("ok"),
			StartedAt:   started,
			FinishedAt:  started.Add(time.Second),
		},
	}
	svc := newSSHService(t, true, sessions)

	resp, err := svc.Exec(context.Background(), &sshsdk.ExecRequest{
		Handle:  9,
		Command: "bash -s",
		Stdin:   []byte("#!/bin/bash"),
	})

	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)
	assert.True(t, resp.Completed)
	assert.True(t, resp.OpSuccess)
	assert.Equal(t, "ok", string(resp.Stdout))
	assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_COMPLETED, resp.Status)
	assert.Empty(t, sessions.recordedSubscriptions(),
		"a command that finished in time needs no completion callback")

	require.Len(t, sessions.recordedExec(), 1)
	assert.False(t, sessions.recordedExec()[0].NotifyCompletion)
	assert.Equal(t, []byte("#!/bin/bash"), sessions.recordedExec()[0].Stdin)
}

// TestSSHService_ExecSubscribesWhenTheBudgetRunsOut is the long-bootstrap
// path: the command keeps running and the plugin is told when it ends.
func TestSSHService_ExecSubscribesWhenTheBudgetRunsOut(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{operationID: "op-2", waitErr: context.DeadlineExceeded}
	svc := newSSHService(t, true, sessions)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := svc.Exec(ctx, &sshsdk.ExecRequest{Handle: 9, Command: "long", TimeoutSeconds: 1})

	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)
	assert.False(t, resp.Completed, "an unfinished command must not look finished")
	assert.Equal(t, "op-2", resp.OperationId)
	assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_RUNNING, resp.Status)
	assert.Equal(t, int32(-1), resp.ExitCode)
	assert.Equal(t, []string{"op-2"}, sessions.recordedSubscriptions())
}

// TestSSHService_ExecAnswersWithoutWaitingNearTheDeadline: waiting past the
// guest deadline would close the plugin's wasm module.
func TestSSHService_ExecAnswersWithoutWaitingNearTheDeadline(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{operationID: "op-3"}
	svc := newSSHService(t, true, sessions)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	resp, err := svc.Exec(ctx, &sshsdk.ExecRequest{Handle: 9, Command: "long"})

	require.NoError(t, err)
	assert.False(t, resp.Completed)
	assert.Equal(t, 0, sessions.waitCalls, "no wait may start inside the deadline grace")
	assert.Equal(t, []string{"op-3"}, sessions.recordedSubscriptions())
}

func TestSSHService_StartExecRequestsCompletionCallback(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{operationID: "op-4"}
	svc := newSSHService(t, true, sessions)

	resp, err := svc.StartExec(context.Background(), &sshsdk.ExecRequest{Handle: 9, Command: "long"})

	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)
	assert.Equal(t, "op-4", resp.OperationId)
	require.Len(t, sessions.recordedExec(), 1)
	assert.True(t, sessions.recordedExec()[0].NotifyCompletion)
}

func TestSSHService_GetExecOperation(t *testing.T) {
	t.Parallel()
	t.Run("unknown_operation_is_not_an_error", func(t *testing.T) {
		t.Parallel()
		svc := newSSHService(t, true, &mockSSHSessions{})

		resp, err := svc.GetExecOperation(context.Background(),
			&sshsdk.GetExecOperationRequest{OperationId: "gone"})

		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.False(t, resp.Found)
	})

	t.Run("passes_offsets_and_maps_the_snapshot", func(t *testing.T) {
		t.Parallel()
		sessions := &mockSSHSessions{
			snapshotFound: true,
			snapshot: pluginssh.ExecSnapshot{
				OperationID:      "op-5",
				Handle:           9,
				Status:           pluginssh.StatusTimedOut,
				ExitCode:         -1,
				Error:            "timed out",
				Stdout:           []byte("tail"),
				StdoutNextOffset: 14,
				StdoutTotal:      99,
				StdoutTruncated:  true,
			},
		}
		svc := newSSHService(t, true, sessions)

		resp, err := svc.GetExecOperation(context.Background(), &sshsdk.GetExecOperationRequest{
			OperationId:  "op-5",
			StdoutOffset: 10,
			StderrOffset: 3,
		})

		require.NoError(t, err)
		require.True(t, resp.Found)
		assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_TIMED_OUT, resp.Status)
		assert.Equal(t, "tail", string(resp.Stdout))
		assert.Equal(t, uint64(14), resp.StdoutNextOffset)
		assert.Equal(t, uint64(99), resp.StdoutTotalBytes)
		assert.True(t, resp.StdoutTruncated)
		assert.False(t, resp.OpSuccess)
		require.NotNil(t, resp.OpError)
		assert.Equal(t, "timed out", *resp.OpError)
		assert.Equal(t, [2]uint64{10, 3}, sessions.snapshotOffset)
		assert.Equal(t, 0, sessions.waitCalls, "wait_ms=0 must answer immediately")
	})
}

func TestSSHService_WriteFile(t *testing.T) {
	t.Parallel()
	t.Run("pipes_content_through_cat_and_applies_mode", func(t *testing.T) {
		t.Parallel()
		sessions := &mockSSHSessions{
			operationID:   "op-6",
			snapshotFound: true,
			snapshot:      pluginssh.ExecSnapshot{Status: pluginssh.StatusCompleted, ExitCode: 0},
		}
		svc := newSSHService(t, true, sessions)

		resp, err := svc.WriteFile(context.Background(), &sshsdk.WriteFileRequest{
			Handle:  9,
			Path:    "/tmp/install script.sh",
			Content: []byte("#!/bin/bash"),
			Mode:    0o755,
		})

		require.NoError(t, err)
		require.True(t, resp.Success, resp.Error)

		require.Len(t, sessions.recordedExec(), 1)
		command := sessions.recordedExec()[0].Command
		temp := tempPathFromCommand(t, command)
		assert.True(t, strings.HasPrefix(temp, "/tmp/install script.sh"+writeFileTempSuffix),
			"temp file must be a sibling of the target, got %q", temp)
		assert.Equal(t, renderWriteFileCommand("/tmp/install script.sh", temp, 0o755), command)
		assert.Equal(t, []byte("#!/bin/bash"), sessions.recordedExec()[0].Stdin)
	})

	t.Run("remote_failure_is_reported_with_stderr", func(t *testing.T) {
		t.Parallel()
		sessions := &mockSSHSessions{
			operationID:   "op-7",
			snapshotFound: true,
			snapshot: pluginssh.ExecSnapshot{
				Status:   pluginssh.StatusCompleted,
				ExitCode: 1,
				Stderr:   []byte("permission denied"),
			},
		}
		svc := newSSHService(t, true, sessions)

		resp, err := svc.WriteFile(context.Background(), &sshsdk.WriteFileRequest{
			Handle: 9, Path: "/etc/shadow", Content: []byte("x"),
		})

		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "permission denied", resp.Stderr)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "exited with code 1")
	})

	t.Run("empty_path_is_refused", func(t *testing.T) {
		t.Parallel()
		sessions := &mockSSHSessions{}
		svc := newSSHService(t, true, sessions)

		resp, err := svc.WriteFile(context.Background(), &sshsdk.WriteFileRequest{Handle: 9, Path: "  "})

		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Empty(t, sessions.recordedExec())
	})

	t.Run("invalid_mode_is_refused_before_the_engine", func(t *testing.T) {
		t.Parallel()
		sessions := &mockSSHSessions{}
		svc := newSSHService(t, true, sessions)

		resp, err := svc.WriteFile(context.Background(), &sshsdk.WriteFileRequest{
			Handle:  9,
			Path:    "/etc/ssh/key",
			Content: []byte("secret"),
			Mode:    755,
		})

		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "0o644")
		assert.Empty(t, sessions.recordedExec())
	})
}

func TestSSHService_ReadFile(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{
		operationID:   "op-8",
		snapshotFound: true,
		snapshot: pluginssh.ExecSnapshot{
			Status:          pluginssh.StatusCompleted,
			ExitCode:        0,
			Stdout:          []byte("file body"),
			StdoutTruncated: true,
		},
	}
	svc := newSSHService(t, true, sessions)

	resp, err := svc.ReadFile(context.Background(), &sshsdk.ReadFileRequest{
		Handle:   9,
		Path:     "/etc/gameap-daemon/gameap-daemon.yaml",
		MaxBytes: 4096,
	})

	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)
	assert.Equal(t, "file body", string(resp.Content))
	assert.True(t, resp.Truncated)

	require.Len(t, sessions.recordedExec(), 1)
	assert.Equal(t, "cat -- '/etc/gameap-daemon/gameap-daemon.yaml'", sessions.recordedExec()[0].Command)
	assert.Equal(t, 4096, sessions.recordedExec()[0].MaxOutputBytes)
}

func TestSSHService_CancelAndDisconnect(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{}
	svc := newSSHService(t, true, sessions)
	ctx := context.Background()

	cancelResp, err := svc.CancelExec(ctx, &sshsdk.CancelExecRequest{OperationId: "op-9", Reason: "scale down"})
	require.NoError(t, err)
	assert.True(t, cancelResp.Success)
	assert.Equal(t, []string{"op-9"}, sessions.canceled)

	disconnectResp, err := svc.Disconnect(ctx, &sshsdk.DisconnectRequest{Handle: 5})
	require.NoError(t, err)
	assert.True(t, disconnectResp.Success)
	assert.Equal(t, []uint64{5}, sessions.disconnected)

	sessions.cancelErr = pluginssh.ErrOperationFinished
	failed, err := svc.CancelExec(ctx, &sshsdk.CancelExecRequest{OperationId: "op-9"})
	require.NoError(t, err)
	assert.False(t, failed.Success)
	require.NotNil(t, failed.Error)
	assert.Contains(t, *failed.Error, "already finished")
}

func TestSSHService_GenerateKeyPair(t *testing.T) {
	t.Parallel()
	svc := newSSHService(t, true, &mockSSHSessions{})

	resp, err := svc.GenerateKeyPair(context.Background(), &sshsdk.GenerateKeyPairRequest{
		Type:    sshsdk.KeyType_KEY_TYPE_ED25519,
		Comment: "autoscale@panel",
	})

	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)
	assert.Contains(t, resp.PrivateKeyPem, "OPENSSH PRIVATE KEY")
	assert.Contains(t, resp.PublicKey, "ssh-ed25519 ")
	assert.Contains(t, resp.PublicKey, "autoscale@panel")
	assert.Equal(t, "ssh-ed25519", resp.KeyType)
	assert.Contains(t, resp.FingerprintSha256, "SHA256:")
}

// TestSSHHostLibrary_CloseReleasesSessions is what keeps an unloaded plugin
// from leaving SSH connections open on the panel.
func TestSSHHostLibrary_CloseReleasesSessions(t *testing.T) {
	t.Parallel()
	sessions := &mockSSHSessions{}
	factory := NewSSHHostLibraryFactory(
		stubSSHOpener{sessions: sessions},
		NewGuard(stubPermissionChecker{allowed: true}),
	)

	lib := factory.Create(sshTestPluginID)

	impl, ok := lib.(*SSHHostLibrary)
	require.True(t, ok)
	assert.Equal(t, uint64(sshTestPluginID), impl.impl.guard.PluginID())

	require.NoError(t, impl.Close(context.Background()))
	assert.True(t, sessions.closed)
}

type stubSSHOpener struct {
	sessions SSHSessionManager
}

func (o stubSSHOpener) NewSessions(uint64) SSHSessionManager { return o.sessions }

func TestSyncWaitBudget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		ctx       func(t *testing.T) context.Context
		requested time.Duration
		assert    func(t *testing.T, budget time.Duration)
	}{
		{
			name:      "no_deadline_and_no_request_uses_the_default",
			ctx:       func(*testing.T) context.Context { return context.Background() },
			requested: 0,
			assert: func(t *testing.T, budget time.Duration) {
				t.Helper()
				assert.Equal(t, defaultSyncWaitBudget, budget)
			},
		},
		{
			name:      "requested_below_the_remaining_time_is_kept",
			ctx:       deadlineCtx(time.Minute),
			requested: 5 * time.Second,
			assert: func(t *testing.T, budget time.Duration) {
				t.Helper()
				assert.Equal(t, 5*time.Second, budget)
			},
		},
		{
			name:      "guest_deadline_caps_the_request",
			ctx:       deadlineCtx(10 * time.Second),
			requested: time.Hour,
			assert: func(t *testing.T, budget time.Duration) {
				t.Helper()
				assert.Less(t, budget, 10*time.Second)
				assert.Greater(t, budget, 6*time.Second)
			},
		},
		{
			name:      "imminent_deadline_yields_no_budget",
			ctx:       deadlineCtx(time.Second),
			requested: time.Hour,
			assert: func(t *testing.T, budget time.Duration) {
				t.Helper()
				assert.LessOrEqual(t, budget, time.Duration(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.assert(t, syncWaitBudget(tt.ctx(t), tt.requested))
		})
	}
}

func deadlineCtx(in time.Duration) func(t *testing.T) context.Context {
	return func(t *testing.T) context.Context {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), in)
		t.Cleanup(cancel)

		return ctx
	}
}

// auditAttrValue extracts one attribute from an audit event's extras.
func auditAttrValue(t *testing.T, event audit.Event, key string) (string, bool) {
	t.Helper()

	for _, attr := range event.Extra {
		if attr.Key == key {
			return attr.Value.String(), true
		}
	}

	return "", false
}

// TestSSHService_GenerateKeyPairIsAuditedWithoutKeyMaterial covers OWASP
// API9:2023 Improper Inventory Management: key generation mints a credential
// the panel never stores, so the fingerprint in the audit trail is the only
// way to tie an authorized_keys line found on a machine back to the plugin —
// and the trail must not become the place the key material ends up.
func TestSSHService_GenerateKeyPairIsAuditedWithoutKeyMaterial(t *testing.T) {
	t.Parallel()
	recorder := &auditRecorder{}
	svc := NewSSHService(
		&mockSSHSessions{},
		NewGuard(stubPermissionChecker{allowed: true}, WithGuardAudit(recorder)).For(sshTestPluginID),
	)

	resp, err := svc.GenerateKeyPair(context.Background(), &sshsdk.GenerateKeyPairRequest{
		Type:    sshsdk.KeyType_KEY_TYPE_ED25519,
		Comment: "autoscale@panel",
	})
	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)

	events := recorder.all()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventPluginSSHKey, events[0].Type)
	assert.Equal(t, "generate_key_pair", events[0].Action)
	assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	assert.Equal(t, "ssh_key", events[0].ResourceType)
	assert.Equal(t, resp.FingerprintSha256, events[0].ResourceID)

	keyType, ok := auditAttrValue(t, events[0], "key_type")
	require.True(t, ok)
	assert.Equal(t, resp.KeyType, keyType)

	publicKeyBody := strings.Fields(resp.PublicKey)[1]
	assert.NotContains(t, events[0].ResourceID, publicKeyBody)
	for _, attr := range events[0].Extra {
		assert.NotContains(t, attr.Value.String(), "PRIVATE KEY",
			"key material must never enter the audit stream")
		assert.NotContains(t, attr.Value.String(), publicKeyBody,
			"the fingerprint stands in for the key; the key itself stays out")
	}
}

// TestSSHService_ConnectAuditLinksHandleAndAddress covers OWASP API9:2023
// Improper Inventory Management: commands are audited by session handle, so
// the connect record must carry that handle — and the numeric address that was
// actually dialed, which with an allowlisted name can differ from the request.
func TestSSHService_ConnectAuditLinksHandleAndAddress(t *testing.T) {
	t.Parallel()
	recorder := &auditRecorder{}
	sessions := &mockSSHSessions{
		connectResult: &pluginssh.ConnectResult{
			Handle:          7,
			Address:         "203.0.113.9:22",
			HostKeyType:     "ssh-ed25519",
			HostKeyVerified: true,
		},
	}
	svc := NewSSHService(
		sessions,
		NewGuard(stubPermissionChecker{allowed: true}, WithGuardAudit(recorder)).For(sshTestPluginID),
	)

	_, err := svc.Connect(context.Background(), &sshsdk.ConnectRequest{Host: "node.internal", User: "root"})
	require.NoError(t, err)

	events := recorder.all()
	require.Len(t, events, 1)

	handle, ok := auditAttrValue(t, events[0], "handle")
	require.True(t, ok, "the connect record must name the handle commands are audited under")
	assert.Equal(t, "7", handle)

	address, ok := auditAttrValue(t, events[0], "address")
	require.True(t, ok, "the record must name the address that was actually dialed")
	assert.Equal(t, "203.0.113.9:22", address)

	verified, ok := auditAttrValue(t, events[0], "host_key_verified")
	require.True(t, ok)
	assert.Equal(t, "true", verified)
}

// TestSSHService_ExecAuditNamesTheHost covers OWASP API9:2023 Improper
// Inventory Management: without the host in the command and file records, the
// audit stream cannot say which machine a command ran on.
func TestSSHService_ExecAuditNamesTheHost(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	recorder := &auditRecorder{}
	sessions := &mockSSHSessions{
		operationID:   "op-1",
		hosts:         map[uint64]string{4: "build-42.example.com"},
		snapshotFound: true,
		snapshot:      pluginssh.ExecSnapshot{Status: pluginssh.StatusCompleted, ExitCode: 0},
	}
	svc := NewSSHService(
		sessions,
		NewGuard(stubPermissionChecker{allowed: true}, WithGuardAudit(recorder)).For(sshTestPluginID),
	)

	_, err := svc.StartExec(ctx, &sshsdk.ExecRequest{Handle: 4, Command: "uptime"})
	require.NoError(t, err)

	_, err = svc.WriteFile(ctx, &sshsdk.WriteFileRequest{Handle: 4, Path: "/etc/motd", Content: []byte("hi")})
	require.NoError(t, err)

	_, err = svc.StartExec(ctx, &sshsdk.ExecRequest{Handle: 9, Command: "uptime"})
	require.NoError(t, err)

	events := recorder.all()
	require.Len(t, events, 3)

	host, ok := auditAttrValue(t, events[0], "host")
	require.True(t, ok, "the command record must name the machine it ran on")
	assert.Equal(t, "build-42.example.com", host)

	host, ok = auditAttrValue(t, events[1], "host")
	require.True(t, ok, "the file record must name the machine it touched")
	assert.Equal(t, "build-42.example.com", host)

	_, ok = auditAttrValue(t, events[2], "host")
	assert.False(t, ok, "an unknown handle has no host to report")
}
