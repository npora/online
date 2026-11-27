# Npora 总官网

官网使用 Cloudflare Workers Static Assets，线上入口为 [www.npora.live](https://www.npora.live/)，页面源码位于本仓库 [`public/`](public/)，部署配置由本目录维护。官网提供 [Vue 后台演示](https://admin.npora.live/)、[React 后台演示](https://react.npora.live/)和[请求库控制台](https://request.npora.live/)入口。对应项目源码分别由 [Vue 模板](https://github.com/npora/admin-template)、[React 模板](https://github.com/npora/admin-template-react)、[请求库](https://github.com/npora/request)和[生成器](https://github.com/npora/create-admin)维护。

Vue 和 React 演示站各自提供 Go / Node API 指南：[`admin.npora.live/api-guide/`](https://admin.npora.live/api-guide/) 与 [`react.npora.live/api-guide/`](https://react.npora.live/api-guide/)。指南源码位于 [`shared/api-guide/`](../shared/api-guide/)，由固定版本候选构建复制到两个站点；接口覆盖与可运行状态以 [`api-examples/`](../api-examples/) 中各语言 README 为准。

`/api` 和 `/api/*` 兼容旧请求库示例，由官网 Worker 透传到固定的请求库域名。历史 `/#playground` 链接会跳到新的控制台。先验收请求站及其状态绑定，再切换官网域名。www 使用 Worker Route 接管流量，保留原 Pages DNS 和绑定作为回滚来源。根域现有 DNS 不由本配置管理。切换后检查旧 API 客户端。

## 快速开始文案

生成器支持交互式选择 Vue 或 React，并创建对应的纯净版后台项目。两套后台的页面与功能覆盖一致，技术栈分别为 Vue 3 + Element Plus 和 React + Ant Design。官网展示两套模板的独立演示入口；线上演示使用独立配置，不作为生成器选项。官网的 SQL 指南链接分别指向 Vue 与 React 的 `/database/` 页面。

## 本地预览与部署预检

在 `www/` 下运行：

```sh
node --test worker.test.mjs
npx --yes wrangler@4.154.0 dev --ip 127.0.0.1 --port 8790
npx --yes wrangler@4.154.0 deploy --dry-run
```

预览检查首页、手机布局、项目入口、快速开始和 404。正式切换需要部署授权和 Cloudflare 域名配置。

首屏的 `public/assets/vue-demo.gif` 和 `public/assets/react-demo.gif` 展示各自后台。两套预览使用相同尺寸，支持手动隐藏动态内容并尊重系统的减少动态效果偏好。
