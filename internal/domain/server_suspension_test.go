package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var suspendedAt = time.Date(2026, 9, 26, 10, 15, 0, 0, time.UTC)

func TestServer_Suspension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server domain.Server
		want   *domain.ServerSuspension
	}{
		{
			name:   "not_suspended",
			server: domain.Server{},
			want:   nil,
		},
		{
			name: "record_of_a_server_no_longer_suspended_is_ignored",
			server: domain.Server{Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			}},
			want: nil,
		},
		{
			name:   "blocked_without_record",
			server: domain.Server{Blocked: true},
			want:   &domain.ServerSuspension{},
		},
		{
			name: "full_record",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			}},
			want: &domain.ServerSuspension{Since: new(suspendedAt), Reason: "unpaid"},
		},
		{
			name: "record_without_reason",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z"},
			}},
			want: &domain.ServerSuspension{Since: new(suspendedAt)},
		},
		{
			name: "unparsable_date_counts_as_unknown",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "yesterday", "reason": "unpaid"},
			}},
			want: &domain.ServerSuspension{Reason: "unpaid"},
		},
		{
			name: "values_of_wrong_type_are_ignored",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": map[string]any{"since": 1758881700, "reason": 42},
			}},
			want: &domain.ServerSuspension{},
		},
		{
			name: "record_that_is_not_an_object",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": "since yesterday",
			}},
			want: &domain.ServerSuspension{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.server.Suspension())
		})
	}
}

func TestServer_IsSuspended(t *testing.T) {
	t.Parallel()

	assert.False(t, (&domain.Server{}).IsSuspended())
	assert.True(t, (&domain.Server{Blocked: true}).IsSuspended())
}

func TestServer_Suspend(t *testing.T) {
	t.Parallel()

	later := suspendedAt.Add(48 * time.Hour)

	tests := []struct {
		name         string
		server       domain.Server
		now          time.Time
		reason       *string
		wantSince    *time.Time
		wantReason   string
		wantMetadata domain.Metadata
	}{
		{
			name:       "first_suspension_records_the_date",
			server:     domain.Server{},
			now:        suspendedAt,
			wantSince:  new(suspendedAt),
			wantReason: "",
			wantMetadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z"},
			},
		},
		{
			name:       "first_suspension_with_reason_keeps_other_keys",
			server:     domain.Server{Metadata: domain.Metadata{"public_ip": "203.0.113.10"}},
			now:        suspendedAt,
			reason:     new("unpaid"),
			wantSince:  new(suspendedAt),
			wantReason: "unpaid",
			wantMetadata: domain.Metadata{
				"public_ip":  "203.0.113.10",
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			},
		},
		{
			name:       "date_is_stored_in_utc_to_the_second",
			server:     domain.Server{},
			now:        time.Date(2026, 9, 26, 13, 15, 0, 999, time.FixedZone("MSK", 3*60*60)),
			wantSince:  new(suspendedAt),
			wantReason: "",
			wantMetadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z"},
			},
		},
		{
			name: "suspending_again_keeps_the_date_and_the_reason",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			}},
			now:        later,
			wantSince:  new(suspendedAt),
			wantReason: "unpaid",
			wantMetadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			},
		},
		{
			name: "suspending_again_replaces_the_reason",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			}},
			now:        later,
			reason:     new("abuse report"),
			wantSince:  new(suspendedAt),
			wantReason: "abuse report",
			wantMetadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "abuse report"},
			},
		},
		{
			name: "empty_reason_clears_it",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			}},
			now:        later,
			reason:     new(""),
			wantSince:  new(suspendedAt),
			wantReason: "",
			wantMetadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z"},
			},
		},
		{
			name:       "block_without_record_does_not_get_a_made_up_date",
			server:     domain.Server{Blocked: true},
			now:        later,
			reason:     new("unpaid"),
			wantSince:  nil,
			wantReason: "unpaid",
			wantMetadata: domain.Metadata{
				"suspension": map[string]any{"reason": "unpaid"},
			},
		},
		{
			name: "stale_record_of_an_unsuspended_server_is_replaced",
			server: domain.Server{Metadata: domain.Metadata{
				"suspension": map[string]any{"since": "2020-01-01T00:00:00Z", "reason": "old"},
			}},
			now:        suspendedAt,
			wantSince:  new(suspendedAt),
			wantReason: "",
			wantMetadata: domain.Metadata{
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := tt.server

			server.Suspend(tt.now, tt.reason)

			assert.True(t, server.Blocked)
			assert.Equal(t, tt.wantMetadata, server.Metadata)

			suspension := server.Suspension()
			require.NotNil(t, suspension)
			assert.Equal(t, tt.wantSince, suspension.Since)
			assert.Equal(t, tt.wantReason, suspension.Reason)
		})
	}
}

func TestServer_Suspend_DoesNotMutateSharedMetadata(t *testing.T) {
	t.Parallel()

	shared := domain.Metadata{"public_ip": "203.0.113.10"}
	server := domain.Server{Metadata: shared}

	server.Suspend(suspendedAt, new("unpaid"))

	assert.Equal(t, domain.Metadata{"public_ip": "203.0.113.10"}, shared)
}

func TestServer_Suspend_RecordSurvivesJSONRoundTrip(t *testing.T) {
	t.Parallel()

	server := domain.Server{}
	server.Suspend(suspendedAt, new("unpaid"))

	stored, err := server.Metadata.Value()
	require.NoError(t, err)

	restored := domain.Server{Blocked: true}
	require.NoError(t, restored.Metadata.Scan(stored))

	assert.Equal(t, &domain.ServerSuspension{Since: new(suspendedAt), Reason: "unpaid"}, restored.Suspension())
}

func TestServer_Unsuspend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		server       domain.Server
		wantMetadata domain.Metadata
	}{
		{
			name: "drops_the_record_and_keeps_other_keys",
			server: domain.Server{Blocked: true, Metadata: domain.Metadata{
				"public_ip":  "203.0.113.10",
				"suspension": map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"},
			}},
			wantMetadata: domain.Metadata{"public_ip": "203.0.113.10"},
		},
		{
			name:         "block_without_record",
			server:       domain.Server{Blocked: true, Metadata: domain.Metadata{"public_ip": "203.0.113.10"}},
			wantMetadata: domain.Metadata{"public_ip": "203.0.113.10"},
		},
		{
			name:         "not_suspended",
			server:       domain.Server{},
			wantMetadata: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := tt.server

			server.Unsuspend()

			assert.False(t, server.Blocked)
			assert.Equal(t, tt.wantMetadata, server.Metadata)
			assert.Nil(t, server.Suspension())
		})
	}
}

func TestServer_Unsuspend_DoesNotMutateSharedMetadata(t *testing.T) {
	t.Parallel()

	shared := domain.Metadata{"suspension": map[string]any{"reason": "unpaid"}}
	server := domain.Server{Blocked: true, Metadata: shared}

	server.Unsuspend()

	assert.Contains(t, shared, "suspension")
}

func TestServer_ReplaceMetadata(t *testing.T) {
	t.Parallel()

	record := map[string]any{"since": "2026-09-26T10:15:00Z", "reason": "unpaid"}

	tests := []struct {
		name     string
		current  domain.Metadata
		incoming domain.Metadata
		want     domain.Metadata
	}{
		{
			name:     "no_record_anywhere",
			current:  domain.Metadata{"docker_image": "old"},
			incoming: domain.Metadata{"docker_image": "new"},
			want:     domain.Metadata{"docker_image": "new"},
		},
		{
			name:     "record_is_kept",
			current:  domain.Metadata{"docker_image": "old", "suspension": record},
			incoming: domain.Metadata{"docker_image": "new"},
			want:     domain.Metadata{"docker_image": "new", "suspension": record},
		},
		{
			name:     "record_is_kept_when_everything_else_is_removed",
			current:  domain.Metadata{"docker_image": "old", "suspension": record},
			incoming: domain.Metadata{},
			want:     domain.Metadata{"suspension": record},
		},
		{
			name:     "sent_record_does_not_replace_the_stored_one",
			current:  domain.Metadata{"suspension": record},
			incoming: domain.Metadata{"suspension": map[string]any{"reason": "forged"}},
			want:     domain.Metadata{"suspension": record},
		},
		{
			name:     "sent_record_is_dropped_when_none_is_stored",
			current:  nil,
			incoming: domain.Metadata{"public_ip": "203.0.113.10", "suspension": map[string]any{"reason": "forged"}},
			want:     domain.Metadata{"public_ip": "203.0.113.10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := domain.Server{Metadata: tt.current}

			server.ReplaceMetadata(tt.incoming)

			assert.Equal(t, tt.want, server.Metadata)
		})
	}
}

func TestServer_PublicMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata domain.Metadata
		want     domain.Metadata
	}{
		{
			name:     "nil",
			metadata: nil,
			want:     nil,
		},
		{
			name:     "empty",
			metadata: domain.Metadata{},
			want:     domain.Metadata{},
		},
		{
			name:     "without_record",
			metadata: domain.Metadata{"public_ip": "203.0.113.10"},
			want:     domain.Metadata{"public_ip": "203.0.113.10"},
		},
		{
			name: "record_is_hidden",
			metadata: domain.Metadata{
				"public_ip":  "203.0.113.10",
				"suspension": map[string]any{"reason": "unpaid"},
			},
			want: domain.Metadata{"public_ip": "203.0.113.10"},
		},
		{
			name:     "only_the_record",
			metadata: domain.Metadata{"suspension": map[string]any{"reason": "unpaid"}},
			want:     domain.Metadata{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := domain.Server{Blocked: true, Metadata: tt.metadata}

			assert.Equal(t, tt.want, server.PublicMetadata())
			assert.Equal(t, tt.metadata, server.Metadata, "the stored metadata must stay untouched")
		})
	}
}

func TestServer_MayBeRunning(t *testing.T) {
	t.Parallel()

	fresh := time.Now().Add(-30 * time.Second)
	stale := time.Now().Add(-10 * time.Minute)

	tests := []struct {
		name   string
		server domain.Server
		want   bool
	}{
		{
			name:   "not_installed",
			server: domain.Server{Installed: domain.ServerInstalledStatusNotInstalled, ProcessActive: true},
			want:   false,
		},
		{
			name: "installation_in_progress",
			server: domain.Server{
				Installed:     domain.ServerInstalledStatusInstallationInProg,
				ProcessActive: true,
			},
			want: false,
		},
		{
			name: "reported_running",
			server: domain.Server{
				Installed:        domain.ServerInstalledStatusInstalled,
				ProcessActive:    true,
				LastProcessCheck: new(fresh),
			},
			want: true,
		},
		{
			name: "stale_report_of_running",
			server: domain.Server{
				Installed:        domain.ServerInstalledStatusInstalled,
				ProcessActive:    true,
				LastProcessCheck: new(stale),
			},
			want: true,
		},
		{
			name: "freshly_reported_down",
			server: domain.Server{
				Installed:        domain.ServerInstalledStatusInstalled,
				LastProcessCheck: new(fresh),
			},
			want: false,
		},
		{
			name: "stale_report_of_down",
			server: domain.Server{
				Installed:        domain.ServerInstalledStatusInstalled,
				LastProcessCheck: new(stale),
			},
			want: true,
		},
		{
			name:   "never_checked",
			server: domain.Server{Installed: domain.ServerInstalledStatusInstalled},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.server.MayBeRunning())
		})
	}
}

func TestDaemonTaskType_RefusedWhileSuspended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		taskType domain.DaemonTaskType
		want     bool
	}{
		{domain.DaemonTaskTypeServerStart, true},
		{domain.DaemonTaskTypeServerRestart, true},
		{domain.DaemonTaskTypeServerUpdate, true},
		{domain.DaemonTaskTypeServerInstall, true},
		{domain.DaemonTaskTypeServerStop, false},
		{domain.DaemonTaskTypeServerDelete, false},
		{domain.DaemonTaskTypeServerMove, false},
		{domain.DaemonTaskTypeCmdExec, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.taskType), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.taskType.RefusedWhileSuspended())
		})
	}
}

func TestServer_Suspend_MetadataIsValidJSON(t *testing.T) {
	t.Parallel()

	server := domain.Server{}
	server.Suspend(suspendedAt, new("Invoice #1042 is overdue"))

	raw, err := json.Marshal(server.Metadata)
	require.NoError(t, err)

	assert.JSONEq(t, `{"suspension":{"since":"2026-09-26T10:15:00Z","reason":"Invoice #1042 is overdue"}}`, string(raw))
}
