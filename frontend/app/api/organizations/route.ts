import { NextResponse, type NextRequest } from "next/server"

import { requireSession } from "@/features/auth"
import type { OrganizationPayload } from "@/features/organizations"

export async function POST(request: NextRequest) {
  await requireSession()

  const payload = (await request.json()) as OrganizationPayload

  const response = await fetch(`${process.env.BACKEND_API_URL}/organizations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })

  const body = await response.json()

  return NextResponse.json(body, { status: response.status })
}
