import { requireSession } from "@/features/auth";

export default async function JoinOrganizationPage() {
  await requireSession();

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="font-medium">Organisation beitreten</h1>
        <p className="text-sm text-muted-foreground">Das funktioniert noch nicht :(</p>
      </div>
    </div>
  );
}
