"use client"

import { CalendarIcon, MapPinIcon, SparklesIcon } from "lucide-react"
import Link from "next/link"
import { useEffect, useRef, useState } from "react"

import {
  formatEventDate,
  formatEventPrice,
} from "@/features/catalog/lib/format"
import type { PublishedEvent } from "@/features/catalog/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
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

    section.addEventListener("mouseenter", pause)
    section.addEventListener("mouseleave", resume)
    section.addEventListener("focusin", pause)
    section.addEventListener("focusout", resumeOnFocusLeave)

    return () => {
      section.removeEventListener("mouseenter", pause)
      section.removeEventListener("mouseleave", resume)
      section.removeEventListener("focusin", pause)
      section.removeEventListener("focusout", resumeOnFocusLeave)
    }
  }, [])

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

  return (
    <section
      ref={sectionRef}
      aria-roledescription="carousel"
      aria-labelledby="recommendations-heading"
      className="relative overflow-hidden rounded-2xl bg-muted ring-1 ring-foreground/10"
    >
      <div
        aria-hidden
        className="absolute inset-0 bg-gradient-to-br from-primary/25 via-muted to-muted"
      />
      <div
        aria-hidden
        className="absolute inset-y-0 right-0 hidden w-1/2 items-center justify-center md:flex"
      >
        <span className="font-mono text-sm tracking-widest text-muted-foreground uppercase">
          {event.category.category}
        </span>
      </div>

      <div
        key={event.eventId}
        aria-live={rotating ? "off" : "polite"}
        className="relative flex min-h-64 animate-in flex-col justify-center gap-3 p-6 duration-500 fade-in md:w-1/2 md:p-10"
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
            {event.location.name}, {event.location.city}
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

      {events.length > 1 && (
        <div className="absolute inset-x-0 bottom-4 flex justify-center gap-2">
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
