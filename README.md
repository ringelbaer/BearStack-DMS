# BearStack

BearStack ist eine Go-Webanwendung zur Ablage und Verwaltung von Dokumenten. Die Daten liegen lokal in SQLite und im Dateisystem. Der Betrieb ist fuer kleine Server wie einen Raspberry Pi ausgelegt.

Diese Datei beschreibt die wichtigsten Admin-Punkte. Eine vollstaendige Pi-Installation steht in [deploy-raspberrypi.md](deploy-raspberrypi.md). Eine systemd-Beispiel-Unit liegt in [deploy/bearstack.service](deploy/bearstack.service).

## Schnellstart

Voraussetzung fuer Entwicklung und Build ist Go `1.26.6` oder eine kompatible neuere Version. Fuer `make test-js` wird zusaetzlich `node` benoetigt. Die optionalen Playwright-Smokes brauchen zusaetzlich `npm` und einen Playwright-kompatiblen Browser; lokal ist standardmaessig der Chrome-Channel konfiguriert.

```sh
BEARSTACK_AUTH_USER=admin \
BEARSTACK_AUTH_PASSWORD=change-me \
go run ./cmd/bearstack
```

Standardwerte:

- Adresse: `127.0.0.1:8080`
- Datenverzeichnis: `data`
- Datenbank: `data/bearstack.db`
- Dokumente: `data/documents`
- Fotos: deaktiviert, Root bei Aktivierung standardmaessig `data/photos`
- Upload-Limit: 50 MiB

Ohne aktives Config- oder SQLite-Konto ist Auth auf Loopback-Adressen wie `127.0.0.1:8080` fuer die Ersteinrichtung deaktiviert. Ein Listener auf nicht-lokalen Interfaces wie `0.0.0.0:8080` oder `:8080` startet nur mit mindestens einem aktiven Konto; dieses darf vollstaendig aus der SQLite-Datenbank stammen. Ohne Auth werden nur Anfragen von Loopback-Gegenstellen mit `localhost` oder einer Loopback-IP als HTTP-Host akzeptiert; fremde Hostnamen werden zum Schutz vor DNS-Rebinding mit `403` abgewiesen. Vor dem Zugriff ueber einen eigenen Hostnamen oder Reverse Proxy muss ein Konto eingerichtet werden.

## Android-App zum Personenbenennen

Ab BearStack **0.30.0** steht unter [`apps/android/`](apps/android/README.md) eine native Android-App (Android 8.0+, App-Version 0.5.3) zur Verfügung. Sie zeigt bis zu vier Gesichtsausschnitte, unterstützt Benennen und Zuordnen mit Namensvorschlägen, Abtrennen, Ignorieren mit klickbarer Rückgängig-Meldung am oberen Bildschirmrand bei weiter bedienbarer Ansicht und lokale Statistiken. Die aktuelle App benötigt BearStack **0.35.0**: Beim Halten eines Gesichtsausschnitts zeigt sie das vollständige Originalfoto, beim Loslassen wieder das Grid. Eine dünne Bounding Box markiert das Gesicht; Herunterwischen während des Haltens zoomt zum Gesicht, Hochwischen wieder heraus. Unter Ausschnitten und Originalfoto steht der vollständige, nach Galerieregeln aufbereitete Bildpfad. Wischaktionen funktionieren auch auf dem freien Hintergrund der Bearbeitungsansicht. Rechtswischen holt die zuletzt übersprungene Person zurück und korrigiert die lokale Statistik. Erforderlich sind HTTPS und ein Konto mit Personenrechten; ab BearStack 0.36.0 reicht dafür „Fotos bearbeiten“ (`photos.edit`). Selbstsignierte Zertifikate werden vor der Anmeldung über ihren SHA-256-Fingerabdruck bestätigt. Ab App 0.5.2 bleibt der Inhalt beim Ein- und Ausblenden des Ladebalkens an derselben Position. Ab App 0.5.3 warten automatische Ignorier-Schreibvorgänge auf das Schließen eines offenen Namensdialogs; ablaufende Rückgängig-Meldungen unterbrechen dessen Texteingabe und Tastatur nicht mehr. App 0.4.1 behebt den allgemeinen Fehler beim Kontowechsel; falsche Zugangsdaten und fehlende Personenrechte werden auch im Zertifikatsdialog angezeigt.

`make test-android` prüft die App und baut eine Debug-APK. Einrichtung, Gesten, Zertifikatsabgleich, Emulator-Integrationstest und private Release-Signierung stehen in der [Android-Anleitung](apps/android/README.md). Der gemeinsame Vertrag bleibt [`openapi.yaml`](openapi.yaml), neue Endpunkte liegen unter `/api/photos/labeling/v1`. Android hat einen eigenen Gradle-Build und wird nicht in Go- oder Docker-Builds einbezogen.

## Lokale Entwicklung

Der lokale Start nutzt standardmaessig das Repo-nahe Verzeichnis `data/`:

```sh
go run ./cmd/bearstack
```

Mit getrenntem Entwicklungsdatenverzeichnis:

```sh
BEARSTACK_DATA_DIR=/tmp/bearstack-dev go run ./cmd/bearstack
```

Fuer einen lokalen Start mit Auth:

```sh
BEARSTACK_AUTH_USER=admin \
BEARSTACK_AUTH_PASSWORD=change-me \
go run ./cmd/bearstack
```

Eine `.env`-Datei im Arbeitsverzeichnis wird automatisch gelesen. Zusaetzlich kann `BEARSTACK_ENV_FILE` auf eine weitere Env-Datei zeigen. Bereits gesetzte Prozess-Umgebungsvariablen haben Vorrang vor Werten aus Env-Dateien; Werte aus `.env` haben fuer denselben Key Vorrang vor `BEARSTACK_ENV_FILE`.

Externe Werkzeuge sind je nach Funktion optional: `pdftoppm`/`pdfinfo`/`pdfunite` aus `poppler-utils` fuer PDF-Vorschau, E-Mail-Archiv-Merge und OCR-Vorbereitung, `chromium` fuer gerenderte EML-Mailabbildungen, `soffice` aus LibreOffice fuer Text-/Office-Vorschau und Volltextextraktion, `tesseract` plus Sprachpakete fuer OCR, `ffmpeg` fuer Video- und Fallback-Bild-Thumbnails sowie optional `vipsthumbnail` fuer Bild-Thumbnails.

## Tests

Go-Tests und Browser-Syntaxchecks:

```sh
make test
```

Einzeln:

```sh
go test ./...
make test-go
make test-js
make test-playwright
```

`make test-go` fuehrt `go test ./...` aus. `make test-js` nutzt `scripts/check-js.sh` und fuehrt `node --check` fuer alle Browser-Skripte unter `internal/server/static/*.js` aus. `make test-playwright` installiert bei Bedarf die fest versionierte Testabhängigkeit und prüft Dokumenten-Upload, Benutzerverwaltung und Foto-Galerie. Die Make-Variablen `GO`, `NODE` und `NPM` koennen bei Bedarf ueberschrieben werden, z. B. `NODE=/opt/node/bin/node make test-js`. Falls kein lokaler Chrome-Channel verfuegbar ist, kann Playwright wie ueblich mit eigenem Browser-Download verwendet werden, z. B. `npx playwright install chromium` und `PLAYWRIGHT_BROWSER_CHANNEL=chromium make test-playwright`.

Playwright baut einmal pro Testlauf ein temporäres BearStack-Binary. Alle Suiten verwenden denselben Helfer für Start, Gesundheitsprüfung und geordnetes Beenden; temporäre Daten werden erst nach Prozessende entfernt. Die Testabhängigkeit ist in `package-lock.json` festgelegt und wird bei Bedarf mit `npm ci --ignore-scripts` installiert. Ein vorhandener `GOCACHE` wird weiterverwendet.

Reine Go-Testhelfer liegen in `_test.go`-Dateien und werden nicht in das Anwendungsbinary übernommen. Die GPX-Benchmarks setzen für Messungen ohne Cache neben den Einträgen auch LRU-Verwaltung und Speicherzähler zurück.

Ab BearStack 0.39.2 bleiben Gesichtsausschnitte im Web und in der Android-App unverzerrt: Sie werden proportional auf dunkelgrauem Hintergrund in die gleichmäßig quadratischen Kacheln eingepasst. Alte gestreckte Vorschauen werden beim nächsten Abruf einzeln ersetzt; ein App-Update oder vollständiger Cache-Neuaufbau ist dafür nicht nötig. Bereits im App-Speicher geladene Altbilder werden nach erneutem Anmelden oder einem App-Neustart neu geladen.

Ab 0.40.0 lassen sich einzelne Gesichter in der Web-Detailansicht einer Person mit einem Stern als Vergleichsbilder favorisieren. Alle Favoriten werden beim Gesichtsabgleich berücksichtigt, auch oberhalb der eingestellten Referenzanzahl. Freie Plätze werden möglichst über verschiedene Galerieordner verteilt. Die dokumentierte Favoriten-API ist für eine spätere App-Erweiterung verfügbar; die Android-Oberfläche bleibt unverändert.

Ab 0.39.3 erscheinen das Ignorieren-× und der Stift für den Benenn-Dialog in der Personenübersicht nur bei unbenannten Gruppen. Nach dem Benennen oder Zusammenführen passt sich die Darstellung ohne Seitenreload an.

Regressionstests pruefen unter anderem die gleichen Foto-Wertebereiche aus Formular und Datenbank, unterschiedliche HTML-/API-Antworten auf zu hohe Dokumentseiten sowie Fotoframe und Lightbox ohne Galerie-Script. Details zur internen Trennung stehen in der [Architekturbeschreibung](_site-src/docs/architektur.md).

Ab 0.39.1 werden allgemeine, Dokument- und Fotoeinstellungen je Formular atomar gespeichert. Fehler hinterlassen keine teilweise gespeicherten Werte; zusammengehörige Einstellungen werden gemeinsam gelesen und ihre Caches mit Schreibvorgängen synchronisiert. Personenbezogene Prüfungen berücksichtigen die betroffenen Gruppen und die Herkunft importierter Namen. Neue `.adminonly`-Markierungen wirken beim nächsten Zugriff; Gesamtübersichten prüfen weiterhin sämtliche relevanten Gesichtsverzeichnisse. Der Gesichtsabgleich lädt Referenzkandidaten gebündelt. Mailimport und EML-Archivierung verwenden gemeinsame MIME-Decoder einschließlich Windows-1252-Headern.

`make test-js` führt nach den Syntaxchecks auch die DOM-Regressionen in `scripts/js-dom-tests.mjs` aus. Weitere Tests sichern Rollback, konkurrierende Einstellungen, neue Schutzmarkierungen und die Zahl der Referenzabfragen ab. `go test ./internal/photos -run '^$' -bench '^BenchmarkFaceVisibility$' -benchmem` vergleicht eine Gesamtprüfung mit einer gezielten Personenprüfung bei 1.000 Verzeichnissen.

Weitere Tests sichern die OCR-Prozessausfuehrung mit kontrollierten Werkzeug-Fixtures ab: Seitenreihenfolge, Fallback-Limit, Fehler, Abbruch und temporaere Dateien. Dateisystemtests pruefen Traversal und Symlinks auch beim Erstellen von Verzeichnissen. Browser-Regressionen decken verspaetete Lightbox-Antworten, fehlgeschlagene Metadaten-/Vorschauabrufe sowie die Navigation per Tastatur ab.

Performance-Regressionen pruefen begrenzte Unicode-Distanzberechnung fuer Feldwertvorschlaege, Tagdefinitionen ohne Dokumentzaehlung und gebuendelte `.adminonly`-Startabfragen mit Rollback. Die Fotokarte liest ihre Abmessungen einmal je Renderdurchlauf und verwendet Track-Polylinien weiter; die Lightbox stellt bereits geladene Medien pro Auswahl einmal dar. Der Benchmark `go test ./internal/server -run '^$' -bench BenchmarkSimilarCustomFieldValues -benchmem` misst die Feldwertsuche mit 1.500 Werten.

Go-Anweisungsabdeckung messen (Browser-Tests werden separat ausgefuehrt):

```sh
go test ./... -coverprofile=/tmp/bearstack-coverage.out
go tool cover -func=/tmp/bearstack-coverage.out
```

Die Foto-Lightbox liegt in `app-photos-lightbox.js`. Die Galerie initialisiert das Modul mit expliziten Abhaengigkeiten fuer Bearbeitungsmodus, gebuendeltes Metadaten-Nachladen und Kartenhelfer; gemeinsame Medienfunktionen kommen aus `app-photos-media.js`.

## Build

Lokal ueber Make:

```sh
make build
```

Direkt:

```sh
go build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
```

Cross-Build fuer Raspberry Pi OS 64-bit:

```sh
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
```

## Einstellungen

Im Systemmenue bilden Einstellungen (Zahnrad), Konto (Person) und Logout eine gemeinsame Icon-Zeile. API und Log stehen im Footer neben der Versionsnummer. Sichtbar sind jeweils die Aktionen, fuer die das Konto berechtigt ist.

Die Einstellungen starten auf **Allgemein** (`/settings/general`). Unter **Einstellungen → Allgemein** (`/settings/general`) stehen die globalen Optionen fuer Anwendungsname, Design, Startseite, Tag-Darstellung und Favicon. **Einstellungen → Dokumente** (`/settings`) enthaelt Desktop-Vorschau, Dokument-Wolke, Tag-Ordner und Papierkorb-Aufbewahrung. Speichern aendert jeweils nur den geoeffneten Bereich; Favicon-Upload und Zuruecksetzen bleiben separate Aktionen.

Die Einstellungsnavigation steht auf grossen Bildschirmen seitlich und bricht auf kleineren Bildschirmen in mehrere Zeilen um. Formulargruppen, lange Beschriftungen und Aktionen passen sich der verfuegbaren Breite an.

## Versionierung

Die Anwendungs-Version steht zentral in `VERSION` und wird in der Weboberflaeche dezent im Footer angezeigt. BearStack nutzt semantische Versionierung:

- `PATCH`: Bugfixes, Security-Fixes, Performance, Refactors ohne neues Verhalten, kleine UI-Korrekturen.
- `MINOR`: neue rueckwaertskompatible Funktionen, neue optionale Einstellungen, nicht-brechende API- oder UI-Faehigkeiten, automatische kompatible Migrationen.
- `MAJOR`: brechende Aenderungen an HTTP/API/WebDAV, Konfiguration, Datenformaten, Berechtigungen oder manuelle/incompatible Migrationen.

Bei mehreren Aenderungstypen gewinnt die hoechste Kategorie. Docs-only- und Test-only-Aenderungen erhoehen die Version nicht. Solange BearStack in `0.x` ist, fuehrt der erste echte Major Change auf `1.0.0`.

## Docker

Lokale Python-Umgebungen (`.venv`, `.venv-faces`), `.cache` sowie `_site` und `_site-src` sind vom Docker-Buildkontext ausgeschlossen.

Image lokal bauen:

```sh
docker build -t bearstack:local .
```

Container starten:

```sh
docker run --rm \
  -p 8080:8080 \
  -v bearstack-data:/var/lib/bearstack \
  -e BEARSTACK_AUTH_USER=admin \
  -e BEARSTACK_AUTH_PASSWORD='change-me' \
  bearstack:local
```

Mit Compose:

```sh
BEARSTACK_AUTH_PASSWORD='change-me' docker compose up -d --build
```

Alternativ mit Passwort-Hash:

```sh
BEARSTACK_AUTH_PASSWORD_HASH='$2a$10$...' docker compose up -d --build
```

Der Container lauscht intern auf `0.0.0.0:8080` und speichert Daten unter `/var/lib/bearstack`. Deshalb muss Auth im Container gesetzt sein; `compose.yaml` nutzt standardmaessig `admin` als Benutzer, reicht `BEARSTACK_AUTH_PASSWORD` und `BEARSTACK_AUTH_PASSWORD_HASH` durch und veroeffentlicht den Port ueber `BEARSTACK_PORT` oder sonst `8080`. Das Runtime-Image basiert auf `debian:trixie-slim`, die Build-Stage auf `golang:1.26-trixie`. Das Beispiel-Image enthaelt `chromium`, `ffmpeg`, `libreoffice-writer`, `poppler-utils`, `tesseract-ocr`, `tesseract-ocr-deu` und `tesseract-ocr-eng`, damit EML-Archive, Foto-/Video-Vorschaubilder, PDF-/Office-Vorschauen, Text-/Office-Volltextextraktion und OCR im Container funktionieren. Bei aktiviertem Fotomodul muss ein Host-Fotoverzeichnis read-only nach `/srv/photos` gemountet werden.

Eine detaillierte Anleitung fuer Synology DSM mit Container Manager steht in [`deploy-synology.md`](deploy-synology.md).

Multi-Arch-Image fuer eine Registry bauen und veroeffentlichen:

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/<owner>/bearstack:<tag> \
  --push .
```

## Konfiguration

Konfiguration erfolgt ueber Defaults, eine JSON-Datei (`BEARSTACK_CONFIG`) und Umgebungsvariablen. Beim Start werden zuerst `.env` und danach `BEARSTACK_ENV_FILE` in die Prozessumgebung geladen, ohne bereits gesetzte Variablen zu ueberschreiben. Danach wird die JSON-Datei geladen und zum Schluss ueberschreiben Umgebungsvariablen die JSON-Werte. Effektive Prioritaet: Defaults < JSON < Env-Dateien < bereits gesetzte Prozess-Env. Innerhalb der Env-Dateien gewinnt `.env` vor `BEARSTACK_ENV_FILE`, wenn beide denselben Key setzen.

Unbekannte Felder in der JSON-Datei werden ignoriert, damit aeltere Installationen mit zusaetzlichen oder kuenftigen Parametern weiter starten. Env-Dateien unterstuetzen einfache `KEY=value`-Zeilen, optional mit `export`, einfachen oder doppelten Quotes sowie Kommentare nach einem Leerzeichen.

Beispiel:

```json
{
  "addr": "127.0.0.1:8080",
  "data_dir": "/var/lib/bearstack",
  "storage_dir": "/var/lib/bearstack/documents",
  "db_path": "/var/lib/bearstack/bearstack.db",
  "max_upload_bytes": 52428800,
  "tls": {
    "enabled": false,
    "cert_file": "",
    "key_file": "",
    "auto_cert": true
  },
  "photos": {
    "enabled": false,
    "root_dir": "/srv/photos",
    "data_dir": "/var/lib/bearstack/photos",
    "cache_dir": "/var/lib/bearstack/photos/thumbnails",
    "db_path": "/var/lib/bearstack/photos/photos.db",
    "page_size": 120
  },
  "webdav": {
    "path": "/webdav"
  },
  "auth": {
    "username": "admin",
    "password": "",
    "password_hash": "$2a$10$...",
    "realm": "BearStack"
  }
}
```

Wichtige Umgebungsvariablen:

| Variable | Bedeutung |
| --- | --- |
| `BEARSTACK_CONFIG` | Pfad zur JSON-Konfiguration. |
| `BEARSTACK_ENV_FILE` | Optionale zweite Env-Datei zusaetzlich zu `.env`. |
| `BEARSTACK_ADDR` | HTTP-Listener (ohne TLS) bzw. HTTPS-Listener (mit TLS), Standard `127.0.0.1:8080`; nicht-lokale Listener erfordern Auth. |
| `BEARSTACK_DATA_DIR` | Basisdatenverzeichnis, Standard `data`. |
| `BEARSTACK_STORAGE_DIR` | Dokumentenspeicher, Standard `${BEARSTACK_DATA_DIR}/documents`. |
| `BEARSTACK_DB_PATH` | SQLite-Datenbank, Standard `${BEARSTACK_DATA_DIR}/bearstack.db`. |
| `BEARSTACK_MAX_UPLOAD_MB` | Upload-Limit in MiB, Standard `50`. |
| `BEARSTACK_MAX_UPLOAD_BYTES` | Upload-Limit in Bytes; hat Vorrang vor `BEARSTACK_MAX_UPLOAD_MB`. |
| `BEARSTACK_AUTH_USER` | Basic-Auth-Benutzer. |
| `BEARSTACK_AUTH_PASSWORD` | Klartextpasswort, vor allem fuer lokale Tests. |
| `BEARSTACK_AUTH_PASSWORD_HASH` | bcrypt-Hash; hat Vorrang vor `BEARSTACK_AUTH_PASSWORD`. |
| `BEARSTACK_AUTH_REALM` | Basic-Auth-Realm, Standard `BearStack`. |
| `BEARSTACK_TLS_ENABLED` | Direktes HTTPS in BearStack aktivieren (`1`, `true`, `yes`, `on`). |
| `BEARSTACK_TLS_CERT_FILE` / `BEARSTACK_TLS_KEY_FILE` | Zertifikat und Key fuer direktes HTTPS; muessen gemeinsam gesetzt werden. |
| `BEARSTACK_TLS_AUTO_CERT` | Self-Signed-Zertifikat automatisch erzeugen, Standard `true`. |
| `BEARSTACK_PHOTOS_ENABLED` | Fotomodul aktivieren. |
| `BEARSTACK_PHOTOS_DIR` | Vorhandenes Fotoverzeichnis, Standard `${BEARSTACK_DATA_DIR}/photos`. |
| `BEARSTACK_PHOTOS_DATA_DIR` | BearStack-eigene Fotodaten, Standard `${BEARSTACK_DATA_DIR}/photos-data`. |
| `BEARSTACK_PHOTOS_CACHE_DIR` | Thumbnail-Cache, Standard `${BEARSTACK_PHOTOS_DATA_DIR}/thumbnails`. |
| `BEARSTACK_PHOTOS_DB_PATH` | Separate Foto-Indexdatenbank, Standard `${BEARSTACK_PHOTOS_DATA_DIR}/photos.db`. |
| `BEARSTACK_PHOTOS_PAGE_SIZE` | Seitengroesse der Fotoliste, Standard `120`. |
| `BEARSTACK_WEBDAV_PATH` | HTTP-Pfad fuer WebDAV, Standard `/webdav`; muss mit `/` beginnen, darf nicht `/` sein und darf keine Leerzeichen enthalten. |

Weitere projektnahe Variablen:

| Variable | Bedeutung |
| --- | --- |
| `BEARSTACK_PORT` | Nur `compose.yaml`: Host-Port fuer Docker Compose, Standard `8080`. |
| `GO` | Nur `Makefile`: Go-Binary fuer `make test-go` und `make build`, Standard `go`. |
| `NODE` | Nur `Makefile`/`scripts/check-js.sh`: Node-Binary fuer `make test-js`, Standard `node`. |
| `NPM` | Nur `Makefile`: npm-Binary fuer `make test-playwright`, Standard `npm`. |
| `PLAYWRIGHT_BROWSER_CHANNEL` | Nur Playwright-Konfiguration: Browser-Channel fuer `make test-playwright`, Standard `chrome`. |
| `BEARSTACK_WEBDAV_TRACE` | Diagnose fuer WebDAV-Clients: bei `1`, `true`, `yes` oder `on` protokolliert BearStack WebDAV-Methode, Status und Pfadmetadaten. |

## Dokumentenfunktion

Dokumente werden ueber die Weboberflaeche (`POST /upload`), die JSON-API (`POST /api/upload`) oder WebDAV-`PUT` importiert. BearStack speichert die Originaldatei im konfigurierten `storage_dir`, legt Metadaten in SQLite ab und fuehrt Text-/Vorschau-/Thumbnail-Verarbeitung asynchron im Hintergrund aus. Unterstuetzt werden PDF, Bilder sowie einfache Text- und Office-Formate; Office-Text und Office-Vorschauen benoetigen LibreOffice.

Endgültiges Löschen speichert Dateibereinigungsaufträge zusammen mit der Löschung der Dokumentmetadaten in einer SQLite-Transaktion. Originale und Vorschauen werden zunächst nach `storage_dir/.purge/` verschoben. Fehlgeschlagene Aufträge werden beim Start und anschließend minütlich erneut verarbeitet, auch bei deaktivierter Papierkorb-Aufbewahrungsfrist. Betroffene Originaldateinamen bleiben bis zum Abschluss reserviert. Die Dokumentdatenbank wird dafür automatisch auf Schema 17 migriert; Datenbank und vollständigen Dokumentenspeicher einschließlich `.purge/` zusammen sichern.

Gleichzeitige Aufrufe der Dokumentstatistik teilen sich eine Berechnung; zwischenzeitliche Cache-Invalidierungen bleiben wirksam. Bei der endgültigen Dateibereinigung warten nur Zugriffe auf dasselbe Dokument aufeinander. Eine laufende Vorschauerzeugung blockiert keine parallel angeforderte Löschung anderer Dokumente.

PDF-Thumbnails werden erst nach erfolgreicher Erzeugung und JPEG-Prüfung atomar veröffentlicht. Der Thumbnail-Nachholprozess arbeitet in kleinen Batches über Dokument-IDs weiter, auch wenn ein kompletter Batch fehlschlägt; fehlerhafte Dateien bleiben für spätere Läufe erhalten.

Dokument-Bildthumbnails prüfen vor dem Dekodieren die Bilddimensionen und erlauben höchstens 40 Megapixel. Bei ungültigen oder größeren Bildern wird keine neue Vorschau erzeugt; Originaldatei und vorhandenes Thumbnail bleiben erhalten.

Standardmaessig zeigt BearStack PDF-Ausgaben mit dem nativen Viewer des Browsers. Unter `Konto -> Darstellung` kann jeder Nutzer den integrierten BearStack-PDF-Viewer aktivieren; Nutzerverwalter koennen dieselbe geraeteuebergreifende Praeferenz pro verwaltbarem Konto setzen. Der lokale Viewer bietet Seitennavigation, Zoom, Breiten-/Seitenanpassung, Textauswahl, Links sowie Zugriff auf Browser-Viewer und Originaldatei. Er wird erst beim Oeffnen eines PDFs geladen und faellt bei nicht unterstuetzten oder passwortgeschuetzten Dateien automatisch auf den Browser-Viewer zurueck. Die Einstellung gilt auch fuer von LibreOffice erzeugte PDF-Vorschauen und wird fuer SQLite- wie Config-Konten in `bearstack.db` gespeichert.

Die maschinenlesbare API-Beschreibung liegt im Repository als `openapi.yaml` und wird von einer laufenden Instanz authentifiziert unter `GET /api/openapi.yaml` ausgeliefert. Sie verwendet OpenAPI 3.1 und beschreibt die oeffentlichen JSON-, Upload- und Dokumentmedien-Endpunkte.

Bei Versionsänderungen muss `info.version` in `openapi.yaml` mit der Root-Datei `VERSION` übereinstimmen; `TestOpenAPISpecMatchesApplicationVersion` prüft diesen Abgleich.

Uploads werden nach dem konfigurierten Limit begrenzt, Dateinamen werden normalisiert, unbekannte Dateitypen werden abgelehnt und gespeicherte Pfade werden immer gegen den Storage-Root aufgeloest. Unerwartete Import- und Vorschaufehler werden fuer HTTP-Antworten generisch ausgegeben, damit interne Pfade oder Werkzeugdetails nicht im Browser landen.

Der E-Mail-Import verarbeitet erlaubte IMAP-Nachrichten mit PDF-Anhaengen oder angehaengten `.eml`-Dateien. PDF-Anhaenge werden wie normale Uploads importiert. Jede EML-Datei wird zu einem Archiv-PDF mit unabhaengig erzeugtem Metadaten-Deckblatt, gerenderter sicherer Mailabbildung und anschliessenden PDF-Anhaengen aus der EML; andere Anhaenge werden auf dem Deckblatt gelistet. Die Mailabbildung benoetigt `chromium`; das Zusammenfuehren von Deckblatt, Mailabbildung und PDF-Anhaengen benoetigt `pdfunite` aus `poppler-utils`.

Berechtigungen sind capability-basiert. `documents_read` darf lesen und WebDAV lesen, `documents_editor` darf zusaetzlich hochladen und Metadaten bearbeiten, `documents_manager` darf ausserdem loeschen und Struktur-Daten wie Tags, Felder und Suchfavoriten pflegen. Benutzer ohne Struktur-Recht koennen nur vorhandene Dokument-Tags zuweisen; neue Tags werden bei Dokument-Metadaten, Batch-Tagging und WebDAV-Uploads abgelehnt.

Die Ordneransicht ist virtuell: Ordner entstehen aus Dokument-Tags, Feldwerten und Suchfavoriten, nicht aus einem beschreibbaren Dateisystembaum. WebDAV bildet diese virtuelle Struktur ab. `PROPFIND`, `GET`, `HEAD` und `PUT` sind unterstuetzt; `DELETE`, `MKCOL`, `MOVE`, `COPY`, `LOCK`, `UNLOCK`, `PROPPATCH`, `PATCH` und `POST` werden als read-only abgelehnt. `PUT` importiert neue Dateien in den Zielordner und uebernimmt vorhandene Tag-Ordner als Initial-Tags, ueberschreibt aber keine existierenden Ressourcen.

## Fotomodul

Das Fotomodul ist optional und nutzt einen directory-first Ansatz: BearStack importiert die Fotos nicht in die Dokumentenablage, sondern rendert ein vorhandenes, read-only Fotoverzeichnis als Galerie.

Minimal in `.env`:

```env
BEARSTACK_PHOTOS_ENABLED=true
BEARSTACK_PHOTOS_DIR=/srv/photos
BEARSTACK_PHOTOS_DATA_DIR=/var/lib/bearstack/photos
```

`BEARSTACK_PHOTOS_DIR` ist das vorhandene read-only Fotoverzeichnis. `BEARSTACK_PHOTOS_DATA_DIR` ist der BearStack-eigene Fotobereich fuer erzeugte Dateien; standardmaessig liegen darunter `thumbnails/` und die separate Foto-Indexdatenbank `photos.db`. Bei Bedarf koennen `BEARSTACK_PHOTOS_CACHE_DIR` und `BEARSTACK_PHOTOS_DB_PATH` einzeln ueberschrieben werden.

Nach dem Neustart erscheint `Fotos` in der Hauptnavigation. Unterstuetzt werden Ordnernavigation, Bild- und Videowiedergabe (`jpg`, `jpeg`, `png`, `gif`, `webp`, `svg`, `mp4`, `webm`, `ogv`, `ogg`), On-demand-Thumbnails fuer JPEG/PNG/GIF, eine separate SQLite-Foto-DB fuer den Metadatenindex, EXIF-Aufnahmedatum und -zeit/Kamera/GPS fuer JPEGs, eine Info-Seitenleiste mit vollstaendigem Aufnahmezeitpunkt, Adobe/MWG-XMP-Gesichtsregionen, BearStack-Tags auf Ordnern und Medien, Umlaut-tolerante Suche mit Feldfiltern wie `date:2024`, `directory:Urlaub`, `file_name:IMG`, `type:image`, `gps:true`, `tag:urlaub`, `person:"Marie Curie"` und `face:Marie`, eine Kartenansicht, die die verfuegbare Fensterhoehe ausnutzt, mit konfigurierbarer Foto-Track-Aufloesung, Markdown-Blogdateien, GPX-Hinweise, Zufallslink und Fotoframe.

Beim Zurueckkehren aus einem Unterordner ueber den Fotopfad oder Browser-Zurueck/Vorwaerts erscheint die Ordnerliste wieder an der vorherigen Scrollposition. Pfadlinks behalten dabei die zuvor besuchte Sortierung, Filter und Seite bei. Die Positionen werden innerhalb des aktuellen Tabs gespeichert.

Ordnerthumbnails zeigen Bilder bei 20 %, 40 %, 60 % und 80 % der sichtbaren Bilder einschliesslich Unterordnern, sortiert nach Datum absteigend (Aufnahmedatum, ersatzweise Dateiaenderungsdatum). Bruchteile einer Bildposition werden aufgerundet: Bei 100 Bildern werden Bild 20, 40, 60 und 80 verwendet. Ordner mit bis zu vier Bildern zeigen jedes Bild hoechstens einmal; eine kleinere konfigurierte Vorschauanzahl nutzt die ersten dieser Positionen. Ordner ohne Bilder behalten Video-/Audiovorschauen. Bestehende Vorschauzuordnungen werden automatisch erneuert.

Der Zufallsendpunkt `/photos/random` liefert standardmaessig das Original direkt aus. Mit `size=original` bleibt es beim Original; mit `size=ordner`, `size=galerie`, `size=gross`/`size=gro%C3%9F` oder `size=hd` wird stattdessen die jeweilige konfigurierte Thumbnailgroesse ausgeliefert.
Zusaetzlich liefert der Endpunkt Metadaten als Response-Header: `X-BearStack-Photo-Title` (Titel), `X-BearStack-Photo-Path` (Medienpfad), `X-BearStack-Photo-Folder-Path` (Ordnerpfad), `X-BearStack-Photo-Folder-URL` (absolute Ordner-URL inkl. Domain), `X-BearStack-Photo-Folder-Title` (Ordnername) sowie `Link: <https://.../photos?...>; rel="up"` als standardisierter Parent-Link.

Gesichtsdaten werden aus eingebettetem JPEG-XMP sowie XMP-Sidecars gelesen (`photo.jpg.xmp`, `photo.jpg.XMP`, `photo.xmp`, `photo.XMP`). BearStack speichert Namen und normalisierte Gesichtsboxen im Fotoindex und liefert sie in der Foto-JSON-API aus. Sie bleiben eigene Metadaten: Gesichtsnamen werden nicht automatisch zu Foto-Tags, erscheinen nicht in Tag-Listen und sind gezielt ueber `person:` oder `face:` suchbar.

Unter `Einstellungen -> Fotos` kann die Foto-Track-Aufloesung der Karte in sinnvollen Stufen von 500 m bis 10 km eingestellt werden. Sie legt fest, wie nah GPS-Fotos liegen muessen, um im fotobasierten Karten-Track zu einem Trackpunkt zusammengefasst zu werden. Dort kann auch ein Index-Worker aktiviert werden. Er crawlt den Foto-Root ordnerweise im Hintergrund, ueberspringt unveraenderte Ordner anhand ihres letzten Scan-Zeitpunkts und entfernt nicht mehr vorhandene Foto-, Ordner- und Blogeintraege ordnerlokal aus dem Index. Standardmaessig ist er deaktiviert; bei Aktivierung laeuft er alle 60 Minuten mit niedriger I/O-Prioritaet, falls vom System unterstuetzt, und 250 ms Pause pro gescanntem Ordner. Der separate Thumbnail-Worker ist ebenfalls standardmaessig deaktiviert; bei Aktivierung laeuft er alle 15 Minuten, erzeugt standardmaessig bis zu 15 fehlende Thumbnails pro Lauf und nutzt standardmaessig eine Thumbnail-Parallelitaet von 1.

Bei Stat- oder Blog-Lesefehlern bleibt der Index des betroffenen Ordners einschließlich manueller Tags unverändert. Der Scan wird später erneut versucht. Vollständige Scans entfernen tatsächlich verschwundene Einträge; zusammengehörige Metadaten, Tagzuordnungen und Such-/Vorschauindizes werden je Löschpaket beziehungsweise Ordnerteilbaum gemeinsam zurückgerollt, falls die Bereinigung scheitert oder abgebrochen wird.

Foto-Info-Batchanfragen bündeln Metadaten- und Gesichtsabfragen in Paketen und teilen sich die Rechteprüfung gemeinsamer Elternordner. Dateistand und XMP-Sidecars werden weiterhin geprüft; aktuelle `.adminonly`-Markierungen und geschützte Gesichtsnamen bleiben berücksichtigt.

Ordner koennen nach Ordnerstandard, Name, Datum oder zufaellig sortiert werden. Die Datumssortierung von Ordnern nutzt das aus dem Ordnernamen erkannte Anzeigedatum. Die Ordnerstandard-Sortierung wird ueber eine leere Steuerdatei im Ordner gesetzt: `.order_descending_name.pg2conf`, `.order_ascending_name.pg2conf`, `.order_descending_date.pg2conf`, `.order_ascending_date.pg2conf` oder `.order_random.pg2conf`.

Foto-Tags, Tagzuordnungen und Volltextsuche werden gemeinsam in einer Transaktion aktualisiert. Gleichzeitiges Hinzufügen oder Entfernen von Tags auf mehreren Fotos erhält parallele Änderungen; bei Fehlern wird die gesamte Bulk-Aktion zurückgerollt. Laufende Indexscans erhalten den aktuellen Tagstand. Tag-Umbenennungen und -Löschungen lesen über die Tagindizes nur betroffene Einträge.

Die Anzahl und Größe der Foto-Thumbnail-Dateien werden beim Start und anschließend alle 30 Minuten im Hintergrund ermittelt. Die Statistikseite zeigt den Messzeitpunkt und durchsucht den Thumbnail-Cache bei Seitenaufrufen nicht. Vor der ersten Messung erscheint „Thumbnail-Cache wird ermittelt“; bei einem fehlgeschlagenen oder abgebrochenen Durchlauf bleibt der letzte vollständige Stand erhalten. Die Messung läuft auch bei deaktivierter Thumbnail-Erzeugung.

Ein Ordner mit der Datei `.adminonly` ist nur fuer Benutzer mit der Rolle `admin` zugaenglich. Admin-only-Inhalte sind auch fuer Admins standardmaessig in Galerie, Suche, Zufall, Fotoframe, Kartenansicht und Foto-Tag-Listen ausgeblendet. Admins koennen sie im Sortieren-Menue der Galerie per Schalter einblenden; die Auswahl bleibt in der aktuellen Session gespeichert. Direkte Medien- und Thumbnail-URLs bleiben weiterhin nur Admins vorbehalten.

GPX-Dateien werden bis 16 MiB und 100.000 eingelesene Track-/Routenpunkte verarbeitet; größere oder fehlerhafte Dateien werden übersprungen. Wartezeiten und Lesen reagieren auf einen Anfrageabbruch. Je Foto-Library läuft höchstens ein Parser gleichzeitig. Eine Kartenantwort enthält höchstens 256 Tracks und 250.000 Punkte; weitere Tracks, die das Budget überschreiten, werden ausgelassen. Der GPX-Cache hat ein Speicherbudget von 32 MiB für Punktarrays und Eintragskosten und verdrängt die am längsten ungenutzten Tracks.

Gültige Foto-Thumbnail-Cachetreffer lesen ihre Metadaten ohne SQLite-Schreibaufruf. Nur fehlende oder zu reparierende Queue-Metadaten werden nachgetragen; ein paralleler Datenbank-Writer blockiert reguläre Cachetreffer nicht.

### Optionale lokale Gesichtserkennung

Ab 0.24.0 kann BearStack Gesichter automatisch erkennen und ähnliche Gesichter zu
Personen gruppieren. Die Funktion ist standardmäßig aus. Unter **Fotos → Personen**
sehen Fotoleser die Gruppen; Fotobearbeiter (`photos.edit`, Rolle „Fotos bearbeiten“) können Namen vergeben,
Gruppen zusammenführen, ausgewählte Gesichter in andere oder neue Gruppen verschieben
und Fehlfunde ignorieren. Der Filter „Ignorierte Gesichter“ unter `/photos/people` zeigt einzelne ignorierte Gesichter, auch aus vollständig ausgeblendeten Gruppen. Er lässt sich mit Namenssuche und „Nur bekannte Personen“ kombinieren und bleibt beim Blättern erhalten. Fotobearbeiter können ein Gesicht direkt mit „Benennen und wiederherstellen“ einer neuen benannten Person zuordnen; dabei wird nur dieses Gesicht wieder sichtbar. Die Liste verwendet die vorhandenen Gesichtsvorschauen aus dem Cache und lädt höchstens 60 Gesichter pro Seite. Mit „Nur bekannte Personen“ zeigt die Personenübersicht ausschließlich benannte Gruppen; Namenssuche und Seitenwechsel behalten den Filter bei. Fotobearbeiter können Personen per Checkbox ohne sichtbaren Beschriftungstext auswählen; für Screenreader bleibt die personenbezogene Beschriftung erhalten. Ab zwei ausgewählten Gruppen erscheint unten rechts „Personen zusammenführen“ mit dem Zielnamen. Benannte Gruppen haben beim Zusammenführen Vorrang vor unbenannten: Die zuerst ausgewählte benannte Person bleibt samt Namen erhalten. Sind alle Gruppen unbenannt, bleibt die zuerst ausgewählte Person erhalten; alle ausgewählten Gruppen werden atomar per AJAX zusammengeführt und die Übersicht ohne Seitenreload aktualisiert. Die Auswahl gilt für die aktuelle Seite. Fotobearbeiter können in der Personenübersicht mit „×“ das angezeigte Gesicht ohne Seitenreload ignorieren. Weitere Gesichter der Gruppe bleiben erhalten; Vorschaubild und Fotoanzahl aktualisieren sich automatisch. Die Fotoinfo verlinkt erkannte Personen und trennt mehrere Namen mit „·“. Die Kopfaktionen der Personenseiten erscheinen als Navigationsbuttons. In der Personenübersicht öffnet das dezente Stiftsymbol in der unteren rechten Kartenecke „Benennen / zuordnen“ für Fotobearbeiter einen Dialog mit Namensvorschlägen und „Neu anlegen“. Ein neuer Name benennt die gesamte Gruppe; die Auswahl einer vorhandenen Person führt beide Gruppen zusammen. Speichern und Aktualisieren erfolgen ohne Seitenreload; Suchfilter und Seitenposition bleiben erhalten. Fehler werden im Dialog angezeigt. Über „Auswahl benennen / zuordnen“ lassen sich markierte Gruppen gemeinsam bearbeiten: Ein neuer Name führt sie unter diesem Namen zusammen; eine vorhandene benannte Person wird zum gemeinsamen Ziel. Eine bereits markierte Person kann ebenfalls Ziel sein. Die gesamte Änderung erfolgt atomar. Im Benenn-Modal speichert ein Klick auf einen Namensvorschlag oder „Neu anlegen“ sofort; dasselbe gilt für die Auswahl mit Pfeiltasten und Enter. Dies funktioniert auch für mehrere markierte Gruppen ohne Seitenreload. Autovervollständigungen zeigen ausschließlich benannte Personen; die Suche filtert diese bereits auf dem Server. Auf Personendetailseiten kombiniert das Feld „Name“ Benennen und Zusammenführen mit AJAX-Vorschlägen (Name, Personen-ID und Fotoanzahl): Ein frei eingegebener Name oder „Neu anlegen“ benennt die aktuelle Gruppe; die Auswahl einer vorhandenen Person wechselt den Button zu „Gruppen zusammenführen“. Die aktuelle Gruppe wird nicht als Ziel angeboten. Beim Verschieben bietet „Zielperson suchen“ weiterhin Autovervollständigung. Eine Person lässt sich per Klick oder mit Pfeiltasten und Enter auswählen; Escape schließt die Vorschläge. Änderungen am Suchtext verwerfen die vorherige Auswahl. Beim Verschieben erzeugt ein leeres Zielfeld eine neue Person. Auf Mobilgeräten stehen die Felder und Aktionen zum Benennen, Zusammenführen und Verschieben untereinander in voller Breite. Die Paginierung bietet „Erste Seite“, „Zurück“, „Weiter“ und „Letzte Seite“ sowie die aktuelle Seite und Gesamtseitenzahl. Nicht verfügbare Randaktionen sind deaktiviert. Die Personenübersicht merkt sich Seite und Filter im Browser pro Benutzer, auch über Browser-Neustarts hinweg. Beim erneuten Öffnen von `/photos/people` wird diese Ansicht wiederhergestellt; ausdrücklich angegebene Seiten oder Suchfilter haben Vorrang. Nicht mehr vorhandene Seiten werden auf die letzte verfügbare Seite begrenzt. Dies gilt auch für AJAX-Aktualisierungen; Personendetailseiten und ignorierte Gesichter haben passende Gesamtseitenzahlen. Auf Personenseiten bleiben Vorschaubilder quadratisch und lange Dateipfade brechen innerhalb der Karte um. Beim Überfahren wird der Dateipfad unterstrichen, die Bildfläche bleibt unverändert. Auf einer Personenseite öffnen Gesichtsbild und Dateiname die Foto-Lightbox; dort stehen Navigation, Zoom und Bildinformationen zur Verfügung. `person:Juergen`
und `face:"Marie Curie"` suchen sowohl XMP-Namen als auch benannte automatische Gruppen.

Gesichtsvorschauen werden unter `BEARSTACK_PHOTOS_CACHE_DIR/faces/v1` als JPEG zwischengespeichert, auch über Neustarts hinweg. Aktive Gesichtsvorschauen bleiben ohne Größen- oder Anzahlbegrenzung gespeichert. Beim Ignorieren erhalten vorhandene Vorschauen eine feste Ablaufzeit von höchstens 48 Stunden; erneute Aufrufe oder wiederholtes Ignorieren verlängern diese Frist nicht. Abgelaufene Vorschauen werden auch ohne weitere Zugriffe automatisch gelöscht. Ein späterer Aufruf kann eine neue Vorschau erzeugen, die ebenfalls höchstens 48 Stunden gespeichert wird. Gesichtsdaten, Embeddings und die Möglichkeit zur Wiederherstellung bleiben dauerhaft erhalten. Ignorierte Gesichter dienen zu keinem Zeitpunkt als Referenzen für den Gesichtsabgleich. Beim Wiederherstellen entfällt die Ablaufzeit der Vorschau. Die Bereinigung läuft unabhängig von der aktivierten Gesichtserkennung, berücksichtigt Neustarts und verarbeitet nur fällige Cacheeinträge in kleinen, indexgestützten Paketen. Beim Update bleibt der bestehende Cache unter `faces/v1` erhalten. Die Zuordnung zu Gesichts-IDs wird einmalig im Hintergrund in Paketen von höchstens 100 Gesichtern ergänzt und nach Unterbrechungen fortgesetzt. Dazu werden vorhandene Indexdaten und die beiden möglichen Vorschaupfade geprüft; Originalbilder werden weder gelesen noch neu dekodiert. Bereits vor Abschluss der Migration werden vorhandene Vorschauen beim Aufruf direkt übernommen. Dateinamen, Bildinhalt und Änderungszeiten bleiben erhalten. Ignorierte Altvorschauen erhalten eine feste Frist von höchstens 48 Stunden ab dem Update, die bei Neustarts nicht verlängert wird. Nicht mehr zuordenbare Altdateien werden nicht pauschal gelöscht. Hashbasierte Unterverzeichnisse vermeiden übergroße Einzelordner; Cache-Zugriffe benötigen keinen Verzeichnisscan und keine vollständige Dateiliste im Arbeitsspeicher. Die Größen 160 und 640 Pixel werden getrennt gespeichert; parallele Abrufe desselben Ausschnitts teilen sich eine Erzeugung. Personenübersicht, Personendetailseite, AJAX und Android-/Labeling-API verwenden denselben Datei-Cache: Vorhandene Vorschauen werden direkt daraus ausgeliefert; fehlt eine Vorschau, wird sie erzeugt und dort gespeichert. Der Browser darf Web-Gesichtsvorschauen privat speichern, muss sie aber vor erneuter Verwendung validieren (ETag/304). Authentifizierung, aktuelle Sichtbarkeit und Quelldatei werden weiterhin geprüft. Bei AJAX-Aktualisierungen bleiben vorhandene Bild-Elemente erhalten; Änderungen an Namen oder Fotoanzahlen laden unveränderte Bilder nicht erneut.

Unter **Einstellungen → Gesichtserkennung** stehen Statuszahlen, Verarbeitung und
Löschaktion in getrennten Bereichen. Die Einstellungsnavigation steht auf großen
Bildschirmen links und ist auf kleinen Bildschirmen als horizontale Reiterleiste erreichbar.

Die Verarbeitung erfolgt in einem separaten lokalen Dienst mit OpenCV/YuNet/SFace.
Bilder verlassen die eigene Infrastruktur nicht. Der Dienst hat weder Zugriff auf
das Fotoverzeichnis noch auf die Datenbank. BearStack sendet ausgerichtete, auf
höchstens 1.600 Pixel verkleinerte JPEGs. Modelle werden bei der Einrichtung beziehungsweise beim Image-Build mit festen
SHA-256-Prüfsummen geladen und beim Start geprüft; im Betrieb gibt es keine Downloads.

**Einrichtung mit Compose:**

1. Einen zufälligen Token erzeugen, beispielsweise mit `openssl rand -hex 32`.
2. In `.env` `BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN` auf diesen Token setzen.
3. Fotomodul und Fotoverzeichnis wie oben konfigurieren und
   `docker compose --profile faces up -d --build` starten.
4. Unter **Einstellungen → Gesichtserkennung** die Verarbeitung einschalten.
   Aktivierung ist nur bei erreichbarem, kompatiblem Dienst möglich.

Compose verwendet intern `http://faces:8091`, ohne veröffentlichten Dienstport.
Eine eigene Dienstadresse lässt sich über folgende optionale Werte setzen:

| JSON-Feld unter `photos` | Umgebungsvariable | Bedeutung |
| --- | --- | --- |
| `face_service_url` | `BEARSTACK_PHOTOS_FACE_SERVICE_URL` | HTTP(S)-Adresse des eigenen Erkennungsdienstes. |
| `face_service_token` | `BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN` | Gemeinsamer geheimer Token, mindestens 32 Zeichen. |

#### Einrichtung ohne Compose (native Installation)

BearStack und der Erkennungsdienst können auf demselben Rechner direkt laufen.
Voraussetzungen sind ein eingerichtetes Fotomodul, die BearStack-Quelldateien passend
zur installierten Version, **Python 3.12 oder neuer mit `venv`**, OpenSSL und curl.
Eine GPU ist nicht erforderlich. Die folgenden Befehle werden im BearStack-Projektordner
ausgeführt; für die Installation der Python-Pakete und Modelle wird Internetzugang benötigt.

1. **Python-Umgebung und Modelle installieren:**

    ```sh
    python3 --version
    python3 -m venv .venv-faces
    .venv-faces/bin/python -m pip install --only-binary=:all: -r services/faces/requirements.txt
    .venv-faces/bin/python services/faces/download_models.py "$HOME/.local/share/bearstack-face-models"
    ```

    Fehlt `venv`, das zur Python-Version passende Paket des Betriebssystems installieren
    (unter Debian/Ubuntu beispielsweise `python3-venv`). Das Downloadskript lädt die
    festgelegten Modelle samt Lizenzhinweisen und prüft ihre SHA-256-Prüfsummen.
    Die Modelldateien bleiben lokal erhalten; dieser Schritt ist nur bei der Einrichtung
    oder einem vorgesehenen Modellupdate nötig.

2. **Gemeinsamen Token erzeugen:**

    ```sh
    openssl rand -hex 32
    ```

    Den erzeugten Wert in den folgenden Beispielen anstelle von `TOKEN_HIER_EINTRAGEN`
    verwenden. Erkennungsdienst und BearStack müssen denselben Token erhalten.

3. **Erkennungsdienst in einem eigenen Terminal starten:**

    ```sh
    export BEARSTACK_FACE_MODELS_DIR="$HOME/.local/share/bearstack-face-models"
    export BEARSTACK_FACE_SERVICE_TOKEN='TOKEN_HIER_EINTRAGEN'
    export BEARSTACK_FACE_BIND=127.0.0.1
    .venv-faces/bin/python services/faces/server.py
    ```

    Der Dienst lauscht auf `127.0.0.1:8091`. Er benötigt Leserechte auf die Modelle,
    aber keine Rechte auf Fotoverzeichnis oder BearStack-Datenbank. Das Terminal bleibt
    während des Betriebs geöffnet. Der Python-Dienst liest `.env` **nicht** automatisch;
    seine Variablen müssen in der Prozessumgebung gesetzt sein.

4. **Erreichbarkeit in einem zweiten Terminal prüfen:**

    ```sh
    export BEARSTACK_FACE_SERVICE_TOKEN='TOKEN_HIER_EINTRAGEN'
    curl --fail --silent --show-error \
      -H "Authorization: Bearer $BEARSTACK_FACE_SERVICE_TOKEN" \
      http://127.0.0.1:8091/health
    ```

    Die JSON-Antwort muss `"ready": true`, `"protocol": 1` und die Modellkennung
    `yunet-2023mar-sface-2021dec-v1` enthalten. Bei HTTP 401 stimmt der Token nicht;
    bei einem Verbindungsfehler zuerst das Dienstterminal prüfen.

5. **BearStack verbinden und neu starten:** Die folgenden Werte in die von BearStack
    gelesene `.env`-Datei beziehungsweise seine Prozessumgebung eintragen:

    ```env
    BEARSTACK_PHOTOS_FACE_SERVICE_URL=http://127.0.0.1:8091
    BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN=TOKEN_HIER_EINTRAGEN
    ```

    Bei der systemd-Beispielinstallation können stattdessen die Felder
    `face_service_url` und `face_service_token` im vorhandenen `photos`-Objekt in
    `/etc/bearstack/bearstack.json` ergänzt werden. Danach `sudo systemctl restart bearstack`
    ausführen. Die übrige Foto-Konfiguration beibehalten und die Datei mit dem Token
    nur für den Administrator und den jeweiligen Dienst lesbar halten.

6. **Verarbeitung aktivieren:** Als Foto-Verwalter unter **Einstellungen → Gesichtserkennung**
    einschalten. Dort erscheinen Fortschritt und Fehler; die Ergebnisse stehen unter
    **Fotos → Personen** bereit. Der vorhandene Fotobestand wird schrittweise verarbeitet.

Für dauerhaften Betrieb muss der Python-Prozess ebenfalls durch einen Dienstmanager
wie systemd gestartet werden. Dabei einen absoluten Pfad zu `.venv-faces/bin/python`,
`services/faces/server.py` und den Modellen verwenden sowie die drei Dienstvariablen
aus Schritt 3 setzen. BearStack startet diesen Prozess nicht selbst. Die Begrenzungen
auf einen halben CPU-Kern und 1 GiB RAM gelten nur für die Compose-Konfiguration;
bei nativem Betrieb müssen entsprechende Ressourcenlimits im Dienstmanager gesetzt werden.
Der einzelne Inferenzthread und die BearStack-Pausen gelten auch ohne Compose.

#### Verarbeitung, Metadaten und Datenschutz

**Schonender Betrieb:** Ein Worker verarbeitet standardmäßig 100 Bilder, mit einer
Sekunde Pause pro Bild und 15 Minuten Wartezeit zwischen Läufen. Der Dienst nutzt
einen Inferenzthread; Compose begrenzt ihn auf 0,5 CPU-Kerne und 1 GiB RAM. Die
Einstellungen erlauben 1–1.000 Bilder pro Lauf, 100–60.000 ms Pause und 1–1.440 Minuten
Wartezeit. Der Erstlauf holt die Sammlung schrittweise nach. Wenn der separate
Fotoindex-Worker aus ist, nutzt die Gesichtserkennung dessen Scanmechanismus stündlich.
Nach dem Erstlauf entstehen neue Aufträge direkt bei Indexänderungen. Unveränderte
Bilder einschließlich Ergebnissen ohne Gesicht werden nicht erneut analysiert.
Fehler werden mit zunehmender Wartezeit bis zu fünfmal versucht und können manuell
zurückgesetzt werden. Neustarts setzen die persistente Warteschlange fort.

Die Info-Symbole in den Einstellungen der Gesichtserkennung erklären Aktivierung, Bilder pro Lauf, Pausen, Referenzlimit und das Löschen der Gesichtsdaten. Die Kontexthilfen lassen sich per Klick oder Tastatur öffnen. Zum Löschen aller Gesichtsdaten sind die Löschbestätigung und das aktuelle Passwort des angemeldeten Nutzers erforderlich. Das Passwort wird vor jeder Zustandsänderung serverseitig geprüft; fehlende oder falsche Bestätigungen lassen Gesichtsdaten und Verarbeitung unverändert. Wiederholte Fehlversuche werden gedrosselt.

**Personenrechte:** Seit BearStack 0.36.0 reicht „Fotos bearbeiten“ (`photos_editor` bzw. `photos.edit`) zum Benennen, Zuordnen, Zusammenführen, Ignorieren und Wiederherstellen von Gesichtern sowie für die Android-App. Einstellungen der Gesichtserkennung und das Löschen aller Gesichtsdaten benötigen weiterhin „Fotos verwalten“ (`photos.manage`).

**Referenzen pro Person:** Unter **Einstellungen → Gesichtserkennung** lässt sich
die Zielanzahl auf 1–100 einstellen; der Standard ist **30**. Seit 0.40.0 werden
Vergleichsbilder möglichst über verschiedene Galerieordner verteilt. Innerhalb
eines Ordners haben manuelle Zuordnungen Vorrang, danach die Erkennungssicherheit.
Die Auswahl ist bei unveränderten Daten stabil.

**Favorisierte Gesichter:** In der Web-Detailansicht einer benannten oder unbenannten
Person können Fotobearbeiter einzelne Gesichter mit einem Stern favorisieren.
Alle aktiven Favoriten werden bei jedem Abgleich vollständig verglichen, auch wenn
ihre Anzahl das eingestellte Limit übersteigt. Bei weniger Favoriten füllen
unfavorisierte Gesichter bis zur Zielanzahl auf, sofern vorhanden. Dabei haben
bisher nicht vertretene Ordner Vorrang; Favoriten zählen bereits für ihren Ordner.
Mehr Favoriten benötigen mehr Arbeitsspeicher und Rechenzeit pro neu erkanntem Gesicht.
Die Aktion speichert sofort, ohne Seitenreload oder erneute Bildanalyse.
Favoriten bleiben nach Neustart, Zuordnen und Zusammenführen erhalten.
Ignorierte und private Gesichter werden weiterhin ausgeschlossen. Wiederhergestellte
Gesichter behalten ihren Stern; bei erneuter Erkennung wird er nur auf eine eindeutig
wiedergefundene Region übertragen. Gelöschte oder ersetzte Fotos verlieren ihre
veralteten Gesichtsdaten einschließlich der Favoriten.

Änderungen der Zielanzahl und das Update bestehender Referenzen werden vor der
nächsten Analyse in kurzen, fortsetzbaren Schritten übernommen. Bestehende Gruppen
werden dadurch nicht automatisch zusammengeführt. Die Android-Oberfläche erhält
vorerst keine Sterne. Für spätere Clients stehen `GET` und `PUT` unter
`/api/photos/labeling/v1/faces/{id}/favorite` bereit (`photos.edit`). Der PUT-Body
enthält `person_id` und den gewünschten booleschen Wert `favorite`; Wiederholungen
schalten den Zustand nicht erneut um. Eine inzwischen geänderte Gruppenzuordnung
oder ein ignoriertes Gesicht führt zu `409`. Personendetails liefern `faces[].favorite`,
die Sitzung meldet Unterstützung über `face_favorites: true`. Alte Server ohne dieses
Feld unterstützen die Erweiterung nicht. Details und Fehlerantworten stehen in der
[OpenAPI-Beschreibung](openapi.yaml).

**Metadaten und Korrekturen:** Eindeutige XMP-Gesichtsregionen liefern Namen und
Referenzen. Manuelle Zuordnungen haben Vorrang. XMP und automatische Gesichter werden
getrennt gespeichert; Originale und Sidecars werden nicht verändert. Die Foto-DB
migriert automatisch auf Schema 22. Ihre Sicherung muss die erzeugten Gesichtsdaten
sowie manuelle Korrekturen und Favoriten einschließen. Ein Index-Neuaufbau erhält die Korrekturen
unveränderter Bilder; Dateiaustausch und Löschung entfernen veraltete Analysen.
Bei einem Modellwechsel werden manuelle Zuordnungen nur auf eindeutig wiedergefundene
Regionen übertragen. Unsichere Treffer bleiben getrennt. Das Zusammenführen bestehender
Gruppen erfolgt ausschließlich manuell; die Erkennung stellt keine sichere Identitätsfeststellung dar.

**Sichtbarkeit und Aufbewahrung:** `.adminonly`-Fotos sind von automatischer Analyse
und Personengruppen ausgeschlossen. Nachträgliche Schutzmarkierungen entfernen deren
automatische Ergebnisse und Referenzen. Gesichtsvorschauen prüfen die aktuellen Rechte.
Pausieren oder Ausschalten behält bisherige Ergebnisse. Die separate, ausdrücklich
zu bestätigende Löschaktion entfernt erzeugte Gesichtsdaten, Gruppen und Korrekturen
und schaltet die Verarbeitung aus; importierte XMP-Gesichtsdaten bleiben erhalten.
Merkmalsvektoren erscheinen weder in HTTP-Antworten an Browser noch in Logs.

V1 unterstützt JPEG, PNG, WebP und das erste GIF-Bild. Videos und SVG werden nicht
analysiert. Bilder über 40 Megapixel werden zur Begrenzung des Speichers zurückgewiesen;
sehr kleine, verdeckte oder durch die Verkleinerung zu kleine Gesichter können fehlen.

## Auth

Fuer den Betrieb wird Basic Auth mit bcrypt-Hash empfohlen:

```sh
sudo apt install -y apache2-utils
htpasswd -bnBC 10 bearstack 'mein-passwort' | cut -d: -f2
```

Den Hash in JSON direkt eintragen oder in Shell-/Env-Dateien wegen der `$`-Zeichen quoten:

```env
BEARSTACK_AUTH_PASSWORD_HASH='$2a$10$...'
```

Wenn `password_hash` gesetzt ist, hat er Vorrang vor `password`. Nach erfolgreichem Basic-Auth-Login setzt BearStack ein signiertes HttpOnly-Session-Cookie fuer 12 Stunden. Mit der Login-Checkbox "Eingeloggt bleiben" wird die Session auf 30 Tage verlaengert. Der Signierschluessel liegt persistent im Datenverzeichnis, sodass unveraenderte Konten auch nach einem Neustart angemeldet bleiben. Beim Upgrade auf BearStack 0.22.0 ist wegen des neuen Sessionformats einmalig eine erneute Anmeldung erforderlich.

Optional kann `auth.credentials` mehrere Basic-Auth-Credentials mit unterschiedlichen Rollen definieren. Sobald diese Liste gesetzt ist, werden `auth.username`, `auth.password` und `auth.password_hash` ignoriert:

```json
"auth": {
  "realm": "BearStack",
  "credentials": [
    {"username": "admin", "password_hash": "$2a$10$...", "role": "admin"},
    {"username": "dav", "password_hash": "$2a$10$...", "role": "documents_read"},
    {"username": "photos", "password_hash": "$2a$10$...", "role": "photos_read"}
  ]
}
```

Rollen: `admin`, `documents_read`, `documents_editor`, `documents_manager`, `photos_read`, `photos_editor`, `photos_manager`, `api_uploader`. Statt oder zusaetzlich zu `role` koennen `permissions` gesetzt werden, z. B. `documents.read`, `documents.webdav.read`, `documents.upload`, `documents.edit`, `documents.delete`, `documents.structure`, `photos.read`, `photos.edit`, `photos.manage`, `system.manage`, `system.users.manage`, `system.audit`.

Admins verwalten zusaetzliche Konten unter `Einstellungen -> Benutzer`. Diese Konten liegen mit bcrypt-Hash in `bearstack.db`; Passwoerter muessen mindestens 12 Zeichen lang sein und duerfen die bcrypt-Grenze von 72 UTF-8-Bytes nicht ueberschreiten. Konten aus JSON oder Env bleiben parallel aktiv, werden im UI als `Konfiguration` angezeigt und dort nicht veraendert. Benutzernamen duerfen sich zwischen beiden Quellen nicht doppeln. Ein Konto mit `system.users.manage` darf normale Konten innerhalb seiner eigenen Fachrechte verwalten; nur die Rolle `admin` darf Admins oder weitere Nutzerverwalter verwalten.

Die PDF-Vorschau-Option zeigt Checkbox und normal geschriebene Beschriftung nebeneinander; auf schmalen Bildschirmen bricht die Beschriftung neben der Checkbox um. Die reine Darstellungsoption `BearStack PDF-Vorschau` kann ohne erneute Passwortbestaetigung im eigenen Konto oder durch einen berechtigten Nutzerverwalter gespeichert werden. Sie veraendert keine Rechte und widerruft keine Sitzung; Aenderungen an fremden Konten folgen den bestehenden Delegationsgrenzen und werden auditiert. Config-Zugangsdaten bleiben dabei weiterhin extern und schreibgeschuetzt.

Passwort-, Rechte- und Statusaenderungen widerrufen bestehende Sitzungen des betroffenen UI-Kontos sofort. Wiederholte Fehlanmeldungen werden pro Benutzername zeitlich begrenzt. Fuer den Notfall kann ein temporaeres Config-Admin-Konto mit eindeutigem Benutzernamen gesetzt, BearStack neu gestartet und damit ein UI-Passwort zurueckgesetzt werden. Backups sollten das gesamte Datenverzeichnis einschliesslich `bearstack.db` und `auth-session.key` sowie weiterhin verwendete Konfigurationsdateien enthalten.

BearStack startet ohne aktives Konto nur auf Loopback-Adressen. Bei `BEARSTACK_ADDR=0.0.0.0:8080`, `:8080` oder einem anderen nicht-lokalen Host muss mindestens ein aktives Config- oder SQLite-Konto vorhanden sein. Auch bei einem Reverse Proxy vor `127.0.0.1:8080` sollte Auth aktiv bleiben, wenn der Proxy keine eigene Zugriffskontrolle uebernimmt.

## TLS

Empfohlen fuer produktiven Betrieb: BearStack nur auf `127.0.0.1:8080` lauschen lassen und TLS in nginx oder Caddy terminieren.

Direktes HTTPS in BearStack ist moeglich:

```sh
BEARSTACK_TLS_ENABLED=1 go run ./cmd/bearstack
```

Mit `auto_cert=true` erzeugt BearStack ein lokales Self-Signed-Zertifikat unter `data/tls/`. Browser melden dieses Zertifikat als nicht vertrauenswuerdig. Fuer echte Zertifikate `tls.cert_file` und `tls.key_file` bzw. `BEARSTACK_TLS_CERT_FILE` und `BEARSTACK_TLS_KEY_FILE` setzen.

Wenn TLS aktiv ist, akzeptiert BearStack auf demselben `addr` sowohl HTTPS als auch plain HTTP. HTTP-Anfragen auf diesem Port werden mit einer permanenten Weiterleitung auf `https://` beantwortet.

## systemd

Die Beispiel-Unit nutzt eine JSON-Konfiguration:

```sh
sudo install -o root -g root -m 0755 bearstack /usr/local/bin/bearstack
sudo install -o root -g root -m 0644 deploy/bearstack.service /etc/systemd/system/bearstack.service
sudo systemctl daemon-reload
sudo systemctl enable --now bearstack
systemctl status bearstack
journalctl -u bearstack -n 100 --no-pager
```

Die Pfade in `ReadWritePaths` und `RequiresMountsFor` muessen zur eigenen Konfiguration passen.

## Backup

Sicherste Variante: Dienst kurz stoppen und Datenverzeichnis plus Konfiguration sichern.

```sh
sudo mkdir -p /var/backups/bearstack
sudo systemctl stop bearstack
sudo tar -czf "/var/backups/bearstack/bearstack-$(date +%Y%m%d-%H%M%S).tgz" \
  /var/lib/bearstack \
  /etc/bearstack/bearstack.json
sudo systemctl start bearstack
```

Wenn `storage_dir` ausserhalb von `data_dir` liegt, diesen Pfad ebenfalls sichern. Bei SQLite im WAL-Modus gehoeren `bearstack.db`, `bearstack.db-wal` und `bearstack.db-shm` zusammen; das Sichern des ganzen Datenverzeichnisses vermeidet Fehler.

## Restore

```sh
sudo systemctl stop bearstack
sudo mv /var/lib/bearstack "/var/lib/bearstack.before-restore-$(date +%Y%m%d-%H%M%S)"
sudo tar -C / -xzf /var/backups/bearstack/<backup-datei>.tgz
sudo chown -R bearstack:bearstack /var/lib/bearstack
sudo chown root:bearstack /etc/bearstack /etc/bearstack/bearstack.json
sudo chmod 640 /etc/bearstack/bearstack.json
sudo systemctl start bearstack
journalctl -u bearstack -n 100 --no-pager
```

Bei getrenntem Dokumentenspeicher auch diesen vor dem Restore verschieben und nach dem Entpacken die Rechte setzen.

## Update

Vor Updates ein Datenbackup erstellen. Fuer systemd-Installationen liegt ein Update-Skript im Repo:

```sh
cd /opt/bearstack-src/BearStack
./update.sh
```

`update.sh` aktualisiert den aktuellen Branch mit `git fetch --all --tags` und `git pull --ff-only`, führt `go test ./...` aus und baut ein temporäres Binary. Danach wartet es auf `systemctl stop` und prüft `ActiveState=inactive`, `Result=success` und `MainPID=0`. Bei einem Stop-Fehler, Timeout oder verbliebenen Hauptprozess bricht das Update vor der Installation ab. Nur nach erfolgreichem Stopp wird das Binary installiert und der Dienst gestartet. Den Dienststatus und Smoke-Test anschließend prüfen; automatisches Backup und Rollback sind nicht enthalten.

BearStack beendet bei SIGTERM und SIGINT zunächst die Annahme neuer Hintergrundjobs und signalisiert laufenden Workern den Abbruch. HTTP-Anfragen und Hintergrundjobs einschließlich manuell gestarteter Foto- und Gesichtsläufe haben zusammen bis zu 60 Sekunden zum Abschluss. Erst danach schließen die Datenbanken; ein überschrittenes Zeitlimit führt zu einem Fehlerstatus. Die systemd-Vorlage verwendet `TimeoutStopSec=75s`. Bestehende eigene Units sollten mindestens diesen Wert erhalten; anschließend `sudo systemctl daemon-reload` ausführen.

Variablen fuer das Skript:

| Variable | Standard |
| --- | --- |
| `BEARSTACK_REPO_DIR` | Verzeichnis des Skripts |
| `BEARSTACK_SERVICE` | `bearstack.service` |
| `BEARSTACK_INSTALL_PATH` | `/usr/local/bin/bearstack` |

Ohne Auth reicht fuer den manuellen Smoke-Test:

```sh
curl -fsS http://127.0.0.1:8080/healthz
```

Mit Auth:

```sh
curl -fsS -u admin:mein-passwort http://127.0.0.1:8080/healthz
```

Manuell entspricht das im Kern:

```sh
git fetch --all --tags
git pull --ff-only
make test
go build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
sudo systemctl stop bearstack
sudo install -o root -g root -m 0755 bearstack /usr/local/bin/bearstack
sudo systemctl start bearstack
curl -fsS -u admin:mein-passwort http://127.0.0.1:8080/healthz
```

## Typische Fehler

- `401 Unauthorized`: Auth ist aktiv; mit Basic-Auth-Daten anmelden.
- Start bricht mit `at least one active authentication account is required when addr listens on non-loopback interfaces` ab: Ein aktives Config-/SQLite-Konto bereitstellen oder `BEARSTACK_ADDR` auf `127.0.0.1:8080`/`localhost:8080` beschraenken.
- Das Log meldet `auth_enabled=false`: Es gibt weder ein aktives Config- noch ein aktives SQLite-Konto. Dieser Zustand ist nur auf Loopback zur Ersteinrichtung zulaessig.
- `open config file` oder `decode config file`: `BEARSTACK_CONFIG`-Pfad und JSON-Syntax pruefen.
- `permission denied` bei Datenbank oder Dokumenten: Besitzer/Rechte und `ReadWritePaths` der Unit pruefen.
- `address already in use`: Port mit `sudo ss -ltnp | grep ':8080'` pruefen.
- Upload scheitert mit `413`: `max_upload_bytes` und `client_max_body_size` im Reverse Proxy angleichen.
- Ein frischer Docker-Container startet auf einem nicht-lokalen Listener wegen fehlender Auth nicht: ein Config-Konto bereitstellen oder zuerst auf Loopback ein SQLite-Admin-Konto anlegen; bestehende DB-Konten im Daten-Volume werden ebenfalls akzeptiert.
- `make test-js` bricht mit `node is required...` ab: Node installieren oder `NODE=/pfad/zu/node make test-js` verwenden.
- `make test-playwright` scheitert beim Abruf des Test-Runners: `npm`-Zugang (Netzwerk/Proxy) pruefen oder mit lokalem Cache erneut starten.
- `make test-playwright` findet keinen Browser: Chrome installieren oder `npx playwright install chromium` ausfuehren und mit `PLAYWRIGHT_BROWSER_CHANNEL=chromium make test-playwright` starten.
- `go test`/`go build` meldet Schreibfehler im Go-Build-Cache: `GOCACHE` auf ein beschreibbares Verzeichnis setzen, z. B. `GOCACHE=/tmp/go-build-cache go test ./...`.
- `update.sh` startet den Dienst, aber Status oder manueller Smoke-Test schlagen fehl: `systemctl status bearstack` und `journalctl -u bearstack -n 100 --no-pager` pruefen; bei aktiver Auth den Smoke-Test mit `curl -u` ausfuehren.
- Vorschaubilder fuer PDFs fehlen: `poppler-utils` installieren, besonders `pdftoppm`.
- Text-/Office-Vorschau oder Volltextextraktion fuer TXT, Markdown, RTF, DOC, DOCX oder Pages fehlt: LibreOffice installieren und `command -v soffice` pruefen.
- Foto-/Video-Vorschaubilder fehlen: `ffmpeg` installieren; fuer Bild-Thumbnails kann optional `vipsthumbnail` genutzt werden.
- OCR scheitert: `tesseract-ocr`, Sprachpakete und `pdfinfo`/`pdftoppm` pruefen.
- Fotomodul fehlt in der Navigation: `BEARSTACK_PHOTOS_ENABLED=true` setzen und Dienst neu starten.
