"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { useCreateOrganization } from "@/features/organizations/lib/api";
import {
  type DraftErrors,
  emptyDraft,
  type OrganizationDraft,
  toOrganizationPayload,
  validateDraft,
} from "@/features/organizations/lib/organization";
import slugify from "slugify";
import { Button } from "@/features/shared/components/ui/button";
import { toast } from "@/features/shared/components/ui/toast";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/features/shared/components/ui/field";
import { Input } from "@/features/shared/components/ui/input";

interface CreateOrganizationFormProps {
  defaultOrgAdmin?: string;
}

/**
 * Visual only: screen readers get aria-required on the input instead. Not the
 * native required attribute, which would pre-empt validateDraft's messages.
 */
function RequiredMark() {
  return (
    <span aria-hidden="true" className="text-destructive">
      *
    </span>
  );
}

export function CreateOrganizationForm({
  defaultOrgAdmin = "",
}: CreateOrganizationFormProps) {
  const router = useRouter();
  const [draft, setDraft] = useState<OrganizationDraft>(() => ({
    ...emptyDraft,
    orgAdmin: defaultOrgAdmin,
  }));
  const [errors, setErrors] = useState<DraftErrors>({});
  const [aliasEdited, setAliasEdited] = useState(false);
  const createOrganization = useCreateOrganization();

  function update<K extends keyof OrganizationDraft>(
    key: K,
    value: OrganizationDraft[K],
  ) {
    setDraft((current) => ({ ...current, [key]: value }));
    setErrors((current) => ({ ...current, [key]: undefined }));
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const found = validateDraft(draft);
    setErrors(found);

    if (Object.keys(found).length > 0) {
      return;
    }

    createOrganization.mutate(toOrganizationPayload(draft), {
      onSuccess: (data) => {
        if (data.selfAdmin) {
          toast.add({
            type: "success",
            title: "Organisation erfolgreich erstellt",
            description: `Du verwaltest jetzt ${data.name}.`,
          });
          router.push("/organization");
        } else {
          toast.add({
            title: "Organisation erstellt",
            description: `${data.name} wurde erstellt und ${draft.orgAdmin} zugewiesen.`,
          });
          router.push("/organizations");
        }
      },
    });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5">
      <div className="grid gap-5 md:grid-cols-2">
        <Field data-invalid={Boolean(errors.name)}>
          <FieldLabel htmlFor="organization-name">
            Name
            <RequiredMark />
          </FieldLabel>
          <Input
            id="organization-name"
            aria-required="true"
            value={draft.name}
            placeholder="Fachschaft Informatik"
            autoComplete="off"
            aria-invalid={Boolean(errors.name)}
            onChange={(event) => {
              const value = event.target.value;
              update("name", value);

              if (!aliasEdited) {
                update("alias", slugify(value, { lower: true }));
              }
            }}
          />
          <FieldError>{errors.name}</FieldError>
        </Field>

        <Field data-invalid={Boolean(errors.alias)}>
          <FieldLabel htmlFor="organization-alias">
            Alias
            <RequiredMark />
          </FieldLabel>
          <Input
            id="organization-alias"
            aria-required="true"
            value={draft.alias}
            placeholder="fachschaft-informatik"
            autoComplete="off"
            aria-invalid={Boolean(errors.alias)}
            onChange={(event) => {
              setAliasEdited(true);
              update("alias", event.target.value);
            }}
          />
          <FieldDescription>
            Der dauerhafte Schlüssel für URLs und Tokens. Er kann nach dem
            Erstellen der Organisation nicht mehr geändert werden.
          </FieldDescription>
          <FieldError>{errors.alias}</FieldError>
        </Field>
      </div>

      <Field data-invalid={Boolean(errors.orgAdmin)}>
        <FieldLabel htmlFor="organization-admin">
          Organisations-Admin
          <RequiredMark />
        </FieldLabel>
        <Input
          id="organization-admin"
          aria-required="true"
          value={draft.orgAdmin}
          placeholder="admin@fs-inf.uni-example.de"
          autoComplete="off"
          aria-invalid={Boolean(errors.orgAdmin)}
          onChange={(event) => update("orgAdmin", event.target.value)}
        />
        <FieldDescription>
          Benutzername oder E-Mail-Adresse der Person, die diese Organisation
          verwaltet. Sie muss bereits ein EventHub-Konto haben.
        </FieldDescription>
        <FieldError>{errors.orgAdmin}</FieldError>
      </Field>

      <div className="grid gap-5 md:grid-cols-2">
        <FieldSet className="rounded-xl border border-border p-4">
          <FieldLegend variant="label">Kontaktdaten</FieldLegend>
          <FieldDescription>
            Optional, sichtbar für Mitglieder und Besucher.
          </FieldDescription>

          <Field data-invalid={Boolean(errors.contactEmail)}>
            <FieldLabel htmlFor="organization-contact-email">
              Kontakt-E-Mail
            </FieldLabel>
            <Input
              id="organization-contact-email"
              type="email"
              value={draft.contactEmail}
              placeholder="kontakt@fs-inf.uni-example.de"
              aria-invalid={Boolean(errors.contactEmail)}
              onChange={(event) => update("contactEmail", event.target.value)}
            />
            <FieldError>{errors.contactEmail}</FieldError>
          </Field>

          <Field>
            <FieldLabel htmlFor="organization-contact-phone">
              Kontakt-Telefonnummer
            </FieldLabel>
            <Input
              id="organization-contact-phone"
              type="tel"
              value={draft.contactPhoneNumber}
              placeholder="+49 30 123456"
              onChange={(event) =>
                update("contactPhoneNumber", event.target.value)
              }
            />
          </Field>
        </FieldSet>

        <FieldSet className="rounded-xl border border-border p-4">
          <FieldLegend variant="label">Adresse</FieldLegend>
          <FieldDescription>Optional</FieldDescription>

          <div className="flex gap-3">
            <Field className="min-w-0 flex-1">
              <FieldLabel htmlFor="organization-street">Straße</FieldLabel>
              <Input
                id="organization-street"
                value={draft.street}
                placeholder="Hauptstraße"
                onChange={(event) => update("street", event.target.value)}
              />
            </Field>
            {/* Field forces its children to full width, so the fixed widths
                go on the Field itself, not on the Input. */}
            <Field className="w-20 shrink-0">
              <FieldLabel htmlFor="organization-house-number">
                Hausnr.
              </FieldLabel>
              <Input
                id="organization-house-number"
                value={draft.houseNumber}
                placeholder="1a"
                onChange={(event) => update("houseNumber", event.target.value)}
              />
            </Field>
          </div>

          <div className="flex gap-3">
            <Field className="w-24 shrink-0">
              <FieldLabel htmlFor="organization-postal-code">PLZ</FieldLabel>
              <Input
                id="organization-postal-code"
                value={draft.postalCode}
                placeholder="10115"
                onChange={(event) => update("postalCode", event.target.value)}
              />
            </Field>
            <Field className="min-w-0 flex-1">
              <FieldLabel htmlFor="organization-city">Stadt</FieldLabel>
              <Input
                id="organization-city"
                value={draft.city}
                placeholder="Berlin"
                onChange={(event) => update("city", event.target.value)}
              />
            </Field>
            <Field className="w-16 shrink-0">
              <FieldLabel htmlFor="organization-country-code">Land</FieldLabel>
              <Input
                id="organization-country-code"
                value={draft.countryCode}
                placeholder="DE"
                onChange={(event) => update("countryCode", event.target.value)}
              />
            </Field>
          </div>
        </FieldSet>
      </div>

      {createOrganization.isError ? (
        <p className="text-sm font-medium text-destructive">
          {createOrganization.error.message}
        </p>
      ) : null}

      <div className="flex gap-2">
        <Button type="submit" disabled={createOrganization.isPending}>
          {createOrganization.isPending
            ? "Wird erstellt…"
            : "Organisation erstellen"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={() => {
            setDraft({ ...emptyDraft, orgAdmin: defaultOrgAdmin });
            setErrors({});
            setAliasEdited(false);
            createOrganization.reset();
          }}
        >
          Zurücksetzen
        </Button>
      </div>
    </form>
  );
}
