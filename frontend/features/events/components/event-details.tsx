/**
 * Gemeinsamer Darstellungsbaustein für die Besucher- und Organizer-Detailseite.
 * Zeigt die übergebenen Event-Daten wie Titel, Beschreibung, Auslastung,
 * Datum, Ort, Kategorie und Preis im gleichen Aufbau an.
 * Lädt selbst keine Daten. Die jeweilige Detailseite liefert die Angaben
 * und kann zusätzliche Bedienelemente ergänzen: Organizer-Aktionen oder
 * den vorbereiteten „Buy ticket“-Bereich auf der Besucher-Detailseite.
 * So müssen Änderungen an der gemeinsamen Darstellung nur hier erfolgen.
 */
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
import type { OrganizerEventWithSales } from "@/features/events/lib/get-organizer-events"

export type EventDetailsData = Pick<
  OrganizerEventWithSales,
  | "title"
  | "description"
  | "status"
  | "startTime"
  | "endTime"
  | "location"
  | "category"
  | "capacity"
  | "availableSeats"
  | "price"
> &
  Partial<
    Pick<OrganizerEventWithSales, "soldTickets" | "createdAt" | "updatedAt">
  >
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

export function EventDetails({
  event,
  actions,
  children,
}: {
  event: EventDetailsData
  actions?: React.ReactNode
  children?: React.ReactNode
}) {
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
        {actions}
      </div>

      <div className="flex w-full flex-col items-center gap-1 rounded-xl border border-border px-6 py-4">
        <OccupancyGauge percent={occupancyPercent} />
        <p className="text-sm text-muted-foreground">
          {formatAvailableSeats(event.availableSeats, event.capacity)}
        </p>
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
        {event.soldTickets && event.soldTickets.status !== "forbidden" && (
          <Detail label="Sold tickets">
            {event.soldTickets.status === "ok"
              ? formatSoldTickets(event.soldTickets.count, event.capacity)
              : EMPTY_VALUE}
          </Detail>
        )}
        <Detail label="Price">{formatEventPrice(event.price)}</Detail>
        {event.createdAt && (
          <Detail label="Created">
            {formatEventDateTime(event.createdAt)}
          </Detail>
        )}
        {event.updatedAt && (
          <Detail label="Last updated">
            {formatEventDateTime(event.updatedAt)}
          </Detail>
        )}
      </dl>
      {children}
    </>
  )
}
