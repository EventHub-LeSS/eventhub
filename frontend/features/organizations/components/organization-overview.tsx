"use client";

import { CheckIcon, PencilIcon } from "lucide-react"
import { useState } from "react"

import { ActivityChart } from "@/features/organizations/components/activity-chart";
import { AuditLogTable } from "@/features/organizations/components/audit-log-table";
import { MembersTable } from "@/features/organizations/components/members-table";
import { OrganizationHeader } from "@/features/organizations/components/organization-header";
import {
  type ActivityEvent,
  activityByMonth,
} from "@/features/organizations/lib/activity";
import {
  useOrganizationAuditLog,
  useOrganizationMembers,
} from "@/features/organizations/lib/api";
import type { OrganizationRight } from "@/features/organizations/lib/types";
import { Button } from "@/features/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card";
import { Input } from "@/features/shared/components/ui/input";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/features/shared/components/ui/tabs";

export function OrganizationOverview({
  alias,
  rights,
  isAdmin = false,
  events,
}: {
  alias: string;
  /** Null when the events could not be loaded. */
  events: ActivityEvent[] | null;
  rights: string[];
  isAdmin?: boolean;
}) {
  const hasRight = (right: OrganizationRight) =>
    isAdmin || rights.includes(right)
  const canViewMembers = isAdmin
  const canViewEvents = hasRight("event_manager")
  const canViewAuditLog = hasRight("org_admin")
  const canEditOrganization = hasRight("org_admin")

  const membersQuery = useOrganizationMembers(alias, canViewMembers)

  const membersQuery = useOrganizationMembers(alias, canViewMembers);

  function handleTabChange(value: unknown) {
    setActiveTab(value as string)
    setIsEditing(false)
    setDraft({ name: alias, alias })
  }
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState({ name: alias, alias });

  function handleEditClick() {
    setDraft({ name: alias, alias });
    setIsEditing(true);
  }

  function handleSaveClick() {
    setIsEditing(false);
  }

  const members = membersQuery.data ?? [];
  const activity = events ? activityByMonth(events, members) : null;

  return (
    <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
      <div className="flex flex-col gap-6">
        <OrganizationHeader
          alias={alias}
          isEditing={isEditing}
          onEditClick={handleEditClick}
          onSaveClick={handleSaveClick}
        />

        <Card>
          <CardHeader>
            <CardTitle>Organisationsdetails</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col divide-y divide-border">
            <SettingsRow
              label="Name"
              value={draft.name}
              isEditing={isEditing}
              onChange={(value) =>
                setDraft((prev) => ({ ...prev, name: value }))
              }
            />
            <SettingsRow
              label="Alias"
              value={draft.alias}
              isEditing={isEditing}
              onChange={(value) =>
                setDraft((prev) => ({ ...prev, alias: value }))
              }
            />
          </CardContent>
        </Card>
      </div>

      <Tabs value={activeTab} onValueChange={handleTabChange} className="min-w-0">
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

        <TabsContent value="settings">
          <Card>
            <CardHeader className="flex items-center justify-between gap-4">
              <CardTitle>Organisationsdetails</CardTitle>
              {canEditOrganization &&
                (isEditing ? (
                  <Button onClick={handleSaveClick}>
                    <CheckIcon />
                    Änderungen speichern
                  </Button>
                ) : (
                  <Button variant="outline" onClick={handleEditClick}>
                    <PencilIcon />
                    Bearbeiten
                  </Button>
                ))}
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
  );
}

function AuditLogCard({ alias }: { alias: string }) {
  const auditLogQuery = useOrganizationAuditLog(alias);
  const entries = auditLogQuery.data?.pages.flatMap((page) => page.items) ?? [];

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
  );
}

function SettingsRow({
  label,
  value,
  isEditing,
  onChange,
}: {
  label: string;
  value: string;
  isEditing?: boolean;
  onChange?: (value: string) => void;
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
  );
}
