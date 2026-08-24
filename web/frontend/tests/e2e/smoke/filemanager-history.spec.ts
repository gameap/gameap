import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// Visit/open history in the file manager: a directory counts only after a
// 5-second stay or a file opened in it (pass-through directories are never
// recorded), files count on every open, everything lives in localStorage per
// server+disk. The API is fully route-mocked; page.clock drives the dwell
// timer and the relative-time labels.

const FILES_TAB = /files|файлы|servers\.files/i;

const TS = 1752800000;
const T0 = new Date('2026-01-10T12:00:00');
const T0MS = T0.getTime();
const MINUTE = 60_000;
const DAY = 24 * 60 * 60 * 1000;
const STORAGE_KEY = 'gameap:fm:history:1:server';

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

const LISTINGS: Record<string, { directories: object[]; files: object[] }> = {
  '': { directories: [dirEntry('cstrike')], files: [] },
  cstrike: {
    directories: [dirEntry('cstrike/addons')],
    files: [fileEntry('cstrike/readme.txt')],
  },
  'cstrike/addons': {
    directories: [dirEntry('cstrike/addons/amxmodx')],
    files: [],
  },
  'cstrike/addons/amxmodx': {
    directories: [dirEntry('cstrike/addons/amxmodx/configs')],
    files: [],
  },
  'cstrike/addons/amxmodx/configs': {
    directories: [],
    files: [
      fileEntry('cstrike/addons/amxmodx/configs/amxx.cfg'),
      fileEntry('cstrike/addons/amxmodx/configs/plugins.ini'),
    ],
  },
};

const CONFIGS = 'cstrike/addons/amxmodx/configs';

function historyEntry(at: number) {
  return { count: 1, lastAt: at, score: 1, scoreAt: at };
}

function seededState(
  entries: { dir?: Record<string, object>; file?: Record<string, object> },
  view: 'recent' | 'frequent' = 'recent',
) {
  return {
    version: 1,
    view,
    entries: { dir: entries.dir ?? {}, file: entries.file ?? {} },
  };
}

const historyButton = (page: Page) => page.locator('button.fm-history-btn');
const popover = (page: Page) => page.locator('.fm-history-pop');
const popItem = (page: Page, path: string) =>
  popover(page).locator(`.fm-history-item[title="${path}"]`);
const popTabs = (page: Page) => popover(page).locator('.fm-history-tab');
const modal = (page: Page) => page.locator('.n-modal');
const dirRow = (page: Page, name: string) =>
  page.locator('.fm-row--directory', { hasText: name });
const fileRow = (page: Page, name: string) =>
  page.locator('.fm-row--file', { hasText: name });

const readStorage = (page: Page) =>
  page.evaluate((key) => JSON.parse(localStorage.getItem(key) ?? 'null'), STORAGE_KEY);

async function openFileManager(
  page: Page,
  request: APIRequestContext,
  seed?: object,
) {
  await page.clock.install({ time: T0 });

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);
  if (seed) {
    // Init scripts re-run on every navigation — seed only the first load so
    // a reload keeps what the page has written since.
    await page.addInitScript(
      ([key, value]) => {
        if (!localStorage.getItem(key)) localStorage.setItem(key, value);
      },
      [STORAGE_KEY, JSON.stringify(seed)] as const,
    );
  }

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
  // A directory outside LISTINGS is treated as gone — the backend answers a
  // failed ReadDir with an HTTP error, and the stale-entry flow needs one.
  await page.route('**/api/file-manager/1/content*', (route) => {
    const url = new URL(route.request().url());
    const path = url.searchParams.get('path') ?? '';
    const listing = LISTINGS[path];
    if (!listing) {
      void route.fulfill({ status: 404, json: { message: 'Directory not found' } });
      return;
    }
    void route.fulfill({
      json: { result: { status: 'success', message: null }, ...listing },
    });
  });
  // The text editor fetches the file body through the download endpoint; the
  // JSON catch-all would break it.
  await page.route('**/api/file-manager/1/download*', (route) =>
    route.fulfill({ body: 'hostname "e2e"', contentType: 'text/plain' }),
  );

  await page.goto('/servers/1');
  await page
    .locator('.n-tabs-tab', { hasText: FILES_TAB })
    .click({ timeout: 20_000 });
  await expect(dirRow(page, 'cstrike')).toBeVisible({ timeout: 20_000 });
}

test('transit directories are skipped; dwell and file-open credit visits', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(page, request);

  // Empty history — no button at all.
  await expect(historyButton(page)).toHaveCount(0);

  // Freeze the virtual clock so the walk provably takes zero dwell time.
  await page.clock.pauseAt(new Date(T0MS + MINUTE));

  await dirRow(page, 'cstrike').dblclick();
  await expect(dirRow(page, 'addons')).toBeVisible();
  await dirRow(page, 'addons').dblclick();
  await expect(dirRow(page, 'amxmodx')).toBeVisible();
  await dirRow(page, 'amxmodx').dblclick();
  await expect(dirRow(page, 'configs')).toBeVisible();
  await dirRow(page, 'configs').dblclick();
  await expect(fileRow(page, 'amxx.cfg')).toBeVisible();

  // The whole walk happened at one instant — nothing recorded yet.
  await expect(historyButton(page)).toHaveCount(0);

  // 5 virtual seconds in the final directory credit it — and only it.
  await page.clock.fastForward(5000);
  await expect(historyButton(page)).toHaveCount(1);
  let stored = await readStorage(page);
  expect(Object.keys(stored.entries.dir)).toEqual([CONFIGS]);
  expect(Object.keys(stored.entries.file)).toEqual([]);

  // Opening a file credits its directory instantly: the clock is still
  // paused, so no dwell time can have passed.
  await page
    .locator('.fm-breadcrumb-text', { hasText: 'cstrike' })
    .first()
    .click();
  await expect(fileRow(page, 'readme.txt')).toBeVisible();
  await fileRow(page, 'readme.txt').dblclick();

  stored = await readStorage(page);
  expect(Object.keys(stored.entries.dir).sort()).toEqual(['cstrike', CONFIGS]);
  expect(Object.keys(stored.entries.file)).toEqual(['cstrike/readme.txt']);

  await page.clock.resume();
  await expect(modal(page)).toBeVisible();
  await expect(modal(page)).toContainText('Editor');
  await page.keyboard.press('Escape');
  await expect(modal(page)).toHaveCount(0);

  // The popover lists the credited items, not the transit directories.
  await historyButton(page).click();
  await expect(popover(page)).toBeVisible();
  await expect(popItem(page, 'cstrike')).toBeVisible();
  await expect(popItem(page, CONFIGS)).toBeVisible();
  await expect(popItem(page, 'cstrike/readme.txt')).toBeVisible();
  await expect(popItem(page, 'cstrike/addons')).toHaveCount(0);
  await expect(popItem(page, 'cstrike/addons/amxmodx')).toHaveCount(0);
  await expect(popItem(page, 'cstrike/readme.txt')).toContainText('just now');

  // Escape closes the popover (and must not clear the table selection).
  await page.keyboard.press('Escape');
  await expect(popover(page)).toHaveCount(0);

  // Relative time labels follow the clock.
  await page.clock.fastForward(3 * MINUTE);
  await historyButton(page).click();
  await expect(popItem(page, 'cstrike/readme.txt')).toContainText('3 min ago');
});

test('popover opens entries, drops stale ones, remembers the view', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(
    page,
    request,
    seededState({
      dir: {
        [CONFIGS]: historyEntry(T0MS - 60 * MINUTE),
        'cstrike/oldmaps': historyEntry(T0MS - 120 * MINUTE),
      },
      file: {
        'cstrike/readme.txt': historyEntry(T0MS - 30 * MINUTE),
        'cstrike/gone.cfg': historyEntry(T0MS - 180 * MINUTE),
      },
    }),
  );

  // A directory entry navigates straight to it.
  await historyButton(page).click();
  await popItem(page, CONFIGS).click();
  await expect(popover(page)).toHaveCount(0);
  await expect(fileRow(page, 'amxx.cfg')).toBeVisible();

  // A file entry navigates to its parent, selects it and opens the editor.
  await historyButton(page).click();
  await popItem(page, 'cstrike/readme.txt').click();
  await expect(fileRow(page, 'readme.txt')).toBeVisible();
  await expect(page.locator('.fm-row--selected')).toContainText('readme.txt');
  await expect(modal(page)).toContainText('Editor');
  await page.keyboard.press('Escape');
  await expect(modal(page)).toHaveCount(0);

  // A directory that no longer exists is dropped from the history; the
  // failed load itself pops the interceptor's error dialog — dismiss it.
  await historyButton(page).click();
  await popItem(page, 'cstrike/oldmaps').click();
  await expect(popover(page)).toHaveCount(0);
  await page.locator('.n-dialog button').click();
  await expect(page.locator('.n-dialog')).toHaveCount(0);
  let stored = await readStorage(page);
  expect(Object.keys(stored.entries.dir)).not.toContain('cstrike/oldmaps');

  // A file missing from its (freshly loaded) parent listing is dropped too,
  // with an info dialog explaining what happened.
  await historyButton(page).click();
  await popItem(page, 'cstrike/gone.cfg').click();
  await expect(page.locator('.n-dialog')).toContainText('no longer exists');
  await page.locator('.n-dialog button').click();
  await expect(page.locator('.n-dialog')).toHaveCount(0);
  stored = await readStorage(page);
  expect(Object.keys(stored.entries.file)).not.toContain('cstrike/gone.cfg');

  // The frequent view hides the time meta and survives a reload.
  await historyButton(page).click();
  await popTabs(page).filter({ hasText: 'Frequent' }).click();
  await expect(popover(page).locator('.fm-history-meta')).toHaveCount(0);
  stored = await readStorage(page);
  expect(stored.view).toBe('frequent');

  await page.reload();
  await page
    .locator('.n-tabs-tab', { hasText: FILES_TAB })
    .click({ timeout: 20_000 });
  await expect(dirRow(page, 'cstrike')).toBeVisible({ timeout: 20_000 });
  await historyButton(page).click();
  await expect(popTabs(page).filter({ hasText: 'Frequent' })).toHaveClass(
    /fm-history-tab--active/,
  );
});

test('entries older than the recent window fall back to a frequent-only popover', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(
    page,
    request,
    seededState({
      dir: {
        [CONFIGS]: historyEntry(T0MS - 40 * DAY),
        'cstrike/maps': historyEntry(T0MS - 41 * DAY),
        'cstrike/addons/metamod': historyEntry(T0MS - 42 * DAY),
        'cstrike/logs': historyEntry(T0MS - 43 * DAY),
        'cstrike/dlls': historyEntry(T0MS - 44 * DAY),
        'cstrike/sound': historyEntry(T0MS - 45 * DAY),
      },
      file: { 'cstrike/readme.txt': historyEntry(T0MS - 35 * DAY) },
    }),
  );

  // Frequent entries survive, so the button is there…
  await expect(historyButton(page)).toHaveCount(1);
  await historyButton(page).click();
  await expect(popover(page)).toBeVisible();

  // …but with nothing recent there is no view switcher and no time meta —
  // just the frequent lists.
  await expect(popTabs(page)).toHaveCount(0);
  await expect(popItem(page, CONFIGS)).toBeVisible();
  await expect(popItem(page, 'cstrike/readme.txt')).toBeVisible();
  await expect(popover(page).locator('.fm-history-meta')).toHaveCount(0);

  // Each section is capped at 4 entries: of the six directories only the
  // four with the highest decayed score stay (4 dirs + 1 file in total).
  await expect(popover(page).locator('.fm-history-item')).toHaveCount(5);
  await expect(popItem(page, 'cstrike/dlls')).toHaveCount(0);
  await expect(popItem(page, 'cstrike/sound')).toHaveCount(0);
});

test('deleting a file removes it from the history', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(
    page,
    request,
    seededState({
      dir: { [CONFIGS]: historyEntry(T0MS - 10 * MINUTE) },
      file: { 'cstrike/readme.txt': historyEntry(T0MS - 5 * MINUTE) },
    }),
  );

  await dirRow(page, 'cstrike').dblclick();
  await expect(fileRow(page, 'readme.txt')).toBeVisible();
  await fileRow(page, 'readme.txt').click();
  await page.keyboard.press('Delete');
  await expect(modal(page)).toContainText('Delete');
  await modal(page).locator('button', { hasText: 'Delete' }).click();
  await expect(modal(page)).toHaveCount(0);

  const stored = await readStorage(page);
  expect(Object.keys(stored.entries.file)).toEqual([]);
  expect(Object.keys(stored.entries.dir)).toContain(CONFIGS);
});
