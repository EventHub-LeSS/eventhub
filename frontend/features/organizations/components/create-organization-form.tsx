"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { DomainInput } from "@/features/organizations/components/domain-input";
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
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/features/shared/components/ui/field";
import { Input } from "@/features/shared/components/ui/input";
import { Switch } from "@/features/shared/components/ui/switch";
import { Textarea } from "@/features/shared/components/ui/textarea";

export function CreateOrganizationForm() {
  const router = useRouter();
  const [draft, setDraft] = useState<OrganizationDraft>(emptyDraft);
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
      onSuccess: () => {
        router.push("/organizations");
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
              update("alias", slugify(value));
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
          The permanent key used in URLs and tokens. It cannot be changed after
          the organization is created.
        </FieldDescription>
        <FieldError>{errors.alias}</FieldError>
      </Field>

      <Field>
        <FieldLabel htmlFor="organization-description">Description</FieldLabel>
        <Textarea
          id="organization-description"
          value={draft.description}
          placeholder="What this organization does and who runs it."
          onChange={(event) => update("description", event.target.value)}
        />
      </Field>

      <DomainInput
        domains={draft.domains}
        onChange={(domains) => update("domains", domains)}
      />

      <Field data-invalid={Boolean(errors.redirectUrl)}>
        <FieldLabel htmlFor="organization-redirect-url">
          Redirect URL
        </FieldLabel>
        <Input
          id="organization-redirect-url"
          type="url"
          value={draft.redirectUrl}
          placeholder="https://eventhub.example/organizations"
          aria-invalid={Boolean(errors.redirectUrl)}
          onChange={(event) => update("redirectUrl", event.target.value)}
        />
        <FieldDescription>
          Where invited members land after accepting the invitation. Optional.
        </FieldDescription>
        <FieldError>{errors.redirectUrl}</FieldError>
      </Field>

      <FieldSet className="rounded-xl border border-border p-4">
        <FieldLegend variant="label">EventHub details</FieldLegend>
        <FieldDescription>
          Keycloak has no fields for these, so they are stored as organization
          attributes.
        </FieldDescription>

        <Field data-invalid={Boolean(errors.logoUrl)}>
          <FieldLabel htmlFor="organization-logo-url">Logo URL</FieldLabel>
          <Input
            id="organization-logo-url"
            type="url"
            value={draft.logoUrl}
            placeholder="https://example.org/logo.png"
            aria-invalid={Boolean(errors.logoUrl)}
            onChange={(event) => update("logoUrl", event.target.value)}
          />
          <FieldError>{errors.logoUrl}</FieldError>
        </Field>

        <Field data-invalid={Boolean(errors.contactEmail)}>
          <FieldLabel htmlFor="organization-contact-email">
            Contact email
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
      </FieldSet>

      <Field orientation="horizontal">
        <FieldContent>
          <FieldLabel
            id="organization-enabled-label"
            htmlFor="organization-enabled"
          >
            Enabled
          </FieldLabel>
          <FieldDescription>
            Disabled organizations cannot be used to publish events.
          </FieldDescription>
        </FieldContent>
        <Switch
          id="organization-enabled"
          aria-labelledby="organization-enabled-label"
          checked={draft.enabled}
          onCheckedChange={(checked) => update("enabled", checked)}
        />
      </Field>

      {createOrganization.isError ? (
        <p className="text-sm font-medium text-destructive">
          {createOrganization.error.message}
        </p>
      ) : null}

      <div className="flex gap-2">
        <Button type="submit" disabled={createOrganization.isPending}>
          {createOrganization.isPending
            ? "Creating..."
            : "Create organization"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={() => {
            setDraft(emptyDraft);
            setErrors({});
            setAliasEdited(false);
            createOrganization.reset();
          }}
        >
          Reset
        </Button>
      </div>
    </form>
  );
}
