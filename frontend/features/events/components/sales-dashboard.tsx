import Link from "next/link"

import { EventStatusBadge } from "@/features/events/components/event-status-badge"
import type { OrganizerDashboardEvent } from "@/features/events/lib/dashboard"
import { formatEventRevenue } from "@/features/events/lib/format"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/features/shared/components/ui/table"

export function SalesDashboard({
  events,
}: {
  events: OrganizerDashboardEvent[]
}) {
  // Revenue of an event whose figures are hidden stays out of the sum, so the
  // total never counts a silent zero.
  const totalRevenue = events.reduce(
    (sum, event) => (event.statsUnavailable ? sum : sum + event.revenue),
    0
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>Verkäufe</CardTitle>
        <CardDescription>
          Bestätigt verkaufte Tickets und freie Plätze pro Veranstaltung
        </CardDescription>
        <CardAction>
          <div className="text-right">
            <p className="text-xs text-muted-foreground">Gesamtumsatz</p>
            <p className="font-medium tabular-nums">
              {formatEventRevenue(totalRevenue)}
            </p>
          </div>
        </CardAction>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Veranstaltung</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Verkauft</TableHead>
              <TableHead className="text-right">Frei</TableHead>
              <TableHead className="text-right">Umsatz</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {events.map((event) => (
              <TableRow key={event.eventId}>
                <TableCell>
                  <Link
                    href={`/organizer/events/${event.eventId}`}
                    className="text-sm font-medium underline-offset-4 hover:underline"
                  >
                    {event.title}
                  </Link>
                </TableCell>
                <TableCell>
                  <EventStatusBadge status={event.status} />
                </TableCell>
                {event.statsUnavailable ? (
                  <TableCell
                    colSpan={3}
                    className="text-center text-xs text-muted-foreground"
                  >
                    Ticketzahlen nicht verfügbar
                  </TableCell>
                ) : (
                  <>
                    <TableCell className="text-right font-mono tabular-nums">
                      {event.soldTickets.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono tabular-nums">
                      {event.availableSeats.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono tabular-nums">
                      {formatEventRevenue(event.revenue)}
                    </TableCell>
                  </>
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
