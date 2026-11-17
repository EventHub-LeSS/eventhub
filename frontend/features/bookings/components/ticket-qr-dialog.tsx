"use client"

import {
  CalendarIcon,
  ClockIcon,
  HashIcon,
  MapPinIcon,
  RotateCcwIcon,
  TicketIcon,
  type LucideIcon,
} from "lucide-react"
import QRCode from "react-qr-code"

import { TicketStatusBadge } from "@/features/bookings/components/ticket-status-badge"
import {
  formatAmount,
  formatDateTime,
  formatEventPeriod,
  formatLocation,
  formatTicketCount,
  hasRefund,
} from "@/features/bookings/lib/format"
import type { OwnBooking } from "@/features/bookings/lib/types"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"

/** The QR code encodes this address, so scanning it opens the linked page. */
const QR_CODE_URL = "https://www.youtube.com/watch?v=QDia3e12czc"

interface TicketQrDialogProps {
  ticket: OwnBooking
  kind: "valid" | "past" | "cancelled"
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** Full screen popup with all ticket details and a large, scannable QR code. */
export function TicketQrDialog({
  ticket,
  kind,
  open,
  onOpenChange,
}: TicketQrDialogProps) {
  const { event } = ticket

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="h-dvh max-w-none gap-0 overflow-y-auto rounded-none p-0 sm:max-w-none">
        <div className="flex min-h-full w-full flex-col items-center justify-center gap-8 p-6 pt-16 lg:gap-12 lg:p-12 lg:pt-12">
          <DialogHeader className="items-center gap-2 text-center">
            <DialogTitle className="text-2xl">
              {event?.title ?? "Veranstaltung nicht mehr verfügbar"}
            </DialogTitle>
            <TicketStatusBadge kind={kind} />
            <DialogDescription className="text-pretty">
              Zeig diesen Code am Einlass vor.
            </DialogDescription>
          </DialogHeader>

          <div className="flex w-full flex-col items-center justify-center gap-8 lg:flex-row lg:gap-16">
            <div className="flex flex-col items-center gap-4">
              <div className="rounded-3xl bg-white p-5 shadow-sm ring-1 ring-border">
                <QRCode
                  value={QR_CODE_URL}
                  title="QR-Code"
                  className="size-56 sm:size-72 lg:size-96"
                />
              </div>
            </div>

            <dl className="grid w-full gap-2 sm:grid-cols-2 lg:w-auto lg:max-w-3xl lg:flex-1 lg:grid-cols-2 lg:content-center">
              {event && (
                <>
                  <DetailRow
                    icon={CalendarIcon}
                    label="Zeitraum"
                    value={formatEventPeriod(event.startTime, event.endTime)}
                  />
                  <DetailRow
                    icon={MapPinIcon}
                    label="Ort"
                    value={formatLocation(event.location)}
                  />
                  <DetailRow
                    icon={TicketIcon}
                    label="Tickets"
                    value={
                      ticket.amount !== null
                        ? `${formatTicketCount(ticket.numberOfTickets)} · ${formatAmount(ticket.amount)}`
                        : formatTicketCount(ticket.numberOfTickets)
                    }
                  />
                </>
              )}
              {hasRefund(ticket) && (
                <DetailRow
                  icon={RotateCcwIcon}
                  label="Erstattet"
                  value={formatAmount(ticket.refundAmount)}
                />
              )}
              <DetailRow
                icon={HashIcon}
                label="Buchungsnummer"
                value={ticket.bookingId}
              />
              <DetailRow
                icon={ClockIcon}
                label="Gebucht am"
                value={formatDateTime(ticket.bookedAt)}
              />
            </dl>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function DetailRow({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon
  label: string
  value: string
}) {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-border bg-card p-3">
      <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      <div className="flex flex-col gap-0.5">
        <dt className="text-xs text-muted-foreground">{label}</dt>
        <dd className="text-sm font-medium break-words">{value}</dd>
      </div>
    </div>
  )
}
