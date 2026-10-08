"use client"

import { useState, useTransition } from "react"

import {
  saveDraftAction,
  updateDraftAction,
} from "@/features/events/lib/actions"
import {
  type DraftErrors,
  type EventDraft,
  emptyDraft,
  toDraft,
} from "@/features/events/lib/draft"
import type { Event, EventCatalog } from "@/features/events/lib/types"
import { Button } from "@/features/shared/components/ui/button"

const fieldClass =
  "h-9 w-full rounded-lg border border-input bg-transparent px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive"

function Field({
  id,
  label,
  error,
  hint,
  children,
}: {
  id: string
  label: string
  error?: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      {children}
      {hint && !error && (
        <p className="text-xs text-muted-foreground">{hint}</p>
      )}
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}

interface DraftFormProps {
  catalog: EventCatalog
  /** Present when an existing draft is edited instead of a new one created. */
  event?: Event
}

export function DraftForm({ catalog, event }: DraftFormProps) {
  const [draft, setDraft] = useState<EventDraft>(() =>
    event ? toDraft(event) : emptyDraft
  )
  const [errors, setErrors] = useState<DraftErrors>({})
  const [message, setMessage] = useState<string>()
  const [isPending, startTransition] = useTransition()

  function update<K extends keyof EventDraft>(key: K, value: EventDraft[K]) {
    setDraft((current) => ({ ...current, [key]: value }))
    setErrors((current) => ({ ...current, [key]: undefined }))
  }

  function handleSubmit(submitEvent: React.FormEvent<HTMLFormElement>) {
    submitEvent.preventDefault()
    setMessage(undefined)

    startTransition(async () => {
      const result = event
        ? await updateDraftAction(event.eventId, draft)
        : await saveDraftAction(draft)

      setErrors(result.errors ?? {})
      setMessage(result.message)
    })
  }

  const catalogMissing =
    catalog.categories.length === 0 || catalog.locations.length === 0

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Field id="title" label="Title" error={errors.title}>
        <input
          id="title"
          className={fieldClass}
          value={draft.title}
          onChange={(changed) => update("title", changed.target.value)}
          aria-invalid={Boolean(errors.title)}
        />
      </Field>

      <Field
        id="description"
        label="Description"
        error={errors.description}
        hint="Optional."
      >
        <textarea
          id="description"
          rows={4}
          className={`${fieldClass} h-auto py-2`}
          value={draft.description}
          onChange={(changed) => update("description", changed.target.value)}
          aria-invalid={Boolean(errors.description)}
        />
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="startTime" label="Start" error={errors.startTime}>
          <input
            id="startTime"
            type="datetime-local"
            className={fieldClass}
            value={draft.startTime}
            onChange={(changed) => update("startTime", changed.target.value)}
            aria-invalid={Boolean(errors.startTime)}
          />
        </Field>

        <Field id="endTime" label="End" error={errors.endTime}>
          <input
            id="endTime"
            type="datetime-local"
            className={fieldClass}
            value={draft.endTime}
            onChange={(changed) => update("endTime", changed.target.value)}
            aria-invalid={Boolean(errors.endTime)}
          />
        </Field>

        <Field id="capacity" label="Capacity" error={errors.capacity}>
          <input
            id="capacity"
            type="number"
            min={1}
            step={1}
            className={fieldClass}
            value={draft.capacity}
            onChange={(changed) => update("capacity", changed.target.value)}
            aria-invalid={Boolean(errors.capacity)}
          />
        </Field>

        <Field
          id="price"
          label="Price"
          error={errors.price}
          hint="Use 0 for a free event."
        >
          <input
            id="price"
            type="number"
            min={0}
            step="0.01"
            className={fieldClass}
            value={draft.price}
            onChange={(changed) => update("price", changed.target.value)}
            aria-invalid={Boolean(errors.price)}
          />
        </Field>

        <Field id="categoryId" label="Category" error={errors.categoryId}>
          <select
            id="categoryId"
            className={fieldClass}
            value={draft.categoryId}
            onChange={(changed) => update("categoryId", changed.target.value)}
            aria-invalid={Boolean(errors.categoryId)}
          >
            <option value="">Please select</option>
            {catalog.categories.map((category) => (
              <option key={category.categoryId} value={category.categoryId}>
                {category.category}
              </option>
            ))}
          </select>
        </Field>

        <Field id="locationId" label="Location" error={errors.locationId}>
          <select
            id="locationId"
            className={fieldClass}
            value={draft.locationId}
            onChange={(changed) => update("locationId", changed.target.value)}
            aria-invalid={Boolean(errors.locationId)}
          >
            <option value="">Please select</option>
            {catalog.locations.map((location) => (
              <option key={location.locationId} value={location.locationId}>
                {location.name}, {location.city}
              </option>
            ))}
          </select>
        </Field>
      </div>

      {catalogMissing && (
        <p className="text-sm text-muted-foreground">
          No categories or locations are available yet, so the draft cannot be
          saved.
        </p>
      )}

      {message && <p className="text-sm text-destructive">{message}</p>}

      <div>
        <Button type="submit" disabled={isPending || catalogMissing}>
          {event ? "Save changes" : "Save as draft"}
        </Button>
      </div>
    </form>
  )
}
