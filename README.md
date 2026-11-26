# Npora Online

本仓库管理 Npora 总官网、Vue 后台演示、React 后台演示和请求库控制台的线上部署。各站页面和应用源码归对应项目仓库维护，演示构建引用已核对的源码版本；本仓库负责线上入口、演示配置、预览和部署。

| 目录 | 职责与线上入口 | 对应源码 |
| --- | --- | --- |
| [`www/`](www/README.md) | 总官网、项目介绍与演示导航：[www.npora.live](https://www.npora.live/) | [官网部署源码](https://github.com/npora/online/tree/main/www)；[生成器](https://github.com/npora/create-admin) |
| [`vue/`](vue/README.md) | Vue 后台在线演示：[admin.npora.live](https://admin.npora.live/) | [Vue 后台模板](https://github.com/npora/admin-template) |
| [`react/`](react/README.md) | React 后台在线演示：[react.npora.live](https://react.npora.live/) | [React 后台模板](https://github.com/npora/admin-template-react) |
| [`request/`](request/README.md) | 请求库在线介绍与控制台：[request.npora.live](https://request.npora.live/) | [请求库与 API 文档](https://github.com/npora/request) |

## 构建迁移候选版本

使用满足 Vue Cloudflare 构建要求的 Node.js 24.22.0 或更新的 24.x。构建脚本从对应目录的 `source.env` 读取固定的提交号，在忽略跟踪的 `.build/` 内创建独立检出目录。构建和预检在本地执行。

```sh
bash scripts/build-site.sh vue
bash scripts/build-site.sh react
bash scripts/build-site.sh request
```

现有站点仍由原部署负责。迁移先验证预览和回滚路径，再按授权切换域名、API 绑定与定时清理所有权。
