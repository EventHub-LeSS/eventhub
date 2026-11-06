import "server-only"
import { getPublishedEvents } from "@/features/catalog"
import type { EventDraftPayload } from "@/features/events/lib/draft"
import type { DecodedImage } from "@/features/events/lib/image"
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

  return `Anfrage fehlgeschlagen (Status ${response.status}).`
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

/** confirmed tickets sold for one of the caller's own events. */
export interface SoldTicketsResponse {
  eventId: string
  soldTickets: number
}

/** confirmed tickets sold for one of the caller's own events. */
export async function getSoldTickets(
  accessToken: string,
  eventId: string
): Promise<SoldTicketsResponse> {
  return apiFetch<SoldTicketsResponse>(`/events/${eventId}/sold-tickets`, {
    accessToken,
  })
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

/**
 * EVENTHUB-264: replaces the event's image. The API takes the raw PNG or JPEG
 * bytes with a matching Content-Type and stores them base64 encoded itself. It
 * only accepts images for drafts and published events.
 */
export async function uploadEventImage(
  accessToken: string,
  eventId: string,
  image: DecodedImage
): Promise<void> {
  await apiFetch<{ message: string }>(`/events/${eventId}/image`, {
    accessToken,
    method: "PUT",
    headers: { "Content-Type": image.contentType },
    body: image.bytes,
  })
}

/** EVENTHUB-77 / AK5, EVENTHUB-288: also republishes a withdrawn event. */
export async function publishEvent(
  accessToken: string,
  eventId: string
): Promise<void> {
  await apiFetch<{ message: string }>(`/events/${eventId}/publish`, {
    accessToken,
    method: "POST",
  })
}

/** EVENTHUB-288: turns a withdrawn event back into a draft. */
export async function restoreEventAsDraft(
  accessToken: string,
  eventId: string
): Promise<void> {
  await apiFetch<Event>(`/events/${eventId}/restore-draft`, {
    accessToken,
    method: "POST",
  })
}

/** EVENTHUB-81: the organizer's own event's remaining capacity. */
export async function getAvailableSeats(
  accessToken: string,
  eventId: string
): Promise<number> {
  const response = await apiFetch<{ availableSeats: number }>(
    `/events/${eventId}/available-seats`,
    { accessToken }
  )

  return response.availableSeats
}

/**
 * EVENTHUB-82: takes a published event off sale. The API moves it to
 * "cancelled" and leaves its bookings untouched. EVENTHUB-291: ticket holders
 * of an upcoming event are notified; notifiedUsers counts them.
 */
export async function withdrawEvent(
  accessToken: string,
  eventId: string
): Promise<void> {
  await apiFetch<{ message: string; notifiedUsers: number }>(
    `/events/${eventId}/withdraw`,
    {
      accessToken,
      method: "POST",
    }
  )
}

/**
 * EVENTHUB-256: irrevocably deletes a draft. The API refuses events that are no
 * longer drafts (400) or already have bookings (409).
 */
export async function deleteEvent(
  accessToken: string,
  eventId: string
): Promise<void> {
  await apiFetch<{ message: string }>(`/events/${eventId}`, {
    accessToken,
    method: "DELETE",
  })
}

/**
 * Categories and locations are master data, but the API exposes no endpoint for
 * them yet. The public listing embeds both on every published event, so the
 * selectable values are derived from there until dedicated endpoints exist.
 * An unreachable catalog yields empty lists, which the form reports as "no
 * categories available" instead of failing.
 */
export async function listLocations(
  accessToken: string
): Promise<EventLocation[]> {
  return apiFetch<EventLocation[]>("/locations", { accessToken })
}

export async function getEventCatalog(
  accessToken: string
): Promise<EventCatalog> {
  const [{ events }, locations] = await Promise.all([
    getPublishedEvents(),
    listLocations(accessToken).catch(() => []),
  ])
  const categories = new Map<string, EventCategory>()

  for (const event of events) {
    categories.set(event.category.categoryId, event.category)
  }

  return {
    categories: [...categories.values()].toSorted((a, b) =>
      a.category.localeCompare(b.category)
    ),
    locations,
  }
}
