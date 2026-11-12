"use client"

import {
  CalendarIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  MapPinIcon,
  SparklesIcon,
} from "lucide-react"
import Link from "next/link"
import { useEffect, useRef, useState } from "react"

import { EventImage } from "@/features/catalog/components/event-image"
import {
  formatEventDate,
  formatEventPrice,
} from "@/features/catalog/lib/format"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { formatPlace } from "@/features/shared/lib/location"
import { cn } from "@/features/shared/lib/utils"

const ROTATION_INTERVAL_MS = 6000

interface RecommendationsHeroProps {
  events: PublishedEvent[]
}

/** True when the user asked the OS for less motion; rotation stays off then. */
function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(false)

  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)")
    const update = () => setReduced(query.matches)

    update()
    query.addEventListener("change", update)

    return () => query.removeEventListener("change", update)
  }, [])

  return reduced
}

/**
 * Hero carousel that rotates through the recommended events. It pauses
 * while hovered or focused so a slide never changes under the pointer or
 * while the user reads it with the keyboard.
 */
export function RecommendationsHero({ events }: RecommendationsHeroProps) {
  const [index, setIndex] = useState(0)
  const [paused, setPaused] = useState(false)
  /** Event currently shown without its image; its category becomes the fallback label. */
  const [missingImageId, setMissingImageId] = useState<string | null>(null)
  const sectionRef = useRef<HTMLElement>(null)
  const reducedMotion = usePrefersReducedMotion()
  const rotating = events.length > 1 && !paused && !reducedMotion

  // Native listeners: the section itself is not interactive, so JSX
  // handlers on it would trip jsx-a11y; focusin/focusout bubble from the
  // links and dots inside.
  useEffect(() => {
    const section = sectionRef.current

    if (!section) {
      return
    }

    const pause = () => setPaused(true)
    const resume = () => setPaused(false)
    const resumeOnFocusLeave = (event: FocusEvent) => {
      if (!section.contains(event.relatedTarget as Node | null)) {
        resume()
      }
    }

    const switchWithArrowKeys = (event: KeyboardEvent) => {
      if (
        events.length < 2 ||
        event.altKey ||
        event.ctrlKey ||
        event.metaKey ||
        event.shiftKey
      ) {
        return
      }

      const step =
        event.key === "ArrowLeft" ? -1 : event.key === "ArrowRight" ? 1 : 0

      if (step === 0) {
        return
      }

      event.preventDefault()
      setIndex((current) => (current + step + events.length) % events.length)
    }

    section.addEventListener("mouseenter", pause)
    section.addEventListener("mouseleave", resume)
    section.addEventListener("focusin", pause)
    section.addEventListener("focusout", resumeOnFocusLeave)
    section.addEventListener("keydown", switchWithArrowKeys)

    return () => {
      section.removeEventListener("mouseenter", pause)
      section.removeEventListener("mouseleave", resume)
      section.removeEventListener("focusin", pause)
      section.removeEventListener("focusout", resumeOnFocusLeave)
      section.removeEventListener("keydown", switchWithArrowKeys)
    }
  }, [events.length])

  useEffect(() => {
    if (!rotating) {
      return
    }

    const timer = window.setInterval(
      () => setIndex((current) => (current + 1) % events.length),
      ROTATION_INTERVAL_MS
    )

    return () => window.clearInterval(timer)
  }, [rotating, events.length])

  const event = events[index]
  const hasMultiple = events.length > 1

  const showPrevious = () =>
    setIndex((current) => (current - 1 + events.length) % events.length)
  const showNext = () => setIndex((current) => (current + 1) % events.length)

  return (
    <section
      ref={sectionRef}
      aria-roledescription="carousel"
      aria-labelledby="recommendations-heading"
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- the carousel handles ArrowLeft/ArrowRight
      tabIndex={hasMultiple ? 0 : undefined}
      aria-keyshortcuts={hasMultiple ? "ArrowLeft ArrowRight" : undefined}
      className="relative overflow-hidden rounded-2xl bg-muted ring-1 ring-foreground/10 outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      <div
        aria-hidden
        className="absolute inset-0 bg-linear-to-br from-primary/12 from-0% via-primary/4 via-45% to-transparent to-100%"
      />
      <div
        aria-hidden
        className="absolute inset-y-0 right-0 hidden w-3/5 items-center justify-center md:flex"
      >
        <div className="absolute inset-0 bg-black/15 mask-fade-left" />
        {missingImageId === event.eventId && (
          <span className="relative font-mono text-sm tracking-widest text-muted-foreground uppercase">
            {event.category.category}
          </span>
        )}
        <EventImage
          key={event.eventId}
          eventId={event.eventId}
          sizes="(min-width: 1280px) 768px, 60vw"
          className="animate-in mask-fade-left opacity-90 duration-500 fade-in"
          onMissing={() => setMissingImageId(event.eventId)}
        />
      </div>

      <div
        key={event.eventId}
        aria-live={rotating ? "off" : "polite"}
        className={cn(
          "relative flex min-h-64 animate-in flex-col justify-center gap-3 p-6 duration-500 fade-in md:w-1/2 md:p-10",
          hasMultiple && "pb-18 md:pb-10 md:pl-20"
        )}
      >
        <h2
          id="recommendations-heading"
          className="flex items-center gap-2 text-sm font-medium text-muted-foreground"
        >
          <SparklesIcon className="size-4 shrink-0" />
          Für dich empfohlen
        </h2>
        <Badge variant="secondary" className="w-fit uppercase">
          {event.category.category}
        </Badge>
        <p className="text-2xl font-semibold tracking-tight md:text-3xl">
          {event.title}
        </p>
        <div className="flex flex-col gap-1 text-sm text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <CalendarIcon className="size-3.5 shrink-0" />
            {formatEventDate(event.startTime, event.endTime)}
          </span>
          <span className="flex items-center gap-1.5">
            <MapPinIcon className="size-3.5 shrink-0" />
            {formatPlace(event.location)}
          </span>
        </div>
        <div className="flex items-center gap-4 pt-2">
          <Button
            render={<Link href={`/events/${event.eventId}`} />}
            nativeButton={false}
            size="lg"
          >
            Details ansehen
          </Button>
          <span className="text-sm font-semibold">
            {formatEventPrice(event.price)}
          </span>
        </div>
      </div>

      {hasMultiple && (
        <>
          <Button
            variant="outline"
            size="icon-lg"
            aria-label="Vorheriger Vorschlag"
            onClick={showPrevious}
            className="absolute bottom-4 left-4 size-10 rounded-full bg-background/80 shadow-sm backdrop-blur-sm md:top-[calc(50%-1.25rem)] md:bottom-auto [&_svg:not([class*='size-'])]:size-5"
          >
            <ChevronLeftIcon />
          </Button>
          <Button
            variant="outline"
            size="icon-lg"
            aria-label="Nächster Vorschlag"
            onClick={showNext}
            className="absolute right-4 bottom-4 size-10 rounded-full bg-background/80 shadow-sm backdrop-blur-sm md:top-[calc(50%-1.25rem)] md:bottom-auto [&_svg:not([class*='size-'])]:size-5"
          >
            <ChevronRightIcon />
          </Button>
        </>
      )}

      {hasMultiple && (
        <div className="absolute inset-x-0 bottom-8 flex justify-center gap-2 md:bottom-4">
          {events.map((item, i) => (
            <button
              key={item.eventId}
              type="button"
              aria-label={`Vorschlag ${i + 1}: ${item.title}`}
              aria-current={i === index}
              onClick={() => setIndex(i)}
              className={cn(
                "h-2 rounded-full transition-all focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
                i === index
                  ? "w-6 bg-foreground"
                  : "w-2 bg-foreground/30 hover:bg-foreground/60"
              )}
            />
          ))}
        </div>
      )}
    </section>
  )
}
