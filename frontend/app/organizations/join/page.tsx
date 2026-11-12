import Link from "next/link"

import { requireSession } from "@/features/auth"
import { CopyInviteEmail, OpenMailButton } from "@/features/organizations"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"

function StepNumber({ value }: { value: number }) {
  return (
    <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-medium text-primary-foreground">
      {value}
    </span>
  )
}

export default async function JoinOrganizationPage() {
  const user = await requireSession()

  return (
    <main className="mx-auto grid w-full max-w-screen-xl flex-1 grid-cols-1 gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
      <aside className="flex flex-col justify-between gap-6 rounded-2xl border border-border bg-muted/40 p-5 lg:p-8">
        <div className="flex flex-col gap-4">
          <p className="text-sm font-medium text-muted-foreground">
            EventHub für Teams
          </p>
          <div className="flex flex-col gap-2">
            <h2 className="text-2xl font-bold tracking-tight text-balance">
              Gemeinsam Events gestalten.
            </h2>
            <p className="max-w-sm text-sm text-muted-foreground">
              Einer bestehenden Organisation trittst du per Einladung bei. Ein
              Administrator der Organisation lädt dich über deine E-Mail-Adresse
              ein – sobald du die Einladung annimmst, bist du Mitglied.
            </p>
          </div>
          <ol className="flex flex-col gap-2 text-sm text-muted-foreground">
            <li>1. Organisations-Admin kontaktieren</li>
            <li>2. Einladungs-E-Mail abwarten</li>
            <li>3. Einladung in der E-Mail annehmen</li>
          </ol>
        </div>

        <div className="flex flex-col gap-2 rounded-xl border border-border bg-background p-4 text-sm">
          <p className="font-medium">Noch keine Organisation?</p>
          <p className="text-xs text-muted-foreground">
            Du kannst auch eine eigene Organisation beantragen.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="w-fit"
            render={<Link href="/organizations/create" />}
            nativeButton={false}
          >
            Organisation beantragen
          </Button>
        </div>
      </aside>

      <div className="flex justify-center lg:items-start">
        <div className="flex w-full max-w-4xl flex-col gap-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight">
              Organisation beitreten
            </h1>
            <p className="text-sm text-muted-foreground">
              So wirst du Mitglied einer bestehenden Organisation.
            </p>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-3">
                <StepNumber value={1} />
                Organisations-Admin kontaktieren
              </CardTitle>
              <CardDescription>
                Bitte einen Admin der Organisation, dich einzuladen. Dafür wird
                deine E-Mail-Adresse benötigt, sie wird auf der Seite der
                Organisation unter „Invite member“ eingetragen.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <CopyInviteEmail email={user.email} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-3">
                <StepNumber value={2} />
                E-Mails prüfen
              </CardTitle>
              <CardDescription>
                Sobald du eingeladen wurdest, bekommst du eine E-Mail von
                EventHub an <span className="font-medium">{user.email}</span>.
                Findest du sie nicht, sieh auch im Spam-Ordner nach.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <OpenMailButton />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-3">
                <StepNumber value={3} />
                Einladung annehmen
              </CardTitle>
              <CardDescription>
                So schließt du den Beitritt aus der E-Mail heraus ab:
              </CardDescription>
            </CardHeader>
            <CardContent>
              <ol className="flex list-decimal flex-col gap-2 pl-5 text-sm">
                <li>
                  Öffne die Einladung und klicke auf den Link zum Beitreten der
                  Organisation.
                </li>
                <li>
                  Melde dich, falls nötig, mit deinem bestehenden
                  EventHub-Konto an.
                </li>
                <li>
                  Zurück in EventHub erscheint die Organisation in deinem
                  Benutzermenü, wo du zu ihr wechseln kannst. Falls nicht, lade
                  die Seite nach einigen Minuten neu oder melde dich ab und
                  wieder an.
                </li>
              </ol>
              <p className="mt-4 text-xs text-muted-foreground">
                Der Einladungslink ist nur begrenzt gültig. Ist er abgelaufen,
                bitte deinen Admin, dich erneut einzuladen.
              </p>
            </CardContent>
          </Card>
        </div>
      </div>
    </main>
  )
}
