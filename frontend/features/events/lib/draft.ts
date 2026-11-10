import { formatInTimeZone, fromZonedTime } from "date-fns-tz"

import { EVENT_TIME_ZONE } from "@/features/events/lib/format"
import type { Event, EventLocation } from "@/features/events/lib/types"

/**
 * Form state. Every field but the accessibility checkbox is a string because
 * that is what the inputs hold.
 */
export interface EventDraft {
  title: string
  description: string
  startTime: string
  endTime: string
  capacity: string
  price: string
  categoryId: string
  street: string
  houseNumber: string
  postalCode: string
  city: string
  addressAddition: string
  accessible: boolean
}

export const emptyDraft: EventDraft = {
  title: "",
  description: "",
  startTime: "",
  endTime: "",
  capacity: "",
  price: "",
  categoryId: "",
  street: "",
  houseNumber: "",
  postalCode: "",
  city: "",
  addressAddition: "",
  accessible: false,
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
  /**
   * The backend links the event to the stored location with this address, or
   * creates one, so no locationId is sent.
   */
  location: {
    street: string
    houseNumber: string | null
    postalCode: string
    city: string
    addressAddition: string | null
    accessible: boolean
  }
}

const TITLE_MIN_LENGTH = 3
const TITLE_MAX_LENGTH = 200
const DESCRIPTION_MAX_LENGTH = 5000
const CAPACITY_MAX = 100_000
const PRICE_MAX = 10_000
const STREET_MAX_LENGTH = 200
const HOUSE_NUMBER_MAX_LENGTH = 20
const CITY_MAX_LENGTH = 100
const ADDRESS_ADDITION_MAX_LENGTH = 200

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

  const start = fromZonedTime(draft.startTime, EVENT_TIME_ZONE).getTime()
  const end = fromZonedTime(draft.endTime, EVENT_TIME_ZONE).getTime()

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

  const street = draft.street.trim()

  if (!street) {
    errors.street = "Bitte gib eine Straße ein."
  } else if (street.length > STREET_MAX_LENGTH) {
    errors.street = `Höchstens ${STREET_MAX_LENGTH} Zeichen.`
  }

  if (draft.houseNumber.trim().length > HOUSE_NUMBER_MAX_LENGTH) {
    errors.houseNumber = `Höchstens ${HOUSE_NUMBER_MAX_LENGTH} Zeichen.`
  }

  const postalCode = draft.postalCode.trim()

  if (!postalCode) {
    errors.postalCode = "Bitte gib eine Postleitzahl ein."
  } else if (!/^\d{5}$/.test(postalCode)) {
    errors.postalCode = "Gib genau 5 Ziffern ein."
  }

  const city = draft.city.trim()

  if (!city) {
    errors.city = "Bitte gib einen Ort ein."
  } else if (city.length > CITY_MAX_LENGTH) {
    errors.city = `Höchstens ${CITY_MAX_LENGTH} Zeichen.`
  }

  if (draft.addressAddition.trim().length > ADDRESS_ADDITION_MAX_LENGTH) {
    errors.addressAddition = `Höchstens ${ADDRESS_ADDITION_MAX_LENGTH} Zeichen.`
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
    startTime: fromZonedTime(draft.startTime, EVENT_TIME_ZONE).toISOString(),
    endTime: fromZonedTime(draft.endTime, EVENT_TIME_ZONE).toISOString(),
    capacity: Number(draft.capacity),
    price: draft.price.trim(),
    categoryId: draft.categoryId,
    location: {
      street: draft.street.trim(),
      houseNumber: draft.houseNumber.trim() || null,
      postalCode: draft.postalCode.trim(),
      city: draft.city.trim(),
      addressAddition: draft.addressAddition.trim() || null,
      accessible: draft.accessible,
    },
  }
}

function toLocalInput(isoTime: string): string {
  const time = new Date(isoTime)

  if (Number.isNaN(time.getTime())) {
    return ""
  }

  return formatInTimeZone(time, EVENT_TIME_ZONE, "yyyy-MM-dd'T'HH:mm")
}

/** Fills the address fields from a stored location. */
export function withLocation(
  draft: EventDraft,
  location: EventLocation
): EventDraft {
  return {
    ...draft,
    street: location.street,
    houseNumber: location.houseNumber ?? "",
    postalCode: location.postalCode,
    city: location.city,
    addressAddition: location.addressAddition ?? "",
    accessible: location.accessible,
  }
}

export function toDraft(event: Event, locations: EventLocation[]): EventDraft {
  const draft: EventDraft = {
    ...emptyDraft,
    title: event.title,
    description: event.description ?? "",
    startTime: toLocalInput(event.startTime),
    endTime: toLocalInput(event.endTime),
    capacity: String(event.capacity),
    price: event.price,
    categoryId: event.categoryId ?? "",
  }
  const location = locations.find(
    (candidate) => candidate.locationId === event.locationId
  )

  return location ? withLocation(draft, location) : draft
}
