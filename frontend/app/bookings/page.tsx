import { requireSession } from "@/features/auth"

export default async function BookingsPage() {
  const user = await requireSession()

  return (
    <div className="flex flex-1 flex-col gap-2 p-6">
      <h1 className="font-medium">Bookings</h1>
      <p className="text-sm text-muted-foreground">
        Signed in as {user.email}. Your bookings will show up here.
      </p>
    </div>
  )
}
