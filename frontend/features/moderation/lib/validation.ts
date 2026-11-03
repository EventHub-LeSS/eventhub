import type { ModerationFilters } from "@/features/moderation/lib/types"

export const MAX_REASON_LENGTH = 1000

export function isUUID(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    value
  )
}

export function reasonError(reason: string): string | undefined {
  const length = Array.from(reason.trim()).length
  if (length === 0) {
    return "Bitte gib eine Begründung an."
  }
  if (length > MAX_REASON_LENGTH) {
    return `Die Begründung darf höchstens ${MAX_REASON_LENGTH} Zeichen enthalten.`
  }
}

export function parseModerationFilters(
  query: URLSearchParams
):
  | { filters: ModerationFilters; error?: never }
  | { filters?: never; error: string } {
  const page = query.get("page") ?? "1"
  const limit = query.get("limit") ?? "50"
  if (
    !/^[1-9]\d*$/.test(page) ||
    !Number.isSafeInteger(Number(page)) ||
    !/^[1-9]\d*$/.test(limit) ||
    Number(limit) > 100 ||
    !Number.isSafeInteger((Number(page) - 1) * Number(limit))
  ) {
    return { error: "Ungültige Seitennummer oder Seitengröße (1–100)." }
  }
  const hidden = query.get("hidden")
  if (hidden !== null && hidden !== "true" && hidden !== "false") {
    return { error: "Ungültiger Sichtbarkeitsfilter." }
  }
  const eventId = query.get("eventId")
  if (eventId !== null && !isUUID(eventId)) {
    return { error: "Die Veranstaltungs-ID muss eine gültige UUID sein." }
  }
  return {
    filters: {
      page: Number(page),
      limit: Number(limit),
      ...(hidden !== null ? { hidden: hidden === "true" } : {}),
      ...(eventId !== null ? { eventId } : {}),
    },
  }
}

export function moderationQuery(filters: ModerationFilters): string {
  const query = new URLSearchParams({
    page: String(filters.page),
    limit: String(filters.limit),
  })
  if (filters.hidden !== undefined) query.set("hidden", String(filters.hidden))
  if (filters.eventId) query.set("eventId", filters.eventId)
  return query.toString()
}
