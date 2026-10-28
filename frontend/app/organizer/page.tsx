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
import { getSession } from "@/features/auth"
import {
  getOrganizationByAlias,
  getOrganizationMembersWithRights,
} from "@/features/organizations"

interface PageProps {
  searchParams: Promise<{ status?: string; category?: string; q?: string }>
}

async function getUserFinancialRights(
  accessToken: string,
  organizationAlias: string,
  userEmail: string,
): Promise<{ canViewFinance: boolean }> {
  try {
    const org = await getOrganizationByAlias(accessToken, organizationAlias)
    const members = await getOrganizationMembersWithRights(accessToken, org.id)
    const member = members.find((m) => m.email.toLowerCase() === userEmail.toLowerCase())
    const rights = member?.rights ?? []
    return {
      canViewFinance: rights.includes("finance_viewer") || rights.includes("org_admin"),
    }
  } catch {
    return { canViewFinance: false }
  }
}
export default async function OrganizerPage({ searchParams }: PageProps) {
  const user = await requireOrganizer()
  const session = await getSession()
  const filters = parseDashboardFilters(await searchParams)
  const dashboard = await getOrganizerDashboardData(filters)

  const { canViewFinance } = user.activeOrganization && session?.accessToken
    ? await getUserFinancialRights(
        session.accessToken,
        user.activeOrganization,
        user.email,
      )
    : { canViewFinance: false }

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="font-medium">Veranstalter-Dashboard</h1>
        <p className="text-sm text-muted-foreground">
          Du veröffentlichst als {user.activeOrganization}. Die Organisation
          kannst du über das EventHub-Logo wechseln.
        </p>
      </div>

      <div className="flex gap-2">
        <Button
          render={<Link href="/organizer/events" />}
          nativeButton={false}
          variant="outline"
        >
          <TicketsIcon />
          Meine Veranstaltungen
        </Button>
        {/* Creating events needs the event_manager role, the API refuses others. */}
        {user.organizationRoles.includes("event_manager") && (
          <Button
            render={<Link href="/organizer/events/new" />}
            nativeButton={false}
          >
            <CirclePlusIcon />
            Veranstaltung erstellen
          </Button>
        )}
      </div>

      {dashboard.status === "unavailable" && (
        <p className="text-sm text-destructive">
          Das Dashboard konnte gerade nicht geladen werden. Bitte versuche es später erneut.
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
              Du hast noch keine Veranstaltungen
            </CardTitle>
            <p className="text-sm text-muted-foreground">
              Erstelle eine Veranstaltung, um Auslastung und Verkäufe zu verfolgen.
            </p>
            <Button
              render={<Link href="/organizer/events/new" />}
              nativeButton={false}
            >
              Veranstaltung erstellen
            </Button>
          </CardContent>
        </Card>
      )}

      {dashboard.status === "ok" &&
        dashboard.totalEvents > 0 &&
        dashboard.events.length === 0 && (
          <p className="text-sm text-muted-foreground">
            Keine Veranstaltungen entsprechen den aktuellen Filtern.
          </p>
        )}

      {dashboard.status === "ok" && dashboard.events.length > 0 && (
        <div className={canViewFinance ? "grid gap-6 lg:grid-cols-2" : "grid gap-6"}>
          <OccupancyDashboard events={dashboard.events} />
          {canViewFinance && <SalesDashboard events={dashboard.events} />}
        </div>
      )}
    </div>
  )
}
