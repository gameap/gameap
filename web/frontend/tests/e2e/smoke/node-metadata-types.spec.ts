import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// Node metadata is edited as a list of text rows, so every stored value is
// rendered as a string. Sending those strings back would rewrite numbers,
// booleans, null and nested documents as text the operator never typed. This
// spec pins that only the edited row changes.
//
// The node API is route-mocked: no node has to exist, and the assertion is on
// the PUT body rather than on what a backend happened to persist.

const NODE_ID = 4242;
const METADATA_TAB = /metadata|метаданные|dedicated_servers\.metadata/i;
const SAVE = /save|сохранить|main\.save/i;

const STORED_METADATA = {
  port: 8080,
  tls: true,
  note: null,
  tags: { region: 'eu', racks: [1, 2] },
  tag: 'eu',
};

const NODE = {
  id: NODE_ID,
  enabled: true,
  name: 'E2E Metadata Node',
  os: 'linux',
  location: 'US',
  provider: 'Hetzner',
  ip: ['10.0.0.1'],
  work_path: '/srv/gameap',
  steamcmd_path: '/srv/gameap/steamcmd',
  gdaemon_host: '10.0.0.1',
  gdaemon_port: 31717,
  gdaemon_server_cert: 'certs/node.crt',
  client_certificate_id: 1,
  prefer_install_method: 'auto',
  metadata: STORED_METADATA,
};

test('editing one metadata row leaves the other values at their original types', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

  await page.route('**/api/client_certificates', (route) =>
    route.fulfill({
      json: [{ id: 1, fingerprint: 'AA:BB:CC', expires: '2030-01-01T00:00:00Z' }],
    }),
  );

  let putBody: Record<string, unknown> | undefined;
  await page.route(`**/api/nodes/${NODE_ID}`, async (route) => {
    if (route.request().method() === 'PUT') {
      putBody = route.request().postDataJSON() as Record<string, unknown>;

      await route.fulfill({ json: NODE });

      return;
    }

    await route.fulfill({ json: NODE });
  });

  await page.goto(`/admin/nodes/${NODE_ID}/edit`);

  await page.locator('.n-tabs-tab', { hasText: METADATA_TAB }).first().click();

  const meta = page.getByTestId('node-metadata');
  const rows = meta.locator('tbody tr');
  await expect(rows).toHaveCount(Object.keys(STORED_METADATA).length);

  // Every value is rendered as text, including the ones that are not strings.
  const rowFor = async (key: string) => {
    const count = await rows.count();
    for (let i = 0; i < count; i++) {
      const value = await rows.nth(i).locator('input').nth(0).inputValue();
      if (value === key) {
        return rows.nth(i);
      }
    }

    throw new Error(`no metadata row for ${key}`);
  };

  await expect((await rowFor('port')).locator('input').nth(1)).toHaveValue('8080');
  await expect((await rowFor('tls')).locator('input').nth(1)).toHaveValue('true');
  await expect((await rowFor('note')).locator('input').nth(1)).toHaveValue('null');

  // Touch exactly one row.
  await (await rowFor('tag')).locator('input').nth(1).fill('us');

  const updateResp = page.waitForResponse(
    (r) => r.url().includes(`/api/nodes/${NODE_ID}`) && r.request().method() === 'PUT',
  );
  await page.getByRole('button', { name: SAVE }).first().click();
  await updateResp;

  expect(putBody).toBeDefined();
  expect(putBody?.metadata).toEqual({
    ...STORED_METADATA,
    tag: 'us',
  });
});
