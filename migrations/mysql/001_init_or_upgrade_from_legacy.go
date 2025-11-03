package mysql

import (
	"context"
	"database/sql"
	"errors"
)

func Up001(ctx context.Context, tx *sql.Tx) error {
	var tableName string
	err := tx.QueryRowContext(
		ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'migrations'",
	).Scan(&tableName)

	if err == nil && tableName == "migrations" {
		return nil
	}

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	queries := []string{
		`CREATE TABLE abilities (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			name varchar(128) NOT NULL,
			title varchar(128) DEFAULT NULL,
			entity_id int(10) unsigned DEFAULT NULL,
			entity_type varchar(128) DEFAULT NULL,
			only_owned tinyint(1) NOT NULL DEFAULT 0,
			options text DEFAULT NULL,
			scope int(11) DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id),
			KEY abilities_scope_index (scope)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE client_certificates (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			fingerprint varchar(128) NOT NULL,
			expires timestamp NOT NULL,
			certificate varchar(128) NOT NULL,
			private_key varchar(128) NOT NULL,
			private_key_pass varchar(128) DEFAULT NULL,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE dedicated_servers (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			enabled tinyint(1) NOT NULL DEFAULT 0,
			name varchar(128) NOT NULL,
			os varchar(128) NOT NULL DEFAULT 'linux',
			location varchar(128) NOT NULL,
			provider varchar(128) DEFAULT NULL,
			ip text NOT NULL,
			ram varchar(128) DEFAULT NULL,
			cpu varchar(128) DEFAULT NULL,
			work_path varchar(128) NOT NULL,
			steamcmd_path varchar(128) DEFAULT NULL,
			gdaemon_host varchar(128) NOT NULL,
			gdaemon_port int(10) unsigned NOT NULL DEFAULT 31717,
			gdaemon_api_key varchar(128) NOT NULL,
			gdaemon_api_token char(64) DEFAULT NULL,
			gdaemon_login varchar(128) DEFAULT NULL,
			gdaemon_password varchar(128) DEFAULT NULL,
			gdaemon_server_cert varchar(128) NOT NULL,
			client_certificate_id int(10) unsigned NOT NULL,
			prefer_install_method enum('auto','copy','download','script','steam','none') NOT NULL DEFAULT 'auto',
			script_install text DEFAULT NULL,
			script_reinstall text DEFAULT NULL,
			script_update text DEFAULT NULL,
			script_start text DEFAULT NULL,
			script_pause text DEFAULT NULL,
			script_unpause text DEFAULT NULL,
			script_stop text DEFAULT NULL,
			script_kill text DEFAULT NULL,
			script_restart text DEFAULT NULL,
			script_status text DEFAULT NULL,
			script_stats text DEFAULT NULL,
			script_get_console text DEFAULT NULL,
			script_send_command text DEFAULT NULL,
			script_delete text DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			deleted_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE ds_stats (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			dedicated_server_id int(10) unsigned NOT NULL,
			time timestamp NOT NULL,
			loa varchar(128) DEFAULT NULL,
			ram varchar(128) NOT NULL,
			cpu varchar(128) NOT NULL,
			ifstat varchar(128) DEFAULT NULL,
			ping int(10) unsigned NOT NULL,
			drvspace varchar(128) NOT NULL,
			PRIMARY KEY (id),
			KEY ds_stats_dedicated_server_id_index (dedicated_server_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE ds_users (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			dedicated_server_id int(10) unsigned NOT NULL,
			username varchar(128) NOT NULL,
			uid int(10) unsigned NOT NULL,
			gid int(10) unsigned NOT NULL,
			password text NOT NULL,
			PRIMARY KEY (id),
			KEY ds_users_dedicated_server_id_index (dedicated_server_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE game_mods (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			game_code varchar(16) NOT NULL,
			name varchar(128) NOT NULL,
			fast_rcon text DEFAULT NULL,
			vars text DEFAULT NULL,
			remote_repository_linux text DEFAULT NULL,
			remote_repository_windows text DEFAULT NULL,
			local_repository_linux text DEFAULT NULL,
			local_repository_windows text DEFAULT NULL,
			start_cmd_linux text DEFAULT NULL,
			start_cmd_windows text DEFAULT NULL,
			kick_cmd varchar(64) DEFAULT NULL,
			ban_cmd varchar(64) DEFAULT NULL,
			chname_cmd varchar(64) DEFAULT NULL,
			srestart_cmd varchar(64) DEFAULT NULL,
			chmap_cmd varchar(64) DEFAULT NULL,
			sendmsg_cmd varchar(64) DEFAULT NULL,
			passwd_cmd varchar(64) DEFAULT NULL,
			PRIMARY KEY (id),
			KEY game_mods_game_code_index (game_code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE games (
			code varchar(16) NOT NULL,
			name varchar(128) NOT NULL,
			engine varchar(128) NOT NULL,
			engine_version varchar(128) NOT NULL DEFAULT '1.0',
			steam_app_id_linux int(10) unsigned DEFAULT NULL,
			steam_app_id_windows int(10) unsigned DEFAULT NULL,
			steam_app_set_config varchar(128) DEFAULT NULL,
			remote_repository_linux varchar(128) DEFAULT NULL,
			remote_repository_windows varchar(128) DEFAULT NULL,
			local_repository_linux varchar(128) DEFAULT NULL,
			local_repository_windows varchar(128) DEFAULT NULL,
			enabled tinyint(1) NOT NULL DEFAULT 1,
			PRIMARY KEY (code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE gdaemon_tasks (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			run_aft_id int(10) unsigned DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			dedicated_server_id int(10) unsigned NOT NULL,
			server_id int(10) unsigned DEFAULT NULL,
			task varchar(8) NOT NULL,
			data mediumtext DEFAULT NULL,
			cmd text DEFAULT NULL,
			output mediumtext DEFAULT NULL,
			status enum('waiting','working','error','success','canceled') NOT NULL DEFAULT 'waiting',
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE jobs (
			id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
			queue varchar(128) NOT NULL,
			payload longtext NOT NULL,
			attempts tinyint(3) unsigned NOT NULL,
			reserved_at int(10) unsigned DEFAULT NULL,
			available_at int(10) unsigned NOT NULL,
			created_at int(10) unsigned NOT NULL,
			PRIMARY KEY (id),
			KEY jobs_queue_index (queue)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE password_resets (
			email varchar(128) NOT NULL,
			token varchar(128) NOT NULL,
			created_at timestamp NULL DEFAULT NULL,
			KEY password_resets_email_index (email)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE permissions (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			ability_id int(10) unsigned NOT NULL,
			entity_id int(10) unsigned DEFAULT NULL,
			entity_type varchar(128) DEFAULT NULL,
			forbidden tinyint(1) NOT NULL DEFAULT 0,
			scope int(11) DEFAULT NULL,
			PRIMARY KEY (id),
			CONSTRAINT permissions_ability_id_foreign
				FOREIGN KEY (ability_id) REFERENCES abilities (id)
					ON UPDATE CASCADE ON DELETE CASCADE ,
			KEY permissions_ability_id_index (ability_id),
			KEY permissions_entity_index (entity_id, entity_type, scope),
			KEY permissions_scope_index (scope)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE personal_access_tokens (
			id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
			tokenable_type varchar(128) NOT NULL,
			tokenable_id bigint(20) unsigned NOT NULL,
			name varchar(128) NOT NULL,
			token varchar(64) NOT NULL,
			abilities text DEFAULT NULL,
			last_used_at timestamp NULL DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY personal_access_tokens_token_unique (token),
			KEY personal_access_tokens_tokenable_type_tokenable_id_index (tokenable_type,tokenable_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE roles (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			name varchar(128) NOT NULL,
			title varchar(128) DEFAULT NULL,
			level int(10) unsigned DEFAULT NULL,
			scope int(11) DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id),
			KEY roles_name_index (name),
			KEY roles_scope_index (scope)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE assigned_roles (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			role_id int(10) unsigned NOT NULL,
			entity_id int(10) unsigned NOT NULL,
			entity_type varchar(128) NOT NULL,
			restricted_to_id int(10) unsigned DEFAULT NULL,
			restricted_to_type varchar(128) DEFAULT NULL,
			scope int(11) DEFAULT NULL,
			PRIMARY KEY (id),
			CONSTRAINT assigned_roles_role_id_foreign
				FOREIGN KEY (role_id) REFERENCES roles (id)
					ON UPDATE CASCADE ON DELETE CASCADE,
			KEY assigned_roles_role_id_index (role_id),
			KEY assigned_roles_entity_id_index (entity_id),
			KEY assigned_roles_scope_index (scope)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE server_user (
			server_id int(10) unsigned NOT NULL,
			user_id int(10) unsigned NOT NULL,
			KEY server_user_server_id_index (server_id),
			KEY server_user_user_id_index (user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE servers (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			uuid varchar(36) NOT NULL,
			uuid_short varchar(8) NOT NULL,
			enabled tinyint(1) NOT NULL DEFAULT 0,
			installed int(11) NOT NULL DEFAULT 0,
			blocked tinyint(1) NOT NULL DEFAULT 0,
			name varchar(128) NOT NULL,
			game_id varchar(16) NOT NULL,
			ds_id int(10) unsigned NOT NULL,
			game_mod_id int(10) unsigned NOT NULL,
			expires timestamp NULL DEFAULT NULL,
			server_ip varchar(255) NOT NULL,
			server_port int(10) unsigned NOT NULL,
			query_port int(10) unsigned DEFAULT NULL,
			rcon_port int(10) unsigned DEFAULT NULL,
			rcon varchar(255) DEFAULT NULL,
			dir varchar(255) NOT NULL,
			su_user varchar(255) DEFAULT NULL,
			cpu_limit int(10) unsigned DEFAULT NULL,
			ram_limit int(10) unsigned DEFAULT NULL,
			net_limit int(10) unsigned DEFAULT NULL,
			start_command text DEFAULT NULL,
			stop_command text DEFAULT NULL,
			force_stop_command text DEFAULT NULL,
			restart_command text DEFAULT NULL,
			process_active tinyint(1) NOT NULL DEFAULT 0,
			last_process_check timestamp NULL DEFAULT NULL,
			vars text DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			deleted_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE servers_settings (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			name varchar(32) NOT NULL,
			server_id int(10) unsigned NOT NULL,
			value text NOT NULL,
			PRIMARY KEY (id),
			KEY servers_settings_name_index (name),
			KEY servers_settings_server_id_index (server_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE servers_stats (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			server_id int(10) unsigned NOT NULL,
			time timestamp NOT NULL,
			ram varchar(128) NOT NULL,
			cpu varchar(128) NOT NULL,
			netstat varchar(128) NOT NULL,
			drvspace varchar(128) NOT NULL,
			PRIMARY KEY (id),
			KEY servers_stats_server_id_index (server_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE servers_tasks (
			id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
			command varchar(16) NOT NULL,
			server_id int(11) NOT NULL,
			` + "`repeat`" + ` tinyint(3) unsigned NOT NULL DEFAULT 1,
			repeat_period int(11) NOT NULL DEFAULT 0,
			counter int(10) unsigned NOT NULL DEFAULT 0,
			execute_date timestamp NOT NULL,
			payload longtext DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id),
			KEY servers_tasks_server_id_index (server_id),
			KEY servers_tasks_execute_date_index (execute_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE servers_tasks_fails (
			id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
			server_task_id bigint(20) unsigned NOT NULL,
			output longtext NOT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id),
			KEY servers_tasks_fails_server_task_id_index (server_task_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE users (
			id int(10) unsigned NOT NULL AUTO_INCREMENT,
			login varchar(128) NOT NULL,
			email varchar(128) NOT NULL,
			password varchar(128) NOT NULL,
			remember_token varchar(100) DEFAULT NULL,
			name varchar(128) DEFAULT NULL,
			created_at timestamp NULL DEFAULT NULL,
			updated_at timestamp NULL DEFAULT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY users_login_unique (login),
			UNIQUE KEY users_email_unique (email)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}

	return nil
}

func Down001(_ context.Context, _ *sql.Tx) error {
	return nil
}
