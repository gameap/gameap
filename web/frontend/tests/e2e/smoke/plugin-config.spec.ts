import { test, expect } from '@playwright/test';
import { gunzipSync } from 'node:zlib';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { loginViaAPI, authHeader } from '../fixtures/auth';
import { expectStatus, loginViaUI } from '../fixtures/ui';

// Plugin configuration form driven through the admin UI: install the
// `introspection` test plugin (a config_schema + gameap-host fixture), open
// its details, save typed values and a secret, verify the stored state
// through the API, then clear the secret again. A plugin installed by the
// test is uninstalled at the end; one that was already there is left alone.

const BASE_URL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';
const PLUGIN_NAME = 'introspection';
const FIXTURE = path.resolve(
  __dirname,
  '../../../../../pkg/plugin/compatrust/testdata/introspection.wasm.gz',
);

// The admin MFA nudge (AUTH_REQUIRE_MFA_FOR_ADMINS) opens a modal on the first
// page after login; it can be postponed.
const REMIND_LATER = /remind me later|напомнить позже|mfa_enforcement\.remind_later/i;

// The save notification is a naive-ui $dialog stacked over the details modal
// (itself a dialog with a card-header close "X"), so the notification is
// matched by its text and waited out as a whole instead of by its button.
async function dismissSaveNotification(page: import('@playwright/test').Page): Promise<void> {
  const dialog = page.getByRole('dialog').filter({ hasText: /configuration saved|конфигурация сохранена|plugins\.config_saved/i });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  await dialog.getByRole('button', { name: /close|закрыть|main\.close/i }).click();
  await expect(dialog).toBeHidden({ timeout: 10_000 });
}

async function dismissMfaNudge(page: import('@playwright/test').Page): Promise<void> {
  const remind = page.getByRole('button', { name: REMIND_LATER });
  if (await remind.isVisible({ timeout: 3_000 }).catch(() => false)) {
    await remind.click();
    await expect(remind).toBeHidden({ timeout: 10_000 });
  }
}

interface LoadedPlugin {
  id: string;
  name: string;
  has_config_schema: boolean;
  health?: { status: string; message?: string } | null;
}

interface ConfigView {
  values: Record<string, unknown>;
  secrets_set: string[];
}

type APIRequest = import('@playwright/test').APIRequestContext;

async function findPlugin(request: APIRequest, token: string): Promise<LoadedPlugin | undefined> {
  const response = await request.get(`${BASE_URL}/api/admin/plugins/loaded`, {
    headers: authHeader(token),
  });
  await expectStatus(response, 200, 'GET /api/admin/plugins/loaded should be 200');

  const body = (await response.json()) as { data: LoadedPlugin[] };

  return body.data.find((p) => p.name === PLUGIN_NAME && p.has_config_schema);
}

async function getConfig(request: APIRequest, token: string, id: string): Promise<ConfigView> {
  const response = await request.get(`${BASE_URL}/api/admin/plugins/${id}/config`, {
    headers: authHeader(token),
  });
  await expectStatus(response, 200, `GET /api/admin/plugins/${id}/config should be 200`);

  return (await response.json()) as ConfigView;
}

async function installFixture(request: APIRequest, token: string): Promise<void> {
  const response = await request.post(`${BASE_URL}/api/admin/plugins/upload/install`, {
    headers: authHeader(token),
    multipart: {
      file: {
        name: 'introspection.wasm',
        mimeType: 'application/wasm',
        buffer: gunzipSync(readFileSync(FIXTURE)),
      },
    },
  });
  await expectStatus(response, 200, 'POST /api/admin/plugins/upload/install should be 200');
}

async function uninstall(request: APIRequest, token: string, id: string): Promise<void> {
  const response = await request.delete(`${BASE_URL}/api/admin/plugins/${id}`, {
    headers: authHeader(token),
  });
  await expectStatus(response, 204, `DELETE /api/admin/plugins/${id} should be 204`);
}

async function resetConfig(request: APIRequest, token: string, id: string): Promise<void> {
  const response = await request.put(`${BASE_URL}/api/admin/plugins/${id}/config`, {
    headers: authHeader(token),
    data: { values: { token: '' } },
  });
  await expectStatus(response, 200, `PUT /api/admin/plugins/${id}/config (reset) should be 200`);
}

let adminToken: string | undefined;
let pluginId: string | undefined;
let installedByTest = false;

test.afterEach(async ({ request }) => {
  if (!adminToken || !pluginId) {
    return;
  }

  if (installedByTest) {
    await uninstall(request, adminToken, pluginId);
  } else {
    await resetConfig(request, adminToken, pluginId);
  }

  pluginId = undefined;
  installedByTest = false;
});

test('admin edits a plugin configuration through the details modal', async ({ page, request }) => {
  test.setTimeout(90_000);

  adminToken = await loginViaAPI(request);

  let plugin = await findPlugin(request, adminToken);
  if (!plugin) {
    await installFixture(request, adminToken);
    installedByTest = true;
    plugin = await findPlugin(request, adminToken);
  }
  expect(plugin, `${PLUGIN_NAME} must be installed with a config_schema`).toBeTruthy();
  pluginId = plugin!.id;
  await resetConfig(request, adminToken, pluginId);

  // 1. Sign in and open the installed plugins list.
  await loginViaUI(page, process.env.E2E_ADMIN_USER ?? 'admin', process.env.E2E_ADMIN_PASSWORD ?? '');
  await page.goto('/admin/plugins');
  await expect(page).toHaveURL(/\/admin\/plugins$/, { timeout: 15_000 });
  await dismissMfaNudge(page);

  const row = page.locator('.n-data-table-tr').filter({ hasText: PLUGIN_NAME }).first();
  await expect(row).toBeVisible({ timeout: 15_000 });
  await expect(row.getByTestId('plugin-configurable-badge')).toBeVisible();
  await expect(row.getByTestId('plugin-health-badge')).toBeVisible();

  // 2. Open the details; the configuration form renders the schema.
  await row.locator('span.font-medium').first().click();
  const form = page.getByTestId('plugin-config');
  await expect(form).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId('plugin-health-badge').last()).toBeVisible();

  const greeting = page.getByTestId('plugin-config-field-greeting').locator('input');
  const retries = page.getByTestId('plugin-config-field-retries').locator('input');
  const token = page.getByTestId('plugin-config-field-token').locator('input');
  await expect(greeting).toBeVisible();
  await expect(greeting).toHaveAttribute('placeholder', /hello/);

  // 3. A key the server refuses (bad name) is reported next to its row.
  await page.getByTestId('plugin-config-add-key').click();
  await page.getByTestId('plugin-config-extra-key-0').locator('input').fill('bad key!');
  await page.getByTestId('plugin-config-extra-value-0').locator('input').fill('x');
  const refused = page.waitForResponse(
    (r) => r.url().includes(`/api/admin/plugins/${pluginId}/config`) && r.request().method() === 'PUT',
  );
  await page.getByTestId('plugin-config-save').click();
  await expectStatus(await refused, 422, 'PUT config with a bad key should be 422');
  await expect(page.getByTestId('plugin-config-extra')).toContainText(/must match/, { timeout: 10_000 });
  await expect(page.getByTestId('plugin-config-error')).toBeVisible();
  await page.getByTestId('plugin-config-extra-remove-0').click();
  await expect(page.getByTestId('plugin-config-extra-key-0')).toBeHidden();

  // 4. Valid values and a secret are saved; the plugin is reloaded.
  await greeting.fill('hi from e2e');
  await retries.fill('5');
  await token.fill('e2e-secret-value');
  await page.getByTestId('plugin-config-field-verbose').click();

  const saved = page.waitForResponse(
    (r) => r.url().includes(`/api/admin/plugins/${pluginId}/config`) && r.request().method() === 'PUT',
  );
  await page.getByTestId('plugin-config-save').click();
  await expectStatus(await saved, 200, 'PUT config should be 200');
  await dismissSaveNotification(page);

  let config = await getConfig(request, adminToken, pluginId);
  expect(config.values).toMatchObject({ greeting: 'hi from e2e', retries: 5, verbose: true });
  expect(config.secrets_set).toEqual(['token']);

  // 5. The secret is masked: the input is empty and the clear checkbox appears.
  await expect(token).toHaveValue('');
  await expect(token).toHaveAttribute('placeholder', /leave blank|set/i);
  const clear = page.getByTestId('plugin-config-clear-token');
  await expect(clear).toBeVisible();

  // 6. Saving without touching the secret keeps it; clearing removes it.
  const kept = page.waitForResponse(
    (r) => r.url().includes(`/api/admin/plugins/${pluginId}/config`) && r.request().method() === 'PUT',
  );
  await page.getByTestId('plugin-config-save').click();
  await expectStatus(await kept, 200, 'PUT config (keep secret) should be 200');
  await dismissSaveNotification(page);
  config = await getConfig(request, adminToken, pluginId);
  expect(config.secrets_set).toEqual(['token']);

  await clear.click();
  const cleared = page.waitForResponse(
    (r) => r.url().includes(`/api/admin/plugins/${pluginId}/config`) && r.request().method() === 'PUT',
  );
  await page.getByTestId('plugin-config-save').click();
  await expectStatus(await cleared, 200, 'PUT config (clear secret) should be 200');
  await dismissSaveNotification(page);
  config = await getConfig(request, adminToken, pluginId);
  expect(config.secrets_set).toEqual([]);
  await expect(page.getByTestId('plugin-config-clear-token')).toBeHidden();
});
