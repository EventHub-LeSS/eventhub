import "server-only"
import { getSession } from "@/features/auth"
import { getPublishedEvents } from "@/features/catalog/lib/get-published-events"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { envConfig } from "@/features/shared/lib/env"

export type RecommendedEventsResult =
  | { status: "ok"; events: PublishedEvent[] }
  /** No signed-in user: the backend only recommends for a known user. */
  | { status: "anonymous" }
  | { status: "unavailable" }

/**
 * Veranstaltungsvorschläge anzeigen
 *
 * Calls `GET /recommendations` (authenticated; see
 * backend/internal/handler/recommendation_handler.go). The backend already
 * ranks the events and only returns future, published events with free
 * seats that the user has not booked yet, so the order is kept as is.
 *
 * The endpoint returns plain `model.EventModel`s with only `categoryId` and
 * `locationId`. Every recommendation is a published event, so the category
 * and location are taken from `GET /events` instead of loading each event on
 * its own. A recommendation missing there (e.g. withdrawn in between) is
 * skipped. Non-critical data: any failure yields "unavailable" and the page
 * keeps rendering the catalog.
 */
export async function getRecommendedEvents(
  limit: number
): Promise<RecommendedEventsResult> {
  const session = await getSession()

  if (!session) {
    return { status: "anonymous" }
  }

  try {
    const [response, catalog] = await Promise.all([
      fetch(`${envConfig.apiBaseUrl}/recommendations?limit=${limit}`, {
        headers: { Authorization: `Bearer ${session.accessToken}` },
        cache: "no-store",
      }),
      getPublishedEvents(),
    ])

    if (!response.ok || catalog.unavailable) {
      return { status: "unavailable" }
    }

    const payload: unknown = await response.json()

    if (!Array.isArray(payload)) {
      return { status: "unavailable" }
    }

    const byId = new Map(catalog.events.map((event) => [event.eventId, event]))
    const events = (payload as { eventId?: unknown }[])
      .map((item) =>
        typeof item?.eventId === "string" ? byId.get(item.eventId) : undefined
      )
      .filter((event): event is PublishedEvent => event !== undefined)

    return { status: "ok", events }
  } catch {
    return { status: "unavailable" }
  }
}
