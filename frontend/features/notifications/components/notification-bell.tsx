"use client"

import { BellIcon, BellOffIcon } from "lucide-react"

import {
  useMarkNotificationRead,
  useNotifications,
} from "@/features/notifications/lib/api"
import type { Notification } from "@/features/notifications/lib/types"
import { Button } from "@/features/shared/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/features/shared/components/ui/dropdown-menu"
import { cn } from "@/features/shared/lib/utils"

const occasionLabels: Record<string, string> = {
  event_reminder: "Erinnerung",
  general: "Allgemein",
}

const dateTimeFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "medium",
  timeStyle: "short",
})

function formatDateTime(value: string) {
  return dateTimeFormat.format(new Date(value))
}

export function NotificationBell() {
  const { data, isPending, isError } = useNotifications()
  const markRead = useMarkNotificationRead()

  const notifications = data?.items ?? []
  // Counts the loaded page only; the API has no global unread count.
  const unread = notifications.filter((item) => item.readAt === null).length

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon-lg" className="relative" />}
        aria-label={
          unread > 0
            ? `Benachrichtigungen, ${unread} ungelesen`
            : "Benachrichtigungen"
        }
      >
        <BellIcon className="size-5" />
        {unread > 0 && (
          <span className="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] leading-none font-semibold text-primary-foreground">
            {unread > 9 ? "9+" : unread}
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="w-80 max-w-[calc(100vw-2rem)]"
      >
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <span>Benachrichtigungen</span>
            {unread > 0 && <span>{unread} ungelesen</span>}
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        {isPending ? (
          <p className="px-2 py-6 text-center text-sm text-muted-foreground">
            Wird geladen …
          </p>
        ) : isError ? (
          <p className="px-2 py-6 text-center text-sm text-destructive">
            Benachrichtigungen konnten nicht geladen werden.
          </p>
        ) : notifications.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-2 py-8 text-center text-sm text-muted-foreground">
            <BellOffIcon className="size-6" />
            <p className="font-medium text-foreground">
              Keine Benachrichtigungen
            </p>
            <p>Erinnerungen zu deinen Buchungen erscheinen hier.</p>
          </div>
        ) : (
          <DropdownMenuGroup className="max-h-96 overflow-y-auto">
            {notifications.map((notification) => (
              <NotificationItem
                key={notification.notificationId}
                notification={notification}
                onRead={() => markRead.mutate(notification.notificationId)}
              />
            ))}
          </DropdownMenuGroup>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function NotificationItem({
  notification,
  onRead,
}: {
  notification: Notification
  onRead: () => void
}) {
  const isUnread = notification.readAt === null
  const occasion =
    occasionLabels[notification.occasion] ?? notification.occasion

  return (
    <DropdownMenuItem
      closeOnClick={false}
      onClick={isUnread ? onRead : undefined}
      className={cn(
        "flex-col items-start gap-0.5 py-2",
        isUnread && "bg-muted/60"
      )}
    >
      <div className="flex w-full items-center gap-2 text-xs text-muted-foreground">
        {isUnread && (
          <span className="size-2 shrink-0 rounded-full bg-primary">
            <span className="sr-only">Ungelesen</span>
          </span>
        )}
        <span>{occasion}</span>
        <time dateTime={notification.createdAt} className="ml-auto">
          {formatDateTime(notification.createdAt)}
        </time>
      </div>
      <p
        className={cn(
          "text-sm",
          isUnread ? "font-semibold" : "text-muted-foreground"
        )}
      >
        {notification.subject}
      </p>
      <p className="text-xs whitespace-pre-line text-muted-foreground">
        {notification.content}
      </p>
      {notification.scheduledAt && (
        <p className="text-xs text-muted-foreground">
          Geplant für{" "}
          <time dateTime={notification.scheduledAt}>
            {formatDateTime(notification.scheduledAt)}
          </time>
        </p>
      )}
    </DropdownMenuItem>
  )
}
