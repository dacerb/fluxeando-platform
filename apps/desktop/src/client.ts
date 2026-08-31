export type ApiRequest = { path: string; method?: string; body?: unknown; token?: string };

declare global {
  interface Window {
    cashflow?: { request<T>(input: ApiRequest): Promise<T>; download?(input: ApiRequest): Promise<{ data: string; contentType: string }>; rememberedSession?: { get(): Promise<string | undefined>; set(token: string): Promise<void>; clear(): Promise<void> }; rendererReady?(): Promise<void>; onPortConflict?(listener: (value: { port: number }) => void): () => void; choosePortConflict?(useAlternatePort: boolean): Promise<void>; mcpEnabledChanged?(enabled: boolean): Promise<void>; onMCPUrlChanged?(listener: () => void): () => void; runtime?(): Promise<{ version: string; mode: string; storageConfigured?: boolean; backendStarting?: boolean; backendReady?: boolean; backendError?: string; mcpUrl?: string; storageType?: string; dbPath?: string; defaultDbPath?: string; mysql?: { host: string; port: string; database: string; username: string } }>; revealDatabase?(): Promise<void>; chooseDatabase?(input: { dbPath?: string }): Promise<string | undefined>; chooseNewDatabase?(input: { dbPath?: string }): Promise<string | undefined>; configureStorage?(input: { mode: 'local'; dbPath: string }): Promise<{ mode: 'local'; dbPath: string }>; configureMySQL?(input: { host: string; port: string; database: string; username: string; password: string }): Promise<{ mode: 'mysql'; host: string; port: string; database: string; username: string }>; createStorage?(input: { mode: 'local'; dbPath?: string }): Promise<{ mode: 'local'; dbPath: string }> };
  }
}

export async function request<T>(path: string, token?: string, method = 'GET', body?: unknown): Promise<T> {
  if (window.cashflow) return window.cashflow.request<T>({ path, token, method, body });

  const response = await fetch(path, {
    method,
    headers: { 'content-type': 'application/json', ...(token ? { authorization: `Bearer ${token}` } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body)
  });
  const text = await response.text();
  let payload: { error?: string } | undefined;
  if (text.trim()) {
    try { payload = JSON.parse(text) as { error?: string }; } catch { payload = undefined; }
  }
  if (!response.ok) {
    if (response.status === 404 && text.includes('404 page not found')) {
      throw new Error('The local API is from another version. Stop the current development processes and start CashFlow with the web command again.');
    }
    throw new Error(payload?.error || text || 'Request failed');
  }
  return (payload ?? text) as T;
}
