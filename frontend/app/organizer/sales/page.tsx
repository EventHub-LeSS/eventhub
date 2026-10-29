import { requireOrganizer } from "@/features/auth"

export default async function OrganizerSalesPage() {
  const user = await requireOrganizer()

  return (
    <div className="flex flex-1 flex-col gap-4 p-6">
      <div>
        <h1 className="font-medium">Sales</h1>
        <p className="text-sm text-muted-foreground">
          Sales figures for {user.activeOrganization} will show up here.
        </p>
      </div>
    </div>
  )
}
