export interface PendingCheckout {
  eventId: string;
  ticketCount: string;
}

interface StoredPendingCheckout extends PendingCheckout {
  savedAt: number;
}

/** What is being bought, as the event page passes it to the payment view. */
export interface CheckoutEvent {
  eventId: string;
  title: string;
  category: string;
  startTime: string;
  endTime: string;
  address: string;
  price: string;
}

const STORAGE_KEY = "eventhub:pending-checkout";
const EVENT_STORAGE_KEY = "eventhub:checkout-event";

/** Only worth continuing for as long as a reservation would live anyway. */
const MAX_AGE_MS = 10 * 60 * 1000;

/** Parks the checkout so it survives the login roundtrip. */
export function savePendingCheckout(checkout: PendingCheckout): void {
  try {
    const stored: StoredPendingCheckout = { ...checkout, savedAt: Date.now() };

    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(stored));
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
    const raw = sessionStorage.getItem(STORAGE_KEY);

    if (raw === null) {
      return null;
    }

    sessionStorage.removeItem(STORAGE_KEY);

    const stored = JSON.parse(raw) as Partial<StoredPendingCheckout>;

    if (
      stored.eventId !== eventId ||
      typeof stored.ticketCount !== "string" ||
      typeof stored.savedAt !== "number" ||
      Date.now() - stored.savedAt > MAX_AGE_MS
    ) {
      return null;
    }

    return { eventId, ticketCount: stored.ticketCount };
  } catch {
    return null;
  }
}

/**
 * Parks what is being bought for the payment view. It is kept (not
 * consumed), so reloading the payment view still shows it; the next
 * checkout simply replaces it.
 */
export function saveCheckoutEvent(event: CheckoutEvent): void {
  try {
    sessionStorage.setItem(EVENT_STORAGE_KEY, JSON.stringify(event));
  } catch {
    // Without storage the payment view falls back to a link to the event.
  }
}

/** Reads what the event page parked for this event, if anything. */
export function readCheckoutEvent(eventId: string): CheckoutEvent | null {
  try {
    const raw = sessionStorage.getItem(EVENT_STORAGE_KEY);

    if (raw === null) {
      return null;
    }

    const stored = JSON.parse(raw) as Partial<CheckoutEvent>;

    if (
      stored.eventId !== eventId ||
      typeof stored.title !== "string" ||
      typeof stored.category !== "string" ||
      typeof stored.startTime !== "string" ||
      typeof stored.endTime !== "string" ||
      typeof stored.address !== "string" ||
      typeof stored.price !== "string"
    ) {
      return null;
    }

    return {
      eventId,
      title: stored.title,
      category: stored.category,
      startTime: stored.startTime,
      endTime: stored.endTime,
      address: stored.address,
      price: stored.price,
    };
  } catch {
    return null;
  }
}
