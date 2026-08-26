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
	ID          Uint64ID `db:"id"`
	Name        string   `db:"name"`
	Version     string   `db:"version"`
	Description string   `db:"description"`
	Author      string   `db:"author"`
	APIVersion  string   `db:"api_version"`
	Filename    *string  `db:"filename"`
	Source      *string  `db:"source"`
	Homepage    *string  `db:"homepage"`
	// Checksum is the sha256 of the wasm file recorded at install; a peer
	// instance verifies a re-downloaded file against it.
	Checksum            *string            `db:"checksum"`
	RequiredPermissions []PluginPermission `db:"-"`
	AllowedPermissions  []PluginPermission `db:"-"`
	Status              PluginStatus       `db:"status"`
	Priority            int                `db:"priority"`
	// Generation is bumped by an operator reload so every panel instance
	// restarts the module on its next reconcile pass.
	Generation   int      `db:"generation"`
	Category     *string  `db:"category"`
	Dependencies []string `db:"-"`
	// Config holds the operator-provided configuration values, handed to the
	// plugin's Initialize.
	Config       map[string]any `db:"-"`
	InstalledAt  *time.Time     `db:"installed_at"`
	LastLoadedAt *time.Time     `db:"last_loaded_at"`
	// LastError explains the most recent load failure or runtime disable
	// (status "error"); cleared when the plugin loads successfully again.
	LastError   *string    `db:"last_error"`
	LastErrorAt *time.Time `db:"last_error_at"`
	CreatedAt   *time.Time `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
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
	// PluginPermissionFilesRead gates the read-only gameap-nodefs operations;
	// "files" includes it.
	PluginPermissionFilesRead    PluginPermission = "files_read"
	PluginPermissionListenEvents PluginPermission = "listen_events"
	PluginPermissionSecrets      PluginPermission = "secrets"
	// PluginPermissionNodeCommands gates arbitrary command execution on nodes
	// (gameap-nodecmd and cmdexec daemon tasks), separately from file access.
	PluginPermissionNodeCommands PluginPermission = "node_commands"
	// PluginPermissionSSH gates gameap-ssh: connections, command execution and
	// file transfers to hosts the plugin names itself, outside the daemon.
	PluginPermissionSSH PluginPermission = "ssh"
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
	PluginPermissionFilesRead,
	PluginPermissionListenEvents,
	PluginPermissionSecrets,
	PluginPermissionNodeCommands,
	PluginPermissionSSH,
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

// pluginPermissionSupersets lists, per permission, the broader grants that
// include it: a plugin granted "files" may do everything "files_read" allows.
var pluginPermissionSupersets = map[PluginPermission][]PluginPermission{
	PluginPermissionFilesRead: {PluginPermissionFiles},
}

// PluginPermissionSupersets reports the broader grants that include the
// permission; nil for permissions nothing else includes.
func PluginPermissionSupersets(permission PluginPermission) []PluginPermission {
	return pluginPermissionSupersets[permission]
}

// PermissionSatisfied reports whether the granted set contains the permission
// itself or a broader grant that includes it.
func PermissionSatisfied(permission PluginPermission, granted []PluginPermission) bool {
	if slices.Contains(granted, permission) {
		return true
	}

	for _, superset := range pluginPermissionSupersets[permission] {
		if slices.Contains(granted, superset) {
			return true
		}
	}

	return false
}

// PluginLoadState is the outcome of a load attempt, persisted with
// PluginRepository.UpdateLoadState so that it never overwrites concurrent
// edits of the configuration or the grants.
type PluginLoadState struct {
	Status       PluginStatus
	LastError    *string
	LastErrorAt  *time.Time
	LastLoadedAt *time.Time
	Generation   int
}

// LoadState extracts the load-related columns of the plugin.
func (p *Plugin) LoadState() PluginLoadState {
	return PluginLoadState{
		Status:       p.Status,
		LastError:    p.LastError,
		LastErrorAt:  p.LastErrorAt,
		LastLoadedAt: p.LastLoadedAt,
		Generation:   p.Generation,
	}
}

// MarkError records a failed load or a runtime disable: the plugin stays
// installed, is retried on the next panel start and can be reloaded by an
// operator. The reason is shown in the admin UI, so keep it short and free of
// secrets.
func (p *Plugin) MarkError(reason string, now time.Time) {
	p.Status = PluginStatusError
	p.LastError = new(reason)
	p.LastErrorAt = new(now)
}

// MarkActive records a successful load and clears the previous failure.
func (p *Plugin) MarkActive(now time.Time) {
	p.Status = PluginStatusActive
	p.LastError = nil
	p.LastErrorAt = nil
	p.LastLoadedAt = new(now)
}

// HasPermission reports whether the operator granted the plugin the given
// capability. Grants live in AllowedPermissions; RequiredPermissions is only
// what the plugin asked for.
func (p *Plugin) HasPermission(permission PluginPermission) bool {
	return PermissionSatisfied(permission, p.AllowedPermissions)
}
