"use client"

import { useState } from "react"

import { ActivityChart } from "@/features/organizations/components/activity-chart"
import { AuditLogTable } from "@/features/organizations/components/audit-log-table"
import { MembersTable } from "@/features/organizations/components/members-table"
import { OrganizationHeader } from "@/features/organizations/components/organization-header"
import {
  type ActivityEvent,
  activityByMonth,
} from "@/features/organizations/lib/activity"
import {
  useOrganization,
  useOrganizationAuditLog,
  useOrganizationMembers,
  useUpdateOrganization,
} from "@/features/organizations/lib/api"
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
import { toast } from "@/features/shared/components/ui/toast"

export function OrganizationOverview({
  alias,
  rights,
  isAdmin = false,
  events,
}: {
  alias: string
  /** Null when the events could not be loaded. */
  events: ActivityEvent[] | null
  rights: string[]
  isAdmin?: boolean
}) {
  const hasRight = (right: OrganizationRight) =>
    isAdmin || rights.includes(right)
  const canViewMembers = hasRight("org_admin")
  const canViewAuditLog = hasRight("org_admin")
  const canEditOrganization = hasRight("org_admin")

  const membersQuery = useOrganizationMembers(alias, canViewMembers)
  const organizationQuery = useOrganization(alias)
  const updateOrganization = useUpdateOrganization(alias)

  // Fall back to the alias until the details request resolves.
  const name = organizationQuery.data?.name ?? alias

  const [isEditing, setIsEditing] = useState(false)
  const [draftName, setDraftName] = useState(name)

  function handleEditClick() {
    setDraftName(name)
    setIsEditing(true)
  }

  function handleSaveClick() {
    const trimmedName = draftName.trim()

    if (!trimmedName) {
      toast.add({
        type: "error",
        title: "Speichern fehlgeschlagen",
        description: "Der Name der Organisation darf nicht leer sein.",
      })
      return
    }

    updateOrganization.mutate(trimmedName, {
      onSuccess: (data) => {
        setIsEditing(false)
        toast.add({
          type: "success",
          title: "Organisation aktualisiert",
          description: `Der Name ist jetzt „${data.name}".`,
        })
      },
      onError: (error) => {
        toast.add({
          type: "error",
          title: "Speichern fehlgeschlagen",
          description: error.message,
        })
      },
    })
  }

  const members = membersQuery.data ?? []
  const activity = events ? activityByMonth(events, members) : null

  return (
    <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
      <div className="flex flex-col gap-6">
        <OrganizationHeader
          alias={alias}
          isEditing={isEditing}
          isSaving={updateOrganization.isPending}
          onEditClick={canEditOrganization ? handleEditClick : undefined}
          onSaveClick={canEditOrganization ? handleSaveClick : undefined}
        />

        <Card>
          <CardHeader>
            <CardTitle>Organisationsdetails</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col divide-y divide-border">
            <SettingsRow
              label="Name"
              value={isEditing ? draftName : name}
              isEditing={isEditing}
              onChange={setDraftName}
            />
            {/* The alias is the identity reference carried by the token claim
                and deliberately not changeable, so it is always read-only. */}
            <SettingsRow label="Alias" value={alias} />
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="overview" className="min-w-0">
        <TabsList>
          <TabsTrigger value="overview">Übersicht</TabsTrigger>
          {canViewAuditLog && (
            <TabsTrigger value="audit-log">Audit-Log</TabsTrigger>
          )}
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-6">
          {activity ? (
            <ActivityChart
              data={activity}
              showJoined={Boolean(membersQuery.data)}
            />
          ) : (
            <Card>
              <CardContent className="py-6 text-center text-sm text-destructive">
                Die Aktivität kann gerade nicht geladen werden. Bitte versuche
                es später erneut.
              </CardContent>
            </Card>
          )}

          {canViewMembers && (
            <Card>
              <CardHeader>
                <CardTitle>
                  Mitglieder{membersQuery.data ? ` (${members.length})` : ""}
                </CardTitle>
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
          )}
        </TabsContent>

        {canViewAuditLog && (
          <TabsContent value="audit-log">
            <AuditLogCard alias={alias} />
          </TabsContent>
        )}
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
            {auditLogQuery.isFetchingNextPage
              ? "Wird geladen..."
              : "Mehr laden"}
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
    <div className="flex flex-col gap-1.5 py-3 first:pt-0 last:pb-0">
      <p className="text-sm font-medium text-foreground">{label}</p>
      {isEditing && onChange ? (
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : (
        <p className="text-sm break-words text-muted-foreground">{value}</p>
      )}
    </div>
  )
}
