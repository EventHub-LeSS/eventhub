"use client"

import { StarIcon } from "lucide-react"
import { useState, useTransition } from "react"

import { createEventRatingAction } from "@/features/bookings/lib/actions"
import { MAX_RATING_TEXT_LENGTH } from "@/features/bookings/lib/validation"
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
import { cn } from "@/features/shared/lib/utils"

const SCORES = [1, 2, 3, 4, 5] as const

interface RatingDialogProps {
  eventId: string
  eventTitle: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onRated: (score?: number, message?: string) => void
}

export function RatingDialog({
  eventId,
  eventTitle,
  open,
  onOpenChange,
  onRated,
}: RatingDialogProps) {
  const [score, setScore] = useState(0)
  const [hovered, setHovered] = useState(0)
  const [text, setText] = useState("")
  const [message, setMessage] = useState<string>()
  const [isPending, startTransition] = useTransition()

  const shown = hovered || score
  const textTooLong = text.length > MAX_RATING_TEXT_LENGTH
  const canSubmit = score >= 1 && !textTooLong && !isPending

  function handleOpenChange(next: boolean) {
    if (isPending) {
      return
    }

    if (!next) {
      setMessage(undefined)
    }

    onOpenChange(next)
  }

  function handleSubmit(submitEvent: React.FormEvent<HTMLFormElement>) {
    submitEvent.preventDefault()
    setMessage(undefined)

    if (!canSubmit) {
      return
    }

    startTransition(async () => {
      const result = await createEventRatingAction(eventId, score, text)

      if (result.rated) {
        onRated(result.score, result.message)
        onOpenChange(false)
        return
      }

      setMessage(result.message)
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Veranstaltung beweten</DialogTitle>
            <DialogDescription>
              Wie hat dir {eventTitle} gefallen?
            </DialogDescription>
          </DialogHeader>

          <fieldset className="flex flex-col gap-1.5">
            <legend className="mb-1.5 text-sm font-medium">Sterne</legend>
            <div className="flex gap-1" onMouseLeave={() => setHovered(0)}>
              {SCORES.map((value) => (
                <button
                  key={value}
                  type="button"
                  aria-label={`${value} von 5 Sternen`}
                  aria-pressed={score === value}
                  disabled={isPending}
                  onMouseEnter={() => setHovered(value)}
                  onClick={() => {
                    setMessage(undefined)
                    setScore(value)
                  }}
                  className="rounded-md p-0.5 outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
                >
                  <StarIcon
                    className={cn(
                      "size-7 transition-colors",
                      value <= shown
                        ? "fill-amber-400 text-amber-400"
                        : "text-muted-foreground"
                    )}
                  />
                </button>
              ))}
            </div>
          </fieldset>

          <div className="flex flex-col gap-1.5">
            <label
              htmlFor={`rating-text-${eventId}`}
              className="text-sm font-medium"
            >
              Dein Eindruck{" "}
              <span className="text-muted-foreground">(optional)</span>
            </label>
            <Textarea
              id={`rating-text-${eventId}`}
              value={text}
              onChange={(changed) => {
                setMessage(undefined)
                setText(changed.target.value)
              }}
              aria-invalid={textTooLong}
              disabled={isPending}
              rows={4}
              placeholder="Was war gut, was könnte besser sein?"
            />
            <p
              className={cn(
                "text-right text-xs tabular-nums",
                textTooLong ? "text-destructive" : "text-muted-foreground"
              )}
            >
              {text.length} / {MAX_RATING_TEXT_LENGTH}
            </p>
          </div>

          {message && (
            <p className="text-sm text-destructive" role="alert">
              {message}
            </p>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={isPending}
            >
              Abbrechen
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              {isPending ? "Wird gesendet …" : "Bewertung abgeben"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
