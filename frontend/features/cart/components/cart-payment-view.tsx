"use client"

import { useQueryClient } from "@tanstack/react-query"
import {
  CircleXIcon,
  ClockIcon,
  LoaderCircleIcon,
  ShieldQuestionIcon,
} from "lucide-react"
import Link from "next/link"
import { useEffect, useState, useTransition } from "react"

import { PaymentResult } from "@/features/cart/components/payment-result"
import { completePaymentAction } from "@/features/cart/lib/actions"
import {
  formatAmount,
  formatDateTime,
  formatRemaining,
  formatTicketCount,
} from "@/features/cart/lib/format"
import type { SimulatedPaymentOutcome } from "@/features/cart/lib/types"
import {
  CART_QUERY_KEY,
  paymentQueryKey,
  useCart,
  usePayment,
} from "@/features/cart/lib/use-cart"
import {
  cardCvcError,
  cardExpiryError,
  cardNumberError,
  cardholderError,
  formatCardExpiry,
  parseAmount,
} from "@/features/cart/lib/validation"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { Input } from "@/features/shared/components/ui/input"

/**
 * EVENTHUB-235: the view that carries out the simulated payment of a whole
 * cart checkout. It shows the line amounts and the total amount stored by
 * the backend before anything is completed, lets the visitor pick the
 * simulated outcome and then shows the stored result. The card data
 * belongs to the simulated payment — it is validated in the frontend (AK4)
 * but never leaves the browser.
 */
export function CartPaymentView({ paymentId }: { paymentId: string }) {
  const queryClient = useQueryClient()
  const paymentQuery = usePayment(paymentId)
  // The cart keeps the items added after this checkout; they are reported
  // here because they cannot join this payment.
  const cartQuery = useCart()
  const [message, setMessage] = useState<string>()
  const [cardholder, setCardholder] = useState("")
  const [cardNumber, setCardNumber] = useState("")
  const [cardExpiry, setCardExpiry] = useState("")
  const [cardCvc, setCardCvc] = useState("")
  const [now, setNow] = useState(() => Date.now())
  const [isPending, startTransition] = useTransition()

  const payment = paymentQuery.data
  const isPendingPayment = payment?.status === "pending"

  // Ticks the remaining reservation time while the payment is open.
  useEffect(() => {
    if (!isPendingPayment) {
      return
    }

    const timer = setInterval(() => setNow(Date.now()), 1000)

    return () => clearInterval(timer)
  }, [isPendingPayment])

  // The payment process is being loaded; nothing to show yet.
  if (paymentQuery.isPending) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircleIcon className="size-4 animate-spin" />
          Zahlungsvorgang wird geladen …
        </CardContent>
      </Card>
    )
  }

  if (paymentQuery.isError || !payment) {
    return (
      <Card className="border-destructive/50">
        <CardContent className="flex flex-col gap-3">
          <div className="flex items-start gap-2">
            <CircleXIcon className="mt-0.5 size-4 shrink-0 text-destructive" />
            <p className="text-sm font-medium">
              Der Zahlungsvorgang konnte nicht geladen werden.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              onClick={() => paymentQuery.refetch()}
              disabled={paymentQuery.isFetching}
            >
              {paymentQuery.isFetching ? "Lädt …" : "Erneut versuchen"}
            </Button>
            <Button
              render={<Link href="/cart" />}
              nativeButton={false}
              size="sm"
              variant="outline"
            >
              Zum Warenkorb
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  // AK3: the stored result of the completed payment.
  if (!isPendingPayment) {
    return <PaymentResult payment={payment} />
  }

  // AK4: what gets confirmed is the stored amount — read from the server's
  // response, not from anything the user could mistype. The backend rejects
  // any deviation from the amount it stored at checkout (422).
  const storedAmount = parseAmount(payment.amount)

  const expiresAt = payment.expiresAt
  const deadline = expiresAt ? Date.parse(expiresAt) : null
  const deadlineKnown = deadline !== null && Number.isFinite(deadline)
  const remaining = deadlineKnown ? formatRemaining(expiresAt, now) : null
  const expired = deadlineKnown && deadline <= now

  // AK4: the simulated card data is validated live, before anything is
  // paid. Untouched fields stay silent; the buttons just stay disabled.
  const holderError = cardholderError(cardholder)
  const numberError = cardNumberError(cardNumber)
  const expiryError = cardExpiryError(cardExpiry)
  const cvcError = cardCvcError(cardCvc)
  const cardFilled =
    cardholder.trim() !== "" &&
    cardNumber.trim() !== "" &&
    cardExpiry.trim() !== "" &&
    cardCvc.trim() !== ""

  const canComplete =
    storedAmount !== null &&
    !expired &&
    cardFilled &&
    !holderError &&
    !numberError &&
    !expiryError &&
    !cvcError &&
    !isPending

  function complete(outcome: SimulatedPaymentOutcome) {
    if (storedAmount === null) {
      return
    }

    setMessage(undefined)

    startTransition(async () => {
      const result = await completePaymentAction(
        paymentId,
        outcome,
        storedAmount
      )

      if (result.payment) {
        // The stored result replaces the open payment; the cart emptied
        // into it (or the items were released after a failure), so the
        // badge must not keep counting them.
        queryClient.setQueryData(paymentQueryKey(paymentId), result.payment)
        queryClient.invalidateQueries({ queryKey: CART_QUERY_KEY })
      } else {
        setMessage(
          result.message ??
            "Die Zahlung konnte nicht abgeschlossen werden. Bitte versuche es erneut."
        )
      }
    })
  }

  // AK2: the view that carries out and simulates the payment process.
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          Zahlungsvorgang
          <Badge variant="secondary">
            <ShieldQuestionIcon data-icon="inline-start" />
            Simulation
          </Badge>
        </CardTitle>
        <CardDescription>
          Kein echter Zahlungsanbieter ist angebunden. Du bestimmst das Ergebnis
          der simulierten Zahlung selbst.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <ul className="flex flex-col gap-2">
          {payment.items.map((item) => (
            <li
              key={item.bookingId}
              className="flex flex-wrap items-start justify-between gap-2 text-sm"
            >
              <div className="flex flex-col">
                <span className="font-medium">
                  {item.event?.title ?? "Veranstaltung nicht mehr verfügbar"}
                </span>
                <span className="text-xs text-muted-foreground">
                  {formatTicketCount(item.numberOfTickets)}
                </span>
              </div>
              <span className="font-medium tabular-nums">
                {formatAmount(item.lineAmount)}
              </span>
            </li>
          ))}
        </ul>

        {cartQuery.data && cartQuery.data.items.length > 0 && (
          <p className="text-sm text-muted-foreground" role="note">
            Dein Warenkorb enthält weitere Artikel (
            {formatTicketCount(cartQuery.data.totalTickets)}). Sie bleiben
            reserviert und können erst ausgecheckt werden, wenn diese Zahlung
            abgeschlossen ist.
          </p>
        )}

        {/* AK1: the amount stored by the backend, shown before completion. */}
        <div className="flex flex-col gap-1 rounded-lg border border-border bg-muted/40 p-4">
          <span className="text-sm text-muted-foreground">
            Zu zahlender Betrag
          </span>
          <span className="text-3xl font-semibold tabular-nums">
            {formatAmount(payment.amount)}
          </span>
          <span className="text-xs text-muted-foreground">
            Summe der Zeilenbeträge aller Items; der Betrag wird serverseitig
            gespeichert.
          </span>
        </div>

        <dl className="grid gap-2 text-sm">
          <div className="flex items-baseline justify-between gap-2">
            <dt className="text-muted-foreground">Reservierung gültig bis</dt>
            <dd className="flex items-center gap-1.5 font-medium">
              {remaining && !expired ? (
                <span className="flex items-center gap-1 tabular-nums">
                  <ClockIcon className="size-3.5 text-muted-foreground" />
                  {remaining}
                </span>
              ) : (
                <span className="tabular-nums">
                  {formatDateTime(payment.expiresAt)}
                </span>
              )}
            </dd>
          </div>
        </dl>

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

        {expired ? (
          <p className="text-sm text-destructive" role="alert">
            Die Reservierung ist abgelaufen. Die Zahlung kann nicht mehr
            abgeschlossen werden — bitte lege die Tickets erneut in den
            Warenkorb.
          </p>
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => complete("success")} disabled={!canComplete}>
              {isPending ? "Verarbeite …" : "Zahlung erfolgreich abschließen"}
            </Button>
            <Button
              variant="outline"
              onClick={() => complete("failure")}
              disabled={!canComplete}
            >
              Zahlung ablehnen (Simulation)
            </Button>
          </div>
        )}

        {message && (
          <p className="text-sm text-destructive" role="alert">
            {message}
          </p>
        )}

        <p className="text-xs text-muted-foreground">
          Alle Items werden zusammen abgeschlossen — oder gar nicht. Nicht
          bezahlte Reservierungen laufen automatisch ab und die Tickets werden
          wieder freigegeben.
        </p>
      </CardContent>
    </Card>
  )
}
