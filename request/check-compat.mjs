import assert from 'node:assert/strict'

const [expectedVersion, primarySite, ...legacySites] = process.argv.slice(2)
if (!expectedVersion || !primarySite) {
  console.error('Usage: node request/check-compat.mjs <version> <request-origin> [<legacy-origin>...]')
  process.exit(2)
}

async function verifySite(site, isPrimary) {
  const origin = new URL(site)
  assert.ok(['https:', 'http:'].includes(origin.protocol), 'Expected an HTTP(S) origin')
  assert.equal(origin.href, origin.origin + '/', 'Pass a site origin only')

  async function read(path, options, status = 200) {
    const response = await fetch(new URL(path, origin), {
      ...options,
      redirect: 'manual',
      signal: AbortSignal.timeout(15000)
    })
    assert.equal(response.status, status, `${origin.origin}${path}: expected direct ${status}`)
    return response
  }

  if (isPrimary) {
    const html = await (await read('/')).text()
    assert.match(html, /\bid="playground"/, 'Playground anchor must be available')
    const { version } = await (await read('/build.json')).json()
    assert.equal(version, expectedVersion, 'Unexpected library build version')
  }
  assert.equal((await (await read('/api/health')).json()).ready, true, 'State service is not ready')

  const wrongMethod = await read('/api/query', undefined, 405)
  assert.equal(wrongMethod.headers.get('allow'), 'QUERY')
  await wrongMethod.body?.cancel()

  const { id } = await (await read('/api/user')).json()
  assert.equal(id, 1, 'Unexpected demo user')
  const filters = { profile: { team: 'Npora' } }
  const query = await (await read('/api/query', {
    method: 'QUERY',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(filters)
  })).json()
  assert.equal(query.method, 'QUERY')
  assert.deepEqual(query.filters, filters)

  const upload = new FormData()
  upload.append('file', new Blob(['Npora preview']), 'preview.txt')
  const stored = await (await read('/api/upload', { method: 'POST', body: upload })).json()
  assert.equal(stored.name, 'preview.txt')
  assert.equal(stored.bytes, 13)

  const sse = await read('/api/sse')
  assert.match(sse.headers.get('content-type') ?? '', /^text\/event-stream\b/)
  const events = (await sse.text()).trim().split(/\r?\n\r?\n/)
  assert.equal(events.length, 3)
  for (const [i, event] of events.entries()) {
    assert.match(event, new RegExp(`^id: ${i + 1}\\r?\\nevent: update\\r?\\ndata: `))
  }
  const verified = isPrimary ? `playground, version ${expectedVersion}, ` : ''
  console.log(`${origin.origin}: ${verified}binding, QUERY, POST and SSE passed`)
}

await verifySite(primarySite, true)
for (const site of legacySites) await verifySite(site, false)
