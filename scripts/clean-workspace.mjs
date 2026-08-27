import { rm } from 'node:fs/promises';
import { resolve } from 'node:path';

const workspaceRoot = resolve(import.meta.dirname, '..');
const targets = [
  'node_modules',
  'apps/desktop/node_modules',
  'apps/desktop/renderer',
  'apps/desktop/dist',
];

for (const target of targets) {
  const path = resolve(workspaceRoot, target);
  await rm(path, { recursive: true, force: true });
  console.log(`Eliminado: ${target}`);
}
