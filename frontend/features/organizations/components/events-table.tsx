import Link from "next/link"

import {
  EMPTY_VALUE,
  EventStatusBadge,
  formatEventDateTime,
  formatSoldTickets,
  type OrganizerEvent,
} from "@/features/events"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/features/shared/components/ui/table"

/** Booked seats are capacity minus available seats; unknown when those couldn't be loaded. */
function formatBookedSeats(event: OrganizerEvent) {
  if (event.availableSeats === null) {
    return EMPTY_VALUE
  }

  return formatSoldTickets(
    Math.max(0, event.capacity - event.availableSeats),
    event.capacity
  )
}

export function EventsTable({ events }: { events: OrganizerEvent[] | null }) {
  if (!events) {
    return (
      <p className="py-6 text-center text-sm text-destructive">
        Die Veranstaltungen können gerade nicht geladen werden. Bitte versuche
        es später erneut
      </p>
    )
  }

  if (events.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        Deine Organisation hat noch keine Veranstaltungen.
      </p>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Veranstaltung</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Datum</TableHead>
          <TableHead className="text-right">Tickets</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {events.map((event) => (
          <TableRow key={event.eventId}>
            <TableCell>
              <div className="flex flex-col">
                <Link
                  href={`/organizer/events/${event.eventId}`}
                  className="font-medium text-foreground underline-offset-4 hover:underline"
                >
                  {event.title}
                </Link>
                <span className="text-xs text-muted-foreground">
                  {event.location?.name ?? EMPTY_VALUE}
                </span>
              </div>
            </TableCell>
            <TableCell>
              <EventStatusBadge status={event.status} />
            </TableCell>
            <TableCell className="text-muted-foreground">
              {formatEventDateTime(event.startTime)}
            </TableCell>
            <TableCell className="text-right font-mono text-foreground tabular-nums">
              {formatBookedSeats(event)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
