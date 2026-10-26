"use client"

import { MinusIcon, PlusIcon, TicketIcon } from "lucide-react"
import { useEffect, useState, useTransition } from "react"

import { createBookingAction } from "@/features/bookings/lib/actions"
import {
  savePendingCheckout,
  takePendingCheckout,
} from "@/features/bookings/lib/checkout-state"
import { formatAmount } from "@/features/bookings/lib/format"
import {
  cardCvcError,
  cardExpiryError,
  cardNumberError,
  cardholderError,
  formatCardExpiry,
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
  /** Total capacity, only shown as context next to the remaining seats. */
  capacity: number
  /** False when the event is sold out or live reservations block booking. */
  bookable: boolean
  isAuthenticated: boolean
}

/**
 * AK1: the checkout card of the event detail page. The amount to pay is
 * calculated from ticket count and price here, before anything is reserved.
 * The card data belongs to the simulated payment — it is validated in the
 * frontend (AK4) but never leaves the browser, and completing the payment
 * happens in the payment view the booking redirects to. Anonymous
 * visitors park their checkout for the login roundtrip and continue with
 * the booking straight after signing in.
 */
export function BookingForm({
  eventId,
  price,
  availableSeats,
  capacity,
  bookable,
  isAuthenticated,
}: BookingFormProps) {
  const [ticketCount, setTicketCount] = useState("1")
  const [cardholder, setCardholder] = useState("")
  const [cardNumber, setCardNumber] = useState("")
  const [cardExpiry, setCardExpiry] = useState("")
  const [cardCvc, setCardCvc] = useState("")
  const [message, setMessage] = useState<string>()
  const [isPending, startTransition] = useTransition()

  // A price that cannot be read must not produce a total (AK4): without a
  // valid amount nothing is reserved.
  const unitPrice = Number(price)
  const priceKnown = Number.isFinite(unitPrice) && unitPrice >= 0

  const count = parseTicketCount(ticketCount)
  const countError = ticketCountError(ticketCount, availableSeats)

  // EVENTHUB-91 / AK3: the stepper stops at the remaining seats, not only at
  // the per-booking limit.
  const maxTickets =
    availableSeats === null
      ? MAX_TICKETS_PER_BOOKING
      : Math.min(MAX_TICKETS_PER_BOOKING, availableSeats)

  // AK1: the amount to pay, calculated from ticket count and price, shown
  // before anything is reserved. The payment view then shows the amount the
  // backend actually stored.
  const total =
    priceKnown && count !== null && count >= 1 ? unitPrice * count : null

  // AK4: the simulated card data is validated live, before anything is sent.
  // Untouched fields stay silent; the submit button just stays disabled.
  const holderError = cardholderError(cardholder)
  const numberError = cardNumberError(cardNumber)
  const expiryError = cardExpiryError(cardExpiry)
  const cvcError = cardCvcError(cardCvc)
  const cardFilled =
    cardholder.trim() !== "" &&
    cardNumber.trim() !== "" &&
    cardExpiry.trim() !== "" &&
    cardCvc.trim() !== ""

  const canSubmit =
    priceKnown &&
    !countError &&
    cardFilled &&
    !holderError &&
    !numberError &&
    !expiryError &&
    !cvcError &&
    bookable &&
    !isPending

  // A checkout parked for the login roundtrip is taken up here: signed-in
  // visitors continue straight with the booking, anonymous ones at least
  // find their input restored. The entry is consumed, so it never books
  // again later.
  //
  // This effect exists to synchronize the form with the checkout parked in
  // sessionStorage — an external system that is only readable in the
  // browser, so the values cannot be derived during render.
  // oxlint-disable react/set-state-in-effect
  useEffect(() => {
    const stored = takePendingCheckout(eventId)

    if (!stored) {
      return
    }

    setTicketCount(stored.ticketCount)
    setCardholder(stored.cardholder)
    setCardNumber(stored.cardNumber)
    setCardExpiry(stored.cardExpiry)
    setCardCvc(stored.cardCvc)

    if (!isAuthenticated || !bookable) {
      return
    }

    const storedCount = parseTicketCount(stored.ticketCount)

    if (
      storedCount === null ||
      ticketCountError(stored.ticketCount, availableSeats)
    ) {
      return
    }

    startTransition(async () => {
      // On success the action redirects to the payment view, so a result
      // only arrives when reserving failed.
      const result = await createBookingAction(eventId, storedCount)
      setMessage(result.message)
    })
  }, [eventId, isAuthenticated, bookable, availableSeats, startTransition])
  // oxlint-enable react/set-state-in-effect

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

    if (!isAuthenticated) {
      // The booking needs a login roundtrip that loses this form's state;
      // park it, so the checkout continues right after the login.
      savePendingCheckout({
        eventId,
        ticketCount,
        cardholder,
        cardNumber,
        cardExpiry,
        cardCvc,
      })
    }

    startTransition(async () => {
      // On success the action redirects to the payment view, so a result
      // only arrives when reserving failed.
      const result = await createBookingAction(eventId, count)
      setMessage(result.message)
    })
  }

  return (
    <Card className="gap-4 shadow-md">
      <CardHeader>
        <CardTitle className="text-xl font-bold">Tickets buchen</CardTitle>
        <CardDescription>
          {availableSeats === null
            ? "Die Platzanzeige ist derzeit nicht verfügbar."
            : `${availableSeats} von ${capacity} Plätzen frei`}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between gap-3">
              <label htmlFor="ticket-count" className="text-sm font-medium">
                Anzahl Tickets
              </label>
              <div className="flex items-center rounded-full border border-border bg-muted focus-within:ring-2 focus-within:ring-ring/50">
                <button
                  type="button"
                  aria-label="Ein Ticket weniger"
                  className="flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                  disabled={isPending || (count ?? 1) <= 1}
                  onClick={() => updateCount(Math.max(1, (count ?? 1) - 1))}
                >
                  <MinusIcon className="size-4" />
                </button>
                <input
                  id="ticket-count"
                  inputMode="numeric"
                  value={ticketCount}
                  onChange={(changed) => {
                    setMessage(undefined)
                    setTicketCount(changed.target.value)
                  }}
                  aria-invalid={Boolean(countError)}
                  disabled={isPending}
                  className="h-8 w-10 bg-transparent text-center text-sm font-medium tabular-nums outline-none disabled:cursor-not-allowed disabled:opacity-50"
                />
                <button
                  type="button"
                  aria-label="Ein Ticket mehr"
                  className="flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                  disabled={isPending || (count ?? 1) >= maxTickets}
                  onClick={() =>
                    updateCount(Math.min(maxTickets, (count ?? 1) + 1))
                  }
                >
                  <PlusIcon className="size-4" />
                </button>
              </div>
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

          <div className="flex flex-col gap-2 rounded-lg border border-border bg-muted/40 p-4">
            <div className="flex items-baseline justify-between gap-2">
              <span className="text-sm font-semibold">Gesamt</span>
              <span className="text-2xl font-extrabold tabular-nums">
                {total !== null ? formatAmount(total) : "—"}
              </span>
            </div>
            {total !== null && (
              <p className="text-xs text-muted-foreground">
                {count} × {formatAmount(price)}
              </p>
            )}
          </div>

          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Input
                id="cardholder"
                aria-label="Karteninhaber"
                placeholder="Karteninhaber"
                autoComplete="cc-name"
                value={cardholder}
                onChange={(changed) => {
                  setMessage(undefined)
                  setCardholder(changed.target.value)
                }}
                aria-invalid={Boolean(holderError)}
                disabled={isPending}
                className="bg-muted"
              />
              {holderError ? (
                <p className="text-xs text-destructive" role="alert">
                  {holderError}
                </p>
              ) : null}
            </div>
            <div className="flex flex-col gap-1.5">
              <Input
                id="card-number"
                aria-label="Kartennummer"
                placeholder="Kartennummer"
                inputMode="numeric"
                autoComplete="cc-number"
                value={cardNumber}
                onChange={(changed) => {
                  setMessage(undefined)
                  setCardNumber(changed.target.value)
                }}
                aria-invalid={Boolean(numberError)}
                disabled={isPending}
                className="bg-muted"
              />
              {numberError ? (
                <p className="text-xs text-destructive" role="alert">
                  {numberError}
                </p>
              ) : null}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="flex flex-col gap-1.5">
                <Input
                  id="card-expiry"
                  aria-label="Ablaufdatum (MM/JJ)"
                  placeholder="MM/JJ"
                  autoComplete="cc-exp"
                  value={cardExpiry}
                  onChange={(changed) => {
                    setMessage(undefined)
                    setCardExpiry(formatCardExpiry(changed.target.value))
                  }}
                  aria-invalid={Boolean(expiryError)}
                  disabled={isPending}
                  className="bg-muted"
                />
                {expiryError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {expiryError}
                  </p>
                ) : null}
              </div>
              <div className="flex flex-col gap-1.5">
                <Input
                  id="card-cvc"
                  aria-label="CVC"
                  placeholder="CVC"
                  inputMode="numeric"
                  autoComplete="cc-csc"
                  value={cardCvc}
                  onChange={(changed) => {
                    setMessage(undefined)
                    setCardCvc(changed.target.value)
                  }}
                  aria-invalid={Boolean(cvcError)}
                  disabled={isPending}
                  className="bg-muted"
                />
                {cvcError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {cvcError}
                  </p>
                ) : null}
              </div>
            </div>
          </div>

          {!bookable && (
            <p className="text-sm text-muted-foreground">
              Diese Veranstaltung kann aktuell nicht gebucht werden.
            </p>
          )}

          {!isAuthenticated && (
            <p className="text-xs text-muted-foreground">
              Beim Absenden wirst du aufgefordert, dich anzumelden. Deine
              Eingaben bleiben erhalten und die Buchung geht nach dem Login
              direkt weiter.
            </p>
          )}

          {message && (
            <p className="text-sm text-destructive" role="alert">
              {message}
            </p>
          )}

          <Button type="submit" disabled={!canSubmit} className="w-full">
            <TicketIcon />
            {isPending ? "Checkout läuft …" : "Zum Checkout"}
          </Button>
          <p className="text-center text-xs text-muted-foreground">
            Simulierte Zahlung – es erfolgt keine echte Abbuchung.
          </p>
        </form>
      </CardContent>
    </Card>
  )
}
