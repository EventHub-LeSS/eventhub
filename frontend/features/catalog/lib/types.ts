/** Mirrors `model.PublishedEventResponse` from `GET /events` on the backend
 * (see backend/internal/model/published_events.go). The repository joins
 * category and location, so both are always present on every item the
 * endpoint returns — events missing either are omitted server-side. */
export interface PublishedEvent {
  eventId: string
  title: string
  startTime: string
  endTime: string
  /** Serialized as a string (shopspring/decimal), e.g. "20.00". */
  price: string
  status: "draft" | "published" | "cancelled" | "completed"
  category: {
    categoryId: string
    category: string
  }
  location: {
    locationId: string
    name: string
    city: string
    postalCode: string
    street: string
    houseNumber: string | null
  }
}

/** Mirrors `model.EventAvailability`: why an event can or cannot be booked right now. */
export type EventAvailability =
  | "available"
  | "almost_sold_out"
  | "sold_out"
  | "temporarily_unavailable"

/**
 * Mirrors `model.PublishedEventDetailsResponse` from `GET /events/{eventId}`
 * (see backend/internal/model/event_details.go). `bookable` is a snapshot;
 * booking checks availability again in its own transaction.
 */
export interface PublishedEventDetails extends PublishedEvent {
  description: string | null
  capacity: number
  availableSeats: number
  bookable: boolean
  soldTickets: number
  occupancyPercent: number
  availability: EventAvailability
}
