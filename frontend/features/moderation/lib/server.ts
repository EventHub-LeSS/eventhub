import "server-only"
import { NextResponse } from "next/server"

import { getCurrentUser, getSession } from "@/features/auth"
import { envConfig } from "@/features/shared/lib/env"

export async function authorizeModeration() {
  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { detail: "Anmeldung erforderlich." },
      { status: 401 }
    )
  }
  const user = await getCurrentUser()
  if (!user?.isAdmin) {
    return NextResponse.json(
      { detail: "Nur Administratoren dürfen Inhalte moderieren." },
      { status: 403 }
    )
  }
  return { accessToken: session.accessToken }
}

export async function forwardModeration<T>(
  path: string,
  accessToken: string,
  validate: (body: unknown) => body is T,
  payload?: { hidden: boolean; reason: string }
) {
  let response: Response
  try {
    response = await fetch(
      `${envConfig.apiBaseUrl}/moderation/ratings${path}`,
      {
        method: payload ? "PUT" : "GET",
        headers: {
          Authorization: `Bearer ${accessToken}`,
          ...(payload ? { "Content-Type": "application/json" } : {}),
        },
        ...(payload ? { body: JSON.stringify(payload) } : {}),
        cache: "no-store",
      }
    )
  } catch (error) {
    console.error("Moderation API could not be reached", error)
    return NextResponse.json(
      {
        detail:
          "Der Moderationsdienst ist nicht erreichbar. Bitte versuche es erneut.",
      },
      { status: 502 }
    )
  }

  let body: unknown
  try {
    body = await response.json()
  } catch (error) {
    console.error("Moderation API returned invalid JSON", {
      status: response.status,
      error,
    })
    return NextResponse.json(
      { detail: "Der Moderationsdienst hat eine ungültige Antwort geliefert." },
      { status: 502 }
    )
  }
  if (response.ok && !validate(body)) {
    console.error("Moderation API returned an invalid response", {
      status: response.status,
    })
    return NextResponse.json(
      { detail: "Die Antwort des Moderationsdienstes ist unvollständig." },
      { status: 502 }
    )
  }
  return NextResponse.json(body, {
    status: response.status,
    headers: { "Cache-Control": "no-store" },
  })
}
