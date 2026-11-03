"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import type {
  Notification,
  NotificationPage,
} from "@/features/notifications/lib/types"

const NOTIFICATIONS_KEY = ["notifications"]

/** Reminders are refreshed by the API on every listing, so polling keeps the bell current. */
const REFETCH_INTERVAL_MS = 60_000

async function readJson<T>(response: Response): Promise<T> {
  const body = await response.json().catch(() => null)

  if (!response.ok) {
    const detail = (body as { detail?: unknown } | null)?.detail
    throw new Error(
      typeof detail === "string" && detail
        ? detail
        : `Anfrage fehlgeschlagen (${response.status}).`
    )
  }

  return body as T
}

async function fetchNotifications(): Promise<NotificationPage> {
  return readJson(await fetch("/api/notifications"))
}

async function markNotificationRead(
  notificationId: string
): Promise<Notification> {
  return readJson(
    await fetch(`/api/notifications/${notificationId}/read`, {
      method: "PATCH",
    })
  )
}

export function useNotifications() {
  return useQuery<NotificationPage, Error>({
    queryKey: NOTIFICATIONS_KEY,
    queryFn: fetchNotifications,
    refetchInterval: REFETCH_INTERVAL_MS,
  })
}

export function useMarkNotificationRead() {
  const queryClient = useQueryClient()

  return useMutation<Notification, Error, string>({
    mutationFn: markNotificationRead,
    onSuccess: (updated) => {
      queryClient.setQueryData<NotificationPage>(NOTIFICATIONS_KEY, (page) =>
        page
          ? {
              ...page,
              items: page.items.map((item) =>
                item.notificationId === updated.notificationId ? updated : item
              ),
            }
          : page
      )
    },
  })
}
