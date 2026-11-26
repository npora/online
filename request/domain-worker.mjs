export default {
  async fetch(request) {
    const target = new URL(request.url);
    target.host = "npora.pages.dev";
    try {
      return await fetch(new Request(target, request), { redirect: "manual" });
    } catch {
      return Response.json(
        { error: "Request demo is temporarily unavailable." },
        { status: 502, headers: { "Cache-Control": "no-store" } },
      );
    }
  },
};
