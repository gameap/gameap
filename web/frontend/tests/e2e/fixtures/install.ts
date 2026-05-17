import type { APIRequestContext } from '@playwright/test';
import {
  ServerInstalled,
  createGameMod,
  createServer,
  getDaemonTask,
  getDaemonTaskOutput,
  getGameModId,
  getServer,
  listNodes,
  seedGame,
  type CreateServerResult,
  type DaemonTaskRecord,
  type GameDefinition,
  type GameModDefinition,
} from './api';

export interface GameServerSpec {
  game: GameDefinition;
  mod: GameModDefinition;
  serverName: string;
  serverIp?: string;
  serverPort: number;
}

export interface ProvisionedServer extends CreateServerResult {
  nodeId: number;
}

// provisionGameServer seeds the game + mod, picks the enrolled linux daemon
// node, and creates a server with install=true. The API persists a "gsinst"
// task and pushes it to the connected daemon over the gRPC stream
// (TaskDispatcher.Dispatch); no daemon reconnect is required.
export async function provisionGameServer(
  request: APIRequestContext,
  token: string,
  spec: GameServerSpec,
): Promise<ProvisionedServer> {
  const nodes = await listNodes(request, token);
  const node = nodes.find((n) => n.enabled && n.os === 'linux');
  if (!node) {
    throw new Error(
      `no enabled linux node available: ${JSON.stringify(nodes)}`,
    );
  }

  await seedGame(request, token, spec.game);
  await createGameMod(request, token, spec.mod);
  const gameModId = await getGameModId(
    request,
    token,
    spec.game.code,
    spec.mod.name,
  );

  const result = await createServer(request, token, {
    name: spec.serverName,
    ds_id: node.id,
    game_id: spec.game.code,
    game_mod_id: gameModId,
    server_ip: spec.serverIp ?? '127.0.0.1',
    server_port: spec.serverPort,
    install: true,
  });

  return { ...result, nodeId: node.id };
}

const TERMINAL: ReadonlySet<string> = new Set([
  'success',
  'error',
  'canceled',
]);

export interface WaitOptions {
  timeoutMs: number;
  intervalMs?: number;
}

// waitForInstallSuccess polls the install task until it reaches a terminal
// state. On failure it surfaces the daemon task output so CI logs explain why
// the install broke, then cross-checks the server's installed flag.
export async function waitForInstallSuccess(
  request: APIRequestContext,
  token: string,
  provisioned: ProvisionedServer,
  options: WaitOptions,
): Promise<DaemonTaskRecord> {
  const intervalMs = options.intervalMs ?? 5_000;
  const deadline = Date.now() + options.timeoutMs;
  let task: DaemonTaskRecord | undefined;

  while (Date.now() < deadline) {
    task = await getDaemonTask(request, token, provisioned.taskId);
    if (TERMINAL.has(task.status)) {
      break;
    }

    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }

  if (!task || !TERMINAL.has(task.status)) {
    throw new Error(
      `install task ${provisioned.taskId} did not finish within ` +
        `${options.timeoutMs}ms (last status: ${task?.status ?? 'unknown'})`,
    );
  }

  if (task.status !== 'success') {
    const output = await getDaemonTaskOutput(
      request,
      token,
      provisioned.taskId,
    );

    throw new Error(
      `install task ${provisioned.taskId} finished with status ` +
        `"${task.status}".\n--- task output ---\n${output}`,
    );
  }

  const server = await getServer(request, token, provisioned.serverId);
  if (server.installed !== ServerInstalled.Installed) {
    const output = await getDaemonTaskOutput(
      request,
      token,
      provisioned.taskId,
    );

    throw new Error(
      `task succeeded but server ${provisioned.serverId} installed flag is ` +
        `${server.installed} (expected ${ServerInstalled.Installed}); ` +
        `task output:\n${output}`,
    );
  }

  return task;
}
