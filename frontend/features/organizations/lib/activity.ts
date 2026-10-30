import type { Event } from "@/features/events"

export interface ActivityPoint {
  month: string
  created: number
  held: number
}

const monthFormat = new Intl.DateTimeFormat("de-DE", { month: "short" })

function monthKey(date: Date) {
  return `${date.getFullYear()}-${date.getMonth()}`
}

export function activityByMonth(
  events: Pick<Event, "createdAt" | "endTime" | "status">[],
  now = new Date(),
  months = 6
): ActivityPoint[] {
  const points = new Map<string, ActivityPoint>()

  for (let offset = months - 1; offset >= 0; offset--) {
    const month = new Date(now.getFullYear(), now.getMonth() - offset, 1)
    points.set(monthKey(month), {
      month: monthFormat.format(month),
      created: 0,
      held: 0,
    })
  }

  for (const event of events) {
    const created = points.get(monthKey(new Date(event.createdAt)))
    if (created) {
      created.created++
    }

    const end = new Date(event.endTime)
    if (
      end <= now &&
      event.status !== "draft" &&
      event.status !== "cancelled"
    ) {
      const held = points.get(monthKey(end))
      if (held) {
        held.held++
      }
    }
  }

  return [...points.values()]
}
