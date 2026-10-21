import "server-only"
import { envConfig } from "@/features/shared/lib/env"

export interface EventRatingSummaryData {
  eventId: string
  /** Null while the event has no ratings yet (see model.EventRatingSummary). */
  averageRating: number | null
  ratingCount: number
}

export type EventRatingSummaryResult =
  | { status: "ok"; summary: EventRatingSummaryData }
  | { status: "unavailable" }

/**
 * EVENTHUB-238: the public rating summary shown in the footer of the event
 * detail page, from `GET /events/{eventId}/rating-summary`. Non-critical
 * data: if it cannot be loaded the page degrades gracefully and shows the
 * reminder note alone.
 */
export async function getEventRatingSummary(
  eventId: string
): Promise<EventRatingSummaryResult> {
  try {
    const response = await fetch(
      `${envConfig.apiBaseUrl}/events/${encodeURIComponent(eventId)}/rating-summary`,
      { cache: "no-store" }
    )

    if (!response.ok) {
      return { status: "unavailable" }
    }

    const summary = (await response.json()) as Partial<EventRatingSummaryData>
    const average = summary.averageRating ?? null

    if (
      !summary ||
      typeof summary.eventId !== "string" ||
      (average !== null && typeof average !== "number") ||
      typeof summary.ratingCount !== "number"
    ) {
      return { status: "unavailable" }
    }

    return {
      status: "ok",
      summary: {
        eventId: summary.eventId,
        averageRating: average,
        ratingCount: summary.ratingCount,
      },
    }
  } catch {
    return { status: "unavailable" }
  }
}
