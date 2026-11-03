import Link from "next/link"

import { EventStatusBadge } from "@/features/events/components/event-status-badge"
import { OccupancyBar } from "@/features/events/components/occupancy-bar"
import type { OrganizerDashboardEvent } from "@/features/events/lib/dashboard"
import { formatEventTimeRange } from "@/features/events/lib/format"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

export function OccupancyDashboard({
  events,
}: {
  events: OrganizerDashboardEvent[]
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Auslastung</CardTitle>
        <CardDescription>
          Bestätigte Buchungen als Anteil der Kapazität pro Veranstaltung
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {events.map((event) => (
          <article key={event.eventId} className="flex flex-col gap-2">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <Link
                href={`/organizer/events/${event.eventId}`}
                className="text-sm font-medium underline-offset-4 hover:underline"
              >
                {event.title}
              </Link>
              <EventStatusBadge status={event.status} />
            </div>
            <p className="text-xs text-muted-foreground">
              {formatEventTimeRange(event.startTime, event.endTime)}
            </p>
            {event.statsUnavailable ? (
              <p className="text-xs text-muted-foreground">
                Ticketzahlen für diese Veranstaltung nicht verfügbar.
              </p>
            ) : (
              <OccupancyBar
                sold={event.soldTickets}
                capacity={event.capacity}
                percent={event.occupancyPercent}
              />
            )}
          </article>
        ))}
      </CardContent>
    </Card>
  )
}