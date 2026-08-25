import { app, BrowserWindow, dialog, ipcMain } from 'electron';
import path from 'node:path';
import fs from 'node:fs';
import crypto from 'node:crypto';
import { spawn, ChildProcess } from 'node:child_process';
import net from 'node:net';

let backend: ChildProcess | undefined;
let mainWindow: BrowserWindow | undefined;
let apiUrl = process.env.CASHFLOW_API_URL ?? '';
type StorageConfig = { mode: 'local' | 'network'; dbPath: string };
let storageConfig: StorageConfig | undefined;
const defaultDatabasePath = () => path.join(app.getPath('userData'), 'cashflow.db');
const newDefaultDatabasePath = () => {
  const stamp = new Date().toISOString().replace(/[-:TZ.]/g, '').slice(0, 14);
  let attempt = 0;
  let candidate = path.join(app.getPath('userData'), `cashflow-${stamp}.db`);
  while (fs.existsSync(candidate)) { attempt += 1; candidate = path.join(app.getPath('userData'), `cashflow-${stamp}-${attempt}.db`); }
  return candidate;
};
const storageConfigPath = () => path.join(app.getPath('userData'), 'storage.json');
function loadStorageConfig(): StorageConfig | undefined {
  if (process.env.CASHFLOW_API_URL) return { mode: 'local', dbPath: defaultDatabasePath() };
  try {
    const value = JSON.parse(fs.readFileSync(storageConfigPath(), 'utf8')) as StorageConfig;
    if ((value.mode === 'local' || value.mode === 'network') && typeof value.dbPath === 'string' && path.isAbsolute(value.dbPath)) return value;
  } catch { /* A fresh install has no saved storage selection. */ }
  return fs.existsSync(defaultDatabasePath()) ? { mode: 'local', dbPath: defaultDatabasePath() } : undefined;
}
function saveStorageConfig(config: StorageConfig) {
  fs.mkdirSync(app.getPath('userData'), { recursive: true });
  fs.writeFileSync(storageConfigPath(), JSON.stringify(config), { encoding: 'utf8', mode: 0o600 });
}
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
  if (!storageConfig) throw new Error('Storage must be configured before starting the local API');
  const port = await findFreePort();
  apiUrl = `http://127.0.0.1:${port}`;
  const executable = process.env.CASHFLOW_API_BIN ?? path.join(process.resourcesPath, 'cashflow-api', process.platform === 'win32' ? 'cashflow-api.exe' : 'cashflow-api');
  const db = storageConfig.dbPath;
  fs.mkdirSync(path.dirname(db), { recursive: true });
  backend = spawn(executable, ['-db', db, '-addr', `127.0.0.1:${port}`], { stdio: 'ignore' });
  backend.on('error', error => console.error('Unable to start local Go API', error));
  await waitForApi();
}
async function stopBackend() {
  const running = backend;
  backend = undefined;
  if (!running || running.exitCode !== null) return;
  await new Promise<void>(resolve => {
    const timeout = setTimeout(resolve, 1_500);
    running.once('exit', () => { clearTimeout(timeout); resolve(); });
    running.kill();
  });
}
async function validateDatabase(dbPath: string) {
  if (process.env.CASHFLOW_API_URL) {
    const response = await fetch(`${apiUrl}/v1/storage/validate`, { method: 'POST', headers: { 'content-type': 'application/vnd.sqlite3' }, body: fs.readFileSync(dbPath), signal: AbortSignal.timeout(15_000) });
    if (!response.ok) throw new Error(await response.text() || 'The selected SQLite database is not compatible with CashFlow');
    return;
  }
  const executable = process.env.CASHFLOW_API_BIN ?? path.join(process.resourcesPath, 'cashflow-api', process.platform === 'win32' ? 'cashflow-api.exe' : 'cashflow-api');
  await new Promise<void>((resolve, reject) => {
    const child = spawn(executable, ['-db', dbPath, '-validate'], { stdio: ['ignore', 'ignore', 'pipe'] });
    let stderr = '';
    child.stderr?.on('data', chunk => { stderr += String(chunk); });
    child.once('error', reject);
    child.once('close', code => code === 0 ? resolve() : reject(new Error(stderr.trim() || 'The selected SQLite database is not compatible with CashFlow')));
  });
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
async function apiDownload(_: Electron.IpcMainInvokeEvent, input: { path: string; token?: string }) {
  if (!input.path.startsWith('/v1/')) throw new Error('Invalid API path');
  const response = await fetch(`${apiUrl}${input.path}`, { headers: { 'x-correlation-id': crypto.randomUUID(), ...(input.token ? { authorization: `Bearer ${input.token}` } : {}) } });
  if (!response.ok) throw new Error(await response.text() || 'Download failed');
  return { data: Buffer.from(await response.arrayBuffer()).toString('base64'), contentType: response.headers.get('content-type') ?? 'application/octet-stream' };
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
ipcMain.handle('cashflow:download', apiDownload);
ipcMain.handle('cashflow:runtime', () => {
  return { version: app.getVersion(), mode: process.env.VITE_DEV_SERVER_URL ? 'Development desktop' : 'Production desktop', storageConfigured: Boolean(storageConfig), storageType: storageConfig?.mode === 'network' ? 'network_sqlite' : 'local_sqlite', dbPath: storageConfig?.dbPath, defaultDbPath: defaultDatabasePath() };
});
ipcMain.handle('cashflow:choose-database', async (_event, input: { dbPath?: string }) => {
  const options: Electron.OpenDialogOptions = { title: 'Seleccionar base de datos SQLite', defaultPath: input.dbPath || storageConfig?.dbPath || defaultDatabasePath(), properties: ['openFile'], filters: [{ name: 'SQLite', extensions: ['db', 'sqlite', 'sqlite3'] }] };
  const result = await (mainWindow ? dialog.showOpenDialog(mainWindow, options) : dialog.showOpenDialog(options));
  if (result.canceled) return undefined;
  const selectedPath = result.filePaths[0];
  await validateDatabase(selectedPath);
  return selectedPath;
});
ipcMain.handle('cashflow:choose-new-database', async (_event, input: { dbPath?: string }) => {
  const options = { title: 'Crear base de datos SQLite', defaultPath: input.dbPath || newDefaultDatabasePath(), filters: [{ name: 'SQLite', extensions: ['db', 'sqlite', 'sqlite3'] }] };
  const result = await (mainWindow ? dialog.showSaveDialog(mainWindow, options) : dialog.showSaveDialog(options));
  return result.canceled ? undefined : result.filePath;
});
ipcMain.handle('cashflow:configure-storage', async (_event, input: StorageConfig) => {
  if ((input.mode !== 'local' && input.mode !== 'network') || !input.dbPath || !path.isAbsolute(input.dbPath)) throw new Error('Invalid storage configuration');
  await validateDatabase(input.dbPath);
  storageConfig = { mode: input.mode, dbPath: input.dbPath };
  saveStorageConfig(storageConfig);
  await stopBackend();
  await startBackend();
  return { ...storageConfig };
});
ipcMain.handle('cashflow:create-storage', async (_event, input: { mode: StorageConfig['mode']; dbPath?: string }) => {
  const dbPath = input.dbPath || newDefaultDatabasePath();
  if ((input.mode !== 'local' && input.mode !== 'network') || !path.isAbsolute(dbPath)) throw new Error('Invalid storage configuration');
  if (fs.existsSync(dbPath)) throw new Error('A database already exists at that location');
  storageConfig = { mode: input.mode, dbPath };
  saveStorageConfig(storageConfig);
  await stopBackend();
  await startBackend();
  return { ...storageConfig };
});
app.whenReady().then(async () => { storageConfig = loadStorageConfig(); try { if (storageConfig) await startBackend(); } catch (error) { console.error('Unable to start local Go API', error); } createWindow(); });
app.on('before-quit', () => backend?.kill());
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit(); });
app.on('activate', () => createWindow());
