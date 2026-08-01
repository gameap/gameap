import { test, expect } from '@playwright/test';
import { loginViaAPI, authHeader } from '../fixtures/auth';
import { deleteUser } from '../fixtures/users';
import { expectStatus, dismissTopDialog, loginViaUI } from '../fixtures/ui';

// Full admin user lifecycle driven through the UI: create → verify in the
// list → edit (name + role) → verify server-side → delete.

const STAMP = Date.now();
const LOGIN = `e2e_au_${STAMP}`;
const EMAIL = `e2e_au_${STAMP}@example.com`;
const PASSWORD = `AdmUsr_${STAMP}`;
const NAME = `AU Created ${STAMP}`;
const NEW_NAME = `AU Edited ${STAMP}`;

// i18n-tolerant matchers — the SPA renders raw keys when window.i18n is empty.
const USERS_TITLE = /users|пользователи|users\.users/i;
const CONFIRM_YES = /^(yes|да|main\.yes)$/i;

// The role labels are hard-coded in the create/update forms (not translated).
const ROLE_USER = /^User$/;
const ROLE_ADMIN = /^Administrator$/;

// naive-ui renders <n-select> options in a body-level portal as plain divs
// without role="option", so they are matched by their own class — the same
// convention games-edit.spec.ts uses for `.n-tabs-tab`.
async function pickRole(
  page: import('@playwright/test').Page,
  testId: string,
  label: RegExp,
): Promise<void> {
  await page.getByTestId(testId).click();
  const option = page
    .locator('.n-base-select-option')
    .filter({ hasText: label })
    .first();
  await expect(option).toBeVisible({ timeout: 10_000 });
  await option.click();
  await page.keyboard.press('Escape');
}

const BASE_URL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8025';

let adminToken: string | undefined;
let createdUserId: number | undefined;

type APIRequest = import('@playwright/test').APIRequestContext;

interface UserListEntry {
  id: number;
  login: string;
  name: string;
  email: string;
}

// GET /api/users omits roles (see internal/api/users/getusers/response.go), so
// the list is only used to resolve the id.
async function findUserByLogin(
  request: APIRequest,
  token: string,
  login: string,
): Promise<UserListEntry | undefined> {
  const response = await request.get(`${BASE_URL}/api/users`, {
    headers: authHeader(token),
  });
  await expectStatus(response, 200, 'GET /api/users should be 200');

  const body = (await response.json()) as UserListEntry[];

  return body.find((u) => u.login === login);
}

// GET /api/users/{id} is the only endpoint that carries roles.
async function getUserDetails(
  request: APIRequest,
  token: string,
  id: number,
): Promise<UserListEntry & { roles: string[] }> {
  const response = await request.get(`${BASE_URL}/api/users/${id}`, {
    headers: authHeader(token),
  });
  await expectStatus(response, 200, `GET /api/users/${id} should be 200`);

  return (await response.json()) as UserListEntry & { roles: string[] };
}

test.afterEach(async ({ request }) => {
  if (adminToken && createdUserId !== undefined) {
    await deleteUser(request, adminToken, createdUserId);
    createdUserId = undefined;
  }
});

test('admin creates, edits and deletes a user through the UI', async ({
  page,
  request,
}) => {
  test.setTimeout(90_000);

  adminToken = await loginViaAPI(request);

  // 1. Sign in as the admin and open the users list.
  await loginViaUI(
    page,
    process.env.E2E_ADMIN_USER ?? 'admin',
    process.env.E2E_ADMIN_PASSWORD ?? '',
  );
  await page.locator('.sidebar-menu a[href="/admin/users"]').first().click();
  await expect(page).toHaveURL(/\/admin\/users$/, { timeout: 15_000 });
  await expect(page.getByRole('heading', { name: USERS_TITLE }).or(
    page.getByTestId('users-table'),
  ).first()).toBeVisible({ timeout: 15_000 });

  // 2. Create the user through the modal form.
  await page.getByTestId('users-create-button').click();
  await expect(
    page.getByTestId('create-user-login').locator('input'),
  ).toBeVisible({ timeout: 10_000 });

  await page.getByTestId('create-user-login').locator('input').fill(LOGIN);
  await page.getByTestId('create-user-email').locator('input').fill(EMAIL);
  await page.getByTestId('create-user-name').locator('input').fill(NAME);
  await page.getByTestId('create-user-password').locator('input').fill(PASSWORD);
  await page
    .getByTestId('create-user-password-confirmation')
    .locator('input')
    .fill(PASSWORD);

  await pickRole(page, 'create-user-roles', ROLE_USER);

  const createResponse = page.waitForResponse(
    (r) =>
      r.url().includes('/api/users') &&
      r.request().method() === 'POST' &&
      !r.url().includes('/servers'),
  );
  await page.getByTestId('create-user-submit').click();
  await expectStatus(await createResponse, 201, 'POST /api/users should be 201');

  // The list only refetches from the success-notification callback (see
  // AdminUsersView.onCreate), so the dialog must be dismissed first.
  await dismissTopDialog(page);

  // 3. The user exists server-side; capture its id for the row selectors and
  //    for teardown.
  const created = await findUserByLogin(request, adminToken, LOGIN);
  expect(created, `user ${LOGIN} must exist after creation`).toBeTruthy();
  createdUserId = created!.id;
  expect(created!.name).toBe(NAME);
  expect(created!.email).toBe(EMAIL);

  const createdDetails = await getUserDetails(request, adminToken, createdUserId);
  expect(createdDetails.roles).toContain('user');
  expect(createdDetails.roles).not.toContain('admin');

  // 4. The new row is rendered in the list with its action buttons.
  const editButton = page.getByTestId(`user-row-edit-${createdUserId}`);
  await expect(editButton).toBeVisible({ timeout: 15_000 });
  await expect(
    page.getByTestId(`user-row-delete-${createdUserId}`),
  ).toBeVisible();

  // 5. Open the edit page and change the name and role.
  await editButton.click();
  await expect(page).toHaveURL(
    new RegExp(`/admin/users/${createdUserId}/edit$`),
    { timeout: 15_000 },
  );

  const nameInput = page.getByTestId('user-edit-name').locator('input');
  await expect(nameInput).toHaveValue(NAME, { timeout: 15_000 });
  await nameInput.fill(NEW_NAME);

  await pickRole(page, 'user-edit-roles', ROLE_ADMIN);

  const updateResponse = page.waitForResponse(
    (r) =>
      r.url().includes(`/api/users/${createdUserId}`) &&
      r.request().method() === 'PUT',
  );
  await page.getByTestId('user-edit-save').click();
  await expectStatus(
    await updateResponse,
    200,
    `PUT /api/users/${createdUserId} should be 200`,
  );
  await dismissTopDialog(page);

  // 6. Cross-check persistence through the API rather than the UI's own state.
  const updated = await getUserDetails(request, adminToken, createdUserId);
  expect(updated.name).toBe(NEW_NAME);
  expect(updated.roles).toContain('admin');
  expect(updated.roles).toContain('user');

  // 7. Delete the user from the list and confirm the dialog.
  await page.goto('/admin/users');
  const deleteButton = page.getByTestId(`user-row-delete-${createdUserId}`);
  await expect(deleteButton).toBeVisible({ timeout: 15_000 });

  const deleteResponse = page.waitForResponse(
    (r) =>
      r.url().includes(`/api/users/${createdUserId}`) &&
      r.request().method() === 'DELETE',
  );
  await deleteButton.click();
  await page.getByRole('button', { name: CONFIRM_YES }).click();
  await expectStatus(
    await deleteResponse,
    204,
    `DELETE /api/users/${createdUserId} should be 204`,
  );
  await dismissTopDialog(page);

  // 8. The row is gone and the user no longer exists server-side.
  await expect(deleteButton).toHaveCount(0, { timeout: 15_000 });
  expect(await findUserByLogin(request, adminToken, LOGIN)).toBeUndefined();
  createdUserId = undefined;
});

test('an admin cannot delete their own account from the users list', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  adminToken = await loginViaAPI(request);
  const self = await findUserByLogin(
    request,
    adminToken,
    process.env.E2E_ADMIN_USER ?? 'admin',
  );
  expect(self, 'the admin account must be listed').toBeTruthy();

  await loginViaUI(
    page,
    process.env.E2E_ADMIN_USER ?? 'admin',
    process.env.E2E_ADMIN_PASSWORD ?? '',
  );
  await page.goto('/admin/users');

  // The delete action for the signed-in admin is rendered disabled, so the
  // self-deletion guard cannot even be triggered from the UI.
  const selfDelete = page.getByTestId(`user-row-delete-${self!.id}`);
  await expect(selfDelete).toBeVisible({ timeout: 15_000 });
  await expect(selfDelete).toBeDisabled();
});
