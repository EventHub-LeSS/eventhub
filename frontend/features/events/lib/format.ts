/** Shown in place of a missing value, so no cell renders as a blank gap. */
export const EMPTY_VALUE = "—"

/**
 * Locale and time zone are pinned, so the output does not depend on the server.
 * Events are shown in their local time.
 */
const eventDateTimeFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Europe/Berlin",
})

const eventPriceFormat = new Intl.NumberFormat("de-DE", {
  style: "currency",
  currency: "EUR",
})

const eventCountFormat = new Intl.NumberFormat("de-DE")

/**
 * A missing or unparsable timestamp must not reach Intl, which would throw a
 * RangeError and take the whole page down.
 */
function toDate(timestamp: string | null | undefined): Date | null {
  if (!timestamp) {
    return null
  }

  const date = new Date(timestamp)

  return Number.isNaN(date.getTime()) ? null : date
}

export function formatEventDateTime(
  timestamp: string | null | undefined
): string {
  const date = toDate(timestamp)

  return date ? eventDateTimeFormat.format(date) : EMPTY_VALUE
}

/**
 * Same-day events read "05.11.2026, 18:00–20:30"; longer ones spell out both
 * dates. Without an end, the start is shown on its own.
 */
export function formatEventTimeRange(
  startTime: string | null | undefined,
  endTime: string | null | undefined
): string {
  const start = toDate(startTime)
  const end = toDate(endTime)

  if (!start) {
    return EMPTY_VALUE
  }

  if (!end) {
    return eventDateTimeFormat.format(start)
  }

  return eventDateTimeFormat.formatRange(start, end)
}

/** The API sends prices as decimal strings such as "49.9"; "0" means free. */
export function formatEventPrice(price: string | null | undefined): string {
  const amount = price ? Number(price) : Number.NaN

  if (!Number.isFinite(amount)) {
    return EMPTY_VALUE
  }

  return amount === 0 ? "Kostenlos" : eventPriceFormat.format(amount)
}

export function formatEventCapacity(capacity: number): string {
  const unit = capacity === 1 ? "Platz" : "Plätze"

  return `${eventCountFormat.format(capacity)} ${unit}`
}

/**
 * Money earned on an event, in EUR. The API exposes no revenue field, so the
 * caller passes price × confirmed tickets; Intl rounds it to cents.
 */
export function formatEventRevenue(amount: number): string {
  return Number.isFinite(amount) ? eventPriceFormat.format(amount) : EMPTY_VALUE
}

/**
 * Returns the percentage of capacity that is booked (0–100), or null when the
 * number of available seats is unknown.
 */
export function computeOccupancyPercent(
  availableSeats: number | null,
  capacity: number
): number | null {
  if (availableSeats === null || capacity === 0) {
    return null
  }
  const booked = capacity - availableSeats
  const percent = (booked / capacity) * 100
  // Clamp to 0–100 in case of data inconsistencies.
  return Math.max(0, Math.min(100, Math.round(percent)))
}

/**
 * Formats the number of available seats, e.g. "23 von 100 Plätze frei".
 */
export function formatAvailableSeats(
  availableSeats: number | null,
  capacity: number
): string {
  if (availableSeats === null) {
    return EMPTY_VALUE
  }
  const unit = availableSeats === 1 ? "Platz" : "Plätze"
  return `${eventCountFormat.format(availableSeats)} ${unit} frei`
}

/**
 * Formats the number of sold tickets, e.g. "77 von 100 Tickets verkauft".
 */
export function formatSoldTickets(sold: number, capacity: number): string {
  const unit = sold === 1 ? "Ticket" : "Tickets"
  return `${eventCountFormat.format(sold)} ${unit} von ${eventCountFormat.format(capacity)} verkauft`
}
