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
    errors.title = "Bitte gib einen Titel ein."
  } else if (title.length < TITLE_MIN_LENGTH) {
    errors.title = `Mindestens ${TITLE_MIN_LENGTH} Zeichen.`
  } else if (title.length > TITLE_MAX_LENGTH) {
    errors.title = `Höchstens ${TITLE_MAX_LENGTH} Zeichen.`
  }

  if (draft.description.trim().length > DESCRIPTION_MAX_LENGTH) {
    errors.description = `Höchstens ${DESCRIPTION_MAX_LENGTH} Zeichen.`
  }

  const start = Date.parse(draft.startTime)
  const end = Date.parse(draft.endTime)

  if (!draft.startTime) {
    errors.startTime = "Bitte gib einen Beginn an."
  } else if (Number.isNaN(start)) {
    errors.startTime = "Gib ein gültiges Datum und eine gültige Uhrzeit ein."
  } else if (start <= Date.now()) {
    errors.startTime = "Der Beginn muss in der Zukunft liegen."
  }

  if (!draft.endTime) {
    errors.endTime = "Bitte gib ein Ende an."
  } else if (Number.isNaN(end)) {
    errors.endTime = "Gib ein gültiges Datum und eine gültige Uhrzeit ein."
  } else if (!Number.isNaN(start) && end <= start) {
    errors.endTime = "Das Ende muss nach dem Beginn liegen."
  }

  const capacity = Number(draft.capacity)

  if (!draft.capacity) {
    errors.capacity = "Bitte gib eine Kapazität an."
  } else if (!Number.isInteger(capacity) || capacity < 1) {
    errors.capacity = "Gib eine ganze Zahl von mindestens 1 ein."
  } else if (capacity > CAPACITY_MAX) {
    errors.capacity = `Höchstens ${CAPACITY_MAX.toLocaleString("de-DE")}.`
  }

  const price = Number(draft.price)

  if (!draft.price) {
    errors.price =
      "Bitte gib einen Preis an. 0 für eine kostenlose Veranstaltung."
  } else if (Number.isNaN(price) || price < 0) {
    errors.price = "Gib 0 oder einen positiven Betrag ein."
  } else if (price > PRICE_MAX) {
    errors.price = `Höchstens ${PRICE_MAX.toLocaleString("de-DE")}.`
  }

  if (!draft.categoryId) {
    errors.categoryId = "Bitte wähle eine Kategorie."
  }

  if (!draft.locationId) {
    errors.locationId = "Bitte wähle einen Veranstaltungsort."
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
