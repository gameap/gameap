import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// JetBrains-style find in the current directory: the file-manager API is fully
// route-mocked (no daemon required) — the spec drives the toolbar search form,
// match highlighting, wrap-around navigation and the keyboard activation paths.

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

// Fifty filler rows force real scrolling, so the wrap-around assertions prove
// that navigation actually scrolls the match into view. Matches for "config":
// the config_backup dir, config.txt at the top and zz_config_last.txt at the
// very bottom.
const LISTINGS: Record<string, { directories: object[]; files: object[] }> = {
  '': {
    directories: [dirEntry('alpha'), dirEntry('config_backup')],
    files: [
      fileEntry('config.txt'),
      ...Array.from({ length: 50 }, (_, i) =>
        fileEntry(`filler_${String(i).padStart(2, '0')}.log`),
      ),
      fileEntry('zz_config_last.txt'),
    ],
  },
  alpha: {
    directories: [],
    files: [fileEntry('alpha/alpha_config.cfg'), fileEntry('alpha/notes.md')],
  },
};

const searchButton = (page: Page) =>
  page.locator('button.fm-tool-btn[title="Search (Ctrl+F)"]');
const nextButton = (page: Page) =>
  page.locator('button.fm-tool-btn[title="Next match (Enter)"]');
const prevButton = (page: Page) =>
  page.locator('button.fm-tool-btn[title="Previous match (Shift+Enter)"]');
const closeButton = (page: Page) =>
  page.locator('button.fm-tool-btn[title="Close search (Esc)"]');
const searchInput = (page: Page) => page.locator('input.fm-search-input');
const marks = (page: Page) => page.locator('mark.fm-name-match');
const currentRow = (page: Page) => page.locator('.fm-row--search-current');

async function openFileManager(page: Page, request: APIRequestContext) {
  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

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
  await page.route('**/api/file-manager/1/content*', (route) => {
    const url = new URL(route.request().url());
    const path = url.searchParams.get('path') ?? '';
    const listing = LISTINGS[path] ?? { directories: [], files: [] };
    void route.fulfill({
      json: { result: { status: 'success', message: null }, ...listing },
    });
  });

  await page.goto('/servers/1');
  await page
    .locator('.n-tabs-tab', { hasText: FILES_TAB })
    .click({ timeout: 20_000 });
  await expect(
    page.locator('.fm-row--file', { hasText: 'config.txt' }).first(),
  ).toBeVisible({ timeout: 20_000 });
}

test('toolbar search: highlight, wrap-around navigation, close clears', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(page, request);

  await expect(searchButton(page)).not.toHaveClass(/fm-tool-btn--toggled/);
  await searchButton(page).click();
  await expect(searchInput(page)).toBeVisible();
  await expect(searchInput(page)).toBeFocused();
  await expect(searchButton(page)).toHaveClass(/fm-tool-btn--toggled/);

  await searchInput(page).fill('config');
  await expect(marks(page)).toHaveCount(3);

  // First match is the config_backup directory (directories go first).
  await expect(currentRow(page)).toHaveCount(1);
  await expect(currentRow(page)).toContainText('config_backup');
  await expect(currentRow(page)).toBeInViewport();
  await expect(page.locator('.fm-row--selected')).toHaveCount(0);

  // ▼ walks dir → top file → bottom file, scrolling each match into view,
  // without stealing focus from the input.
  await nextButton(page).click();
  await expect(currentRow(page)).toContainText('config.txt');
  await expect(currentRow(page)).toBeInViewport();
  await expect(searchInput(page)).toBeFocused();

  await nextButton(page).click();
  await expect(currentRow(page)).toContainText('zz_config_last.txt');
  await expect(currentRow(page)).toBeInViewport();

  // Past the last match ▼ wraps to the first one and scrolls back up.
  await nextButton(page).click();
  await expect(currentRow(page)).toContainText('config_backup');
  await expect(currentRow(page)).toBeInViewport();

  // ▲ from the first match wraps to the last.
  await prevButton(page).click();
  await expect(currentRow(page)).toContainText('zz_config_last.txt');
  await expect(searchInput(page)).toBeFocused();

  // Enter / Shift+Enter mirror ▼ / ▲.
  await page.keyboard.press('Enter');
  await expect(currentRow(page)).toContainText('config_backup');
  await page.keyboard.press('Shift+Enter');
  await expect(currentRow(page)).toContainText('zz_config_last.txt');

  // Selection stayed untouched through all the navigation.
  await expect(page.locator('.fm-row--selected')).toHaveCount(0);

  await closeButton(page).click();
  await expect(searchInput(page)).toHaveCount(0);
  await expect(marks(page)).toHaveCount(0);
  await expect(currentRow(page)).toHaveCount(0);
  await expect(searchButton(page)).not.toHaveClass(/fm-tool-btn--toggled/);

  // Reopening starts from a clean query.
  await searchButton(page).click();
  await expect(searchInput(page)).toHaveValue('');
});

test('keyboard activation: Ctrl+F, Escape, 3-char type-ahead, zero matches', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(page, request);

  await page.keyboard.press('Control+f');
  await expect(searchInput(page)).toBeVisible();
  await expect(searchInput(page)).toBeFocused();

  // Escape closes the search and hands focus back to the table area.
  await page.keyboard.press('Escape');
  await expect(searchInput(page)).toHaveCount(0);
  await expect(page.locator('.fm-table-area')).toBeFocused();

  // Blind typing: the third buffered character opens the search prefilled.
  await page.keyboard.type('con', { delay: 80 });
  await expect(searchInput(page)).toBeVisible();
  await expect(searchInput(page)).toHaveValue('con');
  await expect(searchInput(page)).toBeFocused();
  expect(
    await page.evaluate(
      () => (document.activeElement as HTMLInputElement).selectionStart,
    ),
  ).toBe(3);
  await expect(marks(page)).toHaveCount(3);

  await page.keyboard.press('Escape');
  await expect(searchInput(page)).toHaveCount(0);

  // The type-ahead buffer expires after ~1s of inactivity: two characters,
  // a pause, then a third one must NOT open the search.
  await page.keyboard.type('co', { delay: 80 });
  await page.waitForTimeout(1200);
  await page.keyboard.press('n');
  await expect(searchInput(page)).toHaveCount(0);

  // Search open but focus elsewhere: a printable key refocuses the input
  // and appends instead of starting a new buffer.
  await searchButton(page).click();
  await searchInput(page).fill('conf');
  await page.locator('.fm-row--file', { hasText: 'filler_00.log' }).click();
  await expect(page.locator('.fm-row--selected')).toHaveCount(1);
  await expect(searchInput(page)).not.toBeFocused();
  await page.keyboard.press('i');
  await expect(searchInput(page)).toBeFocused();
  await expect(searchInput(page)).toHaveValue('confi');

  // Ctrl+F inside the input keeps the form open and focused instead of
  // falling through to the browser's native find.
  await page.keyboard.press('Control+f');
  await expect(searchInput(page)).toBeVisible();
  await expect(searchInput(page)).toBeFocused();
  await expect(searchInput(page)).toHaveValue('confi');

  // Escape closes the search first; the selection survives and only the
  // second Escape clears it.
  await page.keyboard.press('Escape');
  await expect(searchInput(page)).toHaveCount(0);
  await expect(page.locator('.fm-row--selected')).toHaveCount(1);
  await page.keyboard.press('Escape');
  await expect(page.locator('.fm-row--selected')).toHaveCount(0);

  // Zero matches: no marks, no current row, ▼ is a no-op.
  await page.keyboard.press('Control+f');
  await expect(searchInput(page)).toHaveValue('');
  await searchInput(page).fill('zzzz');
  await expect(marks(page)).toHaveCount(0);
  await expect(currentRow(page)).toHaveCount(0);
  await nextButton(page).click();
  await expect(currentRow(page)).toHaveCount(0);
  await expect(searchInput(page)).toHaveValue('zzzz');
});

test('directory change keeps the query and recomputes matches', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  await openFileManager(page, request);

  await searchButton(page).click();
  await searchInput(page).fill('config');
  await expect(currentRow(page)).toContainText('config_backup');

  await page.locator('.fm-row--directory', { hasText: 'alpha' }).dblclick();
  await expect(
    page.locator('.fm-row--file', { hasText: 'notes.md' }),
  ).toBeVisible({ timeout: 10_000 });

  // The form survived navigation and the matches were recomputed.
  await expect(searchInput(page)).toBeVisible();
  await expect(searchInput(page)).toHaveValue('config');
  await expect(marks(page)).toHaveCount(1);
  await expect(currentRow(page)).toHaveCount(1);
  await expect(currentRow(page)).toContainText('alpha_config.cfg');
  await expect(currentRow(page)).toBeInViewport();

  await page.locator('.fm-row--up').click();
  await expect(
    page.locator('.fm-row--file', { hasText: 'config.txt' }).first(),
  ).toBeVisible({ timeout: 10_000 });

  await expect(searchInput(page)).toHaveValue('config');
  await expect(currentRow(page)).toHaveCount(1);
  await expect(currentRow(page)).toContainText('config_backup');
  await expect(currentRow(page)).toBeInViewport();
});
