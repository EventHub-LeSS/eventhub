export interface ModeratedRating {
  ratingId: string
  score: number
  text: string
  hidden: boolean
  eventId: string | null
  eventTitle: string | null
  organizationId: string | null
  organizationName: string | null
  createdAt: string
  updatedAt: string
}

export interface ModeratedRatingPage {
  items: ModeratedRating[]
  page: number
  limit: number
  total: number
}

export interface RatingVisibilityResponse {
  ratingId: string
  hidden: boolean
  changed: boolean
}

export interface ModerationFilters {
  page: number
  limit: number
  hidden?: boolean
  eventId?: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === "string"
}

function isRating(value: unknown): value is ModeratedRating {
  return (
    isRecord(value) &&
    typeof value.ratingId === "string" &&
    typeof value.score === "number" &&
    typeof value.text === "string" &&
    typeof value.hidden === "boolean" &&
    isNullableString(value.eventId) &&
    isNullableString(value.eventTitle) &&
    isNullableString(value.organizationId) &&
    isNullableString(value.organizationName) &&
    typeof value.createdAt === "string" &&
    typeof value.updatedAt === "string"
  )
}

export function isRatingPage(value: unknown): value is ModeratedRatingPage {
  return (
    isRecord(value) &&
    Array.isArray(value.items) &&
    value.items.every(isRating) &&
    Number.isSafeInteger(value.page) &&
    typeof value.page === "number" &&
    value.page >= 1 &&
    Number.isSafeInteger(value.limit) &&
    typeof value.limit === "number" &&
    value.limit >= 1 &&
    value.limit <= 100 &&
    Number.isSafeInteger(value.total) &&
    typeof value.total === "number" &&
    value.total >= 0
  )
}

export function isVisibilityResponse(
  value: unknown
): value is RatingVisibilityResponse {
  return (
    isRecord(value) &&
    typeof value.ratingId === "string" &&
    typeof value.hidden === "boolean" &&
    typeof value.changed === "boolean"
  )
}

export function moderationErrorMessage(value: unknown, status: number): string {
  if (isRecord(value)) {
    for (const key of ["detail", "message", "error"]) {
      if (typeof value[key] === "string" && value[key]) {
        return value[key]
      }
    }
    if (isRecord(value.error) && typeof value.error.message === "string") {
      return value.error.message
    }
  }
  return `Die Moderationsanfrage ist fehlgeschlagen (${status}).`
}
