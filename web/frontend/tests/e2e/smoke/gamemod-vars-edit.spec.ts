import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { createUser, deleteUser } from '../fixtures/users';
import { dismissTopDialog, loginViaUI } from '../fixtures/ui';
import { deleteGame } from '../fixtures/games';
import { createGameMod, getGameMod, getGameModId, seedGame } from '../fixtures/api';

const STAMP = Date.now();
const CODE = `e2ev${String(STAMP).slice(-10)}`; // <=16, ^[a-z0-9_-]+$
const MOD_NAME = 'Default';

// i18n-tolerant matchers (English | Russian | raw i18n key).
const VARS_TAB = /^\s*(vars|переменные|games\.vars)\s*$/i;
const SELECT_TYPE = /^\s*(select|список|games\.var_type_select)\s*$/i;
const ADD_OPTION = /add option|добавить вариант|games\.var_option_add/i;
const ADD_TRANSLATION = /add translation|добавить перевод|games\.translations_add/i;
const SAVE = /save|сохранить|main\.save/i;

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

test('admin turns a variable into a select with options, a rule and a translation', async ({
  page,
  request,
}) => {
  test.setTimeout(90_000);

  const login = `e2e_vars_${STAMP}`;
  const password = `VarsPass_${STAMP}`;

  // 1. Throwaway admin plus a game whose mod carries one plain variable.
  adminToken = await loginViaAPI(request);
  const user = await createUser(request, adminToken, {
    login,
    email: `e2e_vars_${STAMP}@example.com`,
    password,
    name: `Vars admin ${STAMP}`,
    roles: ['admin'],
  });
  userId = user.id;

  await seedGame(request, adminToken, {
    code: CODE,
    name: `Vars Game ${STAMP}`,
    engine: 'test',
  });
  await createGameMod(request, adminToken, {
    game_code: CODE,
    name: MOD_NAME,
    vars: [{ var: 'mod', default: 'vanilla', info: 'Mod' }],
  });

  const modId = await getGameModId(request, adminToken, CODE, MOD_NAME);

  // 2. Open the mod editor as that admin.
  await loginViaUI(page, login, password);
  await page.goto(`/admin/games/${CODE}/mods/${modId}/edit`);

  await page.locator('.n-tabs-tab', { hasText: VARS_TAB }).click();

  const editor = page.getByTestId('mod-vars-editor');
  await expect(editor).toBeVisible();

  // 3. Expand the only variable and switch it to a select.
  const card = editor.getByTestId('mod-var-card-0');
  await card.locator('.n-collapse-item__header').click();

  await card.locator('.n-select').first().click();
  await page.locator('.n-base-select-option', { hasText: SELECT_TYPE }).click();

  // 4. Two options: one bare, one carrying a label.
  const options = card.getByTestId('var-options-editor');
  await options.getByRole('button', { name: ADD_OPTION }).click();
  await options.locator('input').nth(0).fill('vanilla');

  await options.getByRole('button', { name: ADD_OPTION }).click();
  await options.locator('input').nth(2).fill('paper');
  await options.locator('input').nth(3).fill('Paper');

  // 5. A Russian label for the variable itself.
  const translations = card.getByTestId('i18n-editor').last();
  await translations.getByRole('button', { name: ADD_TRANSLATION }).click();
  await translations.locator('.n-select').first().click();
  await page.locator('.n-base-select-option', { hasText: /\(ru\)/ }).first().click();
  await translations.locator('input').last().fill('Мод');

  // 6. Save and confirm the request succeeded.
  const saveResponse = page.waitForResponse(
    (r) => r.url().includes(`/api/game_mods/${modId}`) && r.request().method() === 'PUT',
  );
  await page.locator('button', { hasText: SAVE }).first().click();
  expect((await saveResponse).status()).toBe(200);

  await dismissTopDialog(page);

  // 7. Read the stored definition back. The response always expands an option to
  //    {value,label}; the plain-string shorthand is what reaches storage and the
  //    export, and shows up here as a label equal to the value.
  const stored = await getGameMod(request, adminToken, modId);
  const variable = stored.vars.find((v) => v.var === 'mod');

  expect(variable).toBeDefined();
  expect(variable?.type).toBe('select');
  expect(variable?.default).toBe('vanilla');
  expect(variable?.options).toEqual([
    { value: 'vanilla', label: 'vanilla' },
    { value: 'paper', label: 'Paper' },
  ]);
  expect(variable?.i18n).toEqual({ ru: { info: 'Мод' } });
  expect(variable?.i18n).not.toHaveProperty('en');
  // allow_custom was left off, so it must not be sent at all.
  expect(variable?.allow_custom).toBeUndefined();
  expect(variable?.rules).toBeUndefined();
});
