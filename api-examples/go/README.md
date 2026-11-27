# Npora Go API 示例

Go + MySQL 后端示例，按 Vue/React 共享接口契约逐步实现。接口契约以 `admin-template/docs/api.md`、`authentication.md`、`rbac.md` 和 `dictionaries.md` 为准；业务表由模板 SQL 初始化，令牌会话、登录限流等后端专属表由 `../mysql/` 中线上仓库共享迁移维护。**当前实现尚未覆盖完整契约，不应直接作为完整后台后端使用。**

## 运行环境和数据库

- Go 1.26+
- MySQL 8.0.16+
- 在新数据库初始化时，从模板仓库取得官方 SQL，并创建空数据库后执行模板初始化脚本：

  ```sh
  git clone --depth 1 https://github.com/npora/admin-template.git /tmp/npora-admin-template
  cd /tmp/npora-admin-template
  mysql -u root -p -e "CREATE DATABASE npora_admin CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"
  sh database/mysql/init.sh --user root --password npora_admin
  mysql --default-character-set=utf8mb4 -u root -p npora_admin < database/mysql/05-authenticator.sql
  ```

  `init.sh` 只应用正式基础初始化及常规索引；另执行 `05-authenticator.sql` 是因为当前 Go 登录代码需要检查验证器绑定状态。审计 `08`、会话功能 `09`、审计索引 `10` 是对应接口完成后再选择的迁移；不要把尚未实现的功能迁移描述成必需安装步骤。执行前先阅读模板仓库 `database/mysql/README.md` 并确认数据库为空。
- 回到 `online-npora/` 仓库根目录执行 `mysql --default-character-set=utf8mb4 -u root -p npora_admin < api-examples/mysql/0001_auth_runtime.sql`，创建会话与限流表。再在模板克隆目录依次执行 `08-audit-logs.sql`、`09-session-management.sql`；Go 角色授权和管理员撤销会话会与审计记录事务提交。`10-audit-lookup-indexes.sql` 是可选查询索引迁移。

模板 SQL 和迁移使用数据库管理员执行；应用本身使用权限较小的专用账号。用强密码创建账号，只授予应用所需的 DML 权限（如果 Go 服务与 MySQL 不在同一台机器，把 `127.0.0.1` host 限定为 Go 服务的固定地址）：

```sh
mysql -u root -p -e "CREATE USER 'npora'@'127.0.0.1' IDENTIFIED BY '替换为openssl生成的强密码'; GRANT SELECT, INSERT, UPDATE, DELETE ON npora_admin.* TO 'npora'@'127.0.0.1';"
```

密码可用 `openssl rand -hex 24` 生成；将同一密码填入 `.env` 的 `MYSQL_DSN`，例如 `npora:生成的密码@tcp(127.0.0.1:3306)/npora_admin?charset=utf8mb4&parseTime=true&loc=UTC`。不要用该应用账号执行建表或迁移。

模板 SQL 不创建用户。仅 `APP_ENV=development` 时，Go API 首次启动会在数据库级命名锁保护下检查 `sys_user`：仅当表中没有任何用户记录时，创建 `admin` 超级管理员（密码仅保存 bcrypt 哈希），并关联种子中的 `admin` 角色；数据库已有账号或软删除记录时绝不覆盖。初始登录为 `admin` / `123456`，只用于本地隔离演示，首次登录后应立即修改密码。`APP_ENV=production` 时绝不执行弱口令 bootstrap；生产账号应由部署者通过独立安全流程创建，且上线前必须替换 Token pepper、配置 HTTPS 和限制管理入口。监听 `0.0.0.0` 仅用于本地局域网开发，不代表可直接公网暴露。

## 快速开始

先复制环境文件，再替换所有占位值。分别生成 Token pepper 和 TOTP 加密密钥（当前 TOTP 端点尚不可用，但不要复用密钥）：

```sh
openssl rand -hex 32
```

`TOKEN_PEPPER` 必须使用输出的完整 64 位十六进制字符串；服务启动时会拒绝占位符、长度错误或非十六进制值。将两次独立生成的输出分别填入 `.env` 的 `TOKEN_PEPPER`、`TOTP_ENCRYPTION_KEY`，并把 `MYSQL_DSN` 改为上一步创建的应用账号。不要提交 `.env`。

```sh
cp .env.example .env
go mod download
```

配置 `.env` 后启动：

```sh
go run ./cmd/server
```

Vue 或 React 模板 `.env.development` 中将 `VITE_API_URL` 配置为后端地址。与前端在同一台机器开发时可用 `http://localhost:8080`；局域网设备访问时改为服务端局域网 IP，并将该前端来源加入 `APP_ORIGINS`。生产环境推荐由同源反向代理将 `/api` 转到本服务。

健康检查：

```sh
curl http://localhost:8080/health
```

## 当前接口覆盖

已实现：健康检查、密码登录（已启用 TOTP 的账号会 fail-closed 拒绝登录）、退出、当前用户、会话与动态路由读取、用户分页读取和创建/编辑/删除、角色 CRUD/角色选项/授权读取和整体替换、菜单树读取及 CRUD、部门树 CRUD/选项读取、字典类型/数据 CRUD 与业务选项、审计日志筛选读取、权限预览、改密（绑定 TOTP 的账号会 fail-closed 拒绝）、已登录用户会话读取/撤销其他会话/撤销指定其他会话、管理员会话查看与撤销。角色授权替换和管理员撤销会话在同一事务写审计记录。创建用户与开发环境启动管理员都只保存 bcrypt 哈希；启动时仅在账号表为空时自动创建 `admin`/`123456` 并关联 `admin` 角色。角色授权替换使用事务并自动补齐祖先节点。所有成功响应遵循 `{ "code": 0, "message": "success", "data": ... }`；分页数据使用 `{ "list": [], "total": 0 }`；时间使用 Unix 毫秒。

未实现/未完全验收：登录持久限流、TOTP 登录验证与绑定/解绑、密码自助重置、管理员密码重置与管理用户 TOTP；审计失败操作记录、TOTP/密码重置等审计事件；权限预览目前没有区分直接/继承来源。用户列表当前未支持全部筛选条件，用户 DTO/角色详情也未完整对齐；菜单写入校验仍未覆盖所有契约规则（如完整路径/重定向冲突校验与组件白名单）。所有接口尚未逐条对照两套模板 API 模块验收。故目前不能满足“复制后即可完整驱动两套后台”的要求。

后端必须逐接口校验会话和权限，不能只依赖前端按钮权限；完成上述缺口、契约验收和 MySQL 集成验证后，才能称为可完整使用示例。

在所有接口通过验证前，本示例不得作为完整模板后端使用。所有密钥应仅通过 `.env` 注入并妥善保管；不要把真实 `.env` 提交到仓库。
