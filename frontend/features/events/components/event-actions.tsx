"use client"

import {
  EllipsisIcon,
  EyeIcon,
  PencilIcon,
  SendIcon,
  UndoIcon,
} from "lucide-react"
import Link from "next/link"

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
  /** The detail page is the link target itself, so it leaves that entry out. */
  showViewDetails?: boolean
}

/**
 * "View details" links to the detail page. "Publish", "Withdraw" and "Edit"
 * only need an onClick handler once their actions exist.
 */
export function EventActions({
  eventId,
  eventTitle,
  showViewDetails = true,
}: EventActionsProps) {
  return (
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
        <DropdownMenuItem>
          <SendIcon />
          Publish
        </DropdownMenuItem>
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
  )
}
