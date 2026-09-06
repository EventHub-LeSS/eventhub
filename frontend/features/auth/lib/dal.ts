import "server-only"
import { redirect } from "next/navigation"
import { cache } from "react"

import { readActiveOrganization } from "@/features/auth/lib/active-organization"
import { readSession, type Session } from "@/features/auth/lib/session"
import { roleFor, type SessionUser } from "@/features/auth/lib/user"

/** Cached per request so several components can ask for the session without re-decrypting it. */
export const getSession = cache(async (): Promise<Session | null> => {
  return readSession()
})

export async function getCurrentUser(): Promise<SessionUser | null> {
  const session = await getSession()

  if (!session) {
    return null
  }

  return {
    name: session.name,
    email: session.email,
    organizations: session.organizations,
    activeOrganization: await readActiveOrganization(session.organizations),
    role: roleFor(session.organizations),
  }
}

export async function requireSession(): Promise<SessionUser> {
  const user = await getCurrentUser()

  if (!user) {
    redirect("/api/auth/login")
  }

  return user
}

export async function requireOrganizer(): Promise<SessionUser> {
  const user = await requireSession()

  if (user.role !== "organizer") {
    redirect("/")
  }

  return user
}
