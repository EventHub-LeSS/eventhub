import { NextResponse, type NextRequest } from "next/server"

import {
  authorizeModeration,
  forwardModeration,
  isRatingPage,
  moderationQuery,
  parseModerationFilters,
} from "@/features/moderation"

export async function GET(request: NextRequest) {
  const authorization = await authorizeModeration()
  if (authorization instanceof NextResponse) return authorization

  const result = parseModerationFilters(request.nextUrl.searchParams)
  if (result.error !== undefined) {
    return NextResponse.json({ detail: result.error }, { status: 400 })
  }
  return forwardModeration(
    `?${moderationQuery(result.filters)}`,
    authorization.accessToken,
    isRatingPage
  )
}
