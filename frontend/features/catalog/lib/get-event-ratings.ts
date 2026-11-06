import "server-only"
import {
  EVENT_RATINGS_PAGE_SIZE,
  type EventRatingsPage,
  isEventRatingsPage,
} from "@/features/catalog/lib/event-ratings"
import { envConfig } from "@/features/shared/lib/env"

export type EventRatingsResult =
  | { status: "ok"; ratings: EventRatingsPage }
  | { status: "unavailable" }

/**
 * 297: one page of the public ratings of an event, newest first, from
 * `GET /events/{eventId}/ratings`. Non-critical data: if it cannot be loaded
 * the event detail page still works and says so in the ratings section.
 */
export async function getEventRatings(
  eventId: string,
  page: number
): Promise<EventRatingsResult> {
  try {
    const query = new URLSearchParams({
      page: String(page),
      limit: String(EVENT_RATINGS_PAGE_SIZE),
    })
    const response = await fetch(
      `${envConfig.apiBaseUrl}/events/${encodeURIComponent(eventId)}/ratings?${query}`,
      { cache: "no-store" }
    )
    if (!response.ok) {
      return { status: "unavailable" }
    }
    const body: unknown = await response.json()
    return isEventRatingsPage(body)
      ? { status: "ok", ratings: body }
      : { status: "unavailable" }
  } catch {
    return { status: "unavailable" }
  }
}
