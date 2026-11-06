"use server"

import { revalidatePath } from "next/cache"
import { redirect } from "next/navigation"

import { getSession, requireOrganizer } from "@/features/auth"
import type { SessionUser } from "@/features/auth"
import {
  createDraft,
  deleteEvent,
  EventApiError,
  getOwnEvent,
  publishEvent,
  restoreEventAsDraft,
  updateEvent,
  uploadEventImage,
  withdrawEvent,
} from "@/features/events/lib/api"
import {
  type DraftErrors,
  type EventDraft,
  toDraftPayload,
  validateDraft,
} from "@/features/events/lib/draft"
import {
  type DecodedImage,
  IMAGE_FAILED_PARAM,
  readImageFile,
} from "@/features/events/lib/image"

const DRAFTS_PATH = "/organizer/events"

export interface DraftActionResult {
  errors?: DraftErrors
  /** EVENTHUB-264: the image could not be used or stored. */
  imageError?: string
  /** Set when the request failed, e.g. a validation error from the API. */
  message?: string
}

/** Rereads the session so an action never trusts anything sent by the client. */
async function authorize(): Promise<{
  user: SessionUser
  accessToken: string
}> {
  const user = await requireOrganizer()
  const session = await getSession()

  if (!session) {
    redirect("/api/auth/login")
  }

  return { user, accessToken: session.accessToken }
}

/** The API's reasons for refusing an event action, in German for the organizer. */
const apiRefusals: Record<string, string> = {
  "event is incomplete":
    "Die Veranstaltung ist unvollständig. Titel, Beginn, Ende nach dem Beginn, Kapazität, Kategorie und Ort müssen angegeben sein.",
  "this event is already published":
    "Diese Veranstaltung ist bereits veröffentlicht.",
  "only draft or cancelled events can be published":
    "Nur Entwürfe und zurückgezogene Veranstaltungen können veröffentlicht werden.",
  "only cancelled events can be restored as draft":
    "Nur zurückgezogene Veranstaltungen können als Entwurf wiederhergestellt werden.",
  "events withdrawn by moderation cannot be restored or republished by organizers":
    "Diese Veranstaltung wurde von der Moderation zurückgezogen. Du kannst sie weder wiederherstellen noch erneut veröffentlichen.",
  "only published events can be withdrawn":
    "Nur veröffentlichte Veranstaltungen können zurückgezogen werden.",
  "invalid event status":
    "Diese Veranstaltung kann in ihrem aktuellen Status nicht bearbeitet werden.",
  "event not found": "Diese Veranstaltung gibt es nicht mehr.",
  forbidden: "Du darfst Veranstaltungen dieser Organisation nicht verwalten.",
  "missing organization role":
    "Du darfst Veranstaltungen dieser Organisation nicht verwalten.",
}

/** The only API message with a number in it, from lowering the capacity. */
const CAPACITY_BELOW_OCCUPIED = /^capacity must not be lower than the (\d+) /

/** The API answers in English; anything not translated gets a general message. */
function inGerman(error: EventApiError): string {
  const occupied = CAPACITY_BELOW_OCCUPIED.exec(error.message)?.[1]

  if (occupied) {
    return `Die Kapazität darf nicht unter den ${occupied} bereits verkauften oder reservierten Tickets liegen.`
  }

  return (
    apiRefusals[error.message] ??
    "Die Anfrage ist fehlgeschlagen. Bitte versuche es später erneut."
  )
}

/** Turns an expected API rejection into a form message, rethrows the rest. */
function failed(error: unknown): DraftActionResult {
  if (error instanceof EventApiError) {
    return { message: inGerman(error) }
  }

  throw error
}

/**
 * Validates the form and reads the optional image file in one go, so all
 * problems are reported together and nothing is sent before.
 */
async function checkInput(
  draft: EventDraft,
  image?: File
): Promise<{ result?: DraftActionResult; decodedImage?: DecodedImage }> {
  const errors = validateDraft(draft)
  const decoded = image ? await readImageFile(image) : undefined
  const imageError = decoded && "error" in decoded ? decoded.error : undefined

  if (Object.keys(errors).length > 0 || imageError) {
    return { result: { errors, imageError } }
  }

  return {
    decodedImage: decoded && !("error" in decoded) ? decoded : undefined,
  }
}

/** EVENTHUB-77 / AK1: save a new event as a draft, EVENTHUB-264 with its image. */
export async function saveDraftAction(
  draft: EventDraft,
  image?: File
): Promise<DraftActionResult> {
  const { result, decodedImage } = await checkInput(draft, image)

  if (result) {
    return result
  }

  const { user, accessToken } = await authorize()
  let eventId: string

  try {
    const created = await createDraft(
      accessToken,
      toDraftPayload(draft, user.activeOrganization ?? undefined)
    )
    eventId = created.eventId
  } catch (error) {
    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)

  if (decodedImage) {
    try {
      await uploadEventImage(accessToken, eventId, decodedImage)
    } catch (error) {
      if (!(error instanceof EventApiError)) {
        throw error
      }

      // The draft exists already, so staying on the "new" form would invite a
      // duplicate. The edit form tells the organizer to pick the image again.
      redirect(`${DRAFTS_PATH}/${eventId}/edit?${IMAGE_FAILED_PARAM}=1`)
    }
  }

  redirect(DRAFTS_PATH)
}

/** EVENTHUB-77 / AK4: edit an existing draft, EVENTHUB-264 with a new image. */
export async function updateDraftAction(
  eventId: string,
  draft: EventDraft,
  image?: File
): Promise<DraftActionResult> {
  const { result, decodedImage } = await checkInput(draft, image)

  if (result) {
    return result
  }

  const { accessToken } = await authorize()

  try {
    await updateEvent(accessToken, eventId, toDraftPayload(draft))
  } catch (error) {
    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)

  if (decodedImage) {
    try {
      await uploadEventImage(accessToken, eventId, decodedImage)
    } catch (error) {
      if (error instanceof EventApiError) {
        return {
          imageError: `Die Änderungen wurden gespeichert, das Bild aber nicht: ${inGerman(error)}`,
        }
      }

      throw error
    }
  }

  redirect(DRAFTS_PATH)
}

/** EVENTHUB-77 / AK5: publish a draft, EVENTHUB-288 or a withdrawn event. */
export async function publishDraftAction(
  eventId: string
): Promise<DraftActionResult> {
  const { accessToken } = await authorize()

  try {
    // The form requires a start in the future, but the API publishes events
    // whose start has passed, e.g. a withdrawn one that was never edited.
    const event = await getOwnEvent(accessToken, eventId)

    if (event && Date.parse(event.startTime) <= Date.now()) {
      return {
        message:
          event.status === "cancelled"
            ? "Der Beginn liegt in der Vergangenheit. Stelle die Veranstaltung als Entwurf wieder her und wähle einen neuen Termin."
            : "Der Beginn liegt in der Vergangenheit. Wähle unter „Bearbeiten“ einen neuen Termin, bevor du veröffentlichst.",
      }
    }

    await publishEvent(accessToken, eventId)
  } catch (error) {
    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)

  return {}
}

/** EVENTHUB-288: turn a withdrawn event back into a draft to edit it. */
export async function restoreDraftAction(
  eventId: string
): Promise<DraftActionResult> {
  const { accessToken } = await authorize()

  try {
    await restoreEventAsDraft(accessToken, eventId)
  } catch (error) {
    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)

  return {}
}

/** The API's technical reasons for refusing a deletion, in plain words. */
const deleteRefusals: Record<number, string> = {
  400: "Nur Entwürfe können gelöscht werden. Diese Veranstaltung ist kein Entwurf mehr.",
  403: "Du darfst keine Veranstaltungen dieser Organisation löschen.",
  404: "Diesen Entwurf gibt es nicht mehr.",
  409: "Für diese Veranstaltung gibt es bereits Buchungen, deshalb kann sie nicht gelöscht werden.",
}

/** EVENTHUB-256: delete a draft for good. */
export async function deleteDraftAction(
  eventId: string
): Promise<DraftActionResult> {
  const { accessToken } = await authorize()

  try {
    await deleteEvent(accessToken, eventId)
  } catch (error) {
    if (error instanceof EventApiError && deleteRefusals[error.status]) {
      return { message: deleteRefusals[error.status] }
    }

    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)

  return {}
}

/** EVENTHUB-82: withdraw a published event so it can no longer be booked. */
export async function withdrawEventAction(
  eventId: string
): Promise<DraftActionResult> {
  const { accessToken } = await authorize()

  try {
    await withdrawEvent(accessToken, eventId)
  } catch (error) {
    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)

  return {}
}
