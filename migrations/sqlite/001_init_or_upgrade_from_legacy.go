package sqlite

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
)

func Up001(ctx context.Context, tx *sql.Tx) error {
	var tableName string
	err := tx.QueryRowContext(
		ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='migrations'",
	).Scan(&tableName)

	if err == nil && tableName == "migrations" {
		return nil
	}

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	queries := []string{
		`CREATE TABLE abilities (
			id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			title TEXT DEFAULT NULL,
			entity_id INTEGER DEFAULT NULL,
			entity_type TEXT DEFAULT NULL,
			only_owned INTEGER NOT NULL DEFAULT 0,
			options TEXT DEFAULT NULL,
			scope INTEGER DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL
		)`,
		`CREATE INDEX abilities_scope_index ON abilities(scope)`,

		`CREATE TABLE client_certificates (
			id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			fingerprint TEXT NOT NULL,
			expires TEXT NOT NULL,
			certificate TEXT NOT NULL,
			private_key TEXT NOT NULL,
			private_key_pass TEXT DEFAULT NULL
		)`,

		`CREATE TABLE dedicated_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			enabled INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			os TEXT NOT NULL DEFAULT 'linux',
			location TEXT NOT NULL,
			provider TEXT DEFAULT NULL,
			ip TEXT NOT NULL,
			ram TEXT DEFAULT NULL,
			cpu TEXT DEFAULT NULL,
			work_path TEXT NOT NULL,
			steamcmd_path TEXT DEFAULT NULL,
			gdaemon_host TEXT NOT NULL,
			gdaemon_port INTEGER NOT NULL DEFAULT 31717,
			gdaemon_api_key TEXT NOT NULL,
			gdaemon_api_token TEXT DEFAULT NULL,
			gdaemon_login TEXT DEFAULT NULL,
			gdaemon_password TEXT DEFAULT NULL,
			gdaemon_server_cert TEXT NOT NULL,
			client_certificate_id INTEGER NOT NULL,
			prefer_install_method TEXT NOT NULL DEFAULT 'auto',
			script_install TEXT DEFAULT NULL,
			script_reinstall TEXT DEFAULT NULL,
			script_update TEXT DEFAULT NULL,
			script_start TEXT DEFAULT NULL,
			script_pause TEXT DEFAULT NULL,
			script_unpause TEXT DEFAULT NULL,
			script_stop TEXT DEFAULT NULL,
			script_kill TEXT DEFAULT NULL,
			script_restart TEXT DEFAULT NULL,
			script_status TEXT DEFAULT NULL,
			script_stats TEXT DEFAULT NULL,
			script_get_console TEXT DEFAULT NULL,
			script_send_command TEXT DEFAULT NULL,
			script_delete TEXT DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL,
			deleted_at TEXT DEFAULT NULL,
			CHECK(prefer_install_method IN ('auto','copy','download','script','steam','none'))
		)`,

		`CREATE TABLE ds_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dedicated_server_id INTEGER NOT NULL,
			time TEXT NOT NULL,
			loa TEXT DEFAULT NULL,
			ram TEXT NOT NULL,
			cpu TEXT NOT NULL,
			ifstat TEXT DEFAULT NULL,
			ping INTEGER NOT NULL,
			drvspace TEXT NOT NULL
		)`,
		`CREATE INDEX ds_stats_dedicated_server_id_index ON ds_stats(dedicated_server_id)`,

		`CREATE TABLE ds_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dedicated_server_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			uid INTEGER NOT NULL,
			gid INTEGER NOT NULL,
			password TEXT NOT NULL
		)`,
		`CREATE INDEX ds_users_dedicated_server_id_index ON ds_users(dedicated_server_id)`,

		`CREATE TABLE game_mods (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			game_code TEXT NOT NULL,
			name TEXT NOT NULL,
			fast_rcon JSONB DEFAULT NULL,
			vars JSONB DEFAULT NULL,
			remote_repository_linux TEXT DEFAULT NULL,
			remote_repository_windows TEXT DEFAULT NULL,
			local_repository_linux TEXT DEFAULT NULL,
			local_repository_windows TEXT DEFAULT NULL,
			start_cmd_linux TEXT DEFAULT NULL,
			start_cmd_windows TEXT DEFAULT NULL,
			kick_cmd TEXT DEFAULT NULL,
			ban_cmd TEXT DEFAULT NULL,
			chname_cmd TEXT DEFAULT NULL,
			srestart_cmd TEXT DEFAULT NULL,
			chmap_cmd TEXT DEFAULT NULL,
			sendmsg_cmd TEXT DEFAULT NULL,
			passwd_cmd TEXT DEFAULT NULL
		)`,
		`CREATE INDEX game_mods_game_code_index ON game_mods(game_code)`,

		`CREATE TABLE games (
			code TEXT NOT NULL PRIMARY KEY,
			name TEXT NOT NULL,
			engine TEXT NOT NULL,
			engine_version TEXT NOT NULL DEFAULT '1.0',
			steam_app_id_linux INTEGER DEFAULT NULL,
			steam_app_id_windows INTEGER DEFAULT NULL,
			steam_app_set_config TEXT DEFAULT NULL,
			remote_repository_linux TEXT DEFAULT NULL,
			remote_repository_windows TEXT DEFAULT NULL,
			local_repository_linux TEXT DEFAULT NULL,
			local_repository_windows TEXT DEFAULT NULL,
			enabled INTEGER NOT NULL DEFAULT 1
		)`,

		`CREATE TABLE gdaemon_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_aft_id INTEGER DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL,
			dedicated_server_id INTEGER NOT NULL,
			server_id INTEGER DEFAULT NULL,
			task TEXT NOT NULL,
			data TEXT DEFAULT NULL,
			cmd TEXT DEFAULT NULL,
			output TEXT DEFAULT NULL,
			status TEXT NOT NULL DEFAULT 'waiting',
			CHECK(status IN ('waiting','working','error','success','canceled'))
		)`,

		`CREATE TABLE jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			queue TEXT NOT NULL,
			payload TEXT NOT NULL,
			attempts INTEGER NOT NULL,
			reserved_at INTEGER DEFAULT NULL,
			available_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX jobs_queue_index ON jobs(queue)`,

		`CREATE TABLE password_resets (
			email TEXT NOT NULL,
			token TEXT NOT NULL,
			created_at TEXT DEFAULT NULL
		)`,
		`CREATE INDEX password_resets_email_index ON password_resets(email)`,

		`CREATE TABLE permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ability_id INTEGER NOT NULL REFERENCES abilities ON UPDATE CASCADE ON DELETE CASCADE,
			entity_id INTEGER DEFAULT NULL,
			entity_type TEXT DEFAULT NULL,
			forbidden INTEGER NOT NULL DEFAULT 0,
			scope INTEGER DEFAULT NULL
		)`,
		`CREATE INDEX permissions_ability_id_index ON permissions(ability_id)`,
		`CREATE INDEX permissions_entity_index ON permissions(entity_id, entity_type, scope)`,
		`CREATE INDEX permissions_scope_index ON permissions(scope)`,

		`CREATE TABLE personal_access_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tokenable_type TEXT NOT NULL,
			tokenable_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			token TEXT NOT NULL UNIQUE,
			abilities TEXT DEFAULT NULL,
			last_used_at TEXT DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL
		)`,
		`CREATE INDEX personal_access_tokens_tokenable_type_tokenable_id_index ON personal_access_tokens(tokenable_type, tokenable_id)`,

		`CREATE TABLE roles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			title TEXT DEFAULT NULL,
			level INTEGER DEFAULT NULL,
			scope INTEGER DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL
		)`,
		`CREATE INDEX roles_name_index ON roles(name)`,
		`CREATE INDEX roles_scope_index ON roles(scope)`,

		`CREATE TABLE assigned_roles (
			id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			role_id INTEGER NOT NULL references roles ON UPDATE CASCADE ON DELETE CASCADE,
			entity_id INTEGER NOT NULL,
			entity_type TEXT NOT NULL,
			restricted_to_id INTEGER DEFAULT NULL,
			restricted_to_type TEXT DEFAULT NULL,
			scope INTEGER DEFAULT NULL
		)`,
		`CREATE INDEX assigned_roles_role_id_index ON assigned_roles(role_id)`,
		`CREATE INDEX assigned_roles_entity_id_index ON assigned_roles(entity_id)`,
		`CREATE INDEX assigned_roles_scope_index ON assigned_roles(scope)`,

		`CREATE TABLE server_user (
			server_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL
		)`,
		`CREATE INDEX server_user_server_id_index ON server_user(server_id)`,
		`CREATE INDEX server_user_user_id_index ON server_user(user_id)`,

		`CREATE TABLE servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL,
			uuid_short TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			installed INTEGER NOT NULL DEFAULT 0,
			blocked INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			game_id TEXT NOT NULL,
			ds_id INTEGER NOT NULL,
			game_mod_id INTEGER NOT NULL,
			expires TEXT DEFAULT NULL,
			server_ip TEXT NOT NULL,
			server_port INTEGER NOT NULL,
			query_port INTEGER DEFAULT NULL,
			rcon_port INTEGER DEFAULT NULL,
			rcon TEXT DEFAULT NULL,
			dir TEXT NOT NULL,
			su_user TEXT DEFAULT NULL,
			cpu_limit INTEGER DEFAULT NULL,
			ram_limit INTEGER DEFAULT NULL,
			net_limit INTEGER DEFAULT NULL,
			start_command TEXT DEFAULT NULL,
			stop_command TEXT DEFAULT NULL,
			force_stop_command TEXT DEFAULT NULL,
			restart_command TEXT DEFAULT NULL,
			process_active INTEGER NOT NULL DEFAULT 0,
			last_process_check TEXT DEFAULT NULL,
			vars JSONB DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL,
			deleted_at TEXT DEFAULT NULL
		)`,

		`CREATE TABLE servers_settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			server_id INTEGER NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE INDEX servers_settings_name_index ON servers_settings(name)`,
		`CREATE INDEX servers_settings_server_id_index ON servers_settings(server_id)`,

		`CREATE TABLE servers_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL,
			time TEXT NOT NULL,
			ram TEXT NOT NULL,
			cpu TEXT NOT NULL,
			netstat TEXT NOT NULL,
			drvspace TEXT NOT NULL
		)`,
		`CREATE INDEX servers_stats_server_id_index ON servers_stats(server_id)`,

		`CREATE TABLE servers_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL,
			server_id INTEGER NOT NULL,
			repeat INTEGER NOT NULL DEFAULT 1,
			repeat_period INTEGER NOT NULL DEFAULT 0,
			counter INTEGER NOT NULL DEFAULT 0,
			execute_date TEXT NOT NULL,
			payload TEXT DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL
		)`,
		`CREATE INDEX servers_tasks_server_id_index ON servers_tasks(server_id)`,
		`CREATE INDEX servers_tasks_execute_date_index ON servers_tasks(execute_date)`,

		`CREATE TABLE servers_tasks_fails (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_task_id INTEGER NOT NULL,
			output TEXT NOT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL
		)`,
		`CREATE INDEX servers_tasks_fails_server_task_id_index ON servers_tasks_fails(server_task_id)`,

		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			login TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			remember_token TEXT DEFAULT NULL,
			name TEXT DEFAULT NULL,
			created_at TEXT DEFAULT NULL,
			updated_at TEXT DEFAULT NULL
		)`,
	}

	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}

	return nil
}

func Down001(ctx context.Context, tx *sql.Tx) error {
	return nil
}
