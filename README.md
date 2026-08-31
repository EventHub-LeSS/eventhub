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

| Benutzer                     | Passwort    | Organisationen                     |
| ---------------------------- | ----------- | ---------------------------------- |
| `visitor@eventhub.test`      | `visitor`   | –                                  |
| `organizer@acme-events.test` | `organizer` | ACME Events, Stadthalle Bremen     |

Die Organisationsmitgliedschaft entscheidet, ob ein Benutzer Events anlegen und verwalten darf.
Mitgliedschaften werden aktuell in der Admin-Konsole unter *Organizations → … → Members* vergeben.
Der Organizer ist in zwei Organisationen, damit sich der Organisationswechsel im Benutzermenü
testen lässt.

### Frontend starten

```bash
cd frontend
cp .env.example .env.local
bun install
bun dev
```

Das Frontend läuft auf <http://localhost:3000>.