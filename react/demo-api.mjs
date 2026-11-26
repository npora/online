const readMethods = new Set(['GET', 'HEAD', 'OPTIONS'])
const sessionWrites = new Set(['/auth/login', '/auth/logout'])

export function createDemoApi(api) {
  return {
    fetch(request, env) {
      const path = new URL(request.url).pathname
      if (readMethods.has(request.method) ||
          (request.method === 'POST' && sessionWrites.has(path))) {
        return api.fetch(request, env)
      }
      return Response.json(
        { code: 403, message: 'This public demo is read-only', data: null },
        { status: 403, headers: { 'Cache-Control': 'no-store' } }
      )
    }
  }
}
