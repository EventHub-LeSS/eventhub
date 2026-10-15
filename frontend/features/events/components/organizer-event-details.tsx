import { notFound } from "next/navigation"

import { EventActions } from "@/features/events/components/event-actions"
import { EventDetails } from "@/features/events/components/event-details"
import { getOrganizerEvent } from "@/features/events/lib/get-organizer-events"
export async function OrganizerEventDetails({ eventId }: { eventId: string }) {
  const result = await getOrganizerEvent(eventId)

  // Also covers events of other organizations, so URLs cannot reach them.
  if (result.status === "not-found") {
    notFound()
  }

  if (result.status === "unavailable") {
    return (
      <p className="text-sm text-destructive">
        This event can&apos;t be loaded right now. Please try again later.
      </p>
    )
  }

  const { event } = result
  return (
    <EventDetails
      event={event}
      actions={
        <EventActions
          eventId={event.eventId}
          eventTitle={event.title}
          status={event.status}
          showViewDetails={false}
        />
      }
    />
  )
}
