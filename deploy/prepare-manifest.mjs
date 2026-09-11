import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';

const manifestPath = process.argv[2] ?? 'deploy/manifest.yaml';
const envPath = 'deploy/.env';
const secretsDir = 'deploy/secrets';

if (!existsSync(manifestPath)) {
  throw new Error(`No existe ${manifestPath}. Copiá deploy/manifest.example.yaml y completalo.`);
}
if (existsSync(envPath)) {
  throw new Error(`${envPath} ya existe. No se sobrescribe una instalación configurada.`);
}

// El manifiesto tiene un esquema deliberadamente pequeño para que el instalador
// no dependa de herramientas adicionales en el VPS.
const values = {};
let section = '';
for (const rawLine of readFileSync(manifestPath, 'utf8').split(/\r?\n/)) {
  if (/^\s*(?:#|$)/.test(rawLine)) continue;
  const line = rawLine.replace(/\s+#.*$/, '');
  if (!line.trim()) continue;
  const sectionMatch = line.match(/^([a-z_]+):\s*$/);
  if (sectionMatch) { section = sectionMatch[1]; continue; }
  const valueMatch = line.match(/^  ([a-z_]+):\s*(.*?)\s*$/);
  if (!valueMatch || !section) throw new Error(`Formato no admitido en el manifiesto: ${rawLine}`);
  values[`${section}.${valueMatch[1]}`] = valueMatch[2].replace(/^['"]|['"]$/g, '');
}

const required = ['instance.name', 'public.domain', 'public.letsencrypt_email', 'database.name', 'database.user', 'backups.time', 'backups.retention_days', 'backups.timezone'];
for (const key of required) if (!values[key]) throw new Error(`Falta ${key} en ${manifestPath}.`);
if (!/^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$/i.test(values['public.domain']) || !values['public.domain'].includes('.')) throw new Error('public.domain debe ser un nombre de dominio válido.');
if (!/^\d{2}:\d{2}$/.test(values['backups.time'])) throw new Error('backups.time debe usar el formato HH:MM.');
if (!/^\d+$/.test(values['backups.retention_days']) || Number(values['backups.retention_days']) < 1) throw new Error('backups.retention_days debe ser un entero mayor que cero.');

mkdirSync(secretsDir, { recursive: true, mode: 0o700 });
const writeSecret = (name) => {
  const path = `${secretsDir}/${name}`;
  if (!existsSync(path)) writeFileSync(path, `${randomBytes(36).toString('base64url')}\n`, { mode: 0o600 });
  chmodSync(path, 0o600);
};
writeSecret('mysql_app_password.txt');
writeSecret('mysql_root_password.txt');

writeFileSync(envPath, [
  `DOMAIN=${values['public.domain']}`,
  `CERTBOT_EMAIL=${values['public.letsencrypt_email']}`,
  `MYSQL_DATABASE=${values['database.name']}`,
  `MYSQL_USER=${values['database.user']}`,
  `BACKUP_TIME=${values['backups.time']}`,
  `BACKUP_RETENTION_DAYS=${values['backups.retention_days']}`,
  `TZ=${values['backups.timezone']}`,
].join('\n') + '\n', { mode: 0o600 });
chmodSync(envPath, 0o600);

console.log(`Configuración preparada para ${values['instance.name']} (${values['public.domain']}).`);
console.log('Se generaron deploy/.env y las credenciales MySQL locales.');
