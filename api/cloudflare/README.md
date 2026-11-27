# Cloudflare 演示 API

本目录维护 Npora 在线 Vue/React 演示共用的 Cloudflare Worker API 和 D1 迁移。模板仓库只保留前端 API 契约及 MySQL 初始化/迁移 SQL。源码来源与后续同步约定见 [SOURCE.md](SOURCE.md)。

## 目录

| 目录 | 用途 |
| --- | --- |
| `src/` | HTTP 分发、业务处理、DTO、数据库访问和安全逻辑 |
| `migrations/` | D1/SQLite schema、种子和编号迁移，不适用于 MySQL |
| `tests/` | Worker API 与 SQLite 测试 |
| `integration/` | 完整前端请求联调 |
| `scripts/` | 初始化、打包和 smoke 脚本 |

## 本地检查

需要 Node.js 22.13 或更高版本（Node 24 LTS 推荐）。在此目录执行：

```sh
npm test
npm run build
```

这些命令只运行本地检查和构建，不会部署远程服务。`.local/`、`.dev.vars`、`.env`、`dist/` 和 `node_modules/` 不提交；演示密码、`AUTH_PEPPER` 及其他密钥不得写入 Git。

## 与演示站的关系

- Vue 展示站 `admin.npora.live` 通过 `vue/wrangler.jsonc` 的 `API` 服务绑定连接独立的 `npora-online-vue-api` Worker 和 `npora-online-vue-data` D1。候选构建从本目录复制 API 源码和迁移，并组合 Vue 专属的每日数据清理任务：

  ```sh
  VUE_D1_ID="<Vue D1 UUID>" AUTH_PEPPER="<32-byte hex secret>" \
    node vue/prepare-api.mjs
  ```

  不设置 `VUE_DEMO_PASSWORD` 时，种子管理员密码为 `123456`。候选配置中的清理基线应与编号迁移和首次种子保持一致。远程 D1 迁移、种子、Worker Secret 和 API 部署须分别核对目标与授权。
- React 展示站 `react.npora.live` 使用独立的 `npora-online-react-api` 服务和隔离 D1。候选构建从本目录复制 API 源码及迁移：

  ```sh
  mise exec node@24.22.0 -- node react/prepare-api.mjs
  ```

  生成目录下的 Wrangler 配置默认使用本地占位数据库 ID。线上 API 部署前，需在隔离环境中提供 `REACT_D1_ID`；数据库迁移、密钥配置、演示种子和 API 部署须分别核对目标与授权。
- React 的公开演示账号使用只读角色。`react/demo-api.mjs` 会拒绝业务写入；登录和退出会话所需请求除外。API 仍须独立校验会话与权限。

模板的共同 HTTP 契约以 Vue [`docs/api.md`](https://github.com/npora/admin-template/blob/main/docs/api.md)、[安全说明](https://github.com/npora/admin-template/blob/main/docs/authentication.md)、[RBAC 说明](https://github.com/npora/admin-template/blob/main/docs/rbac.md)和两模板 `database/mysql/` 为准。D1 migration 单独维护，不应用到 MySQL。

## 安全与发布

密码哈希、随机会话 Token、限流、TOTP、逐接口权限校验和审计写入都由后端负责。验证器密钥加密 pepper 必须作为 Cloudflare Secret 配置，不能放在前端、源码或示例 `.env` 中。

执行远程 D1 迁移或发布 Worker 前，核对服务名、数据库 UUID、环境、绑定和当前版本，并记录回滚版本。`prepare-api.mjs` 只生成本地候选，不部署。
