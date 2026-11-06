"use client"

import { useEffect, useRef, useState } from "react"

import { cn } from "@/features/shared/lib/utils"

interface EventImageProps {
  eventId: string
  alt: string
  /** Shown while the image loads and when the event has none. */
  fallback: React.ReactNode
  className?: string
}

/**
 * EVENTHUB-264: the event's image over its placeholder. The API does not say
 * whether an event has an image, so it is simply requested and only revealed
 * once it loaded; a 404 leaves the placeholder visible.
 */
export function EventImage({
  eventId,
  alt,
  fallback,
  className,
}: EventImageProps) {
  const image = useRef<HTMLImageElement>(null)
  const [loaded, setLoaded] = useState(false)

  // The image can finish before hydration, when onLoad is not attached yet.
  useEffect(() => {
    const current = image.current

    setLoaded(Boolean(current?.complete && current.naturalWidth > 0))
  }, [eventId])

  return (
    <div className={cn("relative overflow-hidden", className)}>
      {fallback}
      {/* Served by our own route, next/image would only add a second fetch. */}
      {/* oxlint-disable-next-line nextjs/no-img-element */}
      <img
        ref={image}
        src={`/api/events/${eventId}/image`}
        alt={loaded ? alt : ""}
        onLoad={() => setLoaded(true)}
        onError={() => setLoaded(false)}
        className={cn(
          "absolute inset-0 size-full object-cover transition-opacity",
          loaded ? "opacity-100" : "opacity-0"
        )}
      />
    </div>
  )
}
