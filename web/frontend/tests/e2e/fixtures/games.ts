import type { APIRequestContext } from '@playwright/test';
import { authHeader } from './auth';

const BASE_URL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';

// Mirrors getgame/response.go. steam_app_id_* come back as JSON numbers (or
// null); metadata is an object map (or null) — values round-trip as whatever
// JSON.parse produced on save (we use plain strings, so strings).
export interface GameResponse {
  code: string;
  name: string;
  engine: string;
  engine_version: string;
  steam_app_id_linux: number | null;
  steam_app_id_windows: number | null;
  steam_app_set_config: string | null;
  remote_repository_linux: string | null;
  remote_repository_windows: string | null;
  local_repository_linux: string | null;
  local_repository_windows: string | null;
  enabled: boolean;
  metadata: Record<string, unknown> | null;
}

// GET /api/games/{code} is admin-only — pass an admin token.
export async function getGame(
  request: APIRequestContext,
  adminToken: string,
  code: string,
): Promise<GameResponse> {
  const response = await request.get(`${BASE_URL}/api/games/${code}`, {
    headers: authHeader(adminToken),
  });

  if (!response.ok()) {
    throw new Error(
      `get game ${code} failed: ${response.status()} ${await response.text()}`,
    );
  }

  return (await response.json()) as GameResponse;
}

// deleteGame is best-effort teardown: 200 (deleted), 404 (already gone), and
// 422 (a server references it — shouldn't happen for a throwaway game) are all
// tolerated so cleanup never fails the run.
export async function deleteGame(
  request: APIRequestContext,
  adminToken: string,
  code: string,
): Promise<void> {
  const response = await request.delete(`${BASE_URL}/api/games/${code}`, {
    headers: authHeader(adminToken),
  });

  const status = response.status();
  if (status === 200 || status === 404 || status === 422) {
    return;
  }

  throw new Error(
    `delete game ${code} failed: ${status} ${await response.text()}`,
  );
}
