"use client"

import {
  CheckCircle2Icon,
  ClockIcon,
  RefreshCwIcon,
  XCircleIcon,
} from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useEffect, useRef, useState } from "react"

import {
  useDecideOrganizerApplication,
  useOrganizerApplications,
} from "@/features/organizations/lib/api"
import {
  applicationQuery,
  MAX_DECISION_REASON_LENGTH,
  organizerApplicationStatusLabels,
  validateDecisionPayload,
  type OrganizerApplication,
  type OrganizerApplicationFilters,
  type OrganizerApplicationStatus,
} from "@/features/organizations/lib/applications"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"
import { Textarea } from "@/features/shared/components/ui/textarea"
import { toast } from "@/features/shared/components/ui/toast"

function applicationsHref(filters: OrganizerApplicationFilters) {
  return `/admin/organizer-applications?${applicationQuery(filters)}`
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("de-DE", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "Europe/Berlin",
  }).format(new Date(value))
}

function formatAddress(application: OrganizerApplication) {
  const streetLine = [application.street, application.houseNumber]
    .filter(Boolean)
    .join(" ")
  const cityLine = [application.postalCode, application.city]
    .filter(Boolean)
    .join(" ")
  return [streetLine, cityLine, application.countryCode]
    .filter(Boolean)
    .join(", ")
}

const statusFilterOptions: ReadonlyArray<{
  label: string
  status: OrganizerApplicationStatus | undefined
}> = [
  { label: "Alle", status: undefined },
  { label: "Ausstehend", status: "pending" },
  { label: "Angenommen", status: "approved" },
  { label: "Abgelehnt", status: "rejected" },
]

export function OrganizerApplicationsQueue({
  filters,
}: {
  filters: OrganizerApplicationFilters
}) {
  const router = useRouter()
  const query = useOrganizerApplications(filters)
  const mutation = useDecideOrganizerApplication()
  const [selected, setSelected] = useState<OrganizerApplication | null>(null)
  const [action, setAction] = useState<"approve" | "reject">("approve")
  const [dialogOpen, setDialogOpen] = useState(false)
  const [reason, setReason] = useState("")
  const [submitted, setSubmitted] = useState(false)
  const refreshRef = useRef<HTMLButtonElement>(null)

  const data = query.data
  const lastPage = data
    ? Math.max(1, Math.ceil(data.total / filters.limit))
    : filters.page

  const validation = validateDecisionPayload({ reason }, action).error
  const length = Array.from(reason.trim()).length

  useEffect(() => {
    if (data && !query.isFetching && filters.page > lastPage) {
      router.replace(applicationsHref({ ...filters, page: lastPage }), {
        scroll: false,
      })
    }
  }, [data, query.isFetching, filters, lastPage, router])

  function closeDialog(open: boolean) {
    if (!open && !mutation.isPending) {
      setDialogOpen(false)
    }
  }

  function openDecisionDialog(
    application: OrganizerApplication,
    nextAction: "approve" | "reject"
  ) {
    mutation.reset()
    setSelected(application)
    setAction(nextAction)
    setReason("")
    setSubmitted(false)
    setDialogOpen(true)
  }

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitted(true)
    if (!selected || mutation.isPending) return

    const validated = validateDecisionPayload({ reason }, action)
    if (validated.error !== undefined) {
      return
    }

    mutation.mutate(
      {
        applicationId: selected.applicationId,
        action,
        reason: validated.payload.reason,
      },
      {
        onSuccess: (updated) => {
          toast.add({
            type: "success",
            title:
              action === "approve"
                ? "Organisation freigegeben"
                : "Antrag abgelehnt",
            description:
              action === "approve"
                ? `„${updated.organizationName}" (${updated.organizationAlias}) wurde angelegt und die antragstellende Person als Veranstalter eingetragen.`
                : `Der Antrag für „${updated.organizationName}" wurde abgelehnt und die Person benachrichtigt.`,
          })
          setDialogOpen(false)
          router.refresh()
        },
      }
    )
  }

  return (
    <>
      <section
        aria-labelledby="applications-heading"
        className="flex min-w-0 flex-col gap-4"
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <fieldset
            className="flex flex-wrap gap-1"
            aria-label="Antragsstatus filtern"
          >
            {statusFilterOptions.map((option) => (
              <Button
                key={option.label}
                render={
                  <Link
                    href={applicationsHref({
                      ...filters,
                      status: option.status,
                      page: 1,
                    })}
                    scroll={false}
                  />
                }
                nativeButton={false}
                variant={
                  filters.status === option.status ? "secondary" : "ghost"
                }
                className="min-h-11"
                aria-current={
                  filters.status === option.status ? "page" : undefined
                }
              >
                {option.label}
              </Button>
            ))}
          </fieldset>
          <Button
            ref={refreshRef}
            variant="outline"
            className="min-h-11"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCwIcon /> Aktualisieren
          </Button>
        </div>

        <div className="flex items-center justify-between gap-2">
          <h2 id="applications-heading" className="text-base font-medium">
            Anträge{" "}
            {data && (
              <span className="tabular-nums">
                ({data.total.toLocaleString("de-DE")})
              </span>
            )}
          </h2>
          <output className="text-sm text-muted-foreground">
            {query.isFetching ? "Wird geladen…" : "Älteste zuerst"}
          </output>
        </div>

        {query.isError && (
          <div
            className="flex flex-wrap items-center gap-3 rounded-lg border p-4"
            role="alert"
          >
            <p className="text-sm">{query.error.message}</p>
            <Button
              variant="outline"
              className="min-h-11"
              onClick={() => void query.refetch()}
            >
              Erneut versuchen
            </Button>
          </div>
        )}

        {query.isPending && (
          <output className="rounded-lg border p-6 text-sm text-muted-foreground">
            Veranstalteranträge werden geladen…
          </output>
        )}

        {data && data.items.length === 0 && !query.isError && (
          <p className="rounded-lg border p-6 text-sm text-muted-foreground">
            Keine Anträge für diese Auswahl vorhanden.
          </p>
        )}

        {data && data.items.length > 0 && (
          <ul
            className="flex flex-col gap-3"
            aria-label="Anträge auf Organisationserstellung"
            aria-busy={query.isFetching}
          >
            {data.items.map((application) => {
              const address = formatAddress(application)
              return (
                <li
                  key={application.applicationId}
                  className="grid gap-4 rounded-lg border p-4 md:grid-cols-[minmax(0,1fr)_260px] md:gap-6"
                >
                  <div className="flex min-w-0 flex-col gap-3">
                    <div className="flex flex-wrap items-center gap-2 text-sm">
                      <span className="text-base font-semibold break-words">
                        {application.organizationName}
                      </span>
                      <Badge variant="outline">
                        {application.organizationAlias}
                      </Badge>
                      <Badge
                        variant={
                          application.status === "approved"
                            ? "default"
                            : application.status === "rejected"
                              ? "destructive"
                              : "secondary"
                        }
                      >
                        {application.status === "approved" ? (
                          <CheckCircle2Icon />
                        ) : application.status === "rejected" ? (
                          <XCircleIcon />
                        ) : (
                          <ClockIcon />
                        )}
                        {organizerApplicationStatusLabels[application.status]}
                      </Badge>
                      <time
                        className="text-xs text-muted-foreground"
                        dateTime={application.createdAt}
                      >
                        {formatTime(application.createdAt)}
                      </time>
                    </div>

                    <p className="text-sm leading-6 [overflow-wrap:anywhere] break-words whitespace-pre-wrap">
                      {application.message}
                    </p>

                    {application.status !== "pending" && (
                      <div className="rounded-lg bg-muted p-3 text-xs text-muted-foreground">
                        <p className="font-medium text-foreground">
                          {application.status === "approved"
                            ? "Freigegeben"
                            : "Abgelehnt"}
                          {application.decidedBy
                            ? ` durch ${application.decidedBy}`
                            : ""}
                          {application.decidedAt
                            ? ` am ${formatTime(application.decidedAt)}`
                            : ""}
                        </p>
                        {application.decisionReason && (
                          <p className="mt-1 whitespace-pre-wrap">
                            {application.status === "approved"
                              ? `Hinweis: ${application.decisionReason}`
                              : `Begründung: ${application.decisionReason}`}
                          </p>
                        )}
                      </div>
                    )}
                  </div>

                  <div className="flex min-w-0 flex-col items-start gap-2 text-sm">
                    <div className="flex flex-col gap-1 break-words">
                      <span className="font-medium">
                        {application.contactEmail}
                      </span>
                      {application.contactPhoneNumber && (
                        <span className="text-muted-foreground">
                          Tel.: {application.contactPhoneNumber}
                        </span>
                      )}
                      {address && (
                        <span className="text-muted-foreground">{address}</span>
                      )}
                    </div>

                    {application.status === "pending" && (
                      <div className="mt-1 flex flex-wrap gap-2">
                        <Button
                          className="min-h-11"
                          disabled={query.isFetching || query.isError}
                          onClick={() =>
                            openDecisionDialog(application, "approve")
                          }
                        >
                          Freigeben
                        </Button>
                        <Button
                          variant="outline"
                          className="min-h-11"
                          disabled={query.isFetching || query.isError}
                          onClick={() =>
                            openDecisionDialog(application, "reject")
                          }
                        >
                          Ablehnen
                        </Button>
                      </div>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        )}

        {data && data.total > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
            <p className="text-muted-foreground tabular-nums">
              {Math.min(
                (filters.page - 1) * filters.limit + 1,
                data.total
              ).toLocaleString("de-DE")}
              {" – "}
              {Math.min(
                filters.page * filters.limit,
                data.total
              ).toLocaleString("de-DE")}
              {" von "}
              {data.total.toLocaleString("de-DE")}
            </p>
            {lastPage > 1 && (
              <nav
                className="flex items-center gap-2"
                aria-label="Antragsseiten"
              >
                <Button
                  variant="outline"
                  className="min-h-11"
                  disabled={filters.page <= 1 || query.isFetching}
                  onClick={() =>
                    router.push(
                      applicationsHref({ ...filters, page: filters.page - 1 })
                    )
                  }
                >
                  Zurück
                </Button>
                <span className="tabular-nums">
                  {filters.page} / {lastPage}
                </span>
                <Button
                  variant="outline"
                  className="min-h-11"
                  disabled={filters.page >= lastPage || query.isFetching}
                  onClick={() =>
                    router.push(
                      applicationsHref({ ...filters, page: filters.page + 1 })
                    )
                  }
                >
                  Weiter
                </Button>
              </nav>
            )}
          </div>
        )}
      </section>

      <Dialog open={dialogOpen} onOpenChange={closeDialog}>
        <DialogContent
          className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-lg"
          showCloseButton={!mutation.isPending}
          finalFocus={refreshRef}
        >
          <form onSubmit={submit} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>
                {action === "approve"
                  ? "Organisation freigeben"
                  : "Antrag ablehnen"}
              </DialogTitle>
              <DialogDescription>
                {action === "approve"
                  ? "Bei Freigabe wird die Organisation automatisch angelegt, die antragstellende Person als Veranstalter (Organisations-Admin) eingetragen und benachrichtigt."
                  : "Der Antrag wird abgelehnt und die antragstellende Person erhält deine Begründung per Benachrichtigung."}
              </DialogDescription>
            </DialogHeader>

            <div className="rounded-lg bg-muted p-3 text-sm">
              <p className="font-medium">
                {selected?.organizationName}{" "}
                <span className="text-muted-foreground">
                  ({selected?.organizationAlias})
                </span>
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                Kontakt: {selected?.contactEmail}
              </p>
              <p className="mt-2 max-h-40 overflow-y-auto [overflow-wrap:anywhere] break-words whitespace-pre-wrap">
                {selected?.message}
              </p>
            </div>

            <div className="flex flex-col gap-2">
              <label
                htmlFor="application-decision-reason"
                className="text-sm font-medium"
              >
                {action === "approve"
                  ? "Hinweis an Antragsteller (optional)"
                  : "Begründung für die Ablehnung (erforderlich)"}
              </label>
              <Textarea
                id="application-decision-reason"
                value={reason}
                rows={4}
                disabled={mutation.isPending}
                onChange={(event) => setReason(event.target.value)}
                aria-describedby="decision-reason-help decision-reason-count"
                aria-invalid={
                  (submitted && Boolean(validation)) ||
                  length > MAX_DECISION_REASON_LENGTH
                }
              />
              <p
                id="decision-reason-help"
                className="text-sm text-muted-foreground"
              >
                {action === "approve"
                  ? "Wird optional in der Benachrichtigung und im Audit-Log vermerkt. Höchstens 1.000 Zeichen."
                  : "Wird der antragstellenden Person mitgeteilt und im Audit-Log gespeichert. 1–1.000 Zeichen."}
              </p>
              <p
                id="decision-reason-count"
                className="text-right text-xs tabular-nums"
              >
                {length} / {MAX_DECISION_REASON_LENGTH}
              </p>
              {((submitted && validation) ||
                length > MAX_DECISION_REASON_LENGTH) && (
                <p role="alert" className="text-sm text-destructive">
                  {validation}
                </p>
              )}
            </div>

            {mutation.isError && (
              <p role="alert" className="text-sm text-destructive">
                {mutation.error.message}
              </p>
            )}

            <DialogFooter>
              <Button
                type="button"
                className="min-h-11"
                variant="outline"
                disabled={mutation.isPending}
                onClick={() => closeDialog(false)}
              >
                Abbrechen
              </Button>
              <Button
                type="submit"
                className="min-h-11"
                variant={action === "reject" ? "destructive" : "default"}
                disabled={mutation.isPending}
              >
                {mutation.isPending
                  ? "Wird gespeichert…"
                  : action === "approve"
                    ? "Organisation freigeben"
                    : "Antrag ablehnen"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
