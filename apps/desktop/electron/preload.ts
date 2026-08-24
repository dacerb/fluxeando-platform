import { contextBridge, ipcRenderer } from 'electron';
contextBridge.exposeInMainWorld('cashflow', {
  request: (input: { path: string; method?: string; body?: unknown; token?: string }) => ipcRenderer.invoke('cashflow:api', input),
  download: (input: { path: string; token?: string }) => ipcRenderer.invoke('cashflow:download', input),
  runtime: () => ipcRenderer.invoke('cashflow:runtime')
});
