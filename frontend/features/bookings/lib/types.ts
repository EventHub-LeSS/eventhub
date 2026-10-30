/**
 * Mirrors `model.BookingModel` from the backend (see
 * backend/internal/model/bookings.go). A booking starts as a reservation that
 * expires after 15 minutes; only a completed payment confirms it.
 */
export type BookingStatus =
  | "reserved"
  | "confirmed"
  | "cancelled"
  | "failed"
  | "expired"

/** The booking as `POST /bookings` returns it. */
export interface Booking {
  bookingId: string
  userId: string | null
  eventId: string | null
  paymentId: string | null
  numberOfTickets: number
  status: BookingStatus
  expiresAt: string | null
  createdAt: string
  updatedAt: string
}

/** Mirrors `model.PaymentStatus`. */
export type PaymentStatus = "pending" | "paid" | "failed" | "refunded"

/** Mirrors `model.PaymentFailureReason`. */
export type PaymentFailureReason =
  | "declined"
  | "booking_cancelled"
  | "reservation_expired"
  | "event_unavailable"

/**
 * The outcome the client reports on behalf of the simulated payment provider
 * (EVENTHUB-235); no real payment provider is connected.
 */
export type SimulatedPaymentOutcome = "success" | "failure"

/**
 * Mirrors `model.PaymentResponse`: the payment process of a booking, built
 * from the stored payment and booking. `amount` is the number of tickets
 * times the event price, computed and stored by the backend only, and
 * arrives as an exact decimal string (e.g. "40") that keeps its precision.
 */
export interface Payment {
  paymentId: string
  bookingId: string
  eventId: string | null
  numberOfTickets: number
  amount: string
  status: PaymentStatus
  failureReason?: PaymentFailureReason
  bookingStatus: BookingStatus
  /** True only when the payment is paid, the booking confirmed and the event still takes place. */
  ticketsValid: boolean
  /** Human readable result of the payment process, from the stored state. */
  message: string
  /** Deadline for completing a pending payment; omitted once it is completed. */
  expiresAt?: string
  createdAt: string
  updatedAt: string
}

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
