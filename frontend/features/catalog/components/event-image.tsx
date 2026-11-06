"use client"

import Image from "next/image"
import { useState } from "react"

import { cn } from "@/features/shared/lib/utils"

interface EventImageProps {
  eventId: string
  sizes: string
  className?: string
}

export function EventImage({ eventId, sizes, className }: EventImageProps) {
  const [failed, setFailed] = useState(false)

  if (failed) {
    return null
  }

  return (
    <Image
      src={`/api/events/${eventId}/image`}
      alt=""
      fill
      sizes={sizes}
      className={cn("object-cover", className)}
      onError={() => setFailed(true)}
    />
  )
}
