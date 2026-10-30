"use client"

import { usePathname, useRouter } from "next/navigation"
import { useState, useTransition } from "react"

import { deleteDraftAction } from "@/features/events/lib/actions"
import { Button } from "@/features/shared/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"
import { toast } from "@/features/shared/components/ui/toast"

const DRAFTS_PATH = "/organizer/events"

interface DeleteDraftDialogProps {
  eventId: string
  eventTitle: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * EVENTHUB-256: asks before a draft is deleted, because that cannot be undone.
 * The drafts lists drop the event through the action's revalidation; on the
 * detail page there is nothing left to show, so it goes back to the list.
 */
export function DeleteDraftDialog({
  eventId,
  eventTitle,
  open,
  onOpenChange,
}: DeleteDraftDialogProps) {
  const router = useRouter()
  const pathname = usePathname()
  const [isDeleting, startDelete] = useTransition()

  function handleOpenChange(nextOpen: boolean) {
    if (isDeleting) return

    onOpenChange(nextOpen)
  }

  function handleDelete() {
    if (isDeleting) return

    startDelete(async () => {
      const result = await deleteDraftAction(eventId)

      onOpenChange(false)

      // A toast instead of a message in the dialog: a draft published in the
      // meantime leaves the drafts list on the refresh, and this dialog with it.
      if (result.message) {
        toast.add({
          title: "Entwurf konnte nicht gelöscht werden",
          description: result.message,
        })
        router.refresh()
        return
      }

      toast.add({
        type: "success",
        title: "Entwurf gelöscht",
        description: `"${eventTitle}" wurde gelöscht.`,
      })

      if (pathname !== DRAFTS_PATH) {
        router.replace(DRAFTS_PATH)
      }
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Entwurf löschen?</DialogTitle>
          <DialogDescription>
            &quot;{eventTitle}&quot; wird endgültig gelöscht. Das kann nicht
            rückgängig gemacht werden.
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isDeleting}
          >
            Abbrechen
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={isDeleting}
          >
            {isDeleting ? "Wird gelöscht…" : "Entwurf löschen"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** The drafts list's standalone counterpart of the "Delete" menu entry. */
export function DeleteDraftButton({
  eventId,
  eventTitle,
}: {
  eventId: string
  eventTitle: string
}) {
  const [isOpen, setIsOpen] = useState(false)

  return (
    <>
      <Button variant="destructive" size="sm" onClick={() => setIsOpen(true)}>
        Löschen
      </Button>
      <DeleteDraftDialog
        eventId={eventId}
        eventTitle={eventTitle}
        open={isOpen}
        onOpenChange={setIsOpen}
      />
    </>
  )
}
