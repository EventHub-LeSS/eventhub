"use client"

import { EyeIcon, EyeOffIcon, RefreshCwIcon } from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useEffect, useRef, useState } from "react"

import {
  useModeratedRatings,
  useRatingVisibility,
} from "@/features/moderation/lib/api"
import type {
  ModeratedRating,
  ModerationFilters,
} from "@/features/moderation/lib/types"
import {
  MAX_REASON_LENGTH,
  moderationQuery,
  reasonError,
} from "@/features/moderation/lib/validation"
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

function moderationHref(filters: ModerationFilters) {
  return `/admin/moderation?${moderationQuery(filters)}`
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("de-DE", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "Europe/Berlin",
  }).format(new Date(value))
}

export function ModerationQueue({ filters }: { filters: ModerationFilters }) {
  const router = useRouter()
  const query = useModeratedRatings(filters)
  const mutation = useRatingVisibility()
  const [selected, setSelected] = useState<ModeratedRating | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [reason, setReason] = useState("")
  const [submitted, setSubmitted] = useState(false)
  const refreshRef = useRef<HTMLButtonElement>(null)
  const data = query.data
  const lastPage = data
    ? Math.max(1, Math.ceil(data.total / filters.limit))
    : filters.page
  const validation = reasonError(reason)
  const length = Array.from(reason.trim()).length

  useEffect(() => {
    if (data && !query.isFetching && filters.page > lastPage) {
      router.replace(moderationHref({ ...filters, page: lastPage }), {
        scroll: false,
      })
    }
  }, [data, query.isFetching, filters, lastPage, router])

  function closeDialog(open: boolean) {
    if (!open && !mutation.isPending) setDialogOpen(false)
  }

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitted(true)
    if (!selected || validation || mutation.isPending) return
    mutation.mutate(
      {
        ratingId: selected.ratingId,
        hidden: !selected.hidden,
        reason: reason.trim(),
      },
      {
        onSuccess: (result) => {
          toast.add({
            type: "success",
            title: result.changed
              ? result.hidden
                ? "Bewertung ausgeblendet"
                : "Bewertung wieder eingeblendet"
              : "Sichtbarkeit bereits aktuell",
            description: result.changed
              ? "Die Änderung und deine Begründung wurden protokolliert."
              : "Es war keine weitere Änderung erforderlich.",
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
        aria-labelledby="ratings-heading"
        className="flex min-w-0 flex-col gap-4"
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <fieldset
            className="flex flex-wrap gap-1"
            aria-label="Sichtbarkeit filtern"
          >
            {(
              [
                { label: "Alle", hidden: undefined },
                { label: "Sichtbar", hidden: false },
                { label: "Ausgeblendet", hidden: true },
              ] as const
            ).map((filter) => (
              <Button
                key={filter.label}
                render={
                  <Link
                    href={moderationHref({
                      ...filters,
                      hidden: filter.hidden,
                      page: 1,
                    })}
                    scroll={false}
                  />
                }
                nativeButton={false}
                variant={
                  filters.hidden === filter.hidden ? "secondary" : "ghost"
                }
                className="min-h-11"
                aria-current={
                  filters.hidden === filter.hidden ? "page" : undefined
                }
              >
                {filter.label}
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

        {filters.eventId && (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="break-all">
              Veranstaltung:{" "}
              {data?.items.find((item) => item.eventTitle)?.eventTitle ??
                filters.eventId}
            </span>
            <Button
              variant="ghost"
              className="min-h-11"
              render={
                <Link
                  href={moderationHref({
                    ...filters,
                    eventId: undefined,
                    page: 1,
                  })}
                />
              }
              nativeButton={false}
            >
              Veranstaltungsfilter entfernen
            </Button>
          </div>
        )}

        <div className="flex items-center justify-between gap-2">
          <h2 id="ratings-heading" className="text-base font-medium">
            Bewertungen{" "}
            {data && (
              <span className="tabular-nums">
                ({data.total.toLocaleString("de-DE")})
              </span>
            )}
          </h2>
          <output className="text-sm text-muted-foreground">
            {query.isFetching ? "Wird geladen…" : "Neueste zuerst"}
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
          <div
            className="flex flex-col gap-4 rounded-lg border p-4"
            aria-label="Bewertungen werden geladen"
          >
            {[1, 2, 3].map((row) => (
              <div
                key={row}
                className="flex flex-col gap-3 py-3"
                aria-hidden="true"
              >
                <div className="h-4 w-32 rounded bg-muted" />
                <div className="h-5 w-3/4 rounded bg-muted" />
                <div className="h-4 w-1/2 rounded bg-muted" />
              </div>
            ))}
          </div>
        )}
        {data && data.items.length === 0 && !query.isError && (
          <p className="rounded-lg border p-6 text-sm text-muted-foreground">
            Keine Bewertungen für diese Auswahl vorhanden.
          </p>
        )}
        {data && data.items.length > 0 && (
          <ul
            className="divide-y rounded-lg border"
            aria-label="Bewertungen zur Moderation"
            aria-busy={query.isFetching}
          >
            {data.items.map((rating) => (
              <li
                key={rating.ratingId}
                className="grid gap-4 p-4 md:grid-cols-[minmax(0,1fr)_240px] md:gap-6"
              >
                <div className="flex min-w-0 flex-col gap-3">
                  <div className="flex flex-wrap items-center gap-3 text-sm">
                    <span className="font-medium tabular-nums">
                      {rating.score} / 5 Sterne
                    </span>
                    <Badge variant="outline">
                      {rating.hidden ? <EyeOffIcon /> : <EyeIcon />}
                      {rating.hidden ? "Ausgeblendet" : "Sichtbar"}
                    </Badge>
                    <time
                      className="text-xs text-muted-foreground"
                      dateTime={rating.createdAt}
                    >
                      {formatTime(rating.createdAt)}
                    </time>
                  </div>
                  <p className="text-sm leading-6 [overflow-wrap:anywhere] break-words whitespace-pre-wrap">
                    {rating.text || (
                      <span className="text-muted-foreground">
                        Kein Kommentartext vorhanden.
                      </span>
                    )}
                  </p>
                </div>
                <div className="flex min-w-0 flex-col items-start gap-2">
                  <div className="flex flex-col gap-1 text-sm break-words">
                    <span className="font-medium">
                      {rating.eventTitle ??
                        "Veranstaltung nicht mehr zugeordnet"}
                    </span>
                    <span className="text-muted-foreground">
                      {rating.organizationName ??
                        "Organisation nicht mehr zugeordnet"}
                    </span>
                  </div>
                  {rating.eventId && (
                    <Link
                      className="inline-flex min-h-11 items-center text-sm underline underline-offset-4"
                      href={moderationHref({
                        ...filters,
                        eventId: rating.eventId,
                        page: 1,
                      })}
                    >
                      Bewertungen dieser Veranstaltung
                    </Link>
                  )}
                  <Button
                    variant="outline"
                    className="min-h-11"
                    disabled={query.isFetching || query.isError}
                    onClick={() => {
                      mutation.reset()
                      setReason("")
                      setSubmitted(false)
                      setSelected(rating)
                      setDialogOpen(true)
                    }}
                  >
                    {rating.hidden ? <EyeIcon /> : <EyeOffIcon />}
                    {rating.hidden ? "Wieder einblenden" : "Ausblenden"}
                  </Button>
                </div>
              </li>
            ))}
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
                aria-label="Bewertungsseiten"
              >
                <Button
                  variant="outline"
                  className="min-h-11"
                  disabled={filters.page <= 1 || query.isFetching}
                  onClick={() =>
                    router.push(
                      moderationHref({ ...filters, page: filters.page - 1 })
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
                      moderationHref({ ...filters, page: filters.page + 1 })
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
                {selected?.hidden
                  ? "Bewertung wieder einblenden"
                  : "Bewertung ausblenden"}
              </DialogTitle>
              <DialogDescription>
                {selected?.hidden
                  ? "Die Bewertung wird wieder für öffentliche Bewertungszusammenfassungen und Empfehlungen berücksichtigt, sofern sie die übrigen Voraussetzungen erfüllt."
                  : "Die Bewertung wird nicht mehr in öffentlichen Bewertungszusammenfassungen und Empfehlungen berücksichtigt."}
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-lg bg-muted p-3 text-sm">
              <p className="font-medium">
                {selected?.eventTitle ?? "Ohne Veranstaltungszuordnung"}
              </p>
              <p className="mt-2 max-h-40 overflow-y-auto [overflow-wrap:anywhere] break-words whitespace-pre-wrap">
                {selected?.text || "Kein Kommentartext vorhanden."}
              </p>
            </div>
            <div className="flex flex-col gap-2">
              <label
                htmlFor="moderation-reason"
                className="text-sm font-medium"
              >
                Begründung (erforderlich)
              </label>
              <Textarea
                id="moderation-reason"
                value={reason}
                rows={4}
                disabled={mutation.isPending}
                onChange={(event) => setReason(event.target.value)}
                aria-describedby="reason-help reason-count"
                aria-invalid={
                  (submitted && Boolean(validation)) ||
                  length > MAX_REASON_LENGTH
                }
              />
              <p id="reason-help" className="text-sm text-muted-foreground">
                Wird mit deinem Benutzerkonto im Audit-Log gespeichert. 1–1.000
                Zeichen.
              </p>
              <p id="reason-count" className="text-right text-xs tabular-nums">
                {length} / {MAX_REASON_LENGTH}
              </p>
              {((submitted && validation) || length > MAX_REASON_LENGTH) && (
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
                disabled={mutation.isPending || length > MAX_REASON_LENGTH}
              >
                {mutation.isPending
                  ? "Wird gespeichert…"
                  : selected?.hidden
                    ? "Wieder einblenden"
                    : "Ausblenden"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
