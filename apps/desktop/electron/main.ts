import { app, BrowserWindow, ipcMain } from 'electron';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawn, ChildProcess } from 'node:child_process';
import net from 'node:net';

let backend: ChildProcess | undefined;
let mainWindow: BrowserWindow | undefined;
let apiUrl = process.env.CASHFLOW_API_URL ?? '';
function findFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') { server.close(); reject(new Error('Could not allocate local API port')); return; }
      server.close(error => error ? reject(error) : resolve(address.port));
    });
  });
}
async function waitForApi() {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    try { if ((await fetch(`${apiUrl}/health`)).ok) return; } catch { /* backend is starting */ }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('Local CashFlow API did not become ready');
}
async function startBackend() {
  if (process.env.CASHFLOW_API_URL) { apiUrl = process.env.CASHFLOW_API_URL; await waitForApi(); return; }
  const port = await findFreePort();
  apiUrl = `http://127.0.0.1:${port}`;
  const executable = process.env.CASHFLOW_API_BIN ?? path.join(process.resourcesPath, 'cashflow-api', process.platform === 'win32' ? 'cashflow-api.exe' : 'cashflow-api');
  const db = path.join(app.getPath('userData'), 'cashflow.db');
  backend = spawn(executable, ['-db', db, '-addr', `127.0.0.1:${port}`], { stdio: 'ignore' });
  backend.on('error', error => console.error('Unable to start local Go API', error));
  await waitForApi();
}
async function apiRequest(_: Electron.IpcMainInvokeEvent, input: { path: string; method?: string; body?: unknown; token?: string }) {
  if (!input.path.startsWith('/v1/') && input.path !== '/health') throw new Error('Invalid API path');
  const response = await fetch(`${apiUrl}${input.path}`, {
    method: input.method ?? 'GET',
    headers: { 'content-type': 'application/json', 'x-correlation-id': crypto.randomUUID(), ...(input.token ? { authorization: `Bearer ${input.token}` } : {}) },
    body: input.body ? JSON.stringify(input.body) : undefined,
    signal: AbortSignal.timeout(10_000)
  });
  const text = await response.text();
  let payload: unknown = undefined;
  if (text.trim() && response.headers.get('content-type')?.includes('application/json')) {
    try { payload = JSON.parse(text); } catch { payload = undefined; }
  }
  if (!response.ok) {
    const message = typeof payload === 'object' && payload !== null && 'error' in payload ? String(payload.error) : text || 'Request failed';
    throw new Error(message);
  }
  return payload ?? text;
}
function createWindow() {
  if (mainWindow && !mainWindow.isDestroyed()) { mainWindow.show(); mainWindow.focus(); return; }
  const win = new BrowserWindow({ width: 1200, height: 780, show: true, webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, nodeIntegration: false } });
  mainWindow = win;
  win.on('closed', () => { mainWindow = undefined; });
  win.webContents.on('did-fail-load', (_, code, description) => console.error('Renderer failed to load', { code, description }));
  win.webContents.on('console-message', (_, level, message) => console.error('Renderer console message', { level, message }));
  if (process.env.CASHFLOW_DEBUG === '1') win.webContents.openDevTools({ mode: 'detach' });
  const page = process.env.VITE_DEV_SERVER_URL;
  if (page) win.loadURL(page); else win.loadFile(path.join(__dirname, '../renderer/index.html'));
}
ipcMain.handle('cashflow:api', apiRequest);
ipcMain.handle('cashflow:runtime', () => ({ version: app.getVersion(), mode: process.env.VITE_DEV_SERVER_URL ? 'Development desktop' : 'Production desktop' }));
app.whenReady().then(async () => { try { await startBackend(); } catch (error) { console.error('Unable to start local Go API', error); } createWindow(); });
app.on('before-quit', () => backend?.kill());
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit(); });
app.on('activate', () => createWindow());
