"use client"

import { MinusIcon, PlusIcon, TicketIcon } from "lucide-react"
import { useState, useTransition } from "react"

import { createBookingAction } from "@/features/bookings/lib/actions"
import { formatAmount } from "@/features/bookings/lib/format"
import {
  MAX_TICKETS_PER_BOOKING,
  parseTicketCount,
  ticketCountError,
} from "@/features/bookings/lib/validation"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { Input } from "@/features/shared/components/ui/input"

interface BookingFormProps {
  eventId: string
  /** Decimal string as the API returns it, e.g. "20.00"; "0" is a free event. */
  price: string
  /** Snapshot from the event details; the backend counts capacity itself when reserving. */
  availableSeats: number | null
  /** False when the event is sold out or live reservations block booking. */
  bookable: boolean
  isAuthenticated: boolean
}

export function BookingForm({
  eventId,
  price,
  availableSeats,
  bookable,
  isAuthenticated,
}: BookingFormProps) {
  const [ticketCount, setTicketCount] = useState("1")
  const [message, setMessage] = useState<string>()
  const [isPending, startTransition] = useTransition()

  // A price that cannot be read must not produce a total (AK4): without a
  // valid amount nothing is reserved.
  const unitPrice = Number(price)
  const priceKnown = Number.isFinite(unitPrice) && unitPrice >= 0

  const count = parseTicketCount(ticketCount)
  const countError = ticketCountError(ticketCount, availableSeats)

  // AK1: the amount to pay, calculated from ticket count and price, shown
  // before anything is reserved. The payment view then shows the amount the
  // backend actually stored.
  const total =
    priceKnown && count !== null && count >= 1 ? unitPrice * count : null

  const canSubmit = priceKnown && !countError && bookable && !isPending

  function updateCount(next: number) {
    setMessage(undefined)
    setTicketCount(String(next))
  }

  function handleSubmit(submitEvent: React.FormEvent<HTMLFormElement>) {
    submitEvent.preventDefault()
    setMessage(undefined)

    if (count === null || countError || !canSubmit) {
      return
    }

    startTransition(async () => {
      // On success the action redirects to the payment view, so a result
      // only arrives when reserving failed.
      const result = await createBookingAction(eventId, count)
      setMessage(result.message)
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tickets buchen</CardTitle>
        <CardDescription>
          Der zu zahlende Betrag wird vor dem Abschluss angezeigt und von der
          API gespeichert.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="ticket-count" className="text-sm font-medium">
              Ticketanzahl
            </label>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label="Ein Ticket weniger"
                disabled={isPending || (count ?? 1) <= 1}
                onClick={() => updateCount(Math.max(1, (count ?? 1) - 1))}
              >
                <MinusIcon />
              </Button>
              <Input
                id="ticket-count"
                className="w-20 text-center"
                inputMode="numeric"
                value={ticketCount}
                onChange={(changed) => {
                  setMessage(undefined)
                  setTicketCount(changed.target.value)
                }}
                aria-invalid={Boolean(countError)}
                disabled={isPending}
              />
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label="Ein Ticket mehr"
                disabled={isPending || (count ?? 1) >= MAX_TICKETS_PER_BOOKING}
                onClick={() =>
                  updateCount(
                    Math.min(MAX_TICKETS_PER_BOOKING, (count ?? 1) + 1)
                  )
                }
              >
                <PlusIcon />
              </Button>
            </div>
            {countError ? (
              <p className="text-xs text-destructive" role="alert">
                {countError}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                Höchstens {MAX_TICKETS_PER_BOOKING} Tickets pro Buchung.
              </p>
            )}
          </div>

          <div className="flex flex-col gap-1 rounded-lg border border-border bg-muted/40 p-4">
            <div className="flex items-baseline justify-between gap-2">
              <span className="text-sm text-muted-foreground">
                Preis pro Ticket
              </span>
              <span className="text-sm font-medium tabular-nums">
                {priceKnown ? formatAmount(price) : "—"}
              </span>
            </div>
            <div className="flex items-baseline justify-between gap-2">
              <span className="text-sm text-muted-foreground">
                Zu zahlender Betrag
              </span>
              <span className="text-2xl font-semibold tabular-nums">
                {total !== null ? formatAmount(total) : "—"}
              </span>
            </div>
            {total !== null && (
              <p className="text-xs text-muted-foreground">
                {count} × {formatAmount(price)}
              </p>
            )}
          </div>

          {!bookable && (
            <p className="text-sm text-muted-foreground">
              Diese Veranstaltung kann aktuell nicht gebucht werden.
            </p>
          )}

          {!isAuthenticated && (
            <p className="text-xs text-muted-foreground">
              Beim Absenden wirst du aufgefordert, dich anzumelden, und kehrst
              anschließend hierher zurück.
            </p>
          )}

          {message && (
            <p className="text-sm text-destructive" role="alert">
              {message}
            </p>
          )}

          <Button type="submit" disabled={!canSubmit}>
            <TicketIcon />
            {isPending ? "Reserviere …" : "Tickets reservieren & zur Kasse"}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
