import { notFound } from "next/navigation"

import { requireSession } from "@/features/auth"
import { CartPaymentView, isUUID } from "@/features/cart"

interface PageProps {
  params: Promise<{ paymentId: string }>
}

/**
 * EVENTHUB-235: the view that carries out the (simulated) payment process
 * of a whole cart checkout. It shows the line amounts and the total amount
 * stored by the backend before completion and the clear result afterwards.
 * Protected like every page under /cart (see proxy.ts), and the session is
 * rechecked here.
 */
export default async function CartPaymentPage({ params }: PageProps) {
  await requireSession()
  const { paymentId } = await params

  if (!isUUID(paymentId)) {
    notFound()
  }

  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Zahlung</h1>
        <p className="text-sm text-muted-foreground">
          Schließe die Zahlung ab, um alle Tickets verbindlich zu buchen.
        </p>
      </div>
      <CartPaymentView paymentId={paymentId} />
    </div>
  )
}
