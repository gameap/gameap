import { statSync } from 'node:fs';
import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { test, expect, type Page } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// "Download as ZIP" cancel/abort regression coverage. The file-manager API is
// route-mocked (no daemon needed); the archive endpoint is redirected to a
// local Node server that streams bytes slowly, so the download can be
// cancelled or broken mid-stream deterministically.
//
// Two client paths exist and both are exercised: the StreamSaver/service-worker
// path (secure context, the default on 127.0.0.1) and the in-memory Blob path
// the app uses on plain http:// origins (forced via window.isSecureContext).

const FILES_TAB = /files|файлы|servers\.files/i;
const DOWNLOAD_DIR_TITLE = 'Download as ZIP';
const ARCHIVE_NAME = 'E2E_FM_Server.zip';

const CHUNK_SIZE = 64 * 1024;
const COMPLETE_CHUNKS = 8;
const CHUNK_INTERVAL_MS = 50;
const SLOW_MAX_CHUNKS = 1200;

const TS = 1752800000;

type StubMode = 'complete' | 'slow' | 'abort';

interface StubRequest {
  mode: StubMode;
  sent: number;
  ended: boolean;
  closedEarly: boolean;
}

interface ArchiveStub {
  url: string;
  requests: StubRequest[];
  request: (mode: StubMode) => StubRequest | undefined;
  close: () => Promise<void>;
}

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
    extension: dot > 0 ? path.slice(dot + 1) : undefined,
    filename: dot > 0 ? path.slice(0, dot) : path,
    mode: 420,
  };
}

async function startArchiveStub(): Promise<ArchiveStub> {
  const requests: StubRequest[] = [];
  const server = http.createServer((req, res) => {
    const url = new URL(req.url ?? '/', 'http://127.0.0.1');
    const mode = (url.searchParams.get('mode') ?? 'complete') as StubMode;
    const state: StubRequest = { mode, sent: 0, ended: false, closedEarly: false };
    requests.push(state);

    const totalChunks = mode === 'complete' ? COMPLETE_CHUNKS : SLOW_MAX_CHUNKS;
    res.writeHead(200, {
      'Content-Type': 'application/zip',
      'Content-Disposition': `attachment; filename="${ARCHIVE_NAME}"`,
      'X-Archive-Total-Bytes': String(totalChunks * CHUNK_SIZE),
      'X-Archive-Total-Files': '3',
      'X-Archive-Skipped-Count': '0',
      'Cache-Control': 'no-store',
    });
    res.flushHeaders();

    const chunk = Buffer.alloc(CHUNK_SIZE, 0x5a);
    const timer = setInterval(() => {
      if (res.destroyed) {
        clearInterval(timer);

        return;
      }
      if (mode === 'abort' && state.sent >= 3) {
        clearInterval(timer);
        res.destroy();

        return;
      }
      if (state.sent >= totalChunks) {
        clearInterval(timer);
        state.ended = true;
        res.end();

        return;
      }
      res.write(chunk);
      state.sent += 1;
    }, CHUNK_INTERVAL_MS);

    res.on('close', () => {
      clearInterval(timer);
      if (!state.ended) {
        state.closedEarly = true;
      }
    });
  });

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address() as AddressInfo;

  return {
    url: `http://127.0.0.1:${port}`,
    requests,
    request: (mode) => requests.find((r) => r.mode === mode),
    close: () =>
      new Promise<void>((resolve, reject) => {
        server.closeAllConnections();
        server.close((err) => (err ? reject(err) : resolve()));
      }),
  };
}

async function openFileManager(
  page: Page,
  token: string,
  opts: { secure: boolean; stub: ArchiveStub; mode: StubMode },
): Promise<void> {
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);
  if (!opts.secure) {
    // Same code path the app takes on a plain http://IP origin (Blob save).
    await page.addInitScript(() => {
      Object.defineProperty(window, 'isSecureContext', {
        value: false,
        configurable: true,
      });
    });
  }

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
        files: [fileEntry('config.txt'), fileEntry('server.cfg')],
      },
    }),
  );
  // Redirect the archive request to the streaming stub; the browser still
  // sees the app origin, so no CORS is involved.
  await page.route('**/api/file-manager/1/download-archive*', (route) =>
    route.continue({ url: `${opts.stub.url}/archive?mode=${opts.mode}` }),
  );

  await page.goto('/servers/1');
  await page.locator('.n-tabs-tab', { hasText: FILES_TAB }).click({ timeout: 20_000 });
  await expect(page.locator('.fm-row--file', { hasText: 'config.txt' })).toBeVisible({
    timeout: 20_000,
  });
}

function progressBlock(page: Page) {
  return page.locator('.fm-progress-block');
}

async function startArchiveDownload(page: Page): Promise<void> {
  await page.locator(`button.fm-tool-btn[title="${DOWNLOAD_DIR_TITLE}"]`).click();
  await expect(progressBlock(page)).toContainText('Downloading archive', { timeout: 20_000 });
}

test.describe('file manager: download directory as ZIP', () => {
  let stub: ArchiveStub;

  test.beforeAll(async () => {
    stub = await startArchiveStub();
  });

  test.afterAll(async () => {
    await stub.close();
  });

  test.beforeEach(() => {
    stub.requests.length = 0;
  });

  for (const secure of [true, false]) {
    const ctxLabel = secure ? 'secure context (StreamSaver)' : 'insecure context (Blob)';

    test(`${ctxLabel}: completed stream is saved in full`, async ({ page, request }) => {
      test.setTimeout(90_000);
      const token = await loginViaAPI(request);
      await openFileManager(page, token, { secure, stub, mode: 'complete' });

      const downloadPromise = page.waitForEvent('download', { timeout: 30_000 });
      await startArchiveDownload(page);

      const download = await downloadPromise;
      expect(download.suggestedFilename()).toBe(ARCHIVE_NAME);
      expect(await download.failure()).toBeNull();
      const savedPath = await download.path();
      expect(savedPath).not.toBeNull();
      expect(statSync(savedPath as string).size).toBe(COMPLETE_CHUNKS * CHUNK_SIZE);

      // Completed state shows the archive name and no Cancel button anymore.
      await expect(progressBlock(page)).toContainText(ARCHIVE_NAME, { timeout: 20_000 });
      await expect(progressBlock(page)).not.toContainText('Downloading archive');
      await expect(progressBlock(page).getByRole('button', { name: /cancel/i })).toHaveCount(0);
      expect(stub.request('complete')?.ended).toBe(true);
    });

    test(`${ctxLabel}: cancel mid-stream saves nothing`, async ({ page, request }) => {
      test.setTimeout(90_000);
      const token = await loginViaAPI(request);
      await openFileManager(page, token, { secure, stub, mode: 'slow' });

      const downloads: Array<{ failure: () => Promise<string | null> }> = [];
      page.on('download', (d) => downloads.push(d));

      await startArchiveDownload(page);
      await expect.poll(() => stub.request('slow')?.sent ?? 0, { timeout: 20_000 }).toBeGreaterThan(2);

      await progressBlock(page).getByRole('button', { name: /cancel/i }).click();

      // The bar disappears and must not flash back as "completed" or as an error.
      await expect(progressBlock(page)).toBeHidden({ timeout: 10_000 });
      await page.waitForTimeout(2_000);
      await expect(progressBlock(page)).toBeHidden();
      await expect(page.getByText('Archive download failed')).toHaveCount(0);

      // The server sees the disconnect (that is what cancels archive generation).
      await expect.poll(() => stub.request('slow')?.closedEarly, { timeout: 20_000 }).toBe(true);

      if (secure) {
        // StreamSaver had already handed the stream to the browser: the browser-side
        // download must end up cancelled/failed rather than completed.
        await expect.poll(() => downloads.length, { timeout: 20_000 }).toBeGreaterThan(0);
        for (const d of downloads) {
          expect(await d.failure()).not.toBeNull();
        }
      } else {
        // Blob path: no download may be triggered at all after a cancel.
        await page.waitForTimeout(3_000);
        expect(downloads).toHaveLength(0);
      }
    });

    test(`${ctxLabel}: server abort mid-stream is reported as an error, nothing complete is saved`, async ({
      page,
      request,
    }) => {
      test.setTimeout(90_000);
      const token = await loginViaAPI(request);
      await openFileManager(page, token, { secure, stub, mode: 'abort' });

      const downloads: Array<{ failure: () => Promise<string | null> }> = [];
      page.on('download', (d) => downloads.push(d));

      await startArchiveDownload(page);

      await expect(progressBlock(page)).toContainText('Archive download failed', { timeout: 30_000 });
      await expect(progressBlock(page)).not.toContainText('Downloading archive');

      if (secure) {
        await expect.poll(() => downloads.length, { timeout: 20_000 }).toBeGreaterThan(0);
        for (const d of downloads) {
          expect(await d.failure()).not.toBeNull();
        }
      } else {
        await page.waitForTimeout(2_000);
        expect(downloads).toHaveLength(0);
      }
    });
  }
});
