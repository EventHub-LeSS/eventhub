import type { EventAddress } from "@/features/shared/lib/location"

/** Statuses of the backend's event_status enum. */
export type EventStatus = "draft" | "published" | "cancelled" | "completed"

/**
 * An event as the API returns it. Money arrives as a string because the backend
 * uses an exact decimal type, which would lose precision as a JSON number.
 */
export interface Event {
  eventId: string
  title: string
  description: string | null
  startTime: string
  endTime: string
  capacity: number
  status: EventStatus
  /** EVENTHUB-290: organizers can neither restore nor republish such an event. */
  withdrawnByModerator: boolean
  price: string
  categoryId: string | null
  organizerId: string | null
  locationId: string | null
  createdAt: string
  updatedAt: string
}

export interface EventCategory {
  categoryId: string
  category: string
}

/** A stored location, from `GET /locations`. */
export type EventLocation = EventAddress

/** The categories and locations an organizer can pick from. */
export interface EventCatalog {
  categories: EventCategory[]
  locations: EventLocation[]
}
