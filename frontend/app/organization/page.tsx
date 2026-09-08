import { requireSession } from "@/features/auth";
import { OrganizationOverview } from "@/features/organizations";

export default async function OrganizationPage() {
  await requireSession();

  return (
    <div className="flex flex-1 justify-center p-6">
      <div className="w-full max-w-6xl">
        <OrganizationOverview />
      </div>
    </div>
  );
}
