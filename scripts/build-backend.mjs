import { mkdirSync, rmSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import process from 'node:process';

const root = resolve(import.meta.dirname, '..');
const targets = {
  'mac-arm64': { GOOS: 'darwin', GOARCH: 'arm64', output: 'apps/desktop/resources/bin/mac-arm64/cashflow-api' },
  'win-x64': { GOOS: 'windows', GOARCH: 'amd64', output: 'apps/desktop/resources/bin/win-x64/cashflow-api.exe' }
};
const requested = process.argv[2] ?? 'all';
const names = requested === 'all' ? Object.keys(targets) : [requested];

for (const name of names) {
  const target = targets[name];
  if (!target) throw new Error(`Unsupported backend target: ${name}`);
  const output = resolve(root, target.output);
  mkdirSync(dirname(output), { recursive: true });
  rmSync(output, { force: true });
  const result = spawnSync('go', ['build', '-o', output, './cmd/api'], {
    cwd: resolve(root, 'services/cashflow-api'),
    env: { ...process.env, GOOS: target.GOOS, GOARCH: target.GOARCH },
    stdio: 'inherit'
  });
  if (result.status !== 0) process.exit(result.status ?? 1);
}
