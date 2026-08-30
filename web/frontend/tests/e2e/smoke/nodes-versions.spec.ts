import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// The nodes page tells an admin which daemon version every node runs and
// whether a newer stable gameap-daemon release exists. Everything
// version-related is route-mocked: no daemon has to run, and the assertions
// are on what the cards and the details modal render for a given payload.

const GDAEMON_VERSION_ROW = /gameap daemon version|версия gameap daemon|dedicated_servers\.gdaemon_version/i;

const DAEMON_RELEASE_URL = 'https://github.com/gameap/daemon/releases/tag/v3.6.0';

const NODES = [
  { id: 1, enabled: true, name: 'node-outdated', os: 'linux', location: 'EU', provider: 'Hetzner', ip: ['10.0.0.1'] },
  { id: 2, enabled: true, name: 'node-actual', os: 'linux', location: 'US', provider: 'OVH', ip: ['10.0.0.2'] },
];

const SUMMARY = {
  total: 2,
  enabled: 2,
  disabled: 0,
  online: 2,
  offline: 0,
  onlineNodes: [
    { id: 1, name: 'node-outdated', location: 'EU', enabled: true, online: true, version: '3.0.1', buildDate: '2024-01-15', outdated: true },
    { id: 2, name: 'node-actual', location: 'US', enabled: true, online: true, version: '3.6.0', buildDate: '2026-08-01' },
  ],
  offlineNodes: [],
};

const VERSION = {
  panel: {
    current: '4.4.2',
    build_date: '2026-01-15T10:00:00Z',
    is_release: true,
    latest_stable: '4.4.2',
    latest_stable_url: 'https://github.com/gameap/gameap/releases/tag/v4.4.2',
    update_available: false,
  },
  daemon: {
    latest_stable: '3.6.0',
    latest_stable_url: DAEMON_RELEASE_URL,
  },
  update_check_enabled: true,
};

const DAEMON_INFO: Record<string, unknown> = {
  '1': {
    id: 1,
    name: 'node-outdated',
    has_api_key: true,
    connection_type: 'grpc',
    version: { version: '3.0.1', compile_date: '2024-01-15' },
    base_info: { uptime: '1d', working_tasks_count: '0', waiting_tasks_count: '0', online_servers_count: '1' },
  },
  '2': {
    id: 2,
    name: 'node-actual',
    has_api_key: true,
    connection_type: 'grpc',
    version: { version: '3.6.0', compile_date: '2026-08-01' },
    base_info: { uptime: '2d', working_tasks_count: '0', waiting_tasks_count: '0', online_servers_count: '2' },
  },
};

async function openNodes(
  page: import('@playwright/test').Page,
  request: import('@playwright/test').APIRequestContext,
  path = '/admin/nodes',
) {
  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await page.route('**/api/version', (route) => route.fulfill({ json: VERSION }));
  await page.route('**/api/nodes/summary', (route) => route.fulfill({ json: SUMMARY }));
  await page.route('**/api/nodes', (route) => route.fulfill({ json: NODES }));
  await page.route(/\/api\/nodes\/(\d+)\/daemon$/, (route) => {
    const id = route.request().url().match(/\/api\/nodes\/(\d+)\/daemon$/)?.[1] ?? '';

    return route.fulfill({ json: DAEMON_INFO[id] });
  });

  await page.goto(path);
}

test('the node card offers the newer daemon release only for the outdated node', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openNodes(page, request);

  const outdatedCard = page.locator('.node-card').filter({ hasText: 'node-outdated' });
  await expect(outdatedCard).toBeVisible({ timeout: 15_000 });
  await expect(outdatedCard).toContainText('v3.0.1');

  const latestLink = outdatedCard.getByRole('link', { name: '3.6.0' });
  await expect(latestLink).toBeVisible();
  await expect(latestLink).toHaveAttribute('href', DAEMON_RELEASE_URL);
  await expect(outdatedCard.locator('i.fa-triangle-exclamation')).toBeVisible();

  const actualCard = page.locator('.node-card').filter({ hasText: 'node-actual' });
  await expect(actualCard).toBeVisible();
  await expect(actualCard).toContainText('v3.6.0');
  await expect(actualCard.getByRole('link', { name: '3.6.0' })).toHaveCount(0);
});

test('the details modal links the newer daemon release for an outdated node', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openNodes(page, request, '/admin/nodes?node=1');

  const row = page.locator('tr').filter({ hasText: GDAEMON_VERSION_ROW });
  await expect(row).toBeVisible({ timeout: 15_000 });
  await expect(row).toContainText('3.0.1 (2024-01-15)');

  const latestLink = row.getByRole('link', { name: '3.6.0' });
  await expect(latestLink).toBeVisible();
  await expect(latestLink).toHaveAttribute('href', DAEMON_RELEASE_URL);
  await expect(row.locator('i.fa-triangle-exclamation')).toBeVisible();
});

test('the details modal marks an up-to-date daemon with a quiet check', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  await openNodes(page, request, '/admin/nodes?node=2');

  const row = page.locator('tr').filter({ hasText: GDAEMON_VERSION_ROW });
  await expect(row).toBeVisible({ timeout: 15_000 });
  await expect(row).toContainText('3.6.0 (2026-08-01)');

  await expect(row.getByRole('link', { name: '3.6.0' })).toHaveCount(0);
  await expect(row.locator('i.fa-check')).toBeVisible();
});
