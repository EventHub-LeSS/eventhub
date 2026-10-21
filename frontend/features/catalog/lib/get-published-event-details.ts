import "server-only"
import type { PublishedEventDetails } from "@/features/catalog/lib/types"
import { envConfig } from "@/features/shared/lib/env"

export type PublishedEventDetailsResult =
  | { status: "ok"; event: PublishedEventDetails }
  | { status: "not-found" }
  | { status: "unavailable" }

/**
 * EVENTHUB-211 / EVENTHUB-212: public event details with current
 * availability, from `GET /events/{eventId}` (see
 * backend/internal/handler/event_handler.go#GetPublishedEventDetailsHandler).
 *
 * The endpoint only serves published events that have both a category and a
 * location, and answers 404 otherwise; 400 means the `{eventId}` is not a
 * UUID, which for a visitor is the same as an unknown event. Anything else
 * (backend down, 500, …) is "unavailable" so the page can degrade gracefully
 * instead of throwing.
 */
export async function getPublishedEventDetails(
  eventId: string
): Promise<PublishedEventDetailsResult> {
  try {
    const response = await fetch(
      `${envConfig.apiBaseUrl}/events/${encodeURIComponent(eventId)}`,
      { cache: "no-store" }
    )

    if (response.status === 404 || response.status === 400) {
      return { status: "not-found" }
    }

    if (!response.ok) {
      return { status: "unavailable" }
    }

    const event = (await response.json()) as Partial<PublishedEventDetails>

    if (
      !event ||
      typeof event.eventId !== "string" ||
      typeof event.title !== "string" ||
      typeof event.price !== "string" ||
      typeof event.bookable !== "boolean" ||
      typeof event.availableSeats !== "number" ||
      typeof event.availability !== "string" ||
      !event.category ||
      !event.location
    ) {
      return { status: "unavailable" }
    }

    return { status: "ok", event: event as PublishedEventDetails }
  } catch {
    return { status: "unavailable" }
  }
}
