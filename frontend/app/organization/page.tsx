import { requireOrganizer } from "@/features/auth";
import { OrganizationOverview } from "@/features/organizations";

export default async function OrganizationPage() {
  const user = await requireOrganizer();

  return (
    <div className="flex flex-1 justify-center p-6">
      <div className="w-full max-w-6xl">
        <OrganizationOverview
          alias={user.activeOrganization as string}
          currentUserEmail={user.email}
          isAdmin={user.isAdmin}
        />
      </div>
    </div>
  );
}
