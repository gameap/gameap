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

interface CreateFormMocks {
  game?: { code: string; name: string };
  pool: string;
  busy: Record<string, number[]>;
  busyDelayMs?: number;
}

// mockCreateForm serves the create form one game, one node with the address
// 10.0.0.1 and the given pool; the busy ports may answer late.
async function mockCreateForm(page: Page, { game = GAME, pool, busy, busyDelayMs = 0 }: CreateFormMocks) {
  await page.route('**/api/games', (route) =>
    route.fulfill({ json: [{ ...game, engine: 'source', engine_version: '1', enabled: 1 }] }),
  );
  await page.route(`**/api/game_mods/get_list_for_game/${game.code}`, (route) =>
    route.fulfill({ json: [{ id: GAME_MOD_ID, game_code: game.code, name: 'Default' }] }),
  );
  await page.route(`**/api/game_mods/${GAME_MOD_ID}`, (route) =>
    route.fulfill({ json: { id: GAME_MOD_ID, game_code: game.code, name: 'Default', vars: [] } }),
  );
  await page.route('**/api/nodes', (route) =>
    route.fulfill({ json: [{ id: NODE_ID, name: 'E2E Ports Node', enabled: true, os: 'linux', ip: ['10.0.0.1'] }] }),
  );
  await page.route(`**/api/nodes/${NODE_ID}/ip_list`, (route) => route.fulfill({ json: ['10.0.0.1'] }));
  await page.route(`**/api/nodes/${NODE_ID}/busy_ports`, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, busyDelayMs));
    await route.fulfill({ json: busy });
  });
  await mockNode(page, { ...NODE, metadata: { port_range: pool } });
}

const serverPortInput = (page: Page) => page.locator('[name="server_port"] input');
const queryPortInput = (page: Page) => page.locator('[name="query_port"] input');
const rconPortInput = (page: Page) => page.locator('[name="rcon_port"] input');

test('create form picks the first free server port of the node pool', async ({ page }) => {
  test.setTimeout(60_000);

  // The busy ports land after the address and the pool, so the port picked
  // without them has to be picked again. 30001 is taken on the unspecified
  // address, which overlaps 10.0.0.1.
  await mockCreateForm(page, {
    pool: '30000-30010',
    busy: { '10.0.0.1': [30000, 30002], '0.0.0.0': [30001] },
    busyDelayMs: 500,
  });

  await page.goto('/admin/servers/create');
  await page.getByTestId('server-create-name').locator('input').fill(`${GAME.name} server`);

  // The game's default port lies outside the pool.
  await expect(serverPortInput(page)).toHaveValue('30003');
});

test('create form applies the port offsets of the game chosen after the address', async ({ page }) => {
  test.setTimeout(60_000);

  // Rust runs RCON on the port after the server port.
  const rust = { code: 'rust', name: 'E2E Rust' };
  await mockCreateForm(page, { game: rust, pool: '30000-30010', busy: { '10.0.0.1': [30000] } });

  await page.goto('/admin/servers/create');

  // Picked for the address before any game is chosen: every offset is zero.
  await expect(serverPortInput(page)).toHaveValue('30001');
  await expect(rconPortInput(page)).toHaveValue('30001');

  // Rust keeps 30001, the lowest free port of the pool, so only the RCON
  // port has to follow the game.
  await page.getByTestId('server-create-name').locator('input').fill(`${rust.name} server`);
  await expect(queryPortInput(page)).toHaveValue('30001');
  await expect(rconPortInput(page)).toHaveValue('30002');
  await expect(serverPortInput(page)).toHaveValue('30001');
});

test('create form keeps a port typed before the busy ports arrive', async ({ page }) => {
  test.setTimeout(60_000);

  await mockCreateForm(page, { pool: '30000-30010', busy: { '10.0.0.1': [30000] }, busyDelayMs: 1500 });

  await page.goto('/admin/servers/create');
  await expect(serverPortInput(page)).toHaveValue('30000');

  const busyPorts = page.waitForResponse((r) => r.url().endsWith(`/api/nodes/${NODE_ID}/busy_ports`));
  await serverPortInput(page).fill('30007');
  await serverPortInput(page).blur();
  await busyPorts;

  // A late re-pick would have moved the port to 30001.
  await expect(serverPortInput(page)).toHaveValue('30007');
  await expect(rconPortInput(page)).toHaveValue('30007');
});

test('create form leaves the port to the admin when the pool is full', async ({ page }) => {
  test.setTimeout(60_000);

  await mockCreateForm(page, { pool: '30000-30001', busy: { '10.0.0.1': [30000, 30001] } });

  await page.goto('/admin/servers/create');

  const exhausted = page.getByText(/no free port left in the port range|dedicated_servers\.port_range_exhausted/i);
  await expect(exhausted).toBeVisible();
  await expect(serverPortInput(page)).toHaveValue('');
  await expect(rconPortInput(page)).toHaveValue('');

  await serverPortInput(page).fill('31000');
  await serverPortInput(page).blur();

  await expect(exhausted).toBeHidden();
  await expect(queryPortInput(page)).toHaveValue('31000');
  await expect(rconPortInput(page)).toHaveValue('31000');
});

test('create form never picks a port below the field minimum', async ({ page }) => {
  test.setTimeout(60_000);

  await mockCreateForm(page, { pool: '1000-1030', busy: { '10.0.0.1': [1024] } });

  await page.goto('/admin/servers/create');

  await expect(serverPortInput(page)).toHaveValue('1025');
});
