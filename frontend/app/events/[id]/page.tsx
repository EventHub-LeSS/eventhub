import { ArrowLeftIcon, StarIcon } from "lucide-react";
import Link from "next/link";
import { notFound } from "next/navigation";

import { getCurrentUser } from "@/features/auth";
import { BookingForm, formatAmount } from "@/features/bookings";
import {
  getEventRatingSummary,
  getOrganizerRatingSummary,
  getPublishedEventDetails,
} from "@/features/catalog";
import { Button } from "@/features/shared/components/ui/button";

interface PageProps {
  params: Promise<{ id: string }>;
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
 * point of the booking flow (EVENTHUB-235): the amount to pay is calculated
 * from the ticket count and the price here, before anything is reserved.
 * The organizer is shown with its average rating (EVENTHUB-239), the footer
 * shows the event's public rating summary (EVENTHUB-238).
 */
export default async function EventDetailPage({ params }: PageProps) {
  const { id } = await params;
  const [result, user, rating] = await Promise.all([
    getPublishedEventDetails(id),
    getCurrentUser(),
    getEventRatingSummary(id),
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
  const street = [location.street, location.houseNumber]
    .filter(Boolean)
    .join(" ");
  const address = [
    [location.name, street].filter(Boolean).join(", "),
    `${location.postalCode} ${location.city}`,
  ].join(", ");

  // The rating summary is non-critical: without it only the reminder note
  // is shown, the rest of the page works regardless.
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
            <span className="text-xs font-bold tracking-wider text-purple-700 uppercase">
              {category.category}
            </span>
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

          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            {ratingSummary && (
              <>
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
                <div className="hidden h-9 w-px bg-border sm:block" />
              </>
            )}
            <p className="text-sm text-muted-foreground">
              Du erhältst eine Erinnerung vor Veranstaltungsbeginn
            </p>
          </div>
        </div>

        <div className="flex flex-col gap-2">
          <BookingForm
            eventId={event.eventId}
            price={event.price}
            availableSeats={event.availableSeats}
            capacity={event.capacity}
            bookable={event.bookable}
            isAuthenticated={Boolean(user)}
          />
        </div>
      </div>
    </div>
  );
}
