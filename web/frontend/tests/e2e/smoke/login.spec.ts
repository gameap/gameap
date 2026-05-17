import { test, expect } from '@playwright/test';

test('admin can log in and is redirected away from /login', async ({ page }) => {
  const user = process.env.E2E_ADMIN_USER ?? 'admin';
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) {
    throw new Error('E2E_ADMIN_PASSWORD must be set');
  }

  await page.goto('/login');
  await page.locator('#email').fill(user);
  await page.locator('#password').fill(password);

  const loginResponse = page.waitForResponse(
    (response) =>
      response.url().includes('/api/auth/login') && response.request().method() === 'POST',
  );

  await page
    .getByRole('button', { name: /sign.?in|login|вход|войти|auth\.sign_in/i })
    .click();

  const response = await loginResponse;
  expect(response.status(), 'login API should return 200').toBe(200);

  await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
  await expect
    .poll(async () => await page.evaluate(() => localStorage.getItem('auth_token')))
    .toBeTruthy();
});
