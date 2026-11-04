"use client"

import { useQuery } from "@tanstack/react-query"

import type { Cart, CartItem, Payment } from "@/features/cart/lib/types"

/**
 * The cart is the user's live reservations, so its items expire while the
 * visitor looks at them; polling keeps the badge and the page current.
 */
export const CART_QUERY_KEY = ["cart"] as const

const REFETCH_INTERVAL_MS = 60_000

async function readJson<T>(response: Response): Promise<T> {
  const body = await response.json().catch(() => null)

  if (!response.ok) {
    const detail = (body as { detail?: unknown } | null)?.detail
    throw new Error(
      typeof detail === "string" && detail
        ? detail
        : `Anfrage fehlgeschlagen (${response.status}).`
    )
  }

  return body as T
}

async function fetchCart(): Promise<Cart> {
  return readJson(await fetch("/api/cart"))
}

/** The query key of one stored payment of a cart checkout. */
export function paymentQueryKey(paymentId: string) {
  return ["cart-payment", paymentId] as const
}

async function fetchPayment(paymentId: string): Promise<Payment> {
  return readJson(await fetch(`/api/cart/payments/${paymentId}`))
}

interface UseCartOptions {
  /** False while signed out: the BFF route needs a session. */
  enabled?: boolean
}

/**
 * The cart of the signed-in visitor via the BFF route, kept current by
 * polling. Everyone who changes the cart writes the response back into
 * this query, so badge, event page and cart page stay in sync.
 */
export function useCart(options: UseCartOptions = {}) {
  return useQuery<Cart, Error>({
    queryKey: CART_QUERY_KEY,
    queryFn: fetchCart,
    enabled: options.enabled ?? true,
    refetchInterval: REFETCH_INTERVAL_MS,
  })
}

/**
 * The stored payment of one cart checkout via the BFF route: line amounts,
 * total and status. Completing the payment writes the response back into
 * this query.
 */
export function usePayment(paymentId: string) {
  return useQuery<Payment, Error>({
    queryKey: paymentQueryKey(paymentId),
    queryFn: () => fetchPayment(paymentId),
  })
}

/** The live cart item of an event, if the cart holds one. */
export function cartItemOf(
  cart: Cart | null | undefined,
  eventId: string
): CartItem | undefined {
  return cart?.items.find((item) => item.event?.eventId === eventId)
}
