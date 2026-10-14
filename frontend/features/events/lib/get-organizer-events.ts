import "server-only"
import { getCurrentUser, getSession } from "@/features/auth"
import { getPublishedEvents, type PublishedEvent } from "@/features/catalog"
import type { Event } from "@/features/events/lib/types"
import { envConfig } from "@/features/shared/lib/env"

type EventLocation = PublishedEvent["location"]
type EventCategory = PublishedEvent["category"]

/** An event of the active organization with its venue and category resolved. */
export interface OrganizerEvent extends Event {
  location: EventLocation | null
  category: EventCategory | null
}

/**
 * Only organization members with the event_manager role may read sales. Other
 * members still see the event, just without the figure.
 */
export type SoldTicketsResult =
  | { status: "ok"; count: number }
  | { status: "forbidden" }
  | { status: "unavailable" }

export interface OrganizerEventWithSales extends OrganizerEvent {
  soldTickets: SoldTicketsResult
}

/** Failures are returned, not thrown, so the page can say what happened. */
export type OrganizerEventsResult =
  | { status: "ok"; events: OrganizerEvent[] }
  | { status: "unavailable" }

export type OrganizerEventResult =
  | { status: "ok"; event: OrganizerEventWithSales }
  | { status: "not-found" }
  | { status: "unavailable" }

/**
 * The events of the active organization, in every status. The API checks the
 * membership itself and leaves the order open, so they are sorted here.
 */
export async function getOrganizerEvents(): Promise<OrganizerEventsResult> {
  const [user, session] = await Promise.all([getCurrentUser(), getSession()])

  if (!user?.activeOrganization || !session) {
    return { status: "unavailable" }
  }

  const [events, catalog] = await Promise.all([
    fetchOrganizationEvents(user.activeOrganization, session.accessToken),
    getPublishedEvents(),
  ])

  if (!events) {
    return { status: "unavailable" }
  }

  // The endpoint returns venue and category as ids only. Their names come from
  // the public listing, the one place the API exposes them.
  const locations = new Map(
    catalog.events.map((event) => [event.location.locationId, event.location])
  )
  const categories = new Map(
    catalog.events.map((event) => [event.category.categoryId, event.category])
  )

  return {
    status: "ok",
    events: events
      .map((event) => ({
        ...event,
        location: event.locationId
          ? (locations.get(event.locationId) ?? null)
          : null,
        category: event.categoryId
          ? (categories.get(event.categoryId) ?? null)
          : null,
      }))
      .toSorted((a, b) => Date.parse(a.startTime) - Date.parse(b.startTime)),
  }
}

/**
 * One event of the active organization. The API has no endpoint for a single
 * own event that also covers drafts, so it is picked from the list.
 */
export async function getOrganizerEvent(
  eventId: string
): Promise<OrganizerEventResult> {
  const result = await getOrganizerEvents()

  if (result.status !== "ok") {
    return result
  }

  const event = result.events.find((candidate) => candidate.eventId === eventId)
  if (!event) {
    return { status: "not-found" }
  }

  const session = await getSession()
  const soldTickets: SoldTicketsResult = session
    ? await fetchSoldTickets(eventId, session.accessToken)
    : { status: "unavailable" }

  return { status: "ok", event: { ...event, soldTickets } }
}

async function fetchOrganizationEvents(
  organization: string,
  accessToken: string
): Promise<Event[] | null> {
  try {
    const response = await fetch(
      `${envConfig.apiBaseUrl}/events/org/${encodeURIComponent(organization)}`,
      {
        headers: { Authorization: `Bearer ${accessToken}` },
        cache: "no-store",
      }
    )

    if (!response.ok) {
      return null
    }

    const payload: unknown = await response.json()

    return Array.isArray(payload) ? (payload as Event[]) : null
  } catch {
    return null
  }
}

interface SoldTicketsResponse {
  soldTickets: number
}

async function fetchSoldTickets(
  eventId: string,
  accessToken: string
): Promise<SoldTicketsResult> {
  try {
    const response = await fetch(
      `${envConfig.apiBaseUrl}/events/${encodeURIComponent(eventId)}/sold-tickets`,
      {
        headers: { Authorization: `Bearer ${accessToken}` },
        cache: "no-store",
      }
    )

    if (response.status === 401 || response.status === 403) {
      return { status: "forbidden" }
    }

    if (!response.ok) {
      return { status: "unavailable" }
    }

    const payload: unknown = await response.json()
    if (
      !payload ||
      typeof payload !== "object" ||
      typeof (payload as SoldTicketsResponse).soldTickets !== "number"
    ) {
      return { status: "unavailable" }
    }

    return {
      status: "ok",
      count: (payload as SoldTicketsResponse).soldTickets,
    }
  } catch {
    return { status: "unavailable" }
  }
}
