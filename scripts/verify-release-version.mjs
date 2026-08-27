import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const tag = process.env.GITHUB_REF_NAME;
const version = JSON.parse(readFileSync(resolve(root, 'apps/desktop/package.json'), 'utf8')).version;

if (!tag || !/^v\d+\.\d+\.\d+$/.test(tag)) {
  throw new Error('A release tag must use the vMAJOR.MINOR.PATCH format, for example v1.2.3.');
}
if (tag.slice(1) !== version) {
  throw new Error(`Tag ${tag} does not match apps/desktop/package.json version ${version}.`);
}
