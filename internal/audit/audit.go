package audit

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/pkg/auth"
)

// emit enriches the event with actor identity from the request context and
// records it. A nil Logger is a safe no-op so call sites never need a guard.
func emit(ctx context.Context, l Logger, e Event) {
	if l == nil {
		return
	}

	actorFrom(ctx, &e)
	l.Record(ctx, e)
}

// actorFrom fills actor identity from the request context unless the caller
// already set it explicitly (AuthMethod non-empty). It checks the user
// session first, then a daemon session, defaulting to anonymous.
func actorFrom(ctx context.Context, e *Event) {
	if e.AuthMethod != "" {
		return
	}

	if s := auth.SessionFromContext(ctx); s.IsAuthenticated() {
		e.ActorID = s.User.ID
		e.ActorLogin = s.User.Login
		switch {
		case s.ShortLived:
			e.AuthMethod = AuthMethodShortLived
		case s.IsTokenSession():
			e.AuthMethod = AuthMethodPAT
		default:
			e.AuthMethod = AuthMethodSession
		}

		return
	}

	e.AuthMethod = AuthMethodAnonymous
}

// TokenRejected records a rejected bearer/PAT authentication attempt.
// reason must be a stable token (e.g. "missing_token", "token_revoked").
func TokenRejected(ctx context.Context, l Logger, reason string) {
	emit(ctx, l, Event{
		Type:     EventAuthTokenRejected,
		Category: CategoryAuthentication,
		Outcome:  OutcomeFailure,
		Reason:   reason,
	})
}

// DaemonRejected records a rejected daemon (node) authentication attempt.
func DaemonRejected(ctx context.Context, l Logger, reason string) {
	emit(ctx, l, Event{
		Type:     EventAuthDaemonRejected,
		Category: CategoryAuthentication,
		Outcome:  OutcomeFailure,
		Reason:   reason,
	})
}

// LoginSuccess records a successful interactive login. The session is not
// yet in context at this point, so the actor is passed explicitly.
func LoginSuccess(ctx context.Context, l Logger, userID uint, login string) {
	emit(ctx, l, Event{
		Type:       EventLoginSuccess,
		Category:   CategoryAuthentication,
		Outcome:    OutcomeSuccess,
		ActorID:    userID,
		ActorLogin: login,
		AuthMethod: AuthMethodSession,
	})
}

// LoginFailure records a failed login. login is the submitted identifier
// (not a secret); it is recorded as attempted_login, not as an actor.
func LoginFailure(ctx context.Context, l Logger, login, reason string) {
	emit(ctx, l, Event{
		Type:       EventLoginFailure,
		Category:   CategoryAuthentication,
		Outcome:    OutcomeFailure,
		AuthMethod: AuthMethodAnonymous,
		Reason:     reason,
		Extra:      []slog.Attr{slog.String("attempted_login", login)},
	})
}

// LoginBlocked records a login attempt refused by the rate limiter before
// credentials were checked. reason is "ip" or "username".
func LoginBlocked(ctx context.Context, l Logger, login, reason string) {
	emit(ctx, l, Event{
		Type:       EventLoginBlocked,
		Category:   CategoryRateLimit,
		Outcome:    OutcomeBlocked,
		AuthMethod: AuthMethodAnonymous,
		Reason:     reason,
		Extra:      []slog.Attr{slog.String("attempted_login", login)},
	})
}

// TwoFactorChallengeIssued records that a password check passed but a second
// factor is still required, so only a challenge token was returned. The actor
// is recorded explicitly since no session exists yet.
func TwoFactorChallengeIssued(ctx context.Context, l Logger, userID uint, login string) {
	emit(ctx, l, Event{
		Type:       EventTwoFactorChallenge,
		Category:   CategoryAuthentication,
		Outcome:    OutcomeSuccess,
		ActorID:    userID,
		ActorLogin: login,
		AuthMethod: AuthMethodAnonymous,
	})
}

// TwoFactorVerifySuccess records a completed second-factor verification.
// method is "totp" or "recovery_code".
func TwoFactorVerifySuccess(ctx context.Context, l Logger, userID uint, login, method string) {
	emit(ctx, l, Event{
		Type:       EventTwoFactorVerifySuccess,
		Category:   CategoryAuthentication,
		Outcome:    OutcomeSuccess,
		ActorID:    userID,
		ActorLogin: login,
		AuthMethod: AuthMethodSession,
		Extra:      []slog.Attr{slog.String("factor", method)},
	})
}

// TwoFactorVerifyFailure records a wrong code or an invalid/expired challenge
// presented to the verification endpoint.
func TwoFactorVerifyFailure(ctx context.Context, l Logger, login, reason string) {
	emit(ctx, l, Event{
		Type:       EventTwoFactorVerifyFailure,
		Category:   CategoryAuthentication,
		Outcome:    OutcomeFailure,
		AuthMethod: AuthMethodAnonymous,
		Reason:     reason,
		Extra:      []slog.Attr{slog.String("attempted_login", login)},
	})
}

// AccessDenied records an authorization denial (admin gate or per-server
// ability check). The actor is derived from context (auth already passed).
func AccessDenied(
	ctx context.Context,
	l Logger,
	resourceType, resourceID, reason string,
	extra ...slog.Attr,
) {
	emit(ctx, l, Event{
		Type:         EventAccessDenied,
		Category:     CategoryAuthorization,
		Outcome:      OutcomeDenied,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Reason:       reason,
		Extra:        extra,
	})
}

// SensitiveOpFailed records a sensitive operation that did not complete;
// reason must be a stable token. The actor is derived from the request
// context.
func SensitiveOpFailed(
	ctx context.Context,
	l Logger,
	eventType EventType,
	category Category,
	resourceType, resourceID, action, reason string,
	extra ...slog.Attr,
) {
	emit(ctx, l, Event{
		Type:         eventType,
		Category:     category,
		Outcome:      OutcomeFailure,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Reason:       reason,
		Extra:        extra,
	})
}

// SystemOp records an operation the panel performed on its own, outside any
// request: the actor is the system. reason must be a stable token or empty.
func SystemOp(
	ctx context.Context,
	l Logger,
	eventType EventType,
	category Category,
	outcome Outcome,
	resourceType, resourceID, action, reason string,
	extra ...slog.Attr,
) {
	emit(ctx, l, Event{
		Type:         eventType,
		Category:     category,
		Outcome:      outcome,
		AuthMethod:   AuthMethodSystem,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Reason:       reason,
		Extra:        extra,
	})
}

// PluginActor identifies a plugin acting through the host libraries.
type PluginActor struct {
	// ID is the plugin's database ID; 0 is a transient (dry-run) load.
	ID uint64
	// Name is the plugin's declared ID (PluginInfo.id), for readability.
	Name string
}

// PluginOp records an action a plugin performed (or attempted) through a host
// library. The actor is the plugin itself; when the call was triggered by a
// user's request (an event or a plugin HTTP route) the request context still
// carries that session, which is recorded as the on-behalf-of user so the
// initiator is not lost. reason must be a stable token or empty.
func PluginOp(
	ctx context.Context,
	l Logger,
	eventType EventType,
	category Category,
	outcome Outcome,
	actor PluginActor,
	resourceType, resourceID, action, reason string,
	extra ...slog.Attr,
) {
	emit(ctx, l, Event{
		Type:         eventType,
		Category:     category,
		Outcome:      outcome,
		ActorID:      uint(actor.ID),
		ActorLogin:   actor.Name,
		AuthMethod:   AuthMethodPlugin,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Reason:       reason,
		Extra:        appendOnBehalfOf(ctx, extra),
	})
}

// appendOnBehalfOf adds the authenticated user from the request context, if
// any, to a plugin-attributed event.
func appendOnBehalfOf(ctx context.Context, extra []slog.Attr) []slog.Attr {
	s := auth.SessionFromContext(ctx)
	if !s.IsAuthenticated() {
		return extra
	}

	attrs := make([]slog.Attr, 0, len(extra)+2)
	attrs = append(attrs,
		slog.Uint64("on_behalf_of_user_id", uint64(s.User.ID)),
		slog.String("on_behalf_of_login", s.User.Login),
	)

	return append(attrs, extra...)
}

// SensitiveOp records the successful execution of a sensitive operation.
// The actor is derived from the request context.
func SensitiveOp(
	ctx context.Context,
	l Logger,
	eventType EventType,
	category Category,
	resourceType, resourceID, action string,
	extra ...slog.Attr,
) {
	emit(ctx, l, Event{
		Type:         eventType,
		Category:     category,
		Outcome:      OutcomeSuccess,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Extra:        extra,
	})
}
