import { test, expect } from '@playwright/test';
import { loginViaAPI, authHeader } from '../fixtures/auth';
import { createUser, deleteUser } from '../fixtures/users';
import { loginViaUI } from '../fixtures/ui';

// A plain `user` must not reach admin data. The sidebar hides the admin
// section (v-if="isAdmin" in MainSidebar.vue), and typing an admin route
// directly renders the view shell but no records: AdminUsersView fetches
// through userListStore, the API answers 403, and the view surfaces a
// "Forbidden" notification with an empty table.
//
// Note there is no client-side route guard — routes.js has no per-route
// ability meta, and the store-level 403 → /403 redirect in App.vue is wired
// only to nodeStore/daemonTaskStore/gameStore/serverStore/userStore, not to
// userListStore. Enforcement is server-side, which is what these tests pin.

const STAMP = Date.now();
const LOGIN = `e2e_rb_${STAMP}`;
const EMAIL = `e2e_rb_${STAMP}@example.com`;
const PASSWORD = `RbacPass_${STAMP}`;
const NAME = `RBAC User ${STAMP}`;

// Admin-only routes reachable from the sidebar. Kept in sync with routes.js.
const ADMIN_ROUTES = [
  '/admin/users',
  '/admin/nodes',
  '/admin/games',
  '/admin/servers',
  '/admin/client_certificates',
];

// i18n-tolerant matcher for the denial notification.
const FORBIDDEN = /forbidden|доступ запрещ|permissions required/i;

// Any admin account e-mail rendered in the table would mean records leaked.
const ADMIN_EMAIL_PATTERN = /admin@/i;

const BASE_URL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';

let adminToken: string | undefined;
let userId: number | undefined;

test.beforeEach(async ({ request }) => {
  adminToken = await loginViaAPI(request);
  const user = await createUser(request, adminToken, {
    login: LOGIN,
    email: EMAIL,
    password: PASSWORD,
    name: NAME,
    roles: ['user'],
  });
  userId = user.id;
});

test.afterEach(async ({ request }) => {
  if (adminToken && userId !== undefined) {
    await deleteUser(request, adminToken, userId);
    userId = undefined;
  }
});

test('a non-admin user gets no admin navigation and no admin data', async ({
  page,
}) => {
  test.setTimeout(60_000);

  // 1. Sign in through the UI as the throwaway non-admin user.
  await loginViaUI(page, LOGIN, PASSWORD);

  // 2. The admin block of the sidebar is rendered under v-if="isAdmin", so
  //    none of its links may exist for this user.
  for (const route of ADMIN_ROUTES) {
    await expect(
      page.locator(`.sidebar-menu a[href="${route}"]`),
      `sidebar must not offer ${route} to a non-admin`,
    ).toHaveCount(0);
  }

  // 3. Typing the route directly still yields no records: the denial is
  //    surfaced as a Forbidden notification and the table stays empty.
  await page.goto('/admin/users');

  await expect(
    page.getByRole('dialog').filter({ hasText: FORBIDDEN }),
  ).toBeVisible({ timeout: 15_000 });

  const table = page.getByTestId('users-table');
  await expect(table).toBeVisible();
  await expect(
    table.getByRole('row'),
    'only the header row may render — no user records may leak',
  ).toHaveCount(1);
  await expect(page.getByText(ADMIN_EMAIL_PATTERN)).toHaveCount(0);

  // 4. The session is still usable — the denial is per-request, not a logout.
  expect(
    await page.evaluate(() => localStorage.getItem('auth_token')),
  ).toBeTruthy();
});

test('the admin API refuses a non-admin token and accepts an admin one', async ({
  request,
}) => {
  const userToken = await loginViaAPI(request, {
    login: LOGIN,
    password: PASSWORD,
  });

  const denied = await request.get(`${BASE_URL}/api/users`, {
    headers: authHeader(userToken),
  });
  expect(denied.status(), 'GET /api/users must be denied for a plain user').toBe(403);

  const allowed = await request.get(`${BASE_URL}/api/users`, {
    headers: authHeader(adminToken as string),
  });
  expect(allowed.status(), 'GET /api/users must be allowed for an admin').toBe(200);
});

test('an admin does see the admin navigation', async ({ page }) => {
  test.setTimeout(60_000);

  // The negative assertions above are only meaningful if the same selectors
  // do match for an admin — otherwise a renamed class would silently pass.
  await loginViaUI(
    page,
    process.env.E2E_ADMIN_USER ?? 'admin',
    process.env.E2E_ADMIN_PASSWORD ?? '',
  );

  await expect(
    page.locator('.sidebar-menu a[href="/admin/users"]').first(),
  ).toBeAttached({ timeout: 15_000 });
});
