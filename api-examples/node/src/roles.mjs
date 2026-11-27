import { query, transaction } from './db.mjs'

const error = (status, message) => { throw Object.assign(new Error(message), { status, code: status }) }
const requirePermission = (state, name) => {
  if (!state.permissions.includes('*') && !state.permissions.includes(name)) error(403, '没有执行此操作的权限')
}
const isId = (value) => Number.isSafeInteger(Number(value)) && Number(value) > 0
const roleFields = new Set(['code', 'name', 'description', 'sortOrder', 'status'])

async function readJson(request) {
  let text = ''
  for await (const chunk of request) {
    text += chunk
    if (Buffer.byteLength(text) > 8192) error(413, '请求体过大')
  }
  try {
    const data = JSON.parse(text)
    if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error()
    return data
  } catch {
    error(400, 'JSON 格式错误')
  }
}

function validateRole(input, creating) {
  if (Object.keys(input).some((key) => !roleFields.has(key)) ||
      (creating && (typeof input.code !== 'string' || !/^[a-z][a-z0-9_-]{0,63}$/.test(input.code))) ||
      typeof input.name !== 'string' || !input.name.trim() || input.name.trim().length > 100 ||
      !(input.description === null || input.description === undefined || (typeof input.description === 'string' && input.description.length <= 255)) ||
      !Number.isSafeInteger(input.sortOrder) || input.sortOrder < 0 ||
      !(input.status === 0 || input.status === 1)) error(400, '角色资料格式错误')
}

async function roleDto(row) {
  const [count] = await query('SELECT COUNT(*) AS total FROM sys_user_role WHERE role_id = ?', [row.id])
  return {
    id: Number(row.id), name: row.name, code: row.code, status: Number(row.status),
    description: row.description, sortOrder: Number(row.sort_order), userCount: Number(count.total),
    createdAt: Number(row.created_at), updatedAt: Number(row.updated_at)
  }
}

function tree(rows) {
  const children = new Map()
  for (const row of rows.filter((item) => item.deleted_at === null)) {
    const parentId = row.parent_id === null ? null : Number(row.parent_id)
    if (!children.has(parentId)) children.set(parentId, [])
    children.get(parentId).push(row)
  }
  const build = (parent, seen = new Set()) => (children.get(parent) || []).flatMap((row) => {
    const id = Number(row.id)
    if (seen.has(id)) return []
    const next = new Set(seen).add(id)
    const nested = build(id, next)
    return [{
      id, parentId: row.parent_id === null ? null : Number(row.parent_id), type: Number(row.type),
      title: row.title, name: row.name, path: row.path, component: row.component, redirect: row.redirect,
      routeQuery: row.route_query ? (typeof row.route_query === 'string' ? JSON.parse(row.route_query) : row.route_query) : null,
      activeMenu: row.active_menu, icon: row.icon, permission: row.permission,
      visible: Number(row.visible) === 1, alwaysShow: Number(row.always_show) === 1,
      keepAlive: Number(row.keep_alive) === 1, fixedTab: Number(row.fixed_tab) === 1,
      sortOrder: Number(row.sort_order), status: Number(row.status),
      createdAt: Number(row.created_at), updatedAt: Number(row.updated_at),
      ...(nested.length ? { children: nested } : {})
    }]
  })
  return build(null)
}

export async function roleRoute(request, url, routeId, operation, state, actor) {
  if (operation === 'options') {
    if (!['system:user:list','system:user:create','system:user:update','system:department:list','system:department:create','system:department:update'].some((name) => state.permissions.includes('*') || state.permissions.includes(name))) error(403, '没有执行此操作的权限')
    const rows = await query('SELECT id, name, code, status FROM sys_role WHERE deleted_at IS NULL ORDER BY sort_order, id')
    return rows.map((row) => ({ id: Number(row.id), name: row.name, code: row.code, status: Number(row.status) }))
  }
  if (operation === 'menu-options') {
    requirePermission(state, 'system:role:grant')
    return tree(await query('SELECT * FROM sys_menu ORDER BY sort_order, id'))
  }
  if (operation === 'menus' && routeId) {
    requirePermission(state, 'system:role:grant')
    const [role] = await query('SELECT id FROM sys_role WHERE id = ? AND deleted_at IS NULL', [routeId])
    if (!role) error(404, '角色不存在')
    if (request.method === 'GET') {
      const rows = await query('SELECT menu_id FROM sys_role_menu WHERE role_id = ?', [routeId])
      return rows.map((row) => Number(row.menu_id))
    }
    if (request.method !== 'PUT') error(405, 'Method not allowed')
    const data = await readJson(request)
    if (Object.keys(data).length !== 1 || !Array.isArray(data.menuIds) || data.menuIds.length > 1000 || data.menuIds.some((id) => !isId(id))) error(400, '菜单授权格式错误')
    const selected = new Set(data.menuIds.map(Number))
    const all = await query('SELECT id, parent_id, type, status, deleted_at FROM sys_menu')
    const byId = new Map(all.map((row) => [Number(row.id), row]))
    for (const id of selected) {
      let node = byId.get(id)
      if (!node || Number(node.status) !== 1 || node.deleted_at !== null) error(400, '菜单不存在或已停用')
      const visited = new Set()
      while (node.parent_id !== null) {
        if (visited.has(Number(node.id))) error(400, '菜单层级存在循环')
        visited.add(Number(node.id))
        node = byId.get(Number(node.parent_id))
        if (!node || Number(node.status) !== 1 || node.deleted_at !== null) error(400, '菜单祖先不存在或已停用')
        selected.add(Number(node.id))
      }
    }
    const timestamp = Date.now()
    await transaction(async (tx) => {
      await tx.execute('DELETE FROM sys_role_menu WHERE role_id = ?', [routeId])
      for (const menuId of selected) await tx.execute('INSERT INTO sys_role_menu(role_id, menu_id, created_at) VALUES (?, ?, ?)', [routeId, menuId, timestamp])
      await tx.execute(`INSERT INTO sys_audit_log(actor_id,target_type,target_id,action,result,status_code,created_at)
        VALUES (?, 'role', ?, 'role_grant', 'success', 200, ?)`, [actor.id, routeId, timestamp])
    })
    return [...selected].sort((a, b) => a - b)
  }
  if (operation) error(404, 'Not found')
  if (request.method === 'GET') {
    requirePermission(state, 'system:role:list')
    const page = Number(url.searchParams.get('page') || 1)
    const pageSize = Number(url.searchParams.get('pageSize') || 10)
    if (!Number.isSafeInteger(page) || page < 1 || !Number.isSafeInteger(pageSize) || pageSize < 1 || pageSize > 100) error(400, '分页参数错误')
    const clauses = ['deleted_at IS NULL']; const values = []
    const keyword = (url.searchParams.get('keyword') || '').trim().slice(0, 100)
    if (keyword) { clauses.push('(name LIKE ? OR code LIKE ?)'); values.push(`%${keyword}%`, `%${keyword}%`) }
    const status = url.searchParams.get('status')
    if (status !== null) { if (!['0','1'].includes(status)) error(400, '状态参数错误'); clauses.push('status = ?'); values.push(Number(status)) }
    const where = clauses.join(' AND ')
    const [count] = await query(`SELECT COUNT(*) AS total FROM sys_role WHERE ${where}`, values)
    const rows = await query(`SELECT * FROM sys_role WHERE ${where} ORDER BY sort_order, id LIMIT ? OFFSET ?`, [...values, pageSize, (page - 1) * pageSize])
    return { list: await Promise.all(rows.map(roleDto)), total: Number(count.total) }
  }
  if (request.method === 'POST') {
    requirePermission(state, 'system:role:create')
    const input = await readJson(request); validateRole(input, true)
    const timestamp = Date.now()
    const result = await query(`INSERT INTO sys_role(code,name,description,sort_order,status,created_at,updated_at)
      VALUES(?,?,?,?,?,?,?)`, [input.code, input.name.trim(), input.description ?? null, input.sortOrder, input.status, timestamp, timestamp])
    const [row] = await query('SELECT * FROM sys_role WHERE id = ?', [result.insertId])
    return roleDto(row)
  }
  if (!routeId) return undefined
  const [existing] = await query('SELECT * FROM sys_role WHERE id = ? AND deleted_at IS NULL', [routeId])
  if (!existing) error(404, '角色不存在')
  if (request.method === 'DELETE') {
    requirePermission(state, 'system:role:delete')
    const [linked] = await query('SELECT COUNT(*) AS total FROM sys_user_role WHERE role_id = ?', [routeId])
    if (Number(linked.total)) error(409, '角色仍有关联用户')
    await query('UPDATE sys_role SET deleted_at = ?, updated_at = ? WHERE id = ?', [Date.now(), Date.now(), routeId])
    return null
  }
  if (request.method !== 'PUT') error(405, 'Method not allowed')
  requirePermission(state, 'system:role:update')
  const input = await readJson(request); validateRole(input, false)
  await query('UPDATE sys_role SET name = ?, description = ?, sort_order = ?, status = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL',
    [input.name.trim(), input.description ?? null, input.sortOrder, input.status, Date.now(), routeId])
  const [row] = await query('SELECT * FROM sys_role WHERE id = ?', [routeId])
  return roleDto(row)
}
