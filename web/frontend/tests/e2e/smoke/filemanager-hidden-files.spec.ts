import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// Dotfiles are filtered in the browser only — the daemon and the API return
// them as they are. The spec drives the breadcrumb-bar toggle, the choice persisted
// in localStorage, its precedence over the boolean an initialize config may
// carry, and that a shown dotfile opens in the text editor. The file-manager
// API is fully route-mocked (no daemon required).

const FILES_TAB = /files|файлы|servers\.files/i;
const SETTINGS_KEY = 'gameap:fm:settings';

const TS = 1752800000;

function dirEntry(path: string) {
  const basename = path.split('/').pop() ?? path;
  const dirname = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : '';

  return { path, timestamp: TS, type: 'dir', dirname, basename, mode: 493 };
}

// Mirrors parseFilename in the API: filepath.Ext('.env') is '.env', so a
// dotfile gets extension 'env' and an empty filename.
function fileEntry(path: string) {
  const basename = path.split('/').pop() ?? path;
  const dirname = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : '';
  const dot = basename.lastIndexOf('.');
  const hasExt = dot >= 0 && dot < basename.length - 1;

  return {
    path,
    timestamp: TS,
    type: 'file',
    visibility: 'public',
    size: 64,
    dirname,
    basename,
    extension: hasExt ? basename.slice(dot + 1) : undefined,
    filename: hasExt ? basename.slice(0, dot) : basename,
    mode: 420,
  };
}

const LISTINGS: Record<string, { directories: object[]; files: object[] }> = {
  '': {
    directories: [dirEntry('.steam'), dirEntry('cstrike')],
    files: [fileEntry('.env'), fileEntry('server.cfg')],
  },
  cstrike: {
    directories: [],
    files: [fileEntry('cstrike/motd.txt')],
  },
};

const toggleButton = (page: Page) =>
  page.locator('.fm-breadcrumb-nav button.fm-hidden-btn');
const dirRow = (page: Page, name: string) =>
  page.locator('.fm-row--directory', { hasText: name });
const fileRow = (page: Page, name: string) =>
  page.locator('.fm-row--file', { hasText: name });
const modal = (page: Page) => page.locator('.n-modal');

const readStored = (page: Page) =>
  page.evaluate((key) => localStorage.getItem(key), SETTINGS_KEY);

interface OpenOptions {
  // Boolean the mocked initialize config carries; omitted like the real backend.
  initializeHiddenFiles?: boolean;
  // A choice seeded into localStorage before the page loads.
  storedChoice?: boolean;
}

async function openFileManager(
  page: Page,
  request: APIRequestContext,
  options: OpenOptions = {},
) {
  const token = await loginViaAPI(request);
  await page.addInitScript(
    ({ t, key, stored }) => {
      localStorage.setItem('auth_token', t);
      if (stored !== undefined) {
        localStorage.setItem(key, JSON.stringify({ hiddenFiles: stored }));
      }
    },
    { t: token, key: SETTINGS_KEY, stored: options.storedChoice },
  );

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
          disks: { server: { driver: 'local' } },
          lang: 'en',
          leftDisk: 'server',
          leftPath: '',
          windowsConfig: 1,
          ...(options.initializeHiddenFiles === undefined
            ? {}
            : { hiddenFiles: options.initializeHiddenFiles }),
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
  // The text editor fetches the file body through the download endpoint; the
  // JSON catch-all would break it.
  await page.route('**/api/file-manager/1/download*', (route) =>
    route.fulfill({ body: 'KEY=value', contentType: 'text/plain' }),
  );

  await page.goto('/servers/1');
  await openFilesTab(page);
}

async function openFilesTab(page: Page) {
  await page
    .locator('.n-tabs-tab', { hasText: FILES_TAB })
    .click({ timeout: 20_000 });
  await expect(fileRow(page, 'server.cfg')).toBeVisible({ timeout: 20_000 });
}

test('breadcrumb toggle shows and hides dotfiles, the choice survives a reload', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(page, request);

  // Hidden by default, nothing recorded yet.
  await expect(fileRow(page, '.env')).toHaveCount(0);
  await expect(dirRow(page, '.steam')).toHaveCount(0);
  await expect(dirRow(page, 'cstrike')).toBeVisible();
  await expect(toggleButton(page)).not.toHaveClass(/fm-crumb-btn--on/);
  await expect(toggleButton(page)).toHaveAttribute('aria-pressed', 'false');
  await expect(toggleButton(page).locator('i.fa-eye-slash')).toHaveCount(1);
  expect(await readStored(page)).toBeNull();

  await toggleButton(page).click();
  await expect(fileRow(page, '.env')).toBeVisible();
  await expect(dirRow(page, '.steam')).toBeVisible();
  await expect(fileRow(page, '.env')).toHaveClass(/fm-row--hidden/);
  await expect(dirRow(page, '.steam')).toHaveClass(/fm-row--hidden/);
  await expect(fileRow(page, 'server.cfg')).not.toHaveClass(/fm-row--hidden/);
  await expect(dirRow(page, 'cstrike')).not.toHaveClass(/fm-row--hidden/);
  await expect(toggleButton(page)).toHaveClass(/fm-crumb-btn--on/);
  await expect(toggleButton(page)).toHaveAttribute('aria-pressed', 'true');
  await expect(toggleButton(page).locator('i.fa-eye')).toHaveCount(1);
  expect(JSON.parse((await readStored(page)) ?? 'null')).toEqual({ hiddenFiles: true });

  // The persisted choice is applied on a fresh page load.
  await page.reload();
  await openFilesTab(page);
  await expect(fileRow(page, '.env')).toBeVisible();
  await expect(dirRow(page, '.steam')).toBeVisible();
  await expect(toggleButton(page)).toHaveClass(/fm-crumb-btn--on/);

  await toggleButton(page).click();
  await expect(fileRow(page, '.env')).toHaveCount(0);
  await expect(dirRow(page, '.steam')).toHaveCount(0);
  await expect(toggleButton(page)).not.toHaveClass(/fm-crumb-btn--on/);
  expect(JSON.parse((await readStored(page)) ?? 'null')).toEqual({ hiddenFiles: false });
});

test('a boolean in the initialize config is the default until the user chooses', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(page, request, { initializeHiddenFiles: true });

  await expect(fileRow(page, '.env')).toBeVisible();
  await expect(dirRow(page, '.steam')).toBeVisible();
  await expect(toggleButton(page)).toHaveClass(/fm-crumb-btn--on/);
  // Applying the server default records no choice.
  expect(await readStored(page)).toBeNull();
});

test('a stored choice outranks the initialize config', async ({ page, request }) => {
  test.setTimeout(120_000);
  await openFileManager(page, request, {
    initializeHiddenFiles: true,
    storedChoice: false,
  });

  await expect(fileRow(page, '.env')).toHaveCount(0);
  await expect(dirRow(page, '.steam')).toHaveCount(0);
  await expect(toggleButton(page)).not.toHaveClass(/fm-crumb-btn--on/);
});

test('a shown dotfile opens in the text editor', async ({ page, request }) => {
  test.setTimeout(120_000);
  await openFileManager(page, request, { storedChoice: true });

  await expect(fileRow(page, '.env')).toBeVisible();
  await fileRow(page, '.env').dblclick();

  await expect(modal(page)).toBeVisible();
  await expect(modal(page)).toContainText('.env');
  await expect(modal(page).locator('textarea')).toHaveValue('KEY=value');
  await page.keyboard.press('Escape');
  await expect(modal(page)).toHaveCount(0);
});
