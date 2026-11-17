import { Badge } from "@/features/shared/components/ui/badge"

/** The three states a ticket can be shown in. */
export type TicketKind = "valid" | "past" | "cancelled"

/** Status badge shared by the ticket card and the full screen ticket view. */
export function TicketStatusBadge({ kind }: { kind: TicketKind }) {
  if (kind === "cancelled") {
    return <Badge variant="destructive">Storniert</Badge>
  }

  if (kind === "past") {
    return <Badge variant="outline">Vergangen</Badge>
  }

  return (
    <Badge className="bg-emerald-600 text-white dark:bg-emerald-500 dark:text-black">
      Gültig
    </Badge>
  )
}
