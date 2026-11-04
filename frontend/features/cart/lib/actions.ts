"use server"

import { redirect } from "next/navigation"

import { getSession } from "@/features/auth"
import {
  addCartItem,
  CartApiError,
  checkout,
  clearCart,
  getPayment,
  removeCartItem,
  simulatePayment,
  updateCartItem,
} from "@/features/cart/lib/api"
import type { Cart, Payment } from "@/features/cart/lib/types"
import {
  isSimulatedOutcome,
  isUUID,
  MAX_TICKETS_PER_EVENT,
  seatsExceededMessage,
} from "@/features/cart/lib/validation"

export interface CartActionResult {
  /** The stored cart after the change. */
  cart?: Cart
  /** Set when the change failed. */
  message?: string
}

export interface CheckoutActionResult {
  /** The stored payment of the checkout; the client navigates to its view. */
  payment?: Payment
  /** Set when the checkout failed. */
  message?: string
}

export interface PaymentActionResult {
  /** The stored payment after the call. */
  payment?: Payment
  /** Set when the call failed. */
  message?: string
}

/**
 * Rereads the session so an action never trusts anything sent by the client,
 * and sends an anonymous caller to the login with a way back.
 */
async function authorize(returnTo: string): Promise<string> {
  const session = await getSession()

  if (!session) {
    redirect(`/api/auth/login?returnTo=${encodeURIComponent(returnTo)}`)
  }

  return session.accessToken
}

/**
 * EVENTHUB-91 / AK4: seats taken since the page was loaded come back as a
 * technical problem detail ("capacity exceeded: requested 3, available 1");
 * the visitor gets the same message the form shows.
 */
function capacityExceededMessage(detail: string): string | undefined {
  const match = /^capacity exceeded: requested \d+, available (\d+)$/.exec(
    detail
  )

  return match ? seatsExceededMessage(Number(match[1])) : undefined
}

function validQuantity(quantity: number): boolean {
  return (
    typeof quantity === "number" &&
    Number.isInteger(quantity) &&
    quantity >= 1 &&
    quantity <= MAX_TICKETS_PER_EVENT
  )
}

/**
 * Reserves tickets of an event and puts them into the cart. The form
 * already validates the input, but a server action is a public endpoint, so
 * the same rules are enforced here again. The cart after the change comes
 * back, so the badge and the event page stay current without a refetch.
 */
export async function addToCartAction(
  eventId: string,
  quantity: number
): Promise<CartActionResult> {
  if (!isUUID(eventId)) {
    return { message: "Ungültige Veranstaltung." }
  }

  if (!validQuantity(quantity)) {
    return {
      message: `Bitte gib eine Ticketanzahl von 1 bis ${MAX_TICKETS_PER_EVENT} an.`,
    }
  }

  const accessToken = await authorize(`/events/${eventId}`)

  try {
    return { cart: await addCartItem(accessToken, eventId, quantity) }
  } catch (error) {
    if (error instanceof CartApiError) {
      return { message: addItemMessage(error) }
    }

    throw error
  }
}

/**
 * Sets the ticket count of one cart item. Like the cart view, the action
 * only ever deals with the caller's own items.
 */
export async function updateQuantityAction(
  bookingId: string,
  quantity: number
): Promise<CartActionResult> {
  if (!isUUID(bookingId)) {
    return { message: "Ungültiges Warenkorb-Item." }
  }

  if (!validQuantity(quantity)) {
    return {
      message: `Bitte gib eine Ticketanzahl von 1 bis ${MAX_TICKETS_PER_EVENT} an.`,
    }
  }

  const accessToken = await authorize("/cart")

  try {
    return { cart: await updateCartItem(accessToken, bookingId, quantity) }
  } catch (error) {
    if (error instanceof CartApiError) {
      return { message: changeItemMessage(error) }
    }

    throw error
  }
}

export async function removeItemAction(
  bookingId: string
): Promise<CartActionResult> {
  if (!isUUID(bookingId)) {
    return { message: "Ungültiges Warenkorb-Item." }
  }

  const accessToken = await authorize("/cart")

  try {
    return { cart: await removeCartItem(accessToken, bookingId) }
  } catch (error) {
    if (error instanceof CartApiError) {
      return { message: changeItemMessage(error) }
    }

    throw error
  }
}

export async function clearCartAction(): Promise<CartActionResult> {
  const accessToken = await authorize("/cart")

  try {
    return { cart: await clearCart(accessToken) }
  } catch (error) {
    if (error instanceof CartApiError) {
      return { message: error.message }
    }

    throw error
  }
}

/**
 * Turns all live reservations into one payment and returns it; the client
 * navigates to the payment view, so the user ends up with a shareable URL
 * instead of a redirect that cannot be bookmarked.
 */
export async function checkoutAction(): Promise<CheckoutActionResult> {
  const accessToken = await authorize("/cart")

  try {
    return { payment: (await checkout(accessToken)).payment }
  } catch (error) {
    if (error instanceof CartApiError) {
      return { message: checkoutMessage(error) }
    }

    throw error
  }
}

/**
 * Completes the pending payment with the simulated outcome. The amount sent
 * along is the stored amount shown to the user; the backend rejects any
 * deviation without changing anything. After a conflict the stored payment
 * is read back, so the view always shows what became of it.
 */
export async function completePaymentAction(
  paymentId: string,
  outcome: string,
  amount: number
): Promise<PaymentActionResult> {
  if (!isUUID(paymentId)) {
    return { message: "Ungültige Zahlungs-ID." }
  }

  if (!isSimulatedOutcome(outcome)) {
    return { message: "Ungültiges Simulationsergebnis." }
  }

  if (typeof amount !== "number" || !Number.isFinite(amount) || amount < 0) {
    return { message: "Ungültiger Betrag." }
  }

  const accessToken = await authorize(`/cart/payment/${paymentId}`)

  try {
    return {
      payment: await simulatePayment(accessToken, paymentId, outcome, amount),
    }
  } catch (error) {
    if (error instanceof CartApiError && error.status === 409) {
      return await storedPayment(accessToken, paymentId)
    }

    if (error instanceof CartApiError) {
      return { message: error.message }
    }

    throw error
  }
}

/** Reads the stored payment after a conflict, so the UI can show what became of it. */
async function storedPayment(
  accessToken: string,
  paymentId: string
): Promise<PaymentActionResult> {
  try {
    return { payment: await getPayment(accessToken, paymentId) }
  } catch (error) {
    if (error instanceof CartApiError) {
      return { message: error.message }
    }

    throw error
  }
}

/** German wording for the problems putting tickets into the cart can run into. */
function addItemMessage(error: CartApiError): string {
  switch (error.status) {
    case 404:
      return "Diese Veranstaltung gibt es nicht mehr."
    case 409:
      return (
        capacityExceededMessage(error.message) ??
        "Diese Veranstaltung kann aktuell nicht gebucht werden."
      )
    case 422:
      return `Pro Veranstaltung liegen höchstens ${MAX_TICKETS_PER_EVENT} Tickets im Warenkorb.`
    default:
      return error.message
  }
}

/** German wording for the problems changing or removing a cart item can run into. */
function changeItemMessage(error: CartApiError): string {
  switch (error.status) {
    case 404:
      return "Dieses Item ist nicht mehr in deinem Warenkorb."
    case 409:
      return (
        capacityExceededMessage(error.message) ??
        "Die Reservierung ist abgelaufen. Bitte aktualisiere deinen Warenkorb."
      )
    case 422:
      return `Bitte gib eine Ticketanzahl von 1 bis ${MAX_TICKETS_PER_EVENT} an.`
    default:
      return error.message
  }
}

/** German wording for the problems a checkout can run into. */
function checkoutMessage(error: CartApiError): string {
  switch (error.status) {
    case 409:
      return "Die Reservierungen sind abgelaufen oder eine Veranstaltung ist nicht mehr verfügbar. Bitte aktualisiere deinen Warenkorb."
    case 422:
      return "Dein Warenkorb ist leer."
    default:
      return error.message
  }
}
