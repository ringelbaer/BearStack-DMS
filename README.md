# BearStack

BearStack ist eine Go-Webanwendung zur Ablage und Verwaltung von Dokumenten. Die Daten liegen lokal in SQLite und im Dateisystem. Der Betrieb ist fuer kleine Server wie einen Raspberry Pi ausgelegt.

Diese Datei beschreibt die wichtigsten Admin-Punkte. Eine vollstaendige Pi-Installation steht in [deploy-raspberrypi.md](deploy-raspberrypi.md). Eine systemd-Beispiel-Unit liegt in [deploy/bearstack.service](deploy/bearstack.service).

## Schnellstart

Voraussetzung fuer Entwicklung und Build ist Go `1.25.x` oder eine kompatible neuere Version. Fuer `make test-js` wird zusaetzlich `node` benoetigt. Die optionalen Playwright-Smokes brauchen zusaetzlich `npm` und einen Playwright-kompatiblen Browser; lokal ist standardmaessig der Chrome-Channel konfiguriert.

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

Ohne vollstaendige Auth-Konfiguration ist Basic Auth auf Loopback-Adressen wie `127.0.0.1:8080` deaktiviert. Ein Listener auf nicht-lokalen Interfaces wie `0.0.0.0:8080` oder `:8080` startet nur mit gesetztem Benutzer plus Passwort oder Passwort-Hash.

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

Externe Werkzeuge sind je nach Funktion optional: `pdftoppm`/`pdfinfo` aus `poppler-utils` fuer PDF-Vorschau und OCR-Vorbereitung, `soffice` aus LibreOffice fuer Text-/Office-Vorschau und Volltextextraktion, `tesseract` plus Sprachpakete fuer OCR, `ffmpeg` fuer Video- und Fallback-Bild-Thumbnails sowie optional `vipsthumbnail` fuer Bild-Thumbnails.

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

`make test-go` fuehrt `go test ./...` aus. `make test-js` nutzt `scripts/check-js.sh` und fuehrt `node --check` fuer alle Browser-Skripte unter `internal/server/static/*.js` aus. `make test-playwright` ruft den Test-Runner ueber `npm exec` on-demand ab; die Tests erzeugen temporaere Daten, starten lokal `go run ./cmd/bearstack` mit eigener Testkonfiguration und pruefen Dokumenten-Upload sowie die Foto-Galerie. Die Make-Variablen `GO`, `NODE`, `NPM` und `PLAYWRIGHT_TEST_VERSION` koennen bei Bedarf ueberschrieben werden, z. B. `NODE=/opt/node/bin/node make test-js`. Falls kein lokaler Chrome-Channel verfuegbar ist, kann Playwright wie ueblich mit eigenem Browser-Download verwendet werden, z. B. `npx playwright install chromium` und `PLAYWRIGHT_BROWSER_CHANNEL=chromium make test-playwright`.

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

## Versionierung

Die Anwendungs-Version steht zentral in `VERSION` und wird in der Weboberflaeche dezent im Footer angezeigt. BearStack nutzt semantische Versionierung:

- `PATCH`: Bugfixes, Security-Fixes, Performance, Refactors ohne neues Verhalten, kleine UI-Korrekturen.
- `MINOR`: neue rueckwaertskompatible Funktionen, neue optionale Einstellungen, nicht-brechende API- oder UI-Faehigkeiten, automatische kompatible Migrationen.
- `MAJOR`: brechende Aenderungen an HTTP/API/WebDAV, Konfiguration, Datenformaten, Berechtigungen oder manuelle/incompatible Migrationen.

Bei mehreren Aenderungstypen gewinnt die hoechste Kategorie. Docs-only- und Test-only-Aenderungen erhoehen die Version nicht. Solange BearStack in `0.x` ist, fuehrt der erste echte Major Change auf `1.0.0`.

## Docker

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

Der Container lauscht intern auf `0.0.0.0:8080` und speichert Daten unter `/var/lib/bearstack`. Deshalb muss Auth im Container gesetzt sein; `compose.yaml` nutzt standardmaessig `admin` als Benutzer, reicht `BEARSTACK_AUTH_PASSWORD` und `BEARSTACK_AUTH_PASSWORD_HASH` durch und veroeffentlicht den Port ueber `BEARSTACK_PORT` oder sonst `8080`. Das Runtime-Image basiert auf `debian:trixie-slim`, die Build-Stage auf `golang:1.26-trixie`. Das Beispiel-Image enthaelt `ffmpeg`, `libreoffice-writer`, `poppler-utils`, `tesseract-ocr`, `tesseract-ocr-deu` und `tesseract-ocr-eng`, damit Foto-/Video-Vorschaubilder, PDF-/Office-Vorschauen, Text-/Office-Volltextextraktion und OCR im Container funktionieren. Bei aktiviertem Fotomodul muss ein Host-Fotoverzeichnis read-only nach `/srv/photos` gemountet werden.

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
| `PLAYWRIGHT_TEST_VERSION` | Nur `Makefile`: Version des per `npm exec` geladenen `@playwright/test`, Standard `1.60.0`. |
| `PLAYWRIGHT_BROWSER_CHANNEL` | Nur Playwright-Konfiguration: Browser-Channel fuer `make test-playwright`, Standard `chrome`. |
| `BEARSTACK_WEBDAV_TRACE` | Diagnose fuer WebDAV-Clients: bei `1`, `true`, `yes` oder `on` protokolliert BearStack WebDAV-Methode, Status und Pfadmetadaten. |

## Dokumentenfunktion

Dokumente werden ueber die Weboberflaeche (`POST /upload`), die JSON-API (`POST /api/upload`) oder WebDAV-`PUT` importiert. BearStack speichert die Originaldatei im konfigurierten `storage_dir`, legt Metadaten in SQLite ab und fuehrt Text-/Vorschau-/Thumbnail-Verarbeitung asynchron im Hintergrund aus. Unterstuetzt werden PDF, Bilder sowie einfache Text- und Office-Formate; Office-Text und Office-Vorschauen benoetigen LibreOffice.

Uploads werden nach dem konfigurierten Limit begrenzt, Dateinamen werden normalisiert, unbekannte Dateitypen werden abgelehnt und gespeicherte Pfade werden immer gegen den Storage-Root aufgeloest. Unerwartete Import- und Vorschaufehler werden fuer HTTP-Antworten generisch ausgegeben, damit interne Pfade oder Werkzeugdetails nicht im Browser landen.

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

Nach dem Neustart erscheint `Fotos` in der Hauptnavigation. Unterstuetzt werden Ordnernavigation, Bild- und Videowiedergabe (`jpg`, `jpeg`, `png`, `gif`, `webp`, `svg`, `mp4`, `webm`, `ogv`, `ogg`), On-demand-Thumbnails fuer JPEG/PNG/GIF, eine separate SQLite-Foto-DB fuer den Metadatenindex, EXIF-Datum/Kamera/GPS fuer JPEGs, Adobe/MWG-XMP-Gesichtsregionen, BearStack-Tags auf Ordnern und Medien, Umlaut-tolerante Suche mit Feldfiltern wie `date:2024`, `directory:Urlaub`, `file_name:IMG`, `type:image`, `gps:true`, `tag:urlaub`, `person:"Marie Curie"` und `face:Marie`, eine einfache Kartenansicht mit konfigurierbarer Foto-Track-Aufloesung, Markdown-Blogdateien, GPX-Hinweise, Zufallslink und Fotoframe.

Der Zufallsendpunkt `/photos/random` liefert standardmaessig das Original direkt aus. Mit `size=original` bleibt es beim Original; mit `size=ordner`, `size=galerie`, `size=gross`/`size=gro%C3%9F` oder `size=hd` wird stattdessen die jeweilige konfigurierte Thumbnailgroesse ausgeliefert.
Zusaetzlich liefert der Endpunkt Metadaten als Response-Header: `X-BearStack-Photo-Title` (Titel), `X-BearStack-Photo-Path` (Medienpfad), `X-BearStack-Photo-Folder-Path` (Ordnerpfad), `X-BearStack-Photo-Folder-URL` (absolute Ordner-URL inkl. Domain), `X-BearStack-Photo-Folder-Title` (Ordnername) sowie `Link: <https://.../photos?...>; rel="up"` als standardisierter Parent-Link.

Gesichtsdaten werden aus eingebettetem JPEG-XMP sowie XMP-Sidecars gelesen (`photo.jpg.xmp`, `photo.jpg.XMP`, `photo.xmp`, `photo.XMP`). BearStack speichert Namen und normalisierte Gesichtsboxen im Fotoindex und liefert sie in der Foto-JSON-API aus. Sie bleiben eigene Metadaten: Gesichtsnamen werden nicht automatisch zu Foto-Tags, erscheinen nicht in Tag-Listen und sind gezielt ueber `person:` oder `face:` suchbar.

Unter `Einstellungen -> Fotos` kann die Foto-Track-Aufloesung der Karte in sinnvollen Stufen von 500 m bis 10 km eingestellt werden. Sie legt fest, wie nah GPS-Fotos liegen muessen, um im fotobasierten Karten-Track zu einem Trackpunkt zusammengefasst zu werden. Dort kann auch ein Index-Worker aktiviert werden. Er crawlt den Foto-Root ordnerweise im Hintergrund, ueberspringt unveraenderte Ordner anhand ihres letzten Scan-Zeitpunkts und entfernt nicht mehr vorhandene Foto-, Ordner- und Blogeintraege ordnerlokal aus dem Index. Standardmaessig ist er deaktiviert; bei Aktivierung laeuft er alle 60 Minuten mit niedriger I/O-Prioritaet, falls vom System unterstuetzt, und 250 ms Pause pro gescanntem Ordner. Der separate Thumbnail-Worker ist ebenfalls standardmaessig deaktiviert; bei Aktivierung laeuft er alle 15 Minuten, erzeugt standardmaessig bis zu 15 fehlende Thumbnails pro Lauf und nutzt standardmaessig eine Thumbnail-Parallelitaet von 1.

Ordner koennen nach Ordnerstandard, Name, Datum oder zufaellig sortiert werden. Die Datumssortierung von Ordnern nutzt das aus dem Ordnernamen erkannte Anzeigedatum. Die Ordnerstandard-Sortierung wird ueber eine leere Steuerdatei im Ordner gesetzt: `.order_descending_name.pg2conf`, `.order_ascending_name.pg2conf`, `.order_descending_date.pg2conf`, `.order_ascending_date.pg2conf` oder `.order_random.pg2conf`.

Ein Ordner mit der Datei `.adminonly` ist nur fuer Benutzer mit der Rolle `admin` zugaenglich. Admin-only-Inhalte sind auch fuer Admins standardmaessig in Galerie, Suche, Zufall, Fotoframe, Kartenansicht und Foto-Tag-Listen ausgeblendet. Admins koennen sie im Sortieren-Menue der Galerie per Schalter einblenden; die Auswahl bleibt in der aktuellen Session gespeichert. Direkte Medien- und Thumbnail-URLs bleiben weiterhin nur Admins vorbehalten.

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

Wenn `password_hash` gesetzt ist, hat er Vorrang vor `password`. Nach erfolgreichem Basic-Auth-Login setzt BearStack ein signiertes HttpOnly-Session-Cookie fuer 12 Stunden. Mit der Login-Checkbox "Eingeloggt bleiben" wird die Session auf 30 Tage verlaengert. Nach einem Neustart werden bestehende Sessions ungueltig.

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

Rollen: `admin`, `documents_read`, `documents_editor`, `documents_manager`, `photos_read`, `photos_editor`, `photos_manager`, `api_uploader`. Statt oder zusaetzlich zu `role` koennen `permissions` gesetzt werden, z. B. `documents.read`, `documents.webdav.read`, `documents.upload`, `documents.edit`, `documents.delete`, `documents.structure`, `photos.read`, `photos.edit`, `photos.manage`, `system.manage`, `system.audit`.

BearStack startet ohne Auth nur auf Loopback-Adressen. Bei `BEARSTACK_ADDR=0.0.0.0:8080`, `:8080` oder einem anderen nicht-lokalen Host muss Auth vollstaendig gesetzt sein. Auch bei einem Reverse Proxy vor `127.0.0.1:8080` sollte Auth aktiv bleiben, wenn der Proxy keine eigene Zugriffskontrolle uebernimmt.

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

`update.sh` fuehrt im aktuellen Stand `git fetch --all --tags`, `git pull --ff-only`, `go test ./...` und einen Build in ein temporaeres Artefakt aus. Danach stoppt es den systemd-Dienst, installiert das neue Binary, startet den Dienst wieder und zeigt `systemctl status`. Ein Binary-Backup, automatischer `/healthz`-Check und Rollback sind im Skript nicht aktiv; den Smoke-Test deshalb nach dem Lauf manuell ausfuehren.

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
- Start bricht mit `auth username and password or password_hash are required when addr listens on non-loopback interfaces` ab: Auth setzen oder `BEARSTACK_ADDR` auf `127.0.0.1:8080`/`localhost:8080` beschraenken.
- Dienst startet ohne Auth-Warnung im Log: `BEARSTACK_AUTH_USER` plus Passwort oder Hash fehlt; das ist nur fuer rein lokale Tests geeignet.
- `open config file` oder `decode config file`: `BEARSTACK_CONFIG`-Pfad und JSON-Syntax pruefen.
- `permission denied` bei Datenbank oder Dokumenten: Besitzer/Rechte und `ReadWritePaths` der Unit pruefen.
- `address already in use`: Port mit `sudo ss -ltnp | grep ':8080'` pruefen.
- Upload scheitert mit `413`: `max_upload_bytes` und `client_max_body_size` im Reverse Proxy angleichen.
- Docker-Container startet wegen fehlender Auth nicht: `BEARSTACK_AUTH_PASSWORD` oder `BEARSTACK_AUTH_PASSWORD_HASH` setzen.
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
