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
            Entwürfe
          </Button>
        </div>
        <div>
          <h1 className="text-lg font-medium">Veranstaltung erstellen</h1>
          <p className="text-sm text-muted-foreground">
            Die Veranstaltung wird als Entwurf für {user.activeOrganization}
            gespeichert. Du kannst sie später bearbeiten und veröffentlichen.
          </p>
        </div>
        <DraftForm catalog={catalog} />
      </div>
    </div>
  )
}
