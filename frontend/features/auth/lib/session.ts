import "server-only"
import { EncryptJWT, jwtDecrypt } from "jose"
import { cookies } from "next/headers"

import { authConfig } from "@/features/auth/lib/config"

export const SESSION_COOKIE = "eh_session"
export const TRANSACTION_COOKIE = "eh_oidc_tx"

const SESSION_MAX_AGE_SECONDS = 60 * 60 * 8
const TRANSACTION_MAX_AGE_SECONDS = 60 * 10
const SESSION_CHUNK_SIZE = 3800

interface NamedCookie {
  name: string
  value: string
}

function chunkIndex(name: string) {
  const prefix = `${SESSION_COOKIE}.`
  if (!name.startsWith(prefix)) {
    return null
  }

  const index = Number(name.slice(prefix.length))

  return Number.isInteger(index) && index >= 0 ? index : null
}

/** Splits a sealed session into the cookies that should be set. */
export function splitSessionCookie(value: string): NamedCookie[] {
  const chunks: NamedCookie[] = []

  for (let i = 0; i * SESSION_CHUNK_SIZE < value.length; i++) {
    chunks.push({
      name: `${SESSION_COOKIE}.${i}`,
      value: value.slice(i * SESSION_CHUNK_SIZE, (i + 1) * SESSION_CHUNK_SIZE),
    })
  }

  return chunks
}

/** Reassembles the sealed session. A missing chunk yields a broken value, which fails to unseal. */
export function joinSessionCookie(cookies: NamedCookie[]): string | undefined {
  const chunks = cookies
    .map((cookie) => ({ index: chunkIndex(cookie.name), value: cookie.value }))
    .filter((chunk) => chunk.index !== null)
    .sort((a, b) => a.index! - b.index!)

  if (chunks.length === 0) {
    return undefined
  }

  return chunks.map((chunk) => chunk.value).join("")
}

/**
 * Session cookies to delete, either all of them or those beyond the first `keep` chunks
 * (left over from a larger session). Includes the unchunked cookie from before chunking.
 */
export function staleSessionCookieNames(
  cookies: { name: string }[],
  keep = 0
): string[] {
  return cookies
    .map((cookie) => cookie.name)
    .filter((name) => {
      if (name === SESSION_COOKIE) {
        return true
      }

      const index = chunkIndex(name)
      return index !== null && index >= keep
    })
}

export interface Session {
  sub: string
  email: string
  name: string
  /** Aliases of the Keycloak organizations the user belongs to. */
  organizations: string[]
  /** Keycloak access token, forwarded as a Bearer token to the backend API. */
  accessToken: string
  refreshToken: string
  accessTokenExpiresAt: number
  expiresAt: number
}

/** The pending OIDC login, kept in a cookie between /api/auth/login and the callback. */
export interface OidcTransaction {
  state: string
  nonce: string
  codeVerifier: string
  returnTo: string
}

function encryptionKey() {
  const key = Buffer.from(authConfig.sessionSecret, "base64")

  if (key.length !== 32) {
    throw new Error(
      "SESSION_SECRET must be 32 bytes encoded as base64. Generate one with: openssl rand -base64 32"
    )
  }

  return new Uint8Array(key)
}

async function seal(payload: object, maxAgeSeconds: number) {
  return new EncryptJWT({ ...payload })
    .setProtectedHeader({ alg: "dir", enc: "A256GCM" })
    .setIssuedAt()
    .setExpirationTime(`${maxAgeSeconds}s`)
    .encrypt(encryptionKey())
}

async function unseal<T>(value: string | undefined): Promise<T | null> {
  if (!value) {
    return null
  }

  try {
    const { payload } = await jwtDecrypt(value, encryptionKey())
    return payload as T
  } catch {
    // Expired or tampered with, so treat it as "not logged in".
    return null
  }
}

function cookieOptions(maxAge: number) {
  return {
    httpOnly: true,
    sameSite: "lax" as const,
    secure: process.env.NODE_ENV === "production",
    path: "/",
    maxAge,
  }
}

/** Exported so the refresh middleware (which uses the NextRequest/NextResponse cookie API instead of next/headers) can reseal a session without duplicating the crypto setup. */
export async function sealSession(session: Session) {
  return seal(session, SESSION_MAX_AGE_SECONDS)
}

export async function unsealSession(value: string | undefined) {
  return unseal<Session>(value)
}

export const sessionCookieOptions = () => cookieOptions(SESSION_MAX_AGE_SECONDS)

export async function createSessionCookie(session: Session) {
  const chunks = splitSessionCookie(await sealSession(session))
  const store = await cookies()

  for (const name of staleSessionCookieNames(store.getAll(), chunks.length)) {
    store.delete(name)
  }

  for (const chunk of chunks) {
    store.set(chunk.name, chunk.value, cookieOptions(SESSION_MAX_AGE_SECONDS))
  }
}

export async function readSession(): Promise<Session | null> {
  const store = await cookies()
  const session = await unsealSession(joinSessionCookie(store.getAll()))

  if (!session || session.expiresAt < Date.now()) {
    return null
  }

  return session
}

export async function clearSessionCookie() {
  const store = await cookies()

  for (const name of staleSessionCookieNames(store.getAll())) {
    store.delete(name)
  }
}

export async function createTransactionCookie(transaction: OidcTransaction) {
  const value = await seal(transaction, TRANSACTION_MAX_AGE_SECONDS)
  const store = await cookies()

  store.set(
    TRANSACTION_COOKIE,
    value,
    cookieOptions(TRANSACTION_MAX_AGE_SECONDS)
  )
}

/** Reads and immediately invalidates the transaction so a login attempt cannot be replayed. */
export async function consumeTransactionCookie(): Promise<OidcTransaction | null> {
  const store = await cookies()
  const transaction = await unseal<OidcTransaction>(
    store.get(TRANSACTION_COOKIE)?.value
  )

  store.delete(TRANSACTION_COOKIE)

  return transaction
}

export const sessionMaxAgeSeconds = SESSION_MAX_AGE_SECONDS
