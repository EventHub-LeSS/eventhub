import { CalendarIcon, MapPinIcon } from "lucide-react"
import Link from "next/link"

import {
  formatEventDate,
  formatEventPrice,
} from "@/features/catalog/lib/format"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { EventImage } from "@/features/catalog/components/event-image"
import { Badge } from "@/features/shared/components/ui/badge"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { formatPlace } from "@/features/shared/lib/location"

interface EventCardProps {
  event: PublishedEvent
}

export function EventCard({ event }: EventCardProps) {
  return (
    <Link
      href={`/events/${event.eventId}`}
      className="group block h-full rounded-xl focus-visible:outline-none"
    >
      <Card className="h-full gap-0 overflow-hidden py-0 ring-1 ring-foreground/10 transition-all group-hover:ring-foreground/30 group-focus-visible:ring-2 group-focus-visible:ring-ring">
        <div className="relative flex h-32 items-center justify-center bg-muted">
          <span className="font-mono text-xs text-muted-foreground">
            {event.category.category}
          </span>
          <EventImage
            eventId={event.eventId}
            sizes="(min-width: 1280px) 400px, (min-width: 1024px) 33vw, (min-width: 640px) 50vw, 100vw"
          />
        </div>
        <CardHeader className="gap-2 pt-4 pb-0">
          <Badge variant="secondary" className="w-fit uppercase">
            {event.category.category}
          </Badge>
          <CardTitle>{event.title}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-1 pt-2 pb-4">
          <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <CalendarIcon className="size-3.5 shrink-0" />
            {formatEventDate(event.startTime, event.endTime)}
          </span>
          <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <MapPinIcon className="size-3.5 shrink-0" />
            {formatPlace(event.location)}
          </span>
          <span className="pt-2 text-sm font-semibold">
            {formatEventPrice(event.price)}
          </span>
        </CardContent>
      </Card>
    </Link>
  )
}
