import { CirclePlusIcon, TicketsIcon } from "lucide-react"
import Link from "next/link"

import { requireOrganizer } from "@/features/auth"
import { Button } from "@/features/shared/components/ui/button"

export default async function OrganizerPage() {
  const user = await requireOrganizer()

  return (
    <div className="flex flex-1 flex-col gap-4 p-6">
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
    </div>
  )
}
