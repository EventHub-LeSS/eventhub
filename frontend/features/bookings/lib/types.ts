import type { EventAddress } from "@/features/shared/lib/location"

/**
 * Mirrors the own-booking models of the backend (see
 * backend/internal/model/own_bookings.go). A booking starts as a
 * reservation that expires after 15 minutes; only a completed payment
 * confirms it.
 */
export type BookingStatus =
  | "reserved"
  | "confirmed"
  | "cancelled"
  | "failed"
  | "expired"

/** Mirrors `model.PaymentStatus`. */
export type PaymentStatus = "pending" | "paid" | "failed" | "refunded"

/** Mirrors `model.PaymentFailureReason`; `booking_cancelled` was added with the cart. */
export type PaymentFailureReason =
  | "declined"
  | "booking_cancelled"
  | "reservation_expired"
  | "event_unavailable"

export type OwnBookingLocation = EventAddress

export interface OwnBookingEvent {
  eventId: string
  title: string
  startTime: string
  endTime: string
  status: string
  location: OwnBookingLocation | null
}

/**
 * Mirrors `model.CancelledBy`: who cancelled tickets that had been paid for.
 * `organizer` — the event was withdrawn; `user` — the visitor cancelled the
 * booking after paying for it.
 */
export type CancelledBy = "organizer" | "user"

export interface OwnBooking {
  bookingId: string
  numberOfTickets: number
  bookingStatus: BookingStatus
  paymentStatus: PaymentStatus | null
  failureReason?: PaymentFailureReason
  amount: string | null
  refundAmount: string | null
  ticketsValid: boolean
  /** Set only for paid tickets that were cancelled; never valid then. */
  cancelledBy?: CancelledBy
  ratingScore: number | null
  message?: string
  bookedAt: string
  expiresAt?: string
  event: OwnBookingEvent | null
}

export type TicketView = "upcoming" | "past" | "cancelled"
