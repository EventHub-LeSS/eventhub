import type {
  SimulatedPaymentOutcome,
  TicketView,
} from "@/features/bookings/lib/types"

/**
 * The backend's `payments.amount` column is NUMERIC(12,2), so nothing above
 * this value can ever be charged (see service.maxPaymentAmount).
 */
export const MAX_PAYMENT_AMOUNT = 9999999999.99

/**
 * UI policy: at most this many tickets per booking, so the total stays
 * reviewable at a glance. The backend counts capacity itself.
 */
export const MAX_TICKETS_PER_BOOKING = 10

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function isUUID(value: string): boolean {
  return UUID_PATTERN.test(value)
}

/**
 * Parses a ticket count the user typed: only whole numbers count, anything
 * else ("1.5", "-2", "abc", "") is invalid.
 */
export function parseTicketCount(raw: string): number | null {
  const trimmed = raw.trim()

  if (!/^\d+$/.test(trimmed)) {
    return null
  }

  const count = Number(trimmed)

  return Number.isSafeInteger(count) ? count : null
}

/**
 * AK4: invalid input is caught in the frontend before anything is sent. The
 * count must be a whole number from 1 up to the per-booking limit and must
 * fit the event's remaining seats when those are known.
 */
export function ticketCountError(
  raw: string,
  availableSeats: number | null
): string | undefined {
  const count = parseTicketCount(raw)

  if (count === null || count < 1) {
    return "Bitte gib eine ganze Ticketanzahl von mindestens 1 ein."
  }

  if (count > MAX_TICKETS_PER_BOOKING) {
    return `Pro Buchung sind höchstens ${MAX_TICKETS_PER_BOOKING} Tickets möglich.`
  }

  if (availableSeats !== null && count > availableSeats) {
    return seatsExceededMessage(availableSeats)
  }

  return undefined
}

/**
 * EVENTHUB-91 / AK4: the same message whether the form or the backend (409,
 * capacity exceeded) notices that the count exceeds the remaining seats.
 */
export function seatsExceededMessage(availableSeats: number): string {
  if (availableSeats < 1) {
    return "Derzeit sind keine Tickets verfügbar."
  }

  const unit = availableSeats === 1 ? "Ticket" : "Tickets"

  return `Nur noch ${availableSeats} ${unit} verfügbar.`
}

/**
 * Parses a payment amount — e.g. the stored amount the backend sends as an
 * exact decimal string. Returns null for anything that is not a finite,
 * non-negative money value within the storable range — including the huge
 * exponents the backend rejects (AK4).
 */
export function parseAmount(raw: string): number | null {
  const trimmed = raw.trim()

  if (trimmed === "") {
    return null
  }

  const value = Number(trimmed)

  if (!Number.isFinite(value) || value < 0 || value > MAX_PAYMENT_AMOUNT) {
    return null
  }

  return value
}

/** Server-side guard: a client may call a server action with any payload. */
export function isSimulatedOutcome(
  value: string
): value is SimulatedPaymentOutcome {
  return value === "success" || value === "failure"
}

/**
 * AK4: the card data of the simulated checkout is validated in the frontend
 * before anything is booked. Every check stays silent while the field is
 * untouched — the submit button simply remains disabled — and only speaks
 * up once something was typed, like the amount checks above.
 */
export function cardholderError(raw: string): string | undefined {
  const trimmed = raw.trim()

  if (trimmed === "") {
    return undefined
  }

  if (trimmed.length < 2) {
    return "Bitte gib den Namen des Karteninhabers an."
  }

  return undefined
}

export function cardNumberError(raw: string): string | undefined {
  const trimmed = raw.trim()

  if (trimmed === "") {
    return undefined
  }

  if (!/^\d{12,19}$/.test(trimmed.replace(/[\s-]/g, ""))) {
    return "Bitte gib eine gültige Kartennummer an (12–19 Ziffern)."
  }

  return undefined
}

export function cardExpiryError(raw: string): string | undefined {
  const trimmed = raw.trim()

  if (trimmed === "") {
    return undefined
  }

  const match = /^(0?[1-9]|1[0-2])\/(\d{2})$/.exec(trimmed)

  if (!match) {
    return "Bitte gib das Ablaufdatum im Format MM/JJ an."
  }

  // Expired at the end of the month: a card from 10/26 is invalid from 11/26.
  const expires = new Date(2000 + Number(match[2]), Number(match[1]), 1)
  const now = new Date()
  now.setDate(1)
  now.setHours(0, 0, 0, 0)

  if (expires <= now) {
    return "Diese Karte ist abgelaufen."
  }

  return undefined
}

/**
 * Formats what is typed into the expiry field: digits only, and the MM/JJ
 * slash appears by itself once the month is complete, so it never has to
 * be typed.
 */
export function formatCardExpiry(raw: string): string {
  const digits = raw.replace(/\D/g, "").slice(0, 4)

  if (digits.length <= 2) {
    return digits
  }

  return `${digits.slice(0, 2)}/${digits.slice(2)}`
}

export function cardCvcError(raw: string): string | undefined {
  const trimmed = raw.trim()

  if (trimmed === "") {
    return undefined
  }

  if (!/^\d{3,4}$/.test(trimmed)) {
    return "Bitte gib eine gültige CVC an (3–4 Ziffern)."
  }

  return undefined
}

export const MAX_RATING_TEXT_LENGTH = 2000

export function isRatingScore(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isInteger(value) &&
    value >= 1 &&
    value <= 5
  )
}

const TICKET_VIEWS: readonly string[] = ["upcoming", "past", "cancelled"]

// Reads the ?view= of the tickets page 
export function parseTicketView(
  value: string | string[] | undefined
): TicketView {
  return typeof value === "string" && TICKET_VIEWS.includes(value)
    ? (value as TicketView)
    : "upcoming"
}
