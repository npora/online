import { mkdir, readFile, copyFile, writeFile, rm } from 'node:fs/promises'
import { execFileSync } from 'node:child_process'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'esbuild'

const root = new URL('./', import.meta.url)
const output = new URL('./dist/', root)
const sourceDir = process.env.REQUEST_SOURCE_DIR
if (!sourceDir) throw new Error('Set REQUEST_SOURCE_DIR to the pinned request checkout')
const pin = await readFile(new URL('../source.env', root), 'utf8')
const revision = /^SOURCE_REV=([0-9a-f]{40})$/m.exec(pin)?.[1]
const actualRevision = execFileSync('git', ['-C', sourceDir, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
if (!revision || actualRevision !== revision) {
  throw new Error('REQUEST_SOURCE_DIR must match the Git revision in request/source.env')
}
const { name, version } = JSON.parse(await readFile(resolve(sourceDir, 'package.json'), 'utf8'))
if (name !== '@npora/request' || typeof version !== 'string') {
  throw new Error('REQUEST_SOURCE_DIR must point to the @npora/request source')
}
await rm(output, { recursive: true, force: true })
await mkdir(output, { recursive: true })
// Explicit allowlist: never upload functions, tooling, credentials or source maps.
for (const name of ['index.html', 'style.css', 'favicon.svg', '_headers', '_routes.json', '404.html']) {
  await copyFile(new URL(name, root), new URL(name, output))
}
await build({ entryPoints: {
    app: fileURLToPath(new URL('app.js', root)),
    'not-found': fileURLToPath(new URL('not-found.js', root))
  }, outdir: fileURLToPath(output), bundle: true,
  platform: 'browser', format: 'esm', target: 'es2022', minify: true,
  alias: { '@request-source': resolve(sourceDir, 'src') },
  define: { __VERSION__: JSON.stringify(version) } })
await writeFile(new URL('build.json', output), JSON.stringify({ version, sourceRevision: revision }))
console.log(`Website v${version} built in website/dist`)
