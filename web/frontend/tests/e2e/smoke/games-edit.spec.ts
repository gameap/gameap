import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { createUser, deleteUser } from '../fixtures/users';
import { expectStatus, dismissTopDialog } from '../fixtures/ui';
import { getGame, deleteGame } from '../fixtures/games';

const STAMP = Date.now();
const CODE = `e2eg${String(STAMP).slice(-10)}`; // <=16, ^[a-z0-9_-]+$
const NAME0 = `Game ${STAMP}`;
const NAME1 = `Game ${STAMP} upd`;

// i18n-tolerant matchers (English | Russian | raw i18n key).
const SIGN_IN = /sign.?in|login|вход|войти|auth\.sign_in/i;
const METADATA_TAB = /metadata|метаданные|games\.metadata/i;
const ADD_ROW = /add|добавить|main\.add/i;

let adminToken: string | undefined;
let userId: number | undefined;

test.afterEach(async ({ request }) => {
  if (adminToken) {
    await deleteGame(request, adminToken, CODE);
    if (userId !== undefined) {
      await deleteUser(request, adminToken, userId);
      userId = undefined;
    }
  }
});

test('admin creates a game, sees defaults, edits fields + metadata, and it persists', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  const login = `e2e_gm_${STAMP}`;
  const password = `GmPass_${STAMP}`;

  // 1. Provision a throwaway admin user (the whole flow is admin-only).
  adminToken = await loginViaAPI(request);
  const user = await createUser(request, adminToken, {
    login,
    email: `e2e_gm_${STAMP}@example.com`,
    password,
    name: `GM admin ${STAMP}`,
    roles: ['admin'],
  });
  userId = user.id;

  // 2. UI login as that admin.
  await page.goto('/login');
  await page.locator('#email').fill(login);
  await page.locator('#password').fill(password);
  const loginResp = page.waitForResponse(
    (r) =>
      r.url().includes('/api/auth/login') && r.request().method() === 'POST',
  );
  await page.getByRole('button', { name: SIGN_IN }).click();
  await expectStatus(await loginResp, 200, 'admin login should be 200');
  await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
  await expect
    .poll(async () => page.evaluate(() => localStorage.getItem('auth_token')))
    .toBeTruthy();

  // 3. Left sidebar → Games (href is stable across minimized/full sidebar).
  await page.locator('.sidebar-menu a[href="/admin/games"]').first().click();
  await expect(page).toHaveURL(/\/admin\/games$/, { timeout: 10_000 });

  // 4. Add Game → fill code + name, leave engine/version/repos empty → Create.
  await page.getByTestId('games-add-button').click();
  await page.getByTestId('create-game-code').locator('input').fill(CODE);
  await page.getByTestId('create-game-name').locator('input').fill(NAME0);
  const createResp = page.waitForResponse(
    (r) => r.url().includes('/api/games') && r.request().method() === 'POST',
  );
  await page.getByTestId('create-game-submit').click();
  await expectStatus(await createResp, 200, 'create game should be 200');

  // 5. Success dialog, then redirected to the edit page for our code.
  await dismissTopDialog(page);
  await expect(page).toHaveURL(new RegExp(`/admin/games/${CODE}$`), {
    timeout: 10_000,
  });

  // 6. Create-time defaults shown on the edit page.
  await expect(page.getByTestId('game-engine').locator('input')).toHaveValue(
    'unknown',
    { timeout: 15_000 },
  );
  await expect(
    page.getByTestId('game-engine-version').locator('input'),
  ).toHaveValue('');
  await expect(
    page.getByTestId('game-steam-app-id-linux').locator('input'),
  ).toHaveValue('0');
  await expect(
    page.getByTestId('game-steam-app-id-windows').locator('input'),
  ).toHaveValue('0');
  for (const id of [
    'game-local-repo-linux',
    'game-local-repo-windows',
    'game-remote-repo-linux',
    'game-remote-repo-windows',
  ]) {
    await expect(page.getByTestId(id).locator('input')).toHaveValue('');
  }

  // 7. Edit the Main-tab fields.
  await page.getByTestId('game-name').locator('input').fill(NAME1);
  await page.getByTestId('game-engine').locator('input').fill('source');
  await page.getByTestId('game-engine-version').locator('input').fill('17');
  await page
    .getByTestId('game-steam-app-id-linux')
    .locator('input')
    .fill('440');
  await page
    .getByTestId('game-steam-app-id-windows')
    .locator('input')
    .fill('441');
  await page
    .getByTestId('game-steam-app-set-config')
    .locator('input')
    .fill('90 mod cstrike');

  // 8. Metadata tab → add two rows (plain non-JSON strings round-trip as
  //    strings; AdminGamesEdit JSON.parse-attempts each value).
  await page
    .locator('.n-tabs-tab', { hasText: METADATA_TAB })
    .first()
    .click();
  const meta = page.getByTestId('game-metadata');
  await meta.getByRole('button', { name: ADD_ROW }).click();
  await meta.getByRole('button', { name: ADD_ROW }).click();
  const rows = meta.locator('tbody tr');
  await rows.nth(0).locator('input').nth(0).fill('e2e_meta_1');
  await rows.nth(0).locator('input').nth(1).fill('meta value one');
  await rows.nth(1).locator('input').nth(0).fill('e2e_meta_2');
  await rows.nth(1).locator('input').nth(1).fill('meta value two');

  // 9. Save.
  const updateResp = page.waitForResponse(
    (r) =>
      r.url().includes(`/api/games/${CODE}`) &&
      r.request().method() === 'PUT',
  );
  await page.getByTestId('game-save').click();
  await expectStatus(await updateResp, 200, 'update game should be 200');
  await dismissTopDialog(page);

  // 10. Verify the persisted data via the API (admin-only endpoint).
  const g = await getGame(request, adminToken, CODE);
  expect(g.name).toBe(NAME1);
  expect(g.engine).toBe('source');
  expect(g.engine_version).toBe('17');
  expect(g.steam_app_id_linux).toBe(440);
  expect(g.steam_app_id_windows).toBe(441);
  expect(g.steam_app_set_config).toBe('90 mod cstrike');
  expect(g.metadata?.e2e_meta_1).toBe('meta value one');
  expect(g.metadata?.e2e_meta_2).toBe('meta value two');
});
