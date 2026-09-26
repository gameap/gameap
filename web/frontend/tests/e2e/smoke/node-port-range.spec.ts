import { test, expect, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { dismissTopDialog } from '../fixtures/ui';

// A node's pool of ports for new servers lives in its metadata under
// `port_range`, but the node form edits it in a field of its own, and the
// create-server form picks a free server port from it.
//
// The API is mocked: no node, game or administrator has to exist.

const NODE_ID = 4252;
const GAME_MOD_ID = 4253;
const GAME = { code: 'e2eports', name: 'E2E Ports Game' };
const METADATA_TAB = /metadata|метаданные|dedicated_servers\.metadata/i;
const MAIN_TAB = /^\s*(main|основное|dedicated_servers\.main)\s*$/i;
const SAVE = /save|сохранить|main\.save/i;
const INVALID_RANGE = /list ports and ranges|перечислите через запятую|dedicated_servers\.port_range_invalid/i;
const ENGLISH = JSON.parse(readFileSync(path.resolve(__dirname, '../../../../../internal/i18n/en.json'), 'utf8'));

const NODE = {
  id: NODE_ID,
  enabled: true,
  name: 'E2E Ports Node',
  os: 'linux',
  location: 'DE',
  provider: 'Hetzner',
  ip: ['10.0.0.1'],
  work_path: '/srv/gameap',
  steamcmd_path: '/srv/gameap/steamcmd',
  gdaemon_host: '10.0.0.1',
  gdaemon_port: 31717,
  gdaemon_server_cert: 'certs/node.crt',
  client_certificate_id: 1,
  prefer_install_method: 'auto',
  metadata: { region: 'fsn1', port_range: '27015-27020' },
};

async function mockNode(page: Page, node: Record<string, unknown>): Promise<void> {
  await page.route(`**/api/nodes/${NODE_ID}`, (route) => route.fulfill({ json: node }));
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('auth_token', 'port-range-ui-test'));
  await page.routeWebSocket('**/api/**', () => {});
  await page.route('**/api/**', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/profile', (route) =>
    route.fulfill({ json: { name: 'Test administrator', roles: ['admin'] } }),
  );
  await page.route('**/lang', (route) => route.fulfill({ json: [{ code: 'en', name: 'English' }] }));
  await page.route('**/lang/*.json', (route) => route.fulfill({ json: ENGLISH }));
  await page.route('**/plugins.js', (route) => route.fulfill({ body: '', contentType: 'application/javascript' }));
  await page.route('**/plugins.css', (route) => route.fulfill({ body: '', contentType: 'text/css' }));
  await page.route('**/api/client_certificates', (route) =>
    route.fulfill({ json: [{ id: 1, fingerprint: 'AA:BB:CC', expires: '2030-01-01T00:00:00Z' }] }),
  );
});

test('node form edits the port pool in its own field and stores it in metadata', async ({ page }) => {
  test.setTimeout(60_000);

  let putBody: Record<string, unknown> | undefined;
  await page.route(`**/api/nodes/${NODE_ID}`, async (route) => {
    if (route.request().method() === 'PUT') {
      putBody = route.request().postDataJSON() as Record<string, unknown>;
    }

    await route.fulfill({ json: NODE });
  });

  await page.goto(`/admin/nodes/${NODE_ID}/edit`);

  const portRange = page.getByTestId('node-port-range').locator('input');
  await expect(portRange).toHaveValue('27015-27020');

  // The pool is not repeated as a raw metadata row.
  await page.locator('.n-tabs-tab', { hasText: METADATA_TAB }).first().click();
  const rows = page.getByTestId('node-metadata').locator('tbody tr');
  await expect(rows).toHaveCount(1);
  await expect(rows.first().locator('input').first()).toHaveValue('region');

  await page.locator('.n-tabs-tab', { hasText: MAIN_TAB }).first().click();

  // An unreadable pool is refused before it reaches the API.
  await portRange.fill('27015-');
  await page.getByRole('button', { name: SAVE }).first().click();
  await expect(page.getByText(INVALID_RANGE)).toBeVisible();
  expect(putBody).toBeUndefined();

  await portRange.fill('30000-30010, 31000');

  const updateResp = page.waitForResponse(
    (r) => r.url().includes(`/api/nodes/${NODE_ID}`) && r.request().method() === 'PUT',
  );
  await page.getByRole('button', { name: SAVE }).first().click();
  await updateResp;

  expect(putBody).toBeDefined();
  expect(putBody).not.toHaveProperty('port_range');
  expect(putBody?.metadata).toEqual({ region: 'fsn1', port_range: '30000-30010, 31000' });

  await dismissTopDialog(page);
});

test('clearing the field drops the pool from metadata', async ({ page }) => {
  test.setTimeout(60_000);

  let putBody: Record<string, unknown> | undefined;
  await page.route(`**/api/nodes/${NODE_ID}`, async (route) => {
    if (route.request().method() === 'PUT') {
      putBody = route.request().postDataJSON() as Record<string, unknown>;
    }

    await route.fulfill({ json: NODE });
  });

  await page.goto(`/admin/nodes/${NODE_ID}/edit`);

  const portRange = page.getByTestId('node-port-range').locator('input');
  await expect(portRange).toHaveValue('27015-27020');
  await portRange.fill('');

  const updateResp = page.waitForResponse(
    (r) => r.url().includes(`/api/nodes/${NODE_ID}`) && r.request().method() === 'PUT',
  );
  await page.getByRole('button', { name: SAVE }).first().click();
  await updateResp;

  expect(putBody?.metadata).toEqual({ region: 'fsn1' });

  await dismissTopDialog(page);
});

test('create form picks the first free server port of the node pool', async ({ page }) => {
  test.setTimeout(60_000);

  await page.route('**/api/games', (route) =>
    route.fulfill({ json: [{ ...GAME, engine: 'source', engine_version: '1', enabled: 1 }] }),
  );
  await page.route(`**/api/game_mods/get_list_for_game/${GAME.code}`, (route) =>
    route.fulfill({ json: [{ id: GAME_MOD_ID, game_code: GAME.code, name: 'Default' }] }),
  );
  await page.route(`**/api/game_mods/${GAME_MOD_ID}`, (route) =>
    route.fulfill({ json: { id: GAME_MOD_ID, game_code: GAME.code, name: 'Default', vars: [] } }),
  );
  await page.route('**/api/nodes', (route) =>
    route.fulfill({ json: [{ id: NODE_ID, name: 'E2E Ports Node', enabled: true, os: 'linux', ip: ['10.0.0.1'] }] }),
  );
  await page.route(`**/api/nodes/${NODE_ID}/ip_list`, (route) => route.fulfill({ json: ['10.0.0.1'] }));
  // The busy ports land after the address and the pool, so the port picked
  // without them has to be picked again.
  await page.route(`**/api/nodes/${NODE_ID}/busy_ports`, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 500));
    await route.fulfill({ json: { '10.0.0.1': [30000, 30002] } });
  });
  await mockNode(page, { ...NODE, metadata: { port_range: '30000-30010' } });

  await page.goto('/admin/servers/create');
  await page.getByTestId('server-create-name').locator('input').fill(`${GAME.name} server`);

  // 30000 and 30002 are taken; the game's default port lies outside the pool.
  await expect(page.locator('[name="server_port"] input')).toHaveValue('30001');
});

test('create form applies the port offsets of the game chosen after the address', async ({ page }) => {
  test.setTimeout(60_000);

  // Rust runs RCON on the port after the server port.
  const rust = { code: 'rust', name: 'E2E Rust' };
  await page.route('**/api/games', (route) =>
    route.fulfill({ json: [{ ...rust, engine: 'rust', engine_version: '1', enabled: 1 }] }),
  );
  await page.route(`**/api/game_mods/get_list_for_game/${rust.code}`, (route) =>
    route.fulfill({ json: [{ id: GAME_MOD_ID, game_code: rust.code, name: 'Vanilla' }] }),
  );
  await page.route(`**/api/game_mods/${GAME_MOD_ID}`, (route) =>
    route.fulfill({ json: { id: GAME_MOD_ID, game_code: rust.code, name: 'Vanilla', vars: [] } }),
  );
  await page.route('**/api/nodes', (route) =>
    route.fulfill({ json: [{ id: NODE_ID, name: 'E2E Ports Node', enabled: true, os: 'linux', ip: ['10.0.0.1'] }] }),
  );
  await page.route(`**/api/nodes/${NODE_ID}/ip_list`, (route) => route.fulfill({ json: ['10.0.0.1'] }));
  await page.route(`**/api/nodes/${NODE_ID}/busy_ports`, (route) =>
    route.fulfill({ json: { '10.0.0.1': [30000] } }),
  );
  await mockNode(page, { ...NODE, metadata: { port_range: '30000-30010' } });

  await page.goto('/admin/servers/create');

  // Picked for the address before any game is chosen: every offset is zero.
  const serverPort = page.locator('[name="server_port"] input');
  const rconPort = page.locator('[name="rcon_port"] input');
  await expect(serverPort).toHaveValue('30001');
  await expect(rconPort).toHaveValue('30001');

  // Rust keeps 30001, the lowest free port of the pool, so only the RCON
  // port has to follow the game.
  await page.getByTestId('server-create-name').locator('input').fill(`${rust.name} server`);
  await expect(page.locator('[name="query_port"] input')).toHaveValue('30001');
  await expect(rconPort).toHaveValue('30002');
  await expect(serverPort).toHaveValue('30001');
});
