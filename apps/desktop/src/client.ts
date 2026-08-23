export type ApiRequest = { path: string; method?: string; body?: unknown; token?: string };

declare global {
  interface Window {
    cashflow?: { request<T>(input: ApiRequest): Promise<T>; runtime?(): Promise<{ version: string; mode: string }> };
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
  if (!response.ok) throw new Error(payload?.error || text || 'Request failed');
  return (payload ?? text) as T;
}
