import { CalendarIcon, MapPinIcon } from "lucide-react"

import {
  formatEventDate,
  formatEventPrice,
} from "@/features/catalog/lib/format"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

interface EventCardProps {
  event: PublishedEvent
}

export function EventCard({ event }: EventCardProps) {
  return (
    <Card className="gap-0 overflow-hidden py-0">
      <div className="flex h-32 items-center justify-center bg-muted">
        <span className="font-mono text-xs text-muted-foreground">
          {event.category.category}
        </span>
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
          {formatEventDate(event.startTime)}
        </span>
        <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <MapPinIcon className="size-3.5 shrink-0" />
          {event.location.name}, {event.location.city}
        </span>
        <span className="pt-2 text-sm font-semibold">
          {formatEventPrice(event.price)}
        </span>
      </CardContent>
    </Card>
  )
}
