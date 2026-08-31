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

Wichtig: `--import-realm` importiert nur, wenn der Realm noch nicht existiert. Nach Änderungen an
der Realm-Datei muss deshalb das Datenbank-Volume gelöscht werden:

```bash
docker compose down -v && docker compose up -d
```

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