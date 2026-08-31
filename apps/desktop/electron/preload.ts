import { contextBridge, ipcRenderer } from 'electron';
contextBridge.exposeInMainWorld('cashflow', {
  request: (input: { path: string; method?: string; body?: unknown; token?: string }) => ipcRenderer.invoke('cashflow:api', input),
  download: (input: { path: string; token?: string }) => ipcRenderer.invoke('cashflow:download', input),
  rememberedSession: { get: () => ipcRenderer.invoke('cashflow:remembered-session:get'), set: (token: string) => ipcRenderer.invoke('cashflow:remembered-session:set', token), clear: () => ipcRenderer.invoke('cashflow:remembered-session:clear') },
  rendererReady: () => ipcRenderer.invoke('cashflow:renderer-ready'),
  onPortConflict: (listener: (value: { port: number }) => void) => { const callback = (_: Electron.IpcRendererEvent, value: { port: number }) => listener(value); ipcRenderer.on('cashflow:port-conflict', callback); return () => ipcRenderer.removeListener('cashflow:port-conflict', callback); },
  choosePortConflict: (useAlternatePort: boolean) => ipcRenderer.invoke('cashflow:port-conflict:choose', useAlternatePort),
  mcpEnabledChanged: (enabled: boolean) => ipcRenderer.invoke('cashflow:mcp-enabled-changed', enabled),
  onMCPUrlChanged: (listener: () => void) => { const callback = () => listener(); ipcRenderer.on('cashflow:mcp-url-changed', callback); return () => ipcRenderer.removeListener('cashflow:mcp-url-changed', callback); },
  runtime: () => ipcRenderer.invoke('cashflow:runtime'),
  revealDatabase: () => ipcRenderer.invoke('cashflow:reveal-database'),
  chooseDatabase: (input: { dbPath?: string }) => ipcRenderer.invoke('cashflow:choose-database', input),
  chooseNewDatabase: (input: { dbPath?: string }) => ipcRenderer.invoke('cashflow:choose-new-database', input),
  configureStorage: (input: { mode: 'local' | 'network'; dbPath: string }) => ipcRenderer.invoke('cashflow:configure-storage', input),
  configureMySQL: (input: { host: string; port: string; database: string; username: string; password: string }) => ipcRenderer.invoke('cashflow:configure-mysql', input),
  createStorage: (input: { mode: 'local' | 'network'; dbPath?: string }) => ipcRenderer.invoke('cashflow:create-storage', input)
});
