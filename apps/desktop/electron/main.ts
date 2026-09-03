import { app, BrowserWindow, dialog, ipcMain, safeStorage, shell } from 'electron';
import path from 'node:path';
import fs from 'node:fs';
import crypto from 'node:crypto';
import { execFileSync, spawn, ChildProcess } from 'node:child_process';
import net from 'node:net';

let backend: ChildProcess | undefined;
let mainWindow: BrowserWindow | undefined;
let apiUrl = process.env.CASHFLOW_API_URL ?? '';
const appIconPath = () => app.isPackaged ? path.join(process.resourcesPath, 'app-icon.png') : path.join(__dirname, '../build/icon.png');
type LocalStorageConfig = { mode: 'local'; dbPath: string };
type MySQLStorageConfig = { mode: 'mysql'; host: string; port: string; database: string; username: string; encryptedPassword: string };
type StorageConfig = LocalStorageConfig | MySQLStorageConfig;
let storageConfig: StorageConfig | undefined;
let backendError = '';
let backendStarting = false;
let resolveRendererReady: (() => void) | undefined;
let resolvePortConflict: ((useAlternatePort: boolean) => void) | undefined;
const rendererReady = new Promise<void>(resolve => { resolveRendererReady = resolve; });
const defaultAPIPort = 8787;
const defaultDatabasePath = () => path.join(app.getPath('userData'), 'cashflow.db');
const newDefaultDatabasePath = () => {
  const stamp = new Date().toISOString().replace(/[-:TZ.]/g, '').slice(0, 14);
  let attempt = 0;
  let candidate = path.join(app.getPath('userData'), `cashflow-${stamp}.db`);
  while (fs.existsSync(candidate)) { attempt += 1; candidate = path.join(app.getPath('userData'), `cashflow-${stamp}-${attempt}.db`); }
  return candidate;
};
const storageConfigPath = () => path.join(app.getPath('userData'), 'storage.json');
const rememberedSessionPath = () => path.join(app.getPath('userData'), 'remembered-session.bin');
function rememberCleanupDatabase(config: StorageConfig | undefined) {
  if (process.platform !== 'win32') return;
  try {
    if (config?.mode === 'local') {
      execFileSync('reg.exe', ['add', 'HKCU\\Software\\CashFlow', '/v', 'LocalDatabasePath', '/t', 'REG_SZ', '/d', config.dbPath, '/f'], { windowsHide: true });
    } else {
      execFileSync('reg.exe', ['delete', 'HKCU\\Software\\CashFlow', '/v', 'LocalDatabasePath', '/f'], { windowsHide: true });
    }
  } catch { /* The uninstaller can still remove the app profile if the registry is unavailable. */ }
}
function saveRememberedSession(token: string) {
  if (!safeStorage.isEncryptionAvailable()) throw new Error('Secure system storage is unavailable on this device');
  fs.mkdirSync(app.getPath('userData'), { recursive: true });
  fs.writeFileSync(rememberedSessionPath(), safeStorage.encryptString(token), { mode: 0o600 });
}
function loadRememberedSession() {
  try {
    if (!safeStorage.isEncryptionAvailable() || !fs.existsSync(rememberedSessionPath())) return undefined;
    return safeStorage.decryptString(fs.readFileSync(rememberedSessionPath()));
  } catch {
    return undefined;
  }
}
function clearRememberedSession() {
  try { fs.unlinkSync(rememberedSessionPath()); } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
}
function loadStorageConfig(): StorageConfig | undefined {
  if (process.env.CASHFLOW_API_URL) return { mode: 'local', dbPath: defaultDatabasePath() };
  try {
    const value = JSON.parse(fs.readFileSync(storageConfigPath(), 'utf8')) as StorageConfig;
    if (value.mode === 'local' && typeof value.dbPath === 'string' && path.isAbsolute(value.dbPath)) return value;
    if ((value as { mode?: string; dbPath?: unknown }).mode === 'network' && typeof (value as { dbPath?: unknown }).dbPath === 'string' && path.isAbsolute((value as { dbPath: string }).dbPath)) return { mode: 'local', dbPath: (value as { dbPath: string }).dbPath };
    if (value.mode === 'mysql' && [value.host, value.port, value.database, value.username, value.encryptedPassword].every(part => typeof part === 'string' && part.length > 0)) return value;
  } catch { /* A fresh install has no saved storage selection. */ }
  return fs.existsSync(defaultDatabasePath()) ? { mode: 'local', dbPath: defaultDatabasePath() } : undefined;
}
function saveStorageConfig(config: StorageConfig) {
  fs.mkdirSync(app.getPath('userData'), { recursive: true });
  fs.writeFileSync(storageConfigPath(), JSON.stringify(config), { encoding: 'utf8', mode: 0o600 });
  rememberCleanupDatabase(config);
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
function isPortAvailable(port: number): Promise<boolean> {
  return new Promise(resolve => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.listen(port, '127.0.0.1', () => server.close(error => resolve(!error)));
  });
}
async function chooseBackendPort(): Promise<number> {
  if (await isPortAvailable(defaultAPIPort)) return defaultAPIPort;
  if (!mainWindow || mainWindow.isDestroyed()) throw new Error(`El puerto ${defaultAPIPort} está ocupado`);
  const useAlternatePort = await new Promise<boolean>(resolve => {
    resolvePortConflict = resolve;
    mainWindow?.webContents.send('cashflow:port-conflict', { port: defaultAPIPort });
  });
  resolvePortConflict = undefined;
  if (!useAlternatePort) throw new Error(`El puerto ${defaultAPIPort} está ocupado y no se autorizó usar otro puerto local`);
  return findFreePort();
}
async function waitForApi() {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    try { if ((await fetch(`${apiUrl}/health`)).ok) return; } catch { /* backend is starting */ }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('Local CashFlow API did not become ready');
}
async function startBackend(options: { preferMCPPort?: boolean } = {}) {
  backendError = '';
  if (process.env.CASHFLOW_API_URL) { apiUrl = process.env.CASHFLOW_API_URL; await waitForApi(); return; }
  if (!storageConfig) throw new Error('Storage must be configured before starting the local API');
  const port = options.preferMCPPort === false ? await findFreePort() : await chooseBackendPort();
  apiUrl = `http://127.0.0.1:${port}`;
  const executable = process.env.CASHFLOW_API_BIN ?? path.join(process.resourcesPath, 'cashflow-api', process.platform === 'win32' ? 'cashflow-api.exe' : 'cashflow-api');
  const args = ['-addr', `127.0.0.1:${port}`];
  const environment: NodeJS.ProcessEnv = { ...process.env };
  if (storageConfig.mode === 'mysql') {
    if (!safeStorage.isEncryptionAvailable()) throw new Error('Secure system storage is unavailable on this device');
    environment.CASHFLOW_MYSQL_PASSWORD = safeStorage.decryptString(Buffer.from(storageConfig.encryptedPassword, 'base64'));
    args.push('-mysql-host', storageConfig.host, '-mysql-port', storageConfig.port, '-mysql-database', storageConfig.database, '-mysql-username', storageConfig.username);
  } else {
    fs.mkdirSync(path.dirname(storageConfig.dbPath), { recursive: true });
    args.push('-db', storageConfig.dbPath);
  }
  backend = spawn(executable, args, { stdio: 'ignore', env: environment });
  backend.on('error', error => { backendError = error.message; console.error('Unable to start local Go API', error); });
  try { await waitForApi(); } catch (error) { backendError ||= error instanceof Error ? error.message : 'The local API did not start'; throw error; }
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
  const win = new BrowserWindow({ width: 1200, height: 780, minWidth: 1024, minHeight: 720, show: true, icon: appIconPath(), webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, nodeIntegration: false } });
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
ipcMain.handle('cashflow:remembered-session:get', () => loadRememberedSession());
ipcMain.handle('cashflow:remembered-session:set', (_event, token: string) => { if (typeof token !== 'string' || !token) throw new Error('Invalid remembered session'); saveRememberedSession(token); });
ipcMain.handle('cashflow:remembered-session:clear', () => clearRememberedSession());
ipcMain.handle('cashflow:renderer-ready', () => resolveRendererReady?.());
ipcMain.handle('cashflow:port-conflict:choose', (_, useAlternatePort: boolean) => resolvePortConflict?.(Boolean(useAlternatePort)));
ipcMain.handle('cashflow:mcp-enabled-changed', async (_, enabled: boolean) => {
  if (process.env.CASHFLOW_API_URL || !storageConfig) return;
  await stopBackend();
  try {
    await startBackend({ preferMCPPort: Boolean(enabled) });
  } catch (error) {
    await startBackend({ preferMCPPort: false });
    throw error;
  } finally {
    mainWindow?.webContents.send('cashflow:mcp-url-changed');
  }
});
ipcMain.handle('cashflow:runtime', () => {
  return { version: app.getVersion(), mode: process.env.VITE_DEV_SERVER_URL ? 'Development desktop' : 'Production desktop', platform: process.platform, storageConfigured: Boolean(storageConfig), backendStarting, backendReady: Boolean(apiUrl) && !backendError, backendError: backendError || undefined, mcpUrl: apiUrl ? `${apiUrl}/mcp` : undefined, storageType: storageConfig?.mode === 'mysql' ? 'mysql' : 'local_sqlite', dbPath: storageConfig?.mode === 'local' ? storageConfig.dbPath : undefined, defaultDbPath: defaultDatabasePath(), mysql: storageConfig?.mode === 'mysql' ? { host: storageConfig.host, port: storageConfig.port, database: storageConfig.database, username: storageConfig.username } : undefined };
});
ipcMain.handle('cashflow:reveal-database', () => {
  if (storageConfig?.mode !== 'local' || !storageConfig.dbPath) throw new Error('A local database is not configured');
  if (!fs.existsSync(storageConfig.dbPath)) throw new Error('The configured database file could not be found');
  shell.showItemInFolder(storageConfig.dbPath);
});
ipcMain.handle('cashflow:choose-database', async (_event, input: { dbPath?: string }) => {
  const savedDatabasePath = storageConfig?.mode === 'local' ? storageConfig.dbPath : undefined;
  const options: Electron.OpenDialogOptions = { title: 'Seleccionar base de datos SQLite', defaultPath: input.dbPath || savedDatabasePath || defaultDatabasePath(), properties: ['openFile'], filters: [{ name: 'SQLite', extensions: ['db', 'sqlite', 'sqlite3'] }] };
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
ipcMain.handle('cashflow:configure-storage', async (_event, input: LocalStorageConfig) => {
  if (input.mode !== 'local' || !input.dbPath || !path.isAbsolute(input.dbPath)) throw new Error('Invalid storage configuration');
  await validateDatabase(input.dbPath);
  const previous = storageConfig;
  const next: LocalStorageConfig = { mode: input.mode, dbPath: input.dbPath };
  storageConfig = next;
  try { await stopBackend(); await startBackend(); saveStorageConfig(next); }
  catch (error) { storageConfig = previous; if (previous) { try { await startBackend(); } catch { /* Recovery screen remains available. */ } } throw error; }
  return next;
});
ipcMain.handle('cashflow:create-storage', async (_event, input: { mode: StorageConfig['mode']; dbPath?: string }) => {
  const dbPath = input.dbPath || newDefaultDatabasePath();
  if (input.mode !== 'local' || !path.isAbsolute(dbPath)) throw new Error('Invalid storage configuration');
  if (fs.existsSync(dbPath)) throw new Error('A database already exists at that location');
  storageConfig = { mode: 'local', dbPath };
  saveStorageConfig(storageConfig);
  await stopBackend();
  await startBackend();
  return { ...storageConfig };
});
ipcMain.handle('cashflow:configure-mysql', async (_event, input: { host: string; port: string; database: string; username: string; password: string }) => {
  const values = [input.host, input.port, input.database, input.username, input.password];
  if (values.some(value => typeof value !== 'string' || !value.trim())) throw new Error('Invalid MySQL configuration');
  if (!/^\d{1,5}$/.test(input.port) || Number(input.port) > 65535) throw new Error('Invalid MySQL port');
  if (!safeStorage.isEncryptionAvailable()) throw new Error('Secure system storage is unavailable on this device');
  const next: MySQLStorageConfig = { mode: 'mysql', host: input.host.trim(), port: input.port.trim(), database: input.database.trim(), username: input.username.trim(), encryptedPassword: safeStorage.encryptString(input.password).toString('base64') };
  const previous = storageConfig;
  storageConfig = next;
  try { await stopBackend(); await startBackend(); saveStorageConfig(next); }
  catch (error) { storageConfig = previous; if (previous) { try { await startBackend(); } catch { /* Keep the previous configuration for the next launch. */ } } throw error; }
  return { mode: 'mysql', host: next.host, port: next.port, database: next.database, username: next.username };
});
app.whenReady().then(async () => { app.dock?.setIcon(appIconPath()); storageConfig = loadStorageConfig(); rememberCleanupDatabase(storageConfig); backendStarting = Boolean(storageConfig); createWindow(); await rendererReady; try { if (storageConfig) await startBackend(); } catch (error) { backendError = error instanceof Error ? error.message : 'The local API did not start'; console.error('Unable to start local Go API', error); } finally { backendStarting = false; mainWindow?.webContents.reload(); } });
app.on('before-quit', () => backend?.kill());
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit(); });
app.on('activate', () => createWindow());
