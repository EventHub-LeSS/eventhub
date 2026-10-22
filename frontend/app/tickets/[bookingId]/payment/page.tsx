import { notFound } from "next/navigation"

import { requireSession } from "@/features/auth"
import { PaymentView } from "@/features/bookings"

interface PageProps {
  params: Promise<{ bookingId: string }>
}

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/**
 * EVENTHUB-235: the view that carries out the (simulated) payment process of
 * a booking. It shows the amount stored by the backend before completion and
 * the clear result afterwards. Protected like every page under /tickets
 * (see proxy.ts), and the session is rechecked here.
 */
export default async function PaymentPage({ params }: PageProps) {
  await requireSession()
  const { bookingId } = await params

  if (!UUID_PATTERN.test(bookingId)) {
    notFound()
  }

  return (
    <div className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Zahlung</h1>
        <p className="text-sm text-muted-foreground">
          Schließe die Zahlung ab, um deine Tickets verbindlich zu buchen.
        </p>
      </div>
      <PaymentView bookingId={bookingId} />
    </div>
  )
}
