import {
  expect,
  type APIResponse,
  type Page,
  type Response,
} from '@playwright/test';

// naive-ui $dialog / notification "Close" action button — i18n-tolerant,
// including the raw i18n key rendered when window.i18n is unpopulated.
const CLOSE = /close|закрыть|main\.close/i;

// The SPA renders raw i18n keys when window.i18n is unpopulated (i18n.js
// trans() returns the key), so every text matcher covers English, Russian and
// the raw key.
export const SIGN_IN = /sign.?in|login|вход|войти|auth\.sign_in/i;
export const PROFILE_ITEM = /profile|профиль|navbar\.profile/i;
export const SIGN_OUT_ITEM = /sign[\s_-]?out|выйти|navbar\.sign_out/i;

// Server 500s are masked to a generic body by the API responder; surface the
// response text in the failure message so a backend error is self-describing
// without a re-run. The body can be evicted once the page navigates, so it is
// read best-effort and only on a status mismatch.
// Accepts both a request-context APIResponse and a page-observed network
// Response — specs assert on either, and only status()/text() are used.
export async function expectStatus(
  response: APIResponse | Response,
  expected: number,
  label: string,
): Promise<void> {
  const actual = response.status();
  if (actual === expected) {
    return;
  }

  let body = '<unavailable>';
  try {
    body = await response.text();
  } catch {
    // Response body evicted by a page navigation (e.g. a successful login
    // triggers location.reload()).
  }

  expect(actual, `${label} (body: ${body})`).toBe(expected);
}

// naive-ui notification() opens a blocking $dialog (role="dialog") with a
// single "Close" action button. Scope to the top-most dialog so a stray
// card-header close "X" (aria-label="close") elsewhere on the page does not
// cause a strict-mode match.
export async function dismissTopDialog(page: Page): Promise<void> {
  const close = page
    .getByRole('dialog')
    .last()
    .getByRole('button', { name: CLOSE });
  await expect(close).toBeVisible({ timeout: 10_000 });
  await close.click();
  await expect(close).toBeHidden({ timeout: 10_000 });
}

// Sign in through the real /login form (not the API fast path), asserting the
// POST status and that the SPA left /login with a token in localStorage.
// Expected non-200 statuses (e.g. 401) leave the browser on /login.
export async function loginViaUI(
  page: Page,
  login: string,
  password: string,
  expectedStatus = 200,
): Promise<void> {
  await page.goto('/login');
  await page.locator('#email').fill(login);
  await page.locator('#password').fill(password);

  const response = page.waitForResponse(
    (r) =>
      r.url().includes('/api/auth/login') && r.request().method() === 'POST',
  );
  await page.getByRole('button', { name: SIGN_IN }).click();
  await expectStatus(
    await response,
    expectedStatus,
    `login as ${login} should be ${expectedStatus}`,
  );

  if (expectedStatus !== 200) {
    await expect(page).toHaveURL(/\/login$/);

    return;
  }

  await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
  await expect
    .poll(async () => page.evaluate(() => localStorage.getItem('auth_token')))
    .toBeTruthy();
}
