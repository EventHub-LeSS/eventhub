import { requireSession } from "@/features/auth"

export default async function TicketsPage() {
  const user = await requireSession()

  return (
    <div className="flex flex-1 flex-col gap-2 p-6">
      <h1 className="font-medium">Meine Tickets</h1>
      <p className="text-sm text-muted-foreground">
        Angemeldet als {user.email}. Deine Buchungen erscheinen hier.
      </p>
    </div>
  )
}
