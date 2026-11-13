import Link from "next/link"

import { requireSession } from "@/features/auth"
import { CreateOrganizationForm } from "@/features/organizations"
import { Button } from "@/features/shared/components/ui/button"

export default async function CreateOrganizationPage() {
  const user = await requireSession()

  return (
    <main className="mx-auto grid w-full max-w-screen-xl flex-1 grid-cols-1 gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
      <aside className="flex flex-col justify-between gap-6 rounded-2xl border border-border bg-muted/40 p-5 lg:p-8">
        <div className="flex flex-col gap-4">
          <p className="text-sm font-medium text-muted-foreground">
            EventHub für Veranstalter
          </p>
          <div className="flex flex-col gap-2">
            <h2 className="text-2xl font-bold tracking-tight text-balance">
              Deine Organisation. Deine Events.
            </h2>
            <p className="max-w-sm text-sm text-muted-foreground">
              Jeder Nutzer kann über dieses Kontaktformular eine eigene
              Organisation beantragen. Nach Prüfung und Freigabe durch einen
              EventHub-Administrator wird deine Organisation angelegt und dein
              Konto als Veranstalter eingetragen.
            </p>
          </div>
          <ol className="flex flex-col gap-2 text-sm text-muted-foreground">
            <li>1. Organisationsdaten und Vorhaben im Formular erfassen</li>
            <li>2. Prüfung durch einen EventHub-Administrator</li>
            <li>
              3. Automatische Erstellung der Organisation & Eintragung als
              Veranstalter
            </li>
          </ol>
        </div>

        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-2 rounded-xl border border-border bg-background p-4 text-sm">
            <p className="font-medium">Organisation existiert schon?</p>
            <p className="text-xs text-muted-foreground">
              Du kannst auch einer bestehenden Organisation auf Einladung beitreten.
            </p>
            <Button
              variant="outline"
              size="sm"
              className="w-fit"
              render={<Link href="/organizations/join" />}
              nativeButton={false}
            >
              Organisation beitreten
            </Button>
          </div>

          {user.isAdmin && (
            <div className="flex flex-col gap-2 rounded-xl border border-border bg-background p-4 text-sm">
              <p className="font-medium">Anträge prüfen</p>
              <p className="text-xs text-muted-foreground">
                Als Administrator kannst du offene Bewerbungen anderer Nutzer
                einsehen und freigeben.
              </p>
              <Button
                variant="outline"
                size="sm"
                className="w-fit"
                render={<Link href="/admin/organizer-applications" />}
                nativeButton={false}
              >
                Zur Antragsprüfung
              </Button>
            </div>
          )}
        </div>
      </aside>

      <div className="flex justify-center lg:items-start">
        <div className="flex w-full max-w-4xl flex-col gap-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight">
              Organisation beantragen
            </h1>
            <p className="text-sm text-muted-foreground">
              Fülle das Kontaktformular aus, um deine Organisation zur Freigabe
              einzureichen.
            </p>
          </div>
          <CreateOrganizationForm
            defaultOrgAdmin={user.email}
            sessionOrganizations={user.organizations}
          />
        </div>
      </div>
    </main>
  )
}
