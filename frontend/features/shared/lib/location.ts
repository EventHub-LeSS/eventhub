/**
 * Mirrors `model.PublishedEventLocation` (see
 * backend/internal/model/published_events.go): the address of an event, as
 * event listings, event details, own bookings and `GET /locations` return it.
 */
export interface EventAddress {
  locationId: string
  street: string
  houseNumber: string | null
  postalCode: string
  city: string
  /** Venue, hall or floor, e.g. "Musikhalle Hamburg". */
  addressAddition: string | null
  accessible: boolean
}

type AddressParts = Omit<EventAddress, "locationId" | "accessible">

export function formatStreet(location: AddressParts): string {
  return [location.street, location.houseNumber].filter(Boolean).join(" ")
}

/**
 * The full address in one line, as the backend's reminders show it, e.g.
 * "Musikhalle Hamburg, Lombardsbrücke 1, 20354 Hamburg".
 */
export function formatAddress(location: AddressParts): string {
  return [
    location.addressAddition,
    formatStreet(location),
    `${location.postalCode} ${location.city}`,
  ]
    .filter(Boolean)
    .join(", ")
}

/**
 * A short label for cards and lists: the address addition, or the street when
 * there is none, followed by the city, e.g. "Musikhalle Hamburg, Hamburg".
 */
export function formatPlace(location: AddressParts): string {
  return [location.addressAddition || formatStreet(location), location.city]
    .filter(Boolean)
    .join(", ")
}
