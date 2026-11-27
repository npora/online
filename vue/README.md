# Vue 后台演示

线上演示入口为 [admin.npora.live](https://admin.npora.live)，也可从[官网](https://www.npora.live/)的“演示”菜单进入。`source.env` 固定演示所用的 [Vue 模板源码](https://github.com/npora/admin-template)提交。模板仓库保留纯净后台模板；本目录负责候选构建、Cloudflare Worker、同源 API 绑定和演示数据清理。

## 构建候选版本

在 `online-npora` 根目录执行：

```sh
bash scripts/build-site.sh vue
```

脚本按 `source.env` 检出固定提交，在隔离目录安装锁文件依赖，设置演示使用的 `/api` 前缀并运行模板的标准 Vite `build`。数据库文档随产物复制到 `public/database`，之后运行 Vue 演示清理逻辑的本地回归。输出中的候选目录可传给部署脚本；`admin-template` 不需要也不应提供 Cloudflare 专用构建脚本。

## Cloudflare 预览与部署

`wrangler.jsonc` 和 `worker.mjs` 是本目录的部署入口。Worker 将 `/api` 和 `/api/*` 转发到 `API` 服务绑定，其他请求由静态资产处理并支持 SPA 深链接。当前绑定目标沿用 Cloudflare 中已配置的 `npora-admin-api-test`；正式部署前核对它仍是预期的隔离演示 API。

默认只执行 Wrangler dry-run：

```sh
bash scripts/build-site.sh vue
bash vue/deploy.sh "<Candidate checkout>"
```

确认预览产物、API 绑定、域名和回滚版本后，显式部署：

```sh
bash vue/deploy.sh "<Candidate checkout>" --deploy
```

部署目标为现有 Worker `admin-demo` 和自定义域 `admin.npora.live`。Cloudflare Dashboard 的 Git 集成若仍直接构建 `npora/admin-template`，应在完成新的候选验证后，将构建来源切换到 `npora/online` 的 Vue 部署流程，或停用旧集成；模板仓库不再包含 `build:cloudflare`。不要让旧 Git 集成和本部署脚本同时发布同一 Worker。

## 验收与回滚

- 用同一视口检查登录、权限、SPA 深链接及刷新、静态资源、PWA 更新和 `/api/health`。
- 核对 Cloudflare Worker 的 API 服务绑定、`admin.npora.live` 自定义域以及演示数据库归属。
- 清理 Worker 的源码在 [`cleanup/worker.mjs`](cleanup/worker.mjs)。生产 Cron 与 D1 绑定沿用现行部署；切换前确认同一时段只有一个清理任务运行。
- 上线前记录当前 Worker 版本和构建提交。冒烟失败时恢复之前的 Worker 版本，再检查域名、深链接和 API。

清理逻辑本地回归：

```sh
NPORA_VUE_SOURCE="<Candidate checkout>" node --test vue/cleanup/test/worker.test.mjs
```
