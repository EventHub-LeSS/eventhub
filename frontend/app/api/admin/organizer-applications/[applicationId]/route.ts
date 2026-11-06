import { NextResponse } from "next/server"

import {
  authorizeApplicationReviewer,
  forwardOrganizerApplication,
  isApplicationUUID,
  isOrganizerApplication,
} from "@/features/organizations"

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ applicationId: string }> }
) {
  const authorization = await authorizeApplicationReviewer()
  if (authorization instanceof NextResponse) return authorization

  const { applicationId } = await params
  if (!isApplicationUUID(applicationId)) {
    return NextResponse.json(
      { detail: "Ungültige Antrags-ID." },
      { status: 400 }
    )
  }

  return forwardOrganizerApplication(
    `/admin/organizer-applications/${applicationId}`,
    authorization.accessToken,
    isOrganizerApplication
  )
}
