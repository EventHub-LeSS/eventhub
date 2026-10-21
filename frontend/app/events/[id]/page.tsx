import {
  ArrowLeftIcon,
  CalendarIcon,
  MapPinIcon,
  UsersIcon,
} from "lucide-react"
import Link from "next/link"
import { notFound } from "next/navigation"

import { getCurrentUser } from "@/features/auth"
import { BookingForm } from "@/features/bookings"
import {
  type EventAvailability,
  getPublishedEventDetails,
} from "@/features/catalog"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

interface PageProps {
  params: Promise<{ id: string }>
}

const availabilityLabels: Record<EventAvailability, string> = {
  available: "Verfügbar",
  almost_sold_out: "Fast ausverkauft",
  sold_out: "Ausverkauft",
  temporarily_unavailable: "Aktuell nicht buchbar",
}

const availabilityBadgeVariants: Record<
  EventAvailability,
  "secondary" | "destructive" | "outline"
> = {
  available: "secondary",
  almost_sold_out: "outline",
  sold_out: "destructive",
  temporarily_unavailable: "outline",
}

const dateFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "full",
  timeStyle: "short",
  timeZone: "Europe/Berlin",
})

/**
 * Public detail page of a published event (EVENTHUB-211/212) and the entry
 * point of the booking flow (EVENTHUB-235): the amount to pay is calculated
 * from the ticket count and the price here, before anything is reserved.
 */
export default async function EventDetailPage({ params }: PageProps) {
  const { id } = await params
  const [result, user] = await Promise.all([
    getPublishedEventDetails(id),
    getCurrentUser(),
  ])

  // Unpublished, draft or unknown events, and events missing category or
  // location, all answer 404; URLs must not reach them.
  if (result.status === "not-found") {
    notFound()
  }

  if (result.status === "unavailable") {
    return (
      <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-4 p-6">
        <Button
          render={<Link href="/" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Veranstaltungen entdecken
        </Button>
        <p className="text-sm text-muted-foreground">
          Diese Veranstaltung kann aktuell nicht geladen werden. Bitte versuche
          es später erneut.
        </p>
      </div>
    )
  }

  const { event } = result
  const { location, category } = event
  const street = [location.street, location.houseNumber]
    .filter(Boolean)
    .join(" ")
  const address = [
    [location.name, street].filter(Boolean).join(", "),
    `${location.postalCode} ${location.city}`,
  ].join(", ")

  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div>
        <Button
          render={<Link href="/" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Veranstaltungen entdecken
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="flex min-w-0 flex-col gap-6">
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="secondary" className="uppercase">
                {category.category}
              </Badge>
              <Badge variant={availabilityBadgeVariants[event.availability]}>
                {availabilityLabels[event.availability]}
              </Badge>
            </div>
            <h1 className="text-2xl font-semibold tracking-tight break-words">
              {event.title}
            </h1>
            {event.description && (
              <p className="text-sm text-muted-foreground">
                {event.description}
              </p>
            )}
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Details</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
                <CalendarIcon className="size-3.5 shrink-0" />
                {dateFormat.format(new Date(event.startTime))} –{" "}
                {dateFormat.format(new Date(event.endTime))}
              </span>
              <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
                <MapPinIcon className="size-3.5 shrink-0" />
                {address}
              </span>
              <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
                <UsersIcon className="size-3.5 shrink-0" />
                {event.availableSeats} von {event.capacity} Plätzen verfügbar
              </span>
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-col gap-2">
          <BookingForm
            eventId={event.eventId}
            price={event.price}
            availableSeats={event.availableSeats}
            bookable={event.bookable}
            isAuthenticated={Boolean(user)}
          />
        </div>
      </div>
    </div>
  )
}
