import { NextResponse, type NextRequest } from "next/server"

import {
  applicationQuery,
  authorizeApplicationReviewer,
  forwardOrganizerApplication,
  isOrganizerApplicationPage,
  parseApplicationFilters,
} from "@/features/organizations"

export async function GET(request: NextRequest) {
  const authorization = await authorizeApplicationReviewer()
  if (authorization instanceof NextResponse) return authorization

  const result = parseApplicationFilters(request.nextUrl.searchParams)
  if (result.error !== undefined) {
    return NextResponse.json({ detail: result.error }, { status: 400 })
  }

  return forwardOrganizerApplication(
    `/admin/organizer-applications?${applicationQuery(result.filters)}`,
    authorization.accessToken,
    isOrganizerApplicationPage
  )
}
