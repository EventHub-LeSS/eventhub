"use client"

import {
  EllipsisIcon,
  EyeIcon,
  PencilIcon,
  SendIcon,
  UndoIcon,
} from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useState, useTransition } from "react"

import {
  publishDraftAction,
  withdrawEventAction,
} from "@/features/events/lib/actions"
import type { EventStatus } from "@/features/events/lib/types"
import { Button } from "@/features/shared/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/features/shared/components/ui/dropdown-menu"

interface EventActionsProps {
  eventId: string
  /** Only used to tell the menu buttons of a list apart for screen readers. */
  eventTitle: string
  status: EventStatus
  /** The detail page is the link target itself, so it leaves that entry out. */
  showViewDetails?: boolean
}

/**
 * "View details" links to the detail page, "Publish" calls the same action as
 * the drafts list's standalone button. "Withdraw" asks for confirmation first,
 * because a withdrawn event cannot be published again. "Edit" still only needs
 * an onClick handler once its action exists.
 */
export function EventActions({
  eventId,
  eventTitle,
  status,
  showViewDetails = true,
}: EventActionsProps) {
  const router = useRouter()
  const [error, setError] = useState<string>()
  const [isPending, startTransition] = useTransition()
  const [isWithdrawOpen, setIsWithdrawOpen] = useState(false)
  const [withdrawError, setWithdrawError] = useState<string>()
  const [isWithdrawing, startWithdraw] = useTransition()

  function handlePublish() {
    if (isPending) return

    setError(undefined)
    startTransition(async () => {
      const result = await publishDraftAction(eventId)

      if (result.message) {
        setError(result.message)
        return
      }

      // publishDraftAction only revalidates the drafts list; this component
      // also renders on the event detail page, which needs its own refresh.
      router.refresh()
    })
  }

  function handleWithdrawOpenChange(open: boolean) {
    if (isWithdrawing) return

    setWithdrawError(undefined)
    setIsWithdrawOpen(open)
  }

  function handleWithdraw() {
    if (isWithdrawing) return

    setWithdrawError(undefined)
    startWithdraw(async () => {
      const result = await withdrawEventAction(eventId)

      if (result.message) {
        setWithdrawError(result.message)
        return
      }

      setIsWithdrawOpen(false)
      router.refresh()
    })
  }

  return (
    <div className="flex flex-col items-end gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="ghost" size="icon-sm" />}
          aria-label={`Actions for ${eventTitle}`}
        >
          <EllipsisIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          {showViewDetails && (
            <>
              <DropdownMenuItem
                render={<Link href={`/organizer/events/${eventId}`} />}
              >
                <EyeIcon />
                View details
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          {status === "draft" && (
            <DropdownMenuItem onClick={handlePublish}>
              <SendIcon />
              Publish
            </DropdownMenuItem>
          )}
          {status === "published" && (
            <DropdownMenuItem onClick={() => handleWithdrawOpenChange(true)}>
              <UndoIcon />
              Withdraw
            </DropdownMenuItem>
          )}
          <DropdownMenuItem>
            <PencilIcon />
            Edit
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {error && (
        <p className="max-w-48 text-right text-xs text-destructive">{error}</p>
      )}

      <Dialog open={isWithdrawOpen} onOpenChange={handleWithdrawOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Withdraw event?</DialogTitle>
            <DialogDescription>
              &quot;{eventTitle}&quot; will no longer be bookable and disappears
              from the event search. Existing bookings are kept. This cannot be
              undone.
            </DialogDescription>
          </DialogHeader>

          {withdrawError && (
            <p className="text-sm text-destructive">{withdrawError}</p>
          )}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => handleWithdrawOpenChange(false)}
              disabled={isWithdrawing}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleWithdraw}
              disabled={isWithdrawing}
            >
              {isWithdrawing ? "Withdrawing…" : "Withdraw event"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
