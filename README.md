# Npora Online

本仓库管理 Npora 总官网、Vue 后台演示、React 后台演示和请求库控制台的部署。三个演示引用各自源码仓库的固定提交。

| 目录 | 站点 | 迁移目标 |
| --- | --- | --- |
| `www/` | Npora 总官网 | `www.npora.live` |
| `vue/` | Vue 后台演示 | 保留 `admin.npora.live` |
| `react/` | React 后台演示 | `react.npora.live` |
| `request/` | 请求库控制台 | `request.npora.live` |

## 构建迁移候选版本

使用满足 Vue Cloudflare 构建要求的 Node.js 24.22.0 或更新的 24.x。构建脚本从对应目录的 `source.env` 读取固定的提交号，在忽略跟踪的 `.build/` 内创建独立检出目录。构建和预检在本地执行。

```sh
bash scripts/build-site.sh vue
bash scripts/build-site.sh react
bash scripts/build-site.sh request
```

现有站点仍由原部署负责。迁移先验证预览和回滚路径，再按授权切换域名、API 绑定与定时清理所有权。
