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
        // Reading the clock here is safe: this is a server component, which
        // renders once per request, so this is the request time — not a
        // value that could change between re-renders.
        // oxlint-disable react/purity
        <OwnTickets {...splitTicketsByEnd(tickets, Date.now())} />
        // oxlint-enable react/purity
      )}
    </div>
  )
}
