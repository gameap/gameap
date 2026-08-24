// Every permission the panel can grant to a plugin, in the panel's own
// order (mirrors domain.PluginPermissions on the backend). Labels live in
// i18n under plugins.permission_<name>.
export const PLUGIN_PERMISSIONS = [
  'manage_servers',
  'manage_nodes',
  'manage_games',
  'manage_game_mods',
  'manage_users',
  'manage_rbac',
  'files',
  'listen_events',
  'secrets',
  'node_commands',
]
