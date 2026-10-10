import "server-only"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { envConfig } from "@/features/shared/lib/env"

export interface PublishedEventsResult {
  events: PublishedEvent[]
  /** True when the catalog can't be reached right now (backend down, etc.).
   * The UI should degrade gracefully instead of throwing when this is set. */
  unavailable: boolean
}

/**
 * EVENTHUB-83: Veröffentlichte Veranstaltungen anzeigen
 *
 * Calls `GET /events` (public, unauthenticated; see
 * backend/internal/handler/event_handler.go#ListPublishedEventsHandler).
 * The endpoint already only returns events in status "published" — it has
 * no `status` query param, only the optional `location`/`categoryId`
 * filters this ticket doesn't need. Any non-OK response or network error is
 * treated as "unavailable" rather than thrown, so this page keeps
 * rendering even if the backend is unreachable.
 */
export async function getPublishedEvents(): Promise<PublishedEventsResult> {
  try {
    const response = await fetch(`${envConfig.apiBaseUrl}/events`, {
      cache: "no-store",
    })

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
