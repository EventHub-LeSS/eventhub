/** Mirrors the API's NotificationData; nullable fields arrive as null, not omitted. */
export interface Notification {
  notificationId: string
  userId: string | null
  /** e.g. "event_reminder" or "general" */
  occasion: string
  subject: string
  content: string
  bookingId: string | null
  scheduledAt: string | null
  simulated: boolean
  /** null means unread */
  readAt: string | null
  createdAt: string
  updatedAt: string
}

export interface NotificationPage {
  items: Notification[]
  page: number
  limit: number
  total: number
}
