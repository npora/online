# Npora Admin Node.js + MySQL API

This standalone backend is an in-progress Node.js + MySQL example for the Vue and React admin templates. It does not use Cloudflare Workers or D1. It is not yet a complete replacement backend.

## Requirements

- Node.js 24.11 or newer
- MySQL 8.0.16 or newer
- A newly created, empty MySQL database

## Install dependencies

```sh
cp .env.example .env
npm install
```

Complete the database setup below and edit `.env` with the local database connection and independently generated secrets before starting. The server listens on `0.0.0.0:3000`, so the frontend can reach it from another device on the LAN. Do not expose this example server publicly without setting production secrets, HTTPS, an exact CORS allowlist, rate limiting, and a reviewed deployment configuration.

Generate two independent 32-byte secrets and copy the printed values into `AUTH_PEPPER` and `TOTP_ENCRYPTION_KEY` in `.env`:

```sh
printf 'AUTH_PEPPER=%s\nTOTP_ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)" "$(openssl rand -hex 32)"
```

Use a local MySQL account limited to the example database. After creating `npora_admin`, a local development account can be granted access with:

```sql
CREATE USER 'npora'@'localhost' IDENTIFIED BY 'replace-with-a-strong-local-password';
GRANT ALL PRIVILEGES ON npora_admin.* TO 'npora'@'localhost';
```

Put the same database password in `DB_PASSWORD`. For a remote MySQL host, restrict the account host to the API server's address instead of `%`.

## MySQL schema

Create an empty database first. Get the schema files from the Vue template repository (the SQL is intentionally not duplicated in this API example):

```sh
git clone https://github.com/npora/admin-template.git
```

Create the empty MySQL database (the client will prompt for the password):

```sh
mysql --user=root --password -e 'CREATE DATABASE npora_admin CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci'
```

From the template checkout, create/select the database and run the supported initializer for the base schema and standard indexes:

```sh
cd admin-template
sh database/mysql/init.sh --user root --password npora_admin
```

This applies `01-schema.sql` through `04-query-indexes.sql`, then `06-sort-indexes.sql` and `07-recent-users-index.sql`. This Node example also uses TOTP and audit tables, so apply these additional template migrations in order: `05-authenticator.sql`, `08-audit-logs.sql`, `09-session-management.sql`, `10-audit-lookup-indexes.sql`. Migration `09` depends on `08`; `10` also depends on `08`. Do not run `upgrades/01-millisecond-timestamps.sql` for a fresh database; it is only for an existing legacy database with ISO timestamp columns.

Run these commands from the `admin-template` checkout. Each command prompts for the MySQL password:

```sh
mysql --user=root --password npora_admin < database/mysql/05-authenticator.sql
mysql --user=root --password npora_admin < database/mysql/08-audit-logs.sql
mysql --user=root --password npora_admin < database/mysql/09-session-management.sql
mysql --user=root --password npora_admin < database/mysql/10-audit-lookup-indexes.sql
```

Finally, from the root of the cloned `online` repository, apply the backend-owned session and login-throttling migration:

```sh
mysql --user=root --password --database=npora_admin < api-examples/mysql/0001_auth_runtime.sql
```

The shared example migration creates only new backend-runtime tables. It does not alter or replace the templates' MySQL schema. Run it once, after the template migrations, and do not rerun it against an already initialized database.

## Start the server

After the database migrations are complete and `.env` contains the database credentials plus both generated 32-byte keys, start the API from this directory:

```sh
npm run dev
```

With `APP_ENV=development`, startup acquires a MySQL advisory lock and creates `admin` / `123456` only if `sys_user` has no rows at all. It stores only the configured-pepper scrypt hash, sets `is_super_admin`, and links the seeded `admin` role if available. Any existing user row prevents bootstrap writes. Change this demo password immediately after first login; this credential is only for a local, isolated demo. `APP_ENV=production` refuses the empty-database demo bootstrap; provision the first administrator with a production-specific secure setup instead. Do not expose the demo account publicly.

## Connect either frontend

In the frontend `.env.development`, set `VITE_API_URL=http://localhost:3000` (or the LAN address of the machine running this server, without an `/api` suffix). Keep `VITE_API_PREFIX=/api`; the development proxy forwards that path and this server accepts it. The API returns `{ code, message, data }`, uses Unix millisecond timestamps, and expects a Bearer access token on protected routes.

## API and security

The canonical endpoint and DTO contract is documented in `admin-template/docs/api.md`, `authentication.md`, `rbac.md`, and `dictionaries.md`. Passwords, TOTP seeds, access tokens, permission checks, session revocation, and audit events must stay server-side. Never commit `.env` or use the demo credentials outside a local/private environment.

## Current coverage and limitations

| API group | Implemented paths |
| --- | --- |
| Health and auth | `GET /health`; `POST /auth/login`, `/auth/logout`; `GET /auth/session`, `/auth/user`, `/auth/routes` |
| Password and TOTP | `POST /auth/password/reset`, `/auth/password/change`; `GET /auth/totp`; `POST /auth/totp/setup`, `/auth/totp/enable`, `/auth/totp/disable` |
| Sessions and preview | `GET /auth/sessions`; `DELETE /auth/sessions/:sessionId`, `/auth/sessions/others`; `GET /system/users/:id/sessions`; `DELETE /system/users/:id/sessions/:sessionId`; `GET /system/users/:id/permission-preview` |
| User management | `GET/POST /system/users`; `PUT/DELETE /system/users/:id`; `POST /system/users/:id/password`; `GET /system/users/:id/totp`; `POST /system/users/:id/totp/setup`, `/system/users/:id/totp/disable` |
| Roles and menus | `GET/POST /system/roles`; `PUT/DELETE /system/roles/:id`; `GET /system/roles/options`, `/system/roles/menu-options`, `/system/roles/:id/menus`; `PUT /system/roles/:id/menus`; `GET/POST /system/menus`; `PUT/DELETE /system/menus/:id` |
| Departments | `GET /system/departments`, `/system/departments/options`; `POST /system/departments`; `PUT/DELETE /system/departments/:id` |
| Dictionaries | `GET/POST /system/dict-types`; `PUT/DELETE /system/dict-types/:id`; `GET/POST /system/dict-types/:id/items`; `PUT/DELETE /system/dict-types/:id/items/:itemId`; `GET /system/dictionaries/:code` |
| Audit | `GET /system/audit-logs` |

The server enforces Bearer sessions, permissions on management operations, scrypt password hashes, TOTP replay checks, persisted login throttling, transactional audited writes, and millisecond timestamps. It records success and authenticated failure events for password reset/change, TOTP enable/disable, role grants, and administrator session revocation. Public password-reset attempts are not recorded because no authenticated actor is available and the response intentionally does not confirm whether an account exists. If the audit table is unavailable during a failed operation, the API preserves the original error and writes a generic server log marker; that audit event cannot be persisted until storage recovers.

Menu writes validate parent types and cycles, route paths and names, button permission codes, route component whitelist, and redirect loops. TOTP code consumption is committed in the same transaction as the related password, binding, session, and success-audit writes. The source still has not been exercised against a live MySQL instance or verified end-to-end against both frontends. Do not use this example as a production backend or claim drop-in compatibility until those checks pass. Node and Go share the SQL migration and should keep their route/DTO coverage lists synchronized.
