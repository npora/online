import {
  createCipheriv,
  createDecipheriv,
  createHash,
  createHmac,
  randomBytes,
  scrypt as scryptCallback,
  timingSafeEqual
} from 'node:crypto'
import { promisify } from 'node:util'
import { config } from './config.mjs'

const scrypt = promisify(scryptCallback)

export const sha256 = (value) => createHash('sha256').update(value).digest('hex')
export const token = () => randomBytes(32).toString('base64url')
export const sessionId = () => randomBytes(16).toString('hex')
export const now = () => Date.now()

export async function hashPassword(password) {
  const salt = randomBytes(16)
  const derived = await scrypt(`${password}\0${config.pepper}`, salt, 64, { N: 1 << 15, r: 8, p: 1, maxmem: 64 * 1024 * 1024 })
  return `scrypt$32768$8$1$${salt.toString('base64url')}$${derived.toString('base64url')}`
}

export async function verifyPassword(password, encoded) {
  const match = /^scrypt\$(\d+)\$(\d+)\$(\d+)\$([A-Za-z0-9_-]+)\$([A-Za-z0-9_-]+)$/.exec(encoded || '')
  if (!match) return false
  const [, N, r, p, saltText, digestText] = match
  const salt = Buffer.from(saltText, 'base64url')
  const expected = Buffer.from(digestText, 'base64url')
  if (salt.length !== 16 || expected.length !== 64 || Number(N) !== 32768 || Number(r) !== 8 || Number(p) !== 1) return false
  const actual = await scrypt(`${password}\0${config.pepper}`, salt, expected.length, { N: 32768, r: 8, p: 1, maxmem: 64 * 1024 * 1024 })
  return timingSafeEqual(expected, actual)
}

export function encryptSecret(value) {
  const key = Buffer.from(config.totpKey, 'hex')
  const iv = randomBytes(12)
  const cipher = createCipheriv('aes-256-gcm', key, iv)
  const ciphertext = Buffer.concat([cipher.update(value, 'utf8'), cipher.final()])
  return `${iv.toString('base64url')}.${cipher.getAuthTag().toString('base64url')}.${ciphertext.toString('base64url')}`
}

export function decryptSecret(value) {
  const [ivText, tagText, ciphertextText] = value.split('.')
  const decipher = createDecipheriv('aes-256-gcm', Buffer.from(config.totpKey, 'hex'), Buffer.from(ivText, 'base64url'))
  decipher.setAuthTag(Buffer.from(tagText, 'base64url'))
  return Buffer.concat([decipher.update(Buffer.from(ciphertextText, 'base64url')), decipher.final()]).toString('utf8')
}

const base32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
export function makeTotpSecret() {
  const bytes = randomBytes(20)
  let bits = ''
  for (const byte of bytes) bits += byte.toString(2).padStart(8, '0')
  let result = ''
  for (let i = 0; i < bits.length; i += 5) result += base32[parseInt(bits.slice(i, i + 5).padEnd(5, '0'), 2)]
  return result
}

function decodeBase32(value) {
  let bits = ''
  for (const char of value.toUpperCase().replace(/=+$/g, '').replace(/\s/g, '')) {
    const index = base32.indexOf(char)
    if (index < 0) throw new Error('Invalid TOTP secret')
    bits += index.toString(2).padStart(5, '0')
  }
  const bytes = []
  for (let i = 0; i + 8 <= bits.length; i += 8) bytes.push(parseInt(bits.slice(i, i + 8), 2))
  return Buffer.from(bytes)
}

export function totpStep(secret, code, time = now()) {
  if (!/^\d{6}$/.test(code)) return null
  const counter = Math.floor(time / 30_000)
  const key = decodeBase32(secret)
  for (const step of [counter - 1, counter, counter + 1]) {
    if (step < 0) continue
    const msg = Buffer.alloc(8)
    msg.writeBigUInt64BE(BigInt(step))
    const digest = createHmac('sha1', key).update(msg).digest()
    const offset = digest.at(-1) & 0x0f
    const binary = digest.readUInt32BE(offset) & 0x7fffffff
    if ((binary % 1_000_000).toString().padStart(6, '0') === code) return step
  }
  return null
}
