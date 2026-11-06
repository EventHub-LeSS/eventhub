import { ArrowLeftIcon, StarIcon } from "lucide-react";
import Link from "next/link";
import { notFound } from "next/navigation";

import { getSession } from "@/features/auth";
import { AddToCartForm, formatAmount } from "@/features/cart";
import {
  EventReviews,
  getEventRatings,
  getEventRatingSummary,
  getOrganizerRatingSummary,
  getPublishedEventDetails,
  parseReviewPage,
} from "@/features/catalog";
import { Button } from "@/features/shared/components/ui/button";
import { formatAddress } from "@/features/shared/lib/location";

interface PageProps {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}

const dateFormat = new Intl.DateTimeFormat("de-DE", {
  dateStyle: "full",
  timeStyle: "short",
  timeZone: "Europe/Berlin",
});

const ratingFormat = new Intl.NumberFormat("de-DE", {
  maximumFractionDigits: 1,
});

/** Full-width rows of the event details list: bold key, muted value. */
function DetailRow({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <p className="text-sm">
      <span className="font-semibold">{label}:</span>{" "}
      <span className="text-muted-foreground">{children}</span>
    </p>
  );
}

/**
 * Public detail page of a published event (EVENTHUB-211/212) and the entry
 * point of the cart flow (282): tickets are reserved into the cart here,
 * the amount of this addition is calculated from ticket count and price.
 * The organizer is shown with its average rating (EVENTHUB-239), the
 * event's public rating summary (EVENTHUB-238) sits next to the category
 * above the title, and its ratings are listed below the description (297),
 * paged through `?reviewPage=n`.
 */
export default async function EventDetailPage({
  params,
  searchParams,
}: PageProps) {
  const [{ id }, { reviewPage }] = await Promise.all([params, searchParams]);
  const [result, session, rating, reviews] = await Promise.all([
    getPublishedEventDetails(id),
    getSession(),
    getEventRatingSummary(id),
    getEventRatings(id, parseReviewPage(reviewPage)),
  ]);

  if (result.status === "not-found") {
    notFound();
  }

  if (result.status === "unavailable") {
    return (
      <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-4 p-6">
        <Button
          render={<Link href="/" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Zurück zur Übersicht
        </Button>
        <p className="text-sm text-muted-foreground">
          Diese Veranstaltung kann aktuell nicht geladen werden. Bitte versuche
          es später erneut.
        </p>
      </div>
    );
  }

  const { event } = result;
  const { location, category } = event;
  const organizerRating = await getOrganizerRatingSummary(event.organizerId);
  const organizerAverage =
    organizerRating.status === "ok"
      ? organizerRating.summary.averageRating
      : null;
  const address = location.accessible
    ? `${formatAddress(location)} (barrierefrei)`
    : formatAddress(location);

  // The rating summary is non-critical: without it only the category is
  // shown above the title, the rest of the page works regardless.
  const ratingSummary = rating.status === "ok" ? rating.summary : null;
  const averageRating = ratingSummary?.averageRating ?? null;
  const ratingCount = ratingSummary?.ratingCount ?? 0;
  const hasRating =
    ratingSummary !== null && averageRating !== null && ratingCount > 0;
  const filledStars = hasRating ? Math.round(averageRating) : 0;
  const ratingLabel = hasRating
    ? `${ratingFormat.format(averageRating)} von 5 · ${ratingCount} ${
        ratingCount === 1 ? "Bewertung" : "Bewertungen"
      }`
    : "Noch keine Bewertungen";

  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div>
        <Button
          render={<Link href="/" />}
          nativeButton={false}
          variant="ghost"
          size="sm"
        >
          <ArrowLeftIcon />
          Zurück zur Übersicht
        </Button>
      </div>

      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[minmax(0,11fr)_minmax(0,9fr)]">
        <div className="flex min-w-0 flex-col gap-6">
          {/* Image placeholder: until real pictures exist, a light purple
              banner marks where the event image will appear. */}
          <div
            className="relative aspect-video overflow-hidden rounded-xl"
            style={{
              backgroundImage:
                "repeating-linear-gradient(45deg, #f5f3ff 0 22px, #ede9fe 22px 44px)",
            }}
          >
            <div className="absolute inset-0 flex items-center justify-center p-4">
              <span className="rounded-full border border-purple-200 bg-white/85 px-4 py-1.5 text-sm font-medium text-muted-foreground">
                Bild: {category.category}-Veranstaltung
              </span>
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <span className="text-xs font-bold tracking-wider text-purple-700 uppercase">
                {category.category}
              </span>
              {ratingSummary && (
                <div className="flex items-center gap-2">
                  <span
                    className="flex items-center gap-0.5"
                    aria-hidden="true"
                  >
                    {[0, 1, 2, 3, 4].map((star) => (
                      <StarIcon
                        key={star}
                        className={
                          star < filledStars
                            ? "size-4 fill-amber-500 text-amber-500"
                            : "size-4 text-muted-foreground/40"
                        }
                      />
                    ))}
                  </span>
                  <span className="text-sm text-muted-foreground">
                    {ratingLabel}
                  </span>
                </div>
              )}
            </div>
            <h1 className="text-3xl font-extrabold tracking-tight break-words text-foreground">
              {event.title}
            </h1>
          </div>

          <div className="flex flex-col gap-2">
            <DetailRow label="Veranstalter">
              {event.organizerAlias}
              {organizerAverage !== null && (
                <span
                  className="ml-2 inline-flex items-center gap-1 align-middle font-medium text-foreground"
                  title="Durchschnittliche Bewertung des Veranstalters"
                >
                  <StarIcon
                    className="size-4 fill-amber-500 text-amber-500"
                    aria-hidden="true"
                  />
                  <span className="sr-only">Bewertung:</span>
                  {ratingFormat.format(organizerAverage)}
                </span>
              )}
            </DetailRow>
            <DetailRow label="Zeitpunkt">
              {dateFormat.format(new Date(event.startTime))} –{" "}
              {dateFormat.format(new Date(event.endTime))}
            </DetailRow>
            <DetailRow label="Ort">{address}</DetailRow>
            <DetailRow label="Preis">
              {formatAmount(event.price)} pro Ticket
            </DetailRow>
          </div>

          <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
            {event.description ??
              "Für diese Veranstaltung liegt noch keine Beschreibung vor."}
          </p>

          <hr className="border-border" />

          <p className="text-sm text-muted-foreground">
            Du erhältst eine Erinnerung vor Veranstaltungsbeginn
          </p>

          <hr className="border-border" />

          <EventReviews eventId={event.eventId} result={reviews} />
        </div>

        <div className="flex flex-col gap-2">
          <AddToCartForm
            eventId={event.eventId}
            price={event.price}
            availableSeats={event.availableSeats}
            capacity={event.capacity}
            bookable={event.bookable}
            isAuthenticated={Boolean(session)}
          />
        </div>
      </div>
    </div>
  );
}
