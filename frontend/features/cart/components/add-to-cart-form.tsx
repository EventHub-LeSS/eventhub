"use client"

import { useQueryClient } from "@tanstack/react-query"
import { MinusIcon, PlusIcon, TicketIcon } from "lucide-react"
import Link from "next/link"
import { useCallback, useEffect, useState, useTransition } from "react"

import { addToCartAction } from "@/features/cart/lib/actions"
import type { CartActionResult } from "@/features/cart/lib/actions"
import {
  saveParkedAddToCart,
  takeParkedAddToCart,
} from "@/features/cart/lib/cart-state"
import { formatAmount, formatTicketCount } from "@/features/cart/lib/format"
import {
  cartItemOf,
  CART_QUERY_KEY,
  useCart,
} from "@/features/cart/lib/use-cart"
import {
  MAX_TICKETS_PER_EVENT,
  parseTicketCount,
  ticketCountError,
} from "@/features/cart/lib/validation"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

interface AddToCartFormProps {
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
 * The cart card of the event detail page: how many tickets go into the
 * cart. The amount shown is the line total of this addition, calculated
 * from ticket count and price here — the checkout then stores the amounts
 * it charges. Anonymous visitors park their input for the login roundtrip
 * and the cart is filled right after signing in.
 */
export function AddToCartForm({
  eventId,
  price,
  availableSeats,
  capacity,
  bookable,
  isAuthenticated,
}: AddToCartFormProps) {
  const queryClient = useQueryClient()
  const queryCart = useCart({ enabled: isAuthenticated })
  const [ticketCount, setTicketCount] = useState("1")
  const [message, setMessage] = useState<string>()
  const [notice, setNotice] = useState<string>()
  const [isPending, startTransition] = useTransition()

  // The item already in the cart holds its seats, so they count as room
  // for this addition (see ticketCountError).
  const quantityInCart =
    cartItemOf(queryCart.data, eventId)?.numberOfTickets ?? 0

  // A price that cannot be read must not produce a total (AK4): without a
  // valid amount nothing is reserved.
  const unitPrice = Number(price)
  const priceKnown = Number.isFinite(unitPrice) && unitPrice >= 0

  const count = parseTicketCount(ticketCount)
  const countError = ticketCountError(
    ticketCount,
    availableSeats,
    quantityInCart
  )

  // EVENTHUB-91 / AK3: the stepper stops at the remaining seats, not only
  // at the per-event limit.
  const maxTickets =
    availableSeats === null
      ? MAX_TICKETS_PER_EVENT - quantityInCart
      : Math.min(
          MAX_TICKETS_PER_EVENT - quantityInCart,
          availableSeats + quantityInCart
        )

  // The line total of this addition, shown before anything is reserved.
  const total =
    priceKnown && count !== null && count >= 1 ? unitPrice * count : null

  const canSubmit = priceKnown && !countError && bookable && !isPending

  const applyResult = useCallback(
    (result: CartActionResult) => {
      if (result.cart) {
        // The badge and the cart page read the same query, so the change is
        // everywhere at once, without a refetch.
        queryClient.setQueryData(CART_QUERY_KEY, result.cart)
        setTicketCount("1")
        setMessage(undefined)
        setNotice(
          "Die Tickets liegen im Warenkorb und sind 15 Minuten für dich reserviert."
        )
      } else {
        setNotice(undefined)
        setMessage(
          result.message ??
            "Die Tickets konnten nicht in den Warenkorb gelegt werden. Bitte versuche es erneut."
        )
      }
    },
    [queryClient]
  )

  // An addition parked for the login roundtrip is taken up here: signed-in
  // visitors continue straight with the reservation, anonymous ones at
  // least find their input restored. The entry is consumed, so it never
  // reserves again later.
  //
  // This effect exists to synchronize the form with the addition parked in
  // sessionStorage — an external system that is only readable in the
  // browser, so the values cannot be derived during render. It waits for
  // the cart of a just-signed-in visitor, so the parked addition is
  // checked against what is already in the cart.
  // oxlint-disable react/set-state-in-effect
  useEffect(() => {
    if (isAuthenticated && bookable && queryCart.isPending) {
      return
    }

    const parked = takeParkedAddToCart(eventId)

    if (!parked) {
      return
    }

    setTicketCount(parked.ticketCount)

    if (!isAuthenticated || !bookable) {
      return
    }

    const parkedCount = parseTicketCount(parked.ticketCount)

    if (
      parkedCount === null ||
      ticketCountError(parked.ticketCount, availableSeats, quantityInCart)
    ) {
      return
    }

    startTransition(async () => {
      applyResult(await addToCartAction(eventId, parkedCount))
    })
  }, [
    eventId,
    isAuthenticated,
    bookable,
    availableSeats,
    quantityInCart,
    queryCart.isPending,
    applyResult,
    startTransition,
  ])
  // oxlint-enable react/set-state-in-effect

  function updateCount(next: number) {
    setMessage(undefined)
    setNotice(undefined)
    setTicketCount(String(next))
  }

  function handleSubmit(submitEvent: React.FormEvent<HTMLFormElement>) {
    submitEvent.preventDefault()
    setMessage(undefined)
    setNotice(undefined)

    if (count === null || countError || !canSubmit) {
      return
    }

    if (!isAuthenticated) {
      // The reservation needs a login roundtrip that loses this form's
      // state; park it, so the cart is filled right after the login.
      saveParkedAddToCart({ eventId, ticketCount })
    }

    startTransition(async () => {
      applyResult(await addToCartAction(eventId, count))
    })
  }

  return (
    <Card className="gap-4 shadow-md">
      <CardHeader>
        <CardTitle className="text-xl font-bold">Tickets reservieren</CardTitle>
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
                    setNotice(undefined)
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
                Höchstens {MAX_TICKETS_PER_EVENT} Tickets pro Veranstaltung im
                Warenkorb.
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

          {quantityInCart > 0 && (
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-muted/40 p-3">
              <span className="text-sm text-muted-foreground">
                {formatTicketCount(quantityInCart)} für diese Veranstaltung im
                Warenkorb.
              </span>
              <Button
                render={<Link href="/cart" />}
                nativeButton={false}
                variant="outline"
                size="sm"
              >
                Zum Warenkorb
              </Button>
            </div>
          )}

          {!bookable && (
            <p className="text-sm text-muted-foreground">
              Diese Veranstaltung kann aktuell nicht gebucht werden.
            </p>
          )}

          {!isAuthenticated && (
            <p className="text-xs text-muted-foreground">
              Beim Absenden wirst du aufgefordert, dich anzumelden. Deine
              Eingaben bleiben erhalten und der Warenkorb wird nach dem Login
              direkt befüllt.
            </p>
          )}

          {notice && (
            <output className="text-sm text-emerald-600 dark:text-emerald-400">
              {notice}
            </output>
          )}

          {message && (
            <p className="text-sm text-destructive" role="alert">
              {message}
            </p>
          )}

          <Button type="submit" disabled={!canSubmit} className="w-full">
            <TicketIcon />
            {isPending ? "Wird hinzugefügt …" : "In den Warenkorb"}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
