import { ArrowRightIcon } from "lucide-react";
import Link from "next/link";

import type { CheckoutEvent } from "@/features/bookings/lib/checkout-state";
import {
  formatAmount,
  formatEventPeriod,
  formatTicketCount,
} from "@/features/bookings/lib/format";
import { Button } from "@/features/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card";

interface OrderSummaryProps {
  event: CheckoutEvent | null;
  eventId: string | null;
  numberOfTickets: number;
}

export function OrderSummary({
  event,
  eventId,
  numberOfTickets,
}: OrderSummaryProps) {
  return (
    <Card className="gap-4">
      <CardHeader>
        <CardTitle>Deine Bestellung</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {event ? (
          <>
            <div className="flex flex-col gap-1">
              <span className="text-xs font-bold tracking-wider text-purple-700 uppercase">
                {event.category}
              </span>
              <p className="text-lg font-bold break-words">{event.title}</p>
            </div>
            <dl className="grid gap-2 text-sm">
              <div className="flex flex-col gap-0.5">
                <dt className="font-semibold">Zeitpunkt</dt>
                <dd className="text-muted-foreground">
                  {formatEventPeriod(event.startTime, event.endTime)}
                </dd>
              </div>
              <div className="flex flex-col gap-0.5">
                <dt className="font-semibold">Ort</dt>
                <dd className="text-muted-foreground">{event.address}</dd>
              </div>
            </dl>
          </>
        ) : (
          <div className="flex flex-col items-start gap-2">
            <p className="text-sm text-muted-foreground">
              Die Details zur Veranstaltung sind hier gerade nicht verfügbar.
            </p>
            {eventId && (
              <Button
                render={<Link href={`/events/${eventId}`} />}
                nativeButton={false}
                variant="outline"
                size="sm"
              >
                Veranstaltung ansehen
                <ArrowRightIcon data-icon="inline-end" />
              </Button>
            )}
          </div>
        )}

        <hr className="border-border" />

        <div className="flex items-baseline justify-between gap-2 text-sm">
          <span className="text-muted-foreground">
            {formatTicketCount(numberOfTickets)}
          </span>
          {event && (
            <span className="font-medium tabular-nums">
              {numberOfTickets} × {formatAmount(event.price)}
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
