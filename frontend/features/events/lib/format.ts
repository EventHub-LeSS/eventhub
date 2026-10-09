/** Shown in place of a missing value, so no cell renders as a blank gap. */
export const EMPTY_VALUE = "—"

/**
 * Locale and time zone are pinned, so the output does not depend on the server.
 * Events are shown in their local time.
 */
const eventDateTimeFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Europe/Berlin",
})

const eventPriceFormat = new Intl.NumberFormat("en-GB", {
  style: "currency",
  currency: "EUR",
})

const eventCountFormat = new Intl.NumberFormat("en-GB")

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
 * Same-day events read "5 Nov 2026, 18:00–20:30"; longer ones spell out both
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

  return amount === 0 ? "Free" : eventPriceFormat.format(amount)
}

export function formatEventCapacity(capacity: number): string {
  const unit = capacity === 1 ? "seat" : "seats"

  return `${eventCountFormat.format(capacity)} ${unit}`
}
