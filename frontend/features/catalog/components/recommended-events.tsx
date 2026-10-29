import { RecommendationsHero } from "@/features/catalog/components/recommendations-hero"
import { getRecommendedEvents } from "@/features/catalog/lib/get-recommended-events"

/** Enough to rotate through without the dots turning into a long row. */
const RECOMMENDATION_LIMIT = 5

/**
 * "Für dich empfohlen" hero above the catalog. Only shown to signed-in
 * users with at least one recommendation; hidden entirely otherwise, since
 * the catalog below still lists every event.
 */
export async function RecommendedEvents() {
  const result = await getRecommendedEvents(RECOMMENDATION_LIMIT)

  if (result.status !== "ok" || result.events.length === 0) {
    return null
  }

  return <RecommendationsHero events={result.events} />
}
