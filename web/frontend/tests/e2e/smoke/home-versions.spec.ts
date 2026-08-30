import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// The dashboard version block tells an admin whether the panel and the daemons
// on the dedicated servers are up to date. Both endpoints it reads are
// route-mocked: no release source has to be reachable, no daemon has to run,
// and the assertions are on what the block renders for a given payload.

const VERSIONS_HEADING = /versions|версии|home\.versions/i;
const OUTDATED_SUMMARY = /2 of 5|2 из 5|home\.daemons_outdated/i;
const ALL_ACTUAL = /up to date|актуальны|home\.daemons_actual/i;
const UNAVAILABLE = /unavailable|недоступны|home\.daemons_unavailable/i;

const VERSION_UPDATE_AVAILABLE = {
  panel: {
    current: '4.0.0',
    build_date: '2026-01-15T10:00:00Z',
    is_release: true,
    latest_stable: '4.4.2',
    latest_stable_url: 'https://github.com/gameap/gameap/releases/tag/v4.4.2',
    latest_beta: '4.5.0-beta.1',
    latest_beta_url: 'https://github.com/gameap/gameap/releases/tag/v4.5.0-beta.1',
    update_available: true,
  },
  daemon: {
    latest_stable: '4.1.2',
    latest_stable_url: 'https://github.com/gameap/daemon/releases/tag/v4.1.2',
  },
  update_check_enabled: true,
};

const SUMMARY_WITH_OUTDATED = {
  total: 5,
  enabled: 5,
  disabled: 0,
  online: 3,
  offline: 2,
  onlineNodes: [
    { id: 1, name: 'node-eu-1', location: 'EU', enabled: true, online: true, version: '4.0.8', outdated: true },
    { id: 2, name: 'node-ru-2', location: 'RU', enabled: true, online: true, version: '4.1.0', outdated: true },
    { id: 3, name: 'node-de-3', location: 'DE', enabled: true, online: true, version: '4.1.2' },
  ],
  offlineNodes: [
    { id: 4, name: 'node-us-4', location: 'US', enabled: true, online: false },
    { id: 5, name: 'node-us-5', location: 'US', enabled: true, online: false },
  ],
};

const SUMMARY_ALL_ACTUAL = {
  total: 1,
  enabled: 1,
  disabled: 0,
  online: 1,
  offline: 0,
  onlineNodes: [
    { id: 1, name: 'node-eu-1', location: 'EU', enabled: true, online: true, version: '4.1.2' },
  ],
  offlineNodes: [],
};

async function openDashboard(
  page: import('@playwright/test').Page,
  request: import('@playwright/test').APIRequestContext,
  { version = VERSION_UPDATE_AVAILABLE, summary = SUMMARY_WITH_OUTDATED } = {},
) {
  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await page.route('**/api/version', (route) => route.fulfill({ json: version }));
  await page.route('**/api/nodes/summary', (route) => route.fulfill({ json: summary }));

  await page.goto('/');
}

test('the dashboard shows the panel update and only the outdated daemons', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openDashboard(page, request);

  const block = page.locator('div').filter({ hasText: VERSIONS_HEADING }).last();
  await expect(block).toBeVisible({ timeout: 15_000 });

  // The newer stable release is offered next to the installed one.
  const latestLink = block.getByRole('link', { name: '4.4.2' });
  await expect(latestLink).toBeVisible();
  await expect(latestLink).toHaveAttribute(
    'href',
    'https://github.com/gameap/gameap/releases/tag/v4.4.2',
  );
  await expect(block).toContainText('4.0.0');
  await expect(block).toContainText('4.5.0-beta.1');

  // Only the two outdated daemons are listed; the up-to-date one is not.
  await expect(block.getByRole('link', { name: /node-eu-1/ })).toBeVisible();
  await expect(block.getByRole('link', { name: /node-ru-2/ })).toBeVisible();
  await expect(block.getByRole('link', { name: /node-de-3/ })).toHaveCount(0);

  await expect(block).toContainText(OUTDATED_SUMMARY);
  await expect(block).toContainText(UNAVAILABLE);
});

test('the dashboard reports all daemons as up to date when none is outdated', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openDashboard(page, request, { summary: SUMMARY_ALL_ACTUAL });

  const block = page.locator('div').filter({ hasText: VERSIONS_HEADING }).last();
  await expect(block).toBeVisible({ timeout: 15_000 });

  await expect(block).toContainText(ALL_ACTUAL);
  await expect(block.getByRole('link', { name: /node-eu-1/ })).toHaveCount(0);
});

test('an up-to-date panel is not offered an update', async ({ page, request }) => {
  test.setTimeout(60_000);

  await openDashboard(page, request, {
    version: {
      ...VERSION_UPDATE_AVAILABLE,
      panel: {
        ...VERSION_UPDATE_AVAILABLE.panel,
        current: '4.4.2',
        latest_beta: '',
        latest_beta_url: '',
        update_available: false,
      },
    },
  });

  const block = page.locator('div').filter({ hasText: VERSIONS_HEADING }).last();
  await expect(block).toBeVisible({ timeout: 15_000 });

  await expect(block).toContainText('4.4.2');
  await expect(block.getByRole('link', { name: '4.4.2' })).toHaveCount(0);
  await expect(block).not.toContainText('4.5.0-beta.1');
});
