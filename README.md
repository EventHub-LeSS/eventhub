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

Ändert Keycloak die Organisations-IDs (z. B. nach einem Realm-Re-Import), erkennt der Seed
bestehende Organisationen an Alias oder Name und aktualisiert deren Keycloak-ID, statt
Duplikate anzulegen. Migration 000011 räumte bestehende Duplikate weg und erzwingt die
Eindeutigkeit von Name und Alias per Unique-Index.

#### Service-Konto des Backends (EVENTHUB-188)

Für Keycloak-Admin-Aufrufe, z. B. das Vergeben von Organisationsrollen, meldet sich die API
per Client Credentials mit dem `backend`-Client an. Dessen Service-Konto hat nur die Rollen
`manage-organizations`, `manage-users` und `view-clients` aus `realm-management`, kein `realm-admin`.
Frische Realms bekommen es über `core/realms/eventhub-realm.json`. Bei einem bestehenden
Keycloak-Volume richtet `docker compose up api-seed` das Service-Konto nachträglich ein.

Auch `api-seed` führt ausschließlich Keycloak-Admin-Aufrufe aus und meldet sich daher mit dem
gleichen Service-Konto an (`KEYCLOAK_CLIENT_ID`, Standard `backend`, plus `KEYCLOAK_CLIENT_SECRET`)
statt mit einem Realm-Benutzer. Fehlt das Secret, bricht der Seed sofort ab; bei einer Ablehnung
(403/400) loggt er die Rohantwort des Token-Endpunkts samt Ursachen-Hinweis. Auf Server-Deployments
muss die `api-seed`-Service-Umgebung deshalb `KEYCLOAK_CLIENT_SECRET` enthalten.

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

6 Kategorien, 5 Locations, 39 Veranstaltungen (alle Status: draft/published/cancelled/completed,
davon 13 abgeschlossene historische Events für Empfehlungstests), 53 Zahlungen, 56 Buchungen,
27 Bewertungen, 10 Benachrichtigungen — alle referenziell konsistent.

3 der Buchungen sind Warenkorb-Items von `visitor@eventhub.de` (2× Stand-up Showcase Berlin,
3× Frankfurt Wine Tasting, 1× München Marathon Expo): lebende Reservierungen ohne Zahlung,
deren Plätze bis zum Ablauf der 15-Minuten-Reservierung gehalten werden — Demo-Daten für den
Warenkorb-Flow.

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

### Warenkorb und Zahlungsvorgang (282, EVENTHUB-235)

Besucher legen Tickets beliebig vieler Veranstaltungen in einen **Warenkorb** und bezahlen alles
zusammen an der Kasse. Der Warenkorb ist keine eigene Tabelle: Er ist die Menge der **lebenden
Reservierungen** des Aufrufers ohne Zahlung (`status: reserved`, ohne `payment_id`, `expires_at` in
der Zukunft). Jede Reservierung hält ihre Plätze 15 Minuten; danach ist das Item wieder weg und die
Plätze frei. Pro Veranstaltung liegen höchstens 10 Tickets im Warenkorb. Es ist **kein echter
Zahlungsdienstleister** angebunden; das Ergebnis der Zahlung wählt der Client.

| Endpunkt | Bedeutung |
| --- | --- |
| `GET /api/v1/cart` | Warenkorb des Aufrufers: jedes Item mit Event, Ticketanzahl, aktuellem Einzelpreis und Zeilenbetrag, Platzgrenze (`availableSeats` = freie Plätze **plus** eigene Tickets), `bookable` und `expiresAt`; dazu `totalTickets`, `totalAmount` (Summe der aktuellen Zeilenbeträge) und `checkoutReady`. `openPayment` nennt den offenen Zahlungsvorgang des Aufrufers (`paymentId`, gespeicherter Betrag, Ticketzahl, gemeinsame Ablauffrist), falls einer existiert: Dessen Items haben den Warenkorb verlassen und warten auf ihre Zahlung, und neue Items können erst ausgecheckt werden, wenn er abgeschlossen ist. |
| `POST /api/v1/cart/items` | Reserviert Tickets eines veröffentlichten Events (`eventId`, `quantity`) und legt sie in den Warenkorb. Existiert bereits ein Item für das Event, wächst dessen Anzahl (max. 10) und die Reservierung wird erneuert; ein abgelaufenes Item ohne Zahlung wird in derselben Zeile ersetzt. Der Platz wird sofort gegen die Kapazität geprüft. Antwort `201` mit dem Warenkorb. |
| `PATCH /api/v1/cart/items/{bookingId}` | Setzt die Ticketanzahl eines eigenen Items. Das Item behält seine reservierten Plätze, mehr Tickets brauchen freie Plätze; die Reservierung wird erneuert. |
| `DELETE /api/v1/cart/items/{bookingId}` | Entfernt ein Item und gibt dessen Plätze sofort frei. |
| `DELETE /api/v1/cart` | Leert den Warenkorb. Reservierungen mit offener Zahlung gehören nicht zum Warenkorb und bleiben unberührt. |
| `POST /api/v1/cart/checkout` | Überführt alle lebenden Items in **einen** Zahlungsvorgang. `201` beim Anlegen, `200` wenn bereits ein offener existiert (dann unverändert, spätere Preisänderungen ändern einen angezeigten Betrag nicht). |
| `GET /api/v1/payments/{paymentId}` | Gespeicherter Zahlungsvorgang: Summenbetrag, Status, Items mit Zeilenbetrag, Buchungsstatus und Ticketgültigkeit. |
| `POST /api/v1/payments/{paymentId}/simulate` | Schließt die offene Zahlung mit dem simulierten Ergebnis ab: `{"outcome": "success" \| "failure", "amount": <angezeigter Betrag>}`. |
| `POST /api/v1/bookings/{bookingId}/cancel` | Storniert eine Buchung des Aufrufers (siehe unten). |

Die direkte Buchung `POST /api/v1/bookings` und die alten Zahlungs-Endpunkte
`POST/GET /api/v1/bookings/{bookingId}/payment` sowie
`POST /api/v1/bookings/{bookingId}/payment/simulate` sind ersatzlos entfernt.

**Checkout:** Jede Buchung des Zahlungsvorgangs speichert ihren **Zeilenbetrag** — Ticketanzahl ×
aktueller Veranstaltungspreis —, der Zahlungsbetrag ist die Summe aller Zeilenbeträge. Beides
berechnet und speichert nur das Backend; Beträge im Request werden ignoriert. Alle Buchungen teilen
sich **eine** erneuerte Reservierungsfrist von 15 Minuten, bezahlt wird vor `expiresAt`. Items mit
abgelaufener Reservierung halten keine Plätze und werden ausgelassen; ist kein Item mehr lebend,
antwortet der Checkout `409`, bei leerem Warenkorb `422`. Ein nicht mehr veröffentlichtes Event
lässt den gesamten Checkout scheitern (`409`): Der Zahlungsvorgang gilt für alle Buchungen zusammen
oder für keine. Ein Nutzer hat höchstens einen offenen Zahlungsvorgang; Items, die danach in den
Warenkorb gelegt werden, koexistieren, können aber erst ausgecheckt werden, wenn er abgeschlossen
ist. Der offene Vorgang ist über `openPayment` in `GET /cart` sichtbar, sodass die ausgecheckten
Artikel nicht wie verloren wirken: Wer von der Zahlung zurückkehrt, findet den Vorgang im
Warenkorb und kann ihn fortsetzen.

**Ergebnisse:** Alle drei Zahlungsendpunkte antworten mit `PaymentResponse` aus den gespeicherten
Daten: `amount` (Summe), `items` (je `bookingId`, `event`, `numberOfTickets`, `lineAmount`,
`bookingStatus`, `ticketsValid`), `status` (`pending`/`paid`/`failed`/`refunded`), `failureReason`
und eine lesbare `message`. `ticketsValid` ist nur bei `paid` und bestätigten Buchungen `true` und
nur, solange die Events veröffentlicht oder abgeschlossen sind.

| Ergebnis | Zahlung | Buchungen | `failureReason` |
| --- | --- | --- | --- |
| `success` | `paid` | alle `confirmed` | – |
| `failure` | `failed` | alle `failed` | `declined` |
| Reservierung inzwischen abgelaufen | `failed` | abgelaufene `expired` | `reservation_expired` |
| Event inzwischen zurückgezogen | `failed` | `failed` | `event_unavailable` |
| Buchung vor Abschluss storniert | `failed` | stornierte `cancelled` | `booking_cancelled` |

Fehlgeschlagene Buchungen geben ihre Tickets sofort frei und zählen nicht als verkauft; für einen
neuen Versuch werden die Tickets erneut in den Warenkorb gelegt.

**Stornierung (`POST /bookings/{bookingId}/cancel`):** Eine Reservierung ohne Zahlung ist ein
Warenkorb-Item und wird für sich storniert. Eine Buchung mit Zahlung wird mit dem **ganzen**
Zahlungsvorgang storniert: Während die Zahlung offen ist, wird der gesamte Checkout storniert — es
wurde nichts abgebucht — und nachdem sie bezahlt wurde, wird der Zeilenbetrag der Buchung
rückerstattet; der Zahlungsvorgang gilt als `refunded`, sobald alle seine Buchungen storniert sind
(eine kostenlose Buchung neben einer bezahlten bleibt also gültig, auch wenn die Erstattung schon den
vollen Betrag erreicht). Eine bezahlte Buchung eines vom Veranstalter abgesagten Events ist nicht
stornierbar (`409`): Die Erstattung übernimmt der Veranstalter, und das Ticket bleibt als „Vom
Veranstalter abgesagt“ sichtbar. Wiederholtes Stornieren gelingt ohne weitere Erstattung idempotent.

**Keine dauerhaft offenen Zahlungen:** Zusätzlich schließt die API jede Minute offene Zahlungen
ab, von denen mindestens eine Reservierung abgelaufen ist, als fehlgeschlagen ab
(`PaymentService.RunPaymentCleanup`): ein Durchlauf pro Minute mit `FOR UPDATE SKIP LOCKED` und
höchstens 500 Zahlungen pro Durchlauf, gerade gesperrte Buchungen werden übersprungen. Ein
zurückgezogenes Event allein macht eine Zahlung nicht stale — der Abschluss entscheidet selbst.

**Fehler:** `400` ungültige ID oder ungültiger Body (z. B. `quantity` unter 1), `401` ohne Token,
`403` Nutzer nicht synchronisiert, `404` Event, Item, Buchung oder Zahlung unbekannt oder einem
anderen Nutzer gehörend (Cart-Items verhalten sich bewusst wie unbekannte Ressourcen, sobald sie
abgelaufen, ausgecheckt oder storniert sind), `409` Kapazität überschritten, Reservierung
abgelaufen, Event nicht mehr verfügbar oder Zahlung bereits abgeschlossen, `413` Body von
`simulate` größer als 4 KiB, `422` manipulierter Betrag, Betrag außerhalb von `NUMERIC(12,2)`,
mehr als 10 Tickets pro Event oder leerer Warenkorb beim Checkout.

**Garantien:** Reservieren, Ändern, Entfernen, Auschecken und Abschließen laufen in Transaktionen
mit Zeilensperren (`repository.Transactor`) in der festen Reihenfolge events → bookings →
payments, sodass es zu keinen Deadlocks kommt. Jede Buchung gehört zu höchstens einer Zahlung und
eine Zahlung wird genau einmal abgeschlossen; konkurrierende Checkouts erzeugen genau einen
Zahlungsvorgang. Die Antwort wird noch in der Transaktion gelesen, ein Fehler nach dem Commit ist
damit ausgeschlossen. `GET /payments/{paymentId}` liest Zahlung, Buchungen, Event-Status und
Datenbankuhr in einer einzigen Abfrage. Der Abschluss prüft den Ablauf der Reservierung mit der
Datenbankuhr, so wird eine abgelaufene Reservierung nie bestätigt, nachdem ihre Plätze neu
vergeben wurden. Die Kapazität eines Events lässt sich nicht unter die verkauften und aktiv
reservierten Tickets senken (`PUT /api/v1/events/{id}` antwortet dann mit `409`). Beträge mit
extremem Exponenten (z. B. `1e2000000000`) lehnt die Validierung mit `400` ab, bevor sie
umgerechnet werden.

**Migration 000012:** Mehrere Buchungen dürfen eine Zahlung teilen — `bookings.payment_id` verliert
deshalb seine Eindeutigkeit (Unique-Constraint `bookings_payment_id_key` fällt, Index
`idx_bookings_payment` kommt hinzu) — und `bookings.line_amount` speichert den Zeilenbetrag
(`NUMERIC(12,2)`, nicht-negativ, mit Zahlung immer gesetzt). Bestehende Zeilen erben den Betrag
ihrer Zahlung; Reservierungen ohne Zahlung (Warenkorb-Items) bleiben `NULL`.

**Frontend:** Die Eventseite legt Tickets mit einem Stepper in den Warenkorb (Gesamt der
**Hinzufügung** aus Anzahl × Preis, Kapazitätsgrenze im Stepper); anonyme Besucher parken ihre
Eingabe für den Login-Roundtrip und der Warenkorb wird direkt nach der Anmeldung befüllt. Die
Navigation zeigt einen Warenkorb-Button mit der Anzahl reservierter Tickets — Warenkorb plus
offenem Checkout (geteilter TanStack-Query über die BFF-Route `/api/cart`, Refetch alle 60
Sekunden). Die Seite `/cart` verwaltet Mengen, zeigt Ablauffristen pro Item und checkt an der
Kasse aus; während ein Zahlungsvorgang offen ist, zeigt sie einen Hinweis mit Fortsetzen-Link und
sperrt die Kasse für neue Artikel. `/cart/payment/{paymentId}` führt die simulierte Zahlung über
Kartendaten, die nur im Browser validiert werden, zeigt das Ergebnis aller Items und weist auf
Warenkorb-Artikel hin, die erst nach dieser Zahlung ausgecheckt werden können. `/cart` ist wie
`/tickets` über den Proxy (`proxy.ts`) geschützt.

**Getestet:** Automatisch über `TestAddToCart_*`, `TestGetCart_*`, `TestUpdateQuantity_*`,
`TestRemoveCartItem_*`, `TestClearCart*`, `TestCheckout_*`, `TestGetPayment_*`,
`TestSimulatePayment_*`, `TestExpireStalePayments_*` und `TestCancelBooking_*` in
`backend/internal/service` und `backend/internal/handler` (Merge-Verhalten und Limit, Ablauf und
Ersetzung, Kapazitätsmathematik, parallele Requests ohne Überbuchung, Summenbildung freier und
mehrere Events betreffender Warenkörbe, idempotenter Checkout, manipulierte Beträge, Erfolge und
Ablehnungen je Item, abgelaufene Reservierung und zurückgezogenes Event für den ganzen Checkout,
Stornierung mit und ohne Erstattung, Cleanup in Batches und neben laufenden Abschlüssen). Zusätzlich
manuell mit dem lokalen Stack (`core/docker-compose.yml`) und den Mock-Daten.

### Meine Tickets und Buchungshistorie (EVENTHUB-236)

Drei Endpunkte zeigen angemeldeten Besuchern ihre eigenen Buchungen. Alle lesen dieselben Tabellen
(`bookings`, `payments`, `events`, `locations`) in einer einzigen Abfrage; es gibt keine eigene
Ticket-Tabelle und keine kopierten Status. Ein Ticket ist eine Buchung mit ihrer Ticketanzahl.

| Endpunkt | Inhalt |
| --- | --- |
| `GET /api/v1/me/tickets` | Gültige Tickets (`ticketsValid: true`) und stornierte bezahlte Tickets (`cancelledBy` gesetzt). Kommende Events zuerst nach Startzeit aufsteigend, danach vergangene nach Startzeit absteigend; ein Event gilt als kommend, bis es nach der Datenbankuhr geendet hat. |
| `GET /api/v1/me/bookings?status=…` | Buchungshistorie: alle eigenen Buchungen in jedem Status, die neueste zuerst. `status` (optional) filtert auf den angezeigten Status `reserved`, `confirmed`, `cancelled`, `failed` oder `expired`. |
| `GET /api/v1/me/bookings/{bookingId}` | Details einer eigenen Buchung in jedem Status. |

Alle drei liefern dieselbe Form (`model.OwnBookingResponse`): `bookingId`, `numberOfTickets`,
`bookingStatus`, `paymentStatus`, `failureReason`, `amount`, `refundAmount`, `ticketsValid`,
`cancelledBy`, `message`, `bookedAt`, `expiresAt` und `event` mit Titel, Start, Ende, Status und `location` (Name und
Adresse). Ohne Zahlungsvorgang sind die Zahlungsfelder `null` und `message` fehlt; ein gelöschter Ort
ergibt `location: null`, ein gelöschtes Event `event: null`.

**Gültigkeitsregel:** `ticketsValid` ist nur `true`, wenn die Zahlung `paid`, die Buchung `confirmed`
und das Event `published` oder `completed` ist. Es ist dieselbe Funktion wie bei
`GET /api/v1/payments/{paymentId}` (`hasValidTickets` in
`backend/internal/service/payment_service.go`), und `message` ist derselbe Text wie dort. Stornierte,
fehlgeschlagene und abgelaufene Buchungen sowie Tickets zurückgezogener oder gelöschter Events sind
ungültig und erscheinen in Historie und Details mit `ticketsValid: false`.

**Stornierte Tickets:** `cancelledBy` kennzeichnet bezahlte Tickets, die storniert wurden:
`organizer`, wenn der Veranstalter das Event einer bestätigten, bezahlten Buchung abgesagt hat (die
Buchung bleibt `confirmed`), und `user`, wenn der Besucher die Buchung nach der Bezahlung storniert
hat (Zahlung `paid` oder `refunded`). Hat der Besucher zuerst selbst storniert, bleibt es `user`, auch
wenn das Event später abgesagt wird; umgekehrt kann der Besucher nach der Absage nicht mehr selbst
stornieren. Diese Tickets bleiben in der Ticketliste; das Frontend zeigt sie unter „Meine Tickets“ im
Tab „Storniert“ mit dem Hinweis „Vom Veranstalter abgesagt“ bzw. „Von dir storniert“.
Warenkorb-Reservierungen und vor der Bezahlung abgebrochene Checkouts haben kein `cancelledBy` und
fehlen in der Ticketliste, ebenso alle übrigen ungültigen Buchungen.

**Angezeigter Status:** `bookingStatus` ist der gespeicherte Status. Nur eine Reservierung, deren
`expiresAt` nach der Datenbankuhr verstrichen ist, wird als `expired` angezeigt, auch wenn sie noch als
`reserved` gespeichert ist; `?status=expired` findet beide Fälle. `GET /payments/{paymentId}` zeigt
dagegen den gespeicherten Status `reserved`: mit offener Zahlung höchstens bis zum Cleanup der
Zahlungen nach etwa einer Minute, und ohne Zahlung existiert kein Zahlungsvorgang — die Reservierung
ist dann ein Warenkorb-Item. `ticketsValid`, `paymentStatus`, `failureReason` und `message` stimmen
in beiden Endpunkten immer überein.

**Berechtigung:** Jeder angemeldete Nutzer mit Eintrag in der API-Datenbank. Der Nutzer stammt immer
aus dem Token, nie aus Pfad, Query oder Body; jede Abfrage filtert im Repository auf
`bookings.user_id`.

**Antworten:**

| Status | Bedeutung                                                                            |
| ------ | ------------------------------------------------------------------------------------ |
| `200`  | Liste (`[]`, wenn es keine gibt) bzw. die Buchung                                     |
| `400`  | `status` ist kein Buchungsstatus, oder `{bookingId}` ist keine UUID                   |
| `401`  | nicht angemeldet                                                                      |
| `403`  | Nutzer nicht in der API-Datenbank synchronisiert, wie bei `POST /bookings`            |
| `404`  | Buchung unbekannt **oder** gehört einem anderen Nutzer                                |
| `500`  | Datenbankfehler; Details nur im Server-Log                                            |

Fremde Buchungen ergeben bewusst `404` statt `403`, damit sich ihre IDs nicht von unbekannten
unterscheiden lassen.

**Code:** `OwnBookingHandler` (`backend/internal/handler/own_booking_handler.go`) →
`OwnBookingService` (`backend/internal/service/own_booking_service.go`) → `OwnBookingRepository`
(`backend/internal/repository/own_booking_repository.go`). Die Abfrage folgt `GetBookingPayment`:
`bookings` mit `LEFT JOIN` auf `payments`, `events` und `locations` in einem Statement, die
Datenbankuhr (`clock_timestamp()`) einmal pro Abfrage, sodass alle Zeilen einer Liste nach derselben
Zeit beurteilt werden. Die Ticketliste filtert erst nach dem Lesen mit der Gültigkeitsregel, damit es
die Regel nur einmal gibt.

**Getestet:** Automatisch über `TestHasValidTickets_*`, `TestOwnBookingResponse_*`,
`TestListOwnTickets_*`, `TestListOwnBookings_*` und `TestGetOwnBooking_*` in
`backend/internal/service` sowie `TestListOwnTickets_*`, `TestListOwnBookings_*`,
`TestGetOwnBooking_*`, `TestOwnBookings_*` und `TestOwnBookingHandlers_*` in
`backend/internal/handler` (reservieren, bezahlen und Ticket sehen über die API, abgelehnte Zahlung,
Stornierung mit Erstattung, zurückgezogenes Event, fremde Buchungen, Gegenprobe mit
`GET /payments/{paymentId}`, gleiche Darstellung in allen drei Sichten, abgelaufene
Reservierung und Statusfilter, Reihenfolge, gelöschter Ort und gelöschtes Event, leere Listen,
Statuscodes).

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

### Bewertungsdurchschnitte (EVENTHUB-238, EVENTHUB-239)

Die folgenden Endpunkte sind öffentlich und verwenden Datenbank-UUIDs:

- `GET /api/v1/events/{eventId}/rating-summary` liefert `eventId`, `averageRating` und
  `ratingCount`. Veröffentlichten Events sowie beendeten Events mit Status `completed` kann
  eine Zusammenfassung zugeordnet werden; Entwürfe und abgesagte Events ergeben `404`.
- `GET /api/v1/organizations/{organizationID}/rating-summary` liefert `organizationId`,
  `averageRating` und `ratedEventCount`. Die ID ist weder Keycloak-ID noch Organisationsalias.

Gültig sind sichtbare Bewertungen mit Score 1–5, die über eine bestätigte Buchung dem Event
zugeordnet sind. Jede Bewertung zählt einmal, unabhängig von der Ticketanzahl. Verwaiste
Bewertungen und andere Buchungsstatus werden ausgeschlossen. Ohne gültige Bewertungen ist
`averageRating` ausdrücklich `null`, die jeweilige Anzahl `0`.

Der Veranstalterdurchschnitt ist der Durchschnitt der **ungerundeten Eventdurchschnitte**;
jedes bewertete Event zählt gleich viel. Es zählen nur eigene Events mit Status `published`
oder `completed`, deren `endTime <= NOW()` (Datenbankzeit) ist. Unbewertete Events bleiben aus
dem Nenner ausgeschlossen. Ein Event mit einer 5-Sterne-Bewertung und eines mit neun
1-Sterne-Bewertungen ergeben daher `3.0`, nicht `1.4`. Die API rundet nicht; Anzeige-Rundung
übernimmt der Verbraucher. Empfehlungen behalten ihre bisherige Berechnung und können
deshalb einen anderen internen Veranstalterscore verwenden.

Ungültige UUIDs ergeben `400`, unbekannte Ressourcen `404`, Datenbankfehler `500` mit
generischem Fehlertext. Die bestehenden Eventdetail-Antworten bleiben unverändert;
abgeschlossene Events haben eine Bewertungszusammenfassung, sobald sie beendet sind,
aber noch keine öffentliche Eventdetail-Antwort.

### Bewertungstexte eines Events (297)

`GET /api/v1/events/{eventId}/ratings?page=1&limit=50` ist öffentlich und liefert genau die
Bewertungen, die `rating-summary` zählt (sichtbar, Score 1–5, bestätigte Buchung dieses Events),
neueste zuerst. `total` entspricht daher `ratingCount`. Bewertungen ohne Text sind mit leerem
`text` enthalten. Jedes Element hat `ratingId`, `score`, `text`, `authorName` und `createdAt`.
`authorName` ist nur der **Vorname** der bewertenden Person; ohne Nutzer (z. B. gelöscht) oder
ohne Vornamen ist er `null`. Nachname, E-Mail, Nutzer- und Buchungs-IDs werden nicht ausgegeben.

Paginierung wie bei `/moderation/ratings`: `page` ab 1, `limit` 1–100 (Standard 50); die Antwort
enthält `items`, `page`, `limit` und `total`. Sichtbarkeit und Fehler wie bei `rating-summary`:
Entwürfe, abgesagte und noch nicht beendete `completed`-Events ergeben `404`, ungültige UUIDs
oder Paginierungswerte `400`. Die Event-Detailseite im Frontend zeigt die Bewertungen mit
zehn pro Seite (`?reviewPage=n`); fehlt der Vorname, steht dort „Anonym“.

**Code:** `RatingHandler.ListEventRatingsHandler` → `RatingService.ListEventRatings` →
`RatingRepository.ListEventRatings`. **Getestet:** `TestListEventRatings*` sowie
`TestEventRatingList*Database` in `backend/internal/handler`, im Frontend
`bun test features/catalog/lib/event-ratings.test.ts`.

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

### Inhalte moderieren (Backend)

Alle Moderationsendpunkte verlangen ein Bearer-Token mit globaler Rolle `admin`
oder `moderator`. Besucher und reine Organisations-Admins haben keinen Zugriff
(`401` ohne Anmeldung, `403` ohne globale Moderationsrolle).

Die bestehende Bewertungsmoderation bleibt unveraendert: Bewertungen werden
eingesehen und begruendet aus- oder wieder eingeblendet. Veranstaltungstexte
lassen sich jetzt organisationsuebergreifend pruefen und korrigieren:

| Endpunkt | Funktion |
| --- | --- |
| `GET /api/v1/moderation/events` | Vollstaendige Events aller Status, neueste zuerst; Filter `status` und `organizationId` (lokale UUID), `page` ab 1, `limit` 1-100 (Standard 50). Antwort: `items`, `page`, `limit`, `total`. |
| `GET /api/v1/moderation/events/{eventId}` | Einzelansicht einschliesslich Beschreibung, Status und Organisationsname, auch fuer Entwuerfe und zurueckgezogene Events. |
| `PATCH /api/v1/moderation/events/{eventId}/content` | Titel und/oder Beschreibung korrigieren; Body z. B. `{"title":"Korrigierter Titel","description":"Sachlicher Text","reason":"Problematischer Inhalt"}`. |
| `POST /api/v1/moderation/events/{eventId}/withdraw` | Event zurueckziehen; Body z. B. `{"reason":"Bestaetigter Betrugsfall"}`. |

Jede Aenderung braucht eine Begründung mit 1-1.000 Zeichen nach Entfernen
aeusserer Leerzeichen. Titel haben nach Trimmen 3-200 Zeichen, Beschreibungen
maximal 5.000 Zeichen. Eine leere Beschreibung entfernt den bisherigen Text;
weggelassene Felder bleiben unveraendert. Inhaltskorrekturen sind in jedem
Eventstatus moeglich und veraendern weder Preis, Kapazitaet, Termine,
Veranstalter noch Status. Die Antwort enthaelt `eventId`, `status`, `changed`.

Zurueckziehen setzt Entwuerfe und veroeffentlichte Events auf `cancelled` und
sperrt sie fuer Veranstalter (`withdrawnByModerator`). Vom Veranstalter bereits
abgesagte Events behalten den Status `cancelled` und erhalten nachtraeglich
diese Sperre (`changed=true`); bereits durch Moderation zurueckgezogene Events
bleiben unveraendert. Abgeschlossene Events ergeben `409`. Zurueckgezogene
Events verschwinden aus oeffentlichen Listen, Details
und Empfehlungen. Neue Buchungen und der Abschluss offener Zahlungen sind
nicht mehr moeglich, bereits bezahlte Tickets sind ungueltig. Buchungen und
Zahlungen bleiben in der Historie erhalten; es erfolgen keine automatischen
Rueckerstattungen. Veranstalter koennen durch Moderation zurueckgezogene Events
weder als Entwurf wiederherstellen noch erneut veroeffentlichen (siehe
EVENTHUB-290).

Status- und Textaenderungen werden als `event.moderated` mit vorherigen und
neuen Werten, Begründung und handelndem Konto im Organisations-Audit-Log
gespeichert (ohne Organisation im Scope `platform`). Aenderung und Audit-Eintrag
sind atomar; ein Audit-Fehler rollt die Aenderung zurueck. Identische erneute
Anfragen liefern `changed=false`, ohne einen weiteren Eintrag zu schreiben.
Ungueltige Eingaben ergeben `400`, unbekannte Events `404`.

Fachliche Abgrenzung: Bewertungen werden ausgeblendet statt administrativ
umgeschrieben oder geloescht; Events werden unter Erhalt der Historie
zurueckgezogen statt hart geloescht. Organisations-/Veranstaltersperren sind
nicht Teil dieser Implementierung. Fuer die neuen Eventaktionen ist keine
Datenbankmigration erforderlich. Das Frontend bleibt unveraendert.

Backend-Tests einschliesslich PostgreSQL-Integration: `go test -p 1 ./...`
im Backend-Verzeichnis mit `TEST_DATABASE_DSN` auf einer separaten,
wegwerfbaren Testdatenbank.

### Inhalte moderieren (Frontend)

Globale Administratoren erreichen **Moderation** über die Navigation oder
`/admin/moderation`. Die Ansicht zeigt Bewertungen mit Kommentartext, Sternen,
Sichtbarkeit, Zeitpunkt sowie Veranstaltungs- und Organisationsbezug. Sie gilt
plattformweit, unabhängig von der aktuell gewählten Organisation. Normale Nutzer
und reine Organisations-Admins haben keinen Zugriff auf die Seite oder ihre API-Proxys.

Die Ansicht verwendet `GET /api/v1/moderation/ratings` mit Sichtbarkeitsfilter,
optionalem `eventId` und Seitennavigation. **Ausblenden** und **Wieder einblenden**
verwenden `PUT /api/v1/moderation/ratings/{ratingId}/visibility`. Jede Änderung
erfordert eine Begründung mit 1–1.000 Zeichen nach Entfernen äußerer Leerzeichen.
Erst nach API-Bestätigung wird der Status aktualisiert und eine Bestätigung angezeigt.
Der bestehende Organisations-Audit-Log zeigt die Aktion und ihre Begründung.
Ausgeblendete Bewertungen zählen nicht mehr für öffentliche Zusammenfassungen und
Empfehlungen; Wieder-Einblenden stellt die vorherige Sichtbarkeit wieder her.

**Grenzen der Ansicht:** Die Frontend-Ansicht bietet weiterhin nur
Bewertungsmoderation. Die oben beschriebenen Eventaktionen sind ausschliesslich
als Backend-API implementiert. Das Backend erlaubt zusätzlich globale Moderatoren,
während diese Frontend-Story den Zugriff auf globale Administratoren beschränkt.

Validierungs- und API-Antworttests: im Frontend-Verzeichnis
`bun test features/moderation/lib/validation.test.ts`.

### Benachrichtigungen (EVENTHUB-240)

Nach Migrationen 000009 und 000010 liefert GET /api/v1/notifications die eigenen
Benachrichtigungen, neueste zuerst. Die Antwort enthält items, page, limit und total;
leere Seiten enthalten items: []. page beginnt bei 1, limit ist standardmäßig 50
und maximal 100. Alle Benachrichtigungsendpunkte
benötigen ein Bearer-Token und einen synchronisierten API-Nutzer.

POST /api/v1/bookings/{bookingId}/reminder simuliert eine Erinnerung für eine
eigene bestätigte Buchung einer bevorstehenden veröffentlichten Veranstaltung.
Sie enthält Anlass, Inhalt, Empfänger, Buchungs-ID und Zeitstempel. Der geplante
Zeitpunkt scheduledAt liegt 24 Stunden vor Beginn, bei kurzfristigen Buchungen
bei der Erzeugung. Die Simulation macht sie sofort in der Liste sichtbar
(simulated: true); es gibt keinen automatischen Versandjob oder E-Mail-Versand.

Die erste Erzeugung liefert 201, wiederholte oder parallele Aufrufe dieselbe
Erinnerung mit 200. Fremde oder unbekannte Buchungen liefern 404; nicht
bestätigte Buchungen und nicht mehr verfügbare Veranstaltungen 409.
Das Frontend kann die Daten als Banner oder Benachrichtigungsliste darstellen.

PATCH /api/v1/notifications/{notificationId}/read setzt readAt für eine eigene
Benachrichtigung (null bedeutet ungelesen). Wiederholte Aufrufe erhalten den
ersten Lesezeitpunkt; fremde oder unbekannte IDs liefern 404.

Beim Abruf werden nur Erinnerungen auf der angeforderten Seite mit aktuellen
Veranstaltungs- und Ortsdaten abgeglichen und bei Änderungen gespeichert.
Titel, Startzeit, Veranstaltungsort und Adresse sowie Ticketanzahl bleiben
dadurch aktuell. scheduledAt wird neu berechnet; für kurzfristige Termine
dient der ursprüngliche Erstellungszeitpunkt als stabile Untergrenze.
Geänderte Erinnerungen werden wieder ungelesen. Auch eine erneute Simulation
aktualisiert die bestehende Erinnerung. Es gibt weiterhin keinen Versandjob;
vor einem späteren echten Versand müsste dieser Abgleich ebenfalls erfolgen.

**Absagen (EVENTHUB-291):** Zieht ein Veranstalter eine noch nicht begonnene
Veranstaltung zurück, erhält jeder Nutzer mit bestätigten Buchungen eine
Benachrichtigung mit occasion event_cancelled und eventId (Migration 000016);
die Erinnerungen der Veranstaltung werden gelöscht. Wird die Veranstaltung
erneut veröffentlicht (EVENTHUB-290) und wieder zurückgezogen, erhalten die dann
aktuellen Ticketinhaber eine neue Absage; frühere Absagen bleiben unverändert.

### Zurückgezogene Veranstaltungen erneut verwenden (EVENTHUB-290)

Veranstalter mit event_manager in der zugehörigen Organisation können beide
Wege nutzen:

- POST /api/v1/events/{id}/restore-draft: cancelled wird draft. Der vorhandene
  Datensatz mit seiner ID bleibt erhalten und kann über PUT /api/v1/events/{id}
  bearbeitet und anschließend über POST /api/v1/events/{id}/publish publiziert
  werden. Unvollständige Veranstaltungen dürfen zur Bearbeitung zurück in den
  Entwurf; erst die Veröffentlichung verlangt vollständige Daten.
- POST /api/v1/events/{id}/publish: erlaubt neben draft jetzt auch cancelled
  und veröffentlicht die Veranstaltung direkt erneut.

Der Entwurf bleibt öffentlich unsichtbar und nicht buchbar. Buchungen werden
weder dupliziert noch in ihrem Status verändert; die Kapazitätsprüfung beim
Bearbeiten bleibt bestehen. Die Gültigkeit von Tickets folgt dem Eventstatus:
Solange die Veranstaltung zurückgezogen oder Entwurf ist, sind Tickets
ungültig. Nach der erneuten Veröffentlichung gelten bezahlte, bestätigte
Buchungen wieder als gültige Tickets, und Erinnerungen lassen sich wieder
erzeugen. Ticketinhaber werden darüber nicht gesondert benachrichtigt; wer in
der Zwischenzeit storniert hat, bleibt storniert. Completed-Veranstaltungen
dürfen nicht wiederhergestellt oder
erneut veröffentlicht werden. Statuswechsel werden unter derselben Event-Sperre
und atomar mit dem Audit-Eintrag gespeichert; der Eintrag hält Ausgangs- und
Zielstatus fest (event.restored_as_draft beziehungsweise event.published).
Migration 000014 unterscheidet selbst zurückgezogene Veranstaltungen von
Moderationssperren (withdrawnByModerator). Bestehende, im Audit protokollierte
Moderationsrückzüge werden übernommen. Durch Moderation zurückgezogene Events
bleiben für beide Veranstalteraktionen gesperrt; ein Moderator kann auch eine
bereits vom Veranstalter zurückgezogene Veranstaltung nachträglich sperren.
