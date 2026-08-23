import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
const apiPort = process.env.CASHFLOW_API_PORT ?? '8787';
export default defineConfig({
  base: './',
  plugins: [react()],
  build: { outDir: 'renderer' },
  server: { proxy: { '/v1': `http://127.0.0.1:${apiPort}`, '/health': `http://127.0.0.1:${apiPort}` } }
});
