import "server-only"

import { isEventRatingsPage } from "@/features/catalog"
import {
  type DashboardReview,
  sortReviewsNewestFirst,
} from "@/features/events/lib/dashboard-reviews"
import type { OrganizerEvent } from "@/features/events/lib/get-organizer-events"
import { envConfig } from "@/features/shared/lib/env"

const PAGE_LIMIT = 100
const MAX_PAGES_PER_EVENT = 10

export type DashboardReviewsResult = {
  reviews: DashboardReview[]
  failedEvents: string[]
}

async function getAllEventRatings(
  event: OrganizerEvent
): Promise<DashboardReview[] | null> {
  const reviews: DashboardReview[] = []

  for (let page = 1; page <= MAX_PAGES_PER_EVENT; page++) {
    const query = new URLSearchParams({
      page: String(page),
      limit: String(PAGE_LIMIT),
    })
    const response = await fetch(
      `${envConfig.apiBaseUrl}/events/${encodeURIComponent(event.eventId)}/ratings?${query}`,
      { cache: "no-store" }
    )
    if (response.status === 404) {
      return reviews
    }
    if (!response.ok) {
      return null
    }
    const body: unknown = await response.json()
    if (!isEventRatingsPage(body)) {
      return null
    }

    for (const rating of body.items) {
      reviews.push({
        ...rating,
        eventId: event.eventId,
        eventTitle: event.title,
      })
    }

    if (body.items.length === 0 || page * body.limit >= body.total) {
      break
    }
  }

  return reviews
}

export async function getDashboardReviews(
  events: OrganizerEvent[]
): Promise<DashboardReviewsResult> {
  const rateable = events.filter(
    (event) => event.status === "published" || event.status === "completed"
  )

  const results = await Promise.all(
    rateable.map(async (event) => {
      try {
        return { event, reviews: await getAllEventRatings(event) }
      } catch {
        return { event, reviews: null }
      }
    })
  )

  return {
    reviews: sortReviewsNewestFirst(
      results.flatMap((result) => result.reviews ?? [])
    ),
    failedEvents: results
      .filter((result) => result.reviews === null)
      .map((result) => result.event.title),
  }
}
