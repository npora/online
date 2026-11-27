import { access, cp, mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const api = join(root, 'api/cloudflare')
const apiEntry = join(api, 'src/index.mjs')
const apiMigrations = join(api, 'migrations')
try {
  await access(apiEntry)
  await access(apiMigrations)
} catch {
  throw new Error('Cloudflare demo API source is missing from api/cloudflare')
}

const database = 'npora-online-react-data'
const localDatabaseId = '00000000-0000-0000-0000-000000000000'
const databaseId = process.env.REACT_D1_ID ?? localDatabaseId
if (!/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(databaseId)) {
  throw new Error('REACT_D1_ID must be the UUID of the separate React demo D1 database')
}

const pepper = process.env.AUTH_PEPPER
const password = process.env.REACT_DEMO_PASSWORD
if ((pepper === undefined) !== (password === undefined)) {
  throw new Error('Provide both AUTH_PEPPER and REACT_DEMO_PASSWORD to generate a seed')
}
let passwordHash
if (password !== undefined) {
  const { hashPassword } = await import(pathToFileURL(join(api, 'src/security/crypto.mjs')))
  passwordHash = await hashPassword(password, pepper, 100_000, { minimumLength: 6 })
}

const buildRoot = join(root, '.build')
await mkdir(buildRoot, { recursive: true })
const output = await mkdtemp(join(buildRoot, 'react-api.'))
await cp(join(api, 'src'), join(output, 'src'), { recursive: true })
await cp(apiMigrations, join(output, 'migrations'), { recursive: true })
// D1 limits compound SELECTs. The pinned dictionary seed has eight UNION terms.
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

await cp(new URL('./demo-api.mjs', import.meta.url), join(output, 'demo-api.mjs'))
await writeFile(join(output, 'worker.mjs'), `import api from './src/index.mjs'
import { createDemoApi } from './demo-api.mjs'

export default createDemoApi(api)
`)
await writeFile(join(output, 'wrangler.jsonc'), JSON.stringify({
  name: 'npora-online-react-api',
  main: 'worker.mjs',
  compatibility_date: '2026-10-30',
  workers_dev: false,
  preview_urls: false,
  d1_databases: [{ binding: 'DB', database_name: database, database_id: databaseId }],
  vars: {
    AUTH_DEMO_PROTECTED_USERNAME: 'admin',
    AUTH_TOTP_ENABLED: 'false'
  },
  secrets: { required: ['AUTH_PEPPER'] },
  observability: { enabled: true, traces: { enabled: true } }
}, null, 2) + '\n')

if (passwordHash !== undefined) {
  await writeFile(join(output, 'demo-seed.sql'), `-- Execute once after the numbered migrations.
INSERT INTO sys_role(code, name, description, sort_order)
VALUES ('react-demo-reader', '演示访客', 'React 演示的只读访问权限', 100);

INSERT INTO sys_role_menu(role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.code = 'react-demo-reader' AND m.deleted_at IS NULL
  AND (m.type IN (1, 2) OR m.permission LIKE '%:list' OR m.permission = 'system:download:download');

INSERT INTO sys_user(department_id, username, password_hash, display_name, is_super_admin)
VALUES (2, 'admin', '${passwordHash}', '演示访客', 0);

INSERT INTO sys_user_role(user_id, role_id)
SELECT u.id, r.id FROM sys_user u CROSS JOIN sys_role r
WHERE u.username = 'admin' AND r.code = 'react-demo-reader';
`, { mode: 0o600, flag: 'wx' })
}

console.log(`React API candidate: ${output}`)
