package base

import (
	"strings"

	"github.com/gameap/gameap/internal/domain"
)

const GamesTable = "games"
const GameModsTable = "game_mods"
const ServersTable = "servers"
const UsersTable = "users"
const RolesTable = "roles"
const AssignedRolesTable = "assigned_roles"
const AbilitiesTable = "abilities"
const PermissionsTable = "permissions"
const PersonalAccessTokensTable = "personal_access_tokens"
const DaemonTasksTable = "gdaemon_tasks"
const ServerTasksTable = "servers_tasks"
const ServerTaskExecutionsTable = "servers_task_executions"
const ServerSettingsTable = "servers_settings"
const NodesTable = "dedicated_servers"
const ClientCertificatesTable = "client_certificates"
const PluginStorageTable = "plugin_storage"
const PluginsTable = "plugins"
const PluginScheduledTasksTable = "plugin_scheduled_tasks"
const PluginSecretsTable = "plugin_secrets"
const DLQTable = "pubsub_dlq"

var (
	GameFields                = allFields(domain.Game{})
	GameModFields             = allFields(domain.GameMod{})
	ServerFields              = allFields(domain.Server{})
	UserFields                = allFields(domain.User{})
	RoleFields                = allFields(domain.Role{})
	AssignedRoleFields        = allFields(domain.AssignedRole{})
	AbilityFields             = allFields(domain.Ability{})
	PermissionFields          = allFields(domain.Permission{})
	PersonalAccessTokenFields = allFields(domain.PersonalAccessToken{})
	DaemonTaskFields          = allFields(domain.DaemonTask{})
	ServerTaskFields          = allFields(domain.ServerTask{})
	ServerTaskExecutionFields = allFields(domain.ServerTaskExecution{})
	ServerSettingFields       = allFields(domain.ServerSetting{})
	NodeFields                = allFields(domain.Node{})
	ClientCertificateFields   = allFields(domain.ClientCertificate{})
	PluginStorageFields       = allFields(domain.PluginStorageEntry{})
	PluginScheduledTaskFields = allFields(domain.PluginScheduledTask{})
	PluginSecretFields        = allFields(domain.PluginSecret{})
)

// LikeEscapeChar is the escape character every dialect accepts in a LIKE
// ... ESCAPE clause; a backslash would need dialect-specific quoting.
const LikeEscapeChar = "!"

// LikePrefixPattern builds a LIKE pattern matching values that start with
// prefix, with the LIKE metacharacters of the prefix itself escaped. Use it
// with "... LIKE ? ESCAPE '!'".
func LikePrefixPattern(prefix string) string {
	escaped := strings.NewReplacer(
		LikeEscapeChar, LikeEscapeChar+LikeEscapeChar,
		"%", LikeEscapeChar+"%",
		"_", LikeEscapeChar+"_",
	).Replace(prefix)

	return escaped + "%"
}
