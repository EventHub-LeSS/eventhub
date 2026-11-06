import {
  authorizePermissions,
  isGlobalRole,
  requestAdminUsers,
} from "@/features/permissions"

export async function PUT(
  request: Request,
  { params }: { params: Promise<{ userId: string }> }
) {
  const authorization = await authorizePermissions()
  if (authorization instanceof Response) return authorization

  const userId = (await params).userId.toLowerCase()
  if (!/^[a-f\d]{8}(?:-[a-f\d]{4}){3}-[a-f\d]{12}$/.test(userId)) {
    return Response.json({ detail: "Ungültige Benutzer-ID." }, { status: 400 })
  }

  const body: unknown = await request.json().catch(() => null)
  const roles =
    body && typeof body === "object" && "roles" in body ? body.roles : null
  if (!Array.isArray(roles) || roles.length !== 1 || !isGlobalRole(roles[0])) {
    return Response.json(
      { detail: "Wähle genau eine gültige Rolle aus." },
      { status: 400 }
    )
  }

  const role = roles[0]
  const path = `/${userId}/roles`
  const saved = await requestAdminUsers(path, authorization.accessToken, role)
  if (!saved.ok) {
    const message = {
      401: "Deine Anmeldung ist abgelaufen. Bitte melde dich erneut an.",
      403: "Du hast keine Berechtigung, die Rolle zu ändern.",
      404: "Dieses Benutzerkonto wurde nicht gefunden.",
      409: "Dem letzten globalen Admin kann die Admin-Rolle nicht entzogen werden.",
    }[saved.status as 401 | 403 | 404 | 409]
    return Response.json(
      { detail: message ?? "Die Rolle konnte nicht gespeichert werden." },
      { status: saved.status }
    )
  }

  const checked = await requestAdminUsers(path, authorization.accessToken)
  const current: unknown = checked.ok
    ? await checked.json().catch(() => null)
    : null
  if (
    !current ||
    typeof current !== "object" ||
    !("userId" in current) ||
    current.userId !== userId ||
    !("roles" in current) ||
    !Array.isArray(current.roles) ||
    current.roles.length !== 1 ||
    current.roles[0] !== role
  ) {
    return Response.json(
      { detail: "Die gespeicherte Rolle konnte nicht bestätigt werden." },
      { status: 502 }
    )
  }

  return Response.json({ roles: [role] })
}
