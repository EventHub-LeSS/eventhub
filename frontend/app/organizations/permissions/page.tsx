import { requireSession } from "@/features/auth";

export default async function OrganizationPermissionsPage() {
  await requireSession();

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="font-medium">Permissions</h1>
        <p className="text-sm text-muted-foreground">This Doesnt work yet :(</p>
      </div>
    </div>
  );
}
