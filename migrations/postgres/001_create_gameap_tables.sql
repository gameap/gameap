-- +goose Up

-- GameAP Database Tables Migration
-- This migration creates all the necessary tables for the GameAP application

-- Create abilities table (part of bouncer package)
CREATE TABLE abilities (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    title VARCHAR(128) DEFAULT NULL,
    entity_id INTEGER DEFAULT NULL,
    entity_type VARCHAR(128) DEFAULT NULL,
    only_owned BOOLEAN NOT NULL DEFAULT FALSE,
    options TEXT DEFAULT NULL,
    scope INTEGER DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL
);
CREATE INDEX abilities_scope_index ON abilities (scope);

-- Create client_certificates table
CREATE TABLE client_certificates (
    id SERIAL PRIMARY KEY,
    fingerprint VARCHAR(128) NOT NULL,
    expires TIMESTAMPTZ NOT NULL,
    certificate VARCHAR(128) NOT NULL,
    private_key VARCHAR(128) NOT NULL,
    private_key_pass VARCHAR(128) DEFAULT NULL
);

-- Create dedicated_servers table
CREATE TABLE dedicated_servers (
    id SERIAL PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    name VARCHAR(128) NOT NULL,
    os VARCHAR(128) NOT NULL DEFAULT 'linux',
    location VARCHAR(128) NOT NULL,
    provider VARCHAR(128) DEFAULT NULL,
    ip TEXT NOT NULL,
    ram VARCHAR(128) DEFAULT NULL,
    cpu VARCHAR(128) DEFAULT NULL,
    work_path VARCHAR(128) NOT NULL,
    steamcmd_path VARCHAR(128) DEFAULT NULL,
    gdaemon_host VARCHAR(128) NOT NULL,
    gdaemon_port SMALLINT NOT NULL DEFAULT 31717 CHECK (gdaemon_port BETWEEN 1 AND 65535),
    gdaemon_api_key VARCHAR(128) NOT NULL,
    gdaemon_api_token CHAR(64) DEFAULT NULL,
    gdaemon_login VARCHAR(128) DEFAULT NULL,
    gdaemon_password VARCHAR(128) DEFAULT NULL,
    gdaemon_server_cert VARCHAR(128) NOT NULL,
    client_certificate_id INTEGER NOT NULL,
    prefer_install_method VARCHAR(10) NOT NULL DEFAULT 'auto' CHECK (prefer_install_method IN ('auto','copy','download','script','steam','none')),
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
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL,
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

-- Create ds_stats table
CREATE TABLE ds_stats (
    id BIGSERIAL PRIMARY KEY,
    dedicated_server_id INTEGER NOT NULL,
    time TIMESTAMPTZ NOT NULL,
    loa VARCHAR(128) DEFAULT NULL,
    ram VARCHAR(128) NOT NULL,
    cpu VARCHAR(128) NOT NULL,
    ifstat VARCHAR(128) DEFAULT NULL,
    ping INTEGER NOT NULL,
    drvspace VARCHAR(128) NOT NULL
);
CREATE INDEX ds_stats_dedicated_server_id_index ON ds_stats (dedicated_server_id);

-- Create ds_users table
CREATE TABLE ds_users (
    id SERIAL PRIMARY KEY,
    dedicated_server_id INTEGER NOT NULL,
    username VARCHAR(128) NOT NULL,
    uid INTEGER NOT NULL,
    gid INTEGER NOT NULL,
    password TEXT NOT NULL
);
CREATE INDEX ds_users_dedicated_server_id_index ON ds_users (dedicated_server_id);

-- Create game_mods table
CREATE TABLE game_mods (
    id SERIAL PRIMARY KEY,
    game_code VARCHAR(16) NOT NULL,
    name VARCHAR(128) NOT NULL,
    fast_rcon JSONB DEFAULT NULL,
    vars JSONB DEFAULT NULL,
    remote_repository_linux TEXT DEFAULT NULL,
    remote_repository_windows TEXT DEFAULT NULL,
    local_repository_linux TEXT DEFAULT NULL,
    local_repository_windows TEXT DEFAULT NULL,
    start_cmd_linux TEXT DEFAULT NULL,
    start_cmd_windows TEXT DEFAULT NULL,
    kick_cmd VARCHAR(64) DEFAULT NULL,
    ban_cmd VARCHAR(64) DEFAULT NULL,
    chname_cmd VARCHAR(64) DEFAULT NULL,
    srestart_cmd VARCHAR(64) DEFAULT NULL,
    chmap_cmd VARCHAR(64) DEFAULT NULL,
    sendmsg_cmd VARCHAR(64) DEFAULT NULL,
    passwd_cmd VARCHAR(64) DEFAULT NULL
);
CREATE INDEX game_mods_game_code_index ON game_mods (game_code);

-- Create games table
CREATE TABLE games (
    code VARCHAR(16) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    engine VARCHAR(128) NOT NULL,
    engine_version VARCHAR(128) NOT NULL DEFAULT '1.0',
    steam_app_id_linux INTEGER DEFAULT NULL,
    steam_app_id_windows INTEGER DEFAULT NULL,
    steam_app_set_config VARCHAR(128) DEFAULT NULL,
    remote_repository_linux VARCHAR(128) DEFAULT NULL,
    remote_repository_windows VARCHAR(128) DEFAULT NULL,
    local_repository_linux VARCHAR(128) DEFAULT NULL,
    local_repository_windows VARCHAR(128) DEFAULT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE
);

-- Create gdaemon_tasks table
CREATE TABLE gdaemon_tasks (
    id BIGSERIAL PRIMARY KEY,
    run_aft_id BIGINT DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL,
    dedicated_server_id INTEGER NOT NULL,
    server_id INTEGER DEFAULT NULL,
    task VARCHAR(8) NOT NULL,
    data TEXT DEFAULT NULL,
    cmd TEXT DEFAULT NULL,
    output TEXT DEFAULT NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting','working','error','success','canceled'))
);

-- Create jobs table
CREATE TABLE jobs (
    id BIGSERIAL PRIMARY KEY,
    queue VARCHAR(128) NOT NULL,
    payload TEXT NOT NULL,
    attempts SMALLINT NOT NULL,
    reserved_at INTEGER DEFAULT NULL,
    available_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX jobs_queue_index ON jobs (queue);

-- Create password_resets table
CREATE TABLE password_resets (
    email VARCHAR(128) NOT NULL,
    token VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL
);
CREATE INDEX password_resets_email_index ON password_resets (email);

-- Create permissions table (part of bouncer package)
CREATE TABLE permissions (
    id SERIAL PRIMARY KEY,
    ability_id INTEGER NOT NULL,
    entity_id INTEGER DEFAULT NULL,
    entity_type VARCHAR(128) DEFAULT NULL,
    forbidden BOOLEAN NOT NULL DEFAULT FALSE,
    scope INTEGER DEFAULT NULL,
    CONSTRAINT permissions_ability_id_foreign
     FOREIGN KEY (ability_id) REFERENCES abilities (id)
         ON UPDATE CASCADE ON DELETE CASCADE
);
CREATE INDEX permissions_ability_id_index ON permissions (ability_id);
CREATE INDEX permissions_entity_index ON permissions (entity_id, entity_type, scope);
CREATE INDEX permissions_scope_index ON permissions (scope);

-- Create personal_access_tokens table
CREATE TABLE personal_access_tokens (
    id BIGSERIAL PRIMARY KEY,
    tokenable_type VARCHAR(128) NOT NULL,
    tokenable_id BIGINT NOT NULL,
    name VARCHAR(128) NOT NULL,
    token VARCHAR(64) NOT NULL,
    abilities TEXT DEFAULT NULL,
    last_used_at TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL,
    CONSTRAINT personal_access_tokens_token_unique UNIQUE (token)
);
CREATE INDEX personal_access_tokens_tokenable_type_tokenable_id_index ON personal_access_tokens (tokenable_type, tokenable_id);

-- Create roles table (part of bouncer package)
CREATE TABLE roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    title VARCHAR(128) DEFAULT NULL,
    level INTEGER DEFAULT NULL,
    scope INTEGER DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL
);
CREATE INDEX roles_name_index ON roles (name);
CREATE INDEX roles_scope_index ON roles (scope);

-- Create assigned_roles table (part of bouncer package)
CREATE TABLE assigned_roles (
    id SERIAL PRIMARY KEY,
    role_id INTEGER NOT NULL,
    entity_id INTEGER NOT NULL,
    entity_type VARCHAR(128) NOT NULL,
    restricted_to_id INTEGER DEFAULT NULL,
    restricted_to_type VARCHAR(128) DEFAULT NULL,
    scope INTEGER DEFAULT NULL,
    CONSTRAINT assigned_roles_role_id_foreign
        FOREIGN KEY (role_id) REFERENCES roles (id)
            ON UPDATE CASCADE ON DELETE CASCADE
);
CREATE INDEX assigned_roles_role_id_index ON assigned_roles (role_id);
CREATE INDEX assigned_roles_entity_id_index ON assigned_roles (entity_id);
CREATE INDEX assigned_roles_scope_index ON assigned_roles (scope);

-- Create server_user table
CREATE TABLE server_user (
    server_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL
);
CREATE INDEX server_user_server_id_index ON server_user (server_id);
CREATE INDEX server_user_user_id_index ON server_user (user_id);

-- Create servers table
CREATE TABLE servers (
    id SERIAL PRIMARY KEY,
    uuid UUID NOT NULL,
    uuid_short VARCHAR(8) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    installed SMALLINT NOT NULL DEFAULT 0,
    blocked BOOLEAN NOT NULL DEFAULT FALSE,
    name VARCHAR(128) NOT NULL,
    game_id VARCHAR(16) NOT NULL,
    ds_id INTEGER NOT NULL,
    game_mod_id INTEGER NOT NULL,
    expires TIMESTAMPTZ DEFAULT NULL,
    server_ip VARCHAR(255) NOT NULL,
    server_port SMALLINT NOT NULL CHECK (server_port BETWEEN 1 AND 65535),
    query_port SMALLINT DEFAULT NULL CHECK (query_port BETWEEN 1 AND 65535),
    rcon_port SMALLINT DEFAULT NULL CHECK (rcon_port BETWEEN 1 AND 65535),
    rcon VARCHAR(255) DEFAULT NULL,
    dir VARCHAR(255) NOT NULL,
    su_user VARCHAR(255) DEFAULT NULL,
    cpu_limit INTEGER DEFAULT NULL,
    ram_limit INTEGER DEFAULT NULL,
    net_limit INTEGER DEFAULT NULL,
    start_command TEXT DEFAULT NULL,
    stop_command TEXT DEFAULT NULL,
    force_stop_command TEXT DEFAULT NULL,
    restart_command TEXT DEFAULT NULL,
    process_active BOOLEAN NOT NULL DEFAULT FALSE,
    last_process_check TIMESTAMPTZ DEFAULT NULL,
    vars JSONB DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL,
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

-- Create servers_settings table
CREATE TABLE servers_settings (
    id SERIAL PRIMARY KEY,
    name VARCHAR(32) NOT NULL,
    server_id INTEGER NOT NULL,
    value TEXT NOT NULL
);
CREATE INDEX servers_settings_name_index ON servers_settings (name);
CREATE INDEX servers_settings_server_id_name_index ON servers_settings (server_id, name);

-- Create servers_stats table
CREATE TABLE servers_stats (
    id SERIAL PRIMARY KEY,
    server_id INTEGER NOT NULL,
    time TIMESTAMPTZ NOT NULL,
    ram VARCHAR(128) NOT NULL,
    cpu VARCHAR(128) NOT NULL,
    netstat VARCHAR(128) NOT NULL,
    drvspace VARCHAR(128) NOT NULL
);
CREATE INDEX servers_stats_server_id_index ON servers_stats (server_id);

-- Create servers_tasks table
CREATE TABLE servers_tasks (
    id BIGSERIAL PRIMARY KEY,
    command VARCHAR(16) NOT NULL,
    server_id INTEGER NOT NULL,
    repeat SMALLINT NOT NULL DEFAULT 1,
    repeat_period INTEGER NOT NULL DEFAULT 0,
    counter INTEGER NOT NULL DEFAULT 0,
    execute_date TIMESTAMPTZ NOT NULL,
    payload TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL
);
CREATE INDEX servers_tasks_server_id_index ON servers_tasks (server_id);
CREATE INDEX servers_tasks_execute_date_index ON servers_tasks (execute_date);

-- Create servers_tasks_fails table
CREATE TABLE servers_tasks_fails (
    id BIGSERIAL PRIMARY KEY,
    server_task_id BIGINT NOT NULL,
    output TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NULL,
    updated_at TIMESTAMPTZ DEFAULT NULL
);
CREATE INDEX servers_tasks_fails_server_task_id_index ON servers_tasks_fails (server_task_id);

-- Create users table
CREATE TABLE users (
   id SERIAL PRIMARY KEY,
   login VARCHAR(128) NOT NULL,
   email VARCHAR(128) NOT NULL,
   password VARCHAR(128) NOT NULL,
   remember_token VARCHAR(100) DEFAULT NULL,
   name VARCHAR(128) DEFAULT NULL,
   created_at TIMESTAMPTZ DEFAULT NULL,
   updated_at TIMESTAMPTZ DEFAULT NULL,
   CONSTRAINT users_login_unique UNIQUE (login),
   CONSTRAINT users_email_unique UNIQUE (email)
);