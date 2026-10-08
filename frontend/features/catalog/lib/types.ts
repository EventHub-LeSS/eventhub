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
