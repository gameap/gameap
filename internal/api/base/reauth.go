package base

import (
	"context"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// Errors a handler can compare with errors.Is to choose its HTTP status.
// They are exposed so call sites can rely on api.WrapHTTPError to produce
// the documented status codes.
var (
	// ErrMissingCurrentPassword signals that the input did not include a
	// non-empty current_password field. Handlers should respond 400.
	ErrMissingCurrentPassword = errors.New("re-auth required: current_password is missing")

	// ErrInvalidCurrentPassword signals that the supplied current_password
	// did not verify against the stored hash. Handlers should respond 401.
	ErrInvalidCurrentPassword = errors.New("re-auth failed: current_password is incorrect")

	// ErrReauthNotAvailable signals that the active session has no
	// password to re-verify against — typically a PAT session. Handlers
	// should reject the sensitive op outright (403 or 401 by policy).
	ErrReauthNotAvailable = errors.New("re-auth not available for this session")
)

// PasswordVerifier is the minimal contract VerifyCurrentPassword needs.
// auth.VerifyPassword satisfies it directly; we accept a function for
// easy testing without dragging the whole pkg/auth surface in.
type PasswordVerifier func(hashedPassword, candidate string) (needsRehash bool, err error)

// VerifyCurrentPassword runs the project-wide re-auth gate for sensitive
// operations (ASVS §2.1.6 / §3.7.1 / §4.3.3). It:
//
//   - Refuses PAT sessions outright — a PAT has no password to verify.
//   - Refuses empty current_password with ErrMissingCurrentPassword.
//   - Refuses a wrong current_password with ErrInvalidCurrentPassword.
//   - Emits a stable audit event on success or failure so a defender
//     can spot brute-force or anomaly patterns. The audit reason is a
//     short, non-sensitive token; password values never appear.
//
// Callers are responsible for translating the error into the right HTTP
// status (api.WrapHTTPError handles the common cases) and for invoking
// the actual sensitive operation only when this function returns nil.
//
// The verifier argument lets tests inject a deterministic hash check;
// production code passes auth.VerifyPassword.
func VerifyCurrentPassword(
	ctx context.Context,
	auditLog audit.Logger,
	session *auth.Session,
	current string,
	verifier PasswordVerifier,
) error {
	if session == nil || !session.IsAuthenticated() {
		return api.WrapHTTPError(errors.New("re-auth required: unauthenticated session"), 401)
	}

	if session.Token != nil {
		// PAT sessions cannot re-auth — they carry no password. Emit
		// audit so an operator can see that a PAT was used against a
		// re-auth-gated endpoint (the handler will additionally 403).
		auditReauthFailure(ctx, auditLog, session, "pat_session_not_supported")

		return api.WrapHTTPError(ErrReauthNotAvailable, 403)
	}

	if current == "" {
		auditReauthFailure(ctx, auditLog, session, "missing_current_password")

		return api.WrapHTTPError(ErrMissingCurrentPassword, 400)
	}

	if session.User == nil || session.User.Password == "" {
		// Defensive: a session with no resolved user / password cannot
		// be re-auth'd. This should be unreachable through the auth
		// middleware but is reported instead of panicking.
		auditReauthFailure(ctx, auditLog, session, "session_has_no_password")

		return api.WrapHTTPError(ErrReauthNotAvailable, 401)
	}

	if _, err := verifier(session.User.Password, current); err != nil {
		auditReauthFailure(ctx, auditLog, session, "invalid_current_password")

		return api.WrapHTTPError(ErrInvalidCurrentPassword, 401)
	}

	auditReauthSuccess(ctx, auditLog, session)

	return nil
}

// auditReauthSuccess emits an authentication-category event keyed to the
// session actor. The Reason field is omitted on success.
func auditReauthSuccess(ctx context.Context, log audit.Logger, session *auth.Session) {
	if log == nil {
		return
	}

	log.Record(ctx, audit.Event{
		Type:       audit.EventReauthSuccess,
		Category:   audit.CategoryAuthentication,
		Outcome:    audit.OutcomeSuccess,
		ActorID:    session.User.ID,
		ActorLogin: session.Login,
	})
}

// auditReauthFailure emits the failure event with a stable reason token.
func auditReauthFailure(ctx context.Context, log audit.Logger, session *auth.Session, reason string) {
	if log == nil {
		return
	}

	event := audit.Event{
		Type:     audit.EventReauthFailure,
		Category: audit.CategoryAuthentication,
		Outcome:  audit.OutcomeFailure,
		Reason:   reason,
	}

	if session != nil && session.User != nil {
		event.ActorID = session.User.ID
		event.ActorLogin = session.Login
	}

	log.Record(ctx, event)
}

// silence_domain is a compile-time anchor so the domain import stays
// available for handlers that compose this helper with role checks.
var _ = domain.AbilityNameAdminRolesPermissions
