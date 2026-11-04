"use client"

import { TriangleAlertIcon } from "lucide-react"
import Link from "next/link"
import { useRef, useState } from "react"

import {
  useMarkNotificationRead,
  useNotifications,
} from "@/features/notifications/lib/api"
import type { Notification } from "@/features/notifications/lib/types"
import { Button } from "@/features/shared/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"

/** The API builds the subject as "Veranstaltung abgesagt: <title>". */
const SUBJECT_PREFIX = "Veranstaltung abgesagt: "

function eventTitle(notification: Notification) {
  return notification.subject.startsWith(SUBJECT_PREFIX)
    ? notification.subject.slice(SUBJECT_PREFIX.length)
    : notification.subject
}

/**
 * EVENTHUB-291: shows unread cancellations one at a time, oldest first. Only
 * "Verstanden" marks the shown cancellation as read. Closing the dialog any
 * other way, including "Zu meinen Tickets", pauses all cancellations until the
 * page is loaded again.
 */
export function CancellationNoticeDialog() {
  const { data } = useNotifications()
  const markRead = useMarkNotificationRead()
  const [paused, setPaused] = useState(false)
  const [confirmed, setConfirmed] = useState(0)
  const [shown, setShown] = useState<Notification>()
  const confirmRef = useRef<HTMLButtonElement>(null)

  const pending = (data?.items ?? [])
    .filter(
      (item) => item.occasion === "event_cancelled" && item.readAt === null
    )
    .toSorted((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt))
  const current = pending.at(0)

  // Keeps the last cancellation on screen while the dialog fades out.
  if (current && current.notificationId !== shown?.notificationId) {
    setShown(current)
  }
  if (!current && confirmed !== 0) {
    setConfirmed(0)
  }

  const notice = current ?? shown
  const total = confirmed + pending.length

  function confirm() {
    if (!current) return
    markRead.mutate(current.notificationId, {
      onSuccess: () => setConfirmed((count) => count + 1),
    })
  }

  return (
    <Dialog
      open={!paused && current !== undefined}
      onOpenChange={(open) => {
        if (!open) setPaused(true)
      }}
    >
      <DialogContent
        className="gap-6 p-6 sm:max-w-lg"
        initialFocus={confirmRef}
      >
        {notice && (
          <DialogHeader className="items-center gap-3 text-center">
            <div className="flex size-16 items-center justify-center rounded-full bg-destructive/10 text-destructive">
              <TriangleAlertIcon className="size-8" />
            </div>
            {total > 1 && (
              <p className="text-xs text-muted-foreground tabular-nums">
                Absage {Math.min(confirmed + 1, total)} von {total}
              </p>
            )}
            <DialogTitle className="text-sm font-semibold tracking-wide text-destructive uppercase">
              Veranstaltung abgesagt
            </DialogTitle>
            <p className="font-heading text-2xl leading-tight font-semibold text-balance">
              {eventTitle(notice)}
            </p>
            <DialogDescription className="text-base text-pretty">
              {notice.content}
            </DialogDescription>
          </DialogHeader>
        )}

        {markRead.isError && (
          <p role="alert" className="text-center text-sm text-destructive">
            {markRead.error.message}
          </p>
        )}

        <DialogFooter className="-mx-6 -mb-6 px-6 py-4 sm:justify-center">
          <Button
            variant="outline"
            size="lg"
            className="min-h-11 sm:min-w-40"
            render={<Link href="/tickets" />}
            nativeButton={false}
            onClick={() => setPaused(true)}
          >
            Zu meinen Tickets
          </Button>
          <Button
            ref={confirmRef}
            size="lg"
            className="min-h-11 sm:min-w-40"
            disabled={markRead.isPending}
            onClick={confirm}
          >
            {markRead.isPending ? "Wird gespeichert…" : "Verstanden"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
