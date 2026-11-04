import "server-only"
import type { BookingStatus, OwnBooking } from "@/features/bookings/lib/types"
import { envConfig } from "@/features/shared/lib/env"

/**
 * Talks to the Go API with the signed-in user's own access token, like the
 * other features do. Own tickets and ratings are always the caller's own;
 * the API checks that per request.
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
): Promise<void> {
  await apiFetch<unknown>(`/bookings/${bookingId}/cancel`, {
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
