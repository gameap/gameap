import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { loginViaUI } from '../fixtures/ui';
import { deleteGame } from '../fixtures/games';
import { createGameMod, createServer, getGameModId, listNodes, seedGame } from '../fixtures/api';
import { markServerInstalled } from '../fixtures/servers';

const STAMP = Date.now();
const CODE = `e2ei${String(STAMP).slice(-10)}`; // <=16, ^[a-z0-9_-]+$
const MOD_NAME = 'Default';

const SETTINGS_TAB = /settings|настройки/i;

let adminToken: string | undefined;

test.afterEach(async ({ request }) => {
  if (adminToken) {
    await deleteGame(request, adminToken, CODE);
  }
});

test('variable labels follow the interface language', async ({ page, request }) => {
  test.setTimeout(90_000);

  adminToken = await loginViaAPI(request);

  const nodes = await listNodes(request, adminToken);
  test.skip(nodes.length === 0, 'no node enrolled: a server cannot be created');

  await seedGame(request, adminToken, {
    code: CODE,
    name: `I18n Game ${STAMP}`,
    engine: 'test',
  });
  await createGameMod(request, adminToken, {
    game_code: CODE,
    name: MOD_NAME,
    vars: [
      {
        var: 'maxplayers',
        default: '20',
        info: 'Max players',
        i18n: { ru: { info: 'Максимум игроков' } },
      },
      // No translation: this one must stay English even in a Russian interface.
      { var: 'hostname', default: 'Server', info: 'Hostname' },
    ],
  });

  const modId = await getGameModId(request, adminToken, CODE, MOD_NAME);
  const { serverId } = await createServer(request, adminToken, {
    name: `I18n Server ${STAMP}`,
    ds_id: nodes[0].id,
    game_id: CODE,
    game_mod_id: modId,
    server_ip: '127.0.0.1',
    server_port: 27615,
    install: false,
  });
  await markServerInstalled(request, adminToken, serverId);

  await loginViaUI(page, process.env.E2E_ADMIN_USER ?? 'admin', process.env.E2E_ADMIN_PASSWORD ?? '');

  // index.html reads the language out of localStorage before the app mounts.
  await page.addInitScript(() => {
    localStorage.setItem('gameap_ui_settings', JSON.stringify({ language: 'ru' }));
  });

  await page.goto(`/servers/${serverId}`);
  await page.locator('.n-tabs-tab', { hasText: SETTINGS_TAB }).first().click();

  const form = page.getByTestId('server-settings-form');
  await expect(form).toBeVisible();

  await expect(form.getByTestId('server-setting-maxplayers')).toContainText('Максимум игроков');
  // Untranslated variables fall back to the base English label.
  await expect(form.getByTestId('server-setting-hostname')).toContainText('Hostname');
});
