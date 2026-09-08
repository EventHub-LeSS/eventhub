"use client"

import { useState } from "react"

import { ActivityChart } from "@/features/organizations/components/activity-chart"
import { EventsTable } from "@/features/organizations/components/events-table"
import { MembersTable } from "@/features/organizations/components/members-table"
import { OrganizationHeader } from "@/features/organizations/components/organization-header"
import { OrganizationStats } from "@/features/organizations/components/organization-stats"
import {
  mockActivity,
  mockEvents,
  mockMembers,
  mockOrganization,
} from "@/features/organizations/lib/mock-data"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { Input } from "@/features/shared/components/ui/input"
import { Switch } from "@/features/shared/components/ui/switch"
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/features/shared/components/ui/tabs"

export function OrganizationOverview() {
  const [activeTab, setActiveTab] = useState("overview")
  const [isEditing, setIsEditing] = useState(false)
  const [draft, setDraft] = useState(mockOrganization)

  function handleTabChange(value: unknown) {
    setActiveTab(value as string)
    setIsEditing(false)
  }

  function handleEditClick() {
    setDraft(mockOrganization)
    setActiveTab("settings")
    setIsEditing(true)
  }

  function handleSaveClick() {
    setIsEditing(false)
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[296px_1fr]">
        <OrganizationHeader
          organization={mockOrganization}
          isEditing={isEditing}
          onEditClick={handleEditClick}
          onSaveClick={handleSaveClick}
        />
        <OrganizationStats members={mockMembers} events={mockEvents} />
      </div>

      <Tabs value={activeTab} onValueChange={handleTabChange}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="members">
            Members ({mockMembers.length})
          </TabsTrigger>
          <TabsTrigger value="events">Events ({mockEvents.length})</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-4">
          <ActivityChart data={mockActivity} />
        </TabsContent>

        <TabsContent value="members">
          <Card>
            <CardHeader>
              <CardTitle>Members</CardTitle>
            </CardHeader>
            <CardContent>
              <MembersTable members={mockMembers} />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="events">
          <Card>
            <CardHeader>
              <CardTitle>Events</CardTitle>
            </CardHeader>
            <CardContent>
              <EventsTable events={mockEvents} />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="settings">
          <Card>
            <CardHeader>
              <CardTitle>Organization details</CardTitle>
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
              <SettingsRow
                label="Contact email"
                value={draft.contactEmail}
                isEditing={isEditing}
                onChange={(value) =>
                  setDraft((prev) => ({ ...prev, contactEmail: value }))
                }
              />
              <SettingsRow
                label="Created"
                value={new Date(mockOrganization.createdAt).toLocaleDateString(
                  undefined,
                  { year: "numeric", month: "long", day: "numeric" }
                )}
              />
              <div className="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
                <div>
                  <p className="text-sm font-medium text-foreground">Enabled</p>
                  <p className="text-sm text-muted-foreground">
                    Disabled organizations cannot be used to publish events.
                  </p>
                </div>
                <Switch
                  checked={draft.enabled}
                  disabled={!isEditing}
                  onCheckedChange={(checked) =>
                    setDraft((prev) => ({ ...prev, enabled: checked }))
                  }
                />
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
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
