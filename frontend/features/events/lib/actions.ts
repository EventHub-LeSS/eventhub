"use server"

import { revalidatePath } from "next/cache"
import { redirect } from "next/navigation"

import { getSession, requireOrganizer } from "@/features/auth"
import type { SessionUser } from "@/features/auth"
import {
  createDraft,
  deleteEvent,
  EventApiError,
  publishEvent,
  updateEvent,
  withdrawEvent,
} from "@/features/events/lib/api"
import {
  type DraftErrors,
  type EventDraft,
  toDraftPayload,
  validateDraft,
} from "@/features/events/lib/draft"

const DRAFTS_PATH = "/organizer/events"

export interface DraftActionResult {
  errors?: DraftErrors
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

/** Turns an expected API rejection into a form message, rethrows the rest. */
function failed(error: unknown): DraftActionResult {
  if (error instanceof EventApiError) {
    return { message: error.message }
  }

  throw error
}

/** EVENTHUB-77 / AK1: save a new event as a draft. */
export async function saveDraftAction(
  draft: EventDraft
): Promise<DraftActionResult> {
  const errors = validateDraft(draft)

  if (Object.keys(errors).length > 0) {
    return { errors }
  }

  const { user, accessToken } = await authorize()

  try {
    await createDraft(
      accessToken,
      toDraftPayload(draft, user.activeOrganization ?? undefined)
    )
  } catch (error) {
    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)
  redirect(DRAFTS_PATH)
}

/** EVENTHUB-77 / AK4: edit an existing draft. */
export async function updateDraftAction(
  eventId: string,
  draft: EventDraft
): Promise<DraftActionResult> {
  const errors = validateDraft(draft)

  if (Object.keys(errors).length > 0) {
    return { errors }
  }

  const { accessToken } = await authorize()

  try {
    await updateEvent(accessToken, eventId, toDraftPayload(draft))
  } catch (error) {
    return failed(error)
  }

  revalidatePath(DRAFTS_PATH)
  redirect(DRAFTS_PATH)
}

/** EVENTHUB-77 / AK5: publish a draft. */
export async function publishDraftAction(
  eventId: string
): Promise<DraftActionResult> {
  const { accessToken } = await authorize()

  try {
    await publishEvent(accessToken, eventId)
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
