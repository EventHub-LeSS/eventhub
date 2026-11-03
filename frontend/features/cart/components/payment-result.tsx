import { CircleAlertIcon, CircleCheckIcon, CircleXIcon } from "lucide-react"
import Link from "next/link"

import {
  bookingStatusLabels,
  failureReasonLabels,
  formatAmount,
  formatTicketCount,
  paymentStatusLabels,
} from "@/features/cart/lib/format"
import type { Payment } from "@/features/cart/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Card, CardContent } from "@/features/shared/components/ui/card"

/**
 * AK3: the clear result of a completed cart payment, straight from the
 * stored state. `payment.message` is the backend's own wording, so the
 * result can never contradict what was booked and charged — for every
 * single item.
 */
export function PaymentResult({ payment }: { payment: Payment }) {
  const success = payment.status === "paid" && payment.ticketsValid
  const failed = payment.status === "failed"
  const paidWithoutTickets = payment.status === "paid" && !payment.ticketsValid
  const refunded = payment.status === "refunded"

  const heading = success
    ? "Zahlung erfolgreich"
    : paidWithoutTickets
      ? "Zahlung abgeschlossen"
      : failed
        ? "Zahlung fehlgeschlagen"
        : refunded
          ? "Zahlung erstattet"
          : "Zahlungsvorgang"

  const showsTickets = success || refunded

  return (
    <Card data-testid="payment-result" className="gap-4">
      <CardContent className="flex flex-col gap-4">
        {/* The result of the payment process, announced politely (the output
            tag carries the status role). */}
        <output className="flex flex-col gap-2 sm:flex-row sm:items-start">
          {success ? (
            <CircleCheckIcon className="size-10 shrink-0 text-emerald-600 dark:text-emerald-400" />
          ) : paidWithoutTickets ? (
            <CircleAlertIcon className="size-10 shrink-0 text-amber-600 dark:text-amber-400" />
          ) : (
            <CircleXIcon className="size-10 shrink-0 text-destructive" />
          )}
          <div className="flex flex-col gap-1">
            <p className="text-lg font-medium" data-success={success}>
              {heading}
            </p>
            <p className="text-sm text-muted-foreground">{payment.message}</p>
          </div>
        </output>

        <div className="flex flex-wrap gap-2">
          <Badge variant={success ? "secondary" : "outline"}>
            Zahlung: {paymentStatusLabels[payment.status]}
          </Badge>
          {payment.ticketsValid && (
            <Badge className="bg-emerald-600 text-white dark:bg-emerald-500 dark:text-black">
              Tickets gültig
            </Badge>
          )}
        </div>

        <ul className="flex flex-col gap-2">
          {payment.items.map((item) => (
            <li
              key={item.bookingId}
              className="flex flex-wrap items-start justify-between gap-2 border-b border-border pb-2 text-sm last:border-b-0 last:pb-0"
            >
              <div className="flex flex-col">
                <span className="font-medium">
                  {item.event?.title ?? "Veranstaltung nicht mehr verfügbar"}
                </span>
                <span className="text-xs text-muted-foreground">
                  {formatTicketCount(item.numberOfTickets)} · Buchung:{" "}
                  {bookingStatusLabels[item.bookingStatus]}
                </span>
              </div>
              <span className="font-medium tabular-nums">
                {formatAmount(item.lineAmount)}
              </span>
            </li>
          ))}
        </ul>

        <dl className="grid gap-2 text-sm">
          <div className="flex items-baseline justify-between gap-2">
            <dt className="text-muted-foreground">Gesamtbetrag</dt>
            <dd className="font-medium tabular-nums">
              {formatAmount(payment.amount)}
            </dd>
          </div>
          {payment.failureReason && (
            <div className="flex items-baseline justify-between gap-2">
              <dt className="text-muted-foreground">Grund</dt>
              <dd className="font-medium">
                {failureReasonLabels[payment.failureReason] ??
                  payment.failureReason}
              </dd>
            </div>
          )}
        </dl>

        {failed && (
          <p className="text-xs text-muted-foreground">
            Für einen neuen Versuch lege die Tickets einfach erneut in den
            Warenkorb.
          </p>
        )}

        <div className="flex flex-wrap gap-2">
          {showsTickets ? (
            <>
              <Button render={<Link href="/tickets" />} nativeButton={false}>
                Zu meinen Tickets
              </Button>
              <Button
                render={<Link href="/" />}
                nativeButton={false}
                variant="outline"
              >
                Weitere Veranstaltungen entdecken
              </Button>
            </>
          ) : (
            <>
              <Button render={<Link href="/" />} nativeButton={false}>
                Veranstaltungen entdecken
              </Button>
              <Button
                render={<Link href="/cart" />}
                nativeButton={false}
                variant="outline"
              >
                Zum Warenkorb
              </Button>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
