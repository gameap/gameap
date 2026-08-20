import type { APIRequestContext } from '@playwright/test';
import { authHeader } from './auth';

const BASE_URL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';

// Mirrors openapi/schemas/ServerSetting.yaml. `value` and `default` are typed
// after `type`: a boolean for bool, a number for int/float, a string otherwise.
export interface ServerSettingResponse {
  name: string;
  value: unknown;
  default: unknown;
  type: string;
  label: string;
  description?: string;
  options?: { value: string; label: string; i18n?: Record<string, { label: string }> }[];
  allow_custom?: boolean;
  rules?: Record<string, unknown>;
  i18n?: Record<string, { info?: string; description?: string }>;
  admin_var?: boolean;
}

export async function getServerSettings(
  request: APIRequestContext,
  token: string,
  serverId: number,
): Promise<ServerSettingResponse[]> {
  const response = await request.get(
    `${BASE_URL}/api/servers/${serverId}/settings`,
    { headers: authHeader(token) },
  );

  if (!response.ok()) {
    throw new Error(
      `get server settings failed: ${response.status()} ${await response.text()}`,
    );
  }

  return (await response.json()) as ServerSettingResponse[];
}

export interface ServerSettingInput {
  name: string;
  value: unknown;
}

// Returns the HTTP status so a test can assert the 422 path without the helper
// throwing first.
export async function putServerSettings(
  request: APIRequestContext,
  token: string,
  serverId: number,
  settings: ServerSettingInput[],
): Promise<{ status: number; body: string }> {
  const response = await request.put(
    `${BASE_URL}/api/servers/${serverId}/settings`,
    {
      headers: { ...authHeader(token), 'Content-Type': 'application/json' },
      data: settings,
    },
  );

  return { status: response.status(), body: await response.text() };
}

// markServerInstalled flips installed to 1 so the server page renders its tabs.
// PUT /api/servers/{id} is a full replace, so the current record is read back
// and sent again with only that field changed.
export async function markServerInstalled(
  request: APIRequestContext,
  token: string,
  serverId: number,
): Promise<void> {
  const current = await request.get(`${BASE_URL}/api/servers/${serverId}`, {
    headers: authHeader(token),
  });

  if (!current.ok()) {
    throw new Error(`get server ${serverId} failed: ${current.status()}`);
  }

  const server = (await current.json()) as Record<string, unknown>;

  const response = await request.put(`${BASE_URL}/api/servers/${serverId}`, {
    headers: { ...authHeader(token), 'Content-Type': 'application/json' },
    data: {
      name: server.name,
      ds_id: server.ds_id,
      game_id: server.game_id,
      game_mod_id: server.game_mod_id,
      server_ip: server.internal_server_ip ?? server.server_ip,
      server_port: server.server_port,
      query_port: server.query_port,
      rcon_port: server.rcon_port,
      dir: server.dir,
      enabled: true,
      installed: 1,
    },
  });

  if (!response.ok()) {
    throw new Error(
      `mark server ${serverId} installed failed: ${response.status()} ${await response.text()}`,
    );
  }
}
