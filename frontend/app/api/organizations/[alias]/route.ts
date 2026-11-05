import { NextResponse, type NextRequest } from "next/server"

import { getSession, requireSession } from "@/features/auth"
import { envConfig } from "@/features/shared/lib/env"

async function proxyOrganization(
  request: Request,
  session: { accessToken: string },
  alias: string,
  init?: RequestInit
) {
  const response = await fetch(
    `${envConfig.apiBaseUrl}/organizations/${alias}`,
    {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${session.accessToken}`,
      },
      ...init,
    }
  )

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

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ alias: string }> }
) {
  await requireSession()

  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Anmeldung erforderlich" } },
      { status: 401 }
    )
  }

  const { alias } = await params

  if (!session.organizations.includes(alias)) {
    return NextResponse.json(
      {
        error: {
          code: "FORBIDDEN",
          message: "Du bist kein Mitglied dieser Organisation",
        },
      },
      { status: 403 }
    )
  }

  return proxyOrganization(_request, session, alias)
}

export async function PUT(
  request: NextRequest,
  { params }: { params: Promise<{ alias: string }> }
) {
  await requireSession()

  const session = await getSession()
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Anmeldung erforderlich" } },
      { status: 401 }
    )
  }

  const { alias } = await params

  if (!session.organizations.includes(alias)) {
    return NextResponse.json(
      {
        error: {
          code: "FORBIDDEN",
          message: "Du bist kein Mitglied dieser Organisation",
        },
      },
      { status: 403 }
    )
  }

  const payload = (await request.json()) as { name?: string }
  const name = payload.name?.trim()

  if (!name) {
    return NextResponse.json(
      { error: { code: "VALIDATION_ERROR", message: "name is required" } },
      { status: 400 }
    )
  }

  return proxyOrganization(request, session, alias, {
    method: "PUT",
    body: JSON.stringify({ name }),
  })
}
