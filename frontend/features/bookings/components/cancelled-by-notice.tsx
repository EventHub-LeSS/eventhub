import { TriangleAlertIcon, Undo2Icon } from "lucide-react"

import type { CancelledBy } from "@/features/bookings/lib/types"

/**
 * Tells who cancelled paid tickets. A cancellation by the organizer stands
 * out, because the visitor did not ask for it and the refund is still open.
 */
export function CancelledByNotice({
  cancelledBy,
}: {
  cancelledBy: CancelledBy
}) {
  if (cancelledBy === "organizer") {
    return (
      <div className="flex items-start gap-2 rounded-md bg-destructive/10 p-3 text-sm">
        <TriangleAlertIcon
          aria-hidden
          className="mt-0.5 size-4 shrink-0 text-destructive"
        />
        <p>
          <span className="font-medium text-destructive">
            Vom Veranstalter abgesagt.
          </span>{" "}
          <span className="text-muted-foreground">
            Der Veranstalter meldet sich bei dir wegen der Erstattung.
          </span>
        </p>
      </div>
    )
  }

  return (
    <div className="flex items-start gap-2 rounded-md bg-muted p-3 text-sm">
      <Undo2Icon
        aria-hidden
        className="mt-0.5 size-4 shrink-0 text-muted-foreground"
      />
      <p className="font-medium">Von dir storniert.</p>
    </div>
  )
}
