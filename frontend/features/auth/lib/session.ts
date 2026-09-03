import "server-only"
import { EncryptJWT, jwtDecrypt } from "jose"
import { cookies } from "next/headers"

import { authConfig } from "@/features/auth/lib/config"

export const SESSION_COOKIE = "eh_session"
export const TRANSACTION_COOKIE = "eh_oidc_tx"

const SESSION_MAX_AGE_SECONDS = 60 * 60 * 8
const TRANSACTION_MAX_AGE_SECONDS = 60 * 10

export interface Session {
  sub: string
  email: string
  name: string
  /** Aliases of the Keycloak organizations the user belongs to. */
  organizations: string[]
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

export async function createSessionCookie(session: Session) {
  const value = await seal(session, SESSION_MAX_AGE_SECONDS)
  const store = await cookies()

  store.set(SESSION_COOKIE, value, cookieOptions(SESSION_MAX_AGE_SECONDS))
}

export async function readSession(): Promise<Session | null> {
  const store = await cookies()
  const session = await unseal<Session>(store.get(SESSION_COOKIE)?.value)

  if (!session || session.expiresAt < Date.now()) {
    return null
  }

  return session
}

export async function clearSessionCookie() {
  const store = await cookies()

  store.delete(SESSION_COOKIE)
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
