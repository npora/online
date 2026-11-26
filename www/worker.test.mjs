import assert from "node:assert/strict";
import test from "node:test";
import worker from "./worker.mjs";

test("passes only the exact API path boundary to the Request console", async (t) => {
  const assetRoutes = [];
  const env = {
    ASSETS: {
      fetch(request) {
        assetRoutes.push(new URL(request.url).pathname);
        return new Response("asset");
      },
    },
  };
  const apiRoutes = [];
  t.mock.method(globalThis, "fetch", async (request) => {
    apiRoutes.push(request.url);
    return new Response("api");
  });

  for (const path of ["/", "/styles.css", "/apix", "/api-example"]) {
    assert.equal(await (await worker.fetch(new Request("https://www.npora.live" + path), env)).text(), "asset");
  }
  for (const path of ["/api", "/api/", "/api/user?source=old"]) {
    assert.equal(await (await worker.fetch(new Request("https://www.npora.live" + path), env)).text(), "api");
  }
  assert.deepEqual(assetRoutes, ["/", "/styles.css", "/apix", "/api-example"]);
  assert.deepEqual(apiRoutes, [
    "https://request.npora.live/api",
    "https://request.npora.live/api/",
    "https://request.npora.live/api/user?source=old",
  ]);
});

test("preserves API query, method, request body, response status, headers, and stream", async (t) => {
  const responseStream = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode("data: one\n\n"));
      controller.enqueue(new TextEncoder().encode("data: two\n\n"));
      controller.close();
    },
  });
  t.mock.method(globalThis, "fetch", async (request, options) => {
    assert.equal(request.url, "https://request.npora.live/api/query?limit=3");
    assert.equal(request.method, "QUERY");
    assert.equal(request.headers.get("Content-Type"), "application/json");
    assert.deepEqual(await request.json(), { query: "active" });
    assert.equal(options.redirect, "manual");
    return new Response(responseStream, {
      status: 429,
      headers: { "Content-Type": "text/event-stream", "Retry-After": "5" },
    });
  });
  const request = new Request("https://www.npora.live/api/query?limit=3", {
    method: "QUERY",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query: "active" }),
  });
  const response = await worker.fetch(request);
  assert.equal(response.status, 429);
  assert.equal(response.headers.get("Retry-After"), "5");
  assert.equal(response.headers.get("Content-Type"), "text/event-stream");
  assert.strictEqual(response.body, responseStream);
  assert.equal(await response.text(), "data: one\n\ndata: two\n\n");
});
