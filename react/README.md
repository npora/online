# React 后台演示

计划域名为 `react.npora.live`。`source.env` 固定 React 模板仓库的完整提交。
应用、业务接口契约和依赖锁由模板仓库维护，演示运行配置由此目录负责。

## 构建和预览

从线上站点仓库根目录运行：

```sh
bash scripts/build-site.sh react
node --test react/worker.test.mjs
```

生产静态文件位于构建脚本报告的候选源码目录下的 `dist/`。只预览静态界面时，
在候选源码目录运行 `npm run preview`。登录和管理操作须通过绑定隔离后端的预览环境验收。

## Cloudflare 接入

`wrangler.jsonc` 声明 Worker、`react.npora.live` 自定义域名、`ASSETS` 静态绑定和 SPA 回退。
在仓库根目录把构建脚本报告的候选目录传给部署脚本：

```sh
bash react/deploy.sh "<候选源码目录>"
```

默认执行 dry-run，使用固定版本 Wrangler。实际部署须获授权，再追加 `--deploy`。
`API` 服务绑定指向独立的 `npora-online-react-api`，首次部署前须在同一账户准备该
后端 Worker，并确认其绑定的是 React 的隔离演示数据库。
[Cloudflare 静态资产路由](https://developers.cloudflare.com/workers/static-assets/routing/worker-script/)
· [Wrangler 自定义域名配置](https://developers.cloudflare.com/workers/wrangler/configuration/#custom-domains)

Worker 去掉同源请求的 `/api` 前缀，保留请求方法、查询和请求体。
跨源请求的 Origin 与预检字段交给后端校验。API 响应不缓存。
配置已启用 Workers 日志和跟踪。上线前验证登录、下发菜单、权限拒绝、刷新深层路由、
独立演示数据和 PWA 更新提示。记录验收通过的提交和部署版本，回滚时按该记录恢复。

## 独立演示 API

使用同一份 Vue 参考后端实现和编号迁移。在仓库根目录构建 Vue 候选后，
传入构建脚本报告的 Vue 候选目录。脚本核对锁定的提交，然后将 API 源码和
迁移复制到独立的 `.build/react-api.*` 目录，输出可供 Wrangler 使用的配置。
候选目录中的字典种子用等价的 `VALUES` 处理 D1 对 `UNION ALL` 项数的限制。

```sh
node react/prepare-api.mjs "<Vue 候选源码目录>"
npx --yes wrangler@4.154.0 deploy --config "<API 候选目录>/wrangler.jsonc" --dry-run --no-autoconfig
npx --yes wrangler@4.154.0 d1 migrations apply npora-online-react-data \
  --config "<API 候选目录>/wrangler.jsonc" --local
```

- 省略 `REACT_D1_ID` 时，候选配置使用仅供本地模拟的占位 ID。线上配置需要独立
  `npora-online-react-data` 数据库的实际 UUID。React API Worker 通过服务绑定访问，
  本身没有自定义域名、公开 workers.dev 地址或版本预览地址。
- 首次初始化演示数据时，在受保护的环境中向生成脚本同时提供 `AUTH_PEPPER`
  和 `REACT_DEMO_PASSWORD`。前者是 React 专用的 32 字节随机十六进制 Secret，
  后者须与 React 演示登录页公告的公开密码一致。脚本将密码哈希写入权限为
  `0600` 的 `demo-seed.sql`。Worker 运行时通过平台 Secret 读取同一 pepper。
- 编号迁移应用后，再用 `wrangler d1 execute npora-online-react-data --config
  "<API 候选目录>/wrangler.jsonc" --local --file "<API 候选目录>/demo-seed.sql"`
  初始化本地演示用户。指定了 Wrangler 本地持久化目录时，迁移和初始化须使用同一目录。
- 公开演示用户 `admin` 实际使用只读角色，非超级管理员。API 另有方法限制，
  除登录和退出所需的会话写入外，拒绝业务及账户写入。后端的鉴权、会话和权限判断
  仍由引用的 Vue 参考后端完成。
- 线上数据库创建、远程迁移、导入种子、绑定 Secret、后端及前端部署分别按授权执行。
  先确认独立数据库和后端候选，再部署前端并验证同源登录、菜单、读取及写入拒绝。

[Cloudflare D1 迁移](https://developers.cloudflare.com/d1/reference/migrations/)
