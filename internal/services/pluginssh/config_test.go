package pluginssh

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_WithDefaults pins the defaults by value rather than against the
// constants: they are what an operator gets when the panel config says nothing,
// so changing one has to be a deliberate edit of this table too.
func TestConfig_WithDefaults(t *testing.T) {
	t.Parallel()

	defaults := Config{
		MaxConnections:        8,
		MaxOperations:         16,
		ConnectTimeout:        30 * time.Second,
		MaxExecTimeout:        30 * time.Minute,
		IdleTimeout:           10 * time.Minute,
		MaxOutputBytes:        1 << 20,
		MaxStdinBytes:         1 << 20,
		OperationRetention:    10 * time.Minute,
		MaxRetainedOperations: 64,
		KeepaliveInterval:     30 * time.Second,
		CompletionCallTimeout: 30 * time.Second,
		BusyRetryDelay:        2 * time.Second,
		BusyRetries:           5,
	}

	negative := Config{
		MaxConnections:        -1,
		MaxOperations:         -1,
		ConnectTimeout:        -time.Second,
		MaxExecTimeout:        -time.Second,
		IdleTimeout:           -time.Second,
		MaxOutputBytes:        -1,
		MaxStdinBytes:         -1,
		OperationRetention:    -time.Second,
		MaxRetainedOperations: -1,
		KeepaliveInterval:     -time.Second,
		CompletionCallTimeout: -time.Second,
		BusyRetryDelay:        -time.Second,
		BusyRetries:           -1,
	}

	configured := Config{
		BlockPrivateIPs:       true,
		AllowedHosts:          []string{"bastion.example.com"},
		MaxConnections:        1,
		MaxOperations:         2,
		ConnectTimeout:        3 * time.Second,
		MaxExecTimeout:        4 * time.Second,
		IdleTimeout:           5 * time.Second,
		MaxOutputBytes:        6,
		MaxStdinBytes:         7,
		OperationRetention:    8 * time.Second,
		MaxRetainedOperations: 9,
		KeepaliveInterval:     10 * time.Second,
		CompletionCallTimeout: 11 * time.Second,
		BusyRetryDelay:        12 * time.Second,
		BusyRetries:           13,
	}

	// The address policy has no default: an operator who did not opt in must
	// not have the block silently turned on by the defaulting pass.
	policyOnly := defaults
	policyOnly.BlockPrivateIPs = true
	policyOnly.AllowedHosts = []string{"bastion.example.com"}

	tests := []struct {
		name string
		cfg  Config
		want Config
	}{
		{
			name: "an_empty_config_gets_every_default",
			cfg:  Config{},
			want: defaults,
		},
		{
			name: "negative_values_are_replaced_by_the_defaults",
			cfg:  negative,
			want: defaults,
		},
		{
			name: "configured_values_are_kept_as_they_are",
			cfg:  configured,
			want: configured,
		},
		{
			name: "the_address_policy_is_carried_through_untouched",
			cfg:  Config{BlockPrivateIPs: true, AllowedHosts: []string{"bastion.example.com"}},
			want: policyOnly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			cfg := tt.cfg

			// ACT
			got := cfg.withDefaults()

			// ASSERT
			assert.Equal(t, tt.want, got)
			assert.Equal(t, got, got.withDefaults(), "defaulting is applied on every service build, it must not drift")
			assert.Equal(t, tt.cfg, cfg, "withDefaults works on a copy, the operator config stays untouched")
		})
	}
}

// TestClampDuration covers the ceiling that keeps a plugin from asking for a
// ten-hour command: Go counts coverage per statement, so the "above the
// ceiling" half of the condition looks covered while never having run.
func TestClampDuration(t *testing.T) {
	t.Parallel()

	const ceiling = 30 * time.Second

	tests := []struct {
		name      string
		requested time.Duration
		want      time.Duration
	}{
		{name: "zero_means_the_panel_default", requested: 0, want: ceiling},
		{name: "a_negative_request_means_the_panel_default", requested: -time.Hour, want: ceiling},
		{name: "a_request_below_the_ceiling_is_granted", requested: 5 * time.Second, want: 5 * time.Second},
		{
			name:      "a_request_just_below_the_ceiling_is_granted",
			requested: ceiling - time.Nanosecond,
			want:      ceiling - time.Nanosecond,
		},
		{name: "a_request_equal_to_the_ceiling_is_granted", requested: ceiling, want: ceiling},
		{name: "a_request_just_above_the_ceiling_is_cut_down", requested: ceiling + time.Nanosecond, want: ceiling},
		{name: "a_request_far_above_the_ceiling_is_cut_down", requested: 10 * time.Hour, want: ceiling},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE, ACT
			got := clampDuration(tt.requested, ceiling)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestClampBytes is the byte-sized twin of TestClampDuration: it stops a plugin
// from pinning a gigabyte of captured output per stream in the panel's memory.
func TestClampBytes(t *testing.T) {
	t.Parallel()

	const ceiling = 1 << 20

	tests := []struct {
		name      string
		requested int
		want      int
	}{
		{name: "zero_means_the_panel_default", requested: 0, want: ceiling},
		{name: "a_negative_request_means_the_panel_default", requested: -4096, want: ceiling},
		{name: "a_request_below_the_ceiling_is_granted", requested: 4096, want: 4096},
		{name: "a_request_just_below_the_ceiling_is_granted", requested: ceiling - 1, want: ceiling - 1},
		{name: "a_request_equal_to_the_ceiling_is_granted", requested: ceiling, want: ceiling},
		{name: "a_request_just_above_the_ceiling_is_cut_down", requested: ceiling + 1, want: ceiling},
		{name: "a_request_far_above_the_ceiling_is_cut_down", requested: 1 << 30, want: ceiling},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE, ACT
			got := clampBytes(tt.requested, ceiling)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSessions_ExecLimitsAreClampedToTheConfig walks the clamp end to end: the
// ceilings are the only thing between a plugin and a command that never ends or
// a buffer that never stops growing, so a request far above them is run against
// a real server rather than only checked as a pure function.
func TestSessions_ExecLimitsAreClampedToTheConfig(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)

	t.Run("a_timeout_above_the_ceiling_is_cut_down", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		sessions := newTestSessions(t, Config{MaxExecTimeout: 200 * time.Millisecond})
		handle := connectToTestServer(t, sessions, server)

		// ACT
		snapshot := runToCompletion(t, sessions, ExecParams{
			Handle:  handle,
			Command: "sleep 30000",
			Timeout: 10 * time.Hour,
		})

		// ASSERT
		assert.Equal(t, StatusTimedOut, snapshot.Status,
			"a plugin cannot buy itself more time than the panel allows")
		assert.Contains(t, snapshot.Error, "timed out")
		assert.Equal(t, int32(-1), snapshot.ExitCode, "a command that never exited has no exit code")
	})

	t.Run("an_output_cap_above_the_ceiling_is_cut_down", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		sessions := newTestSessions(t, Config{MaxOutputBytes: 16})
		handle := connectToTestServer(t, sessions, server)

		// ACT
		snapshot := runToCompletion(t, sessions, ExecParams{
			Handle:         handle,
			Command:        "spam 4096",
			MaxOutputBytes: 1 << 30,
		})

		// ASSERT
		require.Len(t, snapshot.Stdout, 16, "the panel ceiling wins over what the plugin asked for")
		assert.True(t, snapshot.StdoutTruncated)
		assert.Equal(t, uint64(4096), snapshot.StdoutTotal,
			"the plugin still learns how much output it did not get")
	})
}

// TestHostKeyRejectedError: a host function never returns a Go error to the
// guest, the rejection reaches a plugin as text. The message therefore has to
// name the key that answered, so the plugin can report or pin it instead of
// guessing why the connection was refused.
func TestHostKeyRejectedError(t *testing.T) {
	t.Parallel()
	// ARRANGE
	const fingerprint = "SHA256:qXfa5jNvZY0Ee4mnzHb9RCEBpKPXJDdWpTBk7cH2Rc"

	rejected := &HostKeyRejectedError{KeyType: "ssh-ed25519", FingerprintSHA256: fingerprint}

	// ACT
	message := rejected.Error()

	// ASSERT
	assert.Contains(t, message, "ssh-ed25519", "the plugin needs to know what kind of key answered")
	assert.Contains(t, message, fingerprint, "the fingerprint is what a plugin pins to get in next time")
	assert.Contains(t, message, ErrHostKeyRejected.Error(), "the text must still read as a host key failure")

	assert.ErrorIs(t, rejected, ErrHostKeyRejected, "callers classify the failure by the sentinel")
	assert.Equal(t, ErrHostKeyRejected, rejected.Unwrap())

	wrapped := errors.WithMessage(rejected, "connect")
	assert.ErrorIs(t, wrapped, ErrHostKeyRejected, "the sentinel must survive the wrapping at the call site")

	var target *HostKeyRejectedError
	require.ErrorAs(t, wrapped, &target)
	assert.Equal(t, fingerprint, target.FingerprintSHA256, "the offered key must reach the plugin through the wrap")
}
