"use client"

import { useQueryClient } from "@tanstack/react-query"
import {
  CalendarIcon,
  ClockIcon,
  LoaderCircleIcon,
  MapPinIcon,
  MinusIcon,
  PlusIcon,
  ShoppingCartIcon,
  TrashIcon,
} from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useEffect, useState, useTransition } from "react"

import {
  checkoutAction,
  clearCartAction,
  removeItemAction,
  updateQuantityAction,
} from "@/features/cart/lib/actions"
import type { CartActionResult } from "@/features/cart/lib/actions"
import {
  formatAmount,
  formatEventPeriod,
  formatLocation,
  formatRemaining,
  formatTicketCount,
} from "@/features/cart/lib/format"
import type { CartItem, OpenCartPayment } from "@/features/cart/lib/types"
import { CART_QUERY_KEY, useCart } from "@/features/cart/lib/use-cart"
import { MAX_TICKETS_PER_EVENT } from "@/features/cart/lib/validation"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Card, CardContent } from "@/features/shared/components/ui/card"

/**
 * The cart page: one card per reserved event with the live ticket count,
 * prices and the remaining reservation time, and the checkout that turns
 * everything into one payment. Every change writes the cart the backend
 * returns back into the cart query, so badge and page never drift apart.
 */
export function CartView() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const cart = useCart()
  const [message, setMessage] = useState<string>()
  const [isPending, startTransition] = useTransition()
  const [now, setNow] = useState(() => Date.now())

  const items = cart.data?.items ?? []
  const totalTickets = cart.data?.totalTickets ?? 0
  const checkoutReady = cart.data?.checkoutReady ?? false
  const openPayment = cart.data?.openPayment ?? null

  // Ticks the remaining reservation time while the cart holds items or an
  // open checkout keeps theirs.
  useEffect(() => {
    if (items.length === 0 && openPayment === null) {
      return
    }

    const timer = setInterval(() => setNow(Date.now()), 1000)

    return () => clearInterval(timer)
  }, [items.length, openPayment])

  // A reservation that ran out holds no seats; the backend drops it on the
  // next read. Until the refresh, the row shows it as expired.
  const anyExpired = items.some((item) => isExpired(item, now))

  function applyResult(result: CartActionResult) {
    if (result.cart) {
      queryClient.setQueryData(CART_QUERY_KEY, result.cart)
      setMessage(undefined)
    } else {
      setMessage(
        result.message ??
          "Der Warenkorb konnte nicht aktualisiert werden. Bitte versuche es erneut."
      )
      // Something changed server-side; resync the cart in the background.
      queryClient.invalidateQueries({ queryKey: CART_QUERY_KEY })
    }
  }

  function changeQuantity(item: CartItem, quantity: number) {
    setMessage(undefined)

    startTransition(async () => {
      applyResult(await updateQuantityAction(item.bookingId, quantity))
    })
  }

  function removeItem(item: CartItem) {
    setMessage(undefined)

    startTransition(async () => {
      applyResult(await removeItemAction(item.bookingId))
    })
  }

  function handleClear() {
    setMessage(undefined)

    startTransition(async () => {
      applyResult(await clearCartAction())
    })
  }

  function handleCheckout() {
    setMessage(undefined)

    startTransition(async () => {
      const result = await checkoutAction()

      if (result.payment) {
        // The items now belong to the payment; the badge must not keep
        // counting them.
        queryClient.invalidateQueries({ queryKey: CART_QUERY_KEY })
        router.push(`/cart/payment/${result.payment.paymentId}`)
      } else {
        setMessage(
          result.message ??
            "Der Checkout konnte nicht gestartet werden. Bitte versuche es erneut."
        )
        queryClient.invalidateQueries({ queryKey: CART_QUERY_KEY })
      }
    })
  }

  if (cart.isPending) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircleIcon className="size-4 animate-spin" />
          Warenkorb wird geladen …
        </CardContent>
      </Card>
    )
  }

  if (cart.isError) {
    return (
      <Card className="border-destructive/50">
        <CardContent className="flex flex-col gap-3">
          <p className="text-sm font-medium">
            Dein Warenkorb kann aktuell nicht geladen werden.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              onClick={() => cart.refetch()}
              disabled={cart.isFetching}
            >
              {cart.isFetching ? "Lädt …" : "Erneut versuchen"}
            </Button>
            <Button
              render={<Link href="/" />}
              nativeButton={false}
              size="sm"
              variant="outline"
            >
              Veranstaltungen entdecken
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  if (items.length === 0 && openPayment === null) {
    return (
      <EmptyCartCard hint="Tickets im Warenkorb bleiben 15 Minuten für dich reserviert." />
    )
  }

  return (
    <div className="flex flex-col gap-4">
      {openPayment && <OpenCheckoutCard payment={openPayment} now={now} />}

      {items.length === 0 ? (
        // The banner above already explains how new items relate to the
        // open checkout, so the card only states that none are waiting.
        <EmptyCartCard />
      ) : (
        <ul className="flex flex-col gap-3">
          {items.map((item) => (
            <li key={item.bookingId}>
              <CartRow
                item={item}
                now={now}
                disabled={isPending}
                onChange={changeQuantity}
                onRemove={removeItem}
              />
            </li>
          ))}
        </ul>
      )}

      {anyExpired && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-muted/40 p-3">
          <p className="text-sm text-muted-foreground">
            Eine Reservierung ist abgelaufen und hält keine Plätze mehr.
          </p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => cart.refetch()}
            disabled={cart.isFetching}
          >
            {cart.isFetching ? "Aktualisiert …" : "Warenkorb aktualisieren"}
          </Button>
        </div>
      )}

      {items.length > 0 && (
        <Card className="gap-4">
          <CardContent className="flex flex-col gap-4">
            <div className="flex flex-col gap-2 rounded-lg border border-border bg-muted/40 p-4">
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-sm font-semibold">Gesamt</span>
                <span className="text-2xl font-extrabold tabular-nums">
                  {formatAmount(cart.data?.totalAmount ?? null)}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                {formatTicketCount(totalTickets)} · Beträge der aktuellen
                Veranstaltungspreise
              </p>
            </div>

            {message && (
              <p className="text-sm text-destructive" role="alert">
                {message}
              </p>
            )}

            <div className="flex flex-wrap items-center gap-2">
              <Button
                onClick={handleCheckout}
                disabled={
                  !checkoutReady ||
                  anyExpired ||
                  isPending ||
                  openPayment !== null
                }
              >
                <ShoppingCartIcon />
                {isPending ? "Checkout läuft …" : "Zur Kasse"}
              </Button>
              <Button
                variant="outline"
                onClick={handleClear}
                disabled={items.length === 0 || isPending}
              >
                Warenkorb leeren
              </Button>
            </div>

            {openPayment ? (
              <p className="text-sm text-muted-foreground" role="note">
                Weitere Artikel können erst ausgecheckt werden, wenn die offene
                Zahlung abgeschlossen ist.
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                Alle Items im Warenkorb werden zusammen in einem
                Zahlungsvorgang ausgecheckt — oder gar nicht. Unbezahlt laufen
                die Reservierungen ab und die Plätze werden wieder freigegeben.
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}

function isExpired(item: CartItem, now: number): boolean {
  const deadline = Date.parse(item.expiresAt)

  return Number.isFinite(deadline) && deadline <= now
}

/**
 * The empty cart, as a quiet centered hint instead of item rows, like the
 * empty states of the event list and the notifications. The hint explains
 * what happens to tickets once they are added; it is omitted while a
 * checkout is open, because the banner above already explains how new
 * items relate to it.
 */
function EmptyCartCard({ hint }: { hint?: string }) {
  return (
    <Card className="border border-dashed border-border py-8 ring-0">
      <CardContent className="flex flex-col items-center gap-2 text-center">
        <ShoppingCartIcon className="size-6 text-muted-foreground" />
        <p className="text-sm font-medium text-foreground">
          Dein Warenkorb ist leer
        </p>
        {hint && <p className="text-sm text-muted-foreground">{hint}</p>}
        <Button
          className="mt-2"
          render={<Link href="/" />}
          nativeButton={false}
          variant="outline"
          size="sm"
        >
          Veranstaltungen entdecken
        </Button>
      </CardContent>
    </Card>
  )
}

/**
 * The banner of the caller's open checkout: its items left the cart with
 * the checkout and wait for their payment. The cart query reports the
 * checkout, so returning from the payment view does not look like lost
 * items.
 */
function OpenCheckoutCard({
  payment,
  now,
}: {
  payment: OpenCartPayment
  now: number
}) {
  const remaining = formatRemaining(payment.expiresAt, now)

  return (
    <Card className="border-primary/40">
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <p className="text-sm font-semibold">
            Dein Checkout wartet auf die Zahlung
          </p>
          {remaining && (
            <Badge variant="secondary" className="tabular-nums">
              <ClockIcon data-icon="inline-start" />
              {remaining}
            </Badge>
          )}
        </div>
        <p className="text-sm text-muted-foreground">
          {formatTicketCount(payment.totalTickets)} für{" "}
          {formatAmount(payment.amount)} sind bereits ausgecheckt und halten
          ihre Plätze, bis die Zahlung abgeschlossen ist oder die Reservierung
          abläuft.
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            render={<Link href={`/cart/payment/${payment.paymentId}`} />}
            nativeButton={false}
          >
            Zahlung abschließen
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          Artikel, die du jetzt in den Warenkorb legst, bleiben reserviert und
          können erst ausgecheckt werden, wenn diese Zahlung abgeschlossen ist.
        </p>
      </CardContent>
    </Card>
  )
}

/** One cart item: the reserved tickets of one event. */
function CartRow({
  item,
  now,
  disabled,
  onChange,
  onRemove,
}: {
  item: CartItem
  now: number
  disabled: boolean
  onChange: (item: CartItem, quantity: number) => void
  onRemove: (item: CartItem) => void
}) {
  const event = item.event

  // Seats free for others plus the seats this item holds: the item may
  // always keep its tickets, more need free seats.
  const maxTickets = Math.min(MAX_TICKETS_PER_EVENT, item.availableSeats)
  const remaining = formatRemaining(item.expiresAt, now)
  const expired = isExpired(item, now)

  return (
    <Card className="gap-3">
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          {event ? (
            <Link
              href={`/events/${event.eventId}`}
              className="font-medium underline-offset-4 hover:underline"
            >
              {event.title}
            </Link>
          ) : (
            <p className="font-medium">Veranstaltung nicht mehr verfügbar</p>
          )}
          {expired ? (
            <Badge variant="outline">Abgelaufen</Badge>
          ) : (
            remaining && (
              <Badge variant="secondary" className="tabular-nums">
                <ClockIcon data-icon="inline-start" />
                {remaining}
              </Badge>
            )
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
          </dl>
        )}

        {event && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <fieldset className="flex items-center rounded-full border border-border bg-muted focus-within:ring-2 focus-within:ring-ring/50">
              <legend className="sr-only">
                Ticketanzahl für {event.title}
              </legend>
              <button
                type="button"
                aria-label="Ein Ticket weniger"
                className="flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                disabled={disabled || item.numberOfTickets <= 1}
                onClick={() =>
                  onChange(item, Math.max(1, item.numberOfTickets - 1))
                }
              >
                <MinusIcon className="size-4" />
              </button>
              <span className="flex h-8 w-10 items-center justify-center text-sm font-medium tabular-nums">
                {item.numberOfTickets}
              </span>
              <button
                type="button"
                aria-label="Ein Ticket mehr"
                className="flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                disabled={disabled || item.numberOfTickets >= maxTickets}
                onClick={() =>
                  onChange(item, Math.min(maxTickets, item.numberOfTickets + 1))
                }
              >
                <PlusIcon className="size-4" />
              </button>
            </fieldset>

            <div className="flex flex-col items-end">
              <span className="text-xs text-muted-foreground">
                {formatAmount(item.unitPrice)} pro Ticket
              </span>
              <span className="text-lg font-extrabold tabular-nums">
                {formatAmount(item.lineTotal)}
              </span>
            </div>
          </div>
        )}

        {!item.bookable && (
          <p className="text-sm text-destructive" role="alert">
            Diese Veranstaltung ist nicht mehr verfügbar. Entferne das Item, um
            den Warenkorb auszuchecken.
          </p>
        )}

        <div className="flex justify-end">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onRemove(item)}
            disabled={disabled}
          >
            <TrashIcon />
            Entfernen
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
