# 请求库展示站

本站介绍 `@npora/request` 并提供可运行的请求控制台。目标地址为 `request.npora.live`。构建使用 [source.env](source.env) 固定的源仓库提交，保留同一提交的站点、库源码、版本标记和站点锁文件。请求库发布仍由 [npora/request](https://github.com/npora/request) 管理。

## 构建与预览

在 `online-npora` 根目录运行以下命令。第二步使用构建脚本输出的候选检出路径。

```sh
bash scripts/build-site.sh request
bash request/pages.sh check .build/request.EXAMPLE
bash request/pages.sh preview .build/request.EXAMPLE
node request/check-compat.mjs 1.23.4 http://127.0.0.1:8787
```

本地预览启动 Pages Functions 和本地状态 Worker。版本参数取候选检出的 `website/dist/build.json`。预检包含 Functions 构建及状态 Worker dry-run。静态产物保留 `_headers`、`_routes.json`、`404.html` 和 `build.json`，Pages 从候选检出的 `website/` 读取 `functions/` 和 `wrangler.jsonc`。

## 部署与旧入口兼容

获准部署后，使用已核对的 Pages 项目和分支，先发预览分支，再按授权发布生产分支。对保留的 Git 集成项目，接管时先协调源仓库自动构建触发设置，避免两条流程覆盖同站点 release。生产继续核对 `DEMO_STATE` 连接 `npora-request-demo-state` Worker 的 `DemoState` 类及现有 SQLite 类迁移状态，预览使用独立状态。 [Cloudflare Git 集成说明](https://developers.cloudflare.com/pages/configuration/git-integration/#disable-automatic-deployments) · [上游部署指南](https://github.com/npora/request/blob/main/docs/cloudflare-website.md)

```sh
bash request/pages.sh deploy .build/request.EXAMPLE npora request-migration
node request/check-compat.mjs 1.23.4 https://request.npora.live https://www.npora.live
```

`request.npora.live` 使用本目录的 Worker，自定义域名转发到固定的 Pages 源站 `npora.pages.dev`，保留 Functions 和状态绑定，`www.npora.live` 由总官网接管。旧 `www.npora.live/#playground` 由官网页面读取浏览器片段，跳到请求库同名锚点。旧 `www.npora.live/api` 与 `/api/*` 由官网 Worker 转发到固定的请求库域名，保留 HTTP 方法、请求体、状态、响应头和响应流。兼容脚本在主站验证页面版本与锚点，在每个给定域名验证状态绑定、非 GET 请求体和 SSE。

## 迁移验收

- 在源检出运行 `pnpm test:website` 和 `pnpm test:website:browser`，依次执行会重建同一 `dist` 的命令。预览核对页面版本、双语、404 和十个控制台场景，包括状态相关的重试、刷新和限流，以及 SSE、NDJSON、上传和下载。
- 保留 `www.npora.live/#playground` 及旧 `/api` 示例的访问。切换前验证旧入口可达和控制台导航，逐一核对旧 API 路径的方法、请求体、响应状态、响应头和流式数据。兼容方案需覆盖非 GET 请求，页面跳转和 API 行为分别验证。
- 切换前记录 Pages 与状态 Worker 的可回滚版本。预览通过后才切换域名和发布入口，最终核对实际域名、`build.json` 版本和 API 行为，再更新官网及请求库文档。
