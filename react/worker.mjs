const apiPrefix = '/api'
const forwardedHeaders = ['accept', 'authorization', 'content-type', 'cf-connecting-ip']

export default {
  async fetch(request, env) {
    const url = new URL(request.url)
    if (url.pathname !== apiPrefix && !url.pathname.startsWith(`${apiPrefix}/`)) {
      return env.ASSETS.fetch(request)
    }

    const target = new URL('https://api.internal')
    target.pathname = url.pathname.slice(apiPrefix.length) || '/'
    target.search = url.search

    const headers = new Headers()
    for (const key of forwardedHeaders) {
      const value = request.headers.get(key)
      if (value !== null) headers.set(key, value)
    }

    // Preserve cross-origin requests for the API service's origin checks.
    const origin = request.headers.get('origin')
    if (origin !== null && origin !== url.origin) {
      headers.set('origin', origin)
      for (const key of ['access-control-request-method', 'access-control-request-headers']) {
        const value = request.headers.get(key)
        if (value !== null) headers.set(key, value)
      }
    }

    try {
      const upstreamRequest = new Request(new Request(target, request), {
        headers,
        redirect: 'manual'
      })
      const upstream = await env.API.fetch(upstreamRequest)
      const response = new Response(upstream.body, upstream)
      response.headers.set('Cache-Control', 'no-store')
      return response
    } catch {
      console.error(JSON.stringify({ event: 'api_proxy_failed' }))
      return Response.json(
        { code: 502, message: 'API service unavailable', data: null },
        { status: 502, headers: { 'Cache-Control': 'no-store' } }
      )
    }
  }
}
