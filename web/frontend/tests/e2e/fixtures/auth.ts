import type { APIRequestContext } from '@playwright/test';

export interface LoginCredentials {
  login: string;
  password: string;
}

export async function loginViaAPI(
  request: APIRequestContext,
  credentials: LoginCredentials = {
    login: process.env.E2E_ADMIN_USER ?? 'admin',
    password: process.env.E2E_ADMIN_PASSWORD ?? '',
  },
): Promise<string> {
  if (!credentials.password) {
    throw new Error('E2E_ADMIN_PASSWORD must be set');
  }

  const baseURL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';
  const response = await request.post(`${baseURL}/api/auth/login`, {
    data: credentials,
  });

  if (!response.ok()) {
    throw new Error(
      `login failed: ${response.status()} ${await response.text()}`,
    );
  }

  const body = (await response.json()) as { token?: string };
  if (!body.token) {
    throw new Error('login response missing token');
  }

  return body.token;
}

export function authHeader(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` };
}
