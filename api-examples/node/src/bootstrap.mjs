import { pool } from './db.mjs'
import { hashPassword, now } from './security.mjs'

// A MySQL advisory lock makes the empty-database bootstrap safe across app instances.
export async function ensureDemoAdmin() {
  const connection = await pool.getConnection()
  if ((process.env.APP_ENV || 'development') === 'production') {
    try {
      const [existing] = await connection.execute('SELECT COUNT(*) AS total FROM sys_user')
      if (Number(existing[0].total) === 0) throw new Error('Production startup refuses the demo admin/123456 bootstrap; provision a strong administrator password explicitly.')
      return false
    } finally { connection.release() }
  }
  const [lock] = await connection.execute("SELECT GET_LOCK('npora_node_initial_admin', 15) AS acquired")
  if (Number(lock[0]?.acquired) !== 1) { connection.release(); throw new Error('Could not acquire initial-admin bootstrap lock') }
  try {
    const [count] = await connection.execute('SELECT COUNT(*) AS total FROM sys_user')
    if (Number(count[0].total) !== 0) return false
    const username = 'admin'
    const passwordHash = await hashPassword('123456')
    const timestamp = now()
    await connection.beginTransaction()
    try {
      const [users] = await connection.execute('SELECT COUNT(*) AS total FROM sys_user FOR UPDATE')
      if (Number(users[0].total) !== 0) { await connection.rollback(); return false }
      const [created] = await connection.execute(`INSERT INTO sys_user
        (username,password_hash,display_name,status,is_super_admin,created_at,updated_at)
        VALUES(?,?,?,1,1,?,?)`, [username, passwordHash, '超级管理员', timestamp, timestamp])
      const [roles] = await connection.execute('SELECT id FROM sys_role WHERE code=? AND deleted_at IS NULL LIMIT 1', ['admin'])
      if (roles[0]) await connection.execute('INSERT INTO sys_user_role(user_id,role_id,created_at) VALUES(?,?,?)', [created.insertId, roles[0].id, timestamp])
      await connection.commit()
    } catch (error) { await connection.rollback(); throw error }
    return true
  } finally {
    try { await connection.execute("SELECT RELEASE_LOCK('npora_node_initial_admin')") } finally { connection.release() }
  }
}
