import "server-only"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { envConfig } from "@/features/shared/lib/env"

export interface PublishedEventsFilter {
  /** Case-insensitive substring of city or venue name. */
  location?: string
  /** Start calendar day in YYYY-MM-DD, interpreted in Europe/Berlin by the backend. */
  date?: string
}

export interface PublishedEventsResult {
  events: PublishedEvent[]
  /** True when the catalog can't be reached right now (backend down, an
   * invalid filter value, etc.). The UI should degrade gracefully instead
   * of throwing when this is set. */
  unavailable: boolean
}

/**
 * EVENTHUB-83: Veröffentlichte Veranstaltungen anzeigen
 * EVENTHUB-223: Veranstaltungen nach Datum und Ort filtern
 *
 * Calls `GET /events` (public, unauthenticated; see
 * backend/internal/handler/event_handler.go#ListPublishedEventsHandler).
 * The endpoint already only returns events in status "published" — it has
 * no `status` query param. `location` and `date` are combined with AND by
 * the backend; omitting a filter (or passing an empty value) resets it.
 * Any non-OK response or network error is treated as "unavailable" rather
 * than thrown, so this page keeps rendering even if the backend is
 * unreachable or a filter value is rejected (e.g. an invalid date).
 */
export async function getPublishedEvents(
  filter: PublishedEventsFilter = {}
): Promise<PublishedEventsResult> {
  try {
    const query = new URLSearchParams()
    if (filter.location) query.set("location", filter.location)
    if (filter.date) query.set("date", filter.date)
    const queryString = query.toString()

    const response = await fetch(
      `${envConfig.apiBaseUrl}/events${queryString ? `?${queryString}` : ""}`,
      { cache: "no-store" }
    )

    if (!response.ok) {
      return { events: [], unavailable: true }
    }

    const payload: unknown = await response.json()

    if (!Array.isArray(payload)) {
      return { events: [], unavailable: true }
    }

    return {
      // Defense in depth: the endpoint only returns published events, but
      // this keeps that acceptance criterion true even if that ever changes.
      events: (payload as PublishedEvent[]).filter(
        (event) => event?.status === "published"
      ),
      unavailable: false,
    }
  } catch {
    return { events: [], unavailable: true }
  }
}
