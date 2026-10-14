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
            All events
          </Button>
        </div>
        <OrganizerEventDetails eventId={id} />
      </div>
    </div>
  )
}
