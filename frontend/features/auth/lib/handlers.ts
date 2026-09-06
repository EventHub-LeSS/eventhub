import "server-only"
import { NextResponse, type NextRequest } from "next/server"

import { setActiveOrganizationCookie } from "@/features/auth/lib/active-organization"
import { authConfig } from "@/features/auth/lib/config"
import {
  buildLogoutUrl,
  createAuthorizationRequest,
  exchangeCode,
} from "@/features/auth/lib/oidc"
import { safeReturnTo } from "@/features/auth/lib/return-to"
import {
  clearSessionCookie,
  consumeTransactionCookie,
  createSessionCookie,
  createTransactionCookie,
  readSession,
  sessionMaxAgeSeconds,
} from "@/features/auth/lib/session"
import { organizationsFromClaims } from "@/features/auth/lib/user"

/** Shared by /api/auth/login and /api/auth/register, which differ only in the prompt. */
export async function startAuthorization(
  request: NextRequest,
  prompt?: string
) {
  const returnTo = safeReturnTo(request.nextUrl.searchParams.get("returnTo"))
  const { url, state, nonce, codeVerifier } = await createAuthorizationRequest({
    prompt,
  })

  await createTransactionCookie({ state, nonce, codeVerifier, returnTo })

  return NextResponse.redirect(url)
}

function failed(request: NextRequest, reason: string) {
  const url = new URL("/", request.nextUrl.origin)
  url.searchParams.set("authError", reason)

  return NextResponse.redirect(url)
}

export async function handleCallback(request: NextRequest) {
  const transaction = await consumeTransactionCookie()

  if (request.nextUrl.searchParams.has("error")) {
    return failed(
      request,
      request.nextUrl.searchParams.get("error") ?? "unknown"
    )
  }

  if (!transaction) {
    return failed(request, "expired")
  }

  let claims
  try {
    claims = await exchangeCode(new URL(request.url), transaction)
  } catch {
    return failed(request, "exchange_failed")
  }

  if (!claims) {
    return failed(request, "no_id_token")
  }

  const email = typeof claims.email === "string" ? claims.email : ""

  await createSessionCookie({
    sub: claims.sub,
    email,
    name: typeof claims.name === "string" ? claims.name : email,
    organizations: organizationsFromClaims(claims.organization),
    expiresAt: Date.now() + sessionMaxAgeSeconds * 1000,
  })

  return NextResponse.redirect(
    new URL(safeReturnTo(transaction.returnTo), request.nextUrl.origin)
  )
}

export async function handleLogout() {
  await clearSessionCookie()

  const logoutUrl = await buildLogoutUrl(`${authConfig.appBaseUrl}/`)

  // 303 makes the browser follow the redirect with GET instead of repeating the POST.
  return NextResponse.redirect(logoutUrl, 303)
}

export async function handleOrganizationSwitch(request: NextRequest) {
  const form = await request.formData()
  const session = await readSession()
  const alias = form.get("organization")

  // The submitted alias is only accepted if the token says the user is a member.
  if (
    session &&
    typeof alias === "string" &&
    session.organizations.includes(alias)
  ) {
    await setActiveOrganizationCookie(alias)
  }

  const returnTo = safeReturnTo(form.get("returnTo")?.toString())

  return NextResponse.redirect(new URL(returnTo, request.nextUrl.origin), 303)
}
