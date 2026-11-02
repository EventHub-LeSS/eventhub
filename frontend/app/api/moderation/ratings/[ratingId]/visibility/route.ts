import { revalidatePath } from "next/cache"
import { NextResponse } from "next/server"

import {
  authorizeModeration,
  forwardModeration,
  isVisibilityResponse,
  isUUID,
  reasonError,
} from "@/features/moderation"

export async function PUT(
  request: Request,
  { params }: { params: Promise<{ ratingId: string }> }
) {
  const authorization = await authorizeModeration()
  if (authorization instanceof NextResponse) return authorization
  const { ratingId } = await params
  if (!isUUID(ratingId)) {
    return NextResponse.json(
      { detail: "Ungültige Bewertungs-ID." },
      { status: 400 }
    )
  }
  let body: unknown
  try {
    body = await request.json()
  } catch {
    return NextResponse.json(
      { detail: "Die Anfrage muss gültiges JSON enthalten." },
      { status: 400 }
    )
  }
  if (
    !body ||
    typeof body !== "object" ||
    !("hidden" in body) ||
    typeof body.hidden !== "boolean" ||
    !("reason" in body) ||
    typeof body.reason !== "string"
  ) {
    return NextResponse.json(
      { detail: "Sichtbarkeit und Begründung sind erforderlich." },
      { status: 400 }
    )
  }
  const error = reasonError(body.reason)
  if (error) return NextResponse.json({ detail: error }, { status: 400 })

  const response = await forwardModeration(
    `/${ratingId}/visibility`,
    authorization.accessToken,
    isVisibilityResponse,
    { hidden: body.hidden, reason: body.reason.trim() }
  )
  if (response.ok) {
    revalidatePath("/")
    revalidatePath("/events/[id]", "page")
  }
  return response
}
