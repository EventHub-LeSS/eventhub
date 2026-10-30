import "server-only"
import type {
  Booking,
  BookingStatus,
  OwnBooking,
  Payment,
  SimulatedPaymentOutcome,
} from "@/features/bookings/lib/types"
import { envConfig } from "@/features/shared/lib/env"

/**
 * Talks to the Go API with the signed-in user's own access token, like the
 * other features do. Bookings and payments are always the caller's own; the
 * API checks that per request.
 */
export class BookingApiError extends Error {
  constructor(
    message: string,
    public readonly status: number
  ) {
    super(message)
    this.name = "BookingApiError"
  }
}

/** Pulls the detail out of the API's RFC 9457 problem body, if there is one. */
async function problemDetail(response: Response): Promise<string> {
  try {
    const problem = (await response.json()) as { detail?: string }

    if (problem.detail) {
      return problem.detail
    }
  } catch {
    // Not a problem body, fall through to the generic message.
  }

  return `Request failed with status ${response.status}.`
}

async function apiFetch<T>(
  path: string,
  init: RequestInit & { accessToken?: string } = {}
): Promise<T> {
  const { accessToken, headers, ...request } = init
  const response = await fetch(`${envConfig.apiBaseUrl}${path}`, {
    ...request,
    headers: {
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...headers,
    },
    cache: "no-store",
  })

  if (!response.ok) {
    throw new BookingApiError(await problemDetail(response), response.status)
  }

  return (await response.json()) as T
}

/**
 * EVENTHUB-235 step 0: reserves tickets of a published event. The
 * reservation expires after 15 minutes unless the payment completes it.
 */
export async function createBooking(
  accessToken: string,
  eventId: string,
  numberOfTickets: number
): Promise<Booking> {
  return apiFetch<Booking>("/bookings", {
    accessToken,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ eventId, numberOfTickets }),
  })
}

/**
 * Step 1: turns the reservation into a pending payment. The amount is the
 * number of tickets times the event price, computed and stored by the
 * backend only; a repeated call returns the pending payment unchanged, so
 * the amount shown stays the amount that is charged.
 */
export async function startPayment(
  accessToken: string,
  bookingId: string
): Promise<Payment> {
  return apiFetch<Payment>(`/bookings/${bookingId}/payment`, {
    accessToken,
    method: "POST",
  })
}

/**
 * Step 2: completes the payment with the outcome of the simulated payment
 * provider. The amount must be the one shown to the user; any deviation is
 * rejected with 422 without changing anything.
 */
export async function simulatePayment(
  accessToken: string,
  bookingId: string,
  outcome: SimulatedPaymentOutcome,
  amount: number
): Promise<Payment> {
  return apiFetch<Payment>(`/bookings/${bookingId}/payment/simulate`, {
    accessToken,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ outcome, amount }),
  })
}

/** Step 3, any time: the stored payment process of the booking. */
export async function getPayment(
  accessToken: string,
  bookingId: string
): Promise<Payment> {
  return apiFetch<Payment>(`/bookings/${bookingId}/payment`, { accessToken })
}

export async function getOwnTickets(
  accessToken: string
): Promise<OwnBooking[]> {
  return apiFetch<OwnBooking[]>("/me/tickets", { accessToken })
}

export async function getOwnBookings(
  accessToken: string,
  status?: BookingStatus
): Promise<OwnBooking[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : ""

  return apiFetch<OwnBooking[]>(`/me/bookings${query}`, { accessToken })
}

export async function getOwnBooking(
  accessToken: string,
  bookingId: string
): Promise<OwnBooking> {
  return apiFetch<OwnBooking>(`/me/bookings/${bookingId}`, { accessToken })
}

export async function cancelBooking(
  accessToken: string,
  bookingId: string
): Promise<Booking> {
  return apiFetch<Booking>(`/bookings/${bookingId}/cancel`, {
    accessToken,
    method: "POST",
  })
}

export async function createEventRating(
  accessToken: string,
  eventId: string,
  score: number,
  text: string
): Promise<void> {
  await apiFetch<unknown>(`/events/${eventId}/ratings`, {
    accessToken,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ score, text }),
  })
}
