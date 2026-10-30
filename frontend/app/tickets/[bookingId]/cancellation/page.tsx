import { notFound } from "next/navigation"

import { getSession, requireSession } from "@/features/auth"
import {
  BookingApiError,
  CancellationDetails,
  getOwnBooking,
  type OwnBooking,
} from "@/features/bookings"

interface PageProps {
  params: Promise<{ bookingId: string }>
}

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export default async function CancellationPage({ params }: PageProps) {
  await requireSession()
  const session = await getSession()
  const { bookingId } = await params

  if (!session || !UUID_PATTERN.test(bookingId)) {
    notFound()
  }

  let booking: OwnBooking | null = null
  try {
    booking = await getOwnBooking(session.accessToken, bookingId)
  } catch (error) {
    if (error instanceof BookingApiError && error.status === 404) {
      notFound()
    }

    console.error("Loading cancelled booking failed:", error)
  }

  if (booking && booking.bookingStatus !== "cancelled") {
    notFound()
  }

  // The details fill the screen below the navbar themselves.
  if (booking) {
    return <CancellationDetails booking={booking} />
  }

  return (
    <div className="flex w-full flex-1 flex-col gap-6 p-6">
      <h1 className="text-2xl font-semibold tracking-tight">Stornierung</h1>
      <p className="text-sm text-muted-foreground" role="alert">
        Die Stornierung kann aktuell nicht geladen werden. Bitte versuche es
        später erneut.
      </p>
    </div>
  )
}
