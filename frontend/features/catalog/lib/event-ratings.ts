/** A public rating of an event (see model.EventRating). */
export interface EventRating {
  ratingId: string
  score: number
  /** Empty if the reviewer left no text. */
  text: string
  /** First name of the reviewer, null if unknown. */
  authorName: string | null
  createdAt: string
}

export interface EventRatingsPage {
  items: EventRating[]
  page: number
  limit: number
  total: number
}

/** Ratings shown per page on the event detail page. */
export const EVENT_RATINGS_PAGE_SIZE = 10

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function isEventRating(value: unknown): value is EventRating {
  return (
    isRecord(value) &&
    typeof value.ratingId === "string" &&
    Number.isInteger(value.score) &&
    typeof value.score === "number" &&
    value.score >= 1 &&
    value.score <= 5 &&
    typeof value.text === "string" &&
    (value.authorName === null || typeof value.authorName === "string") &&
    typeof value.createdAt === "string"
  )
}

export function isEventRatingsPage(value: unknown): value is EventRatingsPage {
  return (
    isRecord(value) &&
    Array.isArray(value.items) &&
    value.items.every(isEventRating) &&
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

/** Reads `?reviewPage=n`; anything but a positive integer means the first page. */
export function parseReviewPage(value: string | string[] | undefined): number {
  if (typeof value !== "string" || !/^[1-9]\d*$/.test(value)) {
    return 1
  }
  const page = Number(value)
  return Number.isSafeInteger(page) ? page : 1
}

/** Link to a page of ratings on the event detail page. */
export function reviewPageHref(eventId: string, page: number): string {
  const path = `/events/${encodeURIComponent(eventId)}`
  return page > 1 ? `${path}?reviewPage=${page}` : path
}
