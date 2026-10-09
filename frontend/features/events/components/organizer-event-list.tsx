import Link from "next/link"

import { EventActions } from "@/features/events/components/event-actions"
import { EventStatusBadge } from "@/features/events/components/event-status-badge"
import {
  EMPTY_VALUE,
  formatEventCapacity,
  formatEventPrice,
  formatEventTimeRange,
} from "@/features/events/lib/format"
import { getOrganizerEvents } from "@/features/events/lib/get-organizer-events"

export async function OrganizerEventList() {
  const result = await getOrganizerEvents()

  return (
    <section className="mt-4 flex flex-col gap-4">
      <div>
        <h2 className="font-medium">All events</h2>
        <p className="text-sm text-muted-foreground">
          Every event of your active organization, from drafts to completed
          ones.
        </p>
      </div>

      {result.status === "unavailable" && (
        <p className="text-sm text-destructive">
          Your events can&apos;t be loaded right now. Please try again later.
        </p>
      )}

      {result.status === "ok" && result.events.length === 0 && (
        <p className="text-sm text-muted-foreground">
          Your organization has no events yet.
        </p>
      )}

      {result.status === "ok" && result.events.length > 0 && (
        <ul className="flex flex-col gap-2">
          {result.events.map((event) => (
            <li
              key={event.eventId}
              className="flex items-center justify-between gap-3 rounded-lg border border-border p-4"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <Link
                    href={`/organizer/events/${event.eventId}`}
                    className="rounded-sm text-sm font-medium break-words underline-offset-4 outline-none hover:underline focus-visible:ring-3 focus-visible:ring-ring/50"
                  >
                    {event.title}
                  </Link>
                  <EventStatusBadge status={event.status} />
                </div>
                <p className="text-xs text-muted-foreground">
                  {formatEventTimeRange(event.startTime, event.endTime)} ·{" "}
                  {event.location?.name ?? EMPTY_VALUE} ·{" "}
                  {formatEventCapacity(event.capacity)} ·{" "}
                  {formatEventPrice(event.price)}
                </p>
              </div>
              <EventActions eventId={event.eventId} eventTitle={event.title} />
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
