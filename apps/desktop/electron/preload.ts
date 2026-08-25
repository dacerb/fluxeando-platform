import { contextBridge, ipcRenderer } from 'electron';
contextBridge.exposeInMainWorld('cashflow', {
  request: (input: { path: string; method?: string; body?: unknown; token?: string }) => ipcRenderer.invoke('cashflow:api', input),
  download: (input: { path: string; token?: string }) => ipcRenderer.invoke('cashflow:download', input),
  rememberedSession: { get: () => ipcRenderer.invoke('cashflow:remembered-session:get'), set: (token: string) => ipcRenderer.invoke('cashflow:remembered-session:set', token), clear: () => ipcRenderer.invoke('cashflow:remembered-session:clear') },
  runtime: () => ipcRenderer.invoke('cashflow:runtime'),
  chooseDatabase: (input: { dbPath?: string }) => ipcRenderer.invoke('cashflow:choose-database', input),
  chooseNewDatabase: (input: { dbPath?: string }) => ipcRenderer.invoke('cashflow:choose-new-database', input),
  configureStorage: (input: { mode: 'local' | 'network'; dbPath: string }) => ipcRenderer.invoke('cashflow:configure-storage', input),
  configureMySQL: (input: { host: string; port: string; database: string; username: string; password: string }) => ipcRenderer.invoke('cashflow:configure-mysql', input),
  createStorage: (input: { mode: 'local' | 'network'; dbPath?: string }) => ipcRenderer.invoke('cashflow:create-storage', input)
});
