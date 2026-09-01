export type UserRole = "guest" | "visitor" | "organizer"

/** Safe to hand to Client Components: no tokens, no Keycloak internals. */
export interface SessionUser {
  name: string
  email: string
  organizations: string[]
  /** The organization the user is currently acting as, if any. */
  activeOrganization: string | null
  role: UserRole
}

/**
 * Keycloak's "organization" claim is a list of organization aliases, e.g. ["acme-events"].
 * Once the mapper is configured to include organization attributes it becomes an object
 * keyed by alias instead, so both shapes are accepted. Users without a membership have no claim.
 */
export function organizationsFromClaims(claim: unknown): string[] {
  if (Array.isArray(claim)) {
    return claim.filter((alias) => typeof alias === "string")
  }

  if (claim && typeof claim === "object") {
    return Object.keys(claim)
  }

  return []
}

/** Organization membership is what allows a user to create and manage events. */
export function roleFor(organizations: string[]): UserRole {
  return organizations.length > 0 ? "organizer" : "visitor"
}
