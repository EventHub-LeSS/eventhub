import {
  formatAmount,
  formatEventPeriod,
  formatLocation,
  formatTicketCount,
} from "@/features/cart/lib/format"
import type { PaymentItem } from "@/features/cart/lib/types"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

interface OrderSummaryProps {
  items: PaymentItem[]
}

/** What is being bought: every item of the cart checkout with its stored line amount. */
export function OrderSummary({ items }: OrderSummaryProps) {
  return (
    <Card className="gap-4">
      <CardHeader>
        <CardTitle>Deine Bestellung</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="flex flex-col divide-y divide-border">
          {items.map((item) => (
            <li
              key={item.bookingId}
              className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0"
            >
              <p className="text-lg font-bold break-words">
                {item.event?.title ?? "Veranstaltung nicht mehr verfügbar"}
              </p>
              {item.event && (
                <dl className="grid gap-2 text-sm">
                  <div className="flex flex-col gap-0.5">
                    <dt className="font-semibold">Zeitpunkt</dt>
                    <dd className="text-muted-foreground">
                      {formatEventPeriod(
                        item.event.startTime,
                        item.event.endTime
                      )}
                    </dd>
                  </div>
                  <div className="flex flex-col gap-0.5">
                    <dt className="font-semibold">Ort</dt>
                    <dd className="text-muted-foreground">
                      {formatLocation(item.event.location)}
                    </dd>
                  </div>
                </dl>
              )}
              <div className="flex items-baseline justify-between gap-2 text-sm">
                <span className="text-muted-foreground">
                  {formatTicketCount(item.numberOfTickets)}
                </span>
                <span className="font-medium tabular-nums">
                  {formatAmount(item.lineAmount)}
                </span>
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
