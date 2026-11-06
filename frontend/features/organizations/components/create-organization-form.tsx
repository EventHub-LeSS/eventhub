"use client"

import { CheckCircle2Icon, ClockIcon, XCircleIcon } from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useState } from "react"
import slugify from "slugify"

import {
  useMyOrganizerApplications,
  useSubmitOrganizerApplication,
} from "@/features/organizations/lib/api"
import {
  emptyApplicationDraft,
  MAX_APPLICATION_MESSAGE_LENGTH,
  organizerApplicationStatusLabels,
  toApplicationPayload,
  validateApplicationDraft,
  type ApplicationDraftErrors,
  type OrganizerApplication,
  type OrganizerApplicationDraft,
} from "@/features/organizations/lib/applications"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/features/shared/components/ui/field"
import { Input } from "@/features/shared/components/ui/input"
import { Textarea } from "@/features/shared/components/ui/textarea"
import { toast } from "@/features/shared/components/ui/toast"

interface CreateOrganizationFormProps {
  defaultOrgAdmin?: string
}

/**
 * Visual only: screen readers get aria-required on the input instead.
 */
function RequiredMark() {
  return (
    <span aria-hidden="true" className="text-destructive">
      *
    </span>
  )
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat("de-DE", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "Europe/Berlin",
  }).format(new Date(value))
}

function formatAddress(application: OrganizerApplication) {
  const streetLine = [application.street, application.houseNumber]
    .filter(Boolean)
    .join(" ")
  const cityLine = [application.postalCode, application.city]
    .filter(Boolean)
    .join(" ")
  return [streetLine, cityLine, application.countryCode]
    .filter(Boolean)
    .join(", ")
}

export function OwnOrganizerApplicationsList({
  applications,
}: {
  applications: OrganizerApplication[]
}) {
  if (applications.length === 0) {
    return null
  }

  return (
    <section
      aria-labelledby="own-applications-heading"
      className="flex flex-col gap-3"
    >
      <h2 id="own-applications-heading" className="text-base font-semibold">
        Deine eingereichten Anträge ({applications.length})
      </h2>
      <ul
        className="flex flex-col gap-3"
        aria-label="Eingereichte Veranstalteranträge"
      >
        {applications.map((item) => {
          const address = formatAddress(item)
          return (
            <li
              key={item.applicationId}
              className="flex flex-col gap-3 rounded-xl border border-border p-4"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-semibold">{item.organizationName}</span>
                  <span className="text-xs text-muted-foreground">
                    ({item.organizationAlias})
                  </span>
                </div>
                <Badge
                  variant={
                    item.status === "approved"
                      ? "default"
                      : item.status === "rejected"
                        ? "destructive"
                        : "outline"
                  }
                >
                  {item.status === "approved" ? (
                    <CheckCircle2Icon />
                  ) : item.status === "rejected" ? (
                    <XCircleIcon />
                  ) : (
                    <ClockIcon />
                  )}
                  {organizerApplicationStatusLabels[item.status]}
                </Badge>
              </div>
              <p className="text-sm whitespace-pre-wrap text-muted-foreground">
                {item.message}
              </p>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span>Kontakt: {item.contactEmail}</span>
                {item.contactPhoneNumber && (
                  <span>Tel.: {item.contactPhoneNumber}</span>
                )}
                {address && <span>Adresse: {address}</span>}
                <span>
                  Eingereicht am{" "}
                  <time dateTime={item.createdAt}>
                    {formatDateTime(item.createdAt)}
                  </time>
                </span>
              </div>
              {item.status === "pending" && (
                <p className="text-xs text-muted-foreground">
                  Dein Antrag wird aktuell durch einen EventHub-Administrator
                  geprüft. Nach der Freigabe wird die Organisation erstellt und
                  dein Konto als Veranstalter eingetragen.
                </p>
              )}
              {item.status === "approved" && (
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-muted p-3 text-sm">
                  <div className="flex flex-col gap-1">
                    <span className="font-medium">
                      Freigegeben — Organisation wurde erstellt
                    </span>
                    {item.decisionReason && (
                      <span className="text-xs text-muted-foreground">
                        Hinweis: {item.decisionReason}
                      </span>
                    )}
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    render={<Link href="/organization" />}
                    nativeButton={false}
                  >
                    Zur Organisation
                  </Button>
                </div>
              )}
              {item.status === "rejected" && (
                <div className="rounded-lg bg-muted p-3 text-sm">
                  <p className="font-medium">Antrag abgelehnt</p>
                  {item.decisionReason && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      Begründung: {item.decisionReason}
                    </p>
                  )}
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}

export function CreateOrganizationForm({
  defaultOrgAdmin = "",
}: CreateOrganizationFormProps) {
  const router = useRouter()
  const [draft, setDraft] = useState<OrganizerApplicationDraft>(() => ({
    ...emptyApplicationDraft,
    contactEmail: defaultOrgAdmin,
  }))
  const [errors, setErrors] = useState<ApplicationDraftErrors>({})
  const [aliasEdited, setAliasEdited] = useState(false)
  const applicationsQuery = useMyOrganizerApplications()
  const submitApplication = useSubmitOrganizerApplication()

  const applications = applicationsQuery.data ?? []
  const pendingApplication = applications.find(
    (item) => item.status === "pending"
  )
  const messageLength = Array.from(draft.message.trim()).length

  function update<K extends keyof OrganizerApplicationDraft>(
    key: K,
    value: OrganizerApplicationDraft[K]
  ) {
    setDraft((current) => ({ ...current, [key]: value }))
    setErrors((current) => ({ ...current, [key]: undefined }))
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const found = validateApplicationDraft(draft)
    setErrors(found)

    if (Object.keys(found).length > 0) {
      return
    }

    submitApplication.mutate(toApplicationPayload(draft), {
      onSuccess: (created) => {
        toast.add({
          type: "success",
          title: "Antrag erfolgreich eingereicht",
          description: `Deine Anfrage für „${created.organizationName}" wartet nun auf die Freigabe durch einen Administrator.`,
        })
        setDraft({
          ...emptyApplicationDraft,
          contactEmail: defaultOrgAdmin,
        })
        setErrors({})
        setAliasEdited(false)
        router.refresh()
      },
    })
  }

  return (
    <div className="flex flex-col gap-6">
      {pendingApplication && (
        <output className="flex flex-col gap-2 rounded-xl border border-border bg-muted/50 p-4 text-sm">
          <p className="font-semibold">
            Offener Antrag für „{pendingApplication.organizationName}“ (
            {pendingApplication.organizationAlias})
          </p>
          <p className="text-muted-foreground">
            Du hast bereits einen offenen Antrag eingereicht. Sobald ein
            Administrator von EventHub über die Freigabe entschieden hat, wirst
            du benachrichtigt und bei Annahme automatisch als Veranstalter
            eingetragen.
          </p>
        </output>
      )}

      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <div className="grid gap-5 md:grid-cols-2">
          <Field data-invalid={Boolean(errors.organizationName)}>
            <FieldLabel htmlFor="organization-name">
              Name der Organisation
              <RequiredMark />
            </FieldLabel>
            <Input
              id="organization-name"
              aria-required="true"
              disabled={
                Boolean(pendingApplication) || submitApplication.isPending
              }
              value={draft.organizationName}
              placeholder="Stadtfest Bremen e.V."
              autoComplete="off"
              aria-invalid={Boolean(errors.organizationName)}
              onChange={(event) => {
                const value = event.target.value
                update("organizationName", value)

                if (!aliasEdited) {
                  update(
                    "organizationAlias",
                    slugify(value, { lower: true, strict: true })
                  )
                }
              }}
            />
            <FieldError>{errors.organizationName}</FieldError>
          </Field>

          <Field data-invalid={Boolean(errors.organizationAlias)}>
            <FieldLabel htmlFor="organization-alias">
              Alias
              <RequiredMark />
            </FieldLabel>
            <Input
              id="organization-alias"
              aria-required="true"
              disabled={
                Boolean(pendingApplication) || submitApplication.isPending
              }
              value={draft.organizationAlias}
              placeholder="stadtfest-bremen"
              autoComplete="off"
              aria-invalid={Boolean(errors.organizationAlias)}
              onChange={(event) => {
                setAliasEdited(true)
                update("organizationAlias", event.target.value)
              }}
            />
            <FieldDescription>
              3–50 Kleinbuchstaben, Ziffern oder Bindestriche für URLs und
              Tokens.
            </FieldDescription>
            <FieldError>{errors.organizationAlias}</FieldError>
          </Field>
        </div>

        <Field data-invalid={Boolean(errors.message)}>
          <FieldLabel htmlFor="organization-message">
            Vorhaben & geplante Veranstaltungen
            <RequiredMark />
          </FieldLabel>
          <Textarea
            id="organization-message"
            aria-required="true"
            rows={4}
            disabled={
              Boolean(pendingApplication) || submitApplication.isPending
            }
            value={draft.message}
            placeholder="Beschreibe kurz deine Organisation und welche Veranstaltungen ihr auf EventHub veröffentlichen möchtet…"
            aria-invalid={Boolean(errors.message)}
            onChange={(event) => update("message", event.target.value)}
          />
          <div className="flex items-center justify-between gap-2">
            <FieldDescription>
              Mindestens 10 Zeichen — hilft den Administratoren bei der Prüfung
              und Freigabe.
            </FieldDescription>
            <span className="text-xs text-muted-foreground tabular-nums">
              {messageLength} / {MAX_APPLICATION_MESSAGE_LENGTH}
            </span>
          </div>
          <FieldError>{errors.message}</FieldError>
        </Field>

        <div className="grid gap-5 md:grid-cols-2">
          <FieldSet className="rounded-xl border border-border p-4">
            <FieldLegend variant="label">Kontaktdaten</FieldLegend>
            <FieldDescription>
              Für Rückfragen zur Freigabe und als Organisationskontakt.
            </FieldDescription>

            <Field data-invalid={Boolean(errors.contactEmail)}>
              <FieldLabel htmlFor="organization-contact-email">
                Kontakt-E-Mail
                <RequiredMark />
              </FieldLabel>
              <Input
                id="organization-contact-email"
                type="email"
                aria-required="true"
                disabled={
                  Boolean(pendingApplication) || submitApplication.isPending
                }
                value={draft.contactEmail}
                placeholder="kontakt@stadtfest-bremen.de"
                aria-invalid={Boolean(errors.contactEmail)}
                onChange={(event) => update("contactEmail", event.target.value)}
              />
              <FieldError>{errors.contactEmail}</FieldError>
            </Field>

            <Field data-invalid={Boolean(errors.contactPhoneNumber)}>
              <FieldLabel htmlFor="organization-contact-phone">
                Kontakt-Telefonnummer
              </FieldLabel>
              <Input
                id="organization-contact-phone"
                type="tel"
                disabled={
                  Boolean(pendingApplication) || submitApplication.isPending
                }
                value={draft.contactPhoneNumber}
                placeholder="+49 421 123456"
                aria-invalid={Boolean(errors.contactPhoneNumber)}
                onChange={(event) =>
                  update("contactPhoneNumber", event.target.value)
                }
              />
              <FieldError>{errors.contactPhoneNumber}</FieldError>
            </Field>
          </FieldSet>

          <FieldSet className="rounded-xl border border-border p-4">
            <FieldLegend variant="label">Adresse</FieldLegend>
            <FieldDescription>Optional</FieldDescription>

            <div className="flex gap-3">
              <Field
                className="min-w-0 flex-1"
                data-invalid={Boolean(errors.street)}
              >
                <FieldLabel htmlFor="organization-street">Straße</FieldLabel>
                <Input
                  id="organization-street"
                  disabled={
                    Boolean(pendingApplication) || submitApplication.isPending
                  }
                  value={draft.street}
                  placeholder="Marktplatz"
                  aria-invalid={Boolean(errors.street)}
                  onChange={(event) => update("street", event.target.value)}
                />
                <FieldError>{errors.street}</FieldError>
              </Field>
              <Field
                className="w-20 shrink-0"
                data-invalid={Boolean(errors.houseNumber)}
              >
                <FieldLabel htmlFor="organization-house-number">
                  Hausnr.
                </FieldLabel>
                <Input
                  id="organization-house-number"
                  disabled={
                    Boolean(pendingApplication) || submitApplication.isPending
                  }
                  value={draft.houseNumber}
                  placeholder="1"
                  aria-invalid={Boolean(errors.houseNumber)}
                  onChange={(event) =>
                    update("houseNumber", event.target.value)
                  }
                />
                <FieldError>{errors.houseNumber}</FieldError>
              </Field>
            </div>

            <div className="flex gap-3">
              <Field
                className="w-24 shrink-0"
                data-invalid={Boolean(errors.postalCode)}
              >
                <FieldLabel htmlFor="organization-postal-code">PLZ</FieldLabel>
                <Input
                  id="organization-postal-code"
                  disabled={
                    Boolean(pendingApplication) || submitApplication.isPending
                  }
                  value={draft.postalCode}
                  placeholder="28195"
                  aria-invalid={Boolean(errors.postalCode)}
                  onChange={(event) => update("postalCode", event.target.value)}
                />
                <FieldError>{errors.postalCode}</FieldError>
              </Field>
              <Field
                className="min-w-0 flex-1"
                data-invalid={Boolean(errors.city)}
              >
                <FieldLabel htmlFor="organization-city">Stadt</FieldLabel>
                <Input
                  id="organization-city"
                  disabled={
                    Boolean(pendingApplication) || submitApplication.isPending
                  }
                  value={draft.city}
                  placeholder="Bremen"
                  aria-invalid={Boolean(errors.city)}
                  onChange={(event) => update("city", event.target.value)}
                />
                <FieldError>{errors.city}</FieldError>
              </Field>
              <Field
                className="w-16 shrink-0"
                data-invalid={Boolean(errors.countryCode)}
              >
                <FieldLabel htmlFor="organization-country-code">
                  Land
                </FieldLabel>
                <Input
                  id="organization-country-code"
                  disabled={
                    Boolean(pendingApplication) || submitApplication.isPending
                  }
                  value={draft.countryCode}
                  placeholder="DE"
                  aria-invalid={Boolean(errors.countryCode)}
                  onChange={(event) =>
                    update("countryCode", event.target.value)
                  }
                />
                <FieldError>{errors.countryCode}</FieldError>
              </Field>
            </div>
          </FieldSet>
        </div>

        {submitApplication.isError ? (
          <p role="alert" className="text-sm font-medium text-destructive">
            {submitApplication.error.message}
          </p>
        ) : null}

        <div className="flex flex-wrap gap-2">
          <Button
            type="submit"
            disabled={
              Boolean(pendingApplication) || submitApplication.isPending
            }
          >
            {submitApplication.isPending
              ? "Wird eingereicht…"
              : "Antrag absenden"}
          </Button>
          <Button
            type="button"
            variant="ghost"
            disabled={submitApplication.isPending}
            onClick={() => {
              setDraft({
                ...emptyApplicationDraft,
                contactEmail: defaultOrgAdmin,
              })
              setErrors({})
              setAliasEdited(false)
              submitApplication.reset()
            }}
          >
            Zurücksetzen
          </Button>
        </div>
      </form>

      <OwnOrganizerApplicationsList applications={applications} />
    </div>
  )
}
