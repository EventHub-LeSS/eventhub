"use client"

import { useState, useTransition } from "react"

import { publishDraftAction } from "@/features/events/lib/actions"
import { Button } from "@/features/shared/components/ui/button"

export function PublishDraftButton({ eventId }: { eventId: string }) {
  const [message, setMessage] = useState<string>()
  const [isPending, startTransition] = useTransition()

  function handleClick() {
    setMessage(undefined)

    startTransition(async () => {
      const result = await publishDraftAction(eventId)
      setMessage(result.message)
    })
  }

  return (
    <div className="flex flex-col items-end gap-1">
      <Button size="sm" onClick={handleClick} disabled={isPending}>
        Publish
      </Button>
      {message && <p className="text-xs text-destructive">{message}</p>}
    </div>
  )
}
