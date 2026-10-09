import { EventCard } from "@/features/catalog/components/event-card"
import {
  getPublishedEvents,
  type PublishedEventsFilter,
} from "@/features/catalog/lib/get-published-events"
import { Card, CardContent } from "@/features/shared/components/ui/card"

function EmptyState({ children }: { children: React.ReactNode }) {
  return (
    <Card className="border border-dashed border-border py-8 ring-0">
      <CardContent className="flex flex-col items-center text-center text-sm text-muted-foreground">
        {children}
      </CardContent>
    </Card>
  )
}

interface EventListProps {
  filter?: PublishedEventsFilter
}

export async function EventList({ filter }: EventListProps) {
  const hasActiveFilter = Boolean(
    filter?.title || filter?.location || filter?.date || filter?.categoryId
  )
  const { events, unavailable } = await getPublishedEvents(filter)

  if (unavailable) {
    return (
      <EmptyState>
        Veranstaltungen können aktuell nicht geladen werden. Bitte versuche es
        später erneut.
      </EmptyState>
    )
  }

  if (events.length === 0) {
    return (
      <EmptyState>
        {hasActiveFilter
          ? "Keine Veranstaltungen gefunden, die zu deinem Filter passen."
          : "Aktuell sind keine Veranstaltungen veröffentlicht."}
      </EmptyState>
    )
  }

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {events.map((event) => (
        <EventCard key={event.eventId} event={event} />
      ))}
    </div>
  )
}
