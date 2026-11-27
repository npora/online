import { DatabaseSync } from 'node:sqlite'
import { readFileSync, readdirSync } from 'node:fs'

// Execute the online-owned SQLite fixture with a small D1-compatible adapter.
export function database() {
  const sqlite = new DatabaseSync(':memory:')
  sqlite.exec('PRAGMA foreign_keys = ON')
  try {
    const migrationDirectory = new URL('../../../api/cloudflare/migrations/', import.meta.url)
    const migrations = readdirSync(migrationDirectory)
      .filter((file) => /^\d{4}_.+\.sql$/.test(file))
      .sort()
    for (const file of migrations) {
      sqlite.exec('BEGIN')
      try {
        sqlite.exec(readFileSync(new URL(file, migrationDirectory), 'utf8'))
        sqlite.exec('COMMIT')
      } catch (error) {
        sqlite.exec('ROLLBACK')
        throw error
      }
    }
  } catch (error) {
    sqlite.close()
    throw error
  }
  const db = {
    prepare(sql) {
      const order = []
      const statement = sqlite.prepare(sql.replace(/\?(\d+)/g, (_, index) => {
        order.push(Number(index) - 1)
        return '?'
      }))
      let parameters = []
      const values = () => (order.length ? order.map((index) => parameters[index]) : parameters)
      const totalChanges = () => Number(sqlite.prepare('SELECT total_changes() AS count').get().count)
      const execute = () => {
        const before = totalChanges()
        const results = statement.all(...values()).map((row) => ({ ...row }))
        return { success: true, results, meta: { changes: totalChanges() - before } }
      }
      return {
        bind(...values) { parameters = values; return this },
        async first() { return statement.get(...values()) ?? null },
        async all() { return execute() },
        async run() { return execute() },
        execute
      }
    },
    async batch(statements) {
      sqlite.exec('BEGIN')
      try {
        const results = statements.map((statement) => statement.execute())
        sqlite.exec('COMMIT')
        return results
      } catch (error) {
        sqlite.exec('ROLLBACK')
        throw error
      }
    }
  }
  return { db, sqlite, close: () => sqlite.close() }
}
