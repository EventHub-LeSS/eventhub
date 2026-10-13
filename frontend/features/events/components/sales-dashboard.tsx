import Link from "next/link"

import { EventStatusBadge } from "@/features/events/components/event-status-badge"
import { OccupancyBar } from "@/features/events/components/occupancy-bar"
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
        <CardTitle>Sales</CardTitle>
        <CardDescription>
          Confirmed tickets sold and free places per event
        </CardDescription>
        <CardAction>
          <div className="text-right">
            <p className="text-xs text-muted-foreground">Total revenue</p>
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
              <TableHead>Event</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Sold</TableHead>
              <TableHead className="text-right">Free</TableHead>
              <TableHead className="text-right">Revenue</TableHead>
              <TableHead className="text-right">Occupancy</TableHead>
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
                    colSpan={4}
                    className="text-center text-xs text-muted-foreground"
                  >
                    Ticket figures unavailable
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
                    <TableCell className="flex items-center justify-end">
                      <OccupancyBar
                        sold={event.soldTickets}
                        capacity={event.capacity}
                        percent={event.occupancyPercent}
                        compact
                      />
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
