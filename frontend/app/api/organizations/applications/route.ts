import { NextResponse } from "next/server"

import {
  createSessionCookie,
  organizationsFromClaims,
  refreshAccessToken,
  setActiveOrganizationCookie,
} from "@/features/auth"
import {
  authorizeApplicationSession,
  forwardOrganizerApplication,
  isOrganizerApplication,
  isOrganizerApplicationList,
  parseApplicationSubmissionBody,
} from "@/features/organizations"
import { envConfig } from "@/features/shared/lib/env"

export async function GET() {
  const authorization = await authorizeApplicationSession()
  if (authorization instanceof NextResponse) return authorization

  const { session } = authorization
  let response: Response
  try {
    response = await fetch(
      `${envConfig.apiBaseUrl}/me/organizer-applications`,
      {
        headers: {
          Authorization: `Bearer ${session.accessToken}`,
        },
        cache: "no-store",
      }
    )
  } catch (error) {
    console.error("Failed to fetch own organizer applications", error)
    return NextResponse.json(
      {
        detail:
          "Der Dienst für Veranstalterbewerbungen ist nicht erreichbar. Bitte versuche es erneut.",
      },
      { status: 502 }
    )
  }

  let body: unknown
  try {
    body = await response.json()
  } catch {
    return NextResponse.json(
      {
        detail:
          "Der Dienst für Veranstalterbewerbungen hat eine ungültige Antwort geliefert.",
      },
      { status: 502 }
    )
  }

  if (response.ok && !isOrganizerApplicationList(body)) {
    return NextResponse.json(
      {
        detail:
          "Die Antwort des Dienstes für Veranstalterbewerbungen ist unvollständig.",
      },
      { status: 502 }
    )
  }

  if (response.ok && isOrganizerApplicationList(body)) {
    const newlyApproved = body.find(
      (item) =>
        item.status === "approved" &&
        !session.organizations.includes(item.organizationAlias)
    )
    if (newlyApproved) {
      try {
        const refreshed = await refreshAccessToken(session.refreshToken)
        if (refreshed.accessToken) {
          const updatedOrganizations = refreshed.claims
            ? organizationsFromClaims(refreshed.claims.organization)
            : session.organizations
          await createSessionCookie({
            ...session,
            organizations: updatedOrganizations,
            accessToken: refreshed.accessToken,
            refreshToken: refreshed.refreshToken,
            accessTokenExpiresAt:
              Date.now() + (refreshed.expiresIn ?? 0) * 1000,
          })
          if (
            !session.organizations.length &&
            updatedOrganizations.includes(newlyApproved.organizationAlias)
          ) {
            await setActiveOrganizationCookie(newlyApproved.organizationAlias)
          }
        }
      } catch {
        // Ignore refresh failures; user can still re-login to pick up the new organization claim.
      }
    }
  }

  return NextResponse.json(body, {
    status: response.status,
    headers: { "Cache-Control": "no-store" },
  })
}

export async function POST(request: Request) {
  const authorization = await authorizeApplicationSession()
  if (authorization instanceof NextResponse) return authorization

  let body: unknown
  try {
    body = await request.json()
  } catch {
    return NextResponse.json(
      { detail: "Die Anfrage muss gültiges JSON enthalten." },
      { status: 400 }
    )
  }

  const parsed = parseApplicationSubmissionBody(body)
  if (parsed.error !== undefined) {
    return NextResponse.json({ detail: parsed.error }, { status: 400 })
  }

  return forwardOrganizerApplication(
    "/organizer-applications",
    authorization.session.accessToken,
    isOrganizerApplication,
    {
      method: "POST",
      payload: parsed.payload,
    }
  )
}
