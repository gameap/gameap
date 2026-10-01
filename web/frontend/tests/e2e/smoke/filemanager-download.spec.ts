import { readFileSync } from 'node:fs';
import { test, expect, type Download, type Page, type Route } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// Browser-native file-manager downloads. The file manager exchanges the session
// for a single-use short-lived token and points a hidden frame at the download
// URL: an attachment response becomes a browser download, an error response is
// rendered into the frame and read back into the progress bar. The file-manager
// API and the token endpoint are route-mocked, so no daemon is needed.

const FILES_TAB = /files|файлы|servers\.files/i;
const DOWNLOAD_DIR_TITLE = 'Download as ZIP';
const TS = 1752800000;
const FILE_BODY = 'hostname "E2E"\n';
const ZIP_BODY = 'PK\u0005\u0006' + '\u0000'.repeat(18);

interface Reply {
  status: number;
  headers?: Record<string, string>;
  body: string;
}

type Replier = (url: URL) => Reply;

interface DownloadRequest {
  url: URL;
  authorization: string | undefined;
}

interface Stand {
  minted: string[];
  fileRequests: DownloadRequest[];
  archiveRequests: DownloadRequest[];
}

const attachment = (name: string, body: string, type = 'application/octet-stream'): Reply => ({
  status: 200,
  headers: {
    'Content-Type': type,
    'Content-Disposition': `attachment; filename="${name}"`,
    'Content-Length': String(Buffer.byteLength(body)),
  },
  body,
});

const apiError = (status: number, message: string): Reply => ({
  status,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ status: 'error', error: message, message, http_code: status }),
});

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

function dirEntry(path: string) {
  return { path, timestamp: TS, type: 'dir', dirname: '', basename: path, mode: 493 };
}

async function fulfill(route: Route, reply: Reply) {
  await route.fulfill({ status: reply.status, headers: reply.headers, body: reply.body });
}

async function openFileManager(
  page: Page,
  token: string,
  opts: { file?: Replier; archive?: Replier; archiveGate?: Promise<void>; mintError?: Reply } = {},
): Promise<Stand> {
  const stand: Stand = { minted: [], fileRequests: [], archiveRequests: [] };

  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

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
    route.fulfill({ json: { 'game-server-common': true, 'game-server-files': true } }),
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
        directories: [dirEntry('cfg')],
        files: [fileEntry('server.cfg'), fileEntry('motd.txt')],
      },
    }),
  );

  await page.route('**/api/auth/short-lived-token', async (route) => {
    if (opts.mintError) {
      await fulfill(route, opts.mintError);

      return;
    }
    const minted = `glst_e2e${stand.minted.length + 1}`;
    stand.minted.push(minted);
    await route.fulfill({ json: { token: minted, expires_in: 10 } });
  });

  // download* would also match download-archive, hence the anchored regexes.
  await page.route(/\/api\/file-manager\/1\/download\?/, async (route) => {
    const url = new URL(route.request().url());
    stand.fileRequests.push({ url, authorization: route.request().headers().authorization });
    await fulfill(route, (opts.file ?? ((u) => attachment(u.searchParams.get('path') ?? 'file', FILE_BODY)))(url));
  });
  await page.route(/\/api\/file-manager\/1\/download-archive\?/, async (route) => {
    const url = new URL(route.request().url());
    stand.archiveRequests.push({ url, authorization: route.request().headers().authorization });
    await opts.archiveGate;
    await fulfill(
      route,
      (opts.archive ?? ((u) => attachment(u.searchParams.get('filename') ?? 'archive.zip', ZIP_BODY, 'application/zip')))(url),
    );
  });

  await page.goto('/servers/1');
  await page.locator('.n-tabs-tab', { hasText: FILES_TAB }).click({ timeout: 20_000 });
  await expect(page.locator('.fm-row--file', { hasText: 'server.cfg' })).toBeVisible({ timeout: 20_000 });

  return stand;
}

const progressBlock = (page: Page) => page.locator('.fm-progress-block');
const downloadFrames = (page: Page) => page.locator('iframe[aria-hidden="true"]');

async function chooseFromMenu(page: Page, row: ReturnType<Page['locator']>, item: string) {
  await row.click({ button: 'right' });
  const menuItem = page.locator('.fm-context-menu li', { hasText: new RegExp(`^\\s*${item}\\s*$`) });
  await expect(menuItem).toBeVisible();
  await menuItem.click();
}

const downloadFileFromMenu = (page: Page, name: string) =>
  chooseFromMenu(page, page.locator('.fm-row--file', { hasText: name }), 'Download');

const downloadDirFromMenu = (page: Page, name: string) =>
  chooseFromMenu(page, page.locator('.fm-row--directory', { hasText: name }), DOWNLOAD_DIR_TITLE);

async function savedBody(download: Download): Promise<string> {
  expect(await download.failure()).toBeNull();
  const saved = await download.path();
  expect(saved).not.toBeNull();

  return readFileSync(saved as string, 'utf8');
}

test.describe('file manager: browser-native downloads', () => {
  test('file_is_saved_by_the_browser_with_a_short_lived_token', async ({ page, request }) => {
    const stand = await openFileManager(page, await loginViaAPI(request));

    const downloadPromise = page.waitForEvent('download');
    await downloadFileFromMenu(page, 'server.cfg');
    const download = await downloadPromise;

    expect(download.suggestedFilename()).toBe('server.cfg');
    expect(await savedBody(download)).toBe(FILE_BODY);

    expect(stand.minted).toEqual(['glst_e2e1']);
    expect(stand.fileRequests).toHaveLength(1);
    const { url, authorization } = stand.fileRequests[0];
    expect(url.searchParams.get('disk')).toBe('server');
    expect(url.searchParams.get('path')).toBe('server.cfg');
    expect(url.searchParams.get('token')).toBe('glst_e2e1');
    expect(authorization, 'the long-lived session must not travel with the download').toBeUndefined();

    await expect(progressBlock(page)).toContainText('Download started: server.cfg');
    await expect(progressBlock(page)).toBeHidden({ timeout: 10_000 });
  });

  test('every_download_mints_its_own_token', async ({ page, request }) => {
    const stand = await openFileManager(page, await loginViaAPI(request));

    const first = page.waitForEvent('download');
    await downloadFileFromMenu(page, 'server.cfg');
    await first;
    const second = page.waitForEvent('download');
    await downloadFileFromMenu(page, 'motd.txt');
    expect((await second).suggestedFilename()).toBe('motd.txt');

    expect(stand.minted).toEqual(['glst_e2e1', 'glst_e2e2']);
    expect(stand.fileRequests.map((r) => r.url.searchParams.get('token'))).toEqual(['glst_e2e1', 'glst_e2e2']);
  });

  test('directory_archive_is_saved_under_the_requested_name', async ({ page, request }) => {
    const stand = await openFileManager(page, await loginViaAPI(request));

    const downloadPromise = page.waitForEvent('download');
    await downloadDirFromMenu(page, 'cfg');
    const download = await downloadPromise;

    expect(download.suggestedFilename()).toBe('cfg.zip');
    expect(await savedBody(download)).toBe(ZIP_BODY);

    expect(stand.archiveRequests).toHaveLength(1);
    const { url, authorization } = stand.archiveRequests[0];
    expect(url.searchParams.get('path')).toBe('cfg');
    expect(url.searchParams.get('filename')).toBe('cfg.zip');
    expect(url.searchParams.get('token')).toBe('glst_e2e1');
    expect(url.searchParams.has('compress')).toBe(false);
    expect(authorization).toBeUndefined();

    await expect(progressBlock(page)).toContainText('Archive download started: cfg.zip');
  });

  test('server_root_archive_is_named_after_the_game_server', async ({ page, request }) => {
    const stand = await openFileManager(page, await loginViaAPI(request));

    const downloadPromise = page.waitForEvent('download');
    await page.locator(`button.fm-tool-btn[title="${DOWNLOAD_DIR_TITLE}"]`).click();
    const download = await downloadPromise;

    expect(download.suggestedFilename()).toBe('E2E_FM_Server.zip');
    const { url } = stand.archiveRequests[0];
    expect(url.searchParams.get('path')).toBe('/');
    expect(url.searchParams.get('filename')).toBe('E2E_FM_Server.zip');
  });

  // An archive answers only once its manifest is built, which has no deadline. Giving up on
  // reading back an error must not remove the frame and cancel the request still waiting.
  test('archive_slower_than_the_error_watch_is_still_saved', async ({ page, request }) => {
    let release: () => void = () => {};
    const archiveGate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.clock.install();
    const stand = await openFileManager(page, await loginViaAPI(request), { archiveGate });

    const downloadPromise = page.waitForEvent('download');
    await downloadDirFromMenu(page, 'cfg');
    await expect.poll(() => stand.archiveRequests.length).toBe(1);

    await page.clock.fastForward('11:00');
    release();

    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe('cfg.zip');
    expect(await savedBody(download)).toBe(ZIP_BODY);
  });

  test('json_error_of_a_file_download_is_shown_in_the_progress_bar', async ({ page, request }) => {
    await openFileManager(page, await loginViaAPI(request), {
      file: () => apiError(404, 'failed to get file info: file not found'),
    });

    const downloads: Download[] = [];
    page.on('download', (d) => downloads.push(d));
    await downloadFileFromMenu(page, 'server.cfg');

    await expect(progressBlock(page)).toContainText('Download failed: failed to get file info: file not found');
    await expect(downloadFrames(page), 'the frame that rendered the error is retired').toHaveCount(0);
    expect(downloads).toHaveLength(0);
  });

  test('json_error_of_an_archive_download_is_shown_in_the_progress_bar', async ({ page, request }) => {
    await openFileManager(page, await loginViaAPI(request), {
      archive: () => apiError(429, 'too many concurrent archive downloads for this server'),
    });

    const downloads: Download[] = [];
    page.on('download', (d) => downloads.push(d));
    await downloadDirFromMenu(page, 'cfg');

    await expect(progressBlock(page)).toContainText(
      'Archive download failed: too many concurrent archive downloads for this server',
    );
    expect(downloads).toHaveLength(0);
  });

  test('proxy_error_page_is_reported_by_its_title', async ({ page, request }) => {
    await openFileManager(page, await loginViaAPI(request), {
      file: () => ({
        status: 502,
        headers: { 'Content-Type': 'text/html' },
        body:
          '<html><head><title>502 Bad Gateway</title></head><body><center><h1>502 Bad Gateway</h1></center>' +
          '<hr><center>nginx</center></body></html>',
      }),
    });

    await downloadFileFromMenu(page, 'server.cfg');

    await expect(progressBlock(page)).toContainText('Download failed: 502 Bad Gateway');
  });

  test('token_request_failure_is_shown_and_nothing_is_requested', async ({ page, request }) => {
    const stand = await openFileManager(page, await loginViaAPI(request), {
      mintError: apiError(503, 'token store unavailable'),
    });

    await downloadFileFromMenu(page, 'server.cfg');

    await expect(progressBlock(page)).toContainText('Download failed: token store unavailable');
    expect(stand.fileRequests).toHaveLength(0);
    await expect(downloadFrames(page)).toHaveCount(0);
  });
});
