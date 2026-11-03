/**
 * Mirrors the cart models of the backend (see
 * backend/internal/model/cart.go): the cart of a user is every live
 * reservation without a payment. Each item holds its seats until
 * expiresAt (15 minutes); afterwards it is gone and the seats are free
 * again.
 */
export interface CartLocation {
  locationId: string
  name: string
  city: string
  postalCode: string
  street: string
  houseNumber: string | null
}

/** Mirrors `model.OwnBookingEvent`: the event of a cart item or payment item. */
export interface CartEvent {
  eventId: string
  title: string
  startTime: string
  endTime: string
  status: string
  location: CartLocation | null
}

/** One item of the cart: the reserved tickets of one event. */
export interface CartItem {
  /** ID of the reservation behind the item; used to change or remove it. */
  bookingId: string
  numberOfTickets: number
  /** Null when the event no longer exists. */
  event: CartEvent | null
  /** Price of one ticket, from the current event data, as an exact decimal string. */
  unitPrice: string
  /** Unit price times the number of tickets, from the current event data. */
  lineTotal: string
  /**
   * Seats free for others plus the seats this item holds: the most tickets
   * the item can have. The reservation keeps its tickets until it expires.
   */
  availableSeats: number
  /** False when the event is no longer published; such an item blocks the checkout. */
  bookable: boolean
  /** Deadline of the reservation; the tickets are released when it passes. */
  expiresAt: string
  createdAt: string
}

/**
 * Mirrors `model.OpenCartPayment`: the checkout of the caller that still
 * waits for its payment. Its items have left the cart — they hold their
 * seats until the payment completes or the shared deadline passes. Null
 * while no checkout is open; new items can only be checked out after the
 * open one completes.
 */
export interface OpenCartPayment {
  paymentId: string
  /** Stored sum of the line amounts, as an exact decimal string. */
  amount: string
  /** Reserved tickets over all items of the payment. */
  totalTickets: number
  /** Shared deadline of the payment's reservations. */
  expiresAt: string
}

/** Mirrors `model.CartResponse`. */
export interface Cart {
  items: CartItem[]
  /** Total number of reserved tickets over all items. */
  totalTickets: number
  /** Sum of the line totals, from the current event data. */
  totalAmount: string
  /** True when the cart has items and all of them belong to published events. */
  checkoutReady: boolean
  /**
   * The caller's checkout that is still open, if any: its items left the
   * cart and wait for their payment, so the cart reports the checkout
   * instead of letting the items vanish.
   */
  openPayment: OpenCartPayment | null
}

/** Mirrors `model.BookingStatus`. */
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

/**
 * The outcome the client reports on behalf of the simulated payment provider
 * (EVENTHUB-235); no real payment provider is connected.
 */
export type SimulatedPaymentOutcome = "success" | "failure"

/** One booking of a payment: the tickets of one event of the cart. */
export interface PaymentItem {
  bookingId: string
  /** Null when the event of the booking no longer exists. */
  event: CartEvent | null
  numberOfTickets: number
  /** Tickets times the event price at checkout time; the charged amount of this item. */
  lineAmount: string
  bookingStatus: BookingStatus
  /** True only when the payment is paid, this booking is confirmed and the event takes place. */
  ticketsValid: boolean
}

/**
 * Mirrors `model.PaymentResponse`: the payment process of a cart checkout.
 * `amount` is the sum of the line amounts of all items, computed and stored
 * by the backend only, and arrives as an exact decimal string (e.g. "40").
 */
export interface Payment {
  paymentId: string
  amount: string
  items: PaymentItem[]
  status: PaymentStatus
  failureReason?: PaymentFailureReason
  /** True only when the payment is paid and every item holds valid tickets. */
  ticketsValid: boolean
  /** Human readable result of the payment process, from the stored state. */
  message: string
  /** Deadline for completing a pending payment; omitted once it is completed. */
  expiresAt?: string
  createdAt: string
  updatedAt: string
}
