/**
 * The add-to-cart of a visitor who is not signed in: clicking "In den
 * Warenkorb" parks the typed ticket count here, because the reservation
 * itself only happens after the login roundtrip the server action sends
 * them through. Once the event page is reached again, the parked input is
 * taken up — signed-in visitors continue straight with the reservation,
 * anonymous ones at least find their count restored.
 */
export interface ParkedAddToCart {
  eventId: string
  ticketCount: string
}

interface StoredParkedAddToCart extends ParkedAddToCart {
  savedAt: number
}

const STORAGE_KEY = "eventhub:parked-cart-add"

/** Only worth continuing for as long as a reservation would live anyway. */
const MAX_AGE_MS = 10 * 60 * 1000

/** Parks the add-to-cart so it survives the login roundtrip. */
export function saveParkedAddToCart(parked: ParkedAddToCart): void {
  try {
    const stored: StoredParkedAddToCart = { ...parked, savedAt: Date.now() }

    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(stored))
  } catch {
    // Storage can be unavailable (blocked, private mode): the roundtrip
    // then simply loses the input, as it did before.
  }
}

/**
 * Takes the add-to-cart parked for this event, if there is a fresh one. The
 * entry is always consumed or dropped, so a parked add never reserves
 * again later.
 */
export function takeParkedAddToCart(eventId: string): ParkedAddToCart | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)

    if (raw === null) {
      return null
    }

    sessionStorage.removeItem(STORAGE_KEY)

    const stored = JSON.parse(raw) as Partial<StoredParkedAddToCart>

    if (
      stored.eventId !== eventId ||
      typeof stored.ticketCount !== "string" ||
      typeof stored.savedAt !== "number" ||
      Date.now() - stored.savedAt > MAX_AGE_MS
    ) {
      return null
    }

    return { eventId, ticketCount: stored.ticketCount }
  } catch {
    return null
  }
}
