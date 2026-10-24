"use client"

import {
  EllipsisIcon,
  EyeIcon,
  PencilIcon,
  SendIcon,
  Trash2Icon,
  UndoIcon,
} from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useState, useTransition } from "react"

import type { OrganizationRole } from "@/features/auth"
import { DeleteDraftDialog } from "@/features/events/components/delete-draft-dialog"
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
  /** Roles in the event's organization; actions the API would refuse are left out. */
  roles: OrganizationRole[]
}

/**
 * "Details anzeigen" links to the detail page, "Publish" calls the same action as
 * the drafts list's standalone button. "Withdraw" asks for confirmation first,
 * because a withdrawn event cannot be published again, and so does "Delete" for
 * drafts, because a deleted draft is gone for good. "Edit" still only needs
 * an onClick handler once its action exists.
 */
export function EventActions({
  eventId,
  eventTitle,
  status,
  showViewDetails = true,
  roles,
}: EventActionsProps) {
  const router = useRouter()
  const [error, setError] = useState<string>()
  const [isPending, startTransition] = useTransition()
  const [isWithdrawOpen, setIsWithdrawOpen] = useState(false)
  const [withdrawError, setWithdrawError] = useState<string>()
  const [isWithdrawing, startWithdraw] = useTransition()
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)

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

  // Mirrors the API: event managers manage events, org admins may also delete
  // drafts. A finance viewer only gets "Details anzeigen".
  const canManage = roles.includes("event_manager")
  const canDelete =
    status === "draft" && (canManage || roles.includes("org_admin"))

  if (!showViewDetails && !canManage && !canDelete) {
    return null
  }

  return (
    <div className="flex flex-col items-end gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="ghost" size="icon-sm" />}
          aria-label={`Aktionen für ${eventTitle}`}
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
                Details anzeigen
              </DropdownMenuItem>
              {(canManage || canDelete) && <DropdownMenuSeparator />}
            </>
          )}
          {canManage && status === "draft" && (
            <DropdownMenuItem onClick={handlePublish}>
              <SendIcon />
              Veröffentlichen
            </DropdownMenuItem>
          )}
          {canManage && status === "published" && (
            <DropdownMenuItem onClick={() => handleWithdrawOpenChange(true)}>
              <UndoIcon />
              Zurückziehen
            </DropdownMenuItem>
          )}
          {canManage && (
            <DropdownMenuItem>
              <PencilIcon />
              Bearbeiten
            </DropdownMenuItem>
          )}
          {canDelete && (
            <>
              {canManage && <DropdownMenuSeparator />}
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setIsDeleteOpen(true)}
              >
                <Trash2Icon />
                Löschen
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {error && (
        <p className="max-w-48 text-right text-xs text-destructive">{error}</p>
      )}

      <Dialog open={isWithdrawOpen} onOpenChange={handleWithdrawOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Veranstaltung zurückziehen?</DialogTitle>
            <DialogDescription>
              &quot;{eventTitle}&quot; ist dann nicht mehr buchbar und
              verschwindet aus der Veranstaltungssuche. Bestehende Buchungen
              bleiben erhalten. Das kann nicht rückgängig gemacht werden.
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
              Abbrechen
            </Button>
            <Button
              variant="destructive"
              onClick={handleWithdraw}
              disabled={isWithdrawing}
            >
              {isWithdrawing
                ? "Wird zurückgezogen…"
                : "Veranstaltung zurückziehen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <DeleteDraftDialog
        eventId={eventId}
        eventTitle={eventTitle}
        open={isDeleteOpen}
        onOpenChange={setIsDeleteOpen}
      />
    </div>
  )
}
