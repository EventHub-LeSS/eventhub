import { SparklesIcon } from "lucide-react"

import { EventCard } from "@/features/catalog/components/event-card"
import { getRecommendedEvents } from "@/features/catalog/lib/get-recommended-events"
import { Card, CardContent } from "@/features/shared/components/ui/card"

/** One row of the catalog grid on large screens. */
const RECOMMENDATION_LIMIT = 3

/**
 * "Für dich empfohlen" above the catalog. Only shown to signed-in users;
 * hidden entirely when the recommendations cannot be loaded, since the
 * catalog below still lists every event.
 */
export async function RecommendedEvents() {
  const result = await getRecommendedEvents(RECOMMENDATION_LIMIT)

  if (result.status !== "ok") {
    return null
  }

  return (
    <section
      aria-labelledby="recommendations-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="recommendations-heading"
        className="flex items-center gap-2 text-lg font-semibold tracking-tight"
      >
        <SparklesIcon className="size-4 shrink-0" />
        Für dich empfohlen
      </h2>
      {result.events.length === 0 ? (
        <Card className="border border-dashed border-border py-8 ring-0">
          <CardContent className="flex flex-col items-center text-center text-sm text-muted-foreground">
            Aktuell haben wir keine Vorschläge für dich. Buche Veranstaltungen,
            damit wir passende Empfehlungen finden.
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {result.events.map((event) => (
            <EventCard key={event.eventId} event={event} />
          ))}
        </div>
      )}
    </section>
  )
}
