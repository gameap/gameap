import { test, expect, type Page, type WebSocketRoute } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { dismissTopDialog } from '../fixtures/ui';

// Server-side hash / archive / extract coverage: the file-manager API and
// the archive-operations WebSocket are fully mocked, so no daemon is
// required — the spec verifies the modals' payloads and the progress UI
// driven by pushed WS frames.

const FILES_TAB = /files|файлы|servers\.files/i;

const TS = 1752800000;

function dirEntry(path: string) {
  const basename = path.split('/').pop() ?? path;

  return { path, timestamp: TS, type: 'dir', dirname: '', basename, mode: 493 };
}

function fileEntry(path: string, size = 64) {
  const basename = path.split('/').pop() ?? path;
  const dot = basename.lastIndexOf('.');

  return {
    path,
    timestamp: TS,
    type: 'file',
    visibility: 'public',
    size,
    dirname: '',
    basename,
    extension: dot > 0 ? basename.slice(dot + 1) : undefined,
    filename: dot > 0 ? basename.slice(0, dot) : basename,
    mode: 420,
  };
}

async function mountFileManager(page: Page) {
  await page.goto('/servers/1');
  await page
    .locator('.n-tabs-tab', { hasText: FILES_TAB })
    .click({ timeout: 20_000 });
  await expect(
    page.locator('.fm-row--file', { hasText: 'config.txt' }).first(),
  ).toBeVisible({ timeout: 20_000 });
}

async function setupCommonRoutes(page: Page) {
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

  // A hermetic short-lived token so the WS connect never hits the backend.
  await page.route('**/api/auth/short-lived-token', (route) =>
    route.fulfill({ json: { token: 'slt-e2e' } }),
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
        directories: [dirEntry('subdir')],
        files: [
          fileEntry('config.txt'),
          fileEntry('notes.txt', 128),
          fileEntry('backup.tar.gz', 2048),
          fileEntry('big.bin', 2 * 1024 * 1024),
        ],
      },
    }),
  );
}

async function openContextMenuFor(page: Page, rowText: string, item: string) {
  await page
    .locator('.fm-row--file', { hasText: rowText })
    .first()
    .click({ button: 'right' });
  await page.locator('.fm-context-menu li', { hasText: item }).click();
}

type HashBody = { disk: string; algorithm: string; paths: string[] };

// notes.txt + md5 is the designated per-item error case; everything else
// returns a deterministic `${algorithm}-${path}-cafe` value.
async function mockHashRoute(page: Page): Promise<HashBody[]> {
  const hashBodies: HashBody[] = [];
  await page.route('**/api/file-manager/1/hash', (route) => {
    const body = route.request().postDataJSON() as HashBody;
    hashBodies.push(body);
    void route.fulfill({
      json: {
        algorithm: body.algorithm,
        items: body.paths.map((p) =>
          p === 'notes.txt' && body.algorithm === 'md5'
            ? { path: p, error: 'fileNotFound', size: 0 }
            : { path: p, hash: `${body.algorithm}-${p}-cafe`, size: 64 },
        ),
      },
    });
  });

  return hashBodies;
}

test('hash modal auto-computes sha256 and md5 for a small file', async ({ page, request }) => {
  test.setTimeout(120_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await setupCommonRoutes(page);
  const hashBodies = await mockHashRoute(page);

  await mountFileManager(page);

  await page.locator('.fm-row--file', { hasText: 'config.txt' }).first().click();
  await openContextMenuFor(page, 'config.txt', 'Checksums');

  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('File checksums');

  await expect.poll(() => hashBodies.length).toBe(2);
  expect(hashBodies.map((b) => b.algorithm).sort()).toEqual(['md5', 'sha256']);
  for (const body of hashBodies) {
    expect(body.disk).toBe('server');
    expect(body.paths).toEqual(['config.txt']);
  }

  await expect(dialog).toContainText('sha256-config.txt-cafe');
  await expect(dialog).toContainText('md5-config.txt-cafe');

  const sha1Row = dialog.locator('.fm-hash-row', { hasText: 'SHA1' });
  await expect(sha1Row.getByRole('button', { name: 'Compute' })).toBeVisible();

  const crc32Row = dialog.locator('.fm-hash-row', { hasText: 'CRC32' });
  await crc32Row.getByRole('button', { name: 'Compute' }).click();

  await expect.poll(() => hashBodies.length).toBe(3);
  expect(hashBodies[2].algorithm).toBe('crc32');
  expect(hashBodies[2].paths).toEqual(['config.txt']);
  await expect(dialog).toContainText('crc32-config.txt-cafe');
});

test('hash modal computes only sha256 automatically for a large file', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await setupCommonRoutes(page);
  const hashBodies = await mockHashRoute(page);

  await mountFileManager(page);

  await page.locator('.fm-row--file', { hasText: 'big.bin' }).first().click();
  await openContextMenuFor(page, 'big.bin', 'Checksums');

  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('sha256-big.bin-cafe');

  expect(hashBodies.length).toBe(1);
  expect(hashBodies[0].algorithm).toBe('sha256');
  expect(hashBodies[0].paths).toEqual(['big.bin']);

  const md5Row = dialog.locator('.fm-hash-row', { hasText: 'MD5' });
  await expect(md5Row.getByRole('button', { name: 'Compute' })).toBeVisible();
});

test('hash modal shows a per-item error and retries on demand', async ({ page, request }) => {
  test.setTimeout(120_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await setupCommonRoutes(page);
  const hashBodies = await mockHashRoute(page);

  await mountFileManager(page);

  await page.locator('.fm-row--file', { hasText: 'notes.txt' }).first().click();
  await openContextMenuFor(page, 'notes.txt', 'Checksums');

  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('sha256-notes.txt-cafe');
  await expect(dialog).toContainText('File not found!');
  await expect.poll(() => hashBodies.length).toBe(2);

  const md5Row = dialog.locator('.fm-hash-row', { hasText: 'MD5' });
  await md5Row.getByRole('button', { name: 'Compute' }).click();

  await expect.poll(() => hashBodies.length).toBe(3);
  expect(hashBodies[2].algorithm).toBe('md5');
  expect(hashBodies[2].paths).toEqual(['notes.txt']);
});

test('checksums menu entry requires a single file selection', async ({ page, request }) => {
  test.setTimeout(120_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await setupCommonRoutes(page);

  await mountFileManager(page);

  await page.locator('.fm-row--file', { hasText: 'config.txt' }).first().click();
  await page
    .locator('.fm-row--file', { hasText: 'notes.txt' })
    .first()
    .click({ modifiers: ['ControlOrMeta'] });
  await page
    .locator('.fm-row--file', { hasText: 'notes.txt' })
    .first()
    .click({ button: 'right' });

  const menu = page.locator('.fm-context-menu');
  await expect(menu).toBeVisible();
  await expect(menu.locator('li', { hasText: 'Delete' })).toBeVisible();
  await expect(menu.locator('li', { hasText: 'Checksums' })).toHaveCount(0);
});

test('create archive drives progress over WS and cancel posts the cancel endpoint', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await setupCommonRoutes(page);

  let wsRoute: WebSocketRoute | null = null;
  await page.routeWebSocket('**/file-manager/archive-operations*', (ws) => {
    wsRoute = ws;
    ws.onMessage((message) => {
      try {
        const msg = JSON.parse(String(message));
        if (msg.type === 'ping') {
          ws.send(JSON.stringify({ type: 'pong' }));
        }
      } catch {
        /* non-JSON frames are ignored */
      }
    });
  });

  const archiveBodies: Array<Record<string, unknown>> = [];
  await page.route('**/api/file-manager/1/archive', (route) => {
    archiveBodies.push(route.request().postDataJSON() as Record<string, unknown>);
    void route.fulfill({ status: 202, json: { operation_id: 'op-1' } });
  });

  const cancelCalls: string[] = [];
  await page.route('**/api/file-manager/1/archive-operations/*/cancel', (route) => {
    cancelCalls.push(route.request().url());
    void route.fulfill({
      status: 202,
      json: { result: { status: 'success', message: '' } },
    });
  });

  await mountFileManager(page);

  await openContextMenuFor(page, 'config.txt', 'Zip');

  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('Create archive');

  const nameInput = dialog.locator('#fm-zip-name input');
  await nameInput.fill('bundle.tar.gz');
  await expect(dialog.locator('.n-select')).toContainText('tar.gz');

  await dialog.getByRole('button', { name: 'Submit' }).click();

  await expect.poll(() => archiveBodies.length).toBe(1);
  expect(archiveBodies[0]).toMatchObject({
    disk: 'server',
    path: '',
    name: 'bundle.tar.gz',
    format: 'tar_gz',
    sources: ['config.txt'],
    overwrite: false,
  });

  await expect.poll(() => wsRoute !== null).toBe(true);

  wsRoute!.send(
    JSON.stringify({
      type: 'archive.progress',
      payload: {
        operation_id: 'op-1',
        kind: 'create',
        files_processed: 1,
        files_total: 2,
        bytes_processed: 10,
        bytes_total: 100,
      },
    }),
  );

  const progressRow = page.locator('.fm-progress-block');
  await expect(progressRow).toContainText('Creating archive: bundle.tar.gz');
  await expect(progressRow).toContainText('1/2 files');

  await progressRow.getByRole('button', { name: 'Cancel' }).click();
  await expect.poll(() => cancelCalls.length).toBe(1);
  expect(cancelCalls[0]).toContain('/archive-operations/op-1/cancel');

  wsRoute!.send(
    JSON.stringify({
      type: 'archive.complete',
      payload: { operation_id: 'op-1', success: false, error: 'canceled: by user' },
    }),
  );

  await expect(page.getByRole('dialog').last()).toContainText('Operation cancelled', {
    timeout: 10_000,
  });
  await dismissTopDialog(page);
});

test('extract archive posts the payload and completion notifies and refreshes', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await setupCommonRoutes(page);

  let contentRequests = 0;
  await page.route('**/api/file-manager/1/content*', (route) => {
    contentRequests += 1;
    void route.fulfill({
      json: {
        result: { status: 'success', message: null },
        directories: [dirEntry('subdir')],
        files: [
          fileEntry('config.txt'),
          fileEntry('notes.txt', 128),
          fileEntry('backup.tar.gz', 2048),
        ],
      },
    });
  });

  let wsRoute: WebSocketRoute | null = null;
  await page.routeWebSocket('**/file-manager/archive-operations*', (ws) => {
    wsRoute = ws;
    ws.onMessage((message) => {
      try {
        const msg = JSON.parse(String(message));
        if (msg.type === 'ping') {
          ws.send(JSON.stringify({ type: 'pong' }));
        }
      } catch {
        /* non-JSON frames are ignored */
      }
    });
  });

  const extractBodies: Array<Record<string, unknown>> = [];
  await page.route('**/api/file-manager/1/extract', (route) => {
    extractBodies.push(route.request().postDataJSON() as Record<string, unknown>);
    void route.fulfill({ status: 202, json: { operation_id: 'op-2' } });
  });

  await mountFileManager(page);

  await openContextMenuFor(page, 'backup.tar.gz', 'Unzip');

  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('Unpack archive');

  // Default extraction target is a new folder named after the archive.
  await expect(dialog.locator('#fm-unzip-folder input')).toHaveValue('backup');

  await dialog.getByText('Overwrite', { exact: true }).click();
  await expect(dialog).toContainText('the files will be overwritten');

  const baselineContentRequests = contentRequests;

  await dialog.getByRole('button', { name: 'Submit' }).click();

  await expect.poll(() => extractBodies.length).toBe(1);
  expect(extractBodies[0]).toMatchObject({
    disk: 'server',
    path: 'backup.tar.gz',
    destination: 'backup',
    conflict_policy: 'overwrite',
    create_destination: true,
  });

  await expect.poll(() => wsRoute !== null).toBe(true);

  wsRoute!.send(
    JSON.stringify({
      type: 'archive.complete',
      payload: {
        operation_id: 'op-2',
        success: true,
        files_processed: 5,
        bytes_processed: 2048,
      },
    }),
  );

  await expect(page.getByRole('dialog').last()).toContainText('Archive extracted!', {
    timeout: 10_000,
  });
  await dismissTopDialog(page);

  await expect
    .poll(() => contentRequests, { timeout: 10_000 })
    .toBeGreaterThan(baselineContentRequests);
});
