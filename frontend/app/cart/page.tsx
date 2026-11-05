import { requireSession } from "@/features/auth"
import { CartView } from "@/features/cart"

/**
 * The cart of the visitor: every live reservation without a payment, with
 * quantity, prices and the remaining reservation time per item, and the
 * checkout that turns everything into one payment. Protected like every
 * page under /cart (see proxy.ts), and the session is rechecked here.
 */
export default async function CartPage() {
  await requireSession()

  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Warenkorb</h1>
      </div>
      <CartView />
    </div>
  )
}
