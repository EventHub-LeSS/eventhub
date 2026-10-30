"use server"

import { revalidatePath } from "next/cache"
import { redirect } from "next/navigation"

import { getSession } from "@/features/auth"
import {
  BookingApiError,
  cancelBooking,
  createBooking,
  createEventRating,
  getPayment,
  simulatePayment,
  startPayment,
} from "@/features/bookings/lib/api"
import type { Payment } from "@/features/bookings/lib/types"
import {
  isRatingScore,
  isSimulatedOutcome,
  isUUID,
  MAX_RATING_TEXT_LENGTH,
  MAX_TICKETS_PER_BOOKING,
  seatsExceededMessage,
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

export interface CancelBookingResult {
  cancelled?: boolean
  message?: string
}

export interface RatingActionResult {
  rated?: boolean
  score?: number
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
      return {
        message: capacityExceededMessage(error.message) ?? error.message,
      }
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

export async function cancelBookingAction(
  bookingId: string
): Promise<CancelBookingResult> {
  if (!isUUID(bookingId)) {
    return { message: "Ungültige Buchungs-ID." }
  }

  const accessToken = await authorize("/tickets")

  try {
    await cancelBooking(accessToken, bookingId)
    revalidatePath("/tickets")
    return { cancelled: true }
  } catch (error) {
    if (error instanceof BookingApiError) {
      return { message: cancelErrorMessage(error) }
    }

    throw error
  }
}

function cancelErrorMessage(error: BookingApiError): string {
  switch (error.status) {
    case 404:
      return "Diese Buchung gibt es nicht mehr."
    case 409:
      return "Diese Buchung kann nicht mehr storniert werden."
    default:
      return error.message
  }
}

export async function createEventRatingAction(
  eventId: string,
  score: number,
  text: string
): Promise<RatingActionResult> {
  if (!isUUID(eventId)) {
    return { message: "Ungültige Veranstaltung." }
  }

  if (!isRatingScore(score)) {
    return { message: "Bitte vergib 1 bis 5 Sterne." }
  }

  if (typeof text !== "string" || text.length > MAX_RATING_TEXT_LENGTH) {
    return {
      message: `Der Text darf höchstens ${MAX_RATING_TEXT_LENGTH} Zeichen lang sein.`,
    }
  }

  const accessToken = await authorize("/tickets")

  try {
    await createEventRating(accessToken, eventId, score, text.trim())
    revalidatePath("/tickets")
    return { rated: true, score }
  } catch (error) {
    if (error instanceof BookingApiError) {
      if (error.status === 409 && /already been rated/.test(error.message)) {
        revalidatePath("/tickets")
        return {
          rated: true,
          message: "Du hast diese Veranstaltung bereits bewertet.",
        }
      }

      return { message: ratingErrorMessage(error) }
    }

    throw error
  }
}

function ratingErrorMessage(error: BookingApiError): string {
  switch (error.status) {
    case 403:
      return "Nur Besucher mit bestätigter Buchung können diese Veranstaltung bewerten."
    case 404:
      return "Diese Veranstaltung gibt es nicht mehr."
    case 409:
      return "Bewerten kannst du eine Veranstaltung erst, wenn sie stattgefunden hat."
    default:
      return error.message
  }
}
