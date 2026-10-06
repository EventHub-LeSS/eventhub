import { ArrowLeftIcon } from "lucide-react"
import Link from "next/link"

import { requireOrganizer } from "@/features/auth"
import { DraftForm, getEventCatalog } from "@/features/events"
import { Button } from "@/features/shared/components/ui/button"

export default async function NewEventDraftPage() {
  const user = await requireOrganizer()
  const catalog = await getEventCatalog()

  return (
    <div className="flex flex-1 justify-center p-6">
      <div className="flex w-full max-w-xl flex-col gap-6">
        <div>
          <Button
            render={<Link href="/organizer/events" />}
            nativeButton={false}
            variant="ghost"
            size="sm"
          >
            <ArrowLeftIcon />
            Drafts
          </Button>
        </div>
        <div>
          <h1 className="text-lg font-medium">Create an Event</h1>
          <p className="text-sm text-muted-foreground">
            Saved as a draft for {user.activeOrganization}. You can edit and
            publish it later.
          </p>
        </div>
        <DraftForm catalog={catalog} />
      </div>
    </div>
  )
}
