import Link from "next/link"

import { requireApplicationReviewer } from "@/features/auth"
import {
  OrganizerApplicationsQueue,
  parseApplicationFilters,
} from "@/features/organizations"

export default async function OrganizerApplicationsAdminPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
  await requireApplicationReviewer()
  const params = await searchParams
  const query = new URLSearchParams()
  for (const key of ["status", "page", "limit"]) {
    const value = params[key]
    if (typeof value === "string") {
      query.set(key, value)
    }
  }
  const result = parseApplicationFilters(query)

  return (
    <main className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 px-4 py-6 sm:px-6">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold">
          Anträge auf Organisationserstellung
        </h1>
        <p className="text-sm text-muted-foreground">
          Kontaktformulare zur Gründung neuer Organisationen prüfen, freigeben
          oder begründet ablehnen. Bei Freigabe wird die Organisation erstellt
          und die antragstellende Person automatisch als Veranstalter
          eingetragen.
        </p>
      </header>
      {result.error !== undefined ? (
        <div className="flex flex-col gap-2" role="alert">
          <p>{result.error}</p>
          <Link
            className="underline underline-offset-4"
            href="/admin/organizer-applications"
          >
            Filter zurücksetzen
          </Link>
        </div>
      ) : (
        <OrganizerApplicationsQueue filters={result.filters} />
      )}
    </main>
  )
}
