"use client"

import { useRef, useState, useTransition } from "react"

import {
  saveDraftAction,
  updateDraftAction,
} from "@/features/events/lib/actions"
import {
  type DraftErrors,
  type EventDraft,
  emptyDraft,
  toDraft,
  withLocation,
} from "@/features/events/lib/draft"
import {
  IMAGE_CONTENT_TYPES,
  validateImageFile,
} from "@/features/events/lib/image"
import type {
  Event,
  EventCatalog,
  EventLocation,
} from "@/features/events/lib/types"
import { EventImage } from "@/features/shared/components/event-image"
import { Button } from "@/features/shared/components/ui/button"
import { Checkbox } from "@/features/shared/components/ui/checkbox"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/features/shared/components/ui/select"
import { formatAddress } from "@/features/shared/lib/location"

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
  /** The draft was just created, but storing its image failed. */
  imageFailed?: boolean
}

export function DraftForm({ catalog, event, imageFailed }: DraftFormProps) {
  const [draft, setDraft] = useState<EventDraft>(() =>
    event ? toDraft(event, catalog.locations) : emptyDraft
  )
  const [errors, setErrors] = useState<DraftErrors>({})
  const [message, setMessage] = useState<string>()
  const [isPending, startTransition] = useTransition()
  /** EVENTHUB-264: the picked image, sent to the action as a File. */
  const [imageFile, setImageFile] = useState<File>()
  const [image, setImage] = useState<string>()
  const [imageError, setImageError] = useState<string | undefined>(() =>
    imageFailed
      ? "Der Entwurf wurde gespeichert, das Bild aber nicht. Bitte wähle es erneut aus."
      : undefined
  )
  const imageInput = useRef<HTMLInputElement>(null)

  // The API only takes images for drafts and published events.
  const imageAllowed =
    !event || event.status === "draft" || event.status === "published"

  function update<K extends keyof EventDraft>(key: K, value: EventDraft[K]) {
    setDraft((current) => ({ ...current, [key]: value }))
    setErrors((current) => ({ ...current, [key]: undefined }))
  }

  /** Copies a stored address into the fields, which stay editable afterwards. */
  function applyLocation(location: EventLocation) {
    setDraft((current) => withLocation(current, location))
    setErrors((current) => ({
      ...current,
      street: undefined,
      houseNumber: undefined,
      postalCode: undefined,
      city: undefined,
      addressAddition: undefined,
    }))
  }

  function clearImage() {
    setImage(undefined)
    setImageFile(undefined)

    if (imageInput.current) {
      imageInput.current.value = ""
    }
  }

  /** Checks the picked file and reads it as a data URL for the preview. */
  function pickImage(changed: React.ChangeEvent<HTMLInputElement>) {
    const file = changed.target.files?.[0]

    setImage(undefined)
    setImageFile(undefined)
    setImageError(undefined)

    if (!file) {
      return
    }

    const error = validateImageFile(file)

    if (error) {
      setImageError(error)
      clearImage()
      return
    }

    const reader = new FileReader()
    reader.addEventListener("load", () => {
      if (typeof reader.result === "string") {
        setImage(reader.result)
        setImageFile(file)
      }
    })
    reader.addEventListener("error", () => {
      setImageError("Das Bild konnte nicht gelesen werden.")
      clearImage()
    })
    reader.readAsDataURL(file)
  }

  function handleSubmit(submitEvent: React.FormEvent<HTMLFormElement>) {
    submitEvent.preventDefault()
    setMessage(undefined)

    startTransition(async () => {
      const result = event
        ? await updateDraftAction(event.eventId, draft, imageFile)
        : await saveDraftAction(draft, imageFile)

      setErrors(result.errors ?? {})
      setImageError(result.imageError)
      setMessage(result.message)
    })
  }

  const catalogMissing = catalog.categories.length === 0

  // The labels the selects show for their current value, like in the event filters.
  const categoryItems = Object.fromEntries(
    catalog.categories.map((category) => [
      category.categoryId,
      category.category,
    ])
  )
  const locationItems = Object.fromEntries(
    catalog.locations.map((location) => [
      location.locationId,
      location.accessible
        ? `${formatAddress(location)} (barrierefrei)`
        : formatAddress(location),
    ])
  )

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Field id="title" label="Titel" error={errors.title}>
        <input
          id="title"
          className={fieldClass}
          value={draft.title}
          onChange={(changed) => update("title", changed.target.value)}
          aria-invalid={Boolean(errors.title)}
        />
      </Field>

      <Field id="description" label="Beschreibung" error={errors.description}>
        <textarea
          id="description"
          rows={4}
          className={`${fieldClass} h-auto py-2`}
          value={draft.description}
          onChange={(changed) => update("description", changed.target.value)}
          aria-invalid={Boolean(errors.description)}
        />
      </Field>

      {imageAllowed && (
        <Field
          id="image"
          label="Bild"
          error={imageError}
          hint={
            event
              ? "Optional. PNG oder JPEG, höchstens 2 MB. Ein neues Bild ersetzt das bisherige."
              : "Optional. PNG oder JPEG, höchstens 2 MB."
          }
        >
          <input
            ref={imageInput}
            id="image"
            type="file"
            accept={IMAGE_CONTENT_TYPES.join(",")}
            className={`${fieldClass} h-auto py-1.5 file:mr-3 file:rounded-md file:border-0 file:bg-muted file:px-2 file:py-1 file:text-sm file:font-medium`}
            onChange={pickImage}
            aria-invalid={Boolean(imageError)}
          />
          {!image &&
            event?.status === "published" && (
              // The API only serves images of published events, so a draft's
              // stored image cannot be shown yet.
              <EventImage
                eventId={event.eventId}
                alt="Aktuelles Bild der Veranstaltung"
                className="h-48 rounded-lg border"
                fallback={
                  <div className="flex size-full items-center justify-center bg-muted text-sm text-muted-foreground">
                    Noch kein Bild hinterlegt
                  </div>
                }
              />
            )}
          {image && (
            <div className="flex flex-col items-start gap-2">
              {/* A local data URL preview, next/image has nothing to optimize here. */}
              {/* oxlint-disable-next-line nextjs/no-img-element */}
              <img
                src={image}
                alt="Vorschau des gewählten Bildes"
                className="max-h-48 w-full rounded-lg border object-cover"
              />
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={clearImage}
              >
                Bild entfernen
              </Button>
            </div>
          )}
        </Field>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="startTime" label="Beginn" error={errors.startTime}>
          <input
            id="startTime"
            type="datetime-local"
            className={fieldClass}
            value={draft.startTime}
            onChange={(changed) => update("startTime", changed.target.value)}
            aria-invalid={Boolean(errors.startTime)}
          />
        </Field>

        <Field id="endTime" label="Ende" error={errors.endTime}>
          <input
            id="endTime"
            type="datetime-local"
            className={fieldClass}
            value={draft.endTime}
            onChange={(changed) => update("endTime", changed.target.value)}
            aria-invalid={Boolean(errors.endTime)}
          />
        </Field>

        <Field id="capacity" label="Kapazität" error={errors.capacity}>
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
          label="Preis"
          error={errors.price}
          hint="0 für eine kostenlose Veranstaltung."
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

        <Field id="categoryId" label="Kategorie" error={errors.categoryId}>
          <Select
            items={categoryItems}
            value={draft.categoryId || null}
            onValueChange={(value) => update("categoryId", value ?? "")}
          >
            <SelectTrigger
              id="categoryId"
              className="w-full data-[size=default]:h-9"
              aria-invalid={Boolean(errors.categoryId)}
            >
              <SelectValue placeholder="Bitte auswählen" />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              {catalog.categories.map((category) => (
                <SelectItem
                  key={category.categoryId}
                  value={category.categoryId}
                >
                  {category.category}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      </div>

      <fieldset className="flex flex-col gap-4">
        <legend className="mb-4 text-sm font-medium">Veranstaltungsort</legend>

        {catalog.locations.length > 0 && (
          <Field
            id="storedLocation"
            label="Vorhandenen Ort übernehmen"
            hint="Optional. Die Felder unten bleiben danach noch bearbeitbar."
          >
            <Select
              items={locationItems}
              value={null}
              onValueChange={(value) => {
                const location = catalog.locations.find(
                  (candidate) => candidate.locationId === value
                )

                if (location) {
                  applyLocation(location)
                }
              }}
            >
              <SelectTrigger
                id="storedLocation"
                className="w-full data-[size=default]:h-9"
              >
                <SelectValue placeholder="Bitte auswählen" />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                {catalog.locations.map((location) => (
                  <SelectItem
                    key={location.locationId}
                    value={location.locationId}
                  >
                    {locationItems[location.locationId]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        )}

        <div className="grid gap-4 sm:grid-cols-[1fr_8rem]">
          <Field id="street" label="Straße" error={errors.street}>
            <input
              id="street"
              className={fieldClass}
              autoComplete="address-line1"
              value={draft.street}
              onChange={(changed) => update("street", changed.target.value)}
              aria-invalid={Boolean(errors.street)}
            />
          </Field>

          <Field
            id="houseNumber"
            label="Hausnummer"
            error={errors.houseNumber}
            hint="Optional."
          >
            <input
              id="houseNumber"
              className={fieldClass}
              value={draft.houseNumber}
              onChange={(changed) =>
                update("houseNumber", changed.target.value)
              }
              aria-invalid={Boolean(errors.houseNumber)}
            />
          </Field>
        </div>

        <div className="grid gap-4 sm:grid-cols-[8rem_1fr]">
          <Field id="postalCode" label="PLZ" error={errors.postalCode}>
            <input
              id="postalCode"
              inputMode="numeric"
              maxLength={5}
              autoComplete="postal-code"
              className={fieldClass}
              value={draft.postalCode}
              onChange={(changed) => update("postalCode", changed.target.value)}
              aria-invalid={Boolean(errors.postalCode)}
            />
          </Field>

          <Field id="city" label="Ort" error={errors.city}>
            <input
              id="city"
              autoComplete="address-level2"
              className={fieldClass}
              value={draft.city}
              onChange={(changed) => update("city", changed.target.value)}
              aria-invalid={Boolean(errors.city)}
            />
          </Field>
        </div>

        <Field
          id="addressAddition"
          label="Adresszusatz"
          error={errors.addressAddition}
          hint="Optional, z. B. Name der Halle oder Etage."
        >
          <input
            id="addressAddition"
            className={fieldClass}
            value={draft.addressAddition}
            onChange={(changed) =>
              update("addressAddition", changed.target.value)
            }
            aria-invalid={Boolean(errors.addressAddition)}
          />
        </Field>

        <label
          htmlFor="accessible"
          className="flex items-center gap-2 text-sm font-medium"
        >
          <Checkbox
            id="accessible"
            checked={draft.accessible}
            onCheckedChange={(checked) => update("accessible", checked)}
          />
          Barrierefrei
        </label>
      </fieldset>

      {catalogMissing && (
        <p className="text-sm text-muted-foreground">
          Es sind noch keine Kategorien verfügbar, daher kann der Entwurf nicht
          gespeichert werden.
        </p>
      )}

      {message && <p className="text-sm text-destructive">{message}</p>}

      <div>
        <Button type="submit" disabled={isPending || catalogMissing}>
          {event ? "Änderungen speichern" : "Als Entwurf speichern"}
        </Button>
      </div>
    </form>
  )
}
