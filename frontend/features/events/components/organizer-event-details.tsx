import { notFound } from "next/navigation"

import { EventActions } from "@/features/events/components/event-actions"
import { EventStatusBadge } from "@/features/events/components/event-status-badge"
import {
  EMPTY_VALUE,
  formatEventCapacity,
  formatEventDateTime,
  formatEventPrice,
  formatEventTimeRange,
} from "@/features/events/lib/format"
import { getOrganizerEvent } from "@/features/events/lib/get-organizer-events"

function Detail({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <dt className="text-sm font-medium">{label}</dt>
      <dd className="text-sm text-muted-foreground">{children}</dd>
    </div>
  )
}

export async function OrganizerEventDetails({ eventId }: { eventId: string }) {
  const result = await getOrganizerEvent(eventId)

  // Also covers events of other organizations, so URLs cannot reach them.
  if (result.status === "not-found") {
    notFound()
  }

  if (result.status === "unavailable") {
    return (
      <p className="text-sm text-destructive">
        This event can&apos;t be loaded right now. Please try again later.
      </p>
    )
  }

  const { event } = result
  const { location } = event

  return (
    <>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-lg font-medium break-words">{event.title}</h1>
            <EventStatusBadge status={event.status} />
          </div>
          {event.description && (
            <p className="text-sm text-muted-foreground">{event.description}</p>
          )}
        </div>
        <EventActions
          eventId={event.eventId}
          eventTitle={event.title}
          showViewDetails={false}
        />
      </div>

      <dl className="grid gap-4 sm:grid-cols-2">
        <Detail label="Date & time">
          {formatEventTimeRange(event.startTime, event.endTime)}
        </Detail>
        <Detail label="Location">
          {location ? (
            <span className="flex flex-col">
              <span>{location.name}</span>
              <span>
                {[location.street, location.houseNumber]
                  .filter(Boolean)
                  .join(" ")}
              </span>
              <span>
                {location.postalCode} {location.city}
              </span>
            </span>
          ) : (
            EMPTY_VALUE
          )}
        </Detail>
        <Detail label="Category">
          {event.category?.category ?? EMPTY_VALUE}
        </Detail>
        <Detail label="Capacity">{formatEventCapacity(event.capacity)}</Detail>
        <Detail label="Price">{formatEventPrice(event.price)}</Detail>
        <Detail label="Created">{formatEventDateTime(event.createdAt)}</Detail>
        <Detail label="Last updated">
          {formatEventDateTime(event.updatedAt)}
        </Detail>
      </dl>
    </>
  )
}
