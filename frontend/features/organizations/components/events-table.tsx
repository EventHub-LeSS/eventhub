import type {
  EventStatus,
  OrganizationEvent,
} from "@/features/organizations/lib/mock-data"
import { Badge } from "@/features/shared/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/features/shared/components/ui/table"

const statusVariant: Record<EventStatus, "default" | "secondary" | "outline"> =
  {
    upcoming: "default",
    draft: "outline",
    past: "secondary",
  }

const statusLabel: Record<EventStatus, string> = {
  upcoming: "Bevorstehend",
  draft: "Entwurf",
  past: "Vergangen",
}

function formatDate(value: string) {
  return new Date(value).toLocaleDateString("de-DE", {
    year: "numeric",
    month: "short",
    day: "numeric",
  })
}

export function EventsTable({ events }: { events: OrganizationEvent[] }) {
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
          <TableRow key={event.id}>
            <TableCell>
              <div className="flex flex-col">
                <span className="font-medium text-foreground">
                  {event.name}
                </span>
                <span className="text-xs text-muted-foreground">
                  {event.location}
                </span>
              </div>
            </TableCell>
            <TableCell>
              <Badge variant={statusVariant[event.status]}>
                {statusLabel[event.status]}
              </Badge>
            </TableCell>
            <TableCell className="text-muted-foreground">
              {formatDate(event.date)}
            </TableCell>
            <TableCell className="text-right font-mono text-foreground tabular-nums">
              {event.ticketsSold} / {event.capacity}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
