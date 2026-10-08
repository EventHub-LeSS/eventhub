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

/** Failures are returned, not thrown, so the page can say what happened. */
export type OrganizerEventsResult =
  | { status: "ok"; events: OrganizerEvent[] }
  | { status: "unavailable" }

export type OrganizerEventResult =
  | { status: "ok"; event: OrganizerEvent }
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

  return event ? { status: "ok", event } : { status: "not-found" }
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
