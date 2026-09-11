# eventhub

## Projekt klonen und starten

1. Repository klonen:
   ```bash
   git clone https://github.com/EventHub-LeSS/eventhub
   ```

Rest folgt, sobald das projekt weit genug vorangeschritten ist.

## Lokale Entwicklung

### Keycloak starten

```bash
cd core
docker compose up -d
```

- Admin-Konsole: <http://localhost:5433/admin> (`admin` / `admin`)
- Der Realm `eventhub` wird beim ersten Start aus `core/realms/eventhub-realm.json` importiert.

Beim ersten Start wird ein eigenes Keycloak-Image gebaut, das das Login-Theme enthält
(siehe unten). Das dauert einige Minuten, danach greift der Docker-Cache.

Wichtig: `--import-realm` importiert nur, wenn der Realm noch nicht existiert. Nach Änderungen an
der Realm-Datei muss deshalb das Datenbank-Volume gelöscht werden:

```bash
docker compose down -v && docker compose up -d
```

### Login-Theme (Keycloakify)

Die Login- und Registrierungsseiten von Keycloak liegen als eigenes Projekt in
`core/keycloak-theme/` und sind mit [Keycloakify](https://keycloakify.dev) sowie dem
shadcn-Theme `@oussemasahbeni/keycloakify-login-shadcn` gebaut.

Keycloakify braucht zum Verpacken des Themes Java und Maven. Damit das niemand lokal
installieren muss, passiert der Build in `core/Dockerfile.keycloak`. Nach Änderungen am Theme:

```bash
cd core
docker compose build keycloak && docker compose up -d --force-recreate keycloak
```

Zum Entwickeln der Seiten ohne Keycloak (mit Hot Reload) gibt es Storybook:

```bash
cd core/keycloak-theme
bun install && bun run storybook
```

Aussehen (Farbe, Font, Layout, App-Name) wird über die `environmentVariables` in
`core/keycloak-theme/vite.config.ts` gesteuert.

Testbenutzer:

| Benutzer                     | Passwort    | Organisationen                 |
| ---------------------------- | ----------- | ------------------------------ |
| `visitor@eventhub.test`      | `visitor`   | –                              |
| `organizer@acme-events.test` | `organizer` | ACME Events, Stadthalle Bremen |

Die Organisationsmitgliedschaft entscheidet, ob ein Benutzer Events anlegen und verwalten darf.
Mitgliedschaften werden aktuell in der Admin-Konsole unter _Organizations → … → Members_ vergeben.
Der Organizer ist in zwei Organisationen, damit sich der Organisationswechsel im Benutzermenü
testen lässt.

### Mock-Daten seeding (EVENTHUB-190)

Das `api-seed` Kommando erzeugt konsistente Mockdaten für das überarbeitete Datenmodell
in Keycloak (Nutzer, Organisationen, graduated-permission Gruppen) und in der API-Datenbank
(Kategorien, Locations, Veranstaltungen, Zahlungen, Buchungen, Bewertungen, Benachrichtigungen).

```bash
cd core
docker compose up -d          # startet Keycloak + API-DB + Migration
docker compose up api-seed    # erzeugt Mockdaten (idempotent, sicher wiederholbar)
```

Alternativ gegen einen laufenden Stack ohne Docker:

```bash
cd backend
go run ./cmd/seed
```

Das Seeding ist idempotent: existierende Nutzer/Organisationen/Gruppen/Datensätze
werden übersprungen, sodass der Befehl bedenkenlos mehrfach ausgeführt werden kann.

#### Mock-Benutzer

Alle Passwörter: `password`

| Benutzer                                  | Globale Rolle | Organisation & Berechtigung                          |
| ----------------------------------------- | ------------- | ---------------------------------------------------- |
| `großmeister_finn`                        | admin         | Provadis `org_admin`, Telekom `org_admin`, ACME `org_admin`, Stadthalle `org_admin` |
| `organizer@provadis-hochschule.de`        | visitor       | Provadis `event_manager`                             |
| `organizer@telekom.de`                    | visitor       | Telekom `event_manager`                              |
| `eva.manager@provadis-hochschule.de`      | visitor       | Provadis `event_manager`, ACME `event_manager`       |
| `tom.finance@provadis-hochschule.de`      | visitor       | Provadis `finance_viewer`                            |
| `lena.admin@telekom.de`                   | visitor       | Telekom `org_admin`, Stadthalle `event_manager`      |
| `max.multi@eventhub.de`                   | visitor       | Provadis `org_admin`, **Telekom `event_manager`**   |
| `visitor@eventhub.de`                     | visitor       | –                                                    |
| `visitor2@eventhub.de`                    | visitor       | –                                                    |
| `visitor3@eventhub.de`                    | visitor       | –                                                    |

`max.multi@eventhub.de` hat bewusst **unterschiedliche Rollen** in verschiedenen
Organisationen, um die abgestuften Berechtigungen (EVENTHUB-188) zu demonstrieren.

#### Mock-Organisationen

Provadis Hochschule, Telekom, ACME Events, Stadthalle Bremen

#### Mock-Geschäftsdaten

6 Kategorien, 5 Locations, 12 Veranstaltungen (alle Status: draft/published/cancelled/completed),
10 Zahlungen, 10 Buchungen, 4 Bewertungen, 5 Benachrichtigungen — alle referenziell konsistent.

### Frontend starten

```bash
cd frontend
cp .env.example .env.local
bun install
bun dev
```

Das Frontend läuft auf <http://localhost:3000>.
