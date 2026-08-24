package hostlibrary

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// Denial reasons reported to the HostCallObserver.
const (
	DenyReasonPermission = "permission"
	DenyReasonRateLimit  = "rate_limit"
)

// hostFunctionResource is the audit resource type of a refused host call;
// the resource ID is "<module>.<export>".
const hostFunctionResource = "host_function"

// HostCallObserver receives refused host calls for metrics.
type HostCallObserver interface {
	HostCallDenied(pluginID uint64, module, function, reason string)
}

type nopHostCallObserver struct{}

func (nopHostCallObserver) HostCallDenied(uint64, string, string, string) {}

// GuardOption tunes a Guard.
type GuardOption func(*Guard)

// WithGuardRateLimits installs the per-class token buckets; classes missing
// from the map are unlimited.
func WithGuardRateLimits(limits map[RateClass]RateLimit) GuardOption {
	return func(g *Guard) {
		g.limiter = newRateLimiter(limits)
	}
}

// WithGuardAudit records refusals and privileged operations to the audit log.
func WithGuardAudit(logger audit.Logger) GuardOption {
	return func(g *Guard) {
		if logger != nil {
			g.audit = logger
		}
	}
}

// WithGuardObserver reports refusals for metrics.
func WithGuardObserver(observer HostCallObserver) GuardOption {
	return func(g *Guard) {
		if observer != nil {
			g.observer = observer
		}
	}
}

// Guard is the shared policy enforcement in front of the privileged host
// libraries: the plugin's grants (hostRPCPolicies), the per-plugin rate
// limits, and the audit trail of what plugins do. One Guard serves every
// plugin; For binds it to one.
type Guard struct {
	checker  PluginPermissionChecker
	limiter  *rateLimiter
	throttle *auditThrottler
	audit    audit.Logger
	observer HostCallObserver
}

func NewGuard(checker PluginPermissionChecker, opts ...GuardOption) *Guard {
	g := &Guard{
		checker:  checker,
		throttle: newAuditThrottler(auditThrottleInterval),
		audit:    audit.NopLogger{},
		observer: nopHostCallObserver{},
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// For binds the guard to one plugin (its database ID; 0 is a transient load
// that is never granted anything).
func (g *Guard) For(pluginID uint64) *PluginGuard {
	return &PluginGuard{
		guard:    g,
		pluginID: pluginID,
		actor: audit.PluginActor{
			ID:   pluginID,
			Name: pkgplugin.CompactPluginID(domain.Uint64ID(pluginID)),
		},
	}
}

// Forget drops the per-plugin limiter state and cached grants (uninstall).
func (g *Guard) Forget(pluginID uint64) {
	g.limiter.forget(pluginID)
	g.throttle.forget(pluginID)

	if cache, ok := g.checker.(PermissionCacheInvalidator); ok {
		cache.Invalidate(pluginID)
	}
}

// PluginGuard enforces the policy for one plugin.
type PluginGuard struct {
	guard    *Guard
	pluginID uint64
	actor    audit.PluginActor
}

// PluginID is the database ID the guard is bound to.
func (p *PluginGuard) PluginID() uint64 {
	return p.pluginID
}

// Check enforces the policy of the host function the plugin is calling: the
// grant first, then the rate limit (a refused call consumes no grant lookup
// budget). It answers the message to return to the guest, or "" when the
// call may proceed. Host libraries wrap it as
//
//	if msg := s.guard.Check(ctx, ModuleX, "export"); msg != "" { return &Resp{Error: new(msg)}, nil }
func (p *PluginGuard) Check(ctx context.Context, module, export string) string {
	policy, ok := PolicyFor(module, export)
	if !ok {
		return ""
	}

	if policy.Permission != "" {
		if msg := p.Require(ctx, module, export, policy.Permission); msg != "" {
			return msg
		}
	}

	return p.limit(ctx, module, export, policy.RateClass)
}

// Require checks one grant for the host function regardless of the policy
// table, for functions whose requirement depends on the request (a cmdexec
// daemon task needs node_commands on top of manage_servers).
func (p *PluginGuard) Require(ctx context.Context, module, export string, permission domain.PluginPermission) string {
	allowed, err := p.guard.checker.Has(ctx, p.pluginID, permission)
	if err != nil {
		slog.ErrorContext(ctx, "failed to check plugin permission",
			slog.Uint64("plugin_id", p.pluginID),
			slog.String("module", module),
			slog.String("function", export),
			slog.String("error", err.Error()))

		return "failed to check plugin permission: " + err.Error()
	}

	if allowed {
		return ""
	}

	slog.WarnContext(ctx, "plugin denied access to a host function",
		slog.Uint64("plugin_id", p.pluginID),
		slog.String("module", module),
		slog.String("function", export),
		slog.String("permission", string(permission)))

	p.guard.observer.HostCallDenied(p.pluginID, module, export, DenyReasonPermission)

	if p.guard.throttle.admit(throttleKey{p.pluginID, module, export, DenyReasonPermission}) {
		audit.PluginOp(ctx, p.guard.audit, audit.EventAccessDenied, audit.CategoryAuthorization,
			audit.OutcomeDenied, p.actor, hostFunctionResource, module+"."+export, export,
			"plugin_permission_missing", slog.String("permission", string(permission)))
	}

	return PermissionDeniedMessage(permission)
}

func (p *PluginGuard) limit(ctx context.Context, module, export string, class RateClass) string {
	allowed, limit := p.guard.limiter.allow(p.pluginID, class)
	if allowed {
		return ""
	}

	p.guard.observer.HostCallDenied(p.pluginID, module, export, DenyReasonRateLimit)

	if p.guard.throttle.admit(throttleKey{p.pluginID, module, export, DenyReasonRateLimit}) {
		slog.WarnContext(ctx, "plugin host call rate limited",
			slog.Uint64("plugin_id", p.pluginID),
			slog.String("module", module),
			slog.String("function", export),
			slog.Float64("rps", limit.RPS),
			slog.Int("burst", limit.burst()))

		audit.PluginOp(ctx, p.guard.audit, audit.EventPluginHostCallRateLimited, audit.CategoryRateLimit,
			audit.OutcomeBlocked, p.actor, hostFunctionResource, module+"."+export, export, "rate_limited",
			slog.String("class", string(class)),
			slog.Float64("rps", limit.RPS),
			slog.Int("burst", limit.burst()))
	}

	return RateLimitedMessage(module, limit)
}

// Audit records a privileged operation the plugin performed through a host
// library: outcome success, or failure with a stable reason when err is set
// (the error text itself is returned to the guest, not written to the audit
// stream).
func (p *PluginGuard) Audit(
	ctx context.Context,
	eventType audit.EventType,
	action, resourceType, resourceID string,
	err error,
	extra ...slog.Attr,
) {
	outcome, reason := audit.OutcomeSuccess, ""
	if err != nil {
		outcome, reason = audit.OutcomeFailure, "operation_failed"
	}

	audit.PluginOp(ctx, p.guard.audit, eventType, audit.CategoryPluginOp, outcome, p.actor,
		resourceType, resourceID, action, reason, extra...)
}

// PermissionDeniedMessage is what a plugin without the grant sees. It names
// the missing permission so plugin authors know what to declare.
func PermissionDeniedMessage(permission domain.PluginPermission) string {
	return "plugin permission " + string(permission) + " required"
}

// RateLimitedMessage is what a plugin sees when its bucket is empty; the
// "rate limited" prefix is stable so plugins can back off on it.
func RateLimitedMessage(module string, limit RateLimit) string {
	return "rate limited: " + module + " allows " + strconv.FormatFloat(limit.RPS, 'f', -1, 64) +
		" calls/s (burst " + strconv.Itoa(limit.burst()) + ")"
}

// nodeResourceID formats a node for the audit resource_id.
func nodeResourceID(nodeID uint64) string {
	return strconv.FormatUint(nodeID, 10)
}

// serverResourceID formats a server for the audit resource_id.
func serverResourceID(serverID uint64) string {
	return strconv.FormatUint(serverID, 10)
}
