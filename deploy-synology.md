# Deployment auf Synology DSM

Diese Anleitung beschreibt den Betrieb von BearStack als Container auf einer Synology NAS mit DSM und Container Manager. Der empfohlene Weg ist ein Container-Manager-Projekt mit Compose-Datei, persistentem Datenordner unter `/volume1/docker/bearstack/data` und optionalem read-only Foto-Share.

## Zielzustand

- DSM-Paket: `Container Manager` auf DSM 7.2 oder neuer; auf aelteren DSM-Versionen heisst das Paket teilweise `Docker`.
- Projektname: `bearstack`
- HTTP-Port im LAN: `8088`
- Container-Port: `8080`
- App-Daten auf der NAS: `/volume1/docker/bearstack/data`
- App-Daten im Container: `/var/lib/bearstack`
- Optionaler Foto-Share auf der NAS: `/volume1/photo`
- Optionaler Foto-Share im Container: `/srv/photos`
- Container-Benutzer im Image: UID/GID `10001`

BearStack lauscht im Container auf `0.0.0.0:8080`. Deshalb muss Auth gesetzt sein. Ohne Benutzer plus Passwort oder Passwort-Hash startet BearStack bei dieser Adresse nicht.

## 1. Voraussetzungen pruefen

1. In DSM anmelden.
2. `Paket-Zentrum` oeffnen.
3. `Container Manager` installieren.

Wenn `Container Manager` nicht im Paket-Zentrum angeboten wird, unterstuetzt das NAS-Modell diese Docker-/Container-Funktion offiziell nicht. Dann ist ein anderes Deployment noetig, zum Beispiel auf einem kleinen Linux-Rechner oder in einer VM.

Architektur per SSH pruefen:

```sh
uname -m
```

Zuordnung:

- `x86_64`: Docker-Plattform `linux/amd64`
- `aarch64`: Docker-Plattform `linux/arm64`

Diese beiden Plattformen sind fuer BearStack sinnvoll. 32-bit-ARM-Modelle sind fuer diese Docker-Auslieferung nicht empfohlen.

## 2. Ordner auf der Synology anlegen

Per DSM File Station oder SSH diese Ordner anlegen:

```sh
mkdir -p /volume1/docker/bearstack/app
mkdir -p /volume1/docker/bearstack/data
```

Der Container laeuft als UID/GID `10001`. Der Datenordner muss fuer diese UID beschreibbar sein:

```sh
sudo chown -R 10001:10001 /volume1/docker/bearstack/data
sudo chmod -R u+rwX,g-rwx,o-rwx /volume1/docker/bearstack/data
```

Wenn SSH nicht moeglich ist, kann statt des Bind-Mounts ein Docker-Volume verwendet werden. Das ist einfacher zu starten, aber schlechter direkt mit Hyper Backup oder File Station zu sichern.

## 3. Quellcode auf die Synology bringen

Variante A: per Git auf der NAS:

```sh
cd /volume1/docker/bearstack/app
git clone <GIT-URL-DIESES-REPOSITORIES> .
```

Variante B: per File Station/SMB hochladen:

Kopiere den Projektinhalt nach `/volume1/docker/bearstack/app`. Notwendig sind mindestens:

- `Dockerfile`
- `compose.yaml`
- `.dockerignore`
- `go.mod`
- `go.sum`
- `cmd/`
- `internal/`

Der Ordner `.git` muss nicht mitkopiert werden.

## 4. Synology-Compose-Datei anlegen

Lege in `/volume1/docker/bearstack/app` eine Datei `docker-compose.yml` an. Diese Datei ist bewusst Synology-spezifisch und nutzt einen Bind-Mount fuer einfache Backups:

```yaml
services:
  bearstack:
    build:
      context: .
      dockerfile: Dockerfile
    image: bearstack:local
    container_name: bearstack
    restart: unless-stopped
    init: true
    ports:
      - "8088:8080"
    environment:
      BEARSTACK_ADDR: "0.0.0.0:8080"
      BEARSTACK_DATA_DIR: "/var/lib/bearstack"
      BEARSTACK_AUTH_USER: "admin"
      BEARSTACK_AUTH_PASSWORD: "BITTE_AENDERN"
      BEARSTACK_AUTH_PASSWORD_HASH: ""
      BEARSTACK_AUTH_REALM: "BearStack"
      BEARSTACK_WEBDAV_PATH: ""
      BEARSTACK_PHOTOS_ENABLED: "false"
      BEARSTACK_PHOTOS_DIR: "/srv/photos"
      BEARSTACK_PHOTOS_DATA_DIR: "/var/lib/bearstack/photos"
    volumes:
      - /volume1/docker/bearstack/data:/var/lib/bearstack
      # Optional, wenn das Fotomodul genutzt werden soll:
      # - /volume1/photo:/srv/photos:ro
```

Wichtig:

- `BEARSTACK_AUTH_PASSWORD` sofort durch ein eigenes starkes Passwort ersetzen.
- `8088:8080` bedeutet: Im Browser wird spaeter `http://NAS-IP:8088` aufgerufen.
- Wenn Port `8088` belegt ist, links einen anderen freien Host-Port setzen, zum Beispiel `8090:8080`.
- Fuer den ersten Start `BEARSTACK_PHOTOS_ENABLED` auf `false` lassen. Das reduziert die Fehlerquellen.

## 5. Optional: Passwort-Hash statt Klartextpasswort

Empfohlen ist ein bcrypt-Hash statt Klartextpasswort in der Compose-Datei. Einen Hash kannst du auf einem Rechner mit `htpasswd` erzeugen:

```sh
htpasswd -bnBC 10 admin 'mein-sehr-langes-passwort' | cut -d: -f2
```

Dann in `docker-compose.yml`:

```yaml
BEARSTACK_AUTH_PASSWORD: ""
BEARSTACK_AUTH_PASSWORD_HASH: "$$2a$$10$$DEIN_HASH_MIT_DOPPELTEN_DOLLARZEICHEN"
```

Warum doppelte Dollarzeichen? Docker Compose interpretiert `$` in YAML-Werten als Variablenersetzung. Ein bcrypt-Hash enthaelt Dollarzeichen. Direkt in YAML muessen sie deshalb als `$$` geschrieben werden. Wenn der Hash ueber eine echte Umgebungsvariable kommt, ist dieses Escaping nicht noetig.

## 6. Projekt in Container Manager erstellen

1. DSM oeffnen.
2. `Container Manager` oeffnen.
3. Links `Projekt` auswaehlen.
4. `Erstellen` anklicken.
5. Projektname: `bearstack`
6. Pfad: `/volume1/docker/bearstack/app`
7. Quelle: vorhandene Compose-Datei verwenden oder den Inhalt aus Schritt 4 einfuegen.
8. Web-Portal-Einstellungen zunaechst ueberspringen.
9. Projekt erstellen.

Beim ersten Start baut die Synology das Image aus dem Dockerfile. Das kann auf einer NAS mehrere Minuten dauern, weil Go, Debian-Pakete, `chromium`, `ffmpeg`, `poppler-utils` und `tesseract` in das Image einbezogen werden.

## 7. Start pruefen

Im Container Manager:

1. `Projekt` > `bearstack` oeffnen.
2. Status pruefen.
3. Falls der Container stoppt: `Container` > `bearstack` > `Details` > `Protokoll` oeffnen.

Per Browser:

```text
http://NAS-IP:8088
```

Login:

- Benutzer: `admin`
- Passwort: der Wert aus `BEARSTACK_AUTH_PASSWORD` oder das Passwort, zu dem der Hash gehoert.

Per SSH oder von einem anderen Rechner im LAN:

```sh
curl -u admin:'DEIN_PASSWORT' http://NAS-IP:8088/healthz
```

Erwartete Antwort:

```json
{"status":"ok"}
```

## 8. Fotomodul aktivieren

Erst aktivieren, wenn BearStack ohne Fotomodul sauber startet.

In `docker-compose.yml`:

```yaml
BEARSTACK_PHOTOS_ENABLED: "true"
BEARSTACK_PHOTOS_DIR: "/srv/photos"
BEARSTACK_PHOTOS_DATA_DIR: "/var/lib/bearstack/photos"
```

Und den Foto-Share read-only einbinden:

```yaml
volumes:
  - /volume1/docker/bearstack/data:/var/lib/bearstack
  - /volume1/photo:/srv/photos:ro
```

Danach Projekt neu erstellen oder aktualisieren.

Wenn die Fotos nicht erscheinen, sind meistens Dateirechte die Ursache. Der Container-Benutzer UID `10001` braucht Leserechte auf `/volume1/photo`. Aendere nicht blind alle Rechte rekursiv, wenn dort private Daten liegen; setze lieber in DSM fuer den Foto-Share eine passende Leseberechtigung oder verwende einen separaten Foto-Ordner fuer BearStack.

## 9. Reverse Proxy und HTTPS

Fuer reinen LAN-Betrieb reicht:

```text
http://NAS-IP:8088
```

Fuer HTTPS sollte TLS auf der Synology oder einem Reverse Proxy enden, nicht direkt im BearStack-Container.

Typischer Aufbau:

- Extern/LAN: `https://bearstack.example.net`
- Reverse Proxy auf Synology: Ziel `http://127.0.0.1:8088`
- BearStack im Container: `0.0.0.0:8080`

In BearStack selbst bleibt dann:

```yaml
BEARSTACK_TLS_ENABLED: "false"
```

Falls der Dienst aus dem Internet erreichbar sein soll:

- Starkes Passwort oder Passwort-Hash verwenden.
- DSM-Firewall und Router-Portfreigaben bewusst konfigurieren.
- Wenn moeglich VPN statt direkter Internetfreigabe nutzen.

## 10. Backup

Sichern musst du mindestens:

```text
/volume1/docker/bearstack/data
/volume1/docker/bearstack/app/docker-compose.yml
```

Der Datenordner enthaelt:

- SQLite-Datenbank
- Dokumentenspeicher
- Foto-Indexdatenbank, falls Fotomodul aktiv
- Thumbnail-Cache, falls Fotomodul aktiv

Mit Bind-Mount ist dieser Ordner direkt per Hyper Backup sicherbar. Wenn du stattdessen Docker-Named-Volumes verwendest, musst du gesondert pruefen, wie diese Volumes in deinem Backup erfasst werden.

## 11. Update

Wenn du aus dem Quellcode auf der Synology baust:

```sh
cd /volume1/docker/bearstack/app
git pull --ff-only
```

Danach im Container Manager das Projekt neu bauen/starten. Alternativ per SSH:

```sh
cd /volume1/docker/bearstack/app
docker compose up -d --build
```

Wenn du ein Registry-Image verwendest, ersetzt du `build:` durch `image: ghcr.io/<owner>/bearstack:<tag>` und aktualisierst spaeter den Tag oder pullst das Image neu.

## 12. Alternative: Registry-Image statt Build auf der NAS

Fuer langsame NAS-Geraete ist es oft besser, das Image auf einem anderen Rechner oder in CI zu bauen und in eine Registry zu pushen:

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/<owner>/bearstack:<tag> \
  --push .
```

Dann in der Synology-Compose-Datei:

```yaml
services:
  bearstack:
    image: ghcr.io/<owner>/bearstack:<tag>
    container_name: bearstack
    restart: unless-stopped
    init: true
    ports:
      - "8088:8080"
    environment:
      BEARSTACK_ADDR: "0.0.0.0:8080"
      BEARSTACK_DATA_DIR: "/var/lib/bearstack"
      BEARSTACK_AUTH_USER: "admin"
      BEARSTACK_AUTH_PASSWORD: "BITTE_AENDERN"
      BEARSTACK_AUTH_PASSWORD_HASH: ""
      BEARSTACK_AUTH_REALM: "BearStack"
      BEARSTACK_WEBDAV_PATH: ""
      BEARSTACK_PHOTOS_ENABLED: "false"
      BEARSTACK_PHOTOS_DIR: "/srv/photos"
      BEARSTACK_PHOTOS_DATA_DIR: "/var/lib/bearstack/photos"
    volumes:
      - /volume1/docker/bearstack/data:/var/lib/bearstack
```

## 13. Typische Fehler

- Container startet sofort neu: Auth fehlt. `BEARSTACK_AUTH_USER` plus `BEARSTACK_AUTH_PASSWORD` oder `BEARSTACK_AUTH_PASSWORD_HASH` setzen.
- `permission denied` bei Datenbank/Dokumenten: `/volume1/docker/bearstack/data` gehoert nicht UID/GID `10001`.
- Browser erreicht BearStack nicht: Port-Mapping pruefen, zum Beispiel `8088:8080`, und DSM-Firewall pruefen.
- Build bricht bei `apt-get` oder `go mod download` ab: NAS hat keinen Internetzugang oder DNS/Proxy ist falsch konfiguriert.
- Fotomodul startet nicht: `/srv/photos` existiert im Container nicht oder der gemountete Foto-Share ist fuer UID `10001` nicht lesbar.
- OCR, PDF-, EML-Archiv- oder Office-Vorschau fehlt: Image neu bauen; das Dockerfile installiert `chromium`, `libreoffice-writer`, `poppler-utils`, `tesseract-ocr`, `tesseract-ocr-deu`, `tesseract-ocr-eng` und `ffmpeg`.
