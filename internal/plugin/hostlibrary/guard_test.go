// Security tests for the host-call guard shared by the privileged plugin
// host libraries.
//
// OWASP API Security Top 10:2023 — API5:2023 Broken Function Level
// Authorization (every privileged host function is gated on the plugin's own
// grant), API4:2023 Unrestricted Resource Consumption (per-plugin token
// buckets on the expensive modules) and API9:2023 Improper Inventory
// Management (the policy table is the single inventory of gated functions and
// is checked against the generated SDK glue).
package hostlibrary

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const guardTestPluginID = uint64(31)

var (
	errGuardTestDB        = errors.New("db down")
	errGuardTestOperation = errors.New("daemon unreachable")
)

// auditRecorder keeps every audit event the guard emitted.
type auditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *auditRecorder) Record(_ context.Context, e audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *auditRecorder) all() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.events)
}

// denialRecorder keeps what the guard reported to the metrics observer.
type denialRecorder struct {
	mu      sync.Mutex
	reasons []string
}

func (r *denialRecorder) HostCallDenied(_ uint64, module, function, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reasons = append(r.reasons, module+"."+function+":"+reason)
}

func (r *denialRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.reasons)
}

// grantSet answers Has from an explicit set of grants.
type grantSet map[domain.PluginPermission]bool

func (g grantSet) Has(_ context.Context, _ uint64, permission domain.PluginPermission) (bool, error) {
	granted := make([]domain.PluginPermission, 0, len(g))
	for name, ok := range g {
		if ok {
			granted = append(granted, name)
		}
	}

	return domain.PermissionSatisfied(permission, granted), nil
}

func TestPluginGuard_Check_enforces_policy_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		module    string
		export    string
		grants    grantSet
		wantError string
	}{
		{
			name:      "ungated_function_needs_no_grant",
			module:    ModuleServers,
			export:    "find_servers",
			grants:    grantSet{},
			wantError: "",
		},
		{
			name:      "server_control_without_manage_servers",
			module:    ModuleServerControl,
			export:    "start_server",
			grants:    grantSet{domain.PluginPermissionFiles: true},
			wantError: "plugin permission manage_servers required",
		},
		{
			name:      "server_control_with_manage_servers",
			module:    ModuleServerControl,
			export:    "restart_server",
			grants:    grantSet{domain.PluginPermissionManageServers: true},
			wantError: "",
		},
		{
			name:      "node_command_needs_node_commands_not_files",
			module:    ModuleNodeCmd,
			export:    "execute_command",
			grants:    grantSet{domain.PluginPermissionFiles: true, domain.PluginPermissionManageServers: true},
			wantError: "plugin permission node_commands required",
		},
		{
			name:      "node_command_with_node_commands",
			module:    ModuleNodeCmd,
			export:    "execute_command",
			grants:    grantSet{domain.PluginPermissionNodeCommands: true},
			wantError: "",
		},
		{
			name:      "nodefs_read_needs_files_read",
			module:    ModuleNodeFS,
			export:    "read_dir",
			grants:    grantSet{domain.PluginPermissionNodeCommands: true},
			wantError: "plugin permission files_read required",
		},
		{
			name:      "nodefs_read_satisfied_by_files",
			module:    ModuleNodeFS,
			export:    "read_dir",
			grants:    grantSet{domain.PluginPermissionFiles: true},
			wantError: "",
		},
		{
			name:      "nodefs_read_satisfied_by_files_read",
			module:    ModuleNodeFS,
			export:    "download",
			grants:    grantSet{domain.PluginPermissionFilesRead: true},
			wantError: "",
		},
		{
			name:      "nodefs_write_needs_files",
			module:    ModuleNodeFS,
			export:    "upload",
			grants:    grantSet{domain.PluginPermissionFilesRead: true},
			wantError: "plugin permission files required",
		},
		{
			name:      "server_save_needs_manage_servers",
			module:    ModuleServers,
			export:    "save_server",
			grants:    grantSet{},
			wantError: "plugin permission manage_servers required",
		},
		{
			name:      "server_setting_needs_manage_servers",
			module:    ModuleServerSettings,
			export:    "save_server_setting",
			grants:    grantSet{},
			wantError: "plugin permission manage_servers required",
		},
		{
			name:      "daemon_task_creation_needs_manage_servers",
			module:    ModuleDaemonTasks,
			export:    "create_daemon_task",
			grants:    grantSet{},
			wantError: "plugin permission manage_servers required",
		},
		{
			name:      "rbac_needs_manage_rbac",
			module:    ModuleRBAC,
			export:    "save_role",
			grants:    grantSet{domain.PluginPermissionManageServers: true},
			wantError: "plugin permission manage_rbac required",
		},
		{
			name:      "secrets_need_secrets",
			module:    ModuleSecrets,
			export:    "get",
			grants:    grantSet{},
			wantError: "plugin permission secrets required",
		},
		{
			name:      "http_fetch_is_open_to_every_plugin",
			module:    ModuleHTTP,
			export:    "fetch",
			grants:    grantSet{},
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			guard := NewGuard(tt.grants).For(guardTestPluginID)

			msg := guard.Check(context.Background(), tt.module, tt.export)

			if tt.wantError == "" {
				assert.Empty(t, msg)

				return
			}

			assert.Equal(t, tt.wantError, msg)
		})
	}
}

func TestPluginGuard_Check_admits_everything_without_enforcement(t *testing.T) {
	t.Parallel()

	// The container wires this checker while PLUGIN_PERMISSIONS_ENFORCE is
	// off: a plugin holding no grants at all keeps every host function.
	guard := NewGuard(AllowAllPermissionChecker{}).For(guardTestPluginID)

	msg := guard.Check(context.Background(), ModuleNodeCmd, "execute_command")

	assert.Empty(t, msg)
}

func TestPluginGuard_Check_transient_load_is_never_granted(t *testing.T) {
	t.Parallel()

	// Plugin ID 0 is a dry-run load; the repository checker refuses it
	// before touching the database.
	guard := NewGuard(NewRepositoryPermissionChecker(nil)).For(0)

	msg := guard.Check(context.Background(), ModuleNodeCmd, "execute_command")

	assert.Equal(t, "plugin permission node_commands required", msg)
}

func TestPluginGuard_Check_checker_failure_denies(t *testing.T) {
	t.Parallel()

	guard := NewGuard(stubPermissionChecker{err: errGuardTestDB}).For(guardTestPluginID)

	msg := guard.Check(context.Background(), ModuleServerControl, "stop_server")

	assert.Contains(t, msg, "failed to check plugin permission")
	assert.Contains(t, msg, "db down")
}

func TestPluginGuard_denial_is_audited_with_plugin_actor_and_initiator(t *testing.T) {
	t.Parallel()

	recorder := &auditRecorder{}
	denials := &denialRecorder{}
	guard := NewGuard(grantSet{},
		WithGuardAudit(recorder),
		WithGuardObserver(denials),
	).For(guardTestPluginID)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		User: &domain.User{ID: 77, Login: "operator"},
	})

	msg := guard.Check(ctx, ModuleNodeCmd, "execute_command")
	require.Equal(t, "plugin permission node_commands required", msg)

	events := recorder.all()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, audit.EventAccessDenied, event.Type)
	assert.Equal(t, audit.CategoryAuthorization, event.Category)
	assert.Equal(t, audit.OutcomeDenied, event.Outcome)
	assert.Equal(t, audit.AuthMethodPlugin, event.AuthMethod)
	assert.Equal(t, uint(guardTestPluginID), event.ActorID)
	assert.Equal(t, "host_function", event.ResourceType)
	assert.Equal(t, "gameap-nodecmd.execute_command", event.ResourceID)
	assert.Equal(t, "plugin_permission_missing", event.Reason)
	assert.Equal(t, "77", attrValue(event, "on_behalf_of_user_id"))
	assert.Equal(t, "operator", attrValue(event, "on_behalf_of_login"))
	assert.Equal(t, "node_commands", attrValue(event, "permission"))

	assert.Equal(t, []string{"gameap-nodecmd.execute_command:permission"}, denials.all())
}

func TestPluginGuard_denial_audit_is_throttled_but_metric_counts_every_call(t *testing.T) {
	t.Parallel()

	recorder := &auditRecorder{}
	denials := &denialRecorder{}
	guard := NewGuard(grantSet{},
		WithGuardAudit(recorder),
		WithGuardObserver(denials),
	).For(guardTestPluginID)

	for range 5 {
		guard.Check(context.Background(), ModuleNodeCmd, "execute_command")
	}

	assert.Len(t, recorder.all(), 1, "a plugin looping on a refused call must not flood the audit stream")
	assert.Len(t, denials.all(), 5, "every refusal is counted")
}

func TestPluginGuard_rate_limit(t *testing.T) {
	t.Parallel()

	recorder := &auditRecorder{}
	denials := &denialRecorder{}
	guard := NewGuard(grantSet{domain.PluginPermissionNodeCommands: true},
		WithGuardRateLimits(map[RateClass]RateLimit{
			RateClassNodeCmd: {RPS: 1, Burst: 2},
		}),
		WithGuardAudit(recorder),
		WithGuardObserver(denials),
	).For(guardTestPluginID)

	ctx := context.Background()

	assert.Empty(t, guard.Check(ctx, ModuleNodeCmd, "execute_command"))
	assert.Empty(t, guard.Check(ctx, ModuleNodeCmd, "execute_command"))

	msg := guard.Check(ctx, ModuleNodeCmd, "execute_command")
	assert.Equal(t, "rate limited: gameap-nodecmd allows 1 calls/s (burst 2)", msg)

	// The limit applies per class: another class is untouched.
	assert.Empty(t, guard.Check(ctx, ModuleHTTP, "fetch"))

	events := recorder.all()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventPluginHostCallRateLimited, events[0].Type)
	assert.Equal(t, audit.CategoryRateLimit, events[0].Category)
	assert.Equal(t, audit.OutcomeBlocked, events[0].Outcome)
	assert.Equal(t, audit.AuthMethodPlugin, events[0].AuthMethod)
	assert.Equal(t, "rate_limited", events[0].Reason)
	assert.Equal(t, "nodecmd", attrValue(events[0], "class"))

	assert.Equal(t, []string{"gameap-nodecmd.execute_command:rate_limit"}, denials.all())
}

func TestPluginGuard_rate_limit_is_per_plugin(t *testing.T) {
	t.Parallel()

	guard := NewGuard(grantSet{domain.PluginPermissionNodeCommands: true},
		WithGuardRateLimits(map[RateClass]RateLimit{RateClassNodeCmd: {RPS: 1, Burst: 1}}),
	)

	ctx := context.Background()

	first := guard.For(1)
	second := guard.For(2)

	assert.Empty(t, first.Check(ctx, ModuleNodeCmd, "execute_command"))
	assert.NotEmpty(t, first.Check(ctx, ModuleNodeCmd, "execute_command"), "first plugin's bucket is empty")
	assert.Empty(t, second.Check(ctx, ModuleNodeCmd, "execute_command"), "second plugin has its own bucket")

	guard.Forget(1)

	assert.Empty(t, first.Check(ctx, ModuleNodeCmd, "execute_command"), "forgotten bucket starts full again")
}

func TestPluginGuard_rate_limit_refills(t *testing.T) {
	t.Parallel()

	guard := NewGuard(grantSet{},
		WithGuardRateLimits(map[RateClass]RateLimit{RateClassHTTP: {RPS: 10, Burst: 1}}),
	)

	now := time.Unix(1_700_000_000, 0)
	guard.limiter.now = func() time.Time { return now }

	plugin := guard.For(guardTestPluginID)
	ctx := context.Background()

	assert.Empty(t, plugin.Check(ctx, ModuleHTTP, "fetch"))
	assert.NotEmpty(t, plugin.Check(ctx, ModuleHTTP, "fetch"))

	now = now.Add(100 * time.Millisecond)

	assert.Empty(t, plugin.Check(ctx, ModuleHTTP, "fetch"), "one token per 100ms at 10 rps")
}

func TestPluginGuard_rate_limit_disabled_class(t *testing.T) {
	t.Parallel()

	guard := NewGuard(grantSet{},
		WithGuardRateLimits(map[RateClass]RateLimit{RateClassHTTP: {RPS: 0, Burst: 5}}),
	).For(guardTestPluginID)

	for range 50 {
		assert.Empty(t, guard.Check(context.Background(), ModuleHTTP, "fetch"))
	}
}

func TestPluginGuard_Audit_records_outcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantOutcome audit.Outcome
		wantReason  string
	}{
		{name: "success", err: nil, wantOutcome: audit.OutcomeSuccess, wantReason: ""},
		{name: "operation_error", err: errGuardTestOperation, wantOutcome: audit.OutcomeFailure, wantReason: "operation_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := &auditRecorder{}
			guard := NewGuard(grantSet{}, WithGuardAudit(recorder)).For(guardTestPluginID)

			guard.Audit(context.Background(), audit.EventPluginNodeCommand, "execute", "node", "3", tt.err,
				slog.Int("exit_code", 0))

			events := recorder.all()
			require.Len(t, events, 1)
			assert.Equal(t, audit.EventPluginNodeCommand, events[0].Type)
			assert.Equal(t, audit.CategoryPluginOp, events[0].Category)
			assert.Equal(t, tt.wantOutcome, events[0].Outcome)
			assert.Equal(t, tt.wantReason, events[0].Reason)
			assert.Equal(t, audit.AuthMethodPlugin, events[0].AuthMethod)
			assert.Equal(t, "node", events[0].ResourceType)
			assert.Equal(t, "3", events[0].ResourceID)
			assert.Equal(t, "execute", events[0].Action)
			assert.Equal(t, "0", attrValue(events[0], "exit_code"))
			// The error text is returned to the guest, never written to the
			// audit stream.
			for _, attr := range events[0].Extra {
				assert.NotContains(t, attr.Value.String(), "daemon unreachable")
			}
		})
	}
}

func TestGuard_Forget_drops_the_cached_grants(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
	cache, _ := newTestCache(source, time.Hour)
	guard := NewGuard(cache)

	require.Empty(t, guard.For(7).Check(t.Context(), ModuleNodeFS, "read_dir"))

	// The uninstall deleted the record; without the drop the plugin would keep
	// its grant until the entry expired.
	source.set()
	guard.Forget(7)

	assert.Equal(t, PermissionDeniedMessage(domain.PluginPermissionFilesRead),
		guard.For(7).Check(t.Context(), ModuleNodeFS, "read_dir"))
}

// TestHostRPCPolicies_cover_generated_exports fails when a gated module
// gains a host function the policy table does not know: the SDK glue in
// sdk/<module>/<module>_host.pb.go is the ground truth of what a guest can
// import.
func TestHostRPCPolicies_cover_generated_exports(t *testing.T) {
	t.Parallel()

	exportPattern := regexp.MustCompile(`Export\("([a-z_]+)"\)`)

	modules := map[string]string{
		ModuleServerControl:  "servercontrol",
		ModuleNodeCmd:        "nodecmd",
		ModuleNodeFS:         "nodefs",
		ModuleRBAC:           "rbac",
		ModuleSecrets:        "secrets",
		ModuleHTTP:           "http",
		ModuleDaemonTasks:    "daemontasks",
		ModuleServers:        "servers",
		ModuleServerSettings: "serversettings",
		ModuleHost:           "host",
		ModuleSSH:            "ssh",
	}

	// Read-only functions of mixed modules that deliberately need no grant,
	// and the introspection module, open as a whole.
	open := map[HostRPC]struct{}{
		{ModuleDaemonTasks, "find_daemon_tasks"}:       {},
		{ModuleServers, "find_servers"}:                {},
		{ModuleServers, "get_server"}:                  {},
		{ModuleServerSettings, "find_server_settings"}: {},
		{ModuleHost, "get_grants"}:                     {},
		{ModuleHost, "get_config"}:                     {},
		{ModuleHost, "get_host_info"}:                  {},
		{ModuleHost, "report_status"}:                  {},
	}

	for module, dir := range modules {
		glue, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "plugin", "sdk", dir, dir+"_host.pb.go"))
		require.NoError(t, err, module)

		matches := exportPattern.FindAllStringSubmatch(string(glue), -1)
		require.NotEmpty(t, matches, module)

		for _, match := range matches {
			rpc := HostRPC{Module: module, Export: match[1]}
			if _, ok := open[rpc]; ok {
				_, gated := PolicyFor(module, match[1])
				assert.Falsef(t, gated, "%s.%s is documented as open", module, match[1])

				continue
			}

			_, ok := PolicyFor(module, match[1])
			assert.Truef(t, ok, "%s.%s has no policy entry", module, match[1])
		}
	}

	// And nothing in the table points at a function that does not exist.
	for rpc := range hostRPCPolicies {
		dir, ok := modules[rpc.Module]
		require.Truef(t, ok, "policy for unknown module %s", rpc.Module)

		glue, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "plugin", "sdk", dir, dir+"_host.pb.go"))
		require.NoError(t, err)
		assert.Truef(t, strings.Contains(string(glue), `Export("`+rpc.Export+`")`),
			"%s.%s is not exported by the SDK glue", rpc.Module, rpc.Export)
	}
}

func TestPermissionForImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		module   string
		export   string
		wantPerm domain.PluginPermission
		wantOK   bool
	}{
		{name: "nodecmd", module: ModuleNodeCmd, export: "execute_command", wantPerm: domain.PluginPermissionNodeCommands, wantOK: true},
		{name: "nodefs", module: ModuleNodeFS, export: "upload", wantPerm: domain.PluginPermissionFiles, wantOK: true},
		{name: "http_has_no_grant", module: ModuleHTTP, export: "fetch", wantOK: false},
		{name: "read_only_module", module: "gameap-users", export: "get_user", wantOK: false},
		{name: "unknown_function", module: ModuleNodeCmd, export: "nope", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			perm, ok := PermissionForImport(tt.module, tt.export)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantPerm, perm)
		})
	}
}

func attrValue(event audit.Event, key string) string {
	for _, attr := range event.Extra {
		if attr.Key == key {
			return attr.Value.String()
		}
	}

	return ""
}
