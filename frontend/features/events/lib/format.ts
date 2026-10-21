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

export function formatSoldTickets(
  soldTickets: number,
  capacity: number
): string {
  const unit = capacity === 1 ? "Platz" : "Plätze"

  return `${eventCountFormat.format(soldTickets)} / ${eventCountFormat.format(capacity)} ${unit}`
}

/** EVENTHUB-81. `null` means the count couldn't be loaded. */
export function formatAvailableSeats(
  availableSeats: number | null,
  capacity: number
): string {
  if (availableSeats === null) {
    return EMPTY_VALUE
  }

  const unit = capacity === 1 ? "Platz" : "Plätzen"

  return `${eventCountFormat.format(availableSeats)} von ${eventCountFormat.format(capacity)} ${unit} frei`
}

/**
 * EVENTHUB-81. Share of capacity that's booked, rounded to a whole percent and
 * capped at 100 (a drafted capacity cut below sold tickets must not read over
 * full). `null` when the seat count couldn't be loaded, or capacity is 0.
 */
export function computeOccupancyPercent(
  availableSeats: number | null,
  capacity: number
): number | null {
  if (availableSeats === null || capacity <= 0) {
    return null
  }

  const booked = capacity - availableSeats

  return Math.min(100, Math.max(0, Math.round((booked / capacity) * 100)))
}
