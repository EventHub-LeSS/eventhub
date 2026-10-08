"use server"

import { revalidatePath } from "next/cache"
import { redirect } from "next/navigation"

import { getSession, requireOrganizer } from "@/features/auth"
import type { SessionUser } from "@/features/auth"
import {
  createDraft,
  EventApiError,
  publishEvent,
  updateEvent,
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
