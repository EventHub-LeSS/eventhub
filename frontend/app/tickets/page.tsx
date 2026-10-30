import { getSession, requireSession } from "@/features/auth"
import {
  getOwnBookings,
  getOwnTickets,
  OwnTickets,
  parseTicketView,
  splitTicketsByEnd,
  type OwnBooking,
} from "@/features/bookings"

interface PageProps {
  searchParams: Promise<{ view?: string | string[] }>
}

export default async function TicketsPage({ searchParams }: PageProps) {
  await requireSession()
  const { view } = await searchParams
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

  if (tickets !== null) {
    return (
      <OwnTickets
        {...splitTicketsByEnd(tickets, Date.now())}
        cancelled={cancelled}
        initialView={parseTicketView(view)}
      />
    )
  }

  return (
    <div className="flex w-full flex-1 flex-col gap-6 p-6">
      <h1 className="text-2xl font-semibold tracking-tight">Meine Tickets</h1>
      <p className="text-sm text-muted-foreground" role="alert">
        Deine Tickets können aktuell nicht geladen werden. Bitte versuche es
        später erneut.
      </p>
    </div>
  )
}
