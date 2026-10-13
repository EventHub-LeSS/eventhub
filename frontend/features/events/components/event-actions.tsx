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

import { publishDraftAction } from "@/features/events/lib/actions"
import type { EventStatus } from "@/features/events/lib/types"
import { Button } from "@/features/shared/components/ui/button"
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
 * the drafts list's standalone button. "Withdraw" and "Edit" still only need
 * an onClick handler once their actions exist.
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
          <DropdownMenuItem>
            <UndoIcon />
            Withdraw
          </DropdownMenuItem>
          <DropdownMenuItem>
            <PencilIcon />
            Edit
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {error && (
        <p className="max-w-48 text-right text-xs text-destructive">{error}</p>
      )}
    </div>
  )
}
