import "server-only"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { envConfig } from "@/features/shared/lib/env"

export interface PublishedEventDetails extends PublishedEvent {
  description: string | null
  capacity: number
  availableSeats: number
  bookable: boolean
  soldTickets: number
  occupancyPercent: number
  availability:
    | "available"
    | "almost_sold_out"
    | "sold_out"
    | "temporarily_unavailable"
}

type Result =
  | { status: "ok"; event: PublishedEventDetails }
  | { status: "not-found" | "unavailable" }

export async function getPublishedEvent(eventId: string): Promise<Result> {
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      eventId
    )
  ) {
    return { status: "not-found" }
  }
  try {
    const response = await fetch(
      `${envConfig.apiBaseUrl}/events/${encodeURIComponent(eventId)}`,
      { cache: "no-store" }
    )
    if (response.status === 404) return { status: "not-found" }
    if (!response.ok) return { status: "unavailable" }
    const event: PublishedEventDetails = await response.json()
    if (!event || event.eventId !== eventId || event.status !== "published")
      return { status: "not-found" }
    return { status: "ok", event }
  } catch {
    return { status: "unavailable" }
  }
}
