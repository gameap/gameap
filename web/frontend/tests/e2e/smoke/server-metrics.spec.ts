import { test, expect, type Page } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';

// Server CPU comes from the daemon as a percentage of one core, the way
// `docker stats` reports it, so a multi-core server goes past 100%. The chart
// axis has to follow such values instead of clipping them at 100%, the strip
// bar is measured against the CPU limit, and the limit is shown the way
// Pterodactyl and Pelican show it: "250.0% / 400%", or "/ ∞" when none is set.
//
// The server API and the metrics WebSocket are mocked, so no daemon is needed.

const SERVER = {
  id: 1,
  uid: '11111111-1111-1111-1111-111111111111',
  uuid: '11111111-1111-1111-1111-111111111111',
  uuid_short: '11111111',
  enabled: true,
  installed: 1,
  blocked: false,
  name: 'E2E Metrics Server',
  game_id: 'cs',
  ds_id: 1,
  game_mod_id: 1,
  server_ip: '127.0.0.1',
  server_port: 27015,
  online: true,
  game: { code: 'cs', name: 'Counter-Strike' },
};

const SAMPLE_STEP_MS = 5_000;

interface WireSeries {
  name: string;
  type: string;
  unit: string;
  labels: Record<string, string>;
  points: Array<{ timestamp: string; value: string }>;
}

// One point per sample step, the last one now; counters are sent as running
// totals, the way the daemon reports them.
function series(name: string, type: 'gauge' | 'counter', values: number[]): WireSeries {
  const now = Date.now();

  return {
    name,
    type,
    unit: type === 'counter' ? 'bytes' : 'percent',
    labels: { container: 'e2e-metrics-server' },
    points: values.map((v, i) => ({
      timestamp: new Date(now - (values.length - 1 - i) * SAMPLE_STEP_MS).toISOString(),
      value: String(v),
    })),
  };
}

function cpuSeries(values: number[]): WireSeries {
  return series('gameap_server_cpu_usage_percent', 'gauge', values);
}

// A counter growing by bytesPerSecond, so the UI derives exactly that rate.
function counterSeries(name: string, bytesPerSecond: number, samples = 3): WireSeries {
  const step = (bytesPerSecond * SAMPLE_STEP_MS) / 1000;

  return series(name, 'counter', Array.from({ length: samples }, (_, i) => i * step));
}

// cpuLimit is in millicores; undefined leaves the key out, as the API does for
// a viewer who is not an administrator.
async function openServer(page: Page, cpuLimit: number | null | undefined, metrics: WireSeries[]) {
  const server: Record<string, unknown> = { ...SERVER };
  if (cpuLimit !== undefined) {
    server.cpu_limit = cpuLimit;
  }

  await page.route('**/api/servers/1/**', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/servers/1', (route) => route.fulfill({ json: server }));
  await page.route('**/api/servers/1/abilities', (route) =>
    route.fulfill({
      json: { 'game-server-common': true, 'game-server-metrics': true },
    }),
  );
  await page.route('**/api/auth/short-lived-token', (route) =>
    route.fulfill({ json: { token: 'slt-e2e' } }),
  );

  // The strip and the chart modal each open their own metrics socket; both
  // get the same history.
  await page.routeWebSocket('**/api/ws/servers/1/metrics*', (ws) => {
    ws.onMessage((message) => {
      try {
        if (JSON.parse(String(message)).type === 'ping') {
          ws.send(JSON.stringify({ type: 'pong' }));
        }
      } catch {
        /* non-JSON frames are ignored */
      }
    });
    ws.send(
      JSON.stringify({
        type: 'metrics.replay',
        payload: [{ timestamp: new Date().toISOString(), series: metrics }],
      }),
    );
    ws.send(JSON.stringify({ type: 'metrics.replay.done', payload: {} }));
  });

  await page.goto('/servers/1');
}

// The rendered range of the CPU chart's y axis. Vue links DOM nodes to their
// component only in dev builds, which is what Playwright's webServer (the Vite
// dev server) serves.
async function cpuAxisExtent(page: Page): Promise<number[] | null> {
  return page
    .getByTestId('server-stats-cpu-chart')
    .locator('x-vue-echarts')
    .evaluate((el) => {
      const chart = (el as any).__vueParentComponent?.exposed?.chart;
      if (!chart) {
        return null;
      }

      return chart.getModel().getComponent('yAxis', 0).axis.scale.getExtent();
    });
}

interface CpuCase {
  name: string;
  cpuLimit: number | null | undefined;
  values: number[];
  stripText: string;
  barMaxWidth: string | null;
  headerText: string;
  axis: [number, number];
}

const CPU_CASES: CpuCase[] = [
  {
    name: 'admin without a CPU limit sees usage past one core on an axis that follows it',
    cpuLimit: null,
    values: [130, 200, 250],
    stripText: '250.0% / ∞',
    barMaxWidth: null,
    headerText: '250.0% / ∞',
    axis: [0, 250],
  },
  {
    name: 'admin with a CPU limit sees the bar and the axis measured against the limit',
    cpuLimit: 4000,
    values: [130, 200, 250],
    stripText: '250.0% / 400%',
    barMaxWidth: '62.5%',
    headerText: '250.0% / 400%',
    axis: [0, 400],
  },
  {
    name: 'viewer who cannot see the limit gets the bare value',
    cpuLimit: undefined,
    values: [130, 200, 250],
    stripText: '250.0%',
    barMaxWidth: null,
    headerText: '250.0%',
    axis: [0, 250],
  },
  {
    name: 'idle server keeps the one-core scale',
    cpuLimit: null,
    values: [2, 3],
    stripText: '3.0% / ∞',
    barMaxWidth: null,
    headerText: '3.0% / ∞',
    axis: [0, 100],
  },
];

for (const c of CPU_CASES) {
  test(c.name, async ({ page, request }) => {
    test.setTimeout(60_000);

    const token = await loginViaAPI(request);
    await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);

    await openServer(page, c.cpuLimit, [cpuSeries(c.values)]);

    const strip = page.getByTestId('server-stats-cpu');
    await expect(strip).toContainText(c.stripText, { timeout: 20_000 });
    if (c.cpuLimit === undefined) {
      await expect(strip).not.toContainText('/');
    }

    const bar = strip.locator('.n-progress-graph-line-fill');
    if (c.barMaxWidth === null) {
      await expect(bar).toHaveCount(0);
    } else {
      await expect(bar).toHaveAttribute(
        'style',
        new RegExp(`max-width: ${c.barMaxWidth.replace('.', '\\.')}`),
      );
    }

    await strip.click();

    await expect(page.getByTestId('server-stats-cpu-current')).toHaveText(c.headerText, {
      timeout: 20_000,
    });
    await expect.poll(() => cpuAxisExtent(page), { timeout: 20_000 }).toEqual(c.axis);
  });
}

// At tablet width the page leaves the strip ~500px. Squeezed into one row, the
// cells used to split their labels letter by letter ("N/E/T") and collapse the
// CPU bar to nothing.
test('strip keeps labels and rates whole and the CPU bar visible at tablet width', async ({
  page,
  request,
}) => {
  test.setTimeout(60_000);

  const token = await loginViaAPI(request);
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);
  await page.setViewportSize({ width: 768, height: 900 });

  await openServer(page, 4000, [
    cpuSeries([130, 200, 250]),
    counterSeries('gameap_server_network_receive_bytes_total', 24_000),
    counterSeries('gameap_server_network_transmit_bytes_total', 68_000),
    counterSeries('gameap_server_block_io_read_bytes_total', 2_048),
    counterSeries('gameap_server_block_io_write_bytes_total', 10_000),
  ]);

  const strip = page.locator('.server-stats-strip');
  await expect(strip).toContainText('23.4 KiB/s', { timeout: 20_000 });

  // text-xs is 16px tall per line. A rate is matched as a substring: it shares
  // its element with the direction arrow.
  const texts = [
    ...['CPU', 'MEM', 'NET', 'DISK'].map((t) => strip.getByText(t, { exact: true })),
    ...['23.4 KiB/s', '66.4 KiB/s'].map((t) => strip.getByText(t)),
  ];
  for (const text of texts) {
    const box = await text.boundingBox();
    expect(box, String(text)).not.toBeNull();
    expect(box!.height, `${String(text)} must stay on one line`).toBeLessThan(24);
  }

  const barBox = await page.getByTestId('server-stats-cpu').locator('.n-progress').boundingBox();
  expect(barBox).not.toBeNull();
  expect(barBox!.width, 'CPU bar must keep its width').toBeGreaterThan(40);
});
