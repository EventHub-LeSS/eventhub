import type {
  BookingStatus,
  PaymentFailureReason,
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

/**
 * The time left to complete a pending payment, as "14:32 Min". The clock of
 * the backend is what counts; this is a display that ticks along.
 */
export function formatRemaining(
  expiresAt: string | null | undefined,
  now: number
): string | null {
  if (!expiresAt) {
    return null
  }

  const deadline = Date.parse(expiresAt)

  if (Number.isNaN(deadline)) {
    return null
  }

  const remaining = Math.max(0, Math.ceil((deadline - now) / 1000))
  const minutes = Math.floor(remaining / 60)
  const seconds = remaining % 60

  return `${minutes}:${String(seconds).padStart(2, "0")} Min`
}

export const paymentStatusLabels: Record<PaymentStatus, string> = {
  pending: "Offen",
  paid: "Bezahlt",
  failed: "Fehlgeschlagen",
  refunded: "Erstattet",
}

export const bookingStatusLabels: Record<BookingStatus, string> = {
  reserved: "Reserviert",
  confirmed: "Bestätigt",
  cancelled: "Storniert",
  failed: "Fehlgeschlagen",
  expired: "Abgelaufen",
}

export const failureReasonLabels: Record<PaymentFailureReason, string> = {
  declined: "Zahlung abgelehnt",
  reservation_expired: "Reservierung abgelaufen",
  event_unavailable: "Veranstaltung nicht mehr verfügbar",
}
