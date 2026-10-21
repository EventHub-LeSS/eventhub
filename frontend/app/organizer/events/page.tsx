import { ArrowLeftIcon, CirclePlusIcon } from "lucide-react"
import Link from "next/link"

import { getSession, requireOrganizer } from "@/features/auth"
import {
  type Event,
  EventApiError,
  listOwnEvents,
  OrganizerEventList,
  PublishDraftButton,
} from "@/features/events"
import { Button } from "@/features/shared/components/ui/button"

const dateFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "medium",
  timeStyle: "short",
})

function formatTimeSpan(event: Event): string {
  return `${dateFormat.format(new Date(event.startTime))} – ${dateFormat.format(
    new Date(event.endTime)
  )}`
}

export default async function OrganizerDraftsPage() {
  await requireOrganizer()
  const session = await getSession()

  let drafts: Event[] = []
  let loadError: string | undefined

  try {
    drafts = await listOwnEvents(session?.accessToken ?? "", "draft")
  } catch (error) {
    if (!(error instanceof EventApiError)) {
      throw error
    }

    loadError = error.message
  }

  return (
    <div className="flex flex-1 flex-col gap-4 p-6">
      <div>
        <Button
          render={<Link href="/organizer" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Dashboard
        </Button>
      </div>

      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h1 className="font-medium">Entwürfe</h1>
          <p className="text-sm text-muted-foreground">
            Nur du und deine Organisation können diese sehen. Besucher finden
            sie erst, wenn du sie veröffentlichst.
          </p>
        </div>
        <Button
          render={<Link href="/organizer/events/new" />}
          nativeButton={false}
        >
          <CirclePlusIcon />
          Veranstaltung erstellen
        </Button>
      </div>

      {loadError && <p className="text-sm text-destructive">{loadError}</p>}

      {!loadError && drafts.length === 0 && (
        <p className="text-sm text-muted-foreground">
          Du hast noch keine Entwürfe.
        </p>
      )}

      {drafts.length > 0 && (
        <ul className="flex flex-col gap-2">
          {drafts.map((draft) => (
            <li
              key={draft.eventId}
              className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border p-4"
            >
              <div>
                <p className="text-sm font-medium">{draft.title}</p>
                <p className="text-xs text-muted-foreground">
                  {formatTimeSpan(draft)} · {draft.capacity} Plätze ·{" "}
                  {draft.price}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  render={
                    <Link href={`/organizer/events/${draft.eventId}/edit`} />
                  }
                  nativeButton={false}
                  variant="outline"
                  size="sm"
                >
                  Bearbeiten
                </Button>
                <PublishDraftButton eventId={draft.eventId} />
              </div>
            </li>
          ))}
        </ul>
      )}

      <OrganizerEventList />
    </div>
  )
}
