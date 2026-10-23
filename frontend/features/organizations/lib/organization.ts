export interface OrganizationPayload {
  name: string;
  alias: string;
  orgAdmin: string;
  contactEmail?: string;
  contactPhoneNumber?: string;
  street?: string;
  houseNumber?: string;
  postalCode?: string;
  city?: string;
  countryCode?: string;
}

export interface OrganizationDraft {
  name: string;
  alias: string;
  orgAdmin: string;
  contactEmail: string;
  contactPhoneNumber: string;
  street: string;
  houseNumber: string;
  postalCode: string;
  city: string;
  countryCode: string;
}

export const emptyDraft: OrganizationDraft = {
  name: "",
  alias: "",
  orgAdmin: "",
  contactEmail: "",
  contactPhoneNumber: "",
  street: "",
  houseNumber: "",
  postalCode: "",
  city: "",
  countryCode: "",
};

const ALIAS_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export type DraftErrors = Partial<Record<keyof OrganizationDraft, string>>;

export function validateDraft(draft: OrganizationDraft): DraftErrors {
  const errors: DraftErrors = {};

  if (!draft.name.trim()) {
    errors.name = "Bitte gib einen Namen ein.";
  } else if (draft.name.trim().length > 255) {
    errors.name = "Der Name darf höchstens 255 Zeichen lang sein.";
  }

  if (!draft.alias.trim()) {
    errors.alias = "Bitte gib einen Alias ein.";
  } else if (!ALIAS_PATTERN.test(draft.alias)) {
    errors.alias = "Nur Kleinbuchstaben, Ziffern und einzelne Bindestriche.";
  }

  if (!draft.orgAdmin.trim()) {
    errors.orgAdmin = "Bitte gib einen Organisations-Admin an.";
  }

  if (draft.contactEmail && !EMAIL_PATTERN.test(draft.contactEmail)) {
    errors.contactEmail = "Bitte gib eine gültige E-Mail-Adresse ein.";
  }

  return errors;
}

export function toOrganizationPayload(
  draft: OrganizationDraft,
): OrganizationPayload {
  return {
    name: draft.name.trim(),
    alias: draft.alias.trim(),
    orgAdmin: draft.orgAdmin.trim(),
    ...(draft.contactEmail.trim()
      ? { contactEmail: draft.contactEmail.trim() }
      : {}),
    ...(draft.contactPhoneNumber.trim()
      ? { contactPhoneNumber: draft.contactPhoneNumber.trim() }
      : {}),
    ...(draft.street.trim() ? { street: draft.street.trim() } : {}),
    ...(draft.houseNumber.trim()
      ? { houseNumber: draft.houseNumber.trim() }
      : {}),
    ...(draft.postalCode.trim()
      ? { postalCode: draft.postalCode.trim() }
      : {}),
    ...(draft.city.trim() ? { city: draft.city.trim() } : {}),
    ...(draft.countryCode.trim()
      ? { countryCode: draft.countryCode.trim() }
      : {}),
  };
}
