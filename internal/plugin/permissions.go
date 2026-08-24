package plugin

import (
	"slices"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/plugin/hostlibrary"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
)

// UsedPermissions derives the grants a plugin needs from what its module can
// actually do: every imported host function that is gated (hostlibrary's
// policy table) and, when it subscribes to events, listen_events. A guest can
// only call what it imports, so this is exact for host calls; it is
// informational — the panel grants only what a plugin declares — and lets
// the dry-run and the admin UI point out a plugin that will be denied.
func UsedPermissions(
	imports []pkgplugin.HostImport,
	subscribedEvents []proto.EventType,
) []domain.PluginPermission {
	used := make([]domain.PluginPermission, 0)

	for _, imp := range imports {
		permission, ok := hostlibrary.PermissionForImport(imp.Module, imp.Function)
		if !ok || slices.Contains(used, permission) {
			continue
		}

		used = append(used, permission)
	}

	if len(subscribedEvents) > 0 && !slices.Contains(used, domain.PluginPermissionListenEvents) {
		used = append(used, domain.PluginPermissionListenEvents)
	}

	// Stable order: the panel's own permission list.
	slices.SortFunc(used, func(a, b domain.PluginPermission) int {
		return slices.Index(domain.PluginPermissions, a) - slices.Index(domain.PluginPermissions, b)
	})

	return used
}

// MissingPermissions lists the used permissions the plugin does not hold.
func MissingPermissions(used, granted []domain.PluginPermission) []domain.PluginPermission {
	missing := make([]domain.PluginPermission, 0)

	for _, permission := range used {
		if !slices.Contains(granted, permission) {
			missing = append(missing, permission)
		}
	}

	return missing
}

// PermissionNames converts permissions for JSON responses.
func PermissionNames(permissions []domain.PluginPermission) []string {
	names := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		names = append(names, string(permission))
	}

	return names
}
