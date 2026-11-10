import "server-only"
import { NextResponse } from "next/server"

import { getCurrentUser, getSession } from "@/features/auth"
import { envConfig } from "@/features/shared/lib/env"

export async function authorizeApplicationSession() {
  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { detail: "Anmeldung erforderlich." },
      { status: 401 }
    )
  }
  return { session }
}

export async function authorizeApplicationReviewer() {
  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { detail: "Anmeldung erforderlich." },
      { status: 401 }
    )
  }
  const user = await getCurrentUser()
  // EVENTHUB-304: only EventHub admins review applications, org admins do not.
  if (!user?.isAdmin) {
    return NextResponse.json(
      {
        detail:
          "Nur Administratoren dürfen Veranstalterbewerbungen prüfen und entscheiden.",
      },
      { status: 403 }
    )
  }
  return { accessToken: session.accessToken }
}

export async function forwardOrganizerApplication<T>(
  path: string,
  accessToken: string,
  validate: (body: unknown) => body is T,
  options?: {
    method?: "GET" | "POST"
    payload?: unknown
  }
) {
  let response: Response
  try {
    response = await fetch(`${envConfig.apiBaseUrl}${path}`, {
      method:
        options?.method ?? (options?.payload !== undefined ? "POST" : "GET"),
      headers: {
        Authorization: `Bearer ${accessToken}`,
        ...(options?.payload !== undefined
          ? { "Content-Type": "application/json" }
          : {}),
      },
      ...(options?.payload !== undefined
        ? { body: JSON.stringify(options.payload) }
        : {}),
      cache: "no-store",
    })
  } catch (error) {
    console.error("Organizer application API could not be reached", error)
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
  } catch (error) {
    console.error("Organizer application API returned invalid JSON", {
      status: response.status,
      error,
    })
    return NextResponse.json(
      {
        detail:
          "Der Dienst für Veranstalterbewerbungen hat eine ungültige Antwort geliefert.",
      },
      { status: 502 }
    )
  }

  if (response.ok && !validate(body)) {
    console.error("Organizer application API returned an incomplete response", {
      status: response.status,
    })
    return NextResponse.json(
      {
        detail:
          "Die Antwort des Dienstes für Veranstalterbewerbungen ist unvollständig.",
      },
      { status: 502 }
    )
  }

  return NextResponse.json(body, {
    status: response.status,
    headers: { "Cache-Control": "no-store" },
  })
}
