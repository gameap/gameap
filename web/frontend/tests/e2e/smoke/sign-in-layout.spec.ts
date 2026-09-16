import { test, expect, type Page } from '@playwright/test';
import { loginViaAPI, mintSsoTicket } from '../fixtures/auth';
import { createUser, deleteUser } from '../fixtures/users';
import { loginViaUI } from '../fixtures/ui';

// The sign-in pages receive a session before they are done: both the SSO view
// and the login view still fetch the profile and redirect afterwards. When the
// panel layout replaced the guest layout at that moment the view was mounted
// again — the SSO view had no ticket left and said the link was no longer
// valid until the redirect, the login form rendered once more inside the
// panel. Those states last only until the redirect, so they are recorded on
// every DOM change instead of being asserted at the end.

const SIGN_IN_PATHS = ['/login', '/sso'];

// English | Russian | raw i18n key, same convention as the other smoke specs.
const INVALID_LINK =
  /no longer valid|больше не действительна|auth\.sso_failed/i;

interface SignInStates {
  invalidLinkShown: boolean;
  panelOnSignInPage: boolean;
}

// Installed before any page script runs, so nothing the SPA renders on mount
// is missed. Arguments reach the page as JSON, hence the pattern source.
async function recordSignInStates(page: Page): Promise<void> {
  await page.addInitScript(
    ({ signInPaths, invalidLinkSource }) => {
      const states = { invalidLinkShown: false, panelOnSignInPage: false };
      (window as unknown as { signInStates: SignInStates }).signInStates =
        states;
      const invalidLink = new RegExp(invalidLinkSource, 'i');

      new MutationObserver(() => {
        const appText = document.getElementById('app')?.textContent ?? '';
        if (invalidLink.test(appText)) {
          states.invalidLinkShown = true;
        }

        if (
          signInPaths.includes(location.pathname) &&
          document.querySelector('.sidebar-menu')
        ) {
          states.panelOnSignInPage = true;
        }
      }).observe(document, {
        childList: true,
        subtree: true,
        characterData: true,
      });
    },
    { signInPaths: SIGN_IN_PATHS, invalidLinkSource: INVALID_LINK.source },
  );
}

async function recordedSignInStates(page: Page): Promise<SignInStates> {
  return page.evaluate(
    () => (window as unknown as { signInStates: SignInStates }).signInStates,
  );
}

let adminToken: string | undefined;
let userId: number | undefined;

test.afterEach(async ({ request }) => {
  if (adminToken && userId !== undefined) {
    await deleteUser(request, adminToken, userId);
    userId = undefined;
  }
});

test('SSO sign-in keeps the guest layout and never reports the link as invalid', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  const stamp = Date.now();
  const login = `e2e_sso_layout_${stamp}`;

  adminToken = await loginViaAPI(request);
  const user = await createUser(request, adminToken, {
    login,
    email: `${login}@example.com`,
    password: `SsoLayout_${stamp}`,
    name: `SSO layout ${stamp}`,
  });
  userId = user.id;

  const ticket = await mintSsoTicket(request, adminToken, user.id);

  await recordSignInStates(page);
  await page.goto(`/sso#t=${ticket}`);

  await page.waitForURL((url) => url.pathname === '/', { timeout: 15_000 });
  await expect(page.locator('.sidebar-menu')).toBeVisible();

  expect(await recordedSignInStates(page)).toEqual({
    invalidLinkShown: false,
    panelOnSignInPage: false,
  });
});

// Keeps the recorder honest: the same matcher has to notice the message when
// the exchange really goes wrong. A 5xx is used because a 401 is turned into a
// redirect to /login by the axios interceptor before the view can render.
test('an SSO exchange error is reported as an invalid link in the guest layout', async ({
  page,
}) => {
  await page.route('**/api/auth/sso/exchange', (route) =>
    route.fulfill({ status: 500, json: { message: 'e2e exchange failure' } }),
  );

  await recordSignInStates(page);
  await page.goto('/sso#t=e2e-ticket');

  await expect(page.getByText(INVALID_LINK)).toBeVisible({ timeout: 15_000 });
  await expect(page.locator('.sidebar-menu')).toHaveCount(0);
  expect((await recordedSignInStates(page)).invalidLinkShown).toBe(true);
});

test('password sign-in keeps the guest layout until the dashboard opens', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  const stamp = Date.now();
  const login = `e2e_login_layout_${stamp}`;
  const password = `LoginLayout_${stamp}`;

  adminToken = await loginViaAPI(request);
  const user = await createUser(request, adminToken, {
    login,
    email: `${login}@example.com`,
    password,
    name: `Login layout ${stamp}`,
  });
  userId = user.id;

  await recordSignInStates(page);
  await loginViaUI(page, login, password);

  await expect(page.locator('.sidebar-menu')).toBeVisible();
  expect((await recordedSignInStates(page)).panelOnSignInPage).toBe(false);
});
