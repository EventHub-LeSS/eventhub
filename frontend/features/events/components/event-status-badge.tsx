import type { EventStatus } from "@/features/events/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"

type BadgeVariant = React.ComponentProps<typeof Badge>["variant"]

const statusPresentation: Record<
  EventStatus,
  { label: string; variant: BadgeVariant }
> = {
  draft: { label: "Entwurf", variant: "secondary" },
  published: { label: "Veröffentlicht", variant: "default" },
  // Organizers withdraw events; the backend stores that as "cancelled".
  cancelled: { label: "Zurückgezogen", variant: "destructive" },
  completed: { label: "Abgeschlossen", variant: "outline" },
}

const unknownStatus = { label: "Unbekannt", variant: "outline" } as const

export function EventStatusBadge({ status }: { status: EventStatus }) {
  // A status this UI does not know yet shows as a neutral badge, not an error.
  const { label, variant } = statusPresentation[status] ?? unknownStatus

  return <Badge variant={variant}>{label}</Badge>
}
