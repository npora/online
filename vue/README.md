# Vue 后台演示

线上入口为 [admin.npora.live](https://admin.npora.live)。`source.env` 固定演示使用的 [Vue 模板](https://github.com/npora/admin-template) 提交。模板负责后台页面、接口契约和可生成预设。本目录负责引用版本、预览验收和部署接管。

## 构建和预览

使用 Node.js 24.22.0 或更新的 24.x。在本仓库根目录执行 `bash scripts/build-site.sh vue`，构建脚本在独立检出目录安装锁定的依赖，并调用模板的 `build:cloudflare` 生成候选产物。修改版本引用时，先确认对应提交已在上游，再记录构建产物和实际验收版本。

在脚本输出的候选检出目录执行 `npm run dev:demo`，用本地开发服务预览页面。需要验证完整部署产物时，在候选检出目录按模板部署说明执行 `npm run check:cloudflare`。Cloudflare 预览使用候选 Worker 名称和隔离演示数据，可在构建时用 `CLOUDFLARE_API_WORKER` 指定对应的测试 API 服务。操作仅针对已授权的环境。

## 验收和接管

- 验证登录、权限边界、直接打开和刷新后台路由、静态资源及 PWA 更新。核对同源 `/api` 和 `/api/*` 走固定 API 服务绑定，保留路径、查询和业务响应语义。
- 保留现有域名。接管前核对当前部署来源、域名、API 绑定、演示数据库和可回滚版本。候选部署验收后按授权切换正式入口，并复查线上首页和接口。
- 记录访客数据清理的实际归属、运行频率和保留基线。接管前后都由单一部署执行清理，使用隔离数据验证清理与官方数据保留。
- Vue 模板的功能、配置、API 或迁移发生变化时，同一任务核对本目录相应的版本、绑定和验收场景。切换前继续使用现有发布来源。

[模板 Cloudflare 部署说明](https://github.com/npora/admin-template/blob/f87ee58872ce81d44e13d19a64eb6712ee34737a/deploy/cloudflare/README.md)

## 生产接管和回滚

生产继续使用现有前端 Worker `admin-demo`，连同 `admin.npora.live` 自定义域。源仓库的 `cloudflare.config.ts` 仅在生产 Worker 名称下附加该域。候选构建使用独立 Worker 名称，生产构建通过下面的命令重新生成产物。

```sh
bash scripts/build-site.sh vue
# 将以下环境变量设置为当前线上 API Worker 的实际名称。
export CLOUDFLARE_API_WORKER='VERIFIED_API_WORKER'
bash vue/deploy.sh check "<Vue candidate checkout>"
bash vue/deploy.sh deploy "<Vue candidate checkout>"
```

执行前核对当前线上 Worker 的 `API_PREFIX` 和 `API` 服务绑定。源配置默认 API 服务为 `npora-admin-api-test`，脚本要求显式传入当天已核对的服务名。当前前端 Worker 入口只处理请求转发。清理脚本是独立入口，须核对原有 Cron 和数据库绑定的部署归属。以前端接管为范围时，API 服务和清理任务沿用当前部署。

生产接管前记录当前发布版本和 Git 构建来源，先在候选环境验证。生产冒烟失败时，恢复此前记录的 Worker 版本，并复查域名、SPA 深链接和 `/api/health`。若问题来自自动发布来源切换，从原仓库固定已知可用版本重新构建，恢复原 Git 触发配置。回滚前端不执行数据库迁移。
