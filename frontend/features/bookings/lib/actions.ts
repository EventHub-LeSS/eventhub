"use server"

import { redirect } from "next/navigation"

import { getSession } from "@/features/auth"
import {
  BookingApiError,
  createBooking,
  getPayment,
  simulatePayment,
  startPayment,
} from "@/features/bookings/lib/api"
import type { Payment } from "@/features/bookings/lib/types"
import {
  isSimulatedOutcome,
  isUUID,
  MAX_TICKETS_PER_BOOKING,
} from "@/features/bookings/lib/validation"

export interface CreateBookingResult {
  /** Set when reserving failed, e.g. sold out. On success the action redirects to the payment view. */
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

/** Reads the stored payment after a conflict, so the UI can show what became of it. */
async function storedPayment(
  accessToken: string,
  bookingId: string
): Promise<PaymentActionResult> {
  try {
    return { payment: await getPayment(accessToken, bookingId) }
  } catch (error) {
    if (error instanceof BookingApiError) {
      return { message: error.message }
    }

    throw error
  }
}

/**
 * Reserves tickets and sends the user to the payment view. The form already
 * validates the input, but a server action is a public endpoint, so the same
 * rules are enforced here again.
 */
export async function createBookingAction(
  eventId: string,
  numberOfTickets: number
): Promise<CreateBookingResult> {
  if (!isUUID(eventId)) {
    return { message: "Ungültige Veranstaltung." }
  }

  if (
    typeof numberOfTickets !== "number" ||
    !Number.isInteger(numberOfTickets) ||
    numberOfTickets < 1 ||
    numberOfTickets > MAX_TICKETS_PER_BOOKING
  ) {
    return {
      message: `Bitte gib eine Ticketanzahl von 1 bis ${MAX_TICKETS_PER_BOOKING} an.`,
    }
  }

  const accessToken = await authorize(`/events/${eventId}`)

  let booking
  try {
    booking = await createBooking(accessToken, eventId, numberOfTickets)
  } catch (error) {
    if (error instanceof BookingApiError) {
      return { message: error.message }
    }

    throw error
  }

  if (!isUUID(booking.bookingId)) {
    return { message: "Die Buchung konnte nicht erstellt werden." }
  }

  redirect(`/tickets/${booking.bookingId}/payment`)
}

/**
 * Starts the payment process — or, when it was already started or completed,
 * reads what became of it, so the view always shows the stored state.
 */
export async function startPaymentAction(
  bookingId: string
): Promise<PaymentActionResult> {
  if (!isUUID(bookingId)) {
    return { message: "Ungültige Buchungs-ID." }
  }

  const accessToken = await authorize(`/tickets/${bookingId}/payment`)

  try {
    return { payment: await startPayment(accessToken, bookingId) }
  } catch (error) {
    if (error instanceof BookingApiError && error.status === 409) {
      return await storedPayment(accessToken, bookingId)
    }

    if (error instanceof BookingApiError) {
      return { message: error.message }
    }

    throw error
  }
}

/**
 * Completes the pending payment with the simulated outcome. The amount sent
 * along is the stored amount shown to the user; the backend rejects any
 * deviation without changing anything.
 */
export async function completePaymentAction(
  bookingId: string,
  outcome: string,
  amount: number
): Promise<PaymentActionResult> {
  if (!isUUID(bookingId)) {
    return { message: "Ungültige Buchungs-ID." }
  }

  if (!isSimulatedOutcome(outcome)) {
    return { message: "Ungültiges Simulationsergebnis." }
  }

  if (typeof amount !== "number" || !Number.isFinite(amount) || amount < 0) {
    return { message: "Ungültiger Betrag." }
  }

  const accessToken = await authorize(`/tickets/${bookingId}/payment`)

  try {
    return {
      payment: await simulatePayment(accessToken, bookingId, outcome, amount),
    }
  } catch (error) {
    if (error instanceof BookingApiError && error.status === 409) {
      return await storedPayment(accessToken, bookingId)
    }

    if (error instanceof BookingApiError) {
      return { message: error.message }
    }

    throw error
  }
}
