import { execFileSync } from 'node:child_process';

const CONTAINER = process.env.E2E_DAEMON_CONTAINER ?? 'gameap-daemon';

export function runInDaemonContainer(args: string[], timeoutMs = 5 * 60_000): string {
  return execFileSync(
    'docker',
    ['exec', CONTAINER, ...args],
    { stdio: 'pipe', timeout: timeoutMs, encoding: 'utf8' },
  );
}

export function enrollDaemon(setupKey: string): void {
  runInDaemonContainer([
    'gameap-daemon', 'enroll',
    '--connect', `grpc://host.docker.internal:31718/${setupKey}`,
    '--config-path', '/etc/gameap-daemon/gameap-daemon.yaml',
    '--certs-dir', '/etc/gameap-daemon/certs',
    '--work-path', '/var/gameap/work',
    '--listen-ip', '127.0.0.1',
    '--listen-port', '31717',
  ]);
}

export function startDaemon(): void {
  execFileSync('docker', [
    'exec', '-d', CONTAINER,
    'gameap-daemon', '--config=/etc/gameap-daemon/gameap-daemon.yaml',
  ]);
}
