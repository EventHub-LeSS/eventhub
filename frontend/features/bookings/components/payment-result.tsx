import { CircleAlertIcon, CircleCheckIcon, CircleXIcon } from "lucide-react"
import Link from "next/link"

import {
  bookingStatusLabels,
  failureReasonLabels,
  formatAmount,
  formatTicketCount,
  paymentStatusLabels,
} from "@/features/bookings/lib/format"
import type { Payment } from "@/features/bookings/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Card, CardContent } from "@/features/shared/components/ui/card"

/**
 * AK3: the clear result of a completed payment, straight from the stored
 * state. `payment.message` is the backend's own wording, so the result can
 * never contradict what was booked and charged.
 */
export function PaymentResult({ payment }: { payment: Payment }) {
  const success = payment.status === "paid" && payment.ticketsValid
  const failed = payment.status === "failed"
  const paidWithoutTickets = payment.status === "paid" && !payment.ticketsValid

  const heading = success
    ? "Zahlung erfolgreich"
    : paidWithoutTickets
      ? "Zahlung abgeschlossen"
      : failed
        ? "Zahlung fehlgeschlagen"
        : payment.status === "refunded"
          ? "Zahlung erstattet"
          : "Zahlungsvorgang"

  const eventHref = payment.eventId ? `/events/${payment.eventId}` : "/"

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
          <Badge variant={success ? "secondary" : "outline"}>
            Buchung: {bookingStatusLabels[payment.bookingStatus]}
          </Badge>
          {payment.ticketsValid && (
            <Badge className="bg-emerald-600 text-white dark:bg-emerald-500 dark:text-black">
              Tickets gültig
            </Badge>
          )}
        </div>

        <dl className="grid gap-2 text-sm">
          <div className="flex items-baseline justify-between gap-2">
            <dt className="text-muted-foreground">Bezahlter Betrag</dt>
            <dd className="font-medium tabular-nums">
              {formatAmount(payment.amount)}
            </dd>
          </div>
          <div className="flex items-baseline justify-between gap-2">
            <dt className="text-muted-foreground">Tickets</dt>
            <dd className="font-medium">
              {formatTicketCount(payment.numberOfTickets)}
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
            Für einen neuen Versuch ist eine neue Buchung nötig.
          </p>
        )}

        <div className="flex flex-wrap gap-2">
          {success ? (
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
              <Button render={<Link href={eventHref} />} nativeButton={false}>
                Erneut buchen
              </Button>
              <Button
                render={<Link href="/" />}
                nativeButton={false}
                variant="outline"
              >
                Veranstaltungen entdecken
              </Button>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
