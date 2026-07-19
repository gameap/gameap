import { test, expect, type Page } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { dismissTopDialog } from '../fixtures/ui';

// Same-directory paste regression coverage (file self-overwrite bug): the
// file-manager API is fully route-mocked, so no daemon is required — the
// spec verifies the frontend clipboard logic and the exact paste payloads.

const FILES_TAB = /files|файлы|servers\.files/i;

const TS = 1752800000;

function dirEntry(path: string) {
  const basename = path.split('/').pop() ?? path;
  const dirname = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : '';

  return { path, timestamp: TS, type: 'dir', dirname, basename, mode: 493 };
}

function fileEntry(path: string) {
  const basename = path.split('/').pop() ?? path;
  const dirname = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : '';
  const dot = basename.lastIndexOf('.');

  return {
    path,
    timestamp: TS,
    type: 'file',
    visibility: 'public',
    size: 64,
    dirname,
    basename,
    extension: dot > 0 ? basename.slice(dot + 1) : undefined,
    filename: dot > 0 ? basename.slice(0, dot) : basename,
    mode: 420,
  };
}

// Root already contains config_1.txt so the auto-suffix must skip to _2.
const LISTINGS: Record<string, { directories: object[]; files: object[] }> = {
  '': {
    directories: [dirEntry('subdir')],
    files: [fileEntry('config.txt'), fileEntry('config_1.txt')],
  },
  subdir: {
    directories: [],
    files: [fileEntry('subdir/other.txt')],
  },
};

async function clickTool(page: Page, title: string) {
  await page.locator(`button.fm-tool-btn[title="${title}"]`).click();
}

test('same-dir paste auto-renames, same-dir cut is a no-op, folder is not pasted into itself', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  const pasteBodies: Array<Record<string, unknown>> = [];

  // The server page API is mocked as well: CI runs without any server row
  // in the DB (servers cannot be provisioned without an enrolled daemon),
  // so the spec must not depend on GET /api/servers/1 hitting the backend.
  await page.route('**/api/servers/1/**', (route) =>
    route.fulfill({ json: {} }),
  );
  await page.route('**/api/servers/1', (route) =>
    route.fulfill({
      json: {
        id: 1,
        uid: '11111111-1111-1111-1111-111111111111',
        uuid: '11111111-1111-1111-1111-111111111111',
        uuid_short: '11111111',
        enabled: true,
        installed: 1,
        blocked: false,
        name: 'E2E FM Server',
        game_id: 'cs',
        ds_id: 1,
        game_mod_id: 1,
        server_ip: '127.0.0.1',
        server_port: 27015,
        online: false,
        game: { code: 'cs', name: 'Counter-Strike' },
      },
    }),
  );
  await page.route('**/api/servers/1/abilities', (route) =>
    route.fulfill({
      json: {
        'game-server-common': true,
        'game-server-files': true,
      },
    }),
  );

  // Catch-all first: overlapping routes registered later win, so anything
  // not mocked explicitly still gets a harmless success instead of hitting
  // the daemon-less backend.
  await page.route('**/api/file-manager/1/**', (route) =>
    route.fulfill({ json: { result: { status: 'success', message: '' } } }),
  );
  await page.route('**/api/file-manager/1/initialize*', (route) =>
    route.fulfill({
      json: {
        result: { status: 'success', message: null },
        config: {
          acl: false,
          hiddenFiles: true,
          disks: { server: { driver: 'local' } },
          lang: 'en',
          leftDisk: 'server',
          leftPath: '',
          windowsConfig: 1,
        },
      },
    }),
  );
  await page.route('**/api/file-manager/1/content*', (route) => {
    const url = new URL(route.request().url());
    const path = url.searchParams.get('path') ?? '';
    const listing = LISTINGS[path] ?? { directories: [], files: [] };
    void route.fulfill({
      json: { result: { status: 'success', message: null }, ...listing },
    });
  });
  await page.route('**/api/file-manager/1/paste', (route) => {
    pasteBodies.push(route.request().postDataJSON() as Record<string, unknown>);
    void route.fulfill({ json: { result: { status: 'success', message: '' } } });
  });

  await page.goto('/servers/1');
  await page
    .locator('.n-tabs-tab', { hasText: FILES_TAB })
    .click({ timeout: 20_000 });

  const rootFile = page.locator('.fm-row--file', { hasText: 'config.txt' }).first();
  await expect(rootFile).toBeVisible({ timeout: 20_000 });

  // 1. Copy + paste in the same directory → auto-renamed payload; the
  //    suffix skips the already-existing config_1.txt.
  await rootFile.click();
  await clickTool(page, 'Copy');
  await dismissTopDialog(page); // "Copied to clipboard!"
  await clickTool(page, 'Paste');
  await expect.poll(() => pasteBodies.length).toBe(1);
  expect(pasteBodies[0].names).toEqual({ 'config.txt': 'config_2.txt' });
  expect(pasteBodies[0].path).toBeNull();
  expect((pasteBodies[0].clipboard as { type: string }).type).toBe('copy');

  // 2. Cut + paste in the same directory → no request, info dialog,
  //    clipboard survives (paste button stays enabled).
  await rootFile.click();
  await clickTool(page, 'Cut');
  await dismissTopDialog(page); // "Cut to clipboard!"
  await clickTool(page, 'Paste');
  await expect(page.getByRole('dialog').last()).toContainText(
    'Items are already in this folder',
    { timeout: 10_000 },
  );
  await dismissTopDialog(page);
  expect(pasteBodies).toHaveLength(1);
  await expect(
    page.locator('button.fm-tool-btn[title="Paste"]'),
  ).toBeEnabled();

  // 3. Copy a folder, enter it, paste → error dialog, no request.
  const subdirRow = page.locator('.fm-row--directory', { hasText: 'subdir' });
  await subdirRow.click();
  await clickTool(page, 'Copy');
  await dismissTopDialog(page);
  await subdirRow.dblclick();
  await expect(
    page.locator('.fm-row--file', { hasText: 'other.txt' }),
  ).toBeVisible({ timeout: 10_000 });
  await clickTool(page, 'Paste');
  await expect(page.getByRole('dialog').last()).toContainText(
    'Cannot paste a folder into itself',
    { timeout: 10_000 },
  );
  await dismissTopDialog(page);
  expect(pasteBodies).toHaveLength(1);

  // 4. Cross-directory copy stays unchanged: no names in the payload.
  await page.locator('.fm-row--up').click();
  await expect(rootFile).toBeVisible({ timeout: 10_000 });
  await rootFile.click();
  await clickTool(page, 'Copy');
  await dismissTopDialog(page);
  await subdirRow.dblclick();
  await expect(
    page.locator('.fm-row--file', { hasText: 'other.txt' }),
  ).toBeVisible({ timeout: 10_000 });
  await clickTool(page, 'Paste');
  await expect.poll(() => pasteBodies.length).toBe(2);
  expect(pasteBodies[1].path).toBe('subdir');
  expect(pasteBodies[1].names).toBeUndefined();
  expect((pasteBodies[1].clipboard as { files: string[] }).files).toEqual([
    'config.txt',
  ]);
});
