import { requireAdmin } from "@/features/auth"
import { PermissionsTable } from "@/features/permissions"

export default async function PermissionsPage() {
  await requireAdmin()

  return (
    <main className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 px-4 py-6 sm:px-6">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">
          Berechtigungen
        </h1>
        <p className="text-sm text-muted-foreground">
          Globale Rollen für registrierte Benutzer verwalten.
        </p>
      </header>
      <PermissionsTable />
    </main>
  )
}
