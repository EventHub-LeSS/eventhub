"use client"

import { ShoppingCartIcon } from "lucide-react"
import Link from "next/link"

import { useCart } from "@/features/cart/lib/use-cart"
import { Button } from "@/features/shared/components/ui/button"

/**
 * The cart in the navbar: shows how many reserved tickets are waiting to
 * be paid. It counts the live reservations of the cart query plus the
 * tickets of the open checkout — both hold seats for the visitor — which
 * every cart change keeps current.
 */
export function CartButton() {
  const { data } = useCart()

  const totalTickets =
    (data?.totalTickets ?? 0) + (data?.openPayment?.totalTickets ?? 0)

  return (
    <Button
      render={<Link href="/cart" />}
      nativeButton={false}
      variant="ghost"
      size="icon-lg"
      className="relative"
      aria-label={
        totalTickets > 0
          ? `Warenkorb, ${totalTickets} reservierte Tickets`
          : "Warenkorb"
      }
    >
      <ShoppingCartIcon className="size-5" />
      {totalTickets > 0 && (
        <span className="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] leading-none font-semibold text-primary-foreground">
          {totalTickets > 9 ? "9+" : totalTickets}
        </span>
      )}
    </Button>
  )
}
