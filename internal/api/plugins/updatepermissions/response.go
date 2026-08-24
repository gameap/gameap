package updatepermissions

import (
	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

type permissionsResponse struct {
	ID                  string   `json:"id"`
	RequiredPermissions []string `json:"required_permissions"`
	AllowedPermissions  []string `json:"allowed_permissions"`
	// Null when the plugin is not loaded on this instance (unknown), an
	// empty list when it is loaded and exercises nothing gated.
	UsedPermissions    []string `json:"used_permissions"`
	MissingPermissions []string `json:"missing_permissions"`
}

// newPermissionsResponse describes the plugin's grants after the update;
// loaded is the module on this instance when there is one.
func newPermissionsResponse(record *domain.Plugin, loaded *pkgplugin.LoadedPlugin) *permissionsResponse {
	resp := &permissionsResponse{
		ID:                  pkgplugin.CompactPluginID(record.ID),
		RequiredPermissions: internalplugin.PermissionNames(record.RequiredPermissions),
		AllowedPermissions:  internalplugin.PermissionNames(record.AllowedPermissions),
	}

	if loaded != nil {
		used := internalplugin.UsedPermissions(loaded.HostImports, loaded.SubscribedEvents)
		resp.UsedPermissions = internalplugin.PermissionNames(used)
		resp.MissingPermissions = internalplugin.PermissionNames(
			internalplugin.MissingPermissions(used, record.AllowedPermissions))
	}

	return resp
}
