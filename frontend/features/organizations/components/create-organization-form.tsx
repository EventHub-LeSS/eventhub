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
      <Field data-invalid={Boolean(errors.name)}>
        <FieldLabel htmlFor="organization-name">Name</FieldLabel>
        <Input
          id="organization-name"
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
        <FieldLabel htmlFor="organization-alias">Alias</FieldLabel>
        <Input
          id="organization-alias"
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

      <Field data-invalid={Boolean(errors.orgAdmin)}>
        <FieldLabel htmlFor="organization-admin">Organisations-Admin</FieldLabel>
        <Input
          id="organization-admin"
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

      <FieldSet className="rounded-xl border border-border p-4">
        <FieldLegend variant="label">Kontaktdaten</FieldLegend>
        <FieldDescription>Optional, sichtbar für Mitglieder und Besucher.</FieldDescription>

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
        <FieldDescription>Optional.</FieldDescription>

        <div className="flex gap-3">
          <Field className="flex-1">
            <FieldLabel htmlFor="organization-street">Straße</FieldLabel>
            <Input
              id="organization-street"
              value={draft.street}
              placeholder="Hauptstraße"
              onChange={(event) => update("street", event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="organization-house-number">
              Hausnr.
            </FieldLabel>
            <Input
              id="organization-house-number"
              value={draft.houseNumber}
              placeholder="1a"
              className="w-24"
              onChange={(event) => update("houseNumber", event.target.value)}
            />
          </Field>
        </div>

        <div className="flex gap-3">
          <Field>
            <FieldLabel htmlFor="organization-postal-code">
              PLZ
            </FieldLabel>
            <Input
              id="organization-postal-code"
              value={draft.postalCode}
              placeholder="10115"
              className="w-32"
              onChange={(event) => update("postalCode", event.target.value)}
            />
          </Field>
          <Field className="flex-1">
            <FieldLabel htmlFor="organization-city">Stadt</FieldLabel>
            <Input
              id="organization-city"
              value={draft.city}
              placeholder="Berlin"
              onChange={(event) => update("city", event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="organization-country-code">
              Land
            </FieldLabel>
            <Input
              id="organization-country-code"
              value={draft.countryCode}
              placeholder="DE"
              className="w-20"
              onChange={(event) => update("countryCode", event.target.value)}
            />
          </Field>
        </div>
      </FieldSet>

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
