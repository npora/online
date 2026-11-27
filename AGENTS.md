# Npora 线上站点协作约定

工作区中的上级 AGENTS.md 定义跨项目分派、联动和验收。本仓库独立克隆时也按以下约定工作。

| 目录 | 职责 | 关联仓库 |
| --- | --- | --- |
| `www/` | 总官网、项目介绍、安装入口和体验导航 | 官网负责人 |
| `vue/` | Vue 后台演示部署、同源 API、演示数据配置 | `npora/admin-template` |
| `react/` | React 后台演示部署、同源 API、演示数据配置 | `npora/admin-template-react` |
| `request/` | 请求库介绍、交互控制台、演示 API 与状态服务部署 | `npora/request` |
| `api/cloudflare/` | Vue/React 在线演示共用的 Cloudflare Worker API 与 D1 迁移 | Vue/React 模板接口契约；D1 SQL 与 MySQL SQL 分开维护 |
| `api-examples/node/` | Node.js + MySQL 后端示例 | Vue/React 模板接口契约与 `database/mysql/` |
| `api-examples/go/` | Go + MySQL 后端示例 | Vue/React 模板接口契约与 `database/mysql/` |
| `shared/api-guide/` | Go/Node 共用 API 指南页面，随 Vue/React 候选构建发布到 `/api-guide/` | 官网及两演示站入口 |

- Vue、React 模板与生成器仅交付纯净版。演示代码、专用 API、部署及演示数据由本仓库管理。清理模板源码时先保留可重建的已验证线上版本，再独立验证新的演示构建来源。
- 请求库控制台源码、专用 API、状态 Worker 与站点测试在 `request/website/`。Vue 公开展示清理任务在 `vue/cleanup/`。
- 在线后台 API Worker 与 D1 migrations 只维护在 `api/cloudflare/`；React API 候选必须从这里复制源码，不再从模板 checkout 读取后端。模板仓库的 MySQL SQL 与此处的 D1 migrations 不可互相替代或执行。
- Go 和 Node MySQL 示例以 Vue 模板 `docs/api.md`、`authentication.md`、`rbac.md`、`dictionaries.md` 及 `database/mysql/` 为接口和结构基准。任一契约变更须同步核对两个示例、在线 Cloudflare API 和 `shared/api-guide/`。示例覆盖范围须在各自 README 如实列明；完整接口、服务端会话和权限校验通过本地验收前，不得称为完整可直接使用。
- `scripts/build-site.sh vue|react` 将 `shared/api-guide/` 注入各自候选的 `public/api-guide/`。对应 URL 为 `admin.npora.live/api-guide/` 与 `react.npora.live/api-guide/`；修改页面后核对两份产物均含 `index.html`、CSS、JS，再检查官网演示卡片入口。页面的复制命令、启动方式、源码链接必须与示例 README 一致，不展示未实现能力。
- 站点分别构建、预览、部署和回滚。通过固定 Git 提交或正式产物引用源仓库，记录实际部署版本；演示所需的密钥使用平台绑定。
- 跨站任务由主 agent 按目录指派子 agent，指定写入边界、来源版本和验收项。子 agent 汇报改动、验证和阻塞；主 agent 汇总检查版本、接口、入口链接及部署状态。
- 修改站点时同步核对其源仓库，修改源仓库时同步处理对应站点目录。涉及功能、配置、构建或演示契约的变化，更新有实际影响的版本引用、文件和验收；官网展示信息随可用功能同步。
- Vue 迁移保留既有演示地址并核对 API 转发和清理任务。React 验证登录、权限、深链接、PWA。请求库验证 Pages Functions、状态绑定和旧页面与 API 地址兼容。各站迁移先验证预览和回滚路径，再按授权切换。
- 官网仅链接已验收的线上入口。请求库切换前验证旧 `www.npora.live/#playground` 和 `/api` 的兼容访问，覆盖请求方法、请求体和流式响应。Vue 清理任务只由一个部署负责执行。

## 官网与生成器信息

- 官网 `https://www.npora.live/` 是总入口；导航中的“演示”列出 Vue、React 后台和 request 控制台，入口需与已验收域名一致。
- Vue 与 React 后台目前提供相同的页面和功能覆盖。官网、README 与演示说明用同一套功能清单介绍两者，只区分 Vue 3 / Element Plus 和 React / Ant Design 的实现技术。
- `@npora/create-admin` 生成器支持交互选择 Vue 或 React，并生成对应纯净模板。官网与生成器文档都列出官网、两套后台演示、request 控制台及数据库指南；不保留过时的“npm 仅支持 Vue”文案。
- 调整官网主导航、生成入口或框架支持状态后，检查 `www/README.md`、生成器 README/CLI 和两套模板 README/AGENTS 的链接与描述。
