import {
  CalendarIcon,
  ExternalLinkIcon,
  EuroIcon,
  type LucideIcon,
  MapPinIcon,
  PencilIcon,
  TagIcon,
} from "lucide-react"
import Link from "next/link"
import { notFound } from "next/navigation"

import { getCurrentUser } from "@/features/auth"
import { EventActions } from "@/features/events/components/event-actions"
import { EventStatusBadge } from "@/features/events/components/event-status-badge"
import { OccupancyGauge } from "@/features/events/components/occupancy-gauge"
import {
  computeOccupancyPercent,
  EMPTY_VALUE,
  formatAvailableSeats,
  formatEventCount,
  formatEventDateTime,
  formatEventPrice,
  formatEventRevenue,
  formatEventTimeRange,
} from "@/features/events/lib/format"
import { getOrganizerEvent } from "@/features/events/lib/get-organizer-events"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { formatStreet } from "@/features/shared/lib/location"

/** A key figure in the row above the details. */
function Stat({
  label,
  value,
  hint,
}: {
  label: string
  value: string
  hint?: string
}) {
  return (
    <Card size="sm">
      <CardContent className="flex flex-col gap-1">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="text-2xl font-semibold tabular-nums">{value}</p>
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      </CardContent>
    </Card>
  )
}

function Detail({
  icon: Icon,
  label,
  children,
}: {
  icon: LucideIcon
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex gap-3">
      <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
        <Icon className="size-4" aria-hidden="true" />
      </div>
      <div className="flex min-w-0 flex-col gap-0.5">
        <dt className="text-xs text-muted-foreground">{label}</dt>
        <dd className="text-sm font-medium break-words">{children}</dd>
      </div>
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
  const { location, soldTickets } = event
  const roles = user?.organizationRoles ?? []
  // Same rule as the "Bearbeiten" entry of EventActions.
  const canEdit =
    roles.includes("event_manager") &&
    event.status !== "completed" &&
    event.status !== "cancelled"
  const occupancyPercent = computeOccupancyPercent(
    event.availableSeats,
    event.capacity
  )
  // The API has no revenue field, so it is derived like on the dashboard.
  const revenue =
    soldTickets.status === "ok" ? Number(event.price) * soldTickets.count : null

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            {event.category && (
              <span className="text-xs font-bold tracking-wider text-purple-700 uppercase">
                {event.category.category}
              </span>
            )}
            <EventStatusBadge status={event.status} />
          </div>
          <h1 className="text-3xl font-extrabold tracking-tight break-words">
            {event.title}
          </h1>
          <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <CalendarIcon className="size-4 shrink-0" aria-hidden="true" />
            {formatEventTimeRange(event.startTime, event.endTime)}
          </p>
        </div>
        <div className="flex items-start gap-2">
          {event.status === "published" && (
            <Button
              render={<Link href={`/events/${event.eventId}`} />}
              nativeButton={false}
              variant="outline"
              size="sm"
            >
              <ExternalLinkIcon />
              Öffentliche Seite
            </Button>
          )}
          {canEdit && (
            <Button
              render={<Link href={`/organizer/events/${event.eventId}/edit`} />}
              nativeButton={false}
              variant="outline"
              size="sm"
            >
              <PencilIcon />
              Bearbeiten
            </Button>
          )}
          <EventActions
            eventId={event.eventId}
            eventTitle={event.title}
            status={event.status}
            withdrawnByModerator={event.withdrawnByModerator}
            showViewDetails={false}
            roles={roles}
          />
        </div>
      </header>

      <div className="grid grid-cols-[repeat(auto-fit,minmax(10rem,1fr))] gap-4">
        <Stat
          label="Auslastung"
          value={
            occupancyPercent === null ? EMPTY_VALUE : `${occupancyPercent}%`
          }
        />
        {soldTickets.status !== "forbidden" && (
          <Stat
            label="Verkaufte Tickets"
            value={
              soldTickets.status === "ok"
                ? formatEventCount(soldTickets.count)
                : EMPTY_VALUE
            }
            hint={`von ${formatEventCount(event.capacity)} Plätzen`}
          />
        )}
        <Stat
          label="Freie Plätze"
          value={
            event.availableSeats === null
              ? EMPTY_VALUE
              : formatEventCount(event.availableSeats)
          }
          hint={`Kapazität ${formatEventCount(event.capacity)}`}
        />
        {revenue !== null && (
          <Stat
            label="Umsatz"
            value={formatEventRevenue(revenue)}
            hint={`${formatEventPrice(event.price)} pro Ticket`}
          />
        )}
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="flex min-w-0 flex-col gap-6">
          {/* Each column has one growing card, so both end at the same line. */}
          <Card className="flex-1">
            <CardHeader>
              <CardTitle>Beschreibung</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="leading-relaxed whitespace-pre-line text-muted-foreground">
                {event.description ||
                  "Für diese Veranstaltung liegt noch keine Beschreibung vor."}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Details</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="grid gap-5 sm:grid-cols-2">
                <Detail icon={CalendarIcon} label="Datum & Uhrzeit">
                  {formatEventTimeRange(event.startTime, event.endTime)}
                </Detail>
                <Detail icon={MapPinIcon} label="Veranstaltungsort">
                  {location ? (
                    <span className="flex flex-col">
                      {location.addressAddition && (
                        <span>{location.addressAddition}</span>
                      )}
                      <span className="font-normal text-muted-foreground">
                        {formatStreet(location)}
                      </span>
                      <span className="font-normal text-muted-foreground">
                        {location.postalCode} {location.city}
                      </span>
                      {location.accessible && (
                        <span className="font-normal text-muted-foreground">
                          Barrierefrei
                        </span>
                      )}
                    </span>
                  ) : (
                    EMPTY_VALUE
                  )}
                </Detail>
                <Detail icon={TagIcon} label="Kategorie">
                  {event.category?.category ?? EMPTY_VALUE}
                </Detail>
                <Detail icon={EuroIcon} label="Preis">
                  {formatEventPrice(event.price)}
                </Detail>
              </dl>
            </CardContent>
          </Card>
        </div>

        <div className="flex min-w-0 flex-col gap-6">
          <Card className="flex-1">
            <CardHeader>
              <CardTitle>Auslastung</CardTitle>
              <CardDescription>Gebuchter Anteil der Kapazität</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-1 flex-col items-center justify-center gap-1">
              <OccupancyGauge percent={occupancyPercent} />
              <p className="text-sm text-muted-foreground">
                {formatAvailableSeats(event.availableSeats, event.capacity)}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Verlauf</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="flex flex-col gap-3">
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Erstellt</dt>
                  <dd className="text-right tabular-nums">
                    {formatEventDateTime(event.createdAt)}
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">
                    Zuletzt aktualisiert
                  </dt>
                  <dd className="text-right tabular-nums">
                    {formatEventDateTime(event.updatedAt)}
                  </dd>
                </div>
              </dl>
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  )
}
