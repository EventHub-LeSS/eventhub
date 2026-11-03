import { NextResponse } from "next/server"

import { getSession } from "@/features/auth"
import { envConfig } from "@/features/shared/lib/env"

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export async function PATCH(
  _request: Request,
  { params }: { params: Promise<{ notificationId: string }> }
) {
  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Anmeldung erforderlich" } },
      { status: 401 }
    )
  }

  const { notificationId } = await params
  if (!UUID_PATTERN.test(notificationId)) {
    return NextResponse.json(
      { detail: "Ungültige Benachrichtigungs-ID." },
      { status: 400 }
    )
  }

  const response = await fetch(
    `${envConfig.apiBaseUrl}/notifications/${notificationId}/read`,
    {
      method: "PATCH",
      headers: { Authorization: `Bearer ${session.accessToken}` },
      cache: "no-store",
    }
  )

  const body = await response.json().catch(() => null)

  return NextResponse.json(body, { status: response.status })
}
