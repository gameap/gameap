import type { APIRequestContext } from '@playwright/test';
import { authHeader } from './auth';

const BASE_URL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';

export interface NodeRecord {
  id: number;
  enabled: boolean;
  name: string;
  os: string;
  location: string;
  provider: string;
  ip: string[];
}

export async function listNodes(
  request: APIRequestContext,
  token: string,
): Promise<NodeRecord[]> {
  const response = await request.get(`${BASE_URL}/api/nodes`, {
    headers: authHeader(token),
  });

  if (!response.ok()) {
    throw new Error(`list nodes failed: ${response.status()}`);
  }

  return (await response.json()) as NodeRecord[];
}

// GameDefinition mirrors CreateGameRequest (openapi/schemas/requests/CreateGameRequest.yaml).
// steam_app_id_linux > 0 with an empty remote_repository_linux makes the daemon
// install via steamcmd; an empty steam app id with a repository URL makes it
// download and extract a tarball.
export interface GameDefinition {
  code: string;
  name: string;
  engine: string;
  engine_version?: string;
  steam_app_id_linux?: number | null;
  remote_repository_linux?: string | null;
}

export async function seedGame(
  request: APIRequestContext,
  token: string,
  game: GameDefinition,
): Promise<void> {
  const response = await request.post(`${BASE_URL}/api/games`, {
    headers: { ...authHeader(token), 'Content-Type': 'application/json' },
    data: game,
  });

  if (response.status() === 409 || response.ok()) {
    return;
  }

  throw new Error(
    `seed game failed: ${response.status()} ${await response.text()}`,
  );
}

export interface GameModVar {
  var: string;
  default?: string;
  info?: string;
  admin_var?: boolean;
}

// GameModDefinition mirrors CreateGameModRequest. CreateServerRequest requires a
// game_mod_id, so every seeded game needs at least one mod.
export interface GameModDefinition {
  game_code: string;
  name: string;
  start_cmd_linux?: string;
  remote_repository_linux?: string | null;
  vars?: GameModVar[];
}

export interface GameModRecord {
  id: number;
  game_code: string;
  name: string;
}

export async function listGameMods(
  request: APIRequestContext,
  token: string,
  gameCode: string,
): Promise<GameModRecord[]> {
  const response = await request.get(
    `${BASE_URL}/api/game_mods/get_list_for_game/${gameCode}`,
    { headers: authHeader(token) },
  );

  if (!response.ok()) {
    throw new Error(`list game mods failed: ${response.status()}`);
  }

  return (await response.json()) as GameModRecord[];
}

// createGameMod is idempotent: POST /api/game_mods returns only {status:"ok"}
// (no id), so on a re-run / Playwright retry we tolerate a duplicate and read
// the id back via listGameMods in getGameModId.
export async function createGameMod(
  request: APIRequestContext,
  token: string,
  mod: GameModDefinition,
): Promise<void> {
  const response = await request.post(`${BASE_URL}/api/game_mods`, {
    headers: { ...authHeader(token), 'Content-Type': 'application/json' },
    data: mod,
  });

  if (response.ok()) {
    return;
  }

  const existing = await listGameMods(request, token, mod.game_code);
  if (existing.some((m) => m.name === mod.name)) {
    return;
  }

  throw new Error(
    `create game mod failed: ${response.status()} ${await response.text()}`,
  );
}

export async function getGameModId(
  request: APIRequestContext,
  token: string,
  gameCode: string,
  modName: string,
): Promise<number> {
  const mods = await listGameMods(request, token, gameCode);
  const mod = mods.find((m) => m.name === modName);

  if (!mod) {
    throw new Error(`game mod "${modName}" not found for game "${gameCode}"`);
  }

  return mod.id;
}

export interface CreateServerInput {
  name: string;
  ds_id: number;
  game_id: string;
  game_mod_id: number;
  server_ip: string;
  server_port: number;
  install: boolean;
}

export interface CreateServerResult {
  taskId: number;
  serverId: number;
}

export async function createServer(
  request: APIRequestContext,
  token: string,
  input: CreateServerInput,
): Promise<CreateServerResult> {
  const response = await request.post(`${BASE_URL}/api/servers`, {
    headers: { ...authHeader(token), 'Content-Type': 'application/json' },
    data: input,
  });

  if (!response.ok()) {
    throw new Error(
      `create server failed: ${response.status()} ${await response.text()}`,
    );
  }

  const body = (await response.json()) as {
    result?: { taskId?: number; serverId?: number };
  };

  if (
    !body.result ||
    typeof body.result.taskId !== 'number' ||
    typeof body.result.serverId !== 'number'
  ) {
    throw new Error(
      `create server response missing result: ${JSON.stringify(body)}`,
    );
  }

  return { taskId: body.result.taskId, serverId: body.result.serverId };
}

export type DaemonTaskStatus =
  | 'waiting'
  | 'working'
  | 'error'
  | 'success'
  | 'canceled';

export interface DaemonTaskRecord {
  id: number;
  status: DaemonTaskStatus;
  task: string;
  output: string | null;
  server_id: number | null;
}

export async function getDaemonTask(
  request: APIRequestContext,
  token: string,
  id: number,
): Promise<DaemonTaskRecord> {
  const response = await request.get(`${BASE_URL}/api/gdaemon_tasks/${id}`, {
    headers: authHeader(token),
  });

  if (!response.ok()) {
    throw new Error(`get daemon task failed: ${response.status()}`);
  }

  return (await response.json()) as DaemonTaskRecord;
}

// /api/gdaemon_tasks/{id} omits `output` (route wired withOutput=false +
// json:"output,omitempty"). The captured daemon/steamcmd log lives only on the
// admin-only sibling endpoint, which the e2e admin token can reach.
export async function getDaemonTaskOutput(
  request: APIRequestContext,
  token: string,
  id: number,
): Promise<string> {
  const response = await request.get(
    `${BASE_URL}/api/gdaemon_tasks/${id}/output`,
    { headers: authHeader(token) },
  );

  if (!response.ok()) {
    return `<failed to fetch task output: ${response.status()}>`;
  }

  const body = (await response.json()) as { output?: string | null };

  return body.output ?? '(empty)';
}

// Matches openapi/schemas/common/ServerInstalledStatus.yaml.
export const ServerInstalled = {
  NotInstalled: 0,
  Installed: 1,
  InProgress: 2,
} as const;

export interface ServerRecord {
  id: number;
  installed: number;
  name: string;
  game_id: string;
}

export async function getServer(
  request: APIRequestContext,
  token: string,
  id: number,
): Promise<ServerRecord> {
  const response = await request.get(`${BASE_URL}/api/servers/${id}`, {
    headers: authHeader(token),
  });

  if (!response.ok()) {
    throw new Error(`get server failed: ${response.status()}`);
  }

  return (await response.json()) as ServerRecord;
}
