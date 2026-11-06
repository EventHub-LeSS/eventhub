import { StarIcon } from "lucide-react"
import Link from "next/link"

import {
  type EventRatingsPage,
  reviewPageHref,
} from "@/features/catalog/lib/event-ratings"
import type { EventRatingsResult } from "@/features/catalog/lib/get-event-ratings"
import { Button } from "@/features/shared/components/ui/button"

const reviewDateFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "medium",
  timeZone: "Europe/Berlin",
})

function formatReviewDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : reviewDateFormat.format(date)
}

function PageLink({
  eventId,
  page,
  disabled,
  children,
}: {
  eventId: string
  page: number
  disabled: boolean
  children: React.ReactNode
}) {
  if (disabled) {
    return (
      <Button variant="outline" className="min-h-11" disabled>
        {children}
      </Button>
    )
  }
  return (
    <Button
      variant="outline"
      className="min-h-11"
      nativeButton={false}
      render={<Link href={reviewPageHref(eventId, page)} scroll={false} />}
    >
      {children}
    </Button>
  )
}

/**
 * 297: public ratings of an event on its detail page, newest first, with
 * "Zurück/Weiter" links that switch the page through `?reviewPage=n`.
 */
export function EventReviews({
  eventId,
  result,
}: {
  eventId: string
  result: EventRatingsResult
}) {
  return (
    <section
      id="bewertungen"
      aria-labelledby="bewertungen-heading"
      className="flex flex-col gap-4"
    >
      <h2 id="bewertungen-heading" className="text-lg font-semibold">
        Bewertungen
      </h2>
      {result.status === "unavailable" ? (
        <p className="text-sm text-muted-foreground">
          Die Bewertungen können aktuell nicht geladen werden.
        </p>
      ) : (
        <ReviewList eventId={eventId} {...result.ratings} />
      )}
    </section>
  )
}

function ReviewList({
  eventId,
  items,
  page,
  limit,
  total,
}: { eventId: string } & EventRatingsPage) {
  if (total === 0) {
    return (
      <p className="text-sm text-muted-foreground">Noch keine Bewertungen.</p>
    )
  }

  const lastPage = Math.max(1, Math.ceil(total / limit))
  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        Auf dieser Seite gibt es keine Bewertungen.{" "}
        <Link
          className="underline underline-offset-4"
          href={reviewPageHref(eventId, 1)}
          scroll={false}
        >
          Zu den neuesten Bewertungen
        </Link>
      </p>
    )
  }

  return (
    <>
      <ul className="divide-y rounded-lg border" aria-label="Bewertungen">
        {items.map((rating) => (
          <li key={rating.ratingId} className="flex flex-col gap-2 p-4">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
              <span className="flex items-center gap-0.5">
                {[0, 1, 2, 3, 4].map((star) => (
                  <StarIcon
                    key={star}
                    aria-hidden="true"
                    className={
                      star < rating.score
                        ? "size-4 fill-amber-500 text-amber-500"
                        : "size-4 text-muted-foreground/40"
                    }
                  />
                ))}
                <span className="sr-only">{rating.score} von 5 Sternen</span>
              </span>
              <span className="font-medium break-words">
                {rating.authorName ?? "Anonym"}
              </span>
              <time
                className="text-muted-foreground"
                dateTime={rating.createdAt}
              >
                {formatReviewDate(rating.createdAt)}
              </time>
            </div>
            {rating.text && (
              <p className="text-sm leading-relaxed [overflow-wrap:anywhere] break-words whitespace-pre-line text-muted-foreground">
                {rating.text}
              </p>
            )}
          </li>
        ))}
      </ul>

      <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <p className="text-muted-foreground tabular-nums">
          {((page - 1) * limit + 1).toLocaleString("de-DE")}
          {" – "}
          {Math.min(page * limit, total).toLocaleString("de-DE")}
          {" von "}
          {total.toLocaleString("de-DE")}
        </p>
        {lastPage > 1 && (
          <nav
            className="flex items-center gap-2"
            aria-label="Bewertungsseiten"
          >
            <PageLink eventId={eventId} page={page - 1} disabled={page <= 1}>
              Zurück
            </PageLink>
            <span className="tabular-nums">
              {page} / {lastPage}
            </span>
            <PageLink
              eventId={eventId}
              page={page + 1}
              disabled={page >= lastPage}
            >
              Weiter
            </PageLink>
          </nav>
        )}
      </div>
    </>
  )
}
