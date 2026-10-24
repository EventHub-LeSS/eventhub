import { requireAdmin } from "@/features/auth"

export default async function AdminPage() {
  const user = await requireAdmin()

  return (
    <div className="flex flex-1 flex-col gap-4 p-6">
      <div>
        <h1 className="font-medium">Administration</h1>
        <p className="text-sm text-muted-foreground">
          Signed in as {user.name}.
        </p>
      </div>
    </div>
  )
}
