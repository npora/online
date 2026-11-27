import { access, cp, mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const api = join(root, 'api/cloudflare')
const apiEntry = join(api, 'src/index.mjs')
const apiMigrations = join(api, 'migrations')
const cleanupWorker = join(root, 'vue/cleanup/worker.mjs')
try {
  await access(apiEntry)
  await access(apiMigrations)
  await access(cleanupWorker)
} catch {
  throw new Error('Cloudflare API source or Vue cleanup Worker is missing')
}

const database = 'npora-online-vue-data'
const localDatabaseId = '00000000-0000-0000-0000-000000000000'
const databaseId = process.env.VUE_D1_ID ?? localDatabaseId
if (!/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(databaseId)) {
  throw new Error('VUE_D1_ID must be the UUID of the separate Vue demo D1 database')
}

const pepper = process.env.AUTH_PEPPER
if (!/^[0-9a-f]{64}$/i.test(pepper ?? '')) {
  throw new Error('Provide a 32-byte hexadecimal AUTH_PEPPER to create the Vue demo admin seed')
}
const password = process.env.VUE_DEMO_PASSWORD ?? '123456'
const { hashPassword } = await import(pathToFileURL(join(api, 'src/security/crypto.mjs')))
const passwordHash = await hashPassword(password, pepper, 100_000, { minimumLength: 6 })

const baseline = {
  users: [1],
  dictItems: [1, 2],
  dictTypes: [1],
  menus: [1, 10, 11, 12, 13, 14, 15, 20, 21, 22, 23, 24, 25, 30, 31, 32, 33, 34, 40, 41, 42, 43, 44, 45, 46, 47, 48, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68],
  departments: [1, 2, 3],
  roles: [1, 2, 3]
}

const buildRoot = join(root, '.build')
await mkdir(buildRoot, { recursive: true })
const output = await mkdtemp(join(buildRoot, 'vue-api.'))
await cp(join(api, 'src'), join(output, 'src'), { recursive: true })
await cp(apiMigrations, join(output, 'migrations'), { recursive: true })
await cp(cleanupWorker, join(output, 'cleanup-worker.mjs'))

// D1 limits compound SELECTs. Convert the shared dictionary seed to VALUES rows.
const dictionaryPath = join(output, 'migrations/0005_dictionaries.sql')
const dictionarySql = await readFile(dictionaryPath, 'utf8')
const dictionaryButtons = /CROSS JOIN \(\n  SELECT ('[^']+' AS title,'[^']+' AS permission,\d+ AS sort_order)((?:\n  UNION ALL SELECT '[^']+','[^']+',\d+)+)\n\) AS button/
const dictionaryCompatible = dictionarySql.replace(dictionaryButtons, (_original, first, rest) => {
  const rows = [first.replace(/ AS (?:title|permission|sort_order)/g, ''),
    ...rest.trim().split('\n').map((line) => line.trim().slice('UNION ALL SELECT '.length))]
  return `CROSS JOIN (SELECT column1 AS title, column2 AS permission, column3 AS sort_order
  FROM (VALUES ${rows.map((row) => `(${row})`).join(', ')})) AS button`
})
await writeFile(dictionaryPath, dictionaryCompatible)

await writeFile(join(output, 'worker.mjs'), `import api from './src/index.mjs'
import cleanup from './cleanup-worker.mjs'

export default {
  fetch(request, env) {
    return api.fetch(request, env)
  },
  scheduled(event, env, context) {
    return cleanup.scheduled(event, env, context)
  }
}
`)
await writeFile(join(output, 'wrangler.jsonc'), JSON.stringify({
  name: 'npora-online-vue-api',
  main: 'worker.mjs',
  compatibility_date: '2026-10-30',
  workers_dev: false,
  preview_urls: false,
  triggers: { crons: ['0 16 * * *'] },
  d1_databases: [{ binding: 'DB', database_name: database, database_id: databaseId }],
  vars: {
    AUTH_DEMO_PROTECTED_USERNAME: 'admin',
    AUTH_TOTP_ENABLED: 'false',
    DEMO_CLEANUP_ENABLED: 'true',
    DEMO_CLEANUP_BASELINE: JSON.stringify(baseline)
  },
  secrets: { required: ['AUTH_PEPPER'] },
  observability: { enabled: true, traces: { enabled: true } }
}, null, 2) + '\n')

await writeFile(join(output, 'demo-seed.sql'), `-- Execute once after numbered migrations. The password is hashed with AUTH_PEPPER.
INSERT INTO sys_user(department_id, username, password_hash, display_name, is_super_admin)
VALUES (2, 'admin', '${passwordHash}', '示例管理员', 1);
INSERT INTO sys_user_role(user_id, role_id)
SELECT id, 1 FROM sys_user WHERE username = 'admin';
`, { mode: 0o600, flag: 'wx' })

console.log(`Vue API candidate: ${output}`)
