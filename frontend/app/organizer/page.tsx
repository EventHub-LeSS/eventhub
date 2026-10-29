import { CirclePlusIcon, TicketsIcon } from "lucide-react"
import Link from "next/link"

import {
  DashboardFilters,
  getOrganizerDashboardData,
  OccupancyDashboard,
  parseDashboardFilters,
  SalesDashboard,
} from "@/features/events"
import { getSession, requireOrganizer } from "@/features/auth"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { envConfig } from "@/features/shared/lib/env"

interface PageProps {
  searchParams: Promise<{ status?: string; category?: string; q?: string }>
}

/** Mirrors the backend's `CurrentUserResponse` (GET /users/me). */
interface CurrentUserResponse {
  organizations: {
    id: string
    alias: string
    roles: string[]
  }[]
  /** The token's active organization, if any. */
  organization?: {
    id: string
    alias: string
    roles: string[]
  }
}

async function getUserFinancialRights(
  accessToken: string,
  organizationAlias: string,
): Promise<{ canViewFinance: boolean }> {
  try {
    const response = await fetch(`${envConfig.apiBaseUrl}/users/me`, {
      headers: { Authorization: `Bearer ${accessToken}` },
      cache: "no-store",
    })

    if (!response.ok) {
      return { canViewFinance: false }
    }

    const me = (await response.json()) as CurrentUserResponse
    // Matching by alias (not only the token's active organization) covers the
    // case where the cookie's org differs from the token's active organization,
    // e.g. right after switching via the EventHub logo.
    const orgs = me.organization
      ? [...me.organizations, me.organization]
      : me.organizations
    const org = orgs.find((candidate) => candidate.alias === organizationAlias)
    const roles = org?.roles ?? []

    return {
      canViewFinance:
        roles.includes("finance_viewer") || roles.includes("org_admin"),
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
    ? await getUserFinancialRights(session.accessToken, user.activeOrganization)
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
