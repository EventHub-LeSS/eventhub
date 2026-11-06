import { NextResponse } from "next/server"

import { envConfig } from "@/features/shared/lib/env"

/**
 * EVENTHUB-264: serves the image of a published event. The API is only
 * reachable from the server, so the browser loads it through here. Public like
 * the event details; events without an image answer 404.
 */
export async function GET(
  _request: Request,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params
  const response = await fetch(
    `${envConfig.apiBaseUrl}/events/${encodeURIComponent(id)}/image`,
    { cache: "no-store" }
  ).catch(() => null)

  if (!response?.ok) {
    return new NextResponse(null, { status: response?.status ?? 502 })
  }

  return new NextResponse(response.body, {
    headers: {
      "Content-Type": response.headers.get("Content-Type") ?? "image/png",
      // A new upload replaces the image under the same URL.
      "Cache-Control": "no-cache",
    },
  })
}
