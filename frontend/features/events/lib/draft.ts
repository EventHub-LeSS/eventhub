import type { Event } from "@/features/events/lib/types"

/** Form state. Every field is a string because that is what the inputs hold. */
export interface EventDraft {
  title: string
  description: string
  startTime: string
  endTime: string
  capacity: string
  price: string
  categoryId: string
  locationId: string
}

export const emptyDraft: EventDraft = {
  title: "",
  description: "",
  startTime: "",
  endTime: "",
  capacity: "",
  price: "",
  categoryId: "",
  locationId: "",
}

export type DraftErrors = Partial<Record<keyof EventDraft, string>>

/** Request body of POST /events/draft and PUT /events/{id}. */
export interface EventDraftPayload {
  organizationId?: string
  title: string
  description?: string
  startTime: string
  endTime: string
  capacity: number
  price: string
  categoryId: string
  locationId: string
}

const TITLE_MIN_LENGTH = 3
const TITLE_MAX_LENGTH = 200
const DESCRIPTION_MAX_LENGTH = 5000
const CAPACITY_MAX = 100_000
const PRICE_MAX = 10_000

/**
 * Mirrors the binding rules of the backend's CreateDraftRequest so the organizer
 * sees a problem before the request goes out. The API stays the authority, and
 * whatever it rejects is surfaced as a form-level message.
 */
export function validateDraft(draft: EventDraft): DraftErrors {
  const errors: DraftErrors = {}
  const title = draft.title.trim()

  if (!title) {
    errors.title = "A title is required."
  } else if (title.length < TITLE_MIN_LENGTH) {
    errors.title = `Use at least ${TITLE_MIN_LENGTH} characters.`
  } else if (title.length > TITLE_MAX_LENGTH) {
    errors.title = `Use at most ${TITLE_MAX_LENGTH} characters.`
  }

  if (draft.description.trim().length > DESCRIPTION_MAX_LENGTH) {
    errors.description = `Use at most ${DESCRIPTION_MAX_LENGTH} characters.`
  }

  const start = Date.parse(draft.startTime)
  const end = Date.parse(draft.endTime)

  if (!draft.startTime) {
    errors.startTime = "A start is required."
  } else if (Number.isNaN(start)) {
    errors.startTime = "Enter a valid date and time."
  } else if (start <= Date.now()) {
    errors.startTime = "The start must be in the future."
  }

  if (!draft.endTime) {
    errors.endTime = "An end is required."
  } else if (Number.isNaN(end)) {
    errors.endTime = "Enter a valid date and time."
  } else if (!Number.isNaN(start) && end <= start) {
    errors.endTime = "The end must be after the start."
  }

  const capacity = Number(draft.capacity)

  if (!draft.capacity) {
    errors.capacity = "A capacity is required."
  } else if (!Number.isInteger(capacity) || capacity < 1) {
    errors.capacity = "Enter a whole number of at least 1."
  } else if (capacity > CAPACITY_MAX) {
    errors.capacity = `Enter at most ${CAPACITY_MAX.toLocaleString("en")}.`
  }

  const price = Number(draft.price)

  if (!draft.price) {
    errors.price = "A price is required. Use 0 for a free event."
  } else if (Number.isNaN(price) || price < 0) {
    errors.price = "Enter 0 or a positive amount."
  } else if (price > PRICE_MAX) {
    errors.price = `Enter at most ${PRICE_MAX.toLocaleString("en")}.`
  }

  if (!draft.categoryId) {
    errors.categoryId = "A category is required."
  }

  if (!draft.locationId) {
    errors.locationId = "A location is required."
  }

  return errors
}

export function toDraftPayload(
  draft: EventDraft,
  organizationId?: string
): EventDraftPayload {
  const description = draft.description.trim()

  return {
    ...(organizationId ? { organizationId } : {}),
    title: draft.title.trim(),
    ...(description ? { description } : {}),
    startTime: new Date(draft.startTime).toISOString(),
    endTime: new Date(draft.endTime).toISOString(),
    capacity: Number(draft.capacity),
    price: draft.price.trim(),
    categoryId: draft.categoryId,
    locationId: draft.locationId,
  }
}

/** Cuts an ISO timestamp down to what a datetime-local input accepts. */
function toLocalInput(isoTime: string): string {
  const time = new Date(isoTime)

  if (Number.isNaN(time.getTime())) {
    return ""
  }

  const offset = time.getTimezoneOffset() * 60 * 1000

  return new Date(time.getTime() - offset).toISOString().slice(0, 16)
}

/** Prefills the form when an existing draft is edited. */
export function toDraft(event: Event): EventDraft {
  return {
    title: event.title,
    description: event.description ?? "",
    startTime: toLocalInput(event.startTime),
    endTime: toLocalInput(event.endTime),
    capacity: String(event.capacity),
    price: event.price,
    categoryId: event.categoryId ?? "",
    locationId: event.locationId ?? "",
  }
}
