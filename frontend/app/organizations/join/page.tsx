import { requireSession } from "@/features/auth";

export default async function JoinOrganizationPage() {
  await requireSession();

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="font-medium">Join Organization</h1>
        <p className="text-sm text-muted-foreground">This Doesnt work yet :(</p>
      </div>
    </div>
  );
}
