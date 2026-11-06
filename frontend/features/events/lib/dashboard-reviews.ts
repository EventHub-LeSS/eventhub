export interface DashboardReview {
  ratingId: string
  eventId: string
  eventTitle: string
  score: number
  text: string
  authorName: string | null
  createdAt: string
}

export type StarScore = 1 | 2 | 3 | 4 | 5

export const STAR_SCORES: readonly StarScore[] = [5, 4, 3, 2, 1]

export interface ReviewStats {
  average: number | null
  total: number
  counts: Record<StarScore, number>
}

export function computeReviewStats(reviews: DashboardReview[]): ReviewStats {
  const counts: Record<StarScore, number> = { 1: 0, 2: 0, 3: 0, 4: 0, 5: 0 }
  let sum = 0

  for (const review of reviews) {
    counts[review.score as StarScore] += 1
    sum += review.score
  }

  return {
    average: reviews.length > 0 ? sum / reviews.length : null,
    total: reviews.length,
    counts,
  }
}

export function filterReviewsByScore(
  reviews: DashboardReview[],
  scores: ReadonlySet<StarScore>
): DashboardReview[] {
  return scores.size === 0
    ? reviews
    : reviews.filter((review) => scores.has(review.score as StarScore))
}

export function sortReviewsNewestFirst(
  reviews: DashboardReview[]
): DashboardReview[] {
  return reviews.toSorted(
    (a, b) =>
      b.createdAt.localeCompare(a.createdAt) ||
      b.ratingId.localeCompare(a.ratingId)
  )
}
