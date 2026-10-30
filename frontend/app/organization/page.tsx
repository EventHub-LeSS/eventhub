import { requireOrganizer } from "@/features/auth";
import { getOrganizerEvents } from "@/features/events";
import {
  EventsTable,
  OrganizationOverview,
  activityByMonth,
} from "@/features/organizations";

export default async function OrganizationPage() {
  const user = await requireOrganizer();
  const eventsResult = await getOrganizerEvents();
  const events = eventsResult.status === "ok" ? eventsResult.events : null;

  return (
    <div className="flex flex-1 justify-center p-6">
      <div className="w-full max-w-6xl">
        <OrganizationOverview
          alias={user.activeOrganization as string}
          rights={user.activeOrganizationRoles}
          isAdmin={user.isAdmin}
          eventCount={events ? events.length : null}
          activity={events ? activityByMonth(events) : null}
          eventsTable={<EventsTable events={events} />}
        />
      </div>
    </div>
  );
}
