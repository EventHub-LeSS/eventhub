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

#### Service-Konto des Backends (EVENTHUB-188)

Für Keycloak-Admin-Aufrufe, z. B. das Vergeben von Organisationsrollen, meldet sich die API
per Client Credentials mit dem `backend`-Client an. Dessen Service-Konto hat nur die Rollen
`manage-organizations`, `manage-users` und `view-clients` aus `realm-management`, kein `realm-admin`.
Frische Realms bekommen es über `core/realms/eventhub-realm.json`. Bei einem bestehenden
Keycloak-Volume richtet `docker compose up api-seed` das Service-Konto nachträglich ein.

#### Mock-Benutzer

Alle Passwörter: `password`

| Benutzer                                  | Globale Rolle | Organisation & Berechtigung                          |
| ----------------------------------------- | ------------- | ---------------------------------------------------- |
| `finn.betz@grossmeister.de`               | admin         | Provadis `org_admin`, Telekom `org_admin`, ACME `org_admin`, Stadthalle `org_admin` |
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

Benutzernamen sind überall die E-Mail-Adresse (`registrationEmailAsUsername`).
Realms, die vor der Vereinheitlichung importiert oder geseedet wurden, benennt
`api-seed` beim nächsten Lauf um; Service-Konten wie `service-account-backend`
haben keine E-Mail-Adresse und behalten ihren Namen.

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

### Event-Empfehlungen

`GET /api/v1/recommendations` liefert für angemeldete Nutzer veröffentlichte,
noch nicht gestartete Events mit freien Plätzen. Bestätigte eigene Buchungen
werden ausgeschlossen. Bei der Verfügbarkeit zählen bestätigte Tickets und
noch gültige Reservierungen; abgelaufene Reservierungen blockieren keine Plätze.
Die Prüfung ist eine Momentaufnahme, keine Platzgarantie bei späterer Buchung.

Das Ranking gewichtet Kategorie-Affinität mit 50 %, bestätigte Tickets relativ
zur Kapazität mit 40 % und die durchschnittliche Veranstalterbewertung mit 10 %.
Für die Kategorie-Affinität zählt jedes vergangene, bestätigt gebuchte Event
einmal. Events ohne Kategorie bleiben im Nenner, tragen aber zu keiner Kategorie
bei. Veranstalterbewertungen stammen wie bisher aus abgeschlossenen Events.
Ohne Historie oder Bewertungen ist der jeweilige Anteil null. Bei gleichem Score
entscheiden Startzeit und Event-ID. Es werden höchstens drei SQL-Abfragen pro
Anfrage ausgeführt. Datenbankfehler führen zu HTTP 500 statt zu einem unbemerkt
unvollständigen Ranking; Details werden nur serverseitig protokolliert.

Migration 000005 erzwingt positive Event-Kapazitäten und ergänzt einen Index für
bestätigte Nutzerbuchungen. Bestehende Kapazitäten kleiner oder gleich null müssen
vor dem Einspielen fachlich korrigiert werden; die Migration verändert sie nicht
automatisch.

Backend-Tests laufen im Backend-Verzeichnis mit `go test ./...`. Die
PostgreSQL-Integrationstests benötigen `TEST_DATABASE_DSN` und sollten mit
`go test -p 1 ./...` ausgeführt werden, weil mehrere Pakete dieselben Testtabellen
zurücksetzen. **Nur eine separate, wegwerfbare Testdatenbank verwenden:** Die
Testvorbereitung migriert das Schema und leert Tabellen mit `TRUNCATE ... CASCADE`.

### Veranstaltungen einer Organisation (EVENTHUB-79)

`GET /api/v1/events/org/{id}` liefert alle Veranstaltungen einer Organisation in jedem Status,
also auch Entwürfe sowie abgesagte und abgeschlossene Events. `{id}` ist die Datenbank-UUID der
Organisation (wie im Feld `organizerId` der Events) oder ihr Alias, wie ihn das Token enthält; so
kann das Frontend die aktive Organisation abfragen. Die Keycloak-ID aus `GET /users/me` wird nicht
akzeptiert.

**Berechtigung:** Jedes Mitglied der Organisation, unabhängig von der Rolle (`org_admin`,
`event_manager`, `finance_viewer` oder ohne Rolle). Die Mitgliedschaft stammt aus dem Token und
wird wie bei den übrigen Event-Endpunkten über Keycloak-ID oder Alias der Organisation geprüft.

**Antworten:**

| Status | Bedeutung                                                                            |
| ------ | ------------------------------------------------------------------------------------ |
| `200`  | Array der Events; `[]`, wenn die Organisation keine hat. Die Reihenfolge ist nicht festgelegt. |
| `401`  | nicht angemeldet                                                                      |
| `404`  | Organisation existiert nicht **oder** der Aufrufer ist kein Mitglied                  |
| `500`  | Datenbankfehler                                                                       |

Nicht-Mitglieder bekommen bewusst `404` statt `403`, damit sich fremde Organisationen nicht von
unbekannten unterscheiden lassen.

**Abgrenzung:** `GET /api/v1/events/self?status=…` liefert die Events aller Organisationen, in
denen der Aufrufer `event_manager` ist (z. B. die eigenen Entwürfe), sortiert nach Startzeit.

**Code:** `EventHandler.ListOrganizationEventsHandler` übergibt `{id}` und `principal.OrganizationIDs()`
an `EventService.ListByOrganizationRef`, das einen Alias in die Datenbank-UUID auflöst und
`EventService.ListByOrganization` aufruft; der Service prüft die Mitgliedschaft mit `managesOrganization`
wie `UpdateEvent`, `PublishEvent`, `WithdrawEvent` und `GetEventStatistics`.

**Getestet:** Automatisch über `TestListOrganizationEventsHandler_*` in `backend/internal/handler`
(Mitgliedschaft je Rolle, Nicht-Mitglied, unbekannte UUID, Abruf per Alias, unbekannter Alias,
ohne Anmeldung, leere Organisation, Datenbankfehler). Zusätzlich wurde der Endpunkt vor dem Merge
von PR #41 manuell mit dem lokalen Stack (`core/docker-compose.yml`) und den Mock-Daten getestet:

- Mitglieder mit `event_manager`, `finance_viewer` und `org_admin` erhalten alle Events ihrer
  Organisation, inklusive Entwürfen.
- Nicht-Mitglieder, Besucher ohne Organisation und unbekannte UUIDs erhalten `404`, eine ungültige
  ID `400` und Aufrufe ohne Token `401`.
- Benutzer in mehreren Organisationen sehen nur die Events der Organisationen, in denen sie Mitglied
  sind.
- `GET /events/self` und `GET /events` verhalten sich unverändert.

Seit `{id}` auch einen Alias annimmt, gibt es kein `400` mehr: Eine `{id}`, die weder eine bekannte
UUID noch ein bekannter Alias ist, ergibt `404`.

### Veranstaltungsentwurf löschen (EVENTHUB-256)

`DELETE /api/v1/events/{id}` löscht einen Entwurf unwiderruflich: Die Zeile in `events` wird
entfernt, kein Soft-Delete. Kategorie, Ort und Organisation bleiben erhalten. Nur Events im Status
`draft` können gelöscht werden; veröffentlichte, abgesagte und abgeschlossene Events bleiben, ebenso
jedes Event mit Buchungen in irgendeinem Status (auch storniert, fehlgeschlagen oder abgelaufen),
weil Buchungen und Zahlungen zur Buchungshistorie der Besucher gehören. `bookings.event_id` ist
`ON DELETE SET NULL`; ohne diese Prüfung verlören Buchungen beim Löschen ihr Event.

**Berechtigung:** `event_manager` **oder** `org_admin` der Organisation, der das Event gehört.
Globale Admins haben keinen Sonderweg. Veröffentlichen, Zurückziehen und Bearbeiten erlauben weiterhin
nur `event_manager`.

**Antworten:**

| Status | Bedeutung                                                                              |
| ------ | -------------------------------------------------------------------------------------- |
| `200`  | `{"message": "event deleted"}`                                                         |
| `400`  | `{id}` ist keine UUID, oder das Event ist kein Entwurf (`only draft events can be deleted`) |
| `401`  | nicht angemeldet                                                                       |
| `403`  | keine Rolle `event_manager`/`org_admin`, oder das Event gehört einer anderen Organisation |
| `404`  | Event existiert nicht (auch beim zweiten Löschen)                                      |
| `409`  | Event hat Buchungen (`events with bookings cannot be deleted`)                         |
| `500`  | Datenbankfehler oder Audit-Eintrag nicht schreibbar; es wird nichts gelöscht           |

Die Reihenfolge der Prüfungen ist wie beim Veröffentlichen: erst Existenz (`404`), dann Organisation
(`403`), dann Status und Buchungen. Eine fremde Organisation erfährt so den Status nicht.

**Code:** `EventHandler.DeleteEventHandler` → `EventService.DeleteEvent`. Prüfen und Löschen laufen in
einer Transaktion unter der Zeilensperre des Events (`LockEvent`, `SELECT … FOR UPDATE`), wie beim
Veröffentlichen und bei der Ticketreservierung; ein paralleles Veröffentlichen oder Buchen kann nicht
dazwischenkommen. Das Löschen wird als `event.deleted` mit dem letzten Stand des Entwurfs im Audit
Log protokolliert; die bisherigen Audit-Einträge des Events bleiben bewusst erhalten.

**Getestet:** Automatisch über `TestDeleteEvent_*` in `backend/internal/service` sowie
`TestDeleteEventHandler_*` und `TestDraftLifecycle_CreateAndDelete_KeepsAuditTrail` in
`backend/internal/handler` (Rollen, fremde Organisation, alle Status, Buchungen in jedem Status,
zweites Löschen, Rollback bei Audit-Fehler, Wettlauf mit dem Veröffentlichen in beiden Reihenfolgen).

### Audit Log der Organisationen

Schreibende Aktionen von Organisationsmitgliedern werden mit dem persönlichen Keycloak-Konto (`sub`, Benutzername) und einem Datenbank-Zeitstempel in der Tabelle `audit_logs` protokolliert. Protokolliert werden: Event anlegen/ändern/veröffentlichen/zurückziehen/löschen und das Ändern der Mitgliedsrollen. Nicht erfasst werden Besucheraktionen, globale Rollen und das Anlegen von Organisationen.

**Einsicht:** `GET /api/v1/organizations/{organizationID}/audit-logs?limit=50&cursor=…` (neueste zuerst, max. 100 pro Seite, Cursor-Paginierung). Zugriff haben `org_admin` der Organisation und globale Admins. `organizationID` ist die Keycloak-Organisations-ID.

**Garantien:** Event-Änderungen und Audit-Eintrag werden in einer Transaktion geschrieben; kann der Eintrag nicht geschrieben werden, wird die Änderung zurückgerollt. Keycloak-Rollenänderungen sind nicht transaktional: vor der ersten Änderung wird ein `started`-Eintrag geschrieben, danach ein Ergebniseintrag (`succeeded`/`incomplete`) mit derselben `operation_id`. Ein `started` ohne Ergebnis bedeutet „Ausgang unbestätigt“.

**Grenzen:** Das Log belegt das verwendete Konto, nicht die reale Person hinter geteilten Zugangsdaten. Backups, Anwendungslogs und privilegierte Datenbankadministratoren sind nicht Teil der Garantie.

**Neue Aktion ergänzen:**
1. Aktion in `backend/internal/audit/audit.go` definieren.
2. Route in `cmd/api/main.go` mit `middleware.Audit(audit.<Action>)` versehen (nach der Authentifizierung).
3. Im Service Organisation autorisieren und `audit.MetaFromContext` auswerten, Eintrag in derselben Transaktion schreiben (`Tx.Audit`). Bei externen Systemen Start-/Ergebniseintrag wie in `ConfigureOrganizationMemberRoles`.

**Aufbewahrung:** Einträge werden nach 10 Jahren täglich um 03:00 UTC per `pg_cron` (Migration `000007`) gelöscht. Die API-Datenbank braucht daher das Image aus `core/Dockerfile.api-db` und die Einstellungen `shared_preload_libraries=pg_cron`, `cron.database_name=<DB-Name>` (siehe `core/docker-compose.yml`). Ohne pg_cron bricht die Migration mit einer klaren Fehlermeldung ab. Bestehende Volumes bleiben erhalten; den `api-db`-Container mit `docker compose up -d --build api-db` neu erzeugen. Für externe Deployments (z. B. Dockhand) das Image `ghcr.io/<owner>/api-db` mit denselben Parametern verwenden.

**Tests mit Datenbank:** `TEST_DATABASE_DSN` auf eine solche Datenbank setzen, dann `go test -p 1 ./...` im Ordner `backend`.
