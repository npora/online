import { query, transaction } from './db.mjs'
import { hashPassword, now } from './security.mjs'

const columns = `id, department_id, username, display_name, avatar, email, mobile, status,
  is_super_admin, last_login_at, created_at, updated_at`
const integer = (value, min = 0) => Number.isSafeInteger(Number(value)) && Number(value) >= min
const nullable = (value) => value === '' || value === undefined ? null : value
const invalid = (message = '请检查提交的数据') => { throw Object.assign(new Error(message), { status: 400, code: 400 }) }
const conflict = (message) => { throw Object.assign(new Error(message), { status: 409, code: 409 }) }
async function readJson(request) {
  const chunks = []
  let size = 0
  for await (const chunk of request) {
    size += chunk.length
    if (size > 8192) invalid('请求体过大')
    chunks.push(chunk)
  }
  try {
    const input = JSON.parse(Buffer.concat(chunks).toString('utf8'))
    if (!input || typeof input !== 'object' || Array.isArray(input)) throw new Error()
    return input
  } catch {
    invalid('JSON 格式错误')
  }
}
const permission = (state, name) => {
  if (!state.permissions.includes('*') && !state.permissions.includes(name)) {
    throw Object.assign(new Error('没有执行此操作的权限'), { status: 403, code: 403 })
  }
}

async function dto(row) {
  const [roles, department] = await Promise.all([
    query(`SELECT r.id, r.name, r.code, r.status FROM sys_user_role ur
      JOIN sys_role r ON r.id = ur.role_id WHERE ur.user_id = ? AND r.deleted_at IS NULL ORDER BY r.sort_order, r.id`, [row.id]),
    row.department_id === null ? [] : query('SELECT name FROM sys_department WHERE id = ? AND deleted_at IS NULL', [row.department_id])
  ])
  const bindings = await query('SELECT enabled FROM sys_user_totp WHERE user_id = ? LIMIT 1', [row.id])
  return {
    id: Number(row.id), username: row.username,
    departmentId: row.department_id === null ? null : Number(row.department_id),
    departmentName: department[0]?.name ?? null,
    displayName: row.display_name, avatar: row.avatar, email: row.email, mobile: row.mobile,
    status: Number(row.status), isSuperAdmin: Number(row.is_super_admin) === 1,
    authenticatorEnabled: Number(bindings[0]?.enabled || 0) === 1,
    lastLoginAt: row.last_login_at === null ? null : Number(row.last_login_at),
    roles: roles.map((role) => ({ id: Number(role.id), name: role.name, code: role.code, status: Number(role.status) })),
    roleIds: roles.map((role) => Number(role.id)),
    createdAt: Number(row.created_at), updatedAt: Number(row.updated_at)
  }
}

function checkFields(input, create) {
  if (!input || typeof input !== 'object' || Array.isArray(input)) invalid()
  const allowed = new Set(['departmentId','displayName','avatar','email','mobile','status','roleIds', ...(create ? ['username','password'] : [])])
  if (Object.keys(input).some((key) => !allowed.has(key))) invalid()
  if (typeof input.displayName !== 'string' || !input.displayName.trim() || input.displayName.trim().length > 100 ||
      !(input.departmentId === null || integer(input.departmentId, 1)) ||
      !(input.status === 0 || input.status === 1) ||
      !(input.avatar === null || input.avatar === undefined || (typeof input.avatar === 'string' && input.avatar.length <= 500 && (!input.avatar || /^https?:\/\//i.test(input.avatar)))) ||
      !(input.email === null || input.email === undefined || (typeof input.email === 'string' && input.email.length <= 255 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(input.email))) ||
      !(input.mobile === null || input.mobile === undefined || (typeof input.mobile === 'string' && input.mobile.length <= 32)) ||
      !Array.isArray(input.roleIds) || input.roleIds.length > 128 || input.roleIds.some((id) => !integer(id, 1)) ||
      new Set(input.roleIds).size !== input.roleIds.length) invalid()
  if (create && (typeof input.username !== 'string' || !/^[A-Za-z][A-Za-z0-9_]{2,63}$/.test(input.username) ||
      typeof input.password !== 'string' || input.password.length < 6 || input.password.length > 128)) invalid('用户名或密码格式错误')
}

async function checkReferences(input, existing = null) {
  if (input.departmentId !== null) {
    const [dept] = await query('SELECT id, status FROM sys_department WHERE id = ? AND deleted_at IS NULL', [input.departmentId])
    if (!dept || (Number(dept.status) !== 1 && Number(existing?.department_id) !== Number(dept.id))) invalid('不能选择已停用的部门')
  }
  if (input.roleIds.length) {
    const placeholders = input.roleIds.map(() => '?').join(',')
    const roles = await query(`SELECT id FROM sys_role WHERE deleted_at IS NULL AND id IN (${placeholders})`, input.roleIds)
    if (roles.length !== input.roleIds.length) invalid('角色不存在或已删除')
  }
}

async function page(request) {
  const url = new URL(request.url)
  const pageNo = Number(url.searchParams.get('page') || 1)
  const pageSize = Number(url.searchParams.get('pageSize') || 10)
  if (!integer(pageNo, 1) || !integer(pageSize, 1) || pageSize > 100) invalid('分页参数错误')
  const clauses = ['u.deleted_at IS NULL']
  const values = []
  const keyword = (url.searchParams.get('keyword') || '').trim().slice(0, 100)
  if (keyword) {
    clauses.push('(u.username LIKE ? OR u.display_name LIKE ? OR COALESCE(u.email, "") LIKE ? OR COALESCE(u.mobile, "") LIKE ?)')
    const like = `%${keyword}%`
    values.push(like, like, like, like)
  }
  const status = url.searchParams.get('status')
  if (status !== null) {
    if (!['0','1'].includes(status)) invalid('状态参数错误')
    clauses.push('u.status = ?'); values.push(Number(status))
  }
  const roleId = url.searchParams.get('roleId')
  if (roleId !== null) {
    if (!integer(roleId, 1)) invalid('角色参数错误')
    clauses.push('EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r ON r.id = ur.role_id AND r.deleted_at IS NULL WHERE ur.user_id = u.id AND ur.role_id = ?)')
    values.push(Number(roleId))
  }
  const departmentId = url.searchParams.get('departmentId')
  if (departmentId !== null) {
    if (!integer(departmentId, 1)) invalid('部门参数错误')
    const tree = await query('SELECT id, parent_id FROM sys_department WHERE deleted_at IS NULL')
    const selected = new Set([Number(departmentId)])
    let changed = true
    while (changed) {
      changed = false
      for (const department of tree) {
        if (selected.has(Number(department.parent_id)) && !selected.has(Number(department.id))) {
          selected.add(Number(department.id)); changed = true
        }
      }
    }
    const ids = [...selected]
    clauses.push(`u.department_id IN (${ids.map(() => '?').join(',')})`)
    values.push(...ids)
  }
  const where = clauses.join(' AND ')
  const [count] = await query(`SELECT COUNT(*) AS total FROM sys_user u WHERE ${where}`, values)
  const rows = await query(`SELECT u.${columns.split(', ').join(', u.')}
    FROM sys_user u WHERE ${where} ORDER BY u.id DESC LIMIT ? OFFSET ?`, [...values, pageSize, (pageNo - 1) * pageSize])
  return { list: await Promise.all(rows.map(dto)), total: Number(count.total) }
}

export async function userRoute(request, url, routeId, actor, state) {
  const method = request.method
  const action = { GET: 'list', POST: 'create', PUT: 'update', DELETE: 'delete' }[method]
  permission(state, `system:user:${action}`)
  if (!routeId && method === 'GET') return page(request)
  if (!routeId && method === 'POST') {
    const input = await readJson(request)
    checkFields(input, true)
    await checkReferences(input)
    if (input.roleIds.length) permission(state, 'system:user:assign-roles')
    const passwordHash = await hashPassword(input.password)
    const timestamp = Date.now()
    const inserted = await transaction(async (tx) => {
      const [result] = await tx.execute(`INSERT INTO sys_user
        (department_id, username, password_hash, display_name, avatar, email, mobile, status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, [input.departmentId, input.username, passwordHash, input.displayName.trim(), nullable(input.avatar), nullable(input.email), nullable(input.mobile), input.status, timestamp, timestamp])
      for (const roleId of input.roleIds) await tx.execute('INSERT INTO sys_user_role(user_id, role_id, created_at) VALUES (?, ?, ?)', [result.insertId, roleId, timestamp])
      return Number(result.insertId)
    })
    const [row] = await query(`SELECT ${columns} FROM sys_user WHERE id = ?`, [inserted])
    return await dto(row)
  }
  if (!routeId) return undefined
  const [existing] = await query(`SELECT ${columns}, password_hash FROM sys_user WHERE id = ? AND deleted_at IS NULL`, [routeId])
  if (!existing) fail404()
  if (method === 'GET') return undefined
  if (method === 'DELETE') {
    if (Number(existing.is_super_admin) === 1) conflict('超级管理员不能删除')
    await query('UPDATE sys_user SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL', [Date.now(), Date.now(), routeId])
    return null
  }
  if (method !== 'PUT') return undefined
  const input = await readJson(request)
  checkFields(input, false)
  await checkReferences(input, existing)
  if (Number(existing.is_super_admin) === 1 && input.status !== 1) conflict('超级管理员必须保持启用')
  const prior = await query('SELECT role_id FROM sys_user_role WHERE user_id = ?', [routeId])
  const priorIds = prior.map((row) => Number(row.role_id))
  if (priorIds.length !== input.roleIds.length || input.roleIds.some((id) => !priorIds.includes(Number(id)))) permission(state, 'system:user:assign-roles')
  const timestamp = Date.now()
  await transaction(async (tx) => {
    await tx.execute(`UPDATE sys_user SET department_id = ?, display_name = ?, avatar = ?, email = ?, mobile = ?, status = ?, updated_at = ?
      WHERE id = ? AND deleted_at IS NULL`, [input.departmentId, input.displayName.trim(), nullable(input.avatar), nullable(input.email), nullable(input.mobile), input.status, timestamp, routeId])
    await tx.execute('DELETE FROM sys_user_role WHERE user_id = ?', [routeId])
    for (const roleId of input.roleIds) await tx.execute('INSERT INTO sys_user_role(user_id, role_id, created_at) VALUES (?, ?, ?)', [routeId, roleId, timestamp])
  })
  const [row] = await query(`SELECT ${columns} FROM sys_user WHERE id = ?`, [routeId])
  return dto(row)
}

function fail404() { throw Object.assign(new Error('用户不存在'), { status: 404, code: 404 }) }
