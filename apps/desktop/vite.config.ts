import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
const apiPort = process.env.CASHFLOW_API_PORT ?? '8787';
const appVersion = JSON.parse(readFileSync(resolve(import.meta.dirname, 'package.json'), 'utf8')).version;
export default defineConfig({
  base: './',
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
    // In web development the API port is selected at startup. Surface the
    // effective local MCP address to the renderer instead of showing 8787.
    __WEB_MCP_URL__: JSON.stringify(`http://127.0.0.1:${apiPort}/mcp`)
  },
  plugins: [react()],
  build: { outDir: 'renderer' },
  server: { proxy: { '/v1': `http://127.0.0.1:${apiPort}`, '/health': `http://127.0.0.1:${apiPort}` } }
});
