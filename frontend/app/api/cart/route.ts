import { NextResponse } from "next/server"

import { getSession } from "@/features/auth"
import { envConfig } from "@/features/shared/lib/env"

/** Forwards to the API with the server-held token; the API only returns the caller's own cart. */
export async function GET() {
  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Anmeldung erforderlich" } },
      { status: 401 }
    )
  }

  const response = await fetch(`${envConfig.apiBaseUrl}/cart`, {
    headers: { Authorization: `Bearer ${session.accessToken}` },
    cache: "no-store",
  })

  const body = await response.json().catch(() => null)

  return NextResponse.json(body, { status: response.status })
}
