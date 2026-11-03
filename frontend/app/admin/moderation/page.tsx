import Link from "next/link"

import { requireAdmin } from "@/features/auth"
import { ModerationQueue, parseModerationFilters } from "@/features/moderation"

export default async function ModerationPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
  await requireAdmin()
  const params = await searchParams
  const query = new URLSearchParams()
  for (const key of ["hidden", "eventId", "page", "limit"]) {
    const value = params[key]
    if (typeof value === "string") query.set(key, value)
  }
  const result = parseModerationFilters(query)

  return (
    <main className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 px-4 py-6 sm:px-6">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold">Inhalte moderieren</h1>
        <p className="text-sm text-muted-foreground">
          Bewertungen und Kommentare aller Organisationen prüfen, ausblenden
          oder wieder freigeben.
        </p>
        {params.organization && (
          <output className="text-sm text-muted-foreground">
            Die Moderation gilt plattformweit. Ein Organisationsfilter wird
            derzeit nicht unterstützt.
          </output>
        )}
      </header>
      {result.error !== undefined ? (
        <div className="flex flex-col gap-2" role="alert">
          <p>{result.error}</p>
          <Link
            className="underline underline-offset-4"
            href="/admin/moderation"
          >
            Filter zurücksetzen
          </Link>
        </div>
      ) : (
        <ModerationQueue filters={result.filters} />
      )}
    </main>
  )
}
