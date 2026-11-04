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

export interface OwnBookingLocation {
  locationId: string
  name: string
  city: string
  postalCode: string
  street: string
  houseNumber: string | null
}

export interface OwnBookingEvent {
  eventId: string
  title: string
  startTime: string
  endTime: string
  status: string
  location: OwnBookingLocation | null
}

export interface OwnBooking {
  bookingId: string
  numberOfTickets: number
  bookingStatus: BookingStatus
  paymentStatus: PaymentStatus | null
  failureReason?: PaymentFailureReason
  amount: string | null
  refundAmount: string | null
  ticketsValid: boolean
  ratingScore: number | null
  message?: string
  bookedAt: string
  expiresAt?: string
  event: OwnBookingEvent | null
}

export type TicketView = "upcoming" | "past" | "cancelled"
