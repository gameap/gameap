import { test, expect, type Page } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// Upload modal with hundreds of files: the file-manager API and the chunked
// upload endpoints are route-mocked, so no daemon is required.

const FILES_TAB = /files|файлы|servers\.files/i;

const TS = 1752800000;

const FILE_COUNT = 300;
const EXISTING_COUNT = 40;

const pickedNames = Array.from(
  { length: FILE_COUNT },
  (_, i) => `map_${String(i + 1).padStart(3, '0')}.bsp`,
);

function fileEntry(path: string) {
  const dot = path.lastIndexOf('.');

  return {
    path,
    timestamp: TS,
    type: 'file',
    visibility: 'public',
    size: 64,
    dirname: '',
    basename: path,
    extension: path.slice(dot + 1),
    filename: path.slice(0, dot),
    mode: 420,
  };
}

async function setupRoutes(page: Page) {
  await page.route('**/api/servers/1/**', (route) => route.fulfill({ json: {} }));
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
      json: { 'game-server-common': true, 'game-server-files': true },
    }),
  );

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
  await page.route('**/api/file-manager/1/content*', (route) =>
    route.fulfill({
      json: {
        result: { status: 'success', message: null },
        directories: [],
        files: pickedNames.slice(0, EXISTING_COUNT).map(fileEntry),
      },
    }),
  );

  // map_007.bsp is rejected the way an nginx client_max_body_size limit
  // does it: a 413 with an HTML body.
  let sessionSeq = 0;
  await page.route('**/api/file-manager/1/upload/sessions', (route) => {
    const body = route.request().postDataJSON() as { filename: string; total_size: number };
    if (body.filename === 'map_007.bsp') {
      return route.fulfill({ status: 413, contentType: 'text/html', body: '<html>413</html>' });
    }
    sessionSeq += 1;

    return route.fulfill({
      json: {
        upload_id: `u${sessionSeq}`,
        chunk_size: 8 * 1024 * 1024,
        total_chunks: 1,
        total_size: body.total_size,
      },
    });
  });
  await page.route('**/api/file-manager/1/upload/sessions/*/chunks/*', (route) =>
    route.fulfill({ json: {} }),
  );
  await page.route('**/api/file-manager/1/upload/sessions/*/complete', (route) =>
    route.fulfill({ json: { completed: true } }),
  );
}

async function openUploadModal(page: Page) {
  await page.goto('/servers/1');
  await page
    .locator('.n-tabs-tab', { hasText: FILES_TAB })
    .click({ timeout: 20_000 });
  await expect(
    page.locator('.fm-row--file', { hasText: 'map_001.bsp' }).first(),
  ).toBeVisible({ timeout: 20_000 });
  await page.locator('button.fm-tool-btn[title="Upload"]').click();

  const modal = page.locator('.n-modal', { hasText: 'Upload files' });
  await expect(modal).toBeVisible();

  return modal;
}

async function pickFiles(page: Page, names: string[]) {
  await page
    .locator('.n-modal input.n-upload-file-input')
    .setInputFiles(
      names.map((name) => ({ name, mimeType: 'application/octet-stream', buffer: Buffer.alloc(16, 1) })),
    );
}

test.beforeEach(async ({ page, request }) => {
  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);
  await setupRoutes(page);
});

test('picked_files_are_reviewed_together_within_the_viewport', async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 640 });
  const modal = await openUploadModal(page);

  await pickFiles(page, pickedNames);

  // n-upload reports a multi-file pick one file at a time; every file of
  // the pick must reach one review, not just the last one.
  await expect(modal).toContainText(`Files: ${FILE_COUNT}`, { timeout: 20_000 });
  const conflicts = modal.locator('.n-alert', { hasText: 'Some items already exist' });
  await expect(conflicts).toBeVisible();
  await expect(conflicts).toContainText('Default action:');

  const fileRows = modal.locator('.upload-tree-node > .grid');
  await expect(fileRows).toHaveCount(FILE_COUNT);
  await expect(fileRows.first()).toContainText('map_001.bsp');
  await expect(fileRows.nth(1)).toContainText('map_002.bsp');
  await expect(modal.locator('.upload-tree-node .n-select')).toHaveCount(EXISTING_COUNT);

  const submit = modal.getByRole('button', { name: 'Submit' });
  await expect(submit).toBeInViewport({ ratio: 1 });
  const card = await modal.boundingBox();
  expect(card).not.toBeNull();
  expect(card!.y).toBeGreaterThanOrEqual(0);
  expect(card!.y + card!.height).toBeLessThanOrEqual(640);
});

test('failed_file_shows_its_reason_in_the_list_without_error_dialogs', async ({ page }) => {
  const modal = await openUploadModal(page);
  const names = pickedNames.slice(EXISTING_COUNT, EXISTING_COUNT + 20).concat('map_007.bsp');

  await pickFiles(page, names);
  await expect(modal).toContainText(`Files: ${names.length}`, { timeout: 20_000 });
  await modal.getByRole('button', { name: 'Submit' }).click();

  const result = modal.locator('.n-alert', { hasText: 'Upload finished with errors' });
  await expect(result).toBeVisible({ timeout: 30_000 });
  await expect(result).toContainText(`Uploaded: ${names.length - 1}, failed: 1, skipped: 0`);

  const failedRow = modal.locator('.upload-tree-node > .grid', { hasText: 'map_007.bsp' });
  await expect(failedRow).toContainText('Rejected by proxy (request too large)');
  await expect(failedRow).toContainText('Failed');
  await expect(modal.getByRole('button', { name: 'Retry failed' })).toBeVisible();

  await expect(page.locator('.n-dialog')).toHaveCount(0);
});
