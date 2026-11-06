import { ArrowLeftIcon } from "lucide-react"
import Link from "next/link"
import { notFound } from "next/navigation"

import { getSession, requireOrganizer } from "@/features/auth"
import {
  DraftForm,
  getEventCatalog,
  getOwnEvent,
  IMAGE_FAILED_PARAM,
} from "@/features/events"
import { Button } from "@/features/shared/components/ui/button"

export default async function EditEventDraftPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
  await requireOrganizer()
  const session = await getSession()
  const { id } = await params
  const imageFailed = Boolean((await searchParams)[IMAGE_FAILED_PARAM])

  const event = await getOwnEvent(session?.accessToken ?? "", id)

  // All events can be edited independent of their status (draft, published, completed).
  if (!event) {
    notFound()
  }

  const catalog = await getEventCatalog(session?.accessToken ?? "")

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
            Zurück zur Veranstaltungsliste
          </Button>
        </div>
        <div>
          <h1 className="text-lg font-medium">Veranstaltung bearbeiten</h1>
          <p className="text-sm text-muted-foreground">
            Ändere die Angaben im Formular und klicke auf „Änderungen
            speichern“.
          </p>
        </div>
        <DraftForm catalog={catalog} event={event} imageFailed={imageFailed} />
      </div>
    </div>
  )
}
