import { decodeJwt } from "jose"

export type UserRole = "guest" | "visitor" | "organizer"

/** Client ID whose Keycloak client roles carry the platform-wide admin/moderator/visitor roles. */
const BACKEND_CLIENT_ID = "backend"

/** Safe to hand to Client Components: no tokens, no Keycloak internals. */
export interface SessionUser {
  name: string
  email: string
  organizations: string[]
  /** The organization the user is currently acting as, if any. */
  activeOrganization: string | null
  role: UserRole
  /** Platform-wide admin, from the "admin" backend client role. Independent of `role`/organization membership. */
  isAdmin: boolean
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

/**
 * Reads the "admin" backend client role out of the (already Keycloak-issued) access token.
 * Not signature-verified: this is only used to gate UI, the API re-checks on every request.
 */
export function isAdminFromAccessToken(accessToken: string): boolean {
  try {
    const claims = decodeJwt(accessToken)
    const resourceAccess = claims.resource_access as
      | Record<string, { roles?: string[] }>
      | undefined

    return Boolean(resourceAccess?.[BACKEND_CLIENT_ID]?.roles?.includes("admin"))
  } catch {
    return false
  }
}
