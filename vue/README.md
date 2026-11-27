# Vue 后台演示

线上演示入口为 [admin.npora.live](https://admin.npora.live)，也可从[官网](https://www.npora.live/)的“演示”菜单进入。Go / Node 后端接入指南在 [admin.npora.live/api-guide/](https://admin.npora.live/api-guide/)。`source.env` 固定演示所用的 [Vue 模板源码](https://github.com/npora/admin-template)提交。模板仓库保留纯净后台模板；本目录负责候选构建、Cloudflare Worker、同源 API 绑定和演示数据清理。

## 构建候选版本

在 `online-npora` 根目录执行：

```sh
bash scripts/build-site.sh vue
```

脚本按 `source.env` 检出固定提交，在隔离目录安装锁文件依赖，设置演示使用的 `/api` 前缀并运行模板的标准 Vite `build`。数据库文档和共用 API 指南分别复制到 `public/database`、`public/api-guide`，之后运行 Vue 演示清理逻辑的本地回归。输出中的候选目录可传给部署脚本；`admin-template` 不需要也不应提供 Cloudflare 专用构建脚本。

## Cloudflare 预览与部署

`wrangler.jsonc` 和 `worker.mjs` 是前端部署入口。Worker 将 `/api` 和 `/api/*` 转发到 `API` 服务绑定，其他请求由静态资产处理并支持 SPA 深链接。Vue 使用独立的 `npora-online-vue-api` Worker 和 `npora-online-vue-data` D1；不再依赖 `npora-admin-api-test`。前端仍由 `admin-demo` Worker 绑定 `admin.npora.live`，以便保持现有访问域名。

Vue/React 演示 API 的 Cloudflare Worker 实现及 D1 编号迁移统一维护在 [`../api/cloudflare/`](../api/cloudflare/)。两套演示分别部署 API Worker、使用独立 D1。D1 迁移与 Vue/React 模板的 MySQL SQL 是不同数据库版本，不能互相执行。

### 部署独立 API

首次部署前，在 Cloudflare 创建空数据库 `npora-online-vue-data`，并准备 32 字节随机 `AUTH_PEPPER`。不要把 pepper 写入仓库或 shell 历史。准备候选时通过环境变量传入数据库 UUID 和 pepper；不设置 `VUE_DEMO_PASSWORD` 时，种子账户为 `admin` / `123456`。

```sh
VUE_D1_ID="<Vue D1 UUID>" AUTH_PEPPER="<32-byte hex secret>" \
  node vue/prepare-api.mjs
```

对输出目录执行编号迁移和演示管理员种子；确认 Worker 配置中的数据库 UUID 与目标数据库一致。之后向 `npora-online-vue-api` 设置同一个 `AUTH_PEPPER` Secret，再部署该 API Worker。部署候选默认只做 dry-run，确认后再显式部署：

```sh
npx --yes wrangler@4.154.0 d1 migrations apply npora-online-vue-data \
  --remote --config "<API 候选目录>/wrangler.jsonc"
npx --yes wrangler@4.154.0 d1 execute npora-online-vue-data \
  --remote --config "<API 候选目录>/wrangler.jsonc" --file "<API 候选目录>/demo-seed.sql"
bash vue/deploy-api.sh "<API 候选目录>"
bash vue/deploy-api.sh "<API 候选目录>" --deploy
```

部署脚本默认只做 dry-run。实际部署时，向候选生成过程和部署脚本提供同一个 `AUTH_PEPPER` 环境变量；脚本会用临时权限为 `600` 的 secrets 文件，并在退出时删除。`vue/cleanup/worker.mjs` 作为 API Worker 的 Cron handler 运行，每日 UTC 16:00 将演示数据恢复到编号迁移和管理员种子定义的基线。API Worker 配置中带有对应的基线 ID；新增初始化记录时须同步更新它。旧 Worker 若仍有清理 Cron，切换前先确认并停用重复任务。

### 部署前端

默认只执行 Wrangler dry-run：

```sh
bash scripts/build-site.sh vue
bash vue/deploy.sh "<Candidate checkout>"
```

确认候选产物、`npora-online-vue-api` 服务绑定、域名和回滚版本后，显式部署：

```sh
bash vue/deploy.sh "<Candidate checkout>" --deploy
```

部署目标为前端 Worker `admin-demo` 和自定义域 `admin.npora.live`。Cloudflare Dashboard 的 Git 集成若仍直接构建 `npora/admin-template`，应将构建来源切换到 `npora/online` 的 Vue 部署流程，或停用旧集成；模板仓库不再包含 `build:cloudflare`。不要让旧 Git 集成和本部署脚本同时发布同一 Worker。

## 验收与回滚

- 用同一视口检查登录、权限、SPA 深链接及刷新、静态资源、PWA 更新和 `/api/health`。
- 核对前端 Worker 的 API 服务绑定、`admin.npora.live` 自定义域，以及 API Worker 的 `npora-online-vue-data` D1 绑定。
- 清理 Cron 与 API 使用同一 Vue D1；确认只有新 API Worker 启用清理任务。
- 上线前记录当前 Worker 版本和构建提交。冒烟失败时恢复之前的 Worker 版本，再检查域名、深链接和 API。

清理逻辑本地回归：

```sh
node --test vue/cleanup/test/worker.test.mjs
```

清理测试使用 [`api/cloudflare/migrations/`](../api/cloudflare/migrations/) 中的在线 API D1 迁移创建内存数据库，不再维护重复的 SQL 副本。更新 D1 schema 后，运行清理回归确认 fixture 仍兼容；测试 helper 仅供本地测试，不会操作 Cloudflare 数据库。
