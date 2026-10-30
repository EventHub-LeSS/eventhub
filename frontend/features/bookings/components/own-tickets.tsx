"use client"

import {
  CalendarIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  MapPinIcon,
  StarIcon,
  TicketIcon,
  TicketXIcon,
} from "lucide-react"
import Link from "next/link"
import { useState } from "react"

import { CancelBookingDialog } from "@/features/bookings/components/cancel-booking-dialog"
import { RatingDialog } from "@/features/bookings/components/rating-dialog"
import {
  formatAmount,
  formatEventPeriod,
  formatLocation,
  formatTicketCount,
} from "@/features/bookings/lib/format"
import type { OwnBooking, OwnBookingEvent } from "@/features/bookings/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Card, CardContent } from "@/features/shared/components/ui/card"

interface OwnTicketsProps {
  upcoming: OwnBooking[]
  past: OwnBooking[]
  cancelled: OwnBooking[]
}

interface LocalRating {
  score?: number
  message: string
}

export function OwnTickets({ upcoming, past, cancelled }: OwnTicketsProps) {
  const [showPast, setShowPast] = useState(false)
  const [showCancelled, setShowCancelled] = useState(false)
  const [cancelling, setCancelling] = useState<OwnBooking | null>(null)
  const [ratingFor, setRatingFor] = useState<OwnBookingEvent | null>(null)
  const [rated, setRated] = useState<Record<string, LocalRating>>({})

  function handleRated(eventId: string, score?: number, message?: string) {
    setRated((previous) => ({
      ...previous,
      [eventId]: { score, message: message ?? "Danke für deine Bewertung!" },
    }))
  }

  return (
    <div className="flex flex-col gap-8">
      <section
        aria-labelledby="upcoming-heading"
        className="flex flex-col gap-3"
      >
        <h2 id="upcoming-heading" className="text-lg font-semibold">
          Bevorstehend
        </h2>
        {upcoming.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-start gap-3">
              <p className="text-sm text-muted-foreground">
                Du hast keine Tickets für bevorstehende Veranstaltungen.
              </p>
              <Button
                render={<Link href="/" />}
                nativeButton={false}
                variant="outline"
              >
                Veranstaltungen entdecken
              </Button>
            </CardContent>
          </Card>
        ) : (
          <ul className="flex flex-col gap-3">
            {upcoming.map((ticket) => (
              <li key={ticket.bookingId}>
                <TicketCard ticket={ticket}>
                  <div>
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => setCancelling(ticket)}
                    >
                      <TicketXIcon />
                      Stornieren
                    </Button>
                  </div>
                </TicketCard>
              </li>
            ))}
          </ul>
        )}
      </section>

      {past.length > 0 && (
        <section aria-labelledby="past-heading" className="flex flex-col gap-3">
          <h2 id="past-heading">
            <Button
              variant="ghost"
              aria-expanded={showPast}
              aria-controls="past-tickets"
              onClick={() => setShowPast((shown) => !shown)}
              className="-ml-2.5 text-lg font-semibold"
            >
              Vergangene Veranstaltungen ({past.length})
              <ChevronDownIcon
                className={
                  showPast
                    ? "rotate-180 transition-transform"
                    : "transition-transform"
                }
              />
            </Button>
          </h2>
          {showPast && (
            <ul id="past-tickets" className="flex flex-col gap-3">
              {past.map((ticket) => {
                const { event } = ticket
                const local = event ? rated[event.eventId] : undefined
                const score = local?.score ?? ticket.ratingScore
                const hasScore = typeof score === "number" && score >= 1

                return (
                  <li key={ticket.bookingId}>
                    <TicketCard ticket={ticket} kind="past">
                      {event &&
                        (hasScore || local ? (
                          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
                            {hasScore && <RatingStars score={score} />}
                            <span>{local?.message ?? "Deine Bewertung"}</span>
                          </div>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setRatingFor(event)}
                          >
                            <StarIcon />
                            Veranstaltung bewerten
                          </Button>
                        ))}
                    </TicketCard>
                  </li>
                )
              })}
            </ul>
          )}
        </section>
      )}

      {cancelled.length > 0 && (
        <section
          aria-labelledby="cancelled-heading"
          className="flex flex-col gap-3"
        >
          <h2 id="cancelled-heading">
            <Button
              variant="ghost"
              aria-expanded={showCancelled}
              aria-controls="cancelled-tickets"
              onClick={() => setShowCancelled((shown) => !shown)}
              className="-ml-2.5 text-lg font-semibold"
            >
              Stornierte Tickets ({cancelled.length})
              <ChevronDownIcon
                className={
                  showCancelled
                    ? "rotate-180 transition-transform"
                    : "transition-transform"
                }
              />
            </Button>
          </h2>
          {showCancelled && (
            <ul id="cancelled-tickets" className="flex flex-col gap-3">
              {cancelled.map((ticket) => (
                <li key={ticket.bookingId}>
                  <TicketCard
                    ticket={ticket}
                    kind="cancelled"
                    aside={
                      <Button
                        render={
                          <Link
                            href={`/tickets/${ticket.bookingId}/cancellation`}
                          />
                        }
                        nativeButton={false}
                        size="sm"
                      >
                        Stornierung ansehen
                        <ChevronRightIcon data-icon="inline-end" />
                      </Button>
                    }
                  />
                </li>
              ))}
            </ul>
          )}
        </section>
      )}

      {cancelling && (
        <CancelBookingDialog
          key={cancelling.bookingId}
          ticket={cancelling}
          open
          onOpenChange={(open) => {
            if (!open) {
              setCancelling(null)
            }
          }}
        />
      )}

      {ratingFor && (
        <RatingDialog
          key={ratingFor.eventId}
          eventId={ratingFor.eventId}
          eventTitle={ratingFor.title}
          open
          onOpenChange={(open) => {
            if (!open) {
              setRatingFor(null)
            }
          }}
          onRated={(score, message) =>
            handleRated(ratingFor.eventId, score, message)
          }
        />
      )}
    </div>
  )
}

function RatingStars({ score }: { score: number }) {
  return (
    <span className="flex items-center gap-0.5">
      <span className="sr-only">{score} von 5 Sternen</span>
      {[1, 2, 3, 4, 5].map((value) => (
        <StarIcon
          key={value}
          aria-hidden
          className={
            value <= score
              ? "size-4 fill-amber-400 text-amber-400"
              : "size-4 text-muted-foreground"
          }
        />
      ))}
    </span>
  )
}

function TicketCard({
  ticket,
  kind = "valid",
  aside,
  children,
}: {
  ticket: OwnBooking
  kind?: "valid" | "past" | "cancelled"
  /** Shown to the right of the details, below them on narrow screens. */
  aside?: React.ReactNode
  children?: React.ReactNode
}) {
  const { event } = ticket
  const past = kind !== "valid"

  return (
    <Card className={past ? "gap-3 opacity-90" : "gap-3"}>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          {event && !past ? (
            <Link
              href={`/events/${event.eventId}`}
              className="font-medium underline-offset-4 hover:underline"
            >
              {event.title}
            </Link>
          ) : (
            <p className="font-medium">
              {event?.title ?? "Veranstaltung nicht mehr verfügbar"}
            </p>
          )}
          {kind === "cancelled" ? (
            <Badge variant="destructive">Storniert</Badge>
          ) : past ? (
            <Badge variant="outline">Vergangen</Badge>
          ) : (
            <Badge className="bg-emerald-600 text-white dark:bg-emerald-500 dark:text-black">
              Gültig
            </Badge>
          )}
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          {event && (
            <dl className="grid gap-1.5 text-sm text-muted-foreground">
              <div className="flex items-start gap-2">
                <dt className="sr-only">Zeitraum</dt>
                <CalendarIcon className="mt-0.5 size-4 shrink-0" />
                <dd>{formatEventPeriod(event.startTime, event.endTime)}</dd>
              </div>
              <div className="flex items-start gap-2">
                <dt className="sr-only">Ort</dt>
                <MapPinIcon className="mt-0.5 size-4 shrink-0" />
                <dd>{formatLocation(event.location)}</dd>
              </div>
              <div className="flex items-start gap-2">
                <dt className="sr-only">Tickets</dt>
                <TicketIcon className="mt-0.5 size-4 shrink-0" />
                <dd>
                  {formatTicketCount(ticket.numberOfTickets)}
                  {ticket.amount !== null &&
                    ` · ${formatAmount(ticket.amount)}`}
                </dd>
              </div>
            </dl>
          )}
          {aside && <div className="shrink-0 sm:ml-auto">{aside}</div>}
        </div>

        {children}
      </CardContent>
    </Card>
  )
}
