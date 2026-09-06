import { NextResponse, type NextRequest } from "next/server"

const SESSION_COOKIE = "eh_session"

/**
 * Optimistic check only. It just avoids rendering a protected page for someone who
 * clearly is not logged in; the real check happens in the Data Access Layer.
 */
export function proxy(request: NextRequest) {
  if (request.cookies.has(SESSION_COOKIE)) {
    return NextResponse.next()
  }

  const loginUrl = new URL("/api/auth/login", request.nextUrl.origin)
  loginUrl.searchParams.set(
    "returnTo",
    `${request.nextUrl.pathname}${request.nextUrl.search}`
  )

  return NextResponse.redirect(loginUrl)
}

export const config = {
  matcher: ["/tickets/:path*", "/favorites/:path*", "/organizer/:path*"],
}
