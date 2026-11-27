import { createServer } from 'node:http'
import { config, validateSecrets } from './config.mjs'
import { pool, query, transaction } from './db.mjs'
import { userRoute } from './users.mjs'
import { roleRoute } from './roles.mjs'
import { managementRoute } from './management.mjs'
import { authManagement, permissionPreview, sessionRoute } from './auth-management.mjs'
import { ensureDemoAdmin } from './bootstrap.mjs'
import {
  decryptSecret,
  encryptSecret,
  hashPassword,
  makeTotpSecret,
  now,
  sessionId,
  sha256,
  token,
  totpStep,
  verifyPassword
} from './security.mjs'

class ApiError extends Error {
  constructor(status, message, code = status, headers = {}) {
    super(message)
    Object.assign(this, { status, code, headers })
  }
}

const fail = (status, message, code = status, headers = {}) => {
  throw new ApiError(status, message, code, headers)
}
const ok = (data) => ({ code: 0, message: 'success', data })
const safeId = (raw) => {
  const id = Number(raw)
  if (!Number.isSafeInteger(id) || id <= 0) fail(400, 'ID 格式错误')
  return id
}
const mysqlBool = (value) => Number(value) === 1

async function body(request) {
  if (!String(request.headers['content-type'] || '').split(';')[0].trim().startsWith('application/json')) {
    fail(415, '请使用 application/json')
  }
  const chunks = []
  let size = 0
  for await (const chunk of request) {
    size += chunk.length
    if (size > 8192) fail(413, '请求体过大')
    chunks.push(chunk)
  }
  try {
    const value = JSON.parse(Buffer.concat(chunks).toString('utf8'))
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error()
    return value
  } catch {
    fail(400, 'JSON 格式错误')
  }
}

function send(response, status, value, headers = {}) {
  const payload = value === null ? '' : JSON.stringify(value)
  response.writeHead(status, {
    ...(value === null ? {} : { 'Content-Type': 'application/json; charset=utf-8' }),
    'Cache-Control': 'no-store',
    'X-Content-Type-Options': 'nosniff',
    ...headers
  })
  response.end(payload)
}

function cors(request, response) {
  const origin = request.headers.origin
  response.setHeader('Vary', 'Origin')
  if (!origin) return
  if (!config.corsOrigins.has(origin)) fail(403, '来源未获准访问')
  response.setHeader('Access-Control-Allow-Origin', origin)
  response.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS')
  response.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type')
  response.setHeader('Access-Control-Max-Age', '600')
}

async function authenticate(request) {
  const match = /^Bearer ([A-Za-z0-9_-]{43})$/.exec(request.headers.authorization || '')
  if (!match) fail(401, '登录状态已失效', 10002)
  const tokenHash = sha256(match[1])
  const rows = await query(
    `SELECT u.*, s.credential_fingerprint, s.public_id AS session_id
     FROM sys_auth_session s JOIN sys_user u ON u.id = s.user_id
     WHERE s.token_hash = ? AND s.expires_at > ? AND u.status = 1 AND u.deleted_at IS NULL`,
    [tokenHash, now()]
  )
  const user = rows[0]
  if (!user) fail(401, '登录状态已失效', 10002)
  if (sha256(user.password_hash) !== user.credential_fingerprint) {
    await query('DELETE FROM sys_auth_session WHERE token_hash = ?', [tokenHash])
    fail(401, '登录状态已失效', 10002)
  }
  request.auditedActorId = Number(user.id)
  return { ...user, tokenHash }
}

function auditedFailureTarget(request) {
  const url = new URL(request.url)
  const path = url.pathname.startsWith('/api/') ? url.pathname.slice(4) : url.pathname
  if (request.method === 'POST' && path === '/auth/password/change') return ['user', request.auditedActorId, 'password_reset']
  if (request.method === 'POST' && /^\/auth\/totp\/enable$/.test(path)) return ['user', request.auditedActorId, 'totp_enable']
  if (request.method === 'POST' && /^\/auth\/totp\/disable$/.test(path)) return ['user', request.auditedActorId, 'totp_disable']
  let match = /^\/system\/users\/(\d+)\/password$/.exec(path)
  if (request.method === 'POST' && match) return ['user', Number(match[1]), 'password_reset']
  match = /^\/system\/users\/(\d+)\/totp\/setup$/.exec(path)
  if (request.method === 'POST' && match) return ['user', Number(match[1]), 'totp_enable']
  match = /^\/system\/users\/(\d+)\/totp\/disable$/.exec(path)
  if (request.method === 'POST' && match) return ['user', Number(match[1]), 'totp_disable']
  match = /^\/system\/users\/(\d+)\/sessions\/[a-f0-9]{32}$/.exec(path)
  if (request.method === 'DELETE' && match) return ['user', Number(match[1]), 'session_revoke']
  match = /^\/system\/roles\/(\d+)\/menus$/.exec(path)
  if (request.method === 'PUT' && match) return ['role', Number(match[1]), 'role_grant']
  return null
}

async function recordAuditedFailure(request, status) {
  const target = auditedFailureTarget(request)
  if (!request.auditedActorId || !target || !Number.isSafeInteger(target[1]) || target[1] <= 0 || status < 400 || status > 599) return
  try {
    await query(`INSERT INTO sys_audit_log(actor_id,target_type,target_id,action,result,status_code,created_at)
      VALUES(?,?,?,?,'failure',?,?)`, [request.auditedActorId, target[0], target[1], target[2], status, now()])
  } catch {
    // Preserve the original operation error; never log request credentials or bodies.
    console.error(JSON.stringify({ event: 'audit_failure_write_failed' }))
  }
}

async function access(user) {
  const [roles, menus, links] = await Promise.all([
    query(`SELECT r.* FROM sys_role r JOIN sys_user_role ur ON ur.role_id = r.id
      WHERE ur.user_id = ? AND r.status = 1 AND r.deleted_at IS NULL ORDER BY r.sort_order, r.id`, [user.id]),
    query('SELECT * FROM sys_menu ORDER BY sort_order, id'),
    query(`SELECT DISTINCT rm.menu_id FROM sys_role_menu rm
      JOIN sys_role r ON r.id = rm.role_id JOIN sys_user_role ur ON ur.role_id = r.id
      WHERE ur.user_id = ? AND r.status = 1 AND r.deleted_at IS NULL`, [user.id])
  ])
  const byId = new Map(menus.map((row) => [Number(row.id), row]))
  const active = (menu) => {
    const seen = new Set()
    let current = menu
    while (current) {
      const id = Number(current.id)
      if (seen.has(id) || current.deleted_at !== null || Number(current.status) !== 1) return false
      seen.add(id)
      if (current.parent_id === null) return true
      current = byId.get(Number(current.parent_id))
    }
    return false
  }
  const granted = new Set(mysqlBool(user.is_super_admin)
    ? menus.filter(active).map((row) => Number(row.id))
    : links.map((row) => Number(row.menu_id)).filter((id) => active(byId.get(id))))
  const permissions = mysqlBool(user.is_super_admin) ? ['*'] : [...new Set(menus
    .filter((menu) => granted.has(Number(menu.id)) && Number(menu.type) === 3 && menu.permission)
    .map((menu) => menu.permission))]
  return { roles, menus, byId, granted, permissions, active }
}

function requirePermission(auth, permission) {
  if (!auth.permissions.includes('*') && !auth.permissions.includes(permission)) fail(403, '没有执行此操作的权限')
}

function routeQuery(value) {
  if (!value) return undefined
  const parsed = typeof value === 'string' ? JSON.parse(value) : value
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed) ||
      Object.entries(parsed).some(([key, item]) => ['__proto__', 'constructor', 'prototype'].includes(key) ||
        !['string', 'number', 'boolean'].includes(typeof item))) throw new Error('Invalid route query in database')
  return parsed
}

function dynamicRoutes(state) {
  const ids = new Set(state.granted)
  for (const id of state.granted) {
    let current = state.byId.get(id)
    while (current?.parent_id !== null && current?.parent_id !== undefined) {
      ids.add(Number(current.parent_id))
      current = state.byId.get(Number(current.parent_id))
    }
  }
  const children = new Map()
  for (const menu of state.menus) {
    const id = Number(menu.id)
    if (Number(menu.type) === 3 || !ids.has(id) || !state.active(menu)) continue
    const parent = menu.parent_id === null ? null : Number(menu.parent_id)
    if (!children.has(parent)) children.set(parent, [])
    children.get(parent).push(menu)
  }
  const build = (parent) => (children.get(parent) || []).flatMap((menu) => {
    const nested = build(Number(menu.id))
    if (Number(menu.type) === 1 && !nested.length) return []
    return [{
      path: menu.path,
      ...(menu.name ? { name: menu.name } : {}),
      title: menu.title,
      ...(menu.component ? { component: menu.component } : {}),
      ...(menu.redirect ? { redirect: menu.redirect } : {}),
      ...(menu.icon ? { icon: menu.icon } : {}),
      ...(routeQuery(menu.route_query) ? { routeQuery: routeQuery(menu.route_query) } : {}),
      ...(menu.active_menu ? { activeMenu: menu.active_menu } : {}),
      hidden: !mysqlBool(menu.visible),
      alwaysShow: mysqlBool(menu.always_show),
      keepAlive: mysqlBool(menu.keep_alive),
      fixedTab: mysqlBool(menu.fixed_tab),
      order: Number(menu.sort_order),
      ...(nested.length ? { children: nested } : {}),
      permissions: state.menus.filter((item) => Number(item.parent_id) === Number(menu.id) &&
        Number(item.type) === 3 && state.granted.has(Number(item.id)))
        .map((item) => ({ name: item.title, permission: item.permission }))
    }]
  })
  return build(null)
}

async function login(request) {
  const input = await body(request)
  if (typeof input.username !== 'string' || !/^[A-Za-z][A-Za-z0-9_]{2,63}$/.test(input.username) ||
      typeof input.password !== 'string' || input.password.length < 1 || input.password.length > 128 ||
      (input.rememberMe !== undefined && typeof input.rememberMe !== 'boolean')) fail(400, '用户名或密码格式错误')
  const username = input.username.toLowerCase()
  const ip = request.socket.remoteAddress || 'unknown'
  const bucket = sha256(`account-ip:${username}:${ip}`)
  const ipBucket = sha256(`ip:${ip}`)
  const current = now()
  await transaction(async (tx) => {
    await tx.execute('DELETE FROM sys_auth_attempt WHERE expires_at <= ?', [current])
    await tx.execute(`INSERT INTO sys_auth_attempt(bucket, attempts, expires_at) VALUES (?, 1, ?)
      ON DUPLICATE KEY UPDATE attempts = LEAST(attempts + 1, 1000000)`, [ipBucket, current + 900_000])
    await tx.execute(`INSERT INTO sys_auth_attempt(bucket, attempts, expires_at) VALUES (?, 1, ?)
      ON DUPLICATE KEY UPDATE attempts = LEAST(attempts + 1, 1000000)`, [bucket, current + 900_000])
  })
  const attemptRows = await query('SELECT bucket, attempts, expires_at FROM sys_auth_attempt WHERE bucket IN (?, ?)', [ipBucket, bucket])
  const maxAttempts = attemptRows.some((row) => Number(row.attempts) > (row.bucket === ipBucket ? 30 : 5))
  if (maxAttempts) {
    const expires = Math.max(...attemptRows.map((row) => Number(row.expires_at)))
    fail(429, '尝试次数过多，请稍后再试', 429, { 'Retry-After': String(Math.max(1, Math.ceil((expires - current) / 1000))) })
  }
  const users = await query('SELECT * FROM sys_user WHERE username = ? AND deleted_at IS NULL LIMIT 1', [input.username])
  const user = users[0]
  if (!user || Number(user.status) !== 1 || !(await verifyPassword(input.password, user.password_hash))) fail(401, '用户名或密码错误', 10001)
  const bindings = await query('SELECT * FROM sys_user_totp WHERE user_id = ? AND enabled = 1 LIMIT 1', [user.id])
  const binding = bindings[0]
  let step = null
  if (binding) {
    if (typeof input.totpCode !== 'string') fail(400, '请输入有效的验证器动态码', 10003)
    step = totpStep(decryptSecret(binding.secret_encrypted), input.totpCode, current)
    if (step === null || step <= Number(binding.last_used_step)) fail(400, '请输入有效的验证器动态码，验证码不能重复使用', 10003)
  }
  const accessToken = token()
  const ttl = input.rememberMe ? config.rememberTtlMs : config.defaultTtlMs
  const tokenHash = sha256(accessToken)
  await transaction(async (tx) => {
    const fresh = await tx.query('SELECT id, password_hash, status, deleted_at FROM sys_user WHERE id = ? FOR UPDATE', [user.id])
    if (!fresh[0] || Number(fresh[0].status) !== 1 || fresh[0].deleted_at !== null || fresh[0].password_hash !== user.password_hash) fail(401, '用户名或密码错误', 10001)
    if (binding) {
      const [totpResult] = await tx.execute('UPDATE sys_user_totp SET last_used_step = ? WHERE user_id = ? AND enabled = 1 AND secret_encrypted = ? AND last_used_step < ?', [step, user.id, binding.secret_encrypted, step])
      if (totpResult.affectedRows !== 1) fail(400, '动态验证码已使用，请等待下一个验证码', 10003)
    }
    await tx.execute('DELETE FROM sys_auth_session WHERE expires_at <= ?', [current])
    await tx.execute(`DELETE FROM sys_auth_session WHERE user_id = ? AND token_hash NOT IN
      (SELECT token_hash FROM (SELECT token_hash FROM sys_auth_session WHERE user_id = ? ORDER BY created_at DESC, token_hash DESC LIMIT 9) AS keep_sessions)`, [user.id, user.id])
    await tx.execute('INSERT INTO sys_auth_session(token_hash, public_id, user_id, credential_fingerprint, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)',
      [tokenHash, sessionId(), user.id, sha256(user.password_hash), current, current + ttl])
    await tx.execute('UPDATE sys_user SET last_login_at = ?, updated_at = ? WHERE id = ?', [current, current, user.id])
  })
  return ok({ accessToken, expiresAt: current + ttl })
}

async function sessionInfo(request) {
  const user = await authenticate(request)
  const state = await access(user)
  return ok({
    user: {
      id: Number(user.id), username: user.username, displayName: user.display_name,
      ...(user.avatar ? { avatar: user.avatar } : {}), departmentId: user.department_id === null ? null : Number(user.department_id),
      isSuperAdmin: mysqlBool(user.is_super_admin), roles: state.roles.map((role) => role.code), permissions: state.permissions
    },
    routes: dynamicRoutes(state)
  })
}

async function currentUser(request) {
  const result = await sessionInfo(request)
  return ok(result.data.user)
}

async function currentRoutes(request) {
  const result = await sessionInfo(request)
  return ok(result.data.routes)
}

async function logout(request) {
  const match = /^Bearer ([A-Za-z0-9_-]{43})$/.exec(request.headers.authorization || '')
  if (!match) fail(401, '登录状态已失效', 10002)
  await query('DELETE FROM sys_auth_session WHERE token_hash = ?', [sha256(match[1])])
  return ok(null)
}

async function dashboard(request) {
  const user = await authenticate(request)
  if (!mysqlBool(user.is_super_admin)) fail(403, '没有执行此操作的权限')
  const [users, roles, departments] = await Promise.all([
    query('SELECT COUNT(*) AS total FROM sys_user WHERE deleted_at IS NULL'),
    query('SELECT COUNT(*) AS total FROM sys_role WHERE deleted_at IS NULL'),
    query('SELECT COUNT(*) AS total FROM sys_department WHERE deleted_at IS NULL')
  ])
  return ok({ userCount: Number(users[0].total), roleCount: Number(roles[0].total), departmentCount: Number(departments[0].total) })
}

async function health() {
  await query('SELECT 1 AS ok')
  return ok({ database: 'connected' })
}

async function dispatch(request) {
  const url = new URL(request.url)
  const path = url.pathname.startsWith('/api/') ? url.pathname.slice(4) : url.pathname === '/api' ? '/' : url.pathname
  if (request.method === 'GET' && path === '/health') return health()
  if (request.method === 'POST' && path === '/auth/login') return login(request)
  if (request.method === 'POST' && path === '/auth/logout') return logout(request)
  if (path === '/auth/password/reset') return ok(await authManagement(request, path, null, null))
  const protectedAuthPath = ['/auth/password/change','/auth/totp','/auth/totp/setup','/auth/totp/enable','/auth/totp/disable'].includes(path)
  const adminSecurityPath = /^\/system\/users\/\d+\/(?:password|totp(?:\/(?:setup|disable))?)$/.test(path)
  if (protectedAuthPath || adminSecurityPath) {
    const actor = await authenticate(request)
    const state = await access(actor)
    return ok(await authManagement(request, path, actor, state))
  }
  if (path.startsWith('/auth/sessions') || /^\/system\/users\/\d+\/sessions(?:\/[^/]+)?$/.test(path)) {
    const actor = await authenticate(request)
    const state = await access(actor)
    return ok(await sessionRoute(request, path, actor, state))
  }
  const previewMatch = /^\/system\/users\/(\d+)\/permission-preview$/.exec(path)
  if (previewMatch) {
    if (request.method !== 'GET') fail(405, 'Method not allowed', 405, { Allow: 'GET' })
    const actor = await authenticate(request)
    const state = await access(actor)
    return ok(await permissionPreview(safeId(previewMatch[1]), state))
  }
  if (request.method === 'GET' && path === '/auth/session') return sessionInfo(request)
  if (request.method === 'GET' && path === '/auth/user') return currentUser(request)
  if (request.method === 'GET' && path === '/auth/routes') return currentRoutes(request)
  if (request.method === 'GET' && path === '/system/dashboard') return dashboard(request)
  const userMatch = /^\/system\/users(?:\/([1-9]\d*))?$/.exec(path)
  if (userMatch) {
    if (!['GET', 'POST', 'PUT', 'DELETE'].includes(request.method)) fail(405, 'Method not allowed', 405, { Allow: userMatch[1] ? 'PUT, DELETE' : 'GET, POST' })
    if ((request.method === 'GET' || request.method === 'POST') && userMatch[1]) fail(405, 'Method not allowed', 405, { Allow: 'PUT, DELETE' })
    if ((request.method === 'PUT' || request.method === 'DELETE') && !userMatch[1]) fail(405, 'Method not allowed', 405, { Allow: 'GET, POST' })
    const actor = await authenticate(request)
    const state = await access(actor)
    return ok(await userRoute(request, url, userMatch[1] ? safeId(userMatch[1]) : undefined, actor, state))
  }
  const roleMatch = /^\/system\/roles(?:\/(options|menu-options|([1-9]\d*)\/menus|([1-9]\d*)))?$/.exec(path)
  if (roleMatch) {
    const [, operation, menuRoleId, roleId] = roleMatch
    const id = menuRoleId || roleId
    const actor = await authenticate(request)
    const state = await access(actor)
    if (operation === 'options' || operation === 'menu-options') {
      if (request.method !== 'GET') fail(405, 'Method not allowed', 405, { Allow: 'GET' })
      return ok(await roleRoute(request, url, undefined, operation, state, actor))
    }
    if (operation?.endsWith('/menus')) {
      if (!['GET', 'PUT'].includes(request.method)) fail(405, 'Method not allowed', 405, { Allow: 'GET, PUT' })
      return ok(await roleRoute(request, url, safeId(menuRoleId), 'menus', state, actor))
    }
    const methods = id ? ['PUT', 'DELETE'] : ['GET', 'POST']
    if (!methods.includes(request.method)) fail(405, 'Method not allowed', 405, { Allow: methods.join(', ') })
    return ok(await roleRoute(request, url, id ? safeId(id) : undefined, undefined, state, actor))
  }
  const managementMatch = /^\/system\/(menus|departments|dict-types|audit-logs|dictionaries)(.*)$/.exec(path)
  if (managementMatch) {
    const [, resource, tail] = managementMatch
    const routePath = `/system/${resource}${tail}`
    const rawId = /^\/(\d+)$/.exec(tail)?.[1] || /^\/(\d+)\/items\/\d+$/.exec(tail)?.[1]
    if (['audit-logs','dictionaries'].includes(resource) && request.method !== 'GET') fail(405, 'Method not allowed', 405, { Allow: 'GET' })
    if (['PUT','DELETE'].includes(request.method) && !rawId && !/\/items$/.test(tail)) fail(405, 'Method not allowed', 405, { Allow: 'GET, POST' })
    if (['GET','POST'].includes(request.method) && /^\/\d+$/.test(tail)) fail(405, 'Method not allowed', 405, { Allow: 'PUT, DELETE' })
    const actor = await authenticate(request)
    const state = await access(actor)
    return ok(await managementRoute(request, url, routePath, rawId ? safeId(rawId) : undefined, actor, state))
  }
  if (path.startsWith('/system/')) return fail(501, '此 API 路由尚未在 Node 示例中实现')
  return fail(404, 'Not found')
}

const server = createServer(async (request, response) => {
  try {
    cors(request, response)
    if (request.method === 'OPTIONS') return send(response, 204, null)
    const result = await dispatch(request)
    send(response, 200, result)
  } catch (error) {
    if (error instanceof ApiError || Number.isInteger(error?.status)) {
      await recordAuditedFailure(request, error.status)
      return send(response, error.status, { code: error.code ?? error.status, message: error.message || '请求失败', data: null }, error.headers || {})
    }
    await recordAuditedFailure(request, 503)
    console.error(JSON.stringify({ event: 'api_error', message: error?.message || 'unknown' }))
    send(response, 503, { code: 503, message: '服务暂时不可用，请稍后重试', data: null })
  }
})

validateSecrets()
ensureDemoAdmin().then(() => {
  server.listen(config.port, config.host, () => {
    console.log(`Npora Node API listening on http://${config.host}:${config.port}`)
  })
}).catch((error) => {
  console.error(`Database startup/bootstrap failed: ${error.message}`)
  process.exitCode = 1
})

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, async () => {
    server.close()
    await pool.end()
    process.exit(0)
  })
}
