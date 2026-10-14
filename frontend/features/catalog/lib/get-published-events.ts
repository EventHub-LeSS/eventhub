import "server-only"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { envConfig } from "@/features/shared/lib/env"

export interface PublishedEventsFilter {
  /** Case-insensitive substring of city or venue name. */
  location?: string
  /** Start calendar day in YYYY-MM-DD, interpreted in Europe/Berlin by the backend. */
  date?: string
  /** Exact category UUID. */
  categoryId?: string
  /** Case-insensitive substring of the event title. Applied client-side
   * (see below), the backend has no `title`/search query param. */
  title?: string
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
 * EVENTHUB-208: Veranstaltungen nach Kategorie filtern
 *
 * Calls `GET /events` (public, unauthenticated; see
 * backend/internal/handler/event_handler.go#ListPublishedEventsHandler).
 * The endpoint already only returns events in status "published" — it has
 * no `status` query param. `location`, `date` and `categoryId` are combined
 * with AND by the backend; omitting a filter (or passing an empty value)
 * resets it. Any non-OK response or network error is treated as
 * "unavailable" rather than thrown, so this page keeps rendering even if
 * the backend is unreachable or a filter value is rejected (e.g. an
 * invalid date or category UUID).
 *
 * `title` has no backend equivalent, so it's applied afterwards on the
 * already-filtered result instead.
 */
export async function getPublishedEvents(
  filter: PublishedEventsFilter = {}
): Promise<PublishedEventsResult> {
  try {
    const query = new URLSearchParams()
    if (filter.location) query.set("location", filter.location)
    if (filter.date) query.set("date", filter.date)
    if (filter.categoryId) query.set("categoryId", filter.categoryId)
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

    const title = filter.title?.trim().toLowerCase()

    return {
      events: (payload as PublishedEvent[]).filter(
        (event) =>
          // Defense in depth: the endpoint only returns published events,
          // but this keeps that acceptance criterion true even if that
          // ever changes.
          event?.status === "published" &&
          (!title || event.title.toLowerCase().includes(title))
      ),
      unavailable: false,
    }
  } catch {
    return { events: [], unavailable: true }
  }
}

export interface EventCategory {
  categoryId: string
  category: string
}

/**
 * EVENTHUB-208: Veranstaltungen nach Kategorie filtern
 *
 * There is no dedicated "list categories" endpoint, so the category filter's
 * options are derived from the categories that currently have at least one
 * published event — i.e. the real, filterable data, not a hardcoded list
 * (same reasoning as the free-text location filter). Fetches the
 * unfiltered list once; on failure this returns an empty list, which just
 * means the category dropdown has no options rather than breaking the page.
 */
export async function getEventCategories(): Promise<EventCategory[]> {
  const { events, unavailable } = await getPublishedEvents()

  if (unavailable) {
    return []
  }

  const byId = new Map<string, EventCategory>()
  for (const event of events) {
    byId.set(event.category.categoryId, event.category)
  }

  return [...byId.values()].toSorted((a, b) =>
    a.category.localeCompare(b.category)
  )
}
