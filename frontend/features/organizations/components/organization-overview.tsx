"use client"

import { useState } from "react"

import { ActivityChart } from "@/features/organizations/components/activity-chart"
import { AuditLogTable } from "@/features/organizations/components/audit-log-table"
import { MembersTable } from "@/features/organizations/components/members-table"
import { OrganizationHeader } from "@/features/organizations/components/organization-header"
import {
  useOrganizationAuditLog,
  useOrganizationMembers,
} from "@/features/organizations/lib/api"
import type { ActivityPoint } from "@/features/organizations/lib/activity"
import type { OrganizationRight } from "@/features/organizations/lib/types"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { Input } from "@/features/shared/components/ui/input"
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/features/shared/components/ui/tabs"

export function OrganizationOverview({
  alias,
  rights,
  isAdmin = false,
  eventCount,
  activity,
  eventsTable,
}: {
  alias: string
  eventCount: number | null
  activity: ActivityPoint[] | null
  eventsTable: React.ReactNode
  rights: string[]
  isAdmin?: boolean
}) {
  const hasRight = (right: OrganizationRight) =>
    isAdmin || rights.includes(right)
  const canViewMembers = isAdmin
  const canViewEvents = hasRight("event_manager")
  const canViewAuditLog = hasRight("org_admin")

  const membersQuery = useOrganizationMembers(alias, canViewMembers)

  const [activeTab, setActiveTab] = useState("overview")
  const [isEditing, setIsEditing] = useState(false)
  const [draft, setDraft] = useState({ name: alias, alias })

  function handleTabChange(value: unknown) {
    setActiveTab(value as string)
    setIsEditing(false)
  }

  function handleEditClick() {
    setDraft({ name: alias, alias })
    setActiveTab("settings")
    setIsEditing(true)
  }

  function handleSaveClick() {
    setIsEditing(false)
  }

  const members = membersQuery.data ?? []

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[296px_1fr]">
      <OrganizationHeader
        alias={alias}
        isEditing={isEditing}
        onEditClick={handleEditClick}
        onSaveClick={handleSaveClick}
      />

      <Tabs value={activeTab} onValueChange={handleTabChange} className="min-w-0">
        <TabsList>
          <TabsTrigger value="overview">Übersicht</TabsTrigger>
          {canViewMembers && (
            <TabsTrigger value="members">
              Mitglieder{membersQuery.data ? ` (${members.length})` : ""}
            </TabsTrigger>
          )}
          {canViewEvents && (
            <TabsTrigger value="events">
              Veranstaltungen{eventCount !== null ? ` (${eventCount})` : ""}
            </TabsTrigger>
          )}
          <TabsTrigger value="settings">Einstellungen</TabsTrigger>
          {canViewAuditLog && (
            <TabsTrigger value="audit-log">Audit-Log</TabsTrigger>
          )}
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-4">
          {activity ? (
            <ActivityChart data={activity} />
          ) : (
            <Card>
              <CardContent className="py-6 text-center text-sm text-destructive">
                Die Aktivität kann gerade nicht geladen werden. Bitte versuche
                es später erneut.
              </CardContent>
            </Card>
          )}
        </TabsContent>

        {canViewMembers && (
          <TabsContent value="members">
            <Card>
              <CardHeader>
                <CardTitle>Mitglieder</CardTitle>
              </CardHeader>
              <CardContent>
                {membersQuery.isLoading ? (
                  <p className="py-6 text-center text-sm text-muted-foreground">
                    Mitglieder werden geladen…
                  </p>
                ) : membersQuery.isError ? (
                  <p className="py-6 text-center text-sm text-muted-foreground">
                    Die Mitglieder können gerade nicht angezeigt werden. Bitte
                    versuche es später erneut.
                  </p>
                ) : (
                  <MembersTable members={members} alias={alias} />
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {canViewEvents && (
          <TabsContent value="events">
            <Card>
              <CardHeader>
                <CardTitle>Veranstaltungen</CardTitle>
              </CardHeader>
              <CardContent>
                {eventsTable}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {canViewAuditLog && (
          <TabsContent value="audit-log">
            <AuditLogCard alias={alias} />
          </TabsContent>
        )}

        <TabsContent value="settings">
          <Card>
            <CardHeader>
              <CardTitle>Organisationsdetails</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col divide-y divide-border">
              <SettingsRow
                label="Name"
                value={draft.name}
                isEditing={isEditing}
                onChange={(value) => setDraft((prev) => ({ ...prev, name: value }))}
              />
              <SettingsRow
                label="Alias"
                value={draft.alias}
                isEditing={isEditing}
                onChange={(value) => setDraft((prev) => ({ ...prev, alias: value }))}
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function AuditLogCard({ alias }: { alias: string }) {
  const auditLogQuery = useOrganizationAuditLog(alias)
  const entries = auditLogQuery.data?.pages.flatMap((page) => page.items) ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle>Audit-Log</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {auditLogQuery.isLoading ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            Audit-Log wird geladen...
          </p>
        ) : auditLogQuery.isError && entries.length === 0 ? (
          <p className="py-6 text-center text-sm text-destructive">
            {auditLogQuery.error.message ||
              "Das Audit-Log konnte nicht geladen werden."}
          </p>
        ) : (
          <AuditLogTable entries={entries} />
        )}
        {auditLogQuery.hasNextPage && (
          <Button
            variant="outline"
            className="self-center"
            disabled={auditLogQuery.isFetchingNextPage}
            onClick={() => auditLogQuery.fetchNextPage()}
          >
            {auditLogQuery.isFetchingNextPage ? "Wird geladen..." : "Mehr laden"}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

function SettingsRow({
  label,
  value,
  isEditing,
  onChange,
}: {
  label: string
  value: string
  isEditing?: boolean
  onChange?: (value: string) => void
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
      <p className="text-sm font-medium text-foreground">{label}</p>
      {isEditing && onChange ? (
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className="max-w-xs"
        />
      ) : (
        <p className="text-sm text-muted-foreground">{value}</p>
      )}
    </div>
  )
}
