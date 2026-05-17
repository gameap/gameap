import { test, expect } from '@playwright/test';
import { loginViaAPI, authHeader } from '../fixtures/auth';
import { createUser, deleteUser } from '../fixtures/users';
import { expectStatus, dismissTopDialog } from '../fixtures/ui';

const API_BASE = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';
const STAMP = Date.now();

// i18n-tolerant matchers (English | Russian | raw i18n key), same convention
// as the other smoke specs.
const SIGN_IN = /sign.?in|login|вход|войти|auth\.sign_in/i;
const TOKENS_ITEM = /tokens|токены|tokens\.tokens/i;
// The one-time "copy it now, you won't see it again" warning rendered in the
// success dialog after a token is created.
const TOKEN_ONCE =
  /make sure to copy|won't be able to see it again|обязательно скопируйте|больше его не сможете увидеть|tokens\.token_created_notification/i;

// admin:server:create + admin:gdaemon-task:read are only offered to admins;
// server:list gates GET /api/servers; server:start is a valid non-admin
// ability used to mint a token that deliberately lacks server:list.
const ADMIN_ABILITIES = [
  'admin:server:create',
  'admin:gdaemon-task:read',
] as const;

interface RoleCase {
  role: 'admin' | 'user';
  adminAbilitiesVisible: boolean;
}

const ROLE_CASES: RoleCase[] = [
  { role: 'admin', adminAbilitiesVisible: true },
  { role: 'user', adminAbilitiesVisible: false },
];

let adminToken: string | undefined;
let userId: number | undefined;

test.afterEach(async ({ request }) => {
  if (adminToken && userId !== undefined) {
    await deleteUser(request, adminToken, userId);
    userId = undefined;
  }
});

for (const { role, adminAbilitiesVisible } of ROLE_CASES) {
  test(`PAT abilities and server:list enforcement for a ${role}-role account`, async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000);

    const login = `e2e_pt_${role}_${STAMP}`;
    const password = `PatPass_${STAMP}`;
    const name = `PT ${role} ${STAMP}`;

    // 1. Provision a throwaway user with the target role.
    adminToken = await loginViaAPI(request);
    const user = await createUser(request, adminToken, {
      login,
      email: `e2e_pt_${role}_${STAMP}@example.com`,
      password,
      name,
      roles: [role],
    });
    userId = user.id;

    // 2. Log in via the /login UI form as that user.
    await page.goto('/login');
    await page.locator('#email').fill(login);
    await page.locator('#password').fill(password);
    const loginResp = page.waitForResponse(
      (r) =>
        r.url().includes('/api/auth/login') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: SIGN_IN }).click();
    await expectStatus(await loginResp, 200, `${role} login should be 200`);
    await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
    await expect
      .poll(async () => page.evaluate(() => localStorage.getItem('auth_token')))
      .toBeTruthy();

    // 3. Navbar user menu → Tokens.
    const nav = page.getByRole('navigation').first();
    await nav.getByRole('button', { name }).click();
    await nav.locator('a', { hasText: TOKENS_ITEM }).click();
    await expect(page).toHaveURL(/\/tokens$/, { timeout: 10_000 });

    // 4. Open the generate-token modal (clicking it fetches abilities first).
    await page.getByTestId('tokens-generate-button').click();
    await expect(page.getByTestId('token-ability-server:list')).toBeVisible({
      timeout: 10_000,
    });

    // 5. Admin abilities must be present for admins, absent for regular users.
    for (const ability of ADMIN_ABILITIES) {
      const locator = page.getByTestId(`token-ability-${ability}`);
      if (adminAbilitiesVisible) {
        await expect(locator).toBeVisible();
      } else {
        await expect(locator).toHaveCount(0);
      }
    }

    // 6. Create a token WITH server:list. The POST /api/tokens response is the
    //    source of truth for the secret; the success dialog is then asserted
    //    to actually display that same secret (regression guard — the dialog
    //    body was previously rendered empty, leaking the token only to the
    //    network tab).
    await page
      .getByTestId('token-form-name')
      .locator('input')
      .fill(`pat-with-${role}-${STAMP}`);
    await page.getByTestId('token-ability-server:list').click();
    const withCreate = page.waitForResponse(
      (r) => r.url().includes('/api/tokens') && r.request().method() === 'POST',
    );
    await page.getByTestId('token-form-submit').click();
    const withResp = await withCreate;
    expect(
      withResp.ok(),
      `create token (with server:list) should be 2xx, got ${withResp.status()}`,
    ).toBeTruthy();
    const secretWith = ((await withResp.json()) as { token: string }).token;

    const withDialog = page.getByRole('dialog').last();
    await expect(withDialog.getByTestId('token-created-message')).toHaveText(
      TOKEN_ONCE,
    );
    await expect(
      withDialog.getByTestId('token-created-value').locator('input'),
    ).toHaveValue(secretWith);
    await expect(withDialog.getByTestId('token-created-copy')).toBeVisible();
    await dismissTopDialog(page);

    // 7. Create a token WITHOUT server:list (server:start instead).
    await page.getByTestId('tokens-generate-button').click();
    await expect(page.getByTestId('token-ability-server:start')).toBeVisible({
      timeout: 10_000,
    });
    await page
      .getByTestId('token-form-name')
      .locator('input')
      .fill(`pat-without-${role}-${STAMP}`);
    await page.getByTestId('token-ability-server:start').click();
    const withoutCreate = page.waitForResponse(
      (r) => r.url().includes('/api/tokens') && r.request().method() === 'POST',
    );
    await page.getByTestId('token-form-submit').click();
    const withoutResp = await withoutCreate;
    expect(
      withoutResp.ok(),
      `create token (without server:list) should be 2xx, got ${withoutResp.status()}`,
    ).toBeTruthy();
    const secretWithout = ((await withoutResp.json()) as { token: string })
      .token;

    const withoutDialog = page.getByRole('dialog').last();
    await expect(
      withoutDialog.getByTestId('token-created-message'),
    ).toHaveText(TOKEN_ONCE);
    await expect(
      withoutDialog.getByTestId('token-created-value').locator('input'),
    ).toHaveValue(secretWithout);
    await expect(
      withoutDialog.getByTestId('token-created-copy'),
    ).toBeVisible();
    await dismissTopDialog(page);

    // 8. The server:list token can list servers; the other is forbidden.
    const allowed = await request.get(`${API_BASE}/api/servers`, {
      headers: authHeader(secretWith),
    });
    await expectStatus(
      allowed,
      200,
      'token WITH server:list should list servers',
    );

    const denied = await request.get(`${API_BASE}/api/servers`, {
      headers: authHeader(secretWithout),
    });
    await expectStatus(
      denied,
      403,
      'token WITHOUT server:list must be forbidden',
    );
  });
}
