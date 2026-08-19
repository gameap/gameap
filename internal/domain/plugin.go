package domain

import (
	"slices"
	"time"
)

type PluginStatus string

const (
	PluginStatusActive   PluginStatus = "active"
	PluginStatusDisabled PluginStatus = "disabled"
	PluginStatusError    PluginStatus = "error"
	PluginStatusUpdating PluginStatus = "updating"
)

type Plugin struct {
	ID                  Uint64ID           `db:"id"`
	Name                string             `db:"name"`
	Version             string             `db:"version"`
	Description         string             `db:"description"`
	Author              string             `db:"author"`
	APIVersion          string             `db:"api_version"`
	Filename            *string            `db:"filename"`
	Source              *string            `db:"source"`
	Homepage            *string            `db:"homepage"`
	RequiredPermissions []PluginPermission `db:"-"`
	AllowedPermissions  []PluginPermission `db:"-"`
	Status              PluginStatus       `db:"status"`
	Priority            int                `db:"priority"`
	Category            *string            `db:"category"`
	Dependencies        []string           `db:"-"`
	Config              map[string]any     `db:"-"`
	InstalledAt         *time.Time         `db:"installed_at"`
	LastLoadedAt        *time.Time         `db:"last_loaded_at"`
	CreatedAt           *time.Time         `db:"created_at"`
	UpdatedAt           *time.Time         `db:"updated_at"`
}

type PluginPermission string

const (
	PluginPermissionManageServers  PluginPermission = "manage_servers"
	PluginPermissionManageNodes    PluginPermission = "manage_nodes"
	PluginPermissionManageGames    PluginPermission = "manage_games"
	PluginPermissionManageGameMods PluginPermission = "manage_game_mods"
	PluginPermissionManageUsers    PluginPermission = "manage_users"
	PluginPermissionManageRBAC     PluginPermission = "manage_rbac"
	PluginPermissionFiles          PluginPermission = "files"
	PluginPermissionListenEvents   PluginPermission = "listen_events"
	PluginPermissionSecrets        PluginPermission = "secrets"
)

// PluginPermissions lists every permission the panel understands. A plugin
// manifest may declare anything; only values from this list survive
// ParsePluginPermission and reach the database.
var PluginPermissions = []PluginPermission{
	PluginPermissionManageServers,
	PluginPermissionManageNodes,
	PluginPermissionManageGames,
	PluginPermissionManageGameMods,
	PluginPermissionManageUsers,
	PluginPermissionManageRBAC,
	PluginPermissionFiles,
	PluginPermissionListenEvents,
	PluginPermissionSecrets,
}

// ParsePluginPermission converts a manifest string into a known permission.
// Unknown values are rejected so a plugin cannot invent its own capability
// name and have it persisted as if the panel had granted something.
func ParsePluginPermission(s string) (PluginPermission, bool) {
	for _, permission := range PluginPermissions {
		if PluginPermission(s) == permission {
			return permission, true
		}
	}

	return "", false
}

// ParsePluginPermissions maps manifest strings to known permissions,
// dropping anything unrecognised. Returns nil when nothing is recognised so
// the column stays NULL rather than an empty list.
func ParsePluginPermissions(values []string) []PluginPermission {
	if len(values) == 0 {
		return nil
	}

	permissions := make([]PluginPermission, 0, len(values))

	for _, value := range values {
		if permission, ok := ParsePluginPermission(value); ok {
			permissions = append(permissions, permission)
		}
	}

	if len(permissions) == 0 {
		return nil
	}

	return permissions
}

// HasPermission reports whether the operator granted the plugin the given
// capability. Grants live in AllowedPermissions; RequiredPermissions is only
// what the plugin asked for.
func (p *Plugin) HasPermission(permission PluginPermission) bool {
	return slices.Contains(p.AllowedPermissions, permission)
}
