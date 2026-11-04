import { ArrowLeftIcon } from "lucide-react"
import Link from "next/link"

import { requireOrganizer } from "@/features/auth"
import { OrganizerEventDetails } from "@/features/events"
import { Button } from "@/features/shared/components/ui/button"

export default async function OrganizerEventPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  await requireOrganizer()
  const { id } = await params

  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div>
        <Button
          render={<Link href="/organizer/events" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Alle Veranstaltungen
        </Button>
      </div>
      <OrganizerEventDetails eventId={id} />
    </div>
  )
}
