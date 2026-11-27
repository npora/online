const number = (name, fallback, min, max) => {
  const value = Number(process.env[name] ?? fallback)
  if (!Number.isSafeInteger(value) || value < min || value > max) {
    throw new Error(`${name} must be an integer between ${min} and ${max}`)
  }
  return value
}

export const config = Object.freeze({
  host: process.env.HOST || '0.0.0.0',
  port: number('PORT', 3000, 1, 65535),
  db: {
    host: process.env.DB_HOST || '127.0.0.1',
    port: number('DB_PORT', 3306, 1, 65535),
    user: process.env.DB_USER || 'npora',
    password: process.env.DB_PASSWORD || '',
    database: process.env.DB_NAME || 'npora_admin',
    connectionLimit: number('DB_CONNECTION_LIMIT', 10, 1, 100)
  },
  pepper: process.env.AUTH_PEPPER || '',
  totpKey: process.env.TOTP_ENCRYPTION_KEY || '',
  defaultTtlMs: number('SESSION_DEFAULT_TTL_MS', 8 * 60 * 60 * 1000, 60_000, 365 * 24 * 60 * 60 * 1000),
  rememberTtlMs: number('SESSION_REMEMBER_TTL_MS', 30 * 24 * 60 * 60 * 1000, 60_000, 365 * 24 * 60 * 60 * 1000),
  totpEnabled: process.env.AUTH_TOTP_ENABLED === 'true',
  corsOrigins: new Set((process.env.CORS_ORIGINS || '').split(',').map((v) => v.trim()).filter(Boolean))
})

export function validateSecrets() {
  if (Buffer.byteLength(config.pepper, 'hex') !== 32 || !/^[0-9a-f]{64}$/i.test(config.pepper)) {
    throw new Error('AUTH_PEPPER must be 32 random bytes encoded as 64 hexadecimal characters')
  }
  if (Buffer.byteLength(config.totpKey, 'hex') !== 32 || !/^[0-9a-f]{64}$/i.test(config.totpKey)) {
    throw new Error('TOTP_ENCRYPTION_KEY must be 32 random bytes encoded as 64 hexadecimal characters')
  }
}
