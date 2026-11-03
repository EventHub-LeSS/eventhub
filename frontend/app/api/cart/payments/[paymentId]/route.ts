import { NextResponse } from "next/server"

import { getSession } from "@/features/auth"
import { envConfig } from "@/features/shared/lib/env"

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/** Forwards to the API with the server-held token; the API only returns the caller's own payments. */
export async function GET(
  _request: Request,
  { params }: { params: Promise<{ paymentId: string }> }
) {
  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Anmeldung erforderlich" } },
      { status: 401 }
    )
  }

  const { paymentId } = await params
  if (!UUID_PATTERN.test(paymentId)) {
    return NextResponse.json(
      { detail: "Ungültige Zahlungs-ID." },
      { status: 400 }
    )
  }

  const response = await fetch(
    `${envConfig.apiBaseUrl}/payments/${paymentId}`,
    {
      headers: { Authorization: `Bearer ${session.accessToken}` },
      cache: "no-store",
    }
  )

  const body = await response.json().catch(() => null)

  return NextResponse.json(body, { status: response.status })
}
