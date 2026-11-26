export default {
  async fetch(request, env) {
    const upstream = new URL(request.url);
    if (upstream.pathname !== "/api" && !upstream.pathname.startsWith("/api/")) {
      return env.ASSETS.fetch(request);
    }

    upstream.protocol = "https:";
    upstream.host = "request.npora.live";
    upstream.port = "";

    try {
      return await fetch(new Request(upstream, request), { redirect: "manual" });
    } catch {
      return Response.json(
        { error: "Request demo API is temporarily unavailable." },
        { status: 502, headers: { "Cache-Control": "no-store" } },
      );
    }
  },
};
