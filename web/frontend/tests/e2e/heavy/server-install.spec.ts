import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { listNodes } from '../fixtures/api';

test('enrolled daemon appears as a node in API', async ({ request }) => {
  const token = await loginViaAPI(request);

  await expect
    .poll(
      async () => {
        const nodes = await listNodes(request, token);

        return nodes.length;
      },
      { timeout: 30_000, intervals: [2_000] },
    )
    .toBeGreaterThan(0);

  const nodes = await listNodes(request, token);
  const node = nodes[0];

  expect(node.os).toBe('linux');
  expect(node.enabled).toBe(true);
  expect(node.ip.length).toBeGreaterThan(0);
});
