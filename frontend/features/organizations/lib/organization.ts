export interface OrganizationDomain {
  name: string;
  verified: boolean;
}

export interface OrganizationPayload {
  name: string;
  alias: string;
  description?: string;
  enabled: boolean;
  redirectUrl?: string;
  domains: OrganizationDomain[];
  attributes: Record<string, string[]>;
}

export interface OrganizationDraft {
  name: string;
  alias: string;
  description: string;
  enabled: boolean;
  redirectUrl: string;
  domains: string[];
  logoUrl: string;
  contactEmail: string;
}

export const emptyDraft: OrganizationDraft = {
  name: "",
  alias: "",
  description: "",
  enabled: true,
  redirectUrl: "",
  domains: [],
  logoUrl: "",
  contactEmail: "",
};

const ALIAS_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const DOMAIN_PATTERN =
  /^(?!-)[a-z0-9-]{1,63}(?<!-)(\.(?!-)[a-z0-9-]{1,63}(?<!-))+$/;
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

  if (draft.redirectUrl && !isHttpUrl(draft.redirectUrl)) {
    errors.redirectUrl = "Must be an http(s) URL.";
  }

  if (draft.logoUrl && !isHttpUrl(draft.logoUrl)) {
    errors.logoUrl = "Must be an http(s) URL.";
  }

  if (draft.contactEmail && !EMAIL_PATTERN.test(draft.contactEmail)) {
    errors.contactEmail = "Must be a valid email address.";
  }

  return errors;
}

export function isValidDomain(domain: string): boolean {
  return DOMAIN_PATTERN.test(domain);
}

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

export function toOrganizationPayload(
  draft: OrganizationDraft,
): OrganizationPayload {
  const attributes: Record<string, string[]> = {};

  if (draft.logoUrl.trim()) {
    attributes.logoUrl = [draft.logoUrl.trim()];
  }

  if (draft.contactEmail.trim()) {
    attributes.contactEmail = [draft.contactEmail.trim()];
  }

  return {
    name: draft.name.trim(),
    alias: draft.alias.trim(),
    ...(draft.description.trim()
      ? { description: draft.description.trim() }
      : {}),
    enabled: draft.enabled,
    ...(draft.redirectUrl.trim()
      ? { redirectUrl: draft.redirectUrl.trim() }
      : {}),
    // Domains start unverified; Keycloak flips `verified` once the org's IdP
    // has actually authenticated a user from that domain.
    domains: draft.domains.map((name) => ({ name, verified: false })),
    attributes,
  };
}
