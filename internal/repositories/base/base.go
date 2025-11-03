package base

import (
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
const ServerTaskFailsTable = "servers_tasks_fails"
const ServerSettingsTable = "servers_settings"
const NodesTable = "dedicated_servers"
const ClientCertificatesTable = "client_certificates"

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
	ServerTaskFailFields      = allFields(domain.ServerTaskFail{})
	ServerSettingFields       = allFields(domain.ServerSetting{})
	NodeFields                = allFields(domain.Node{})
	ClientCertificateFields   = allFields(domain.ClientCertificate{})
)
