import type { EventStatus } from "@/features/events/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"

type BadgeVariant = React.ComponentProps<typeof Badge>["variant"]

const statusPresentation: Record<
  EventStatus,
  { label: string; variant: BadgeVariant }
> = {
  draft: { label: "Draft", variant: "secondary" },
  published: { label: "Published", variant: "default" },
  cancelled: { label: "Cancelled", variant: "destructive" },
  completed: { label: "Completed", variant: "outline" },
}

const unknownStatus = { label: "Unknown", variant: "outline" } as const

export function EventStatusBadge({ status }: { status: EventStatus }) {
  // A status this UI does not know yet shows as a neutral badge, not an error.
  const { label, variant } = statusPresentation[status] ?? unknownStatus

  return <Badge variant={variant}>{label}</Badge>
}
