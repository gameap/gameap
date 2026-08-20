// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
//
// Pins the re-auth helper (ASVS §2.1.6 / §3.7.1 / §4.3.3) used to gate
// sensitive operations behind a fresh password re-entry. The helper is
// covered here at the unit level so handlers can rely on a single,
// audited choke-point.
package base_test

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubVerifier returns a PasswordVerifier that accepts only the literal
// "correct" candidate. Keeps tests deterministic regardless of bcrypt cost.
func stubVerifier() base.PasswordVerifier {
	return func(_, candidate string) (bool, error) {
		if candidate == "correct" {
			return false, nil
		}

		return false, errors.New("verifier: wrong password")
	}
}

func authenticatedSession() *auth.Session {
	return &auth.Session{
		Login: "alice",
		User: &domain.User{
			ID:       7,
			Login:    "alice",
			Password: "stored-hash",
		},
	}
}

// TestVerifyCurrentPassword_AcceptsCorrectPassword — OWASP API2:2023 — the
// happy path returns nil so the calling handler proceeds with the
// sensitive operation. An audit event with the success type must be
// recorded.
func TestVerifyCurrentPassword_AcceptsCorrectPassword(t *testing.T) {
	t.Parallel()

	recorder := newAuditCapture()
	err := base.VerifyCurrentPassword(
		context.Background(),
		recorder,
		authenticatedSession(),
		"correct",
		stubVerifier(),
	)

	require.NoError(t, err)

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventReauthSuccess, events[0].Type)
	assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	assert.Equal(t, uint(7), events[0].ActorID)
	assert.Equal(t, "alice", events[0].ActorLogin)
}

// TestVerifyCurrentPassword_RejectsMissingPassword — OWASP API2:2023 — an
// empty current_password is a 400 (the client failed to supply a required
// field) and the failure is audited with a stable reason token.
func TestVerifyCurrentPassword_RejectsMissingPassword(t *testing.T) {
	t.Parallel()

	recorder := newAuditCapture()
	err := base.VerifyCurrentPassword(
		context.Background(),
		recorder,
		authenticatedSession(),
		"",
		stubVerifier(),
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, base.ErrMissingCurrentPassword))

	var httpErr *api.WrappedError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 400, httpErr.HTTPStatus())

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventReauthFailure, events[0].Type)
	assert.Equal(t, "missing_current_password", events[0].Reason)
}

// TestVerifyCurrentPassword_RejectsWrongPassword — OWASP API2:2023 — a
// wrong password is a 401 and the failure is audited with the stable
// invalid_current_password reason. The stub verifier guarantees the
// audit fires before any sensitive op executes.
func TestVerifyCurrentPassword_RejectsWrongPassword(t *testing.T) {
	t.Parallel()

	recorder := newAuditCapture()
	err := base.VerifyCurrentPassword(
		context.Background(),
		recorder,
		authenticatedSession(),
		"wrong",
		stubVerifier(),
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, base.ErrInvalidCurrentPassword))

	var httpErr *api.WrappedError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 401, httpErr.HTTPStatus())

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventReauthFailure, events[0].Type)
	assert.Equal(t, "invalid_current_password", events[0].Reason)
}

// TestVerifyCurrentPassword_RejectsPATSession — OWASP API2:2023 — PAT
// sessions carry no password and therefore cannot satisfy re-auth. The
// helper refuses with 403 + ErrReauthNotAvailable so the handler can
// distinguish "PAT cannot do this" from "wrong password".
func TestVerifyCurrentPassword_RejectsPATSession(t *testing.T) {
	t.Parallel()

	session := authenticatedSession()
	session.Token = &domain.PersonalAccessToken{ID: 1}

	recorder := newAuditCapture()
	err := base.VerifyCurrentPassword(
		context.Background(),
		recorder,
		session,
		"correct", // value is irrelevant — PAT path bypasses verifier
		stubVerifier(),
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, base.ErrReauthNotAvailable))

	var httpErr *api.WrappedError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 403, httpErr.HTTPStatus())

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventReauthFailure, events[0].Type)
	assert.Equal(t, "pat_session_not_supported", events[0].Reason)
}

// TestVerifyCurrentPassword_RejectsUnauthenticatedSession — OWASP API2:2023
// — a nil or unauthenticated session yields 401 outright. The helper does
// NOT call the verifier and does NOT audit (it would have no actor).
func TestVerifyCurrentPassword_RejectsUnauthenticatedSession(t *testing.T) {
	t.Parallel()

	recorder := newAuditCapture()
	err := base.VerifyCurrentPassword(
		context.Background(),
		recorder,
		nil,
		"correct",
		stubVerifier(),
	)

	require.Error(t, err)
	var httpErr *api.WrappedError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 401, httpErr.HTTPStatus())

	assert.Empty(t, recorder.snapshot())
}

// TestVerifyCurrentPassword_RejectsSessionWithoutPasswordHash — OWASP
// API2:2023 — a session that arrived without a populated User.Password
// cannot be re-auth'd; the helper refuses defensively rather than passing
// an empty string to the verifier (which a careless verifier could match).
func TestVerifyCurrentPassword_RejectsSessionWithoutPasswordHash(t *testing.T) {
	t.Parallel()

	session := authenticatedSession()
	session.User.Password = ""

	recorder := newAuditCapture()
	err := base.VerifyCurrentPassword(
		context.Background(),
		recorder,
		session,
		"correct",
		stubVerifier(),
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, base.ErrReauthNotAvailable))

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, "session_has_no_password", events[0].Reason)
}

// auditCapture mirrors the upload/test recorder so the assertions read
// like the rest of the security test harness.
type auditCapture struct {
	events []audit.Event
}

func newAuditCapture() *auditCapture { return &auditCapture{} }

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	return append([]audit.Event(nil), a.events...)
}
