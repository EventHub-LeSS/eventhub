import "server-only"
import { getCurrentUser, getSession } from "@/features/auth"
import type { GlobalRole } from "@/features/permissions/lib/roles"
import { envConfig } from "@/features/shared/lib/env"

export async function authorizePermissions() {
  const session = await getSession()
  if (!session) {
    return Response.json({ detail: "Anmeldung erforderlich." }, { status: 401 })
  }
  const user = await getCurrentUser()
  if (!user?.isAdmin) {
    return Response.json(
      { detail: "Nur Administratoren dürfen globale Rollen verwalten." },
      { status: 403 }
    )
  }
  return { accessToken: session.accessToken }
}

export async function requestAdminUsers(
  path: string,
  accessToken: string,
  role?: GlobalRole
) {
  try {
    return await fetch(`${envConfig.apiBaseUrl}/admin/users${path}`, {
      method: role ? "PUT" : "GET",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        ...(role && { "Content-Type": "application/json" }),
      },
      body: role ? JSON.stringify({ roles: [role] }) : undefined,
      cache: "no-store",
    })
  } catch {
    return Response.json(
      { detail: "Die Benutzerverwaltung ist gerade nicht erreichbar." },
      { status: 502 }
    )
  }
}
