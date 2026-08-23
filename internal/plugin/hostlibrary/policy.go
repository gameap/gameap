package hostlibrary

import (
	"github.com/gameap/gameap/internal/domain"
)

// Host module names as the guest imports them.
const (
	ModuleServerControl  = "gameap-servercontrol"
	ModuleNodeCmd        = "gameap-nodecmd"
	ModuleDaemonTasks    = "gameap-daemontasks"
	ModuleServers        = "gameap-servers"
	ModuleServerSettings = "gameap-serversettings"
	ModuleNodeFS         = "gameap-nodefs"
	ModuleRBAC           = "gameap-rbac"
	ModuleSecrets        = "gameap-secrets"
	ModuleHTTP           = "gameap-http"
	ModuleCache          = "gameap-cache"
	ModuleStorage        = "gameap-storage"
	// ModuleHost is open to every plugin (introspection only).
	ModuleHost = "gameap-host"
)

// RateClass groups host functions that share one per-plugin token bucket.
type RateClass string

const (
	RateClassNodeCmd       RateClass = "nodecmd"
	RateClassServerControl RateClass = "servercontrol"
	RateClassNodeFS        RateClass = "nodefs"
	RateClassHTTP          RateClass = "http"
	RateClassRBAC          RateClass = "rbac"
)

// HostRPC identifies one host function by module and export name, exactly as
// the guest imports it (the names come from Export(...) in the generated
// sdk/<module>/<module>_host.pb.go files).
type HostRPC struct {
	Module string
	Export string
}

// RPCPolicy is what the panel enforces before a host function runs.
type RPCPolicy struct {
	// Permission is the narrowest grant the plugin must hold; empty means
	// the function is open to every plugin. A broader grant that includes
	// it (domain.PluginPermissionSupersets, e.g. "files" for "files_read")
	// satisfies the check as well.
	Permission domain.PluginPermission
	// RateClass selects the token bucket; empty means unlimited.
	RateClass RateClass
}

// hostRPCPolicies is the single source of truth for which grant and which
// rate limit each privileged host function is subject to. It drives the
// guard (enforcement), the "used permissions" the panel derives from a
// module's import section (dry-run and admin UI), and the documentation.
// Read-only modules (users, nodes, games, gamemods, authz, servers.find/get,
// ...) have no entry and need no grant.
var hostRPCPolicies = buildHostRPCPolicies()

func buildHostRPCPolicies() map[HostRPC]RPCPolicy {
	policies := make(map[HostRPC]RPCPolicy)

	add := func(module string, policy RPCPolicy, exports ...string) {
		for _, export := range exports {
			policies[HostRPC{Module: module, Export: export}] = policy
		}
	}

	add(ModuleServerControl,
		RPCPolicy{Permission: domain.PluginPermissionManageServers, RateClass: RateClassServerControl},
		"start_server", "stop_server", "restart_server", "update_server", "install_server", "reinstall_server")

	// create_daemon_task additionally requires node_commands for cmdexec
	// tasks; the module checks that itself once it knows the task type.
	add(ModuleDaemonTasks,
		RPCPolicy{Permission: domain.PluginPermissionManageServers, RateClass: RateClassServerControl},
		"create_daemon_task")

	add(ModuleServers,
		RPCPolicy{Permission: domain.PluginPermissionManageServers, RateClass: RateClassServerControl},
		"save_server", "delete_server")

	add(ModuleServerSettings,
		RPCPolicy{Permission: domain.PluginPermissionManageServers, RateClass: RateClassServerControl},
		"save_server_setting")

	add(ModuleNodeCmd,
		RPCPolicy{Permission: domain.PluginPermissionNodeCommands, RateClass: RateClassNodeCmd},
		"execute_command")

	// Reads need only files_read; "files" (read and write) includes it.
	add(ModuleNodeFS,
		RPCPolicy{Permission: domain.PluginPermissionFilesRead, RateClass: RateClassNodeFS},
		"read_dir", "download", "get_file_info", "hash", "get_archive_operation")

	add(ModuleNodeFS,
		RPCPolicy{Permission: domain.PluginPermissionFiles, RateClass: RateClassNodeFS},
		"mk_dir", "copy", "move", "upload", "remove", "chmod",
		"create_archive", "extract_archive", "start_create_archive", "start_extract_archive",
		"cancel_archive")

	add(ModuleRBAC,
		RPCPolicy{Permission: domain.PluginPermissionManageRBAC, RateClass: RateClassRBAC},
		"set_user_roles", "allow_user_abilities_for_entity", "revoke_or_forbid_user_abilities_for_entity",
		"get_roles", "save_role", "delete_role", "get_permissions", "get_roles_for_entity",
		"assign_roles_for_entity", "clear_roles_for_entity", "allow", "forbid", "revoke")

	add(ModuleSecrets,
		RPCPolicy{Permission: domain.PluginPermissionSecrets},
		"get", "set", "delete", "list_keys")

	add(ModuleHTTP,
		RPCPolicy{RateClass: RateClassHTTP},
		"fetch")

	return policies
}

// PolicyFor reports the policy of a host function; ok is false for functions
// open to every plugin.
func PolicyFor(module, export string) (RPCPolicy, bool) {
	policy, ok := hostRPCPolicies[HostRPC{Module: module, Export: export}]

	return policy, ok
}

// PermissionForImport reports the grant a plugin needs to call the imported
// host function; ok is false when none is needed.
func PermissionForImport(module, export string) (domain.PluginPermission, bool) {
	policy, ok := hostRPCPolicies[HostRPC{Module: module, Export: export}]
	if !ok || policy.Permission == "" {
		return "", false
	}

	return policy.Permission, true
}

// GatedModules lists the modules with at least one gated function, for
// documentation and tests.
func GatedModules() []string {
	seen := make(map[string]struct{})
	modules := make([]string, 0)

	for rpc := range hostRPCPolicies {
		if _, dup := seen[rpc.Module]; dup {
			continue
		}

		seen[rpc.Module] = struct{}{}
		modules = append(modules, rpc.Module)
	}

	return modules
}
