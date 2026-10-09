import { NextResponse, type NextRequest } from "next/server"

import {
  createSessionCookie,
  getSession,
  organizationsFromClaims,
  refreshAccessToken,
  requireSession,
  setActiveOrganizationCookie,
} from "@/features/auth"
import type { OrganizationPayload } from "@/features/organizations"

export async function POST(request: NextRequest) {
  await requireSession()

  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Authentication is required" } },
      { status: 401 }
    )
  }

  const payload = (await request.json()) as OrganizationPayload

  const response = await fetch(`${process.env.BACKEND_API_URL}/organizations/`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${session.accessToken}`,
    },
    body: JSON.stringify(payload),
  })

  const text = await response.text()
  let body: unknown = null

  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = { error: { code: "BAD_GATEWAY", message: text } }
    }
  }

  const selfAdmin = payload.orgAdmin.trim().toLowerCase() === session.email.trim().toLowerCase()
  const alias = (body as { alias?: unknown } | null)?.alias

  if (response.ok && selfAdmin && typeof alias === "string") {
    try {
      const refreshed = await refreshAccessToken(session.refreshToken)

      if (refreshed.accessToken) {
        await createSessionCookie({
          ...session,
          organizations: refreshed.claims
            ? organizationsFromClaims(refreshed.claims.organization)
            : session.organizations,
          accessToken: refreshed.accessToken,
          refreshToken: refreshed.refreshToken,
          accessTokenExpiresAt: Date.now() + (refreshed.expiresIn ?? 0) * 1000,
        })
      }
    } catch {
    }

    await setActiveOrganizationCookie(alias)
  }

  return NextResponse.json(
    response.ok ? { ...(body as object), selfAdmin } : body,
    { status: response.status }
  )
}
