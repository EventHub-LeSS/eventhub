import "server-only"
import { getPublishedEvents } from "@/features/catalog"
import type { EventDraftPayload } from "@/features/events/lib/draft"
import type {
  Event,
  EventCatalog,
  EventCategory,
  EventLocation,
  EventStatus,
} from "@/features/events/lib/types"
import { envConfig } from "@/features/shared/lib/env"

/**
 * Talks to the Go API with the signed-in user's own access token. The API
 * resolves the organizations the caller manages from that token, so there is
 * nothing to pass besides the payload.
 */
export class EventApiError extends Error {
  constructor(
    message: string,
    public readonly status: number
  ) {
    super(message)
    this.name = "EventApiError"
  }
}

/** Pulls the detail out of the API's RFC 9457 problem body, if there is one. */
async function problemDetail(response: Response): Promise<string> {
  try {
    const problem = (await response.json()) as { detail?: string }

    if (problem.detail) {
      return problem.detail
    }
  } catch {
    // Not a problem body, fall through to the generic message.
  }

  return `Request failed with status ${response.status}.`
}

async function apiFetch<T>(
  path: string,
  init: RequestInit & { accessToken?: string } = {}
): Promise<T> {
  const { accessToken, headers, ...request } = init
  const response = await fetch(`${envConfig.apiBaseUrl}${path}`, {
    ...request,
    headers: {
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...headers,
    },
    cache: "no-store",
  })

  if (!response.ok) {
    throw new EventApiError(await problemDetail(response), response.status)
  }

  return (await response.json()) as T
}

/** EVENTHUB-77 / AK3: the organizer's own events, optionally by status. */
export async function listOwnEvents(
  accessToken: string,
  status?: EventStatus
): Promise<Event[]> {
  const query = status ? `?status=${status}` : ""

  return apiFetch<Event[]>(`/events/self${query}`, { accessToken })
}

/**
 * A single own event. GET /events/{id} serves published events only, so a draft
 * can only be reached by filtering the organizer's own list.
 */
export async function getOwnEvent(
  accessToken: string,
  eventId: string
): Promise<Event | undefined> {
  const events = await listOwnEvents(accessToken)

  return events.find((event) => event.eventId === eventId)
}

/** EVENTHUB-77 / AK1 */
export async function createDraft(
  accessToken: string,
  payload: EventDraftPayload
): Promise<Event> {
  return apiFetch<Event>("/events/draft", {
    accessToken,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

/** EVENTHUB-77 / AK4 */
export async function updateEvent(
  accessToken: string,
  eventId: string,
  payload: EventDraftPayload
): Promise<Event> {
  return apiFetch<Event>(`/events/${eventId}`, {
    accessToken,
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

/** EVENTHUB-77 / AK5 */
export async function publishEvent(
  accessToken: string,
  eventId: string
): Promise<void> {
  await apiFetch<{ message: string }>(`/events/${eventId}/publish`, {
    accessToken,
    method: "POST",
  })
}

/**
 * Categories and locations are master data, but the API exposes no endpoint for
 * them yet. The public listing embeds both on every published event, so the
 * selectable values are derived from there until dedicated endpoints exist.
 * An unreachable catalog yields empty lists, which the form reports as "no
 * categories available" instead of failing.
 */
export async function getEventCatalog(): Promise<EventCatalog> {
  const { events } = await getPublishedEvents()
  const categories = new Map<string, EventCategory>()
  const locations = new Map<string, EventLocation>()

  for (const event of events) {
    categories.set(event.category.categoryId, event.category)
    locations.set(event.location.locationId, {
      locationId: event.location.locationId,
      name: event.location.name,
      city: event.location.city,
    })
  }

  return {
    categories: [...categories.values()].toSorted((a, b) =>
      a.category.localeCompare(b.category)
    ),
    locations: [...locations.values()].toSorted((a, b) =>
      a.name.localeCompare(b.name)
    ),
  }
}
