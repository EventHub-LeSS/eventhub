"use server"

import { revalidatePath } from "next/cache"
import { redirect } from "next/navigation"

import { getSession } from "@/features/auth"
import {
  BookingApiError,
  cancelBooking,
  createEventRating,
} from "@/features/bookings/lib/api"
import {
  isRatingScore,
  isUUID,
  MAX_RATING_TEXT_LENGTH,
} from "@/features/bookings/lib/validation"

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
