import type { SimulatedPaymentOutcome } from "@/features/bookings/lib/types"

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
    const unit = availableSeats === 1 ? "Ticket" : "Tickets"

    return `Nur noch ${availableSeats} ${unit} verfügbar.`
  }

  return undefined
}

/**
 * Normalizes what the user typed into the amount field: trims and turns the
 * German decimal comma into the dot the API expects.
 */
export function normalizeAmountInput(raw: string): string {
  const trimmed = raw.trim()

  if (trimmed.includes(",") && !trimmed.includes(".")) {
    return trimmed.replace(",", ".")
  }

  return trimmed
}

/**
 * Parses a payment amount. Returns null for anything that is not a finite,
 * non-negative money value within the storable range — including the huge
 * exponents the backend rejects (AK4).
 */
export function parseAmount(raw: string): number | null {
  const normalized = normalizeAmountInput(raw)

  if (normalized === "") {
    return null
  }

  const value = Number(normalized)

  if (!Number.isFinite(value) || value < 0 || value > MAX_PAYMENT_AMOUNT) {
    return null
  }

  return value
}

/**
 * AK4: the confirmed amount must equal the amount shown before completion.
 * A deviation is caught here; the backend would reject it with 422 without
 * changing anything.
 */
export function amountMismatchError(
  raw: string,
  expectedAmount: string
): string | undefined {
  if (raw.trim() === "") {
    return "Bitte gib den zu zahlenden Betrag ein."
  }

  const confirmed = parseAmount(raw)

  if (confirmed === null) {
    return "Bitte gib einen gültigen Betrag ein, z. B. 40,00."
  }

  const expected = Number(expectedAmount)

  if (!Number.isFinite(expected)) {
    return "Der zu zahlende Betrag konnte nicht gelesen werden. Bitte lade die Seite neu."
  }

  if (confirmed !== expected) {
    return "Der bestätigte Betrag weicht vom zu zahlenden Betrag ab. Bitte prüfe deine Eingabe."
  }

  return undefined
}

/** Server-side guard: a client may call a server action with any payload. */
export function isSimulatedOutcome(
  value: string
): value is SimulatedPaymentOutcome {
  return value === "success" || value === "failure"
}
