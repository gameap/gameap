import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { loginViaUI } from '../fixtures/ui';
import { deleteGame } from '../fixtures/games';
import { createGameMod, createServer, getGameModId, listNodes, seedGame } from '../fixtures/api';
import { getServerSettings, markServerInstalled, putServerSettings } from '../fixtures/servers';

const STAMP = Date.now();
const CODE = `e2es${String(STAMP).slice(-10)}`; // <=16, ^[a-z0-9_-]+$
const MOD_NAME = 'Default';

const SETTINGS_TAB = /settings|настройки/i;
const SAVE = /save|сохранить|main\.save/i;

let adminToken: string | undefined;

test.afterEach(async ({ request }) => {
  if (adminToken) {
    await deleteGame(request, adminToken, CODE);
  }
});

// One variable per type, so every branch of the field renderer is exercised.
const TYPED_VARS = [
  {
    var: 'maxplayers',
    default: '20',
    info: 'Max players',
    type: 'int' as const,
    rules: { required: true, min: 1, max: 64 },
  },
  {
    var: 'mod',
    default: 'vanilla',
    info: 'Mod',
    type: 'select' as const,
    options: ['vanilla', { value: 'paper', label: 'Paper' }],
  },
  { var: 'pvp', default: 'on', info: 'PvP', type: 'bool' as const, true_value: 'on', false_value: 'off' },
  { var: 'motd', default: 'Hello', info: 'MOTD', type: 'text' as const },
  { var: 'rcon_pass', default: '', info: 'RCON password', type: 'password' as const },
  { var: 'rate', default: '0.5', info: 'Rate', type: 'float' as const },
  // A type this build does not know must still render an editable field.
  { var: 'legacy', default: 'x', info: 'Legacy', type: undefined },
];

test('server settings render a widget per variable type and reject a rule violation', async ({
  page,
  request,
}) => {
  test.setTimeout(90_000);

  adminToken = await loginViaAPI(request);

  const nodes = await listNodes(request, adminToken);
  test.skip(nodes.length === 0, 'no node enrolled: a server cannot be created');

  await seedGame(request, adminToken, {
    code: CODE,
    name: `Settings Game ${STAMP}`,
    engine: 'test',
  });
  await createGameMod(request, adminToken, {
    game_code: CODE,
    name: MOD_NAME,
    start_cmd_linux: './run -maxplayers {maxplayers}',
    vars: TYPED_VARS,
  });

  const modId = await getGameModId(request, adminToken, CODE, MOD_NAME);
  const { serverId } = await createServer(request, adminToken, {
    name: `Settings Server ${STAMP}`,
    ds_id: nodes[0].id,
    game_id: CODE,
    game_mod_id: modId,
    server_ip: '127.0.0.1',
    server_port: 27515,
    install: false,
  });

  // 1. The API types every value after the variable type.
  const settings = await getServerSettings(request, adminToken, serverId);
  const byName = Object.fromEntries(settings.map((s) => [s.name, s]));

  expect(byName.maxplayers.value).toBe(20);
  expect(byName.pvp.value).toBe(true);
  // 0.5 must survive as a float: the old code truncated every number to an int.
  expect(byName.rate.value).toBe(0.5);
  expect(byName.mod.value).toBe('vanilla');
  expect(byName.mod.options).toEqual([
    { value: 'vanilla', label: 'vanilla' },
    { value: 'paper', label: 'Paper' },
  ]);
  expect(byName.autostart.type).toBe('bool');

  // 2. A rule violation is a 422 naming the field, and nothing is written.
  const rejected = await putServerSettings(request, adminToken, serverId, [
    { name: 'motd', value: 'Changed' },
    { name: 'maxplayers', value: 100 },
  ]);
  expect(rejected.status).toBe(422);
  expect(rejected.body).toContain('maxplayers');

  const afterReject = await getServerSettings(request, adminToken, serverId);
  expect(afterReject.find((s) => s.name === 'motd')?.value).toBe('Hello');

  // 3. The UI renders one widget per type. The server page only shows its tabs
  //    once the server counts as installed.
  await markServerInstalled(request, adminToken, serverId);

  await loginViaUI(page, process.env.E2E_ADMIN_USER ?? 'admin', process.env.E2E_ADMIN_PASSWORD ?? '');
  await page.goto(`/servers/${serverId}`);
  await page.locator('.n-tabs-tab', { hasText: SETTINGS_TAB }).first().click();

  const form = page.getByTestId('server-settings-form');
  await expect(form).toBeVisible();

  await expect(form.getByTestId('server-setting-maxplayers').locator('.n-input-number')).toBeVisible();
  await expect(form.getByTestId('server-setting-rate').locator('.n-input-number')).toBeVisible();
  await expect(form.getByTestId('server-setting-mod').locator('.n-select')).toBeVisible();
  await expect(form.getByTestId('server-setting-pvp').locator('.n-switch')).toBeVisible();
  await expect(form.getByTestId('server-setting-motd').locator('textarea')).toBeVisible();
  await expect(
    form.getByTestId('server-setting-rcon_pass').locator('input[type="password"]'),
  ).toBeVisible();
  // The unknown type falls back to a text input rather than rendering nothing.
  await expect(form.getByTestId('server-setting-legacy').locator('input')).toBeVisible();

  // 4. The number widget carries the bounds from `rules`, so an out-of-range
  //    value is clamped as it is typed rather than reaching the server.
  const maxplayersInput = form.getByTestId('server-setting-maxplayers').locator('input');
  await maxplayersInput.fill('100');
  await maxplayersInput.blur();
  await expect(maxplayersInput).toHaveValue('64');

  // 5. Clearing a required field blocks the save before the request is made.
  let putCalled = false;
  page.on('request', (r) => {
    if (r.url().includes(`/api/servers/${serverId}/settings`) && r.method() === 'PUT') {
      putCalled = true;
    }
  });

  await maxplayersInput.fill('');
  await maxplayersInput.blur();
  await page.locator('button', { hasText: SAVE }).first().click();
  await page.waitForTimeout(1_000);

  expect(putCalled).toBe(false);
  await expect(page.getByText(/field is required|обязательно/i).first()).toBeVisible();
});
