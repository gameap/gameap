import { test, expect, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import path from 'node:path';

// A suspended (blocked) server: administrators keep its page but no longer get
// the buttons that would run it, can lift the suspension from the notice and
// suspend a server with a reason; the owner sees when and why in the servers
// list. The edit form sends the suspension only when it was changed there, so
// a form opened before a billing system suspended the server cannot lift it.
//
// The API is mocked: the tests exercise the UI without a backend or a node.

const UUID_V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
// The accessible name carries the icon glyph, hence no ^ anchor; the boundary
// keeps "Restart" and "Unsuspend" out.
const START = /(^|[^a-zа-яё])(start|запустить|servers\.start)\s*$/i;
const STOP = /(^|[^a-zа-яё])(stop|остановить|servers\.stop)\s*$/i;
const RESTART = /(restart|перезапустить|servers\.restart)\s*$/i;
const UPDATE = /(^|[^a-zа-яё])(update|обновить|servers\.update)\s*$/i;
const REINSTALL = /(reinstall|переустановить|servers\.reinstall)\s*$/i;
const SUSPEND = /(^|[^a-zа-яё])(suspend|приостановить|servers\.suspend)\s*$/i;
const UNSUSPEND = /(unsuspend|возобновить|servers\.unsuspend)\s*$/i;
const YES = /^\s*(yes|да|main\.yes)\s*$/i;

const SERVER_ID = 4252;
const REASON = 'Invoice #1042 is overdue';
const ENGLISH = JSON.parse(readFileSync(path.resolve(__dirname, '../../../../../internal/i18n/en.json'), 'utf8'));

const GAME = { code: 'e2esusp', name: 'E2E Suspension Game' };

const SERVER = {
  id: SERVER_ID,
  uid: '42524252-4252-4252-4252-425242524252',
  uuid: '42524252-4252-4252-4252-425242524252',
  uuid_short: '42524252',
  enabled: true,
  installed: 1,
  blocked: false,
  suspension: null,
  name: 'E2E Suspension Server',
  game_id: GAME.code,
  game_mod_id: 4253,
  ds_id: 4254,
  server_ip: '10.0.0.1',
  internal_server_ip: '10.0.0.1',
  server_port: 27015,
  query_port: 27015,
  rcon_port: 27015,
  rcon: 'rcon-secret',
  dir: 'servers/e2e',
  su_user: 'gameap',
  start_command: './start.sh',
  online: false,
  game: GAME,
  metadata: {},
  vars: {},
};

const SUSPENDED = {
  ...SERVER,
  blocked: true,
  suspension: { since: '2026-09-01T08:00:00Z', reason: REASON },
};

async function mockProfile(page: Page, roles: string[]): Promise<void> {
  await page.route('**/api/profile', (route) => route.fulfill({ json: { name: 'Test user', roles } }));
}

// The catch-all is registered first: routes registered later win.
async function mockServerPage(page: Page, server: typeof SERVER): Promise<void> {
  await page.route(`**/api/servers/${SERVER_ID}/**`, (route) => route.fulfill({ json: {} }));
  await page.route(`**/api/servers/${SERVER_ID}`, (route) => route.fulfill({ json: server }));
  await page.route(`**/api/servers/${SERVER_ID}/abilities`, (route) =>
    route.fulfill({
      json: {
        'game-server-common': true,
        'game-server-start': true,
        'game-server-stop': true,
        'game-server-restart': true,
        'game-server-update': true,
      },
    }),
  );
}

async function mockEditForm(page: Page, server: typeof SERVER, puts: unknown[]): Promise<void> {
  await page.route('**/api/games', (route) =>
    route.fulfill({ json: [{ ...GAME, engine: 'source', engine_version: '1', enabled: 1 }] }),
  );
  await page.route(`**/api/game_mods/get_list_for_game/${GAME.code}`, (route) =>
    route.fulfill({ json: [{ id: SERVER.game_mod_id, game_code: GAME.code, name: 'Default' }] }),
  );
  await page.route(`**/api/game_mods/${SERVER.game_mod_id}`, (route) =>
    route.fulfill({ json: { id: SERVER.game_mod_id, game_code: GAME.code, name: 'Default', vars: [] } }),
  );
  await page.route('**/api/nodes', (route) =>
    route.fulfill({ json: [{ id: SERVER.ds_id, name: 'E2E Node', enabled: true, os: 'linux', ip: ['10.0.0.1'] }] }),
  );
  await page.route(`**/api/nodes/${SERVER.ds_id}/ip_list`, (route) => route.fulfill({ json: ['10.0.0.1'] }));
  await page.route(`**/api/nodes/${SERVER.ds_id}/busy_ports`, (route) => route.fulfill({ json: {} }));
  await page.route(`**/api/servers/${SERVER_ID}`, async (route) => {
    if (route.request().method() === 'PUT') {
      puts.push(route.request().postDataJSON());
      await route.fulfill({ json: { status: 'ok' } });

      return;
    }

    await route.fulfill({ json: server });
  });
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('auth_token', 'suspension-ui-test'));
  await page.routeWebSocket('**/api/**', () => {});
  await page.route('**/api/**', (route) => route.fulfill({ json: {} }));
  await page.route('**/lang', (route) => route.fulfill({ json: [{ code: 'en', name: 'English' }] }));
  await page.route('**/lang/*.json', (route) => route.fulfill({ json: ENGLISH }));
  await page.route('**/plugins.js', (route) => route.fulfill({ body: '', contentType: 'application/javascript' }));
  await page.route('**/plugins.css', (route) => route.fulfill({ body: '', contentType: 'text/css' }));
});

test('administrator keeps the page of a suspended server without the buttons that would run it', async ({ page }) => {
  await mockProfile(page, ['admin']);
  await mockServerPage(page, SUSPENDED);

  await page.goto(`/servers/${SERVER_ID}`);

  const alert = page.getByTestId('server-suspended-alert');
  await expect(alert).toBeVisible();
  await expect(alert).toContainText('Server is suspended');
  await expect(alert).toContainText('Suspended since');
  await expect(alert).toContainText(`Reason: ${REASON}`);

  // The tabs are there: the control card and the link to the edit form.
  await expect(page.getByRole('link', { name: /admin|servers\.admin/i })).toBeVisible();
  await expect(page.getByText(/inactive|servers\.inactive/)).toBeVisible();

  const control = page.locator('#serverControl');
  for (const name of [START, RESTART, UPDATE, REINSTALL, SUSPEND]) {
    await expect(control.getByRole('button', { name })).toHaveCount(0);
  }
});

test('administrator can still stop a suspended server that runs', async ({ page }) => {
  await mockProfile(page, ['admin']);
  await mockServerPage(page, { ...SUSPENDED, online: true });

  await page.goto(`/servers/${SERVER_ID}`);

  const control = page.locator('#serverControl');
  await expect(control.getByRole('button', { name: STOP })).toBeVisible();
  await expect(control.getByRole('button', { name: RESTART })).toHaveCount(0);
});

test('administrator lifts a suspension from the notice', async ({ page }) => {
  await mockProfile(page, ['admin']);
  await mockServerPage(page, SUSPENDED);

  const keys: string[] = [];
  await page.route(`**/api/servers/${SERVER_ID}/unsuspend`, async (route) => {
    keys.push(route.request().headers()['idempotency-key'] ?? '');
    await route.fulfill({ json: { status: 'ok' } });
  });

  await page.goto(`/servers/${SERVER_ID}`);

  await page.getByTestId('server-suspended-alert').getByRole('button', { name: UNSUSPEND }).click();

  const confirmation = page.getByRole('dialog').last();
  await expect(confirmation).toContainText('It will not start by itself');
  await confirmation.getByRole('button', { name: YES }).click();

  await expect.poll(() => keys.length).toBe(1);
  expect(keys[0]).toMatch(UUID_V4);
  await expect(page.getByRole('dialog').last()).toContainText('Suspension lifted');
});

test('administrator suspends a running server with a reason', async ({ page }) => {
  await mockProfile(page, ['admin']);
  await mockServerPage(page, { ...SERVER, online: true });

  const requests: Array<{ key: string; body: unknown }> = [];
  await page.route(`**/api/servers/${SERVER_ID}/suspend`, async (route) => {
    requests.push({
      key: route.request().headers()['idempotency-key'] ?? '',
      body: route.request().postDataJSON(),
    });
    await route.fulfill({ json: { gdaemonTaskId: null } });
  });

  await page.goto(`/servers/${SERVER_ID}`);

  await page.locator('#serverControl').getByRole('button', { name: SUSPEND }).click();
  await page.getByTestId('server-suspend-reason').locator('input').fill(`  ${REASON}  `);
  await page.getByTestId('server-suspend-submit').click();

  await expect.poll(() => requests.length).toBe(1);
  expect(requests[0].key).toMatch(UUID_V4);
  expect(requests[0].body).toEqual({ reason: REASON });
  await expect(page.getByRole('dialog').last()).toContainText('Server suspended');
});

test('owner sees when and why a server is suspended in the servers list', async ({ page }) => {
  await mockProfile(page, ['user']);
  await page.route('**/api/servers?**', (route) =>
    route.fulfill({
      json: { current_page: 1, data: [SUSPENDED], from: 1, last_page: 1, per_page: 25, total: 1 },
    }),
  );

  await page.goto('/servers');

  const badge = page.getByTestId('server-suspension-badge');
  await expect(badge).toContainText('suspended');

  await badge.hover();
  await expect(page.getByText(`Reason: ${REASON}`)).toBeVisible();
  await expect(page.getByText(/Suspended since/)).toBeVisible();
});

test('edit form leaves a suspension alone unless it was changed there', async ({ page }) => {
  await mockProfile(page, ['admin']);
  const puts: unknown[] = [];
  await mockEditForm(page, SUSPENDED, puts);

  await page.goto(`/admin/servers/${SERVER_ID}/edit`);
  await expect(page.getByTestId('server-suspend-reason').locator('input')).toHaveValue(REASON);

  await page.getByTestId('server-edit-submit').click();
  await expect.poll(() => puts.length).toBe(1);

  expect(puts[0]).not.toHaveProperty('blocked');
  expect(puts[0]).not.toHaveProperty('suspend_reason');
});

test('edit form lifts a suspension when the switch is turned off', async ({ page }) => {
  await mockProfile(page, ['admin']);
  const puts: unknown[] = [];
  await mockEditForm(page, SUSPENDED, puts);

  await page.goto(`/admin/servers/${SERVER_ID}/edit`);
  await page.getByTestId('server-blocked-switch').click();
  await expect(page.getByTestId('server-suspension-fields')).toHaveCount(0);

  await page.getByTestId('server-edit-submit').click();
  await expect.poll(() => puts.length).toBe(1);

  expect(puts[0]).toMatchObject({ blocked: false });
  expect(puts[0]).not.toHaveProperty('suspend_reason');
});

test('edit form confirms a new suspension and sends its reason', async ({ page }) => {
  await mockProfile(page, ['admin']);
  const puts: unknown[] = [];
  await mockEditForm(page, SERVER, puts);

  await page.goto(`/admin/servers/${SERVER_ID}/edit`);
  await page.getByTestId('server-blocked-switch').click();
  await page.getByTestId('server-suspend-reason').locator('input').fill(REASON);
  await page.getByTestId('server-edit-submit').click();

  const confirmation = page.getByRole('dialog').last();
  await expect(confirmation).toContainText('cannot be started, updated or reinstalled');
  expect(puts, 'nothing is saved before the confirmation').toHaveLength(0);
  await confirmation.getByRole('button', { name: YES }).click();

  await expect.poll(() => puts.length).toBe(1);
  expect(puts[0]).toMatchObject({ blocked: true, suspend_reason: REASON });
});
