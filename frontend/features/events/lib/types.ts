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

export interface EventLocation {
  locationId: string
  name: string
  city: string
}

/** The categories and locations an organizer can pick from. */
export interface EventCatalog {
  categories: EventCategory[]
  locations: EventLocation[]
}
