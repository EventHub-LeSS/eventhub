import { requireAdmin } from "@/features/auth";
import { CreateOrganizationForm } from "@/features/organizations";

export default async function CreateOrganizationPage() {
  const user = await requireAdmin();

  return (
    <div className="flex flex-1 justify-center p-6">
      <div className="flex w-full max-w-xl flex-col gap-6">
        <div>
          <h1 className="text-lg font-medium">Create an Organization</h1>
          <p className="text-sm text-muted-foreground">
            Go do and create great things
          </p>
        </div>
        <CreateOrganizationForm defaultOrgAdmin={user.email} />
      </div>
    </div>
  );
}
