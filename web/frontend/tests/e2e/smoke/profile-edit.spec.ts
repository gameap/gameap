import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { createUser, deleteUser, getProfile } from '../fixtures/users';
import { expectStatus, dismissTopDialog } from '../fixtures/ui';

const STAMP = Date.now();
const LOGIN = `e2e_pe_${STAMP}`;
const EMAIL = `e2e_pe_${STAMP}@example.com`;
const OLD_PASSWORD = `OldPass_${STAMP}`;
const NEW_PASSWORD = `NewPass_${STAMP}`;
const OLD_NAME = `PE Old ${STAMP}`;
const NEW_NAME = `PE New ${STAMP}`;

// i18n-tolerant matchers: the SPA renders raw i18n keys when window.i18n is
// unpopulated (i18n.js trans() returns the key), so every text matcher covers
// English, Russian and the raw key — same convention as login.spec.ts.
const SIGN_IN = /sign.?in|login|вход|войти|auth\.sign_in/i;
const PROFILE_ITEM = /profile|профиль|navbar\.profile/i;
const SIGN_OUT_ITEM = /sign[\s_-]?out|выйти|navbar\.sign_out/i;

let adminToken: string | undefined;
let userId: number | undefined;

test.afterEach(async ({ request }) => {
  if (adminToken && userId !== undefined) {
    await deleteUser(request, adminToken, userId);
    userId = undefined;
  }
});

test('user edits profile (name + password) and can re-login only with the new password', async ({
  page,
  request,
}) => {
  // 1. Provision a throwaway user via the admin API.
  adminToken = await loginViaAPI(request);
  const user = await createUser(request, adminToken, {
    login: LOGIN,
    email: EMAIL,
    password: OLD_PASSWORD,
    name: OLD_NAME,
  });
  userId = user.id;

  // 2. Log in through the /login UI form with the original password.
  await page.goto('/login');
  await page.locator('#email').fill(LOGIN);
  await page.locator('#password').fill(OLD_PASSWORD);
  const firstLogin = page.waitForResponse(
    (r) =>
      r.url().includes('/api/auth/login') && r.request().method() === 'POST',
  );
  await page.getByRole('button', { name: SIGN_IN }).click();
  await expectStatus(await firstLogin, 200, 'initial login should be 200');
  await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
  await expect
    .poll(async () => page.evaluate(() => localStorage.getItem('auth_token')))
    .toBeTruthy();

  // 3. Open the top-right user menu and go to Profile.
  const nav = page.getByRole('navigation').first();
  await nav.getByRole('button', { name: OLD_NAME }).click();
  await nav.locator('a', { hasText: PROFILE_ITEM }).click();
  await expect(page).toHaveURL(/\/profile$/, { timeout: 10_000 });

  // 4. Open the Edit Profile modal.
  await page.getByTestId('profile-edit-button').click();
  const nameInput = page.getByTestId('profile-form-name').locator('input');
  await expect(nameInput).toBeVisible({ timeout: 10_000 });

  // 5. Change name + password and save.
  await nameInput.fill(NEW_NAME);
  await page
    .getByTestId('profile-form-current-password')
    .locator('input')
    .fill(OLD_PASSWORD);
  await page
    .getByTestId('profile-form-new-password')
    .locator('input')
    .fill(NEW_PASSWORD);
  await page
    .getByTestId('profile-form-password-confirmation')
    .locator('input')
    .fill(NEW_PASSWORD);
  const putProfile = page.waitForResponse(
    (r) => r.url().includes('/api/profile') && r.request().method() === 'PUT',
  );
  await page.getByTestId('profile-form-save').click();
  await expectStatus(await putProfile, 200, 'PUT /api/profile should be 200');

  // 6. The success notification is a blocking naive-ui $dialog — dismiss it,
  //    otherwise it overlays the navbar for the sign-out step.
  await dismissTopDialog(page);

  // 7. The /profile page reflects the new name (reactive refresh races a
  //    non-awaited fetchProfile() — poll, then reload as a fallback), and the
  //    change is persisted server-side (independent API cross-check, which
  //    also proves the new password already works at the API layer).
  const nameCell = page.getByTestId('profile-name-value');
  try {
    await expect
      .poll(async () => (await nameCell.textContent())?.trim(), {
        timeout: 15_000,
      })
      .toBe(NEW_NAME);
  } catch {
    await page.reload();
    await expect(page).toHaveURL(/\/profile$/);
    await expect(nameCell).toHaveText(NEW_NAME, { timeout: 15_000 });
  }
  const userToken = await loginViaAPI(request, {
    login: LOGIN,
    password: NEW_PASSWORD,
  });
  expect((await getProfile(request, userToken)).name).toBe(NEW_NAME);

  // 8. Sign out via the user menu (trigger label is now the new name).
  await nav.getByRole('button', { name: NEW_NAME }).click();
  await nav.locator('a', { hasText: SIGN_OUT_ITEM }).click();
  await expect(page).toHaveURL(/\/login$/, { timeout: 15_000 });
  await expect
    .poll(async () => page.evaluate(() => localStorage.getItem('auth_token')))
    .toBeFalsy();

  // 9. The OLD password must be rejected: HTTP 401, stay on /login, no token.
  await page.locator('#email').fill(LOGIN);
  await page.locator('#password').fill(OLD_PASSWORD);
  const badLogin = page.waitForResponse(
    (r) =>
      r.url().includes('/api/auth/login') && r.request().method() === 'POST',
  );
  await page.getByRole('button', { name: SIGN_IN }).click();
  await expectStatus(await badLogin, 401, 'login with the old password must be 401');
  await expect(page).toHaveURL(/\/login$/);
  await page.waitForTimeout(500);
  await expect(page).toHaveURL(/\/login$/);
  expect(
    await page.evaluate(() => localStorage.getItem('auth_token')),
  ).toBeFalsy();

  // Dismiss the blocking error $dialog before the next attempt.
  await dismissTopDialog(page);

  // 10. The NEW password must be accepted.
  await page.locator('#email').fill(LOGIN);
  await page.locator('#password').fill(NEW_PASSWORD);
  const goodLogin = page.waitForResponse(
    (r) =>
      r.url().includes('/api/auth/login') && r.request().method() === 'POST',
  );
  await page.getByRole('button', { name: SIGN_IN }).click();
  await expectStatus(await goodLogin, 200, 'login with the new password must be 200');
  await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
  await expect
    .poll(async () => page.evaluate(() => localStorage.getItem('auth_token')))
    .toBeTruthy();
});
