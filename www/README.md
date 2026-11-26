# Npora 总官网

官网使用 Cloudflare Workers Static Assets，发布域名为 `www.npora.live`。页面放在 `public/`，可直接预览。Vue 演示沿用 `admin.npora.live`，React 演示使用独立的只读数据。请求控制台使用 `request.npora.live`。

`/api` 和 `/api/*` 兼容旧请求库示例，由官网 Worker 透传到固定的请求库域名。历史 `/#playground` 链接会跳到新的控制台。先验收请求站及其状态绑定，再切换官网域名。www 使用 Worker Route 接管流量，保留原 Pages DNS 和绑定作为回滚来源。根域现有 DNS 不由本配置管理。切换后检查旧 API 客户端。

## 本地预览与部署预检

在 `www/` 下运行：

```sh
node --test worker.test.mjs
npx --yes wrangler@4.154.0 dev --ip 127.0.0.1 --port 8790
npx --yes wrangler@4.154.0 deploy --dry-run
```

预览检查首页、手机布局、项目入口、快速开始和 404。正式切换需要部署授权和 Cloudflare 域名配置。

首屏的 `public/assets/vue-demo.gif` 和 `public/assets/react-demo.gif` 展示各自后台。两套预览使用相同尺寸，支持手动隐藏动态内容并尊重系统的减少动态效果偏好。
