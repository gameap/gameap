import { test, expect } from '@playwright/test';
import { loginViaAPI, mintSsoTicket } from '../fixtures/auth';
import { createUser, deleteUser } from '../fixtures/users';
import { expectStatus } from '../fixtures/ui';

// Plugin contributions (sidebar items, server tabs, slot content) are appended
// to the plugins store while the plugin bundle is registered. The SSO landing
// view loads plugins after redeeming the ticket, and app.js has already loaded
// them when the browser still holds a panel session — a billing panel's "open
// game panel" button pressed a second time. Either way every contribution must
// render exactly once.
//
// The ticket is minted and redeemed against the real backend; only the plugin
// bundle is route-mocked, with a marker plugin that needs no build step.

const STAMP = Date.now();

const MENU_TEXT = 'E2E SSO plugin item';
const BANNER_TEST_ID = 'e2e-sso-plugin-banner';

const PLUGIN_BUNDLE = `
const { h } = window.Vue;

export const e2eSsoPlugin = {
  id: 'e2e-sso',
  name: 'E2E SSO',
  version: '1.0.0',
  menuItems: [{ section: 'servers', text: '${MENU_TEXT}', route: '/' }],
  slots: {
    'global-banners': {
      component: {
        setup: () => () => h('div', { 'data-testid': '${BANNER_TEST_ID}' }, 'E2E SSO plugin banner'),
      },
    },
  },
};
`;

// A duplicate comes from a second plugin load that starts right after the SSO
// redirect. Nothing signals that such a load did not happen, so the counts are
// asserted once this window has passed.
const SECOND_LOAD_WINDOW_MS = 2_000;

interface SessionCase {
  name: string;
  existingSession: boolean;
}

const SESSION_CASES: SessionCase[] = [
  { name: 'in a fresh browser', existingSession: false },
  { name: 'in a browser that already holds a session', existingSession: true },
];

let adminToken: string | undefined;
let userId: number | undefined;

test.afterEach(async ({ request }) => {
  if (adminToken && userId !== undefined) {
    await deleteUser(request, adminToken, userId);
    userId = undefined;
  }
});

for (const { name, existingSession } of SESSION_CASES) {
  test(`SSO sign-in renders plugin contributions once ${name}`, async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000);

    const login = `e2e_sso_${existingSession ? 'session' : 'fresh'}_${STAMP}`;
    const password = `SsoPass_${STAMP}`;

    adminToken = await loginViaAPI(request);
    const user = await createUser(request, adminToken, {
      login,
      email: `${login}@example.com`,
      password,
      name: `SSO ${STAMP}`,
    });
    userId = user.id;

    if (existingSession) {
      const sessionToken = await loginViaAPI(request, { login, password });
      await page.addInitScript(
        (token) => localStorage.setItem('auth_token', token),
        sessionToken,
      );
    }

    let bundleRequests = 0;
    await page.route(
      (url) => url.pathname === '/plugins.js',
      async (route) => {
        bundleRequests++;
        await route.fulfill({
          contentType: 'application/javascript',
          body: PLUGIN_BUNDLE,
        });
      },
    );

    const ticket = await mintSsoTicket(request, adminToken, user.id);

    const exchange = page.waitForResponse(
      (r) =>
        r.url().includes('/api/auth/sso/exchange') &&
        r.request().method() === 'POST',
    );
    await page.goto(`/sso#t=${ticket}`);
    await expectStatus(await exchange, 200, 'SSO exchange');

    await page.waitForURL((url) => url.pathname === '/', { timeout: 15_000 });

    const banner = page.getByTestId(BANNER_TEST_ID);
    const menuItem = page
      .locator('.sidebar-menu')
      .getByText(MENU_TEXT, { exact: true });
    await expect(banner.first()).toBeVisible({ timeout: 15_000 });
    await expect(menuItem.first()).toBeVisible();

    await page.waitForTimeout(SECOND_LOAD_WINDOW_MS);

    await expect(banner).toHaveCount(1);
    await expect(menuItem).toHaveCount(1);
    expect(bundleRequests).toBe(1);
  });
}
