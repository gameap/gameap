import type { APIRequestContext } from '@playwright/test';
import { authHeader } from './auth';

const BASE_URL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';

export interface CreateUserInput {
  login: string;
  email: string;
  password: string;
  name: string;
  roles?: string[];
}

export interface ThrowawayUser {
  id: number;
  login: string;
  email: string;
  password: string;
  name: string;
}

// createUser provisions a throwaway user via the admin-only endpoint.
// POST /api/users returns 201 with the created user (see
// internal/api/users/postusers/response.go) — the id is captured for teardown.
export async function createUser(
  request: APIRequestContext,
  adminToken: string,
  input: CreateUserInput,
): Promise<ThrowawayUser> {
  const response = await request.post(`${BASE_URL}/api/users`, {
    headers: { ...authHeader(adminToken), 'Content-Type': 'application/json' },
    data: {
      login: input.login,
      email: input.email,
      password: input.password,
      name: input.name,
      roles: input.roles ?? ['user'],
    },
  });

  if (response.status() !== 201) {
    throw new Error(
      `create user failed: ${response.status()} ${await response.text()}`,
    );
  }

  const body = (await response.json()) as { id?: number };
  if (typeof body.id !== 'number') {
    throw new Error(
      `create user response missing id: ${JSON.stringify(body)}`,
    );
  }

  return {
    id: body.id,
    login: input.login,
    email: input.email,
    password: input.password,
    name: input.name,
  };
}

// deleteUser is idempotent: a 204 (deleted now) and a 404 (already gone, e.g.
// a retried run) are both treated as success so teardown never fails the run.
export async function deleteUser(
  request: APIRequestContext,
  adminToken: string,
  id: number,
): Promise<void> {
  const response = await request.delete(`${BASE_URL}/api/users/${id}`, {
    headers: authHeader(adminToken),
  });

  if (response.status() === 204 || response.status() === 404) {
    return;
  }

  throw new Error(
    `delete user failed: ${response.status()} ${await response.text()}`,
  );
}

export interface ProfileResponse {
  login: string;
  email: string;
  name: string;
  roles: string[];
}

// getProfile reads the profile of whoever owns `token` — used to cross-check
// that a profile change persisted server-side, independent of the SPA state.
export async function getProfile(
  request: APIRequestContext,
  token: string,
): Promise<ProfileResponse> {
  const response = await request.get(`${BASE_URL}/api/profile`, {
    headers: authHeader(token),
  });

  if (!response.ok()) {
    throw new Error(`get profile failed: ${response.status()}`);
  }

  return (await response.json()) as ProfileResponse;
}
