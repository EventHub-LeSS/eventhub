import "server-only"
import type {
  Cart,
  Payment,
  SimulatedPaymentOutcome,
} from "@/features/cart/lib/types"
import { envConfig } from "@/features/shared/lib/env"

/**
 * Talks to the Go API with the signed-in user's own access token, like the
 * other features do. Carts and payments are always the caller's own; the
 * API checks that per request.
 */
export class CartApiError extends Error {
  constructor(
    message: string,
    public readonly status: number
  ) {
    super(message)
    this.name = "CartApiError"
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

async function apiRequest<T>(
  path: string,
  init: RequestInit & { accessToken?: string } = {}
): Promise<{ body: T; status: number }> {
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
    throw new CartApiError(await problemDetail(response), response.status)
  }

  return { body: (await response.json()) as T, status: response.status }
}

async function apiFetch<T>(
  path: string,
  init: RequestInit & { accessToken?: string } = {}
): Promise<T> {
  return (await apiRequest<T>(path, init)).body
}

/**
 * Reserves tickets of a published event and puts them into the cart. An
 * existing item for the event grows (at most 10 tickets per event) and its
 * reservation is renewed. Returns the cart after the change.
 */
export async function addCartItem(
  accessToken: string,
  eventId: string,
  quantity: number
): Promise<Cart> {
  return apiFetch<Cart>("/cart/items", {
    accessToken,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ eventId, quantity }),
  })
}

/**
 * Sets the ticket count of one cart item. The item keeps its reserved
 * seats; more tickets need free seats. Returns the cart after the change.
 */
export async function updateCartItem(
  accessToken: string,
  bookingId: string,
  quantity: number
): Promise<Cart> {
  return apiFetch<Cart>(`/cart/items/${bookingId}`, {
    accessToken,
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ quantity }),
  })
}

/** Removes one cart item and frees its seats immediately. */
export async function removeCartItem(
  accessToken: string,
  bookingId: string
): Promise<Cart> {
  return apiFetch<Cart>(`/cart/items/${bookingId}`, {
    accessToken,
    method: "DELETE",
  })
}

/** Removes all cart items of the caller; reservations with an open payment stay. */
export async function clearCart(accessToken: string): Promise<Cart> {
  return apiFetch<Cart>("/cart", { accessToken, method: "DELETE" })
}

/**
 * Turns all live reservations into one payment: the amount is the sum of
 * per-item line amounts, computed and stored by the backend only. A
 * repeated call returns the pending payment unchanged (created false), so
 * the amount shown stays the amount that is charged.
 */
export async function checkout(
  accessToken: string
): Promise<{ payment: Payment; created: boolean }> {
  const { body, status } = await apiRequest<Payment>("/cart/checkout", {
    accessToken,
    method: "POST",
  })

  return { payment: body, created: status === 201 }
}

/** The stored payment process of a cart checkout. */
export async function getPayment(
  accessToken: string,
  paymentId: string
): Promise<Payment> {
  return apiFetch<Payment>(`/payments/${paymentId}`, { accessToken })
}

/**
 * Completes the pending payment with the outcome of the simulated payment
 * provider. The amount must be the one shown to the user; any deviation is
 * rejected with 422 without changing anything.
 */
export async function simulatePayment(
  accessToken: string,
  paymentId: string,
  outcome: SimulatedPaymentOutcome,
  amount: number
): Promise<Payment> {
  return apiFetch<Payment>(`/payments/${paymentId}/simulate`, {
    accessToken,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ outcome, amount }),
  })
}
