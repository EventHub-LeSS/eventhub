"use client"

import {
  ArrowLeftIcon,
  CircleXIcon,
  ClockIcon,
  LoaderCircleIcon,
  ShieldQuestionIcon,
} from "lucide-react"
import Link from "next/link"
import { useCallback, useEffect, useMemo, useState, useTransition } from "react"

import { OrderSummary } from "@/features/bookings/components/order-summary"
import {
  type FormPaymentMethod,
  PaymentMethods,
  useCardDetails,
  useSepaDetails,
} from "@/features/bookings/components/payment-methods"
import { PaymentResult } from "@/features/bookings/components/payment-result"
import {
  completePaymentAction,
  startPaymentAction,
} from "@/features/bookings/lib/actions"
import { readCheckoutEvent } from "@/features/bookings/lib/checkout-state"
import {
  formatAmount,
  formatDateTime,
  formatRemaining,
  formatTicketCount,
} from "@/features/bookings/lib/format"
import type {
  Payment,
  SimulatedPaymentOutcome,
} from "@/features/bookings/lib/types"
import { parseAmount } from "@/features/bookings/lib/validation"
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
 * EVENTHUB-235: the view that carries out the simulated payment process of a
 * booking. It starts the payment, shows the amount stored by the backend
 */
export function PaymentView({ bookingId }: { bookingId: string }) {
  const [payment, setPayment] = useState<Payment | null>(null)
  const [message, setMessage] = useState<string>()
  const [now, setNow] = useState(() => Date.now())
  const [isPending, startTransition] = useTransition()
  const [selectedMethod, setSelectedMethod] =
    useState<FormPaymentMethod>("card")
  const [method, setMethod] = useState<FormPaymentMethod | "paypal">()
  const card = useCardDetails()
  const sepa = useSepaDetails()

  const eventId = payment?.eventId ?? null
  const checkoutEvent = useMemo(
    () => (eventId ? readCheckoutEvent(eventId) : null),
    [eventId]
  )

  const start = useCallback(() => {
    startTransition(async () => {
      const result = await startPaymentAction(bookingId)

      if (result.payment) {
        setPayment(result.payment)
        setMessage(undefined)
      } else {
        setMessage(
          result.message ??
            "Der Zahlungsvorgang konnte nicht geladen werden. Bitte versuche es später erneut."
        )
      }
    })
  }, [bookingId])

  // The payment process starts as soon as the view opens: the backend stores
  // the amount at that point, so what is shown is what is charged.
  useEffect(() => {
    start()
  }, [start])

  // Ticks the remaining reservation time while the payment is open.
  useEffect(() => {
    if (payment?.status !== "pending") {
      return
    }

    const timer = setInterval(() => setNow(Date.now()), 1000)

    return () => clearInterval(timer)
  }, [payment?.status])

  const isPendingPayment = payment?.status === "pending"

  // AK4: what gets confirmed is the stored amount — read from the server's
  // response, not from anything the user could mistype. The backend rejects
  // any deviation from the amount it stored when the payment started (422).
  const storedAmount = isPendingPayment ? parseAmount(payment.amount) : null

  const expiresAt = payment?.expiresAt
  const deadline = expiresAt ? Date.parse(expiresAt) : null
  const deadlineKnown = deadline !== null && Number.isFinite(deadline)
  const remaining = deadlineKnown ? formatRemaining(expiresAt, now) : null
  const expired = deadlineKnown && deadline <= now

  const canComplete =
    isPendingPayment && storedAmount !== null && !expired && !isPending
  const selectedValid = selectedMethod === "card" ? card.valid : sepa.valid
  const canPayWithSelected = canComplete && selectedValid

  function complete(
    outcome: SimulatedPaymentOutcome,
    via: FormPaymentMethod | "paypal" = selectedMethod
  ) {
    if (!payment || storedAmount === null) {
      return
    }

    setMessage(undefined)
    setMethod(via)

    startTransition(async () => {
      const result = await completePaymentAction(
        bookingId,
        outcome,
        storedAmount
      )

      if (result.payment) {
        setPayment(result.payment)
      } else {
        setMessage(
          result.message ??
            "Die Zahlung konnte nicht abgeschlossen werden. Bitte versuche es erneut."
        )
      }
    })
  }

  // The payment process is being started; nothing to show yet.
  if (!payment) {
    if (isPending) {
      return (
        <Card className="max-w-xl">
          <CardContent className="flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircleIcon className="size-4 animate-spin" />
            Zahlungsvorgang wird geladen …
          </CardContent>
        </Card>
      )
    }

    return (
      <Card className="max-w-xl border-destructive/50">
        <CardContent className="flex flex-col gap-3">
          <div className="flex items-start gap-2">
            <CircleXIcon className="mt-0.5 size-4 shrink-0 text-destructive" />
            <div className="flex flex-col gap-1">
              <p className="text-sm font-medium">
                Der Zahlungsvorgang konnte nicht geladen werden.
              </p>
              {message && (
                <p className="text-sm text-muted-foreground">{message}</p>
              )}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={start} disabled={isPending}>
              Erneut versuchen
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

  // AK3: the stored result of the completed payment.
  if (!isPendingPayment) {
    return (
      <div className="max-w-xl">
        <PaymentResult payment={payment} />
      </div>
    )
  }

  return (
    <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
      <div className="lg:col-start-2 lg:row-start-1">
        <OrderSummary
          event={checkoutEvent}
          eventId={payment.eventId}
          numberOfTickets={payment.numberOfTickets}
        />
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
              <dt className="text-muted-foreground">Reservierte Tickets</dt>
              <dd className="font-medium">
                {formatTicketCount(payment.numberOfTickets)}
              </dd>
            </div>
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

          {/* AK1: the amount stored by the backend, shown before completion. */}
          <div className="flex flex-col gap-1 rounded-lg border border-border bg-muted/40 p-4">
            <span className="text-sm text-muted-foreground">
              Zu zahlender Betrag
            </span>
            <span className="text-3xl font-semibold tabular-nums">
              {formatAmount(payment.amount)}
            </span>
            <span className="text-xs text-muted-foreground">
              Berechnet aus {payment.numberOfTickets}{" "}
              {payment.numberOfTickets === 1 ? "Ticket" : "Tickets"} ×
              Veranstaltungspreis; der Betrag wird serverseitig gespeichert.
            </span>
          </div>

          {expired ? (
            <p className="text-sm text-destructive" role="alert">
              Die Reservierung ist abgelaufen. Die Zahlung kann nicht mehr
              abgeschlossen werden — bitte starte eine neue Buchung.
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
                <Button
                  variant="outline"
                  onClick={() => complete("failure")}
                  disabled={!canPayWithSelected}
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
            Nicht bezahlte Reservierungen laufen automatisch ab und die Tickets
            werden wieder freigegeben.
          </p>
        </CardContent>
      </Card>

      <div className="lg:col-start-2 lg:row-start-3">
        <Button
          render={
            <Link href={payment.eventId ? `/events/${payment.eventId}` : "/"} />
          }
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Zurück zur Veranstaltung
        </Button>
      </div>
    </div>
  )
}
