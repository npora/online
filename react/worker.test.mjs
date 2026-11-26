import assert from 'node:assert/strict'
import test from 'node:test'
import worker from './worker.mjs'

const site = 'https://react.example.com'

test('keeps the API destination fixed while forwarding method, query and body', async () => {
  const request = new Request(`${site}/api//different.example/users?active=1`, {
    method: 'POST',
    headers: {
      origin: site,
      authorization: 'Bearer fixture',
      'content-type': 'application/json',
      cookie: 'private=fixture'
    },
    body: '{"name":"fixture"}'
  })
  const response = await worker.fetch(request, {
    API: {
      async fetch(upstream) {
        assert.equal(upstream.url, 'https://api.internal//different.example/users?active=1')
        assert.equal(upstream.method, 'POST')
        assert.equal(upstream.redirect, 'manual')
        assert.equal(upstream.headers.get('authorization'), 'Bearer fixture')
        assert.equal(upstream.headers.get('origin'), null)
        assert.equal(upstream.headers.get('cookie'), null)
        assert.deepEqual(await upstream.json(), { name: 'fixture' })
        return new Response('created', { status: 201 })
      }
    }
  })
  assert.equal(response.status, 201)
  assert.equal(response.headers.get('cache-control'), 'no-store')
  assert.equal(await response.text(), 'created')
})

test('retains cross-origin preflight metadata for the API service', async () => {
  const request = new Request(`${site}/api`, {
    method: 'OPTIONS',
    headers: {
      origin: 'https://external.example',
      'access-control-request-method': 'POST',
      'access-control-request-headers': 'authorization'
    }
  })
  const response = await worker.fetch(request, {
    API: {
      fetch(upstream) {
        assert.equal(upstream.url, 'https://api.internal/')
        assert.equal(upstream.headers.get('origin'), 'https://external.example')
        assert.equal(upstream.headers.get('access-control-request-method'), 'POST')
        assert.equal(upstream.headers.get('access-control-request-headers'), 'authorization')
        return new Response('Forbidden', { status: 403 })
      }
    }
  })
  assert.equal(response.status, 403)
})

test('delegates deep links and paths outside the API boundary to static assets', async () => {
  for (const path of ['/system/user', '/api-v2', '/assets/app.js']) {
    const request = new Request(`${site}${path}`)
    const response = await worker.fetch(request, {
      ASSETS: {
        fetch(incoming) {
          assert.equal(incoming, request)
          return new Response('static')
        }
      }
    })
    assert.equal(await response.text(), 'static')
  }
})
