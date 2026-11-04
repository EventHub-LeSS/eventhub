import { requireOrganizer } from "@/features/auth";
import { getOrganizerEvents } from "@/features/events";
import { OrganizationOverview } from "@/features/organizations";

export default async function OrganizationPage() {
  const user = await requireOrganizer();
  const eventsResult = await getOrganizerEvents();
  const events = eventsResult.status === "ok" ? eventsResult.events : null;

  return (
    <div className="mx-auto flex w-full max-w-[1440px] flex-1 flex-col p-6">
      <OrganizationOverview
        alias={user.activeOrganization as string}
        rights={user.organizationRoles}
        isAdmin={user.isAdmin}
        events={
          events?.map(({ createdAt, endTime, status }) => ({
            createdAt,
            endTime,
            status,
          })) ?? null
        }
      />
    </div>
  );
}
