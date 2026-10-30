import { getSession, requireSession } from "@/features/auth"
import {
  getOwnTickets,
  OwnTickets,
  splitTicketsByEnd,
  type OwnBooking,
} from "@/features/bookings"

export default async function TicketsPage() {
  await requireSession()
  const session = await getSession()

  let tickets: OwnBooking[] | null = null
  if (session) {
    try {
      tickets = await getOwnTickets(session.accessToken)
    } catch (error) {
      console.error("Loading own tickets failed:", error)
      tickets = null
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Meine Tickets</h1>
        <p className="text-sm text-muted-foreground">
          Deine gültigen Tickets. Vergangene Veranstaltungen kannst du bewerten.
        </p>
      </div>
      {tickets === null ? (
        <p className="text-sm text-muted-foreground" role="alert">
          Deine Tickets können aktuell nicht geladen werden. Bitte versuche es
          später erneut.
        </p>
      ) : (
        <OwnTickets {...splitTicketsByEnd(tickets, Date.now())} />
      )}
    </div>
  )
}
