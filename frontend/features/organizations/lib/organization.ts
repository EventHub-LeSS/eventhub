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
    errors.name = "A name is required.";
  } else if (draft.name.trim().length > 255) {
    errors.name = "Keep the name under 255 characters.";
  }

  if (!draft.alias.trim()) {
    errors.alias = "An alias is required.";
  } else if (!ALIAS_PATTERN.test(draft.alias)) {
    errors.alias = "Use lowercase letters, digits and single dashes.";
  }

  if (!draft.orgAdmin.trim()) {
    errors.orgAdmin = "An organization admin is required.";
  }

  if (draft.contactEmail && !EMAIL_PATTERN.test(draft.contactEmail)) {
    errors.contactEmail = "Must be a valid email address.";
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
