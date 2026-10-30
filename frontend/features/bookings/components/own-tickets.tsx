"use client"

import { Tabs } from "@base-ui/react/tabs"
import {
  CalendarIcon,
  ChevronRightIcon,
  CompassIcon,
  HistoryIcon,
  MapPinIcon,
  StarIcon,
  TicketIcon,
  TicketXIcon,
  type LucideIcon,
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
import type {
  OwnBooking,
  OwnBookingEvent,
  TicketView,
} from "@/features/bookings/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Card, CardContent } from "@/features/shared/components/ui/card"
import { cn } from "@/features/shared/lib/utils"

interface OwnTicketsProps {
  upcoming: OwnBooking[]
  past: OwnBooking[]
  cancelled: OwnBooking[]
  initialView: TicketView
}

interface LocalRating {
  score?: number
  message: string
}

const VIEWS: Record<
  TicketView,
  { label: string; description: string; empty: string; icon: LucideIcon }
> = {
  upcoming: {
    label: "Bevorstehend",
    description:
      "Deine gültigen Tickets für kommende Veranstaltungen. Brauchst du sie nicht mehr, kannst du sie stornieren.",
    empty: "Du hast keine Tickets für bevorstehende Veranstaltungen.",
    icon: TicketIcon,
  },
  past: {
    label: "Vergangen",
    description:
      "Veranstaltungen, die schon stattgefunden haben. Erzähl anderen, wie es war, und bewerte sie.",
    empty: "Du warst noch bei keiner Veranstaltung.",
    icon: HistoryIcon,
  },
  cancelled: {
    label: "Storniert",
    description:
      "Buchungen, die du storniert hast, und was mit deiner Zahlung passiert ist.",
    empty: "Du hast keine Buchung storniert.",
    icon: TicketXIcon,
  },
}

export function OwnTickets({
  upcoming,
  past,
  cancelled,
  initialView,
}: OwnTicketsProps) {
  const [view, setView] = useState<TicketView>(initialView)
  const [cancelling, setCancelling] = useState<OwnBooking | null>(null)
  const [ratingFor, setRatingFor] = useState<OwnBookingEvent | null>(null)
  const [rated, setRated] = useState<Record<string, LocalRating>>({})

  const tickets: Record<TicketView, OwnBooking[]> = {
    upcoming,
    past,
    cancelled,
  }

  function handleViewChange(next: TicketView) {
    setView(next)
    window.history.replaceState(
      null,
      "",
      next === "upcoming" ? "/tickets" : `/tickets?view=${next}`
    )
  }

  function handleRated(eventId: string, score?: number, message?: string) {
    setRated((previous) => ({
      ...previous,
      [eventId]: { score, message: message ?? "Danke für deine Bewertung!" },
    }))
  }

  function renderTicket(ticket: OwnBooking) {
    if (view === "upcoming") {
      return (
        <TicketCard ticket={ticket}>
          <Button
            variant="destructive"
            size="sm"
            className="ml-auto"
            onClick={() => setCancelling(ticket)}
          >
            <TicketXIcon />
            Stornieren
          </Button>
        </TicketCard>
      )
    }

    if (view === "cancelled") {
      return (
        <TicketCard ticket={ticket} kind="cancelled">
          {ticket.paymentStatus === "refunded" && (
            <span className="text-sm font-medium text-emerald-700 tabular-nums dark:text-emerald-400">
              {formatAmount(ticket.refundAmount)} erstattet
            </span>
          )}
          <Button
            render={<Link href={`/tickets/${ticket.bookingId}/cancellation`} />}
            nativeButton={false}
            size="sm"
            className="ml-auto"
          >
            Stornierung ansehen
            <ChevronRightIcon data-icon="inline-end" />
          </Button>
        </TicketCard>
      )
    }

    const { event } = ticket
    const local = event ? rated[event.eventId] : undefined
    const score = local?.score ?? ticket.ratingScore
    const hasScore = typeof score === "number" && score >= 1

    return (
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
              className="ml-auto"
              onClick={() => setRatingFor(event)}
            >
              <StarIcon />
              Veranstaltung bewerten
            </Button>
          ))}
      </TicketCard>
    )
  }

  return (
    <Tabs.Root
      value={view}
      onValueChange={(next) => handleViewChange(next as TicketView)}
      orientation="vertical"
      className="flex flex-1 flex-col md:flex-row"
    >
      <aside className="flex flex-col gap-6 border-b bg-muted/50 px-6 py-8 md:w-72 md:shrink-0 md:border-r md:border-b-0 md:px-6 md:py-12 lg:w-80 lg:px-8">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">
            Meine Tickets
          </h1>
          <p className="text-sm text-muted-foreground">
            Alle deine Buchungen an einem Ort.
          </p>
        </div>

        <Tabs.List
          aria-label="Ticketkategorien"
          className="-mx-2 flex gap-1 overflow-x-auto px-2 pb-1 md:mx-0 md:flex-col md:overflow-visible md:px-0 md:pb-0"
        >
          {(Object.keys(VIEWS) as TicketView[]).map((value) => {
            const { label, icon: Icon } = VIEWS[value]

            return (
              <Tabs.Tab
                key={value}
                value={value}
                className={cn(
                  "group flex shrink-0 items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium text-muted-foreground transition-colors outline-none hover:bg-background/60 hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 md:w-full",
                  "data-active:bg-background data-active:text-foreground data-active:shadow-sm data-active:ring-1 data-active:ring-foreground/10"
                )}
              >
                <Icon className="size-4 shrink-0" />
                {label}
                <span className="ml-auto rounded-md bg-foreground/5 px-1.5 py-0.5 text-xs text-muted-foreground tabular-nums group-data-active:bg-foreground group-data-active:text-background">
                  {tickets[value].length}
                </span>
              </Tabs.Tab>
            )
          })}
        </Tabs.List>

        <Button
          render={<Link href="/" />}
          nativeButton={false}
          variant="outline"
          className="mt-auto hidden self-start md:inline-flex"
        >
          <CompassIcon data-icon="inline-start" />
          Veranstaltungen entdecken
        </Button>
      </aside>

      {(Object.keys(VIEWS) as TicketView[]).map((value) => {
        const { label, description, empty, icon: Icon } = VIEWS[value]
        const list = tickets[value]

        return (
          <Tabs.Panel
            key={value}
            value={value}
            className="flex flex-1 flex-col gap-8 px-6 py-8 outline-none md:px-10 md:py-12 lg:px-16"
          >
            <div className="flex flex-col gap-1">
              <h2 className="text-2xl font-semibold tracking-tight lg:text-3xl">
                {label}
              </h2>
              <p className="max-w-prose text-muted-foreground">{description}</p>
            </div>

            {list.length === 0 ? (
              <div className="flex flex-1 flex-col items-center justify-center gap-4 rounded-2xl border border-dashed p-10 text-center">
                <div className="flex size-14 items-center justify-center rounded-2xl bg-muted">
                  <Icon className="size-7 text-muted-foreground" />
                </div>
                <p className="text-muted-foreground">{empty}</p>
                {value !== "cancelled" && (
                  <Button render={<Link href="/" />} nativeButton={false}>
                    Veranstaltungen entdecken
                  </Button>
                )}
              </div>
            ) : (
              <ul className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
                {list.map((ticket) => (
                  <li key={ticket.bookingId} className="flex">
                    {renderTicket(ticket)}
                  </li>
                ))}
              </ul>
            )}
          </Tabs.Panel>
        )
      })}

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
    </Tabs.Root>
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
  children,
}: {
  ticket: OwnBooking
  kind?: "valid" | "past" | "cancelled"
  children?: React.ReactNode
}) {
  const { event } = ticket
  const past = kind !== "valid"

  return (
    <Card className={cn("w-full gap-3", past && "opacity-90")}>
      <CardContent className="flex flex-1 flex-col gap-3">
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
                {ticket.amount !== null && ` · ${formatAmount(ticket.amount)}`}
              </dd>
            </div>
          </dl>
        )}

        {children && (
          <div className="mt-auto flex flex-wrap items-center gap-2 border-t pt-3">
            {children}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
