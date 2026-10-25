/**
 * The checkout of a visitor who is not signed in: clicking "Zum Checkout"
 * parks the typed data here, because the booking itself only happens after
 * the login roundtrip the server action sends them through. Once the
 * event page is reached again, the parked checkout is taken up — signed-in
 * visitors continue straight with the booking, anonymous ones at least
 * find their input restored.
 */

export interface PendingCheckout {
  eventId: string
  ticketCount: string
  cardholder: string
  cardNumber: string
  cardExpiry: string
  cardCvc: string
}

interface StoredPendingCheckout extends PendingCheckout {
  savedAt: number
}

const STORAGE_KEY = "eventhub:pending-checkout"

/** Only worth continuing for as long as a reservation would live anyway. */
const MAX_AGE_MS = 10 * 60 * 1000

/** Parks the checkout so it survives the login roundtrip. */
export function savePendingCheckout(checkout: PendingCheckout): void {
  try {
    const stored: StoredPendingCheckout = { ...checkout, savedAt: Date.now() }

    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(stored))
  } catch {
    // Storage can be unavailable (blocked, private mode): the roundtrip
    // then simply loses the input, as it did before.
  }
}

/**
 * Takes the checkout parked for this event, if there is a fresh one. The
 * entry is always consumed or dropped, so a parked checkout never books
 * again later.
 */
export function takePendingCheckout(eventId: string): PendingCheckout | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)

    if (raw === null) {
      return null
    }

    sessionStorage.removeItem(STORAGE_KEY)

    const stored = JSON.parse(raw) as Partial<StoredPendingCheckout>

    if (
      stored.eventId !== eventId ||
      typeof stored.ticketCount !== "string" ||
      typeof stored.cardholder !== "string" ||
      typeof stored.cardNumber !== "string" ||
      typeof stored.cardExpiry !== "string" ||
      typeof stored.cardCvc !== "string" ||
      typeof stored.savedAt !== "number" ||
      Date.now() - stored.savedAt > MAX_AGE_MS
    ) {
      return null
    }

    return {
      eventId,
      ticketCount: stored.ticketCount,
      cardholder: stored.cardholder,
      cardNumber: stored.cardNumber,
      cardExpiry: stored.cardExpiry,
      cardCvc: stored.cardCvc,
    }
  } catch {
    return null
  }
}
