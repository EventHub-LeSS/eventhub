"use client"

import { useState, useTransition } from "react"

import { cancelBookingAction } from "@/features/bookings/lib/actions"
import { formatAmount, formatTicketCount } from "@/features/bookings/lib/format"
import type { OwnBooking } from "@/features/bookings/lib/types"
import { Button } from "@/features/shared/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"
import { toast } from "@/features/shared/components/ui/toast"

interface CancelBookingDialogProps {
  ticket: OwnBooking
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CancelBookingDialog({
  ticket,
  open,
  onOpenChange,
}: CancelBookingDialogProps) {
  const [message, setMessage] = useState<string>()
  const [isCancelling, startCancel] = useTransition()

  const title = ticket.event?.title ?? "diese Veranstaltung"
  const paid = ticket.paymentStatus === "paid" && ticket.amount !== null

  function handleOpenChange(nextOpen: boolean) {
    if (isCancelling) return

    if (!nextOpen) {
      setMessage(undefined)
    }

    onOpenChange(nextOpen)
  }

  function handleCancel() {
    if (isCancelling) return

    setMessage(undefined)
    startCancel(async () => {
      const result = await cancelBookingAction(ticket.bookingId)

      if (result.message) {
        setMessage(result.message)
        return
      }

      onOpenChange(false)
      toast.add({
        type: "success",
        title: "Buchung storniert",
        description: paid
          ? `${formatAmount(ticket.amount)} werden dir erstattet.`
          : `Deine Tickets für "${title}" wurden storniert.`,
      })
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Buchung stornieren?</DialogTitle>
          <DialogDescription>
            {formatTicketCount(ticket.numberOfTickets)} für &quot;{title}&quot;
            {paid
              ? ` werden storniert und ${formatAmount(ticket.amount)} vollständig erstattet.`
              : " werden storniert."}{" "}
            Die Tickets werden wieder freigegeben, das kann nicht rückgängig
            gemacht werden.
          </DialogDescription>
        </DialogHeader>

        {message && (
          <p className="text-sm text-destructive" role="alert">
            {message}
          </p>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isCancelling}
          >
            Behalten
          </Button>
          <Button
            variant="destructive"
            onClick={handleCancel}
            disabled={isCancelling}
          >
            {isCancelling ? "Wird storniert…" : "Buchung stornieren"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
