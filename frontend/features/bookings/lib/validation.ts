import type { TicketView } from "@/features/bookings/lib/types"

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function isUUID(value: string): boolean {
  return UUID_PATTERN.test(value)
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
