import { getSession, requireSession } from "@/features/auth"
import {
  getOwnBookings,
  getOwnTickets,
  OwnTickets,
  splitTicketsByEnd,
  type OwnBooking,
} from "@/features/bookings"

export default async function TicketsPage() {
  await requireSession()
  const session = await getSession()

  let tickets: OwnBooking[] | null = null
  let cancelled: OwnBooking[] = []
  if (session) {
    const [ticketsResult, cancelledResult] = await Promise.allSettled([
      getOwnTickets(session.accessToken),
      getOwnBookings(session.accessToken, "cancelled"),
    ])

    if (ticketsResult.status === "fulfilled") {
      tickets = ticketsResult.value
    } else {
      console.error("Loading own tickets failed:", ticketsResult.reason)
    }

    if (cancelledResult.status === "fulfilled") {
      cancelled = cancelledResult.value
    } else {
      console.error(
        "Loading cancelled bookings failed:",
        cancelledResult.reason
      )
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Meine Tickets</h1>
        <p className="text-sm text-muted-foreground">
          Deine gültigen Tickets. Bevorstehende kannst du stornieren, vergangene
          Veranstaltungen bewerten.
        </p>
      </div>
      {tickets === null ? (
        <p className="text-sm text-muted-foreground" role="alert">
          Deine Tickets können aktuell nicht geladen werden. Bitte versuche es
          später erneut.
        </p>
      ) : (
        <OwnTickets
          {...splitTicketsByEnd(tickets, Date.now())}
          cancelled={cancelled}
        />
      )}
    </div>
  )
}
