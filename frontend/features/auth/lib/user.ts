import { decodeJwt } from "jose"

export type UserRole = "guest" | "visitor" | "organizer"

export type OrganizationRole = "org_admin" | "event_manager" | "finance_viewer"

const ORGANIZATION_ROLES: OrganizationRole[] = [
  "org_admin",
  "event_manager",
  "finance_viewer",
]

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
  /** Roles in the active organization, e.g. event_manager to manage its events. */
  organizationRoles: OrganizationRole[]
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

/**
 * The access token's "organization" claim, keyed by alias, carries each organization's
 * Keycloak ID and the user's groups in it. Not signature-verified: only used to gate UI and
 * to address Keycloak, which re-checks the token itself.
 */
function organizationClaim(
  accessToken: string,
  alias: string
): { id?: unknown; groups?: unknown } | undefined {
  try {
    const claim = decodeJwt(accessToken).organization as
      | Record<string, { id?: unknown; groups?: unknown }>
      | undefined

    return claim?.[alias]
  } catch {
    return undefined
  }
}

/** Keycloak's organization ID for an alias the user is a member of. */
export function organizationIdFromAccessToken(
  accessToken: string,
  alias: string
): string | null {
  const id = organizationClaim(accessToken, alias)?.id

  return typeof id === "string" && id ? id : null
}

/**
 * The user's roles in one organization. Keycloak puts them as groups such as
 * "/event_manager" or "/roles/event_manager" into the organization claim, which
 * is where the API reads them too. Like isAdminFromAccessToken, only for UI.
 */
export function organizationRolesFromAccessToken(
  accessToken: string,
  organization: string | null
): OrganizationRole[] {
  if (!organization) {
    return []
  }

  const groups = organizationClaim(accessToken, organization)?.groups

  if (!Array.isArray(groups)) {
    return []
  }

  return ORGANIZATION_ROLES.filter(
    (role) => groups.includes(`/${role}`) || groups.includes(`/roles/${role}`)
  )
}
