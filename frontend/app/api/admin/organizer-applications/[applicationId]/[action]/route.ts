import { NextResponse } from "next/server"

import {
  authorizeApplicationReviewer,
  forwardOrganizerApplication,
  isApplicationUUID,
  isOrganizerApplication,
  validateDecisionPayload,
} from "@/features/organizations"

export async function POST(
  request: Request,
  { params }: { params: Promise<{ applicationId: string; action: string }> }
) {
  const authorization = await authorizeApplicationReviewer()
  if (authorization instanceof NextResponse) return authorization

  const { applicationId, action } = await params
  if (!isApplicationUUID(applicationId)) {
    return NextResponse.json(
      { detail: "Ungültige Antrags-ID." },
      { status: 400 }
    )
  }

  if (action !== "approve" && action !== "reject") {
    return NextResponse.json({ detail: "Unbekannte Aktion." }, { status: 404 })
  }

  let body: unknown = {}
  const rawText = await request.text()
  if (rawText.trim() !== "") {
    try {
      body = JSON.parse(rawText)
    } catch {
      return NextResponse.json(
        { detail: "Die Anfrage muss gültiges JSON enthalten." },
        { status: 400 }
      )
    }
  }

  const validated = validateDecisionPayload(body, action)
  if (validated.error !== undefined) {
    return NextResponse.json({ detail: validated.error }, { status: 400 })
  }

  return forwardOrganizerApplication(
    `/admin/organizer-applications/${applicationId}/${action}`,
    authorization.accessToken,
    isOrganizerApplication,
    {
      method: "POST",
      payload: validated.payload,
    }
  )
}
