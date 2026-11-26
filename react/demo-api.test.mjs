import assert from 'node:assert/strict'
import test from 'node:test'
import { createDemoApi } from './demo-api.mjs'

test('allows reads and session lifecycle while denying data and account writes', async () => {
  const env = {}
  const api = createDemoApi({
    fetch(request, passedEnv) {
      assert.equal(passedEnv, env)
      return new Response(request.method, { status: 200 })
    }
  })
  for (const [method, path] of [
    ['GET', '/system/users'],
    ['HEAD', '/health'],
    ['OPTIONS', '/system/users'],
    ['POST', '/auth/login'],
    ['POST', '/auth/logout']
  ]) {
    assert.equal((await api.fetch(new Request(`https://api.example${path}`, { method }), env)).status, 200)
  }
  for (const [method, path] of [
    ['POST', '/auth/password/change'],
    ['POST', '/auth/totp/setup'],
    ['POST', '/system/users'],
    ['PATCH', '/system/users/1'],
    ['DELETE', '/system/users/1']
  ]) {
    const response = await api.fetch(new Request(`https://api.example${path}`, { method }), env)
    assert.equal(response.status, 403)
    assert.equal(response.headers.get('cache-control'), 'no-store')
  }
})
