import {
  ArrowLeftIcon,
  CalendarIcon,
  CircleCheckIcon,
  MapPinIcon,
  TicketXIcon,
} from "lucide-react"
import Link from "next/link"

import {
  formatAmount,
  formatDateTime,
  formatEventPeriod,
  formatLocation,
  formatTicketCount,
  paymentStatusLabels,
} from "@/features/bookings/lib/format"
import type { OwnBooking } from "@/features/bookings/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"

/**
 * What a cancellation took back: the tickets, and what became of the money.
 * Fills the screen as a voided ticket, with the event on the stub and the
 * cancellation past the perforation: below it on narrow screens, beside it
 * on wide ones.
 */
export function CancellationDetails({ booking }: { booking: OwnBooking }) {
  const { event } = booking
  const refunded = booking.paymentStatus === "refunded"

  return (
    <div
      data-testid="cancellation-details"
      className="flex flex-1 flex-col overflow-hidden md:flex-row"
    >
      <aside className="flex flex-col gap-10 bg-muted/50 px-6 py-8 md:w-2/5 md:max-w-xl md:shrink-0 md:px-10 md:py-12 lg:px-14">
        <Button
          render={<Link href="/tickets" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
          className="-ml-2.5 self-start"
        >
          <ArrowLeftIcon data-icon="inline-start" />
          Meine Tickets
        </Button>

        <div className="flex flex-1 flex-col justify-center gap-6">
          <div className="flex size-16 items-center justify-center rounded-2xl bg-background ring-1 ring-foreground/10">
            <TicketXIcon className="size-8 text-destructive" />
          </div>
          <div className="flex flex-col gap-3">
            <Badge variant="destructive" className="self-start">
              Storniert
            </Badge>
            <p className="text-3xl font-semibold tracking-tight text-balance lg:text-4xl">
              {event?.title ?? "Veranstaltung nicht mehr verfügbar"}
            </p>
          </div>
          {event && (
            <dl className="grid gap-2.5 text-sm text-muted-foreground">
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
            </dl>
          )}
        </div>

        <p className="text-xs text-muted-foreground">
          Buchungsnummer{" "}
          <span className="font-mono break-all text-foreground">
            {booking.bookingId}
          </span>
        </p>
      </aside>

      <Perforation />

      <section
        aria-labelledby="cancellation-heading"
        className="flex flex-1 flex-col gap-10 px-6 py-8 md:px-10 md:py-12 lg:px-16"
      >
        <div className="flex flex-col gap-2">
          <p className="text-sm font-medium text-muted-foreground">
            Stornierung
          </p>
          <h1
            id="cancellation-heading"
            className="text-2xl font-semibold tracking-tight lg:text-3xl"
          >
            Deine Buchung wurde storniert
          </h1>
          <p className="max-w-prose text-muted-foreground">
            {refunded
              ? "Deine Tickets wurden freigegeben und der bezahlte Betrag erstattet."
              : "Deine Tickets wurden freigegeben."}
          </p>
        </div>

        <RefundSummary booking={booking} />

        <dl className="grid gap-3 sm:grid-cols-3">
          <Tile label="Stornierte Tickets">
            <span className="line-through decoration-destructive/60">
              {formatTicketCount(booking.numberOfTickets)}
            </span>
          </Tile>
          <Tile label="Gebucht am">{formatDateTime(booking.bookedAt)}</Tile>
          <Tile label="Zahlung">
            {booking.paymentStatus
              ? paymentStatusLabels[booking.paymentStatus]
              : "Nicht gestartet"}
          </Tile>
        </dl>

        <div className="mt-auto flex flex-col gap-2 border-t pt-6 sm:flex-row">
          <Button render={<Link href="/tickets" />} nativeButton={false}>
            Zu meinen Tickets
          </Button>
          {event?.status === "published" && (
            <Button
              render={<Link href={`/events/${event.eventId}`} />}
              nativeButton={false}
              variant="outline"
            >
              Veranstaltung erneut ansehen
            </Button>
          )}
        </div>
      </section>
    </div>
  )
}

function RefundSummary({ booking }: { booking: OwnBooking }) {
  if (booking.paymentStatus === "refunded") {
    return (
      <section
        aria-labelledby="refund-heading"
        className="flex flex-col gap-6 rounded-2xl bg-emerald-50 p-6 ring-1 ring-emerald-600/15 md:p-8 lg:flex-row lg:items-end lg:justify-between dark:bg-emerald-950/30 dark:ring-emerald-400/20"
      >
        <div className="flex flex-col gap-2">
          <h2
            id="refund-heading"
            className="flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-400"
          >
            <CircleCheckIcon className="size-4" />
            Erstattet
          </h2>
          <p className="text-4xl font-semibold tracking-tight tabular-nums lg:text-5xl">
            {formatAmount(booking.refundAmount)}
          </p>
          <p className="max-w-prose text-sm text-muted-foreground">
            Der Betrag wurde auf dein ursprüngliches Zahlungsmittel
            zurückerstattet.
          </p>
        </div>
        <dl className="grid gap-1.5 text-sm lg:min-w-56">
          <div className="flex items-baseline justify-between gap-6">
            <dt className="text-muted-foreground">Bezahlt</dt>
            <dd className="tabular-nums">{formatAmount(booking.amount)}</dd>
          </div>
          <div className="flex items-baseline justify-between gap-6">
            <dt className="text-muted-foreground">Erstattet</dt>
            <dd className="font-medium tabular-nums">
              {formatAmount(booking.refundAmount)}
            </dd>
          </div>
        </dl>
      </section>
    )
  }

  const nothingCharged =
    booking.paymentStatus === null || booking.paymentStatus === "failed"

  return (
    <section
      aria-labelledby="refund-heading"
      className="flex flex-col gap-2 rounded-2xl bg-muted/50 p-6 ring-1 ring-foreground/10 md:p-8"
    >
      <h2
        id="refund-heading"
        className="flex items-center gap-2 text-sm font-medium text-muted-foreground"
      >
        {nothingCharged && <CircleCheckIcon className="size-4" />}
        Erstattung
      </h2>
      <p className="text-2xl font-semibold tracking-tight">
        {nothingCharged
          ? "Nichts berechnet"
          : booking.paymentStatus
            ? paymentStatusLabels[booking.paymentStatus]
            : "—"}
      </p>
      {nothingCharged && (
        <p className="max-w-prose text-sm text-muted-foreground">
          Die Zahlung wurde nicht abgeschlossen, daher gibt es nichts zu
          erstatten.
        </p>
      )}
    </section>
  )
}

function Tile({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1 rounded-xl p-4 ring-1 ring-foreground/10">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="font-medium">{children}</dd>
    </div>
  )
}

/**
 * The tear line between stub and cancellation, with a notch bitten into
 * either edge: across the screen on narrow screens, down it on wide ones.
 */
function Perforation() {
  return (
    <div aria-hidden className="relative h-0 md:h-auto md:w-0">
      <div className="absolute inset-x-8 top-0 border-t-2 border-dashed border-foreground/15 md:inset-x-auto md:inset-y-8 md:left-0 md:-translate-x-1/2 md:border-t-0 md:border-l-2" />
      <div className="absolute top-0 -left-3 size-6 -translate-y-1/2 rounded-full bg-background md:-top-3 md:left-0 md:-translate-x-1/2 md:translate-y-0" />
      <div className="absolute top-0 -right-3 size-6 -translate-y-1/2 rounded-full bg-background md:top-auto md:right-auto md:-bottom-3 md:left-0 md:-translate-x-1/2 md:translate-y-0" />
    </div>
  )
}
