import {
  BanknoteIcon,
  CalendarDaysIcon,
  TicketIcon,
  UsersIcon,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"

import type {
  OrganizationEvent,
  OrganizationMember,
} from "@/features/organizations/lib/mock-data"
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

interface StatDefinition {
  label: string
  value: string
  hint: string
  icon: LucideIcon
}

export function OrganizationStats({
  members,
  events,
}: {
  members: OrganizationMember[]
  events: OrganizationEvent[]
}) {
  const activeMembers = members.filter((m) => m.status === "active").length
  const upcomingEvents = events.filter((e) => e.status === "upcoming").length
  const ticketsSold = events.reduce((sum, e) => sum + e.ticketsSold, 0)
  const revenue = ticketsSold * 12.5

  const stats: StatDefinition[] = [
    {
      label: "Members",
      value: members.length.toString(),
      hint: `${activeMembers} active`,
      icon: UsersIcon,
    },
    {
      label: "Upcoming events",
      value: upcomingEvents.toString(),
      hint: `${events.length} total`,
      icon: CalendarDaysIcon,
    },
    {
      label: "Tickets sold",
      value: ticketsSold.toLocaleString(),
      hint: "across all events",
      icon: TicketIcon,
    },
    {
      label: "Revenue",
      value: `€${revenue.toLocaleString(undefined, { maximumFractionDigits: 0 })}`,
      hint: "estimated, this year",
      icon: BanknoteIcon,
    },
  ]

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      {stats.map((stat) => (
        <Card key={stat.label}>
          <CardHeader>
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {stat.label}
            </CardTitle>
            <CardAction>
              <stat.icon className="size-4 text-muted-foreground" />
            </CardAction>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold">{stat.value}</div>
            <p className="text-xs text-muted-foreground">{stat.hint}</p>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
