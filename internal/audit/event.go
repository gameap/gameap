package audit

import "log/slog"

// Category groups audit events so operators can build filtering and
// alerting on top of the audit stream.
type Category string

const (
	CategoryAuthentication Category = "authentication"
	CategoryAuthorization  Category = "authorization"
	CategoryRateLimit      Category = "ratelimit"
	CategoryAdminOp        Category = "admin_op"
	CategoryTokenOp        Category = "token_op"
	CategoryNodeOp         Category = "node_op"
	CategoryFileOp         Category = "file_op"
	CategoryPluginOp       Category = "plugin_op"
)

// Outcome is the result of the audited action.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
	OutcomeBlocked Outcome = "blocked"
)

// EventType is a stable identifier for the kind of security event. These
// values are part of the audit contract — never repurpose an existing one.
type EventType string

const (
	EventAuthTokenRejected  EventType = "auth.token.rejected" //nolint:gosec // G101 false positive: event id
	EventAuthDaemonRejected EventType = "auth.daemon.rejected"
	EventLoginSuccess       EventType = "auth.login.success"
	EventLoginFailure       EventType = "auth.login.failure"
	EventLoginBlocked       EventType = "auth.login.blocked"
	EventAccessDenied       EventType = "access.denied"

	EventReauthSuccess EventType = "auth.reauth.success"
	EventReauthFailure EventType = "auth.reauth.failure"

	// MFA-nudge lifecycle events. The "shown" / "snoozed" pair records
	// each step of the soft-enforcement flow for admins without 2FA so
	// an operator can audit how long their admins have been ignoring
	// the recommendation. "enrollment_required" fires when the hard
	// failure threshold is crossed and the login flow refuses to issue
	// a session.
	EventMFANudgeShown          EventType = "auth.mfa.nudge.shown"
	EventMFANudgeSnoozed        EventType = "auth.mfa.nudge.snoozed"
	EventMFAEnrollmentRequired  EventType = "auth.mfa.enrollment.required"
	EventMFAEnrollmentCompleted EventType = "auth.mfa.enrollment.completed"

	EventTwoFactorChallenge          EventType = "auth.2fa.challenge"
	EventTwoFactorVerifySuccess      EventType = "auth.2fa.verify.success"
	EventTwoFactorVerifyFailure      EventType = "auth.2fa.verify.failure"
	EventTwoFactorEnabled            EventType = "auth.2fa.enabled"
	EventTwoFactorDisabled           EventType = "auth.2fa.disabled"
	EventTwoFactorRecoveryRegenerate EventType = "auth.2fa.recovery.regenerate"

	EventUserUpdate           EventType = "user.update"
	EventUserRolesAssign      EventType = "user.roles.assign"
	EventPATCreate            EventType = "token.pat.create"
	EventPATRevoke            EventType = "token.pat.revoke"
	EventDaemonTokenIssue     EventType = "token.daemon.issue"
	EventShortLivedTokenIssue EventType = "token.shortlived.issue"
	EventNodeCreate           EventType = "node.create"
	EventNodeUpdate           EventType = "node.update"
	EventNodeDelete           EventType = "node.delete"
	EventFileDelete           EventType = "file.delete"
	EventFileRename           EventType = "file.rename"
	EventFileChmod            EventType = "file.chmod"
	EventFileWrite            EventType = "file.write"
	EventFileUpload           EventType = "file.upload"
	EventFileArchiveCreate    EventType = "file.archive.create"
	EventFileArchiveExtract   EventType = "file.archive.extract"
	EventFileArchiveCancel    EventType = "file.archive.cancel"
	EventPluginInstall        EventType = "plugin.install"
	EventPluginUninstall      EventType = "plugin.uninstall"
)

// Plugin runtime lifecycle: recorded by the panel itself (AuthMethodSystem)
// when a guest misbehaves and when it is brought back, and by an operator
// reloading a plugin on demand.
const (
	EventPluginDisabled EventType = "plugin.disabled"
	EventPluginReloaded EventType = "plugin.reloaded"
)

// AuthMethod describes how the actor authenticated for the audited request.
type AuthMethod string

const (
	AuthMethodSession    AuthMethod = "session"
	AuthMethodPAT        AuthMethod = "pat"
	AuthMethodShortLived AuthMethod = "shortlived"
	AuthMethodAnonymous  AuthMethod = "anonymous"
	// AuthMethodSystem marks actions the panel takes on its own, outside
	// any request (plugin recovery, background maintenance).
	AuthMethodSystem AuthMethod = "system"
)

// Event is the stable audit-record schema. Zero-valued fields are omitted
// from the emitted record.
//
// Never place secrets (token values, passwords, file contents) or raw error
// strings into any field. Reason must be a short, stable enum-like token,
// and Extra must carry only non-sensitive context.
type Event struct {
	Type     EventType
	Category Category
	Outcome  Outcome

	// Actor identity. When AuthMethod is left empty the helpers derive it
	// from the request context (user session, daemon session, or anonymous);
	// set it explicitly to bypass that derivation.
	ActorID    uint
	ActorLogin string
	AuthMethod AuthMethod

	// Resource the action targeted.
	ResourceType string
	ResourceID   string
	Action       string

	// Reason is a short, stable token explaining a failure or denial.
	Reason string

	// Extra carries event-specific, non-sensitive attributes.
	Extra []slog.Attr
}
