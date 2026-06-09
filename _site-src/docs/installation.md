---
title: Installation
description: BearStack lokal starten, testen, bauen und für kleine Server betreiben.
icon: lucide/play
---

# Installation

## Voraussetzungen

Für Entwicklung und Build wird Go `1.25.x` oder eine kompatible neuere Version benötigt. Für JavaScript-Syntaxchecks kommt zusätzlich `node` dazu. Die optionalen Playwright-Smoke-Tests brauchen `npm` und einen Playwright-kompatiblen Browser; lokal ist standardmäßig der Chrome-Channel konfiguriert.

Vorschau, OCR und Medienfunktionen nutzen externe Werkzeuge nur dort, wo sie gebraucht werden:

- `poppler-utils` für PDF-Vorschau und OCR-Vorbereitung
- LibreOffice, konkret `soffice`, für Office-Vorschau und Volltextextraktion
- `tesseract` mit Sprachpaketen für OCR
- `ffmpeg` für Video- und Fallback-Thumbnails
- optional `vipsthumbnail` für schnelle Bild-Thumbnails

## Schnellstart

```sh
BEARSTACK_AUTH_USER=admin \
BEARSTACK_AUTH_PASSWORD=change-me \
go run ./cmd/bearstack
```

Standardwerte:

| Einstellung | Wert |
| --- | --- |
| Adresse | `127.0.0.1:8080` |
| Datenverzeichnis | `data` |
| Datenbank | `data/bearstack.db` |
| Dokumente | `data/documents` |
| Fotos | deaktiviert, Root bei Aktivierung `data/photos` |
| Upload-Limit | `50 MiB` |

Ohne vollständige Auth-Konfiguration ist Basic Auth auf Loopback-Adressen wie `127.0.0.1:8080` für lokale Entwicklung deaktiviert. Listener auf nicht-lokalen Interfaces wie `0.0.0.0:8080` oder `:8080` starten nur mit Benutzer plus Passwort oder Passwort-Hash.

## Lokale Entwicklung

Der lokale Start nutzt standardmäßig das Repo-nahe Verzeichnis `data/`:

```sh
go run ./cmd/bearstack
```

Mit getrenntem Entwicklungsdatenverzeichnis:

```sh
BEARSTACK_DATA_DIR=/tmp/bearstack-dev go run ./cmd/bearstack
```

Eine `.env`-Datei im Arbeitsverzeichnis wird automatisch gelesen. Zusätzlich kann `BEARSTACK_ENV_FILE` auf eine weitere Env-Datei zeigen. Bereits gesetzte Prozess-Umgebungsvariablen haben Vorrang vor Werten aus Env-Dateien; Werte aus `.env` haben für denselben Key Vorrang vor `BEARSTACK_ENV_FILE`.

## Tests

Go-Tests und Browser-Syntaxchecks laufen über Make:

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

`make test-go` führt `go test ./...` aus. `make test-js` nutzt `scripts/check-js.sh` und ruft `node --check` für alle Browser-Skripte unter `internal/server/static/*.js` auf. `make test-playwright` lädt den Test-Runner bei Bedarf über `npm exec`, erzeugt temporäre Daten, startet BearStack lokal mit eigener Testkonfiguration und prüft Dokumenten-Upload sowie Foto-Galerie.

Die Make-Variablen `GO`, `NODE`, `NPM` und `PLAYWRIGHT_TEST_VERSION` können überschrieben werden, zum Beispiel:

```sh
NODE=/opt/node/bin/node make test-js
```

Falls kein lokaler Chrome-Channel verfügbar ist, kann Playwright wie üblich einen Browser installieren:

```sh
npx playwright install chromium
PLAYWRIGHT_BROWSER_CHANNEL=chromium make test-playwright
```

## Build

Lokal über Make:

```sh
make build
```

Direkt:

```sh
go build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
```

Cross-Build für Raspberry Pi OS 64-bit:

```sh
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
```

## Docker und Compose

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

Der Container lauscht intern auf `0.0.0.0:8080` und speichert Daten unter `/var/lib/bearstack`. Deshalb muss Auth im Container gesetzt sein. `compose.yaml` nutzt standardmäßig `admin` als Benutzer, reicht `BEARSTACK_AUTH_PASSWORD` und `BEARSTACK_AUTH_PASSWORD_HASH` durch und veröffentlicht den Host-Port über `BEARSTACK_PORT` oder sonst `8080`.

Das Runtime-Image basiert auf `debian:trixie-slim`, die Build-Stage auf `golang:1.26-trixie`. Das Beispiel-Image enthält `ffmpeg`, `libreoffice-writer`, `poppler-utils`, `tesseract-ocr`, `tesseract-ocr-deu` und `tesseract-ocr-eng`, damit Foto-/Video-Vorschaubilder, PDF-/Office-Vorschauen, Text-/Office-Volltextextraktion und OCR im Container funktionieren. Bei aktiviertem Fotomodul muss ein Host-Fotoverzeichnis read-only nach `/srv/photos` gemountet werden.

Alle Docker-relevanten Umgebungsvariablen:

| Variable | Bedeutung |
| --- | --- |
| `BEARSTACK_PORT` | Nur für `compose.yaml`: Host-Port, der auf den Container-Port `8080` gemappt wird. Standard `8080`. |
| `BEARSTACK_CONFIG` | Pfad zu einer JSON-Konfigurationsdatei im Container. Die Datei muss per Volume ins Image gemountet werden, wenn sie nicht im Container liegt. |
| `BEARSTACK_ENV_FILE` | Optionale zusätzliche Env-Datei im Container. BearStack liest zuerst `.env` im Arbeitsverzeichnis und danach diese Datei, ohne bereits gesetzte Prozessvariablen zu überschreiben. |
| `BEARSTACK_ADDR` | Listener im Container. Im Dockerfile und in `compose.yaml` auf `0.0.0.0:8080` gesetzt, damit Port-Mapping funktioniert. Nicht-lokale Listener erfordern Auth. |
| `BEARSTACK_DATA_DIR` | Basisdatenverzeichnis. Im Container standardmäßig `/var/lib/bearstack`; dieses Verzeichnis sollte als Volume persistiert werden. |
| `BEARSTACK_STORAGE_DIR` | Dokumentenspeicher. Standard `${BEARSTACK_DATA_DIR}/documents`. |
| `BEARSTACK_DB_PATH` | SQLite-Datenbank für Dokumente, Einstellungen und Metadaten. Standard `${BEARSTACK_DATA_DIR}/bearstack.db`. |
| `BEARSTACK_MAX_UPLOAD_MB` | Upload-Limit in MiB. Standard `50`. |
| `BEARSTACK_MAX_UPLOAD_BYTES` | Upload-Limit in Bytes; hat Vorrang vor `BEARSTACK_MAX_UPLOAD_MB`. Muss größer als `0` sein. |
| `BEARSTACK_AUTH_USER` | Basic-Auth-Benutzer. `compose.yaml` nutzt standardmäßig `admin`. |
| `BEARSTACK_AUTH_PASSWORD` | Basic-Auth-Klartextpasswort. Für Tests und einfache Setups brauchbar; produktiv ist ein Hash besser. |
| `BEARSTACK_AUTH_PASSWORD_HASH` | bcrypt-Hash für Basic Auth; hat Vorrang vor `BEARSTACK_AUTH_PASSWORD`. Wegen `$`-Zeichen in Shell und Compose sorgfältig quoten. |
| `BEARSTACK_AUTH_REALM` | Basic-Auth-Realm. Standard `BearStack`. |
| `BEARSTACK_TLS_ENABLED` | Direktes HTTPS in BearStack aktivieren. Wahr sind `1`, `true`, `yes` und `on`. Bei Docker ist meist TLS im Reverse Proxy einfacher. |
| `BEARSTACK_TLS_CERT_FILE` | Pfad zur TLS-Zertifikatsdatei im Container. Muss gemeinsam mit `BEARSTACK_TLS_KEY_FILE` gesetzt werden. |
| `BEARSTACK_TLS_KEY_FILE` | Pfad zum TLS-Key im Container. Muss gemeinsam mit `BEARSTACK_TLS_CERT_FILE` gesetzt werden. |
| `BEARSTACK_TLS_AUTO_CERT` | Self-Signed-Zertifikat automatisch erzeugen, wenn TLS aktiv ist und kein Zertifikat gesetzt wurde. Standard `true`. |
| `BEARSTACK_PHOTOS_ENABLED` | Fotomodul aktivieren. Wahr sind `1`, `true`, `yes` und `on`. |
| `BEARSTACK_PHOTOS_DIR` | Vorhandenes Fotoverzeichnis im Container. Für Docker typischerweise ein read-only Mount nach `/srv/photos`; Standard `${BEARSTACK_DATA_DIR}/photos`. |
| `BEARSTACK_PHOTOS_DATA_DIR` | BearStack-eigene Fotodaten wie Index, Einstellungen und Metadaten. In `compose.yaml` auf `/var/lib/bearstack/photos` gesetzt; Standard sonst `${BEARSTACK_DATA_DIR}/photos-data`. |
| `BEARSTACK_PHOTOS_CACHE_DIR` | Thumbnail-Cache des Fotomoduls. Standard `${BEARSTACK_PHOTOS_DATA_DIR}/thumbnails`. |
| `BEARSTACK_PHOTOS_DB_PATH` | Separate SQLite-Datenbank für den Fotoindex. Standard `${BEARSTACK_PHOTOS_DATA_DIR}/photos.db`. |
| `BEARSTACK_PHOTOS_PAGE_SIZE` | Seitengröße der Fotoliste. Standard `120`; muss größer als `0` sein. |
| `BEARSTACK_WEBDAV_PATH` | HTTP-Pfad für WebDAV. Standard `/webdav`; muss mit `/` beginnen, darf nicht `/` sein und darf keine Leerzeichen, Querys, Fragmente oder Route-Wildcards enthalten. |
| `BEARSTACK_WEBDAV_TRACE` | Diagnose-Logging für WebDAV. Bei `1`, `true`, `yes` oder `on` protokolliert BearStack Methode, Status und Pfadmetadaten. |

Konfigurationspriorität im Container: Defaults, dann `BEARSTACK_CONFIG`, danach Env-Dateien und zuletzt bereits gesetzte Prozess-Umgebungsvariablen aus `docker run -e` oder `compose.yaml`. Eine Compose-`.env` im Projektverzeichnis dient zunächst nur der Variablenersetzung durch Docker Compose; BearStack selbst sieht sie erst, wenn sie in den Container gelangt oder Werte daraus als `environment` gesetzt werden.

Eine detaillierte Anleitung für Synology DSM mit Container Manager steht in `deploy-synology.md`.

Multi-Arch-Image für eine Registry bauen und veröffentlichen:

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/<owner>/bearstack:<tag> \
  --push .
```

## Auth

Für den Betrieb wird Basic Auth mit bcrypt-Hash empfohlen:

```sh
sudo apt install -y apache2-utils
htpasswd -bnBC 10 bearstack 'mein-passwort' | cut -d: -f2
```

Den Hash in JSON direkt eintragen oder in Shell-/Env-Dateien wegen der `$`-Zeichen quoten:

```env
BEARSTACK_AUTH_PASSWORD_HASH='$2a$10$...'
```

Wenn `password_hash` gesetzt ist, hat er Vorrang vor `password`. Nach erfolgreichem Basic-Auth-Login setzt BearStack ein signiertes HttpOnly-Session-Cookie für 12 Stunden. Mit der Login-Checkbox „Eingeloggt bleiben“ wird die Session auf 30 Tage verlängert. Nach einem Neustart werden bestehende Sessions ungültig.

Für einen einzelnen Benutzer reichen `BEARSTACK_AUTH_USER` und `BEARSTACK_AUTH_PASSWORD_HASH`; dieser Benutzer bekommt die Rolle `admin`. Mehrere Benutzer werden in der JSON-Konfiguration über `auth.credentials` definiert. Sobald diese Liste gesetzt ist, werden `auth.username`, `auth.password` und `auth.password_hash` ignoriert:

```json
"auth": {
  "realm": "BearStack",
  "credentials": [
    {"username": "admin", "password_hash": "$2a$10$...", "role": "admin"},
    {"username": "docs", "password_hash": "$2a$10$...", "role": "documents_read"},
    {"username": "photos", "password_hash": "$2a$10$...", "role": "photos_read"}
  ]
}
```

Rollen und Einzelrechte sind capability-basiert. Verfügbare Rollen sind `admin`, `documents_read`, `documents_editor`, `documents_manager`, `photos_read`, `photos_editor`, `photos_manager` und `api_uploader`. Statt oder zusätzlich zu `role` können `permissions` gesetzt werden, zum Beispiel `documents.read`, `documents.webdav.read`, `documents.upload`, `documents.edit`, `documents.delete`, `documents.structure`, `photos.read`, `photos.edit`, `photos.manage`, `system.manage` oder `system.audit`.

Die ausführliche Rollen- und Rechteübersicht steht unter [Benutzer und Rechte](benutzer-und-rechte.md).

BearStack startet ohne Auth nur auf Loopback-Adressen. Bei `BEARSTACK_ADDR=0.0.0.0:8080`, `:8080` oder einem anderen nicht-lokalen Host muss Auth vollständig gesetzt sein. Auch bei einem Reverse Proxy vor `127.0.0.1:8080` sollte Auth aktiv bleiben, wenn der Proxy keine eigene Zugriffskontrolle übernimmt.

## TLS

Für produktiven Betrieb ist meist am einfachsten: BearStack nur auf `127.0.0.1:8080` lauschen lassen und TLS in nginx oder Caddy terminieren.

Direktes HTTPS in BearStack ist möglich:

```sh
BEARSTACK_TLS_ENABLED=1 go run ./cmd/bearstack
```

Mit `auto_cert=true` erzeugt BearStack ein lokales Self-Signed-Zertifikat unter `data/tls/`. Browser melden dieses Zertifikat als nicht vertrauenswürdig. Für echte Zertifikate `tls.cert_file` und `tls.key_file` bzw. `BEARSTACK_TLS_CERT_FILE` und `BEARSTACK_TLS_KEY_FILE` setzen.

Wenn TLS aktiv ist, akzeptiert BearStack auf demselben `addr` sowohl HTTPS als auch plain HTTP. HTTP-Anfragen auf diesem Port werden permanent auf `https://` weitergeleitet.

## systemd

Eine Beispiel-Unit liegt in `deploy/bearstack.service` und nutzt eine JSON-Konfiguration:

```sh
sudo install -o root -g root -m 0755 bearstack /usr/local/bin/bearstack
sudo install -o root -g root -m 0644 deploy/bearstack.service /etc/systemd/system/bearstack.service
sudo systemctl daemon-reload
sudo systemctl enable --now bearstack
systemctl status bearstack
journalctl -u bearstack -n 100 --no-pager
```

Die Pfade in `ReadWritePaths` und `RequiresMountsFor` müssen zur eigenen Konfiguration passen. Eine vollständige Raspberry-Pi-Installation ist in `deploy-raspberrypi.md` beschrieben.

## Backup und Restore

Sicherste Backup-Variante: Dienst kurz stoppen und Datenverzeichnis plus Konfiguration sichern.

```sh
sudo mkdir -p /var/backups/bearstack
sudo systemctl stop bearstack
sudo tar -czf "/var/backups/bearstack/bearstack-$(date +%Y%m%d-%H%M%S).tgz" \
  /var/lib/bearstack \
  /etc/bearstack/bearstack.json
sudo systemctl start bearstack
```

Wenn `storage_dir` außerhalb von `data_dir` liegt, diesen Pfad ebenfalls sichern. Bei SQLite im WAL-Modus gehören `bearstack.db`, `bearstack.db-wal` und `bearstack.db-shm` zusammen; das Sichern des ganzen Datenverzeichnisses vermeidet unvollständige Backups.

Restore:

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

Vor Updates ein Datenbackup erstellen. Für systemd-Installationen liegt ein Update-Skript im Repo:

```sh
cd /opt/bearstack-src/BearStack
./update.sh
# Alternativ den Alpha-Branch installieren:
./update.sh --alpha
```

`update.sh` führt im aktuellen Stand `git fetch --all --tags` aus, wechselt auf den Update-Branch (`main` als Standard, `Alpha` mit `--alpha` oder frei per `--branch NAME`), führt `git pull --ff-only origin BRANCH`, `go test ./...` und einen Build in ein temporäres Artefakt aus. Danach stoppt es den systemd-Dienst, installiert das neue Binary, startet den Dienst wieder und zeigt `systemctl status`. Ein Binary-Backup, automatischer `/healthz`-Check und Rollback sind im Skript nicht aktiv; den Smoke-Test deshalb nach dem Lauf manuell ausführen.

Variablen für das Skript:

| Variable | Standard |
| --- | --- |
| `BEARSTACK_REPO_DIR` | Verzeichnis des Skripts |
| `BEARSTACK_UPDATE_BRANCH` | `main` |
| `BEARSTACK_GIT_REMOTE` | `origin` |
| `BEARSTACK_SERVICE` | `bearstack.service` |
| `BEARSTACK_INSTALL_PATH` | `/usr/local/bin/bearstack` |

Ohne Auth reicht für den manuellen Smoke-Test:

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
- Start bricht mit `auth username and password or password_hash are required when addr listens on non-loopback interfaces` ab: Auth setzen oder `BEARSTACK_ADDR` auf `127.0.0.1:8080`/`localhost:8080` beschränken.
- Dienst startet ohne Auth-Warnung im Log: `BEARSTACK_AUTH_USER` plus Passwort oder Hash fehlt; das ist nur für rein lokale Tests geeignet.
- `open config file` oder `decode config file`: `BEARSTACK_CONFIG`-Pfad und JSON-Syntax prüfen.
- `permission denied` bei Datenbank oder Dokumenten: Besitzer/Rechte und `ReadWritePaths` der Unit prüfen.
- `address already in use`: Port mit `sudo ss -ltnp | grep ':8080'` prüfen.
- Upload scheitert mit `413`: `max_upload_bytes` und `client_max_body_size` im Reverse Proxy angleichen.
- Docker-Container startet wegen fehlender Auth nicht: `BEARSTACK_AUTH_PASSWORD` oder `BEARSTACK_AUTH_PASSWORD_HASH` setzen.
- `make test-js` bricht mit `node is required...` ab: Node installieren oder `NODE=/pfad/zu/node make test-js` verwenden.
- `make test-playwright` scheitert beim Abruf des Test-Runners: `npm`-Zugang, Netzwerk oder Proxy prüfen und mit lokalem Cache erneut starten.
- `make test-playwright` findet keinen Browser: Chrome installieren oder `npx playwright install chromium` ausführen und mit `PLAYWRIGHT_BROWSER_CHANNEL=chromium make test-playwright` starten.
- `go test` oder `go build` meldet Schreibfehler im Go-Build-Cache: `GOCACHE` auf ein beschreibbares Verzeichnis setzen, zum Beispiel `GOCACHE=/tmp/go-build-cache go test ./...`.
- `update.sh` startet den Dienst, aber Status oder manueller Smoke-Test schlagen fehl: `systemctl status bearstack` und `journalctl -u bearstack -n 100 --no-pager` prüfen; bei aktiver Auth den Smoke-Test mit `curl -u` ausführen.
- Vorschaubilder für PDFs fehlen: `poppler-utils` installieren, besonders `pdftoppm`.
- Text-/Office-Vorschau oder Volltextextraktion für TXT, Markdown, RTF, DOC, DOCX oder Pages fehlt: LibreOffice installieren und `command -v soffice` prüfen.
- Foto-/Video-Vorschaubilder fehlen: `ffmpeg` installieren; für Bild-Thumbnails kann optional `vipsthumbnail` genutzt werden.
- OCR scheitert: `tesseract-ocr`, Sprachpakete und `pdfinfo`/`pdftoppm` prüfen.
- Fotomodul fehlt in der Navigation: `BEARSTACK_PHOTOS_ENABLED=true` setzen und Dienst neu starten.
