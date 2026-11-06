import { getSession, requireSession } from "@/features/auth"
import {
  getOwnTickets,
  OwnTickets,
  parseTicketView,
  splitOwnTickets,
  type OwnBooking,
} from "@/features/bookings"

interface PageProps {
  searchParams: Promise<{ view?: string | string[] }>
}

export default async function TicketsPage({ searchParams }: PageProps) {
  await requireSession()
  const { view } = await searchParams
  const session = await getSession()

  // The ticket list holds valid tickets and paid tickets that were cancelled,
  // by the visitor or by the organizer (cancelledBy), in one consistent read.
  let tickets: OwnBooking[] | null = null
  if (session) {
    try {
      tickets = await getOwnTickets(session.accessToken)
    } catch (error) {
      console.error("Loading own tickets failed:", error)
    }
  }

  if (tickets !== null) {
    const initialView = parseTicketView(view)

    return (
      <OwnTickets
        // A navigation to another ?view= (e.g. from the cancellation notice
        // while on this page) remounts the tabs, so the view is applied.
        key={initialView}
        // Reading the clock here is safe: this is a server component, which
        // renders once per request, so this is the request time — not a
        // value that could change between re-renders.
        // oxlint-disable-next-line react/purity
        {...splitOwnTickets(tickets, Date.now())}
        initialView={initialView}
      />
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <h1 className="text-2xl font-semibold tracking-tight">Meine Tickets</h1>
      <p className="text-sm text-muted-foreground" role="alert">
        Deine Tickets können aktuell nicht geladen werden. Bitte versuche es
        später erneut.
      </p>
    </div>
  )
}
