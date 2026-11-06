"use client"

import { ChevronDownIcon, ChevronUpIcon, StarIcon } from "lucide-react"
import Link from "next/link"
import { useMemo, useState } from "react"

import {
  computeReviewStats,
  type DashboardReview,
  filterReviewsByScore,
  STAR_SCORES,
  type StarScore,
} from "@/features/events/lib/dashboard-reviews"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

const INITIAL_VISIBLE = 10
const COLLAPSE_CHARS = 200
const COLLAPSE_LINES = 3

const averageFormat = new Intl.NumberFormat("de-DE", {
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
})

const reviewDateFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "medium",
  timeZone: "Europe/Berlin",
})

function formatReviewDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : reviewDateFormat.format(date)
}

function Stars({ score, className = "size-4" }: { score: number; className?: string }) {
  const filled = Math.round(score)
  return (
    <span className="flex items-center gap-0.5" aria-hidden="true">
      {[1, 2, 3, 4, 5].map((star) => (
        <StarIcon
          key={star}
          className={
            star <= filled
              ? `${className} fill-amber-500 text-amber-500`
              : `${className} text-muted-foreground/40`
          }
        />
      ))}
    </span>
  )
}

function isLongText(text: string) {
  return text.length > COLLAPSE_CHARS || text.split("\n").length > COLLAPSE_LINES
}

function ReviewItem({
  review,
  showEventTitle,
}: {
  review: DashboardReview
  showEventTitle: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const collapsible = isLongText(review.text)

  return (
    <li className="flex flex-col gap-2 py-4 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <Stars score={review.score} />
        <span className="sr-only">{review.score} von 5 Sternen</span>
        <span className="font-medium break-words">
          {review.authorName ?? "Anonym"}
        </span>
        <time className="text-muted-foreground" dateTime={review.createdAt}>
          {formatReviewDate(review.createdAt)}
        </time>
      </div>
      {showEventTitle && (
        <Link
          href={`/organizer/events/${review.eventId}`}
          className="text-xs text-muted-foreground underline-offset-4 hover:underline"
        >
          {review.eventTitle}
        </Link>
      )}
      {review.text ? (
        <>
          <p
            id={`review-text-${review.ratingId}`}
            className={`text-sm leading-relaxed [overflow-wrap:anywhere] whitespace-pre-line ${
              collapsible && !expanded ? "line-clamp-3" : ""
            }`}
          >
            {review.text}
          </p>
          {collapsible && (
            <Button
              variant="ghost"
              size="sm"
              className="self-start"
              aria-expanded={expanded}
              aria-controls={`review-text-${review.ratingId}`}
              onClick={() => setExpanded((value) => !value)}
            >
              {expanded ? <ChevronUpIcon /> : <ChevronDownIcon />}
              {expanded ? "Weniger anzeigen" : "Mehr anzeigen"}
            </Button>
          )}
        </>
      ) : (
        <p className="text-sm text-muted-foreground italic">Ohne Text</p>
      )}
    </li>
  )
}


export function ReviewsDashboard({
  reviews,
  failedEvents,
  description = "Bewertungen der Besucher zu den angezeigten Veranstaltungen",
  showEventTitle = true,
}: {
  reviews: DashboardReview[]
  failedEvents: string[]
  description?: string
  showEventTitle?: boolean
}) {
  const [selected, setSelected] = useState<ReadonlySet<StarScore>>(new Set())
  const [visible, setVisible] = useState(INITIAL_VISIBLE)

  const stats = useMemo(() => computeReviewStats(reviews), [reviews])
  const filtered = useMemo(
    () => filterReviewsByScore(reviews, selected),
    [reviews, selected]
  )

  function toggleScore(score: StarScore) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(score)) {
        next.delete(score)
      } else {
        next.add(score)
      }
      return next
    })
    setVisible(INITIAL_VISIBLE)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Bewertungen</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-6">
        {failedEvents.length > 0 && (
          <p className="text-sm text-destructive">
            Bewertungen konnten nicht für alle Veranstaltungen geladen werden:{" "}
            {failedEvents.join(", ")}
          </p>
        )}

        {stats.total === 0 ? (
          <p className="text-sm text-muted-foreground">Noch keine Bewertungen.</p>
        ) : (
          <>
            <div className="flex flex-col gap-6 sm:flex-row sm:items-start">
              <div className="flex flex-col items-start gap-1 sm:min-w-40">
                <span className="text-4xl font-semibold tabular-nums">
                  {averageFormat.format(stats.average ?? 0)}
                </span>
                <Stars score={stats.average ?? 0} className="size-5" />
                <span className="text-sm text-muted-foreground">
                  Durchschnitt aus {stats.total}{" "}
                  {stats.total === 1 ? "Bewertung" : "Bewertungen"}
                </span>
              </div>

              <fieldset className="flex flex-1 flex-col gap-1.5">
                <legend className="mb-2 text-sm font-medium">
                  Nach Sternen filtern
                </legend>
                {STAR_SCORES.map((score) => {
                  const count = stats.counts[score]
                  const share = stats.total > 0 ? (count / stats.total) * 100 : 0
                  const active = selected.has(score)
                  return (
                    <button
                      key={score}
                      type="button"
                      aria-pressed={active}
                      disabled={count === 0}
                      onClick={() => toggleScore(score)}
                      className={`flex items-center gap-3 rounded-md px-2 py-1 text-sm transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-50 ${
                        active ? "bg-muted ring-1 ring-ring" : ""
                      }`}
                    >
                      <span className="flex w-10 shrink-0 items-center gap-1 tabular-nums">
                        {score}
                        <StarIcon className="size-3.5 fill-amber-500 text-amber-500" aria-hidden="true" />
                      </span>
                      <span className="h-2 flex-1 overflow-hidden rounded-full bg-muted-foreground/15">
                        <span
                          className="block h-full rounded-full bg-amber-500"
                          style={{ width: `${share}%` }}
                        />
                      </span>
                      <span className="w-8 shrink-0 text-right text-muted-foreground tabular-nums">
                        {count}
                      </span>
                    </button>
                  )
                })}
                {selected.size > 0 && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="self-start"
                    onClick={() => {
                      setSelected(new Set())
                      setVisible(INITIAL_VISIBLE)
                    }}
                  >
                    Filter zurücksetzen
                  </Button>
                )}
              </fieldset>
            </div>

            <p className="text-sm text-muted-foreground" aria-live="polite">
              {filtered.length} von {stats.total}{" "}
              {stats.total === 1 ? "Bewertung" : "Bewertungen"}
              {selected.size > 0 &&
                ` · ${[...selected].toSorted((a, b) => b - a).join(", ")} Sterne`}
            </p>

            <ul className="flex flex-col divide-y">
              {filtered.slice(0, visible).map((review) => (
                <ReviewItem
                  key={review.ratingId}
                  review={review}
                  showEventTitle={showEventTitle}
                />
              ))}
            </ul>

            {filtered.length > visible && (
              <Button
                variant="outline"
                className="self-center"
                onClick={() => setVisible((value) => value + INITIAL_VISIBLE)}
              >
                Weitere Bewertungen anzeigen ({filtered.length - visible})
              </Button>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}
