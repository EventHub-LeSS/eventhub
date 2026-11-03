"use client"

import { useQueryClient } from "@tanstack/react-query"
import {
  ArrowLeftIcon,
  CircleXIcon,
  ClockIcon,
  LoaderCircleIcon,
  ShieldQuestionIcon,
} from "lucide-react"
import Link from "next/link"
import { useEffect, useState, useTransition } from "react"

import { OrderSummary } from "@/features/cart/components/order-summary"
import {
  type FormPaymentMethod,
  PaymentMethods,
  useCardDetails,
  useSepaDetails,
} from "@/features/cart/components/payment-methods"
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
import { parseAmount } from "@/features/cart/lib/validation"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

/**
 * EVENTHUB-235: the view that carries out the simulated payment of a whole
 * cart checkout. It shows the line amounts and the total amount stored by
 * the backend before anything is completed, lets the visitor pick a payment
 * method (card, SEPA direct debit or PayPal Express) and the simulated
 * outcome, and then shows the stored result. Card and account data belong
 * to the simulated payment — they are validated in the frontend (AK4) but
 * never leave the browser.
 */
export function CartPaymentView({ paymentId }: { paymentId: string }) {
  const queryClient = useQueryClient()
  const paymentQuery = usePayment(paymentId)
  // The cart keeps the items added after this checkout; they are reported
  // here because they cannot join this payment.
  const cartQuery = useCart()
  const [message, setMessage] = useState<string>()
  const [now, setNow] = useState(() => Date.now())
  const [isPending, startTransition] = useTransition()
  const [selectedMethod, setSelectedMethod] =
    useState<FormPaymentMethod>("card")
  const [method, setMethod] = useState<FormPaymentMethod | "paypal">()
  const card = useCardDetails()
  const sepa = useSepaDetails()

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
      <Card className="max-w-xl">
        <CardContent className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircleIcon className="size-4 animate-spin" />
          Zahlungsvorgang wird geladen …
        </CardContent>
      </Card>
    )
  }

  if (paymentQuery.isError || !payment) {
    return (
      <Card className="max-w-xl border-destructive/50">
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
    return (
      <div className="max-w-xl">
        <PaymentResult payment={payment} />
      </div>
    )
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

  const canComplete = storedAmount !== null && !expired && !isPending
  const selectedValid = selectedMethod === "card" ? card.valid : sepa.valid
  const canPayWithSelected = canComplete && selectedValid

  function complete(
    outcome: SimulatedPaymentOutcome,
    via: FormPaymentMethod | "paypal" = selectedMethod
  ) {
    if (storedAmount === null) {
      return
    }

    setMessage(undefined)
    setMethod(via)

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
    <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
      <div className="lg:col-start-2 lg:row-start-1">
        <OrderSummary items={payment.items} />
      </div>

      <div className="lg:col-start-1 lg:row-span-3 lg:row-start-1">
        <PaymentMethods
          method={selectedMethod}
          onMethodChange={setSelectedMethod}
          card={card}
          sepa={sepa}
          disabled={!canComplete}
          onPayPal={() => complete("success", "paypal")}
          payPalPending={isPending && method === "paypal"}
        />
      </div>

      <Card className="lg:col-start-2 lg:row-start-2">
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            Zahlungsvorgang
            <Badge variant="secondary">
              <ShieldQuestionIcon data-icon="inline-start" />
              Simulation
            </Badge>
          </CardTitle>
          <CardDescription>
            Kein echter Zahlungsanbieter ist angebunden. Bei Karte und
            Lastschrift bestimmst du das Ergebnis der simulierten Zahlung
            selbst.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
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

          {expired ? (
            <p className="text-sm text-destructive" role="alert">
              Die Reservierung ist abgelaufen. Die Zahlung kann nicht mehr
              abgeschlossen werden — bitte lege die Tickets erneut in den
              Warenkorb.
            </p>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  onClick={() => complete("success")}
                  disabled={!canPayWithSelected}
                >
                  {isPending && method !== "paypal"
                    ? "Verarbeite …"
                    : selectedMethod === "card"
                      ? "Mit Karte bezahlen"
                      : "Per Lastschrift bezahlen"}
                </Button>
                {/* Declining needs no card or account data. */}
                <Button
                  variant="outline"
                  onClick={() => complete("failure")}
                  disabled={!canComplete}
                >
                  Zahlung ablehnen (Simulation)
                </Button>
              </div>
              {!selectedValid && (
                <p className="text-xs text-muted-foreground">
                  {selectedMethod === "card"
                    ? "Gib deine Kartendaten ein, um mit Karte zu bezahlen"
                    : "Gib Kontoinhaber und IBAN ein, um per Lastschrift zu bezahlen"}{" "}
                  — oder nutze PayPal Express.
                </p>
              )}
            </>
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

      <div className="lg:col-start-2 lg:row-start-3">
        <Button
          render={<Link href="/cart" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Zurück zum Warenkorb
        </Button>
      </div>
    </div>
  )
}
