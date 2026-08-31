import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// The dashboard version block tells an admin whether the panel and the daemons
// on the dedicated servers are up to date. Both endpoints it reads are
// route-mocked: no release source has to be reachable, no daemon has to run,
// and the assertions are on what the block renders for a given payload.

const LATEST_STABLE = /latest stable|последняя стабильная/i;
const OUTDATED_TAG = /2 of 5|2 из 5/i;
const ALL_ACTUAL = /all up to date|всё в актуальном состоянии/i;
const INFO_UNAVAILABLE = /failed to get information|не удалось получить информацию/i;
const MODAL_TITLE = /gameap daemon versions|версии gameap daemon/i;
const ROW_OUTDATED = /outdated|устаревшая/i;
const ROW_ACTUAL = /up to date|актуальная/i;
const ROW_UNKNOWN = /unknown|неизвестно/i;
const CHECK_DISABLED = /update check is disabled|проверка обновлений отключена/i;

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

// The version in use is a badge on the heading line, right after the product
// name, so the heading is the first div of a column.
function heading(section: import('@playwright/test').Locator) {
  return section.locator('div').first();
}

function versionSections(page: import('@playwright/test').Page) {
  return {
    panelSection: page.locator('section').filter({ has: page.getByText('GameAP', { exact: true }) }),
    daemonSection: page.locator('section').filter({ has: page.getByText('GameAP Daemon', { exact: true }) }),
  };
}

test('the dashboard shows the outdated panel and daemon versions the same way', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openDashboard(page, request);

  const { panelSection, daemonSection } = versionSections(page);
  await expect(panelSection).toBeVisible({ timeout: 15_000 });

  // Both columns lead with the same latest-release lines.
  await expect(panelSection).toContainText(LATEST_STABLE);
  const latestLink = panelSection.getByRole('link', { name: '4.4.2' });
  await expect(latestLink).toHaveAttribute(
    'href',
    'https://github.com/gameap/gameap/releases/tag/v4.4.2',
  );
  await expect(panelSection).toContainText('4.5.0-beta.1');
  await expect(daemonSection).toContainText(LATEST_STABLE);
  await expect(daemonSection.getByRole('link', { name: '4.1.2' })).toBeVisible();

  // Each column carries exactly one status badge, worn by its heading.
  await expect(panelSection.locator('.badge-orange')).toHaveText('4.0.0');
  await expect(heading(panelSection)).toContainText('4.0.0');
  await expect(daemonSection.locator('.badge-orange')).toContainText(OUTDATED_TAG);
  await expect(heading(daemonSection)).toContainText(OUTDATED_TAG);

  // The individual daemons are not listed on the dashboard itself.
  await expect(panelSection.locator('.badge-green')).toHaveCount(0);
  await expect(daemonSection.getByRole('link', { name: /node-eu-1/ })).toHaveCount(0);
});

test('the outdated tag opens a modal listing every dedicated server state', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openDashboard(page, request);

  const { daemonSection } = versionSections(page);
  await daemonSection.getByRole('button', { name: OUTDATED_TAG }).click();

  const modal = page.locator('.n-modal').filter({ hasText: MODAL_TITLE });
  await expect(modal).toBeVisible();
  await expect(modal.getByRole('link')).toHaveCount(5);

  // Outdated servers come first, unreachable ones next, actual ones last.
  await expect(modal.getByRole('link').nth(0)).toContainText('node-eu-1');
  await expect(modal.getByRole('link').nth(1)).toContainText('node-ru-2');

  await expect(modal.getByRole('link', { name: /node-eu-1/ })).toContainText(ROW_OUTDATED);
  await expect(modal.getByRole('link', { name: /node-ru-2/ })).toContainText(ROW_OUTDATED);
  await expect(modal.getByRole('link', { name: /node-de-3/ })).toContainText(ROW_ACTUAL);
  await expect(modal.getByRole('link', { name: /node-us-4/ })).toContainText(ROW_UNKNOWN);
  await expect(modal.getByRole('link', { name: /node-us-5/ })).toContainText(ROW_UNKNOWN);
});

test('the dashboard reports all daemons as up to date when none is outdated', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openDashboard(page, request, { summary: SUMMARY_ALL_ACTUAL });

  const { daemonSection } = versionSections(page);
  await expect(daemonSection).toBeVisible({ timeout: 15_000 });
  await expect(daemonSection.locator('.badge-green')).toHaveText(ALL_ACTUAL);

  // The green tag opens the same per-server list.
  await daemonSection.getByRole('button', { name: ALL_ACTUAL }).click();
  const modal = page.locator('.n-modal').filter({ hasText: MODAL_TITLE });
  await expect(modal).toBeVisible();
  await expect(modal.getByRole('link', { name: /node-eu-1/ })).toContainText(ROW_ACTUAL);
});

test('an up-to-date panel wears a green version tag', async ({ page, request }) => {
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

  const { panelSection } = versionSections(page);
  await expect(panelSection).toBeVisible({ timeout: 15_000 });

  await expect(panelSection.locator('.badge-green')).toHaveText('4.4.2');
  await expect(panelSection).not.toContainText('4.5.0-beta.1');
});

test('an unreachable daemon turns the status into a dead-end red tag', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openDashboard(page, request, {
    summary: {
      total: 2,
      enabled: 2,
      disabled: 0,
      online: 1,
      offline: 1,
      onlineNodes: [
        { id: 1, name: 'node-eu-1', location: 'EU', enabled: true, online: true, version: '4.1.2' },
      ],
      offlineNodes: [
        { id: 2, name: 'node-us-2', location: 'US', enabled: true, online: false },
      ],
    },
  });

  const { daemonSection } = versionSections(page);
  await expect(daemonSection).toBeVisible({ timeout: 15_000 });

  await expect(daemonSection.locator('.badge-red')).toHaveText(INFO_UNAVAILABLE);
  await expect(daemonSection).not.toContainText(ALL_ACTUAL);
  // The red tag is informational only — nothing to click through to.
  await expect(daemonSection.getByRole('button')).toHaveCount(0);
});

test('a disabled update check leaves only the versions actually in use', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openDashboard(page, request, {
    version: {
      panel: {
        current: '4.0.0',
        build_date: '2026-01-15T10:00:00Z',
        is_release: true,
        update_available: false,
      },
      daemon: {},
      update_check_enabled: false,
    },
  });

  const { panelSection, daemonSection } = versionSections(page);
  await expect(panelSection).toBeVisible({ timeout: 15_000 });

  await expect(panelSection.locator('.badge-stone')).toHaveText('4.0.0');
  await expect(panelSection).not.toContainText(LATEST_STABLE);
  await expect(daemonSection.locator('[class*="badge-"]')).toHaveCount(0);
  await expect(page.getByText(CHECK_DISABLED)).toBeVisible();
});
