import "server-only"
import { cookies } from "next/headers"

export const ACTIVE_ORGANIZATION_COOKIE = "eh_active_org"

/**
 * The active organization is a UI preference, never a permission. It is validated
 * against the memberships in the session on every read, so a manipulated cookie
 * cannot select an organization the user does not belong to.
 */
export async function readActiveOrganization(
  organizations: string[]
): Promise<string | null> {
  if (organizations.length === 0) {
    return null
  }

  const store = await cookies()
  const selected = store.get(ACTIVE_ORGANIZATION_COOKIE)?.value

  if (selected && organizations.includes(selected)) {
    return selected
  }

  return organizations[0]
}

export async function setActiveOrganizationCookie(alias: string) {
  const store = await cookies()

  store.set(ACTIVE_ORGANIZATION_COOKIE, alias, {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    maxAge: 60 * 60 * 24 * 30,
  })
}
