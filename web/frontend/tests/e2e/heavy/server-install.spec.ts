import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { listNodes } from '../fixtures/api';
import { provisionGameServer, waitForInstallSuccess } from '../fixtures/install';

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

// Non-steam install path: the daemon downloads and extracts the repository
// tarball because steam_app_id_linux is 0 and remote_repository_linux is set.
test('installs a Quake 2 (repository) game server', async ({ request }) => {
  const token = await loginViaAPI(request);

  const provisioned = await provisionGameServer(request, token, {
    game: {
      code: 'q2',
      name: 'Quake 2',
      engine: 'idtech',
      engine_version: '2',
      steam_app_id_linux: 0,
      remote_repository_linux:
        'https://files.gameap.ru/quake/quake2-linux-i386-full.tar.gz',
    },
    mod: {
      game_code: 'q2',
      name: 'Default',
      start_cmd_linux:
        './run.sh +set dedicated 1 +set ip {ip} +set port {port} ' +
        '+set hostname "{hostname}" +set maxclients {maxplayers} +map {map}',
      vars: [
        { var: 'hostname', default: 'Quake 2 Server', info: 'Server Hostname' },
        {
          var: 'maxplayers',
          default: '16',
          info: 'Maximum players',
          admin_var: true,
        },
        { var: 'map', default: 'base1', info: 'Map' },
      ],
    },
    serverName: 'E2E Quake 2',
    serverPort: 27910,
  });

  const task = await waitForInstallSuccess(request, token, provisioned, {
    timeoutMs: 8 * 60_000,
  });

  expect(task.status).toBe('success');
});
