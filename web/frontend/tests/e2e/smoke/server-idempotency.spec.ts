import { test, expect, type Page, type Route } from '@playwright/test';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { dismissTopDialog } from '../fixtures/ui';

// Creating, editing and starting a server send an Idempotency-Key. A retry
// after a request with an unknown outcome must reuse the key and body, so the backend
// replays the first outcome instead of creating the server (or queueing the
// task) twice; a new submission after an answer must get a new key.
//
// The API is mocked: these tests exercise the real UI request lifecycle without
// depending on a running backend, an enrolled node or administrator credentials.

const UUID_V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
// The name carries the icon glyph; the boundary keeps "Restart" out.
const START = /(^|[^a-zа-яё])(start|запустить|servers\.start)\s*$/i;
const YES = /^\s*(yes|да|main\.yes)\s*$/i;

const SERVER_ID = 4242;
const NODE_ID = 4243;
const GAME_MOD_ID = 4244;
// The create form picks the game whose code or name matches the server name.
const GAME = { code: 'e2eidem', name: 'E2E Idempotency Game' };
const ENGLISH = JSON.parse(readFileSync(path.resolve(__dirname, '../../../../../internal/i18n/en.json'), 'utf8'));

const SERVER = {
  id: SERVER_ID,
  uid: '42424242-4242-4242-4242-424242424242',
  uuid: '42424242-4242-4242-4242-424242424242',
  uuid_short: '42424242',
  enabled: true,
  installed: 1,
  blocked: false,
  name: 'E2E Idempotency Server',
  game_id: GAME.code,
  game_mod_id: GAME_MOD_ID,
  ds_id: NODE_ID,
  server_ip: '10.0.0.1',
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

async function mockCatalog(page: Page): Promise<void> {
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
    route.fulfill({ json: [{ id: NODE_ID, name: 'E2E Node', enabled: true, os: 'linux', ip: ['10.0.0.1'] }] }),
  );
  await page.route(`**/api/nodes/${NODE_ID}/ip_list`, (route) => route.fulfill({ json: ['10.0.0.1'] }));
  await page.route(`**/api/nodes/${NODE_ID}/busy_ports`, (route) => route.fulfill({ json: {} }));
}

// mockServerPage serves the server page with the start button. The catch-all is
// registered first: routes registered later win, so the specific mocks override
// it and nothing reaches the daemon-less backend.
async function mockServerPage(page: Page): Promise<void> {
  await page.route(`**/api/servers/${SERVER_ID}/**`, (route) => route.fulfill({ json: {} }));
  await page.route(`**/api/servers/${SERVER_ID}`, (route) => route.fulfill({ json: SERVER }));
  await page.route(`**/api/servers/${SERVER_ID}/abilities`, (route) =>
    route.fulfill({ json: { 'game-server-common': true, 'game-server-start': true } }),
  );
}

// answerInTurn records the key of every request the handler sees and answers
// them with the given sequence of outcomes: 'abort' leaves the request without
// an answer, as a dropped connection would.
function answerInTurn(
  keys: string[],
  answers: Array<'abort' | { status: number; json: unknown }>,
): (route: Route) => Promise<void> {
  return async (route) => {
    keys.push(route.request().headers()['idempotency-key'] ?? '');

    const answer = answers[Math.min(keys.length, answers.length) - 1];
    if (answer === 'abort') {
      await route.abort('failed');

      return;
    }

    await route.fulfill(answer);
  };
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('auth_token', 'idempotency-ui-test'));
  await page.routeWebSocket('**/api/**', () => {});
  await page.route('**/api/**', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/profile', (route) =>
    route.fulfill({ json: { name: 'Test administrator', roles: ['admin'] } }),
  );
  await page.route('**/lang', (route) => route.fulfill({ json: [{ code: 'en', name: 'English' }] }));
  await page.route('**/lang/*.json', (route) => route.fulfill({ json: ENGLISH }));
  await page.route('**/plugins.js', (route) => route.fulfill({ body: '', contentType: 'application/javascript' }));
  await page.route('**/plugins.css', (route) => route.fulfill({ body: '', contentType: 'text/css' }));
});

test('create form retries an unanswered create with the same Idempotency-Key', async ({ page }) => {
  test.setTimeout(60_000);

  await mockCatalog(page);

  const keys: string[] = [];
  const handler = answerInTurn(keys, [
    'abort',
    { status: 422, json: { status: 'error', error: 'server port is busy', message: 'server port is busy' } },
    { status: 201, json: { message: 'success', result: { serverId: SERVER_ID, taskId: 0 } } },
  ]);
  await page.route('**/api/servers', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.fallback();

      return;
    }

    await handler(route);
  });

  await page.goto('/admin/servers/create');
  await page.getByTestId('server-create-name').locator('input').fill(`${GAME.name} server`);

  const submit = page.getByTestId('server-create-submit');
  for (let attempt = 1; attempt <= 3; attempt++) {
    await submit.click();
    await expect.poll(() => keys.length).toBe(attempt);
    await dismissTopDialog(page);
  }

  expect(keys[0]).toMatch(UUID_V4);
  expect(keys[1], 'a retry after no answer reuses the key').toBe(keys[0]);
  expect(keys[2], 'a new submission after an answer gets a new key').toMatch(UUID_V4);
  expect(keys[2]).not.toBe(keys[0]);
});

test('edit form retries an unanswered save with the same Idempotency-Key', async ({ page }) => {
  test.setTimeout(60_000);

  await mockCatalog(page);

  const keys: string[] = [];
  const handler = answerInTurn(keys, ['abort', { status: 200, json: { status: 'ok' } }]);
  await page.route(`**/api/servers/${SERVER_ID}`, async (route) => {
    if (route.request().method() === 'PUT') {
      await handler(route);

      return;
    }

    await route.fulfill({ json: SERVER });
  });

  await page.goto(`/admin/servers/${SERVER_ID}/edit`);

  const submit = page.getByTestId('server-edit-submit');
  for (let attempt = 1; attempt <= 2; attempt++) {
    await submit.click();
    await expect.poll(() => keys.length).toBe(attempt);
    await dismissTopDialog(page);
  }

  expect(keys[0]).toMatch(UUID_V4);
  expect(keys[1], 'a retry after no answer reuses the key').toBe(keys[0]);
});

test('command retries keep their key after network, conflict and server errors without reloading', async ({ page }) => {
  test.setTimeout(60_000);

  await mockServerPage(page);

  const keys: string[] = [];
  await page.route(
    `**/api/servers/${SERVER_ID}/start`,
    answerInTurn(keys, [
      'abort',
      ...[409, 503, 500, 502, 504].map((status) => ({ status, json: { message: 'Retry later' } })),
      { status: 200, json: { gdaemonTaskId: 7 } },
    ]),
  );

  let navigations = 0;
  page.on('request', (request) => {
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) navigations++;
  });
  await page.goto(`/servers/${SERVER_ID}`);

  const start = page.locator('#serverControl').getByRole('button', { name: START });

  await start.click();
  await page.getByRole('dialog').last().getByRole('button', { name: YES }).click();
  await expect.poll(() => keys.length).toBe(1);
  for (let attempt = 2; attempt <= 7; attempt++) {
    const errorDialog = page.locator('.n-dialog').last();
    await errorDialog.getByRole('button', { name: /close|закрыть|main\.close/i }).click();
    await expect(errorDialog).toBeHidden();
    await expect(page.getByTestId('idempotency-pending')).toBeVisible();
    await page.getByTestId('server-command-retry').click();
    await expect.poll(() => keys.length).toBe(attempt);
  }

  expect(keys[0]).toMatch(UUID_V4);
  expect(new Set(keys).size, 'all attempts refer to the same command').toBe(1);
  expect(navigations, 'closing transient errors never reloads the page').toBe(1);
});

test('a plain click after an unknown command outcome reopens the notice instead of resending the key', async ({ page }) => {
  await mockServerPage(page);

  const keys: string[] = [];
  await page.route(
    `**/api/servers/${SERVER_ID}/start`,
    answerInTurn(keys, ['abort', { status: 200, json: { gdaemonTaskId: 7 } }]),
  );

  await page.goto(`/servers/${SERVER_ID}`);

  const start = page.locator('#serverControl').getByRole('button', { name: START });
  const pending = page.getByTestId('idempotency-pending');

  await start.click();
  await page.getByRole('dialog').last().getByRole('button', { name: YES }).click();
  await expect.poll(() => keys.length).toBe(1);
  const errorDialog = page.locator('.n-dialog').last();
  await errorDialog.getByRole('button', { name: /close|закрыть|main\.close/i }).click();
  await expect(errorDialog).toBeHidden();
  await expect(pending).toBeVisible();

  // Dismissed without choosing: the outcome is still unknown.
  await page.getByRole('dialog').filter({ has: pending }).getByRole('button', { name: /close/i }).click();
  await expect(pending).toBeHidden();

  await start.click();
  await expect(pending).toBeVisible();
  expect(keys, 'the old key is not resent as a new operation').toHaveLength(1);

  await page.getByTestId('idempotency-reset').click();
  await expect(pending).toBeHidden();

  await start.click();
  await page.getByRole('dialog').last().getByRole('button', { name: YES }).click();
  await expect.poll(() => keys.length).toBe(2);
  expect(keys[1]).toMatch(UUID_V4);
  expect(keys[1], 'a new operation gets a new key').not.toBe(keys[0]);
});

test('create locks edits and reuses exact bytes after 5xx until the outcome is explicitly checked', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockCatalog(page);
  const requests: Array<{key: string; body: string | null}> = [];
  await page.route('**/api/servers', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.fallback();
      return;
    }
    requests.push({
      key: route.request().headers()['idempotency-key'],
      body: route.request().postData(),
    });
    await route.fulfill({ status: 500, json: { message: 'Unknown outcome' } });
  });

  await page.goto('/admin/servers/create');
  const name = page.getByTestId('server-create-name').locator('input');
  await name.fill(`${GAME.name} original`);
  const submit = page.getByTestId('server-create-submit');
  await submit.click();
  await expect.poll(() => requests.length).toBe(1);
  await dismissTopDialog(page);

  await expect(page.getByTestId('idempotency-pending')).toBeVisible();
  await expect(name).toBeDisabled();
  await expect(page.locator('form')).toHaveAttribute('inert', '');
  await expect(page.getByTestId('idempotency-pending').getByRole('link')).toHaveAttribute('target', '_blank');
  await page.screenshot({ path: testInfo.outputPath('pending-request-mobile.png'), fullPage: true });

  await submit.click();
  await expect.poll(() => requests.length).toBe(2);
  await dismissTopDialog(page);
  expect(requests[1]).toEqual(requests[0]);

  await page.getByTestId('idempotency-reset').click();
  await expect(name).toBeEnabled();
  await name.fill(`${GAME.name} new operation`);
  await submit.click();
  await expect.poll(() => requests.length).toBe(3);
  expect(requests[2].key).not.toBe(requests[0].key);
  expect(requests[2].body).not.toBe(requests[0].body);
});
