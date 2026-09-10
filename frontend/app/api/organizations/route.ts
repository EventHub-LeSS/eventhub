import { NextResponse, type NextRequest } from "next/server"

import { getSession, requireSession } from "@/features/auth"
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

  return NextResponse.json(body, { status: response.status })
}
