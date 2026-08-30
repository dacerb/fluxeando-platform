import net from 'node:net';
import { spawn } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const desktop = path.join(root, 'apps', 'desktop');
const api = path.join(root, 'services', 'cashflow-api');
const port = await new Promise((resolve, reject) => {
  const server = net.createServer();
  server.once('error', reject);
  server.listen(0, '127.0.0.1', () => {
    const address = server.address();
    server.close(error => error ? reject(error) : resolve(address.port));
  });
});
const env = { ...process.env, CASHFLOW_API_PORT: String(port), CASHFLOW_ALLOW_STORAGE_CONFIGURATION: 'true' };
const backend = spawn('go', ['run', './cmd/api', '-addr', `127.0.0.1:${port}`], { cwd: api, env, stdio: 'inherit' });
const pnpm = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm';
const vite = spawn(pnpm, ['exec', 'vite'], { cwd: desktop, env, stdio: 'inherit' });
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => { backend.kill(signal); vite.kill(signal); });
backend.on('exit', code => { if (code) vite.kill(); });
vite.on('exit', () => backend.kill());
