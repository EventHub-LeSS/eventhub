import { CirclePlusIcon } from "lucide-react"
import Link from "next/link"

import { requireSession } from "@/features/auth"
import { Button } from "@/features/shared/components/ui/button"

export default async function OrganizationsPage() {
  const user = await requireSession()

  return (
    <main className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Organisationen</h1>
          <p className="text-sm text-muted-foreground">
            Organisationen, in denen du Mitglied bist, oder neue Organisation
            beantragen.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {(user.canReviewApplications || user.isAdmin) && (
            <Button
              variant="outline"
              render={<Link href="/admin/organizer-applications" />}
              nativeButton={false}
            >
              Anträge prüfen
            </Button>
          )}
          <Button
            render={<Link href="/organizations/create" />}
            nativeButton={false}
          >
            <CirclePlusIcon />
            Organisation beantragen
          </Button>
        </div>
      </div>

      {user.organizations.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Du bist noch in keiner Organisation Mitglied. Über „Organisation
          beantragen“ kannst du das Kontaktformular ausfüllen, um Veranstalter
          zu werden.
        </p>
      ) : (
        <ul className="flex flex-col gap-2">
          {user.organizations.map((organization) => (
            <li
              key={organization}
              className="flex items-center justify-between rounded-lg border p-4"
            >
              <span className="text-sm font-medium">{organization}</span>
              {organization === user.activeOrganization && (
                <span className="text-xs text-muted-foreground">Aktiv</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </main>
  )
}
