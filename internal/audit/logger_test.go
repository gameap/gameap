// API Security Tests for OWASP API Security Top 10:2023.
// Category: API8:2023 — Security Misconfiguration.
//
// Insufficient or malformed security logging is a detective-control gap
// (CWE-778 / OWASP ASVS 4.0.3 §7.1, §7.2). These tests pin the audit-record
// schema, severity mapping, field omission and request-context enrichment so
// a regression cannot silently degrade the audit trail.
//
// Reference: https://owasp.org/API-Security/editions/2023/

package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/gameap/gameap/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeRecords splits a JSON-handler buffer into one decoded map per line.
func decodeRecords(t *testing.T, raw []byte) []map[string]any {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(raw))

	var records []map[string]any
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		records = append(records, rec)
	}

	return records
}

// newBufferLogger returns an audit.Logger writing JSON records into buf at
// debug level (so even INFO records are captured).
func newBufferLogger(buf *bytes.Buffer) audit.Logger {
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	return audit.NewLogger(slog.New(h))
}

// TestSlogLogger_Record_SchemaAndSeverity covers OWASP API8:2023.
// Verifies the stable wire schema, the outcome→level mapping and that
// zero-valued fields are omitted from the emitted record.
func TestSlogLogger_Record_SchemaAndSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		event       audit.Event
		wantLevel   string
		wantFields  map[string]any
		absentField []string
	}{
		{
			name: "success_event_is_info_with_full_schema",
			event: audit.Event{
				Type:         audit.EventPATCreate,
				Category:     audit.CategoryTokenOp,
				Outcome:      audit.OutcomeSuccess,
				ActorID:      7,
				ActorLogin:   "alice",
				AuthMethod:   audit.AuthMethodSession,
				ResourceType: "token",
				ResourceID:   "42",
				Action:       "create",
			},
			wantLevel: "INFO",
			wantFields: map[string]any{
				"msg":           "audit",
				"component":     "audit",
				"event_type":    string(audit.EventPATCreate),
				"category":      string(audit.CategoryTokenOp),
				"outcome":       string(audit.OutcomeSuccess),
				"actor_id":      float64(7),
				"actor_login":   "alice",
				"auth_method":   string(audit.AuthMethodSession),
				"resource_type": "token",
				"resource_id":   "42",
				"action":        "create",
			},
			absentField: []string{"reason", "request_id", "ip", "user_agent", "http_method", "path"},
		},
		{
			name: "failure_event_is_warn",
			event: audit.Event{
				Type:     audit.EventAuthTokenRejected,
				Category: audit.CategoryAuthentication,
				Outcome:  audit.OutcomeFailure,
				Reason:   "missing_token",
			},
			wantLevel: "WARN",
			wantFields: map[string]any{
				"event_type": string(audit.EventAuthTokenRejected),
				"outcome":    string(audit.OutcomeFailure),
				"reason":     "missing_token",
			},
			absentField: []string{"actor_id", "actor_login", "resource_type", "resource_id", "action"},
		},
		{
			name: "denied_event_is_warn",
			event: audit.Event{
				Type:     audit.EventAccessDenied,
				Category: audit.CategoryAuthorization,
				Outcome:  audit.OutcomeDenied,
				Reason:   "admin_required",
			},
			wantLevel: "WARN",
			wantFields: map[string]any{
				"event_type": string(audit.EventAccessDenied),
				"outcome":    string(audit.OutcomeDenied),
			},
		},
		{
			name: "blocked_event_is_warn",
			event: audit.Event{
				Type:     audit.EventLoginBlocked,
				Category: audit.CategoryRateLimit,
				Outcome:  audit.OutcomeBlocked,
				Reason:   "ip",
			},
			wantLevel: "WARN",
			wantFields: map[string]any{
				"event_type": string(audit.EventLoginBlocked),
				"outcome":    string(audit.OutcomeBlocked),
			},
		},
		{
			name: "actor_id_zero_is_omitted",
			event: audit.Event{
				Type:       audit.EventLoginFailure,
				Category:   audit.CategoryAuthentication,
				Outcome:    audit.OutcomeFailure,
				AuthMethod: audit.AuthMethodAnonymous,
			},
			wantLevel: "WARN",
			wantFields: map[string]any{
				"auth_method": string(audit.AuthMethodAnonymous),
			},
			absentField: []string{"actor_id", "actor_login"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			var buf bytes.Buffer
			logger := newBufferLogger(&buf)

			// ACT
			logger.Record(context.Background(), tt.event)

			// ASSERT
			records := decodeRecords(t, buf.Bytes())
			require.Len(t, records, 1, "exactly one audit record must be emitted")
			rec := records[0]

			assert.Equal(t, tt.wantLevel, rec["level"], "outcome must map to the documented slog level")

			for k, want := range tt.wantFields {
				assert.Equal(t, want, rec[k], "field %q must be present with expected value", k)
			}

			for _, k := range tt.absentField {
				_, ok := rec[k]
				assert.False(t, ok, "zero-valued field %q must be omitted from the record", k)
			}
		})
	}
}

// TestSlogLogger_Record_ExtraAttrsAppear covers OWASP API8:2023.
// Event-specific Extra attributes must survive into the emitted record so
// operators get the non-sensitive context they were given.
func TestSlogLogger_Record_ExtraAttrsAppear(t *testing.T) {
	t.Parallel()

	// ARRANGE
	var buf bytes.Buffer
	logger := newBufferLogger(&buf)

	event := audit.Event{
		Type:     audit.EventLoginFailure,
		Category: audit.CategoryAuthentication,
		Outcome:  audit.OutcomeFailure,
		Extra: []slog.Attr{
			slog.String("attempted_login", "bob"),
			slog.Int("abilities", 3),
		},
	}

	// ACT
	logger.Record(context.Background(), event)

	// ASSERT
	records := decodeRecords(t, buf.Bytes())
	require.Len(t, records, 1)
	rec := records[0]

	assert.Equal(t, "bob", rec["attempted_login"], "string Extra attr must be emitted")
	assert.Equal(t, float64(3), rec["abilities"], "numeric Extra attr must be emitted")
}

// TestSlogLogger_Record_EnrichesFromRequestInfo covers OWASP API8:2023.
// When a RequestInfo is present in the context the record must carry the
// correlation id and request metadata so records of one request are joinable
// (OWASP ASVS §7.1.4).
func TestSlogLogger_Record_EnrichesFromRequestInfo(t *testing.T) {
	t.Parallel()

	// ARRANGE
	var buf bytes.Buffer
	logger := newBufferLogger(&buf)

	ctx := audit.ContextWithRequestInfo(context.Background(), &audit.RequestInfo{
		RequestID: "req-abc-123",
		IP:        "203.0.113.7",
		UserAgent: "curl/8.0",
		Method:    "DELETE",
		Path:      "/api/nodes/9",
	})

	// ACT
	logger.Record(ctx, audit.Event{
		Type:     audit.EventNodeDelete,
		Category: audit.CategoryNodeOp,
		Outcome:  audit.OutcomeSuccess,
	})

	// ASSERT
	records := decodeRecords(t, buf.Bytes())
	require.Len(t, records, 1)
	rec := records[0]

	assert.Equal(t, "req-abc-123", rec["request_id"], "request_id must be pulled from RequestInfo")
	assert.Equal(t, "203.0.113.7", rec["ip"], "ip must be pulled from RequestInfo")
	assert.Equal(t, "curl/8.0", rec["user_agent"], "user_agent must be pulled from RequestInfo")
	assert.Equal(t, "DELETE", rec["http_method"], "http_method must be pulled from RequestInfo")
	assert.Equal(t, "/api/nodes/9", rec["path"], "path must be pulled from RequestInfo")
}

// TestSlogLogger_Record_OmitsEmptyRequestInfoFields covers OWASP API8:2023.
// A RequestInfo with empty fields must not inject blank attributes.
func TestSlogLogger_Record_OmitsEmptyRequestInfoFields(t *testing.T) {
	t.Parallel()

	// ARRANGE
	var buf bytes.Buffer
	logger := newBufferLogger(&buf)

	ctx := audit.ContextWithRequestInfo(context.Background(), &audit.RequestInfo{
		RequestID: "only-id",
	})

	// ACT
	logger.Record(ctx, audit.Event{
		Type:    audit.EventLoginSuccess,
		Outcome: audit.OutcomeSuccess,
	})

	// ASSERT
	records := decodeRecords(t, buf.Bytes())
	require.Len(t, records, 1)
	rec := records[0]

	assert.Equal(t, "only-id", rec["request_id"])
	for _, k := range []string{"ip", "user_agent", "http_method", "path"} {
		_, ok := rec[k]
		assert.False(t, ok, "empty RequestInfo field %q must be omitted", k)
	}
}

// TestNewLogger_NilFallsBackToDefault covers OWASP API8:2023.
// NewLogger(nil) must not panic and must produce a working sink so a missing
// logger never silently drops the audit trail.
//
//nolint:paralleltest // mutates the global slog default logger via slog.SetDefault
func TestNewLogger_NilFallsBackToDefault(t *testing.T) {
	// ARRANGE
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	logger := audit.NewLogger(nil)

	// ACT
	require.NotPanics(t, func() {
		logger.Record(context.Background(), audit.Event{
			Type:    audit.EventLoginSuccess,
			Outcome: audit.OutcomeSuccess,
		})
	})

	// ASSERT
	records := decodeRecords(t, buf.Bytes())
	require.Len(t, records, 1, "nil logger must fall back to slog.Default()")
	assert.Equal(t, "audit", records[0]["msg"])
	assert.Equal(t, "audit", records[0]["component"])
}

// TestNopLogger_Record covers OWASP API8:2023.
// NopLogger must be a true no-op (used when audit is disabled) and the
// helpers must tolerate a nil Logger without panicking — call sites rely on
// this to omit nil-guards.
func TestNopLogger_Record(t *testing.T) {
	t.Parallel()

	t.Run("nop_logger_writes_nothing", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		var buf bytes.Buffer
		// A real logger is wired to buf but NopLogger must never touch it.
		_ = newBufferLogger(&buf)
		nop := audit.NopLogger{}

		// ACT
		nop.Record(context.Background(), audit.Event{
			Type:    audit.EventNodeDelete,
			Outcome: audit.OutcomeSuccess,
		})

		// ASSERT
		assert.Empty(t, buf.Bytes(), "NopLogger must discard every event")
	})

	t.Run("helpers_with_nil_logger_do_not_panic", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		ctx := context.Background()

		// ACT + ASSERT
		require.NotPanics(t, func() {
			audit.TokenRejected(ctx, nil, "missing_token")
			audit.DaemonRejected(ctx, nil, "bad_token")
			audit.LoginSuccess(ctx, nil, 1, "alice")
			audit.LoginFailure(ctx, nil, "bob", "invalid_credentials")
			audit.LoginBlocked(ctx, nil, "bob", "ip")
			audit.AccessDenied(ctx, nil, "server", "1", "missing_ability")
			audit.SensitiveOp(ctx, nil, audit.EventPATCreate, audit.CategoryTokenOp, "token", "1", "create")
		}, "audit helpers must be a safe no-op when the Logger is nil")
	})
}
