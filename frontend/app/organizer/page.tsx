import { CirclePlusIcon, TicketsIcon } from "lucide-react"
import Link from "next/link"

import {
  DashboardFilters,
  getOrganizerDashboardData,
  OccupancyDashboard,
  parseDashboardFilters,
  SalesDashboard,
} from "@/features/events"
import { requireOrganizer } from "@/features/auth"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardTitle,
} from "@/features/shared/components/ui/card"

interface PageProps {
  searchParams: Promise<{ status?: string; category?: string; q?: string }>
}

export default async function OrganizerPage({ searchParams }: PageProps) {
  const user = await requireOrganizer()
  const filters = parseDashboardFilters(await searchParams)
  const dashboard = await getOrganizerDashboardData(filters)

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="font-medium">Organizer Dashboard</h1>
        <p className="text-sm text-muted-foreground">
          Publishing as {user.activeOrganization}. The organization can be
          changed via the EventHub logo.
        </p>
      </div>

      <div className="flex gap-2">
        <Button
          render={<Link href="/organizer/events" />}
          nativeButton={false}
          variant="outline"
        >
          <TicketsIcon />
          My Events
        </Button>
        <Button
          render={<Link href="/organizer/events/new" />}
          nativeButton={false}
        >
          <CirclePlusIcon />
          Create Event
        </Button>
      </div>

      {dashboard.status === "unavailable" && (
        <p className="text-sm text-destructive">
          The dashboard could not be loaded right now. Please try again later.
        </p>
      )}

      {dashboard.status === "ok" && dashboard.totalEvents > 0 && (
        <DashboardFilters
          categories={dashboard.categories}
          filters={dashboard.filters}
        />
      )}

      {dashboard.status === "ok" && dashboard.totalEvents === 0 && (
        <Card className="py-8">
          <CardContent className="flex flex-col items-center gap-3">
            <TicketsIcon className="size-8 text-muted-foreground" />
            <CardTitle className="text-base font-medium">
              You have no events yet
            </CardTitle>
            <p className="text-sm text-muted-foreground">
              Create an event to start tracking occupancy and sales.
            </p>
            <Button
              render={<Link href="/organizer/events/new" />}
              nativeButton={false}
            >
              Create event
            </Button>
          </CardContent>
        </Card>
      )}

      {dashboard.status === "ok" &&
        dashboard.totalEvents > 0 &&
        dashboard.events.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No events match the current filters.
          </p>
        )}

      {dashboard.status === "ok" && dashboard.events.length > 0 && (
        <div className="grid gap-6 lg:grid-cols-2">
          <OccupancyDashboard events={dashboard.events} />
          <SalesDashboard events={dashboard.events} />
        </div>
      )}
    </div>
  )
}
