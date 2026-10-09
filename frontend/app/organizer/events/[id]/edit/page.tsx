import { ArrowLeftIcon } from "lucide-react"
import Link from "next/link"
import { notFound } from "next/navigation"

import { getSession, requireOrganizer } from "@/features/auth"
import { DraftForm, getEventCatalog, getOwnEvent } from "@/features/events"
import { Button } from "@/features/shared/components/ui/button"

export default async function EditEventDraftPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  await requireOrganizer()
  const session = await getSession()
  const { id } = await params

  const event = await getOwnEvent(session?.accessToken ?? "", id)

  // Only drafts are editable here; a published event is a different story.
  if (!event || event.status !== "draft") {
    notFound()
  }

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
          <h1 className="text-lg font-medium">Edit Draft</h1>
          <p className="text-sm text-muted-foreground">
            Still a draft and not visible to visitors.
          </p>
        </div>
        <DraftForm catalog={catalog} event={event} />
      </div>
    </div>
  )
}
