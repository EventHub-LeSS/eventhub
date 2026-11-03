import { notFound } from "next/navigation"

import { getCurrentUser } from "@/features/auth"
import { EventActions } from "@/features/events/components/event-actions"
import { EventStatusBadge } from "@/features/events/components/event-status-badge"
import { OccupancyGauge } from "@/features/events/components/occupancy-gauge"
import {
  computeOccupancyPercent,
  EMPTY_VALUE,
  formatAvailableSeats,
  formatEventCapacity,
  formatEventDateTime,
  formatEventPrice,
  formatEventTimeRange,
  formatSoldTickets,
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
  const [result, user] = await Promise.all([
    getOrganizerEvent(eventId),
    getCurrentUser(),
  ])

  // Also covers events of other organizations, so URLs cannot reach them.
  if (result.status === "not-found") {
    notFound()
  }

  if (result.status === "unavailable") {
    return (
      <p className="text-sm text-destructive">
        Diese Veranstaltung kann gerade nicht geladen werden. Bitte versuche es
        später erneut.
      </p>
    )
  }

  const { event } = result
  const { location } = event
  const occupancyPercent = computeOccupancyPercent(
    event.availableSeats,
    event.capacity
  )

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
          status={event.status}
          showViewDetails={false}
          roles={user?.organizationRoles ?? []}
        />
      </div>

      <div className="flex w-full flex-col items-center gap-1 rounded-xl border border-border px-6 py-4">
        <OccupancyGauge percent={occupancyPercent} />
        <p className="text-sm text-muted-foreground">
          {formatAvailableSeats(event.availableSeats, event.capacity)}
        </p>
      </div>

      <dl className="grid gap-4 sm:grid-cols-2">
        <Detail label="Datum & Uhrzeit">
          {formatEventTimeRange(event.startTime, event.endTime)}
        </Detail>
        <Detail label="Veranstaltungsort">
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
        <Detail label="Kategorie">
          {event.category?.category ?? EMPTY_VALUE}
        </Detail>
        <Detail label="Kapazität">{formatEventCapacity(event.capacity)}</Detail>
        {event.soldTickets.status !== "forbidden" && (
          <Detail label="Verkaufte Tickets">
            {event.soldTickets.status === "ok"
              ? formatSoldTickets(event.soldTickets.count, event.capacity)
              : EMPTY_VALUE}
          </Detail>
        )}
        <Detail label="Preis">{formatEventPrice(event.price)}</Detail>
        <Detail label="Erstellt">{formatEventDateTime(event.createdAt)}</Detail>
        <Detail label="Zuletzt aktualisiert">
          {formatEventDateTime(event.updatedAt)}
        </Detail>
      </dl>
    </>
  )
}
