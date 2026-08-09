# Deployment auf einem Raspberry Pi

Diese Anleitung beschreibt einen realistischen Betrieb als systemd-Dienst auf Raspberry Pi OS oder Debian/Ubuntu ARM64. Sie nutzt eine JSON-Konfiguration unter `/etc/bearstack` und lokale Daten unter `/var/lib/bearstack`.

## Zielzustand

- Binary: `/usr/local/bin/bearstack`
- Dienstbenutzer: `bearstack`
- Konfiguration: `/etc/bearstack/bearstack.json`
- Datenverzeichnis: `/var/lib/bearstack`
- Datenbank: `/var/lib/bearstack/bearstack.db`
- Dokumente: `/var/lib/bearstack/documents`
- Interner HTTP-Port: `127.0.0.1:8080`
- Externer Zugriff optional ueber nginx oder Caddy

Wenn Dokumente auf einer USB-SSD oder einem NAS liegen sollen, setzen Sie `storage_dir` auf diesen Mount und passen Sie `ReadWritePaths` und `RequiresMountsFor` in der systemd-Unit an.

## 1. System vorbereiten

```sh
sudo apt update
sudo apt full-upgrade -y
sudo reboot
```

Nach dem Neustart:

```sh
sudo apt install -y git ca-certificates curl apache2-utils chromium ffmpeg libreoffice-writer poppler-utils tesseract-ocr tesseract-ocr-deu tesseract-ocr-eng
```

Pakete:

- `apache2-utils`: `htpasswd` zum Erzeugen eines bcrypt-Passwort-Hashes.
- `chromium`: gerenderte Mailabbildungen fuer EML-Archive im E-Mail-Import.
- `ffmpeg`: Vorschaubilder fuer Videos im Fotomodul.
- `libreoffice-writer`: `soffice` fuer Text-/Office-Vorschau und Volltextextraktion von TXT, Markdown, RTF, DOC, DOCX und Pages.
- `poppler-utils`: `pdftoppm` und `pdfinfo` fuer Vorschaubilder und OCR-Vorbereitung.
- `tesseract-ocr`: OCR.
- `tesseract-ocr-deu`, `tesseract-ocr-eng`: OCR-Sprachen Deutsch und Englisch.
- `ca-certificates`: TLS-Pruefung fuer IMAP und HTTPS.

Optional kann `libvips-tools` installiert werden. BearStack nutzt `vipsthumbnail`, wenn es verfuegbar ist; sonst werden Bild-Thumbnails ueber `ffmpeg` erzeugt.

## 2. Go installieren

BearStack verwendet Go `1.25` laut `go.mod`. Fuer Neuinstallationen sollte eine aktuelle Patch-Version aus der `1.25`-Reihe oder eine kompatible neuere Go-Version verwendet werden.

```sh
go version
```

Wenn keine passende Go-Version installiert ist:

```sh
cd /tmp
version=go1.25.10
curl -LO "https://go.dev/dl/${version}.linux-arm64.tar.gz"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "${version}.linux-arm64.tar.gz"
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.profile
. ~/.profile
go version
```

Eine neuere kompatible Go-Version ist in der Regel ebenfalls nutzbar; die `version`-Zeile kann entsprechend angepasst werden.

## 3. Benutzer und Verzeichnisse

```sh
sudo useradd --system --home /var/lib/bearstack --shell /usr/sbin/nologin bearstack
sudo mkdir -p /var/lib/bearstack /var/lib/bearstack/documents /etc/bearstack
sudo chown -R bearstack:bearstack /var/lib/bearstack
sudo chown root:bearstack /etc/bearstack
sudo chmod 750 /var/lib/bearstack /var/lib/bearstack/documents /etc/bearstack
```

Bei externem Dokumentenspeicher:

```sh
sudo mkdir -p /srv/bearstack/documents
sudo chown bearstack:bearstack /srv/bearstack/documents
sudo chmod 750 /srv/bearstack/documents
```

Wenn ein externer Datentraeger genutzt wird, dauerhaft per `/etc/fstab` einbinden. BearStack darf erst starten, wenn der Mount verfuegbar ist.

## 4. Quellcode holen und bauen

```sh
sudo mkdir -p /opt/bearstack-src
sudo chown "$USER":"$USER" /opt/bearstack-src
git clone <GIT-URL-DIESES-REPOSITORIES> /opt/bearstack-src/BearStack
cd /opt/bearstack-src/BearStack
go test ./...
go build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
sudo install -o root -g root -m 0755 bearstack /usr/local/bin/bearstack
```

Bei bestehendem Checkout:

```sh
cd /opt/bearstack-src/BearStack
git pull --ff-only
```

## 5. Auth konfigurieren

Passwort-Hash erzeugen:

```sh
htpasswd -bnBC 10 bearstack 'mein-passwort' | cut -d: -f2
```

Konfiguration anlegen:

```sh
sudo nano /etc/bearstack/bearstack.json
```

Inhalt:

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
  "webdav": {
    "path": "/webdav"
  },
  "auth": {
    "username": "admin",
    "password": "",
    "password_hash": "$2a$10$BITTE-BCRYPT-HASH-SETZEN",
    "realm": "BearStack"
  }
}
```

Rechte:

```sh
sudo chown root:bearstack /etc/bearstack/bearstack.json
sudo chmod 640 /etc/bearstack/bearstack.json
```

Hinweise:

- `auth.password_hash` ist empfohlen.
- `auth.password` ist nur fuer einfache lokale Tests gedacht.
- Wenn `password_hash` gesetzt ist, wird `password` ignoriert.
- Optional ersetzt `auth.credentials` den einzelnen Benutzer durch mehrere Credentials mit Rollen wie `admin`, `documents_read`, `photos_read` oder `api_uploader`.
- Ohne Config-Zugangsdaten und ohne aktives SQLite-Konto ist Auth deaktiviert. Das ist im Netzwerk ein Sicherheitsproblem.
- BearStack verweigert nicht-lokale Listener wie `0.0.0.0:8080` oder `:8080`, wenn weder ein aktives Config- noch ein aktives SQLite-Konto vorhanden ist.
- Bei Reverse-Proxy-Betrieb auf `127.0.0.1:8080` sollte Auth trotzdem gesetzt bleiben, wenn der Proxy keine eigene Zugriffskontrolle uebernimmt.
- Nach erfolgreichem Login setzt BearStack ein signiertes HttpOnly-Session-Cookie fuer 12 Stunden. Mit der Login-Checkbox "Eingeloggt bleiben" wird die Session auf 30 Tage verlaengert. Der Signierschluessel liegt im Datenverzeichnis, sodass unveraenderte Konten einen Neustart ueberstehen; beim Upgrade auf 0.22.0 ist wegen des neuen Sessionformats einmalig eine erneute Anmeldung erforderlich.
- Unbekannte Felder in der JSON-Datei werden ignoriert, damit Installationen mit zusaetzlichen oder kuenftigen Parametern weiter starten.

Beispiel fuer mehrere Credentials:

```json
"auth": {
  "realm": "BearStack",
  "credentials": [
    {"username": "admin", "password_hash": "$2a$10$BITTE-BCRYPT-HASH-SETZEN", "role": "admin"},
    {"username": "dav", "password_hash": "$2a$10$BITTE-BCRYPT-HASH-SETZEN", "role": "documents_read"},
    {"username": "photos", "password_hash": "$2a$10$BITTE-BCRYPT-HASH-SETZEN", "role": "photos_read"}
  ]
}
```

Umgebungsvariablen sind ebenfalls moeglich. Fuer systemd ist eine JSON-Datei meist uebersichtlicher. Relevante Variablen:

- `BEARSTACK_CONFIG`
- `BEARSTACK_ENV_FILE`
- `BEARSTACK_ADDR`
- `BEARSTACK_DATA_DIR`
- `BEARSTACK_STORAGE_DIR`
- `BEARSTACK_DB_PATH`
- `BEARSTACK_MAX_UPLOAD_MB`
- `BEARSTACK_MAX_UPLOAD_BYTES`
- `BEARSTACK_TLS_ENABLED`
- `BEARSTACK_TLS_CERT_FILE`
- `BEARSTACK_TLS_KEY_FILE`
- `BEARSTACK_TLS_AUTO_CERT`
- `BEARSTACK_AUTH_USER`
- `BEARSTACK_AUTH_PASSWORD`
- `BEARSTACK_AUTH_PASSWORD_HASH`
- `BEARSTACK_AUTH_REALM`
- `BEARSTACK_WEBDAV_PATH`
- `BEARSTACK_PHOTOS_ENABLED`
- `BEARSTACK_PHOTOS_DIR`
- `BEARSTACK_PHOTOS_DATA_DIR`
- `BEARSTACK_PHOTOS_CACHE_DIR`
- `BEARSTACK_PHOTOS_DB_PATH`
- `BEARSTACK_PHOTOS_PAGE_SIZE`

Wenn `.env` oder `BEARSTACK_ENV_FILE` verwendet werden, liest BearStack diese Dateien vor der JSON-Konfiguration in die Prozessumgebung ein. Bereits gesetzte Prozessvariablen werden nicht ueberschrieben; Werte aus `.env` haben fuer denselben Key Vorrang vor `BEARSTACK_ENV_FILE`. Effektiv ueberschreiben Env-Werte die JSON-Datei.

## 6. systemd-Dienst einrichten

Beispiel-Unit installieren:

```sh
cd /opt/bearstack-src/BearStack
sudo install -o root -g root -m 0644 deploy/bearstack.service /etc/systemd/system/bearstack.service
```

Wenn `storage_dir` oder `db_path` ausserhalb von `/var/lib/bearstack` liegen, Unit anpassen:

```sh
sudo nano /etc/systemd/system/bearstack.service
```

Mindestens diese Zeilen muessen zu den echten Pfaden passen:

```ini
RequiresMountsFor=/var/lib/bearstack
ReadWritePaths=/var/lib/bearstack
```

Beispiel mit Dokumenten unter `/srv/bearstack/documents`:

```ini
RequiresMountsFor=/var/lib/bearstack /srv/bearstack/documents
ReadWritePaths=/var/lib/bearstack
ReadWritePaths=/srv/bearstack/documents
```

Dienst starten:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now bearstack
systemctl status bearstack
journalctl -u bearstack -n 100 --no-pager
```

Lokaler Test:

```sh
curl -I http://127.0.0.1:8080/
```

Bei aktivierter Auth ist `401 Unauthorized` fuer diesen Test normal. Mit Login:

```sh
curl -I -u admin:mein-passwort http://127.0.0.1:8080/
```

## 7. TLS und Reverse Proxy

Empfohlen: BearStack intern auf `127.0.0.1:8080` lassen und TLS im Reverse Proxy beenden.

nginx installieren:

```sh
sudo apt install -y nginx
```

Site:

```sh
sudo nano /etc/nginx/sites-available/bearstack
```

HTTP-Beispiel fuer ein lokales Netz:

```nginx
server {
    listen 80;
    server_name bearstack.local;

    client_max_body_size 50m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Aktivieren:

```sh
sudo ln -s /etc/nginx/sites-available/bearstack /etc/nginx/sites-enabled/bearstack
sudo nginx -t
sudo systemctl reload nginx
```

TLS mit Let's Encrypt:

```sh
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d bearstack.example.com
sudo certbot renew --dry-run
```

Direktes TLS in BearStack ist moeglich, z. B. fuer ein lokales Self-Signed-Zertifikat:

```json
"tls": {
  "enabled": true,
  "cert_file": "",
  "key_file": "",
  "auto_cert": true
}
```

BearStack erzeugt dann Zertifikat und Key unter `/var/lib/bearstack/tls/`. Browser vertrauen diesem Zertifikat nicht automatisch. Wenn TLS aktiv ist, akzeptiert BearStack auf demselben `addr` sowohl HTTPS als auch plain HTTP; HTTP-Anfragen auf diesem Port werden per Permanent-Redirect auf HTTPS umgeleitet. Fuer produktive externe Erreichbarkeit ist Reverse-Proxy-TLS meist einfacher.

## 8. Betrieb

Status:

```sh
systemctl status bearstack
```

Logs:

```sh
journalctl -u bearstack -f
journalctl -u bearstack -n 200 --no-pager
```

Neustart:

```sh
sudo systemctl restart bearstack
```

Konfiguration nach Aenderungen:

```sh
sudo systemctl restart bearstack
journalctl -u bearstack -n 100 --no-pager
```

BearStack schreibt JSON-Logs nach stdout; systemd sammelt sie im Journal.

## 9. Backup

Ein konsistentes Backup enthaelt:

- `/etc/bearstack/bearstack.json`
- SQLite-Datenbank inklusive WAL/SHM-Dateien
- Dokumentenverzeichnis
- bei direktem BearStack-TLS: `/var/lib/bearstack/tls/`

Wenn alle Daten unter `/var/lib/bearstack` liegen:

```sh
sudo mkdir -p /var/backups/bearstack
sudo chmod 750 /var/backups/bearstack
sudo systemctl stop bearstack
sudo tar -czf "/var/backups/bearstack/bearstack-$(date +%Y%m%d-%H%M%S).tgz" \
  /var/lib/bearstack \
  /etc/bearstack/bearstack.json
sudo systemctl start bearstack
```

Wenn Dokumente z. B. unter `/srv/bearstack/documents` liegen:

```sh
sudo systemctl stop bearstack
sudo tar -czf "/var/backups/bearstack/bearstack-$(date +%Y%m%d-%H%M%S).tgz" \
  /var/lib/bearstack \
  /srv/bearstack/documents \
  /etc/bearstack/bearstack.json
sudo systemctl start bearstack
```

Backup pruefen:

```sh
sudo tar -tzf /var/backups/bearstack/<backup-datei>.tgz | head
```

Backups regelmaessig auf ein anderes Geraet kopieren. SD-Karten und einzelne USB-Datentraeger sind kein Backup.

## 10. Automatisches Backup mit systemd timer

Skript:

```sh
sudo nano /usr/local/sbin/bearstack-backup
```

Inhalt fuer Standardpfade:

```sh
#!/bin/sh
set -eu

backup_dir=/var/backups/bearstack
stamp=$(date +%Y%m%d-%H%M%S)

mkdir -p "$backup_dir"
systemctl stop bearstack
trap 'systemctl start bearstack' EXIT

tar -czf "$backup_dir/bearstack-$stamp.tgz" \
  /var/lib/bearstack \
  /etc/bearstack/bearstack.json

systemctl start bearstack
trap - EXIT

find "$backup_dir" -type f -name 'bearstack-*.tgz' -mtime +30 -delete
```

Wenn `storage_dir` ausserhalb von `/var/lib/bearstack` liegt, diesen Pfad im `tar`-Befehl ergaenzen.

Aktivieren:

```sh
sudo chmod 750 /usr/local/sbin/bearstack-backup
sudo nano /etc/systemd/system/bearstack-backup.service
```

```ini
[Unit]
Description=BearStack Backup

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/bearstack-backup
```

Timer:

```sh
sudo nano /etc/systemd/system/bearstack-backup.timer
```

```ini
[Unit]
Description=Daily BearStack backup

[Timer]
OnCalendar=*-*-* 03:15:00
Persistent=true

[Install]
WantedBy=timers.target
```

Starten:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now bearstack-backup.timer
systemctl list-timers bearstack-backup.timer
```

## 11. Restore

Backup-Inhalt zuerst ansehen:

```sh
sudo tar -tzf /var/backups/bearstack/<backup-datei>.tgz | head -50
```

Restore fuer Standardpfade:

```sh
sudo systemctl stop bearstack
stamp=$(date +%Y%m%d-%H%M%S)
sudo mv /var/lib/bearstack "/var/lib/bearstack.before-restore-$stamp"
sudo tar -C / -xzf /var/backups/bearstack/<backup-datei>.tgz
sudo chown -R bearstack:bearstack /var/lib/bearstack
sudo chown root:bearstack /etc/bearstack /etc/bearstack/bearstack.json
sudo chmod 750 /var/lib/bearstack /etc/bearstack
sudo chmod 640 /etc/bearstack/bearstack.json
sudo systemctl start bearstack
journalctl -u bearstack -n 100 --no-pager
```

Bei getrenntem Dokumentenspeicher:

```sh
sudo systemctl stop bearstack
stamp=$(date +%Y%m%d-%H%M%S)
sudo mv /var/lib/bearstack "/var/lib/bearstack.before-restore-$stamp"
sudo mv /srv/bearstack/documents "/srv/bearstack/documents.before-restore-$stamp"
sudo tar -C / -xzf /var/backups/bearstack/<backup-datei>.tgz
sudo chown -R bearstack:bearstack /var/lib/bearstack /srv/bearstack/documents
sudo systemctl start bearstack
```

Danach im Browser pruefen, ob Dokumentliste, Downloads und Vorschaubilder funktionieren.

## 12. Update

Vor jedem Update ein Backup erstellen.

Das Update-Skript im Repo aktualisiert den Checkout, fuehrt `go test ./...` aus, baut ein temporaeres Artefakt, installiert es nach `/usr/local/bin/bearstack` und startet den systemd-Dienst neu. Es erstellt im aktuellen Stand kein Binary-Backup und fuehrt keinen automatischen Healthcheck oder Rollback aus. Vorher deshalb ein Datenbackup erstellen und danach den Dienststatus plus Smoke-Test manuell pruefen:

```sh
cd /opt/bearstack-src/BearStack
./update.sh
# Alternativ den Alpha-Branch installieren:
./update.sh --alpha
curl -I -u admin:mein-passwort http://127.0.0.1:8080/
```

Wenn das Passwort nicht in der Shell-History landen soll, den Smoke-Test interaktiv oder mit einer lokal geschuetzten Env-/Netrc-Variante ausfuehren.

Anpassbare Variablen:

- `BEARSTACK_REPO_DIR`: Checkout-Verzeichnis, standardmaessig Verzeichnis des Skripts.
- `BEARSTACK_UPDATE_BRANCH`: Update-Branch, standardmaessig `main`.
- `BEARSTACK_GIT_REMOTE`: Git-Remote, standardmaessig `origin`.
- `BEARSTACK_SERVICE`: systemd-Dienst, standardmaessig `bearstack.service`.
- `BEARSTACK_INSTALL_PATH`: Installationspfad, standardmaessig `/usr/local/bin/bearstack`.

Manuelles Update:

```sh
cd /opt/bearstack-src/BearStack
git fetch --all --tags
git pull --ff-only
go test ./...
go build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
```

Binary manuell austauschen:

```sh
sudo cp /usr/local/bin/bearstack "/usr/local/bin/bearstack.$(date +%Y%m%d-%H%M%S).bak"
sudo systemctl stop bearstack
sudo install -o root -g root -m 0755 bearstack /usr/local/bin/bearstack
sudo systemctl start bearstack
systemctl status bearstack
journalctl -u bearstack -n 100 --no-pager
```

Smoke-Test:

```sh
curl -I -u admin:mein-passwort http://127.0.0.1:8080/
```

Rollback des Binaries:

```sh
sudo systemctl stop bearstack
sudo install -o root -g root -m 0755 /usr/local/bin/bearstack.<timestamp>.bak /usr/local/bin/bearstack
sudo systemctl start bearstack
```

Wenn eine neue Version bereits Datenbankmigrationen ausgefuehrt hat, reicht ein Binary-Rollback eventuell nicht. Dann Datenbackup wiederherstellen.

## 13. Mail-Import

Der Mail-Import wird in der Weboberflaeche konfiguriert. Er nutzt IMAP und importiert PDF-Anhaenge.

Hinweise:

- TLS oder STARTTLS verwenden, wenn das Postfach nicht rein lokal ist.
- `security=none` sendet IMAP-Zugangsdaten unverschluesselt.
- Systemzeit und `ca-certificates` muessen stimmen.
- Ein eigenes Import-Postfach verwenden.
- Erfolgreich verarbeitete Mails werden geloescht.
- Nicht erlaubte Absender werden abgelehnt und geloescht.
- Upload-Limit gilt auch fuer PDF-Anhaenge.

Logs:

```sh
journalctl -u bearstack -n 200 --no-pager | grep -i mail
```

## 14. Typische Fehlerfaelle

Dienst startet nicht:

```sh
journalctl -u bearstack -n 200 --no-pager
sudo -u bearstack BEARSTACK_CONFIG=/etc/bearstack/bearstack.json /usr/local/bin/bearstack
```

Port belegt:

```sh
sudo ss -ltnp | grep ':8080'
```

Auth deaktiviert:

- Im Start-Log steht `auth_enabled=false`.
- Config-Zugangsdaten oder die aktiven SQLite-Konten in der Nutzerverwaltung pruefen.
- Bei systemd pruefen, ob `BEARSTACK_CONFIG` auf die richtige Datei zeigt.
- Wenn der Dienst mit `at least one active authentication account is required when addr listens on non-loopback interfaces` abbricht, ist `addr` nicht auf Loopback beschraenkt und es fehlt ein aktives Config- oder SQLite-Konto.

`401 Unauthorized`:

- Normal, wenn ohne Basic Auth getestet wird.
- Mit `curl -u admin:passwort http://127.0.0.1:8080/` pruefen.

`502 Bad Gateway` im Reverse Proxy:

```sh
systemctl status bearstack
curl -I http://127.0.0.1:8080/
sudo nginx -t
journalctl -u nginx -n 100 --no-pager
```

Upload scheitert:

- `max_upload_bytes` in BearStack pruefen.
- `client_max_body_size` in nginx/Caddy pruefen.
- Freien Speicher pruefen: `df -h`.

Update-Skript startet den Dienst, aber Status oder Smoke-Test schlagen fehl:

- `systemctl status bearstack` pruefen.
- `journalctl -u bearstack -n 100 --no-pager` pruefen, ob der Dienst gestartet ist.
- Bei aktivierter Auth den Smoke-Test mit `curl -u` ausfuehren.

Rechteproblem bei Datenbank oder Dokumenten:

```sh
sudo namei -l /var/lib/bearstack
sudo find /var/lib/bearstack -maxdepth 2 -printf '%u:%g %m %p\n'
sudo chown -R bearstack:bearstack /var/lib/bearstack
sudo chmod 750 /var/lib/bearstack /var/lib/bearstack/documents
```

Bei separatem `storage_dir` den Pfad ebenfalls pruefen und in der systemd-Unit unter `ReadWritePaths` eintragen.

Externer Datentraeger nicht gemountet:

```sh
findmnt /srv/bearstack/documents
systemctl status bearstack
```

In der Unit `RequiresMountsFor=` setzen.

Vorschaubilder fehlen:

```sh
command -v pdftoppm
command -v soffice
command -v ffmpeg
command -v vipsthumbnail
journalctl -u bearstack -n 200 --no-pager | grep -i thumbnail
```

`vipsthumbnail` ist optional. Wenn es fehlt, kann BearStack Bild-Thumbnails ueber `ffmpeg` erzeugen.

Text-/Office-Vorschau oder Volltextextraktion funktioniert nicht:

```sh
command -v soffice
journalctl -u bearstack -n 200 --no-pager | grep -i libreoffice
```

OCR funktioniert nicht:

```sh
command -v tesseract
command -v pdfinfo
command -v pdftoppm
tesseract --list-langs
journalctl -u bearstack -n 200 --no-pager | grep -i ocr
```

TLS-Zertifikatfehler bei Mail-Import:

```sh
date
dpkg -l ca-certificates
journalctl -u bearstack -n 200 --no-pager | grep -i mail
```

Browser meldet Self-Signed-Zertifikat:

- Erwartet, wenn BearStack `auto_cert=true` nutzt.
- Fuer produktive Nutzung Reverse Proxy mit Let's Encrypt verwenden oder eigenes Zertifikat eintragen.

## 15. Checkliste

- System aktualisiert.
- Go, `poppler-utils`, `apache2-utils` und optional OCR-Pakete installiert.
- Benutzer `bearstack` angelegt.
- Daten- und Konfigurationsverzeichnis angelegt.
- Binary gebaut und nach `/usr/local/bin/bearstack` installiert.
- `/etc/bearstack/bearstack.json` mit bcrypt-Hash erstellt.
- systemd-Unit installiert und Pfade angepasst.
- Dienst gestartet und mit `curl` geprueft.
- Reverse Proxy und TLS eingerichtet, falls extern erreichbar.
- Backup eingerichtet.
- Restore mindestens einmal auf einem Testpfad geprueft.
