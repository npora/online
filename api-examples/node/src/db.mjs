import mysql from 'mysql2/promise'
import { config } from './config.mjs'

export const pool = mysql.createPool({
  ...config.db,
  waitForConnections: true,
  connectionLimit: config.db.connectionLimit,
  queueLimit: 0,
  charset: 'utf8mb4',
  timezone: 'Z',
  supportBigNumbers: true,
  bigNumberStrings: false,
  decimalNumbers: true
})

export async function query(sql, values = []) {
  const [rows] = await pool.execute(sql, values)
  return rows
}

export async function transaction(fn) {
  const connection = await pool.getConnection()
  try {
    await connection.beginTransaction()
    const result = await fn({
      execute: (sql, values = []) => connection.execute(sql, values),
      query: async (sql, values = []) => (await connection.execute(sql, values))[0]
    })
    await connection.commit()
    return result
  } catch (error) {
    await connection.rollback()
    throw error
  } finally {
    connection.release()
  }
}
