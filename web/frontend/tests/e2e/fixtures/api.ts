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

export interface GameDefinition {
  code: string;
  name: string;
  engine: string;
  engine_version?: string;
  steam_app_id_linux?: number;
  steam_app_id_windows?: number;
  remote_repository_linux?: string;
  remote_repository_windows?: string;
  local_repository_linux?: string;
  local_repository_windows?: string;
  enabled: number;
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

  if (response.status() === 409) {
    return;
  }

  if (!response.ok()) {
    throw new Error(
      `seed game failed: ${response.status()} ${await response.text()}`,
    );
  }
}
