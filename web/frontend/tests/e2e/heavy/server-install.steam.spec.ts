import { test, expect } from '@playwright/test';
import { loginViaAPI } from '../fixtures/auth';
import { provisionGameServer, waitForInstallSuccess } from '../fixtures/install';

// Steamcmd install path. A dedicated game code with steam_app_id_linux set and
// remote_repository_linux explicitly null forces the daemon down the steamcmd
// branch (the standard "cstrike" definition has BOTH a steam app id and a
// repository tarball, so it would not exercise steamcmd).
//
// App 90 is the smallest anonymous Steam dedicated server (HLDS / CS 1.6,
// ~250 MB) but is slow and historically flaky, so this spec runs only in the
// nightly `heavy-steam` Playwright project — never on every push.
test('installs a steamcmd (HLDS app 90) game server', async ({ request }) => {
  const token = await loginViaAPI(request);

  const provisioned = await provisionGameServer(request, token, {
    game: {
      code: 'e2ehlds',
      name: 'E2E HLDS (steamcmd app 90)',
      engine: 'GoldSource',
      engine_version: '1',
      steam_app_id_linux: 90,
      remote_repository_linux: null,
    },
    mod: {
      game_code: 'e2ehlds',
      name: 'Default',
      start_cmd_linux:
        './hlds_run -game cstrike +ip {ip} +port {port} +map de_dust2 ' +
        '+maxplayers {maxplayers}',
      vars: [{ var: 'maxplayers', default: '16', info: 'Maximum players' }],
    },
    serverName: 'E2E CS 1.6 (steamcmd)',
    serverPort: 27015,
  });

  const task = await waitForInstallSuccess(request, token, provisioned, {
    timeoutMs: 22 * 60_000,
  });

  expect(task.status).toBe('success');
});
