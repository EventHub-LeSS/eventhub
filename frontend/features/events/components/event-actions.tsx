"use client"

import {
  EllipsisIcon,
  EyeIcon,
  PencilIcon,
  RotateCcwIcon,
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
  restoreDraftAction,
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
  withdrawnByModerator: boolean
  /** The detail page is the link target itself, so it leaves that entry out. */
  showViewDetails?: boolean
  /** Roles in the event's organization; actions the API would refuse are left out. */
  roles: OrganizationRole[]
}

/**
 * "Details anzeigen" links to the detail page, "Publish" calls the same action as
 * the drafts list's standalone button. "Withdraw" asks for confirmation first,
 * because ticket holders are notified, and so does "Delete" for drafts,
 * because a deleted draft is gone for good. A withdrawn event is edited by
 * restoring it as a draft first, or published again right away (EVENTHUB-288).
 */
export function EventActions({
  eventId,
  eventTitle,
  status,
  withdrawnByModerator,
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

  function handleStatusAction(action: typeof publishDraftAction) {
    if (isPending) return

    setError(undefined)
    startTransition(async () => {
      const result = await action(eventId)

      if (result.message) {
        setError(result.message)
        return
      }

      // The action only revalidates the drafts list; this component also
      // renders on the event detail page, which needs its own refresh.
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
  // The API refuses to edit withdrawn events; they are restored as a draft
  // first. Events withdrawn by moderation can't be restored or republished.
  const canEdit = canManage && status !== "cancelled"
  const canReuse = canManage && status === "cancelled" && !withdrawnByModerator
  const canDelete =
    status === "draft" && (canManage || roles.includes("org_admin"))

  if (!showViewDetails && !canEdit && !canReuse && !canDelete) {
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
        <DropdownMenuContent
          align="end"
          className="w-max min-w-48 whitespace-nowrap"
        >
          {showViewDetails && (
            <>
              <DropdownMenuItem
                render={<Link href={`/organizer/events/${eventId}`} />}
              >
                <EyeIcon />
                Details anzeigen
              </DropdownMenuItem>
              {(canEdit || canReuse || canDelete) && <DropdownMenuSeparator />}
            </>
          )}
          {canManage && status === "draft" && (
            <DropdownMenuItem
              onClick={() => handleStatusAction(publishDraftAction)}
            >
              <SendIcon />
              Veröffentlichen
            </DropdownMenuItem>
          )}
          {canReuse && (
            <>
              <DropdownMenuItem
                onClick={() => handleStatusAction(restoreDraftAction)}
              >
                <RotateCcwIcon />
                Als Entwurf wiederherstellen
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => handleStatusAction(publishDraftAction)}
              >
                <SendIcon />
                Erneut veröffentlichen
              </DropdownMenuItem>
            </>
          )}
          {canManage && status === "published" && (
            <DropdownMenuItem onClick={() => handleWithdrawOpenChange(true)}>
              <UndoIcon />
              Zurückziehen
            </DropdownMenuItem>
          )}
          {canEdit && (
            <DropdownMenuItem
              disabled={status === "completed"}
              className={
                status === "completed" ? "text-muted-foreground" : undefined
              }
              render={
                status !== "completed" ? (
                  <Link href={`/organizer/events/${eventId}/edit`} />
                ) : undefined
              }
            >
              <PencilIcon />
              {status === "completed"
                ? "Abgeschlossen – nicht bearbeitbar"
                : "Bearbeiten"}
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
              bleiben erhalten. Alle Ticketinhaber werden benachrichtigt. Du
              kannst sie später als Entwurf wiederherstellen oder erneut
              veröffentlichen.
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
