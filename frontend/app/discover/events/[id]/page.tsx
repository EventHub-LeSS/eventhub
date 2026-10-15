import { ArrowLeftIcon, TicketIcon } from "lucide-react"
import Link from "next/link"
import { notFound } from "next/navigation"

import { getPublishedEvent } from "@/features/catalog"
import { EventDetails } from "@/features/events"
import { Button } from "@/features/shared/components/ui/button"

export default async function DiscoverEventPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = await params
  const result = await getPublishedEvent(id)
  if (result.status === "not-found") notFound()

  return (
    <div className="flex flex-1 justify-center p-6">
      <div className="flex w-full max-w-xl flex-col gap-6">
        <div>
          <Button
            render={<Link href="/" />}
            nativeButton={false}
            variant="ghost"
            size="sm"
          >
            <ArrowLeftIcon /> All events
          </Button>
        </div>
        {result.status === "ok" ? (
          <EventDetails
            event={{
              ...result.event,
              soldTickets: { status: "ok", count: result.event.soldTickets },
            }}
          >
            <div className="flex flex-col gap-2 rounded-xl border border-border p-4">
              {result.event.availability === "almost_sold_out" && (
                <p className="text-sm font-medium">Almost sold out</p>
              )}
              <Button disabled aria-describedby="ticket-availability">
                <TicketIcon /> Buy ticket
              </Button>
              <p
                id="ticket-availability"
                className="text-sm text-muted-foreground"
              >
                {result.event.availability === "sold_out"
                  ? "This event is sold out."
                  : result.event.availability === "temporarily_unavailable"
                    ? "All remaining seats are currently reserved. Please try again later."
                    : !result.event.bookable
                      ? "This event is not available for booking."
                      : "Online ticket purchasing will be available soon."}
              </p>
            </div>
          </EventDetails>
        ) : (
          <p className="text-sm text-destructive" role="alert">
            This event can&apos;t be loaded right now. Please try again later.
          </p>
        )}
      </div>
    </div>
  )
}
