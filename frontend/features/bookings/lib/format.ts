import type {
  OwnBookingLocation,
  PaymentStatus,
} from "@/features/bookings/lib/types"

/** Shown in place of a missing value, so no line renders as a blank gap. */
export const EMPTY_VALUE = "—"

/**
 * Locale and time zone are pinned, so the output does not depend on the
 * server. Events are shown in their local time.
 */
const currencyFormat = new Intl.NumberFormat("de-DE", {
  style: "currency",
  currency: "EUR",
})

const dateTimeFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Europe/Berlin",
})

/** Turns a backend decimal string (e.g. "40" or "40.00") into "40,00 €". */
export function formatAmount(amount: string | number | null): string {
  const value = typeof amount === "number" ? amount : Number(amount)

  if (!Number.isFinite(value)) {
    return EMPTY_VALUE
  }

  return currencyFormat.format(value)
}

const timeFormat = new Intl.DateTimeFormat("de-DE", {
  timeStyle: "short",
  timeZone: "Europe/Berlin",
})

const dayFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "short",
  timeZone: "Europe/Berlin",
})

/** "2 Tickets" / "1 Ticket". */
export function formatTicketCount(count: number): string {
  return `${count} ${count === 1 ? "Ticket" : "Tickets"}`
}

export function formatDateTime(timestamp: string | null | undefined): string {
  if (!timestamp) {
    return EMPTY_VALUE
  }

  const date = new Date(timestamp)

  return Number.isNaN(date.getTime())
    ? EMPTY_VALUE
    : dateTimeFormat.format(date)
}

export function formatEventPeriod(startTime: string, endTime: string): string {
  const start = new Date(startTime)
  const end = new Date(endTime)

  if (Number.isNaN(start.getTime())) {
    return EMPTY_VALUE
  }

  if (Number.isNaN(end.getTime())) {
    return dateTimeFormat.format(start)
  }

  const sameDay = dayFormat.format(start) === dayFormat.format(end)

  return `${dateTimeFormat.format(start)} – ${
    sameDay ? timeFormat.format(end) : dateTimeFormat.format(end)
  }`
}

export function formatLocation(location: OwnBookingLocation | null): string {
  if (!location) {
    return EMPTY_VALUE
  }

  const street = [location.street, location.houseNumber]
    .filter(Boolean)
    .join(" ")
  const city = [location.postalCode, location.city].filter(Boolean).join(" ")

  return [location.name, street, city].filter(Boolean).join(", ")
}

export const paymentStatusLabels: Record<PaymentStatus, string> = {
  pending: "Offen",
  paid: "Bezahlt",
  failed: "Fehlgeschlagen",
  refunded: "Erstattet",
}

export function splitTicketsByEnd<
  T extends { event: { endTime: string } | null },
>(tickets: T[], now: number): { upcoming: T[]; past: T[] } {
  const upcoming: T[] = []
  const past: T[] = []

  for (const ticket of tickets) {
    const end = ticket.event ? Date.parse(ticket.event.endTime) : Number.NaN

    if (!Number.isNaN(end) && end >= now) {
      upcoming.push(ticket)
    } else {
      past.push(ticket)
    }
  }

  return { upcoming, past }
}

/**
 * Splits the ticket list of the API into cancelled tickets, kept in API order,
 * and the remaining tickets split by event end (see `splitTicketsByEnd`).
 */
export function splitOwnTickets<
  T extends { cancelledBy?: unknown; event: { endTime: string } | null },
>(tickets: T[], now: number): { upcoming: T[]; past: T[]; cancelled: T[] } {
  const cancelled = tickets.filter((ticket) => ticket.cancelledBy)
  const active = tickets.filter((ticket) => !ticket.cancelledBy)

  return { ...splitTicketsByEnd(active, now), cancelled }
}

/**
 * Whether money came back for this booking. Judged by its own refund amount,
 * not the payment status: a payment of several bookings stays `paid` until
 * every booking of it is cancelled.
 */
export function hasRefund(booking: { refundAmount: string | null }): boolean {
  return Number(booking.refundAmount) > 0
}
