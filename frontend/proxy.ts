import { NextResponse, type NextRequest } from "next/server"

import {
  refreshAccessToken,
  sealSession,
  sessionCookieOptions,
  unsealSession,
  type Session,
} from "@/features/auth"

const SESSION_COOKIE = "eh_session"

const REFRESH_BUFFER_MS = 30_000

/**
 * Behind a reverse proxy nextUrl.origin can be the internal container address
 * (0.0.0.0:3000), so the forwarded headers win when they are present.
 */
function externalOrigin(request: NextRequest) {
  const host = request.headers.get("x-forwarded-host")
  if (!host) {
    return request.nextUrl.origin
  }

  const proto = request.headers.get("x-forwarded-proto") ?? "https"

  return `${proto}://${host}`
}

function loginRedirect(request: NextRequest) {
  const loginUrl = new URL("/api/auth/login", externalOrigin(request))
  loginUrl.searchParams.set(
    "returnTo",
    `${request.nextUrl.pathname}${request.nextUrl.search}`
  )

  return NextResponse.redirect(loginUrl)
}

/** Keycloak's access token has a significantly shorter TTL than our session cookie. */
async function withRefreshedSession(
  request: NextRequest,
  session: Session
): Promise<NextResponse> {
  if (session.accessTokenExpiresAt - REFRESH_BUFFER_MS > Date.now()) {
    return NextResponse.next()
  }

  try {
    const refreshed = await refreshAccessToken(session.refreshToken)

    if (!refreshed.accessToken) {
      throw new Error("refresh returned no access token")
    }

    const updated: Session = {
      ...session,
      accessToken: refreshed.accessToken,
      refreshToken: refreshed.refreshToken,
      accessTokenExpiresAt: Date.now() + (refreshed.expiresIn ?? 0) * 1000,
    }
    const value = await sealSession(updated)

    request.cookies.set(SESSION_COOKIE, value)

    const response = NextResponse.next({ request })
    response.cookies.set(SESSION_COOKIE, value, sessionCookieOptions())

    return response
  } catch {
    // The refresh token itself expired (idle/SSO session timeout) or was revoked.
    const response = loginRedirect(request)
    response.cookies.delete(SESSION_COOKIE)

    return response
  }
}

/**
 * Optimistic check only. It just avoids rendering a protected page for someone who
 * clearly is not logged in; the real check happens in the Data Access Layer.
 */
export async function proxy(request: NextRequest) {
  const cookie = request.cookies.get(SESSION_COOKIE)?.value

  if (!cookie) {
    return loginRedirect(request)
  }

  const session = await unsealSession(cookie)

  if (!session || session.expiresAt < Date.now()) {
    return loginRedirect(request)
  }

  return withRefreshedSession(request, session)
}

export const config = {
  matcher: [
    "/tickets/:path*",
    "/favorites/:path*",
    "/organizer/:path*",
    "/organization/:path*",
    "/organizations/:path*",
    "/api/organizations/:path*",
  ],
}
