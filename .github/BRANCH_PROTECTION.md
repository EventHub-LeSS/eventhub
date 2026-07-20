# Branch Protection einrichten

Diese Anleitung beschreibt, wie Branch Protection Rules konfiguriert werden,
damit kein Code ohne grünen Build in `main` oder `development` gemergt werden kann.

## Voraussetzungen

- Du brauchst **Admin-Rechte** im Repository.

## Schritt-für-Schritt Anleitung

### 1. Branch Protection Rules öffnen

1. Gehe zu **Repository** → **Settings** → **Branches**
2. Klicke auf **Add branch ruleset** (oder bearbeite eine bestehende Regel)

### 2. Regel für `main` konfigurieren

Erstelle eine Regel mit folgenden Einstellungen:

| Einstellung | Wert |
|------------|------|
| **Branch name pattern** | `main` |
| **Require a pull request before merging** | ✅ Aktiviert |
| **Require status checks to pass before merging** | ✅ Aktiviert |
| **Status checks that are required** | `Build`, `Startup Check`, `Code Quality` |
| **Require branches to be up to date before merging** | ✅ Aktiviert |

### 3. Regel für `development` konfigurieren

Wiederhole die gleichen Schritte für den Branch `development`.

### 4. Empfohlene Zusatzeinstellungen

- **Require review from Code Owners**: Falls ihr ein `CODEOWNERS`-File verwendet
- **Restrict who can push**: Direktes Pushen auf `main` verhindern
- **Require linear history**: Für eine saubere Git-Historie

## Ergebnis

Nach der Konfiguration gilt:
- ❌ Kein direkter Push auf `main` oder `development` möglich
- ❌ Kein Merge ohne grünen CI-Build
- ✅ Jede Änderung muss über einen Pull Request laufen
- ✅ Build-Status ist für alle Teams sichtbar

## Status-Checks Referenz

Die folgenden Status-Checks werden vom CI-Workflow bereitgestellt:

| Check Name | Beschreibung |
|-----------|-------------|
| `Build` | Kompiliert das Projekt |
| `Startup Check` | Prüft ob die Anwendung startbar ist |
| `Code Quality` | Führt Linter/Code-Analyse aus |
