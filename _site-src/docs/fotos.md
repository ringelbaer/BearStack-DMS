---
title: Fotos
description: Foto-Galerie, Index, Suche, Worker und Metadaten.
icon: lucide/images
---

# Fotos

Das Fotomodul ist optional und nutzt einen directory-first Ansatz: BearStack importiert Fotos nicht in die Dokumentenablage, sondern rendert ein vorhandenes, read-only Fotoverzeichnis als Galerie. Die Mediendateien bleiben unverändert; BearStack legt Index, Tags, Vorschaubilder und Einstellungen getrennt davon ab.

## Aktivierung

Minimal in `.env`:

```env
BEARSTACK_PHOTOS_ENABLED=true
BEARSTACK_PHOTOS_DIR=/srv/photos
BEARSTACK_PHOTOS_DATA_DIR=/var/lib/bearstack/photos
```

`BEARSTACK_PHOTOS_DIR` ist das vorhandene read-only Fotoverzeichnis. `BEARSTACK_PHOTOS_DATA_DIR` ist der BearStack-eigene Fotobereich für erzeugte Dateien; standardmäßig liegen darunter `thumbnails/` und die separate Foto-Indexdatenbank `photos.db`. Bei Bedarf können `BEARSTACK_PHOTOS_CACHE_DIR` und `BEARSTACK_PHOTOS_DB_PATH` einzeln überschrieben werden.

Nach dem Neustart erscheint `Fotos` in der Hauptnavigation.

## Galerie und Suche

Auf kleinen Bildschirmen steht die Foto-Info unter dem Foto über die volle Breite. Das Panel ist separat scrollbar; sein Schließen-Knopf bleibt sichtbar.

Die Foto-Lightbox stoppt am ersten und letzten Medium. Die jeweiligen Navigationspfeile sind dort deaktiviert; auch die Diashow endet beim letzten Medium.

Unterstützt werden:

- Ordnernavigation, Bild- und Videowiedergabe
- Medienformate `jpg`, `jpeg`, `png`, `gif`, `webp`, `svg`, `mp4`, `webm`, `ogv` und `ogg`
- On-demand-Thumbnails für JPEG, PNG und GIF
- separate SQLite-Foto-DB für den Metadatenindex
- EXIF-Aufnahmedatum und -zeit, Kamera und GPS für JPEGs
- Adobe/MWG-XMP-Gesichtsregionen
- BearStack-Tags auf Ordnern und Medien
- Umlaut-tolerante Suche mit Feldfiltern wie `date:2024`, `directory:Urlaub`, `file_name:IMG`, `type:image`, `gps:true`, `tag:urlaub`, `person:"Marie Curie"` und `face:Marie`
- Kartenansicht mit konfigurierbarer Foto-Track-Auflösung
- Markdown-Blogdateien, GPX-Hinweise, Zufallslink und Fotoframe

## Foto-Tags

Tagänderungen aktualisieren Metadaten, Tagzuordnungen und Volltextsuche gemeinsam. Beim Hinzufügen oder Entfernen von Tags auf mehreren Fotos bleiben gleichzeitige Änderungen erhalten. Scheitert eine Bulk-Aktion, wird keine teilweise geänderte Auswahl gespeichert. Auch laufende Indexscans erhalten den aktuellen Tagstand.

Umbenennen und Entfernen eines Tags verwenden die vorhandenen Tagindizes, um nur die betroffenen Medien, Ordner und Blogs zu bearbeiten. Der Aufwand hängt damit von den betroffenen Einträgen statt von allen getaggten Dateien ab.

## Foto-Zufall und Metadaten

Foto-Info-Batchanfragen bündeln Metadaten- und Gesichtsabfragen in Paketen und teilen sich die Rechteprüfung gemeinsamer Elternordner. Dateistand und XMP-Sidecars werden weiterhin geprüft; aktuelle `.adminonly`-Markierungen und geschützte Gesichtsnamen bleiben berücksichtigt.

Der Zufallsendpunkt `/photos/random` liefert standardmäßig das Original direkt aus. Mit `size=original` bleibt es beim Original; mit `size=ordner`, `size=galerie`, `size=gross`, `size=groß` oder `size=hd` wird stattdessen die jeweilige konfigurierte Thumbnailgröße ausgeliefert.

Zusätzlich liefert der Endpunkt Metadaten als Response-Header: `X-BearStack-Photo-Title`, `X-BearStack-Photo-Path`, `X-BearStack-Photo-Folder-Path`, `X-BearStack-Photo-Folder-URL`, `X-BearStack-Photo-Folder-Title` sowie `Link: <https://.../photos?...>; rel="up"` als standardisierter Parent-Link.

## Gesichter und XMP

Gesichtsdaten werden aus eingebettetem JPEG-XMP sowie XMP-Sidecars gelesen: `photo.jpg.xmp`, `photo.jpg.XMP`, `photo.xmp` und `photo.XMP`. BearStack speichert Namen und normalisierte Gesichtsboxen im Fotoindex und liefert sie in der Foto-JSON-API aus. Sie bleiben eigene Metadaten: Gesichtsnamen werden nicht automatisch zu Foto-Tags, erscheinen nicht in Tag-Listen und sind gezielt über `person:` oder `face:` suchbar.

In der Vollansicht zeigt die Info-Seitenleiste den vollständigen Aufnahmezeitpunkt mit Datum und Uhrzeit an.

## Index und Worker

Unter `Einstellungen -> Fotos` kann die Foto-Track-Auflösung der Karte in sinnvollen Stufen von 500 m bis 10 km eingestellt werden. Sie legt fest, wie nah GPS-Fotos liegen müssen, um im fotobasierten Karten-Track zu einem Trackpunkt zusammengefasst zu werden.

Die Kartenansicht passt ihre Höhe an das Browserfenster an und nutzt den zwischen Navigation, Filtern und Footer verfügbaren Bereich vollständig aus. Auf kleinen oder sehr kurzen Fenstern bleibt eine bedienbare Mindesthöhe erhalten.

Dort kann auch ein Index-Worker aktiviert werden. Er crawlt den Foto-Root ordnerweise im Hintergrund, überspringt unveränderte Ordner anhand ihres letzten Scan-Zeitpunkts und entfernt nicht mehr vorhandene Foto-, Ordner- und Blogeinträge ordnerlokal aus dem Index. Standardmäßig ist er deaktiviert; bei Aktivierung läuft er alle 60 Minuten mit niedriger I/O-Priorität, falls vom System unterstützt, und 250 ms Pause pro gescanntem Ordner.

Bei Stat- oder Blog-Lesefehlern bleibt der Index des betroffenen Ordners einschließlich manueller Tags unverändert. Der Scan wird später erneut versucht. Vollständige Scans entfernen tatsächlich verschwundene Einträge; zusammengehörige Metadaten, Tagzuordnungen und Such-/Vorschauindizes werden je Löschpaket beziehungsweise Ordnerteilbaum gemeinsam zurückgerollt, falls die Bereinigung scheitert oder abgebrochen wird.

Der separate Thumbnail-Worker ist ebenfalls standardmäßig deaktiviert. Bei Aktivierung läuft er alle 15 Minuten, erzeugt standardmäßig bis zu 15 fehlende Thumbnails pro Lauf und nutzt standardmäßig eine Thumbnail-Parallelität von 1.

GPX-Dateien werden bis 16 MiB und 100.000 eingelesene Track-/Routenpunkte verarbeitet; größere oder fehlerhafte Dateien werden übersprungen. Wartezeiten und Lesen reagieren auf einen Anfrageabbruch. Je Foto-Library läuft höchstens ein Parser gleichzeitig. Eine Kartenantwort enthält höchstens 256 Tracks und 250.000 Punkte; weitere Tracks, die das Budget überschreiten, werden ausgelassen. Der GPX-Cache hat ein Speicherbudget von 32 MiB für Punktarrays und Eintragskosten und verdrängt die am längsten ungenutzten Tracks.

Gültige Foto-Thumbnail-Cachetreffer lesen ihre Metadaten ohne SQLite-Schreibaufruf. Nur fehlende oder zu reparierende Queue-Metadaten werden nachgetragen; ein paralleler Datenbank-Writer blockiert reguläre Cachetreffer nicht.

## Cache-Statistik

Anzahl und Größe der Thumbnail-Dateien werden beim Start und danach alle 30 Minuten im Hintergrund ermittelt, auch wenn die Thumbnail-Erzeugung deaktiviert ist. Seitenaufrufe lesen den letzten vollständigen Stand und lösen keinen Dateisystemdurchlauf aus. Die Statistik nennt den Messzeitpunkt; vor der ersten Messung erscheint „Thumbnail-Cache wird ermittelt“. Bei Fehler oder Abbruch bleibt der vorherige Stand erhalten. Gleichzeitige Aktualisierungen teilen sich einen Durchlauf; temporäre Dateien und symbolische Links werden nicht mitgezählt.

## Foto-Ordner

Beim Zurückkehren aus einem Unterordner über den Fotopfad oder Browser-Zurück/Vorwärts erscheint die Ordnerliste wieder an der vorherigen Scrollposition. Pfadlinks behalten die zuvor besuchte Sortierung, Filter und Seite bei. Ändert sich die Fensterbreite, bleibt der betretene Ordner an seiner bisherigen Bildschirmposition. Die Positionen werden innerhalb des aktuellen Tabs gespeichert.

Ordnerthumbnails zeigen Bilder bei **20 %, 40 %, 60 % und 80 %** der sichtbaren Bilder einschließlich Unterordnern. Die Auswahl verwendet absteigende Datumsreihenfolge: Aufnahmedatum, ersatzweise Dateiänderungsdatum. Bruchteile einer Bildposition werden aufgerundet; bei 100 Bildern sind es Bild 20, 40, 60 und 80.

Ordner mit bis zu vier Bildern zeigen jedes Bild höchstens einmal. Eine kleinere konfigurierte Vorschauanzahl nutzt die ersten dieser Positionen. Ordner ohne Bilder behalten Video-/Audiovorschauen. Ausgeblendete Admin-only-Bilder zählen nicht zu den Positionen der öffentlichen Ansicht. Bestehende Vorschauzuordnungen werden automatisch erneuert und die Auswahl wird zwischengespeichert.

Ordner können nach Ordnerstandard, Name, Datum oder zufällig sortiert werden. Die Datumssortierung von Ordnern nutzt das aus dem Ordnernamen erkannte Anzeigedatum. Die Ordnerstandard-Sortierung wird über eine leere Steuerdatei im Ordner gesetzt:

- `.order_descending_name.pg2conf`
- `.order_ascending_name.pg2conf`
- `.order_descending_date.pg2conf`
- `.order_ascending_date.pg2conf`
- `.order_random.pg2conf`

Ein Ordner mit der Datei `.adminonly` ist nur für Benutzer mit der Rolle `admin` zugänglich. Admin-only-Inhalte sind auch für Admins standardmäßig in Galerie, Suche, Zufall, Fotoframe, Kartenansicht und Foto-Tag-Listen ausgeblendet. Admins können sie im Sortieren-Menü der Galerie per Schalter einblenden; die Auswahl bleibt in der aktuellen Session gespeichert. Direkte Medien- und Thumbnail-URLs bleiben weiterhin nur Admins vorbehalten.

## Berechtigungen

Fotorechte sind capability-basiert und werden über Rollen oder einzelne Permissions vergeben:

| Rolle oder Recht | Wirkung |
| --- | --- |
| `photos_read` | Galerie, Suche, Medien, Thumbnails, Zufallsbild und Fotoframe lesen |
| `photos_editor` | zusätzlich Foto- und Ordner-Tags sowie Personen bearbeiten und die Personen-App nutzen |
| `photos_manager` | zusätzlich Fotoeinstellungen, Foto-Tag-Bibliothek, Index-Worker und Thumbnail-Worker verwalten |
| `admin` | vollständige Verwaltung und Zugriff auf `.adminonly`-Ordner |

Einzelrechte heißen `photos.read`, `photos.edit` und `photos.manage`. `.adminonly`-Ordner bleiben bewusst an die Rolle `admin` gebunden. Die vollständige Matrix steht unter [Benutzer und Rechte](benutzer-und-rechte.md).

Externe Werkzeuge wie `ffmpeg` und optional `vipsthumbnail` werden nur für Medienfunktionen benötigt, die sie tatsächlich brauchen.

### Optionale lokale Gesichtserkennung

Ab 0.24.0 kann BearStack Gesichter automatisch erkennen und ähnliche Gesichter zu
Personen gruppieren. Die Funktion ist standardmäßig aus. Unter **Fotos → Personen**
sehen Fotoleser die Gruppen; Fotobearbeiter (`photos.edit`, Rolle „Fotos bearbeiten“) können Namen vergeben,
Gruppen zusammenführen, ausgewählte Gesichter in andere oder neue Gruppen verschieben
und Fehlfunde ignorieren. Der Filter „Ignorierte Gesichter“ unter `/photos/people` zeigt einzelne ignorierte Gesichter, auch aus vollständig ausgeblendeten Gruppen. Er lässt sich mit Namenssuche und „Nur bekannte Personen“ kombinieren und bleibt beim Blättern erhalten. Fotobearbeiter können ein Gesicht direkt mit „Benennen und wiederherstellen“ einer neuen benannten Person zuordnen; dabei wird nur dieses Gesicht wieder sichtbar. Die Liste verwendet die vorhandenen Gesichtsvorschauen aus dem Cache und lädt höchstens 60 Gesichter pro Seite. Mit „Nur bekannte Personen“ zeigt die Personenübersicht ausschließlich benannte Gruppen; Namenssuche und Seitenwechsel behalten den Filter bei. Fotobearbeiter können Personen per Checkbox ohne sichtbaren Beschriftungstext auswählen; für Screenreader bleibt die personenbezogene Beschriftung erhalten. Ab zwei ausgewählten Gruppen erscheint unten rechts „Personen zusammenführen“ mit dem Zielnamen. Benannte Gruppen haben beim Zusammenführen Vorrang vor unbenannten: Die zuerst ausgewählte benannte Person bleibt samt Namen erhalten. Sind alle Gruppen unbenannt, bleibt die zuerst ausgewählte Person erhalten; alle ausgewählten Gruppen werden atomar per AJAX zusammengeführt und die Übersicht ohne Seitenreload aktualisiert. Die Auswahl gilt für die aktuelle Seite. Fotobearbeiter können in der Personenübersicht bei unbenannten Gruppen mit „×“ das angezeigte Gesicht ohne Seitenreload ignorieren. Bereits benannte Gruppen zeigen ab 0.39.3 weder dieses × noch den Stift für den Benenn-Dialog. Die Symbole verschwinden auch nach dem Benennen oder Zusammenführen per AJAX sofort. Weitere Gesichter der Gruppe bleiben erhalten; Vorschaubild und Fotoanzahl aktualisieren sich automatisch. Die Fotoinfo verlinkt erkannte Personen und trennt mehrere Namen mit „·“. Die Kopfaktionen der Personenseiten erscheinen als Navigationsbuttons. In der Personenübersicht öffnet bei unbenannten Gruppen das dezente Stiftsymbol in der unteren rechten Kartenecke „Benennen / zuordnen“ für Fotobearbeiter einen Dialog mit Namensvorschlägen und „Neu anlegen“. Ein neuer Name benennt die gesamte Gruppe; die Auswahl einer vorhandenen Person führt beide Gruppen zusammen. Speichern und Aktualisieren erfolgen ohne Seitenreload; Suchfilter und Seitenposition bleiben erhalten. Fehler werden im Dialog angezeigt. Über „Auswahl benennen / zuordnen“ lassen sich markierte Gruppen gemeinsam bearbeiten: Ein neuer Name führt sie unter diesem Namen zusammen; eine vorhandene benannte Person wird zum gemeinsamen Ziel. Eine bereits markierte Person kann ebenfalls Ziel sein. Die gesamte Änderung erfolgt atomar. Im Benenn-Modal speichert ein Klick auf einen Namensvorschlag oder „Neu anlegen“ sofort; dasselbe gilt für die Auswahl mit Pfeiltasten und Enter. Dies funktioniert auch für mehrere markierte Gruppen ohne Seitenreload. Autovervollständigungen zeigen ausschließlich benannte Personen; die Suche filtert diese bereits auf dem Server. Auf Personendetailseiten kombiniert das Feld „Name“ Benennen und Zusammenführen mit AJAX-Vorschlägen (Name, Personen-ID und Fotoanzahl): Ein frei eingegebener Name oder „Neu anlegen“ benennt die aktuelle Gruppe; die Auswahl einer vorhandenen Person wechselt den Button zu „Gruppen zusammenführen“. Die aktuelle Gruppe wird nicht als Ziel angeboten. Beim Verschieben bietet „Zielperson suchen“ weiterhin Autovervollständigung. Eine Person lässt sich per Klick oder mit Pfeiltasten und Enter auswählen; Escape schließt die Vorschläge. Änderungen am Suchtext verwerfen die vorherige Auswahl. Beim Verschieben erzeugt ein leeres Zielfeld eine neue Person. Auf Mobilgeräten stehen die Felder und Aktionen zum Benennen, Zusammenführen und Verschieben untereinander in voller Breite. Die Paginierung bietet „Erste Seite“, „Zurück“, „Weiter“ und „Letzte Seite“ sowie die aktuelle Seite und Gesamtseitenzahl. Nicht verfügbare Randaktionen sind deaktiviert. Die Personenübersicht merkt sich Seite und Filter im Browser pro Benutzer, auch über Browser-Neustarts hinweg. Beim erneuten Öffnen von `/photos/people` wird diese Ansicht wiederhergestellt; ausdrücklich angegebene Seiten oder Suchfilter haben Vorrang. Nicht mehr vorhandene Seiten werden auf die letzte verfügbare Seite begrenzt. Dies gilt auch für AJAX-Aktualisierungen; Personendetailseiten und ignorierte Gesichter haben passende Gesamtseitenzahlen. Auf Personenseiten bleiben die Kacheln quadratisch und lange Dateipfade brechen innerhalb der Karte um. Ab BearStack 0.39.2 werden Gesichtsausschnitte proportional eingepasst und zentriert; freie Flächen erhalten einen dunkelgrauen Hintergrund. Die Vorschauen werden bereits auf dem Server unverzerrt erzeugt, sodass dies auch in der Android-App ohne App-Update gilt. Bereits im App-Speicher geladene Altbilder werden nach erneutem Anmelden oder einem App-Neustart neu geladen. Beim Überfahren wird der Dateipfad unterstrichen, die Bildfläche bleibt unverändert. Auf einer Personenseite öffnen Gesichtsbild und Dateiname die Foto-Lightbox; dort stehen Navigation, Zoom und Bildinformationen zur Verfügung. `person:Juergen`
und `face:"Marie Curie"` suchen sowohl XMP-Namen als auch benannte automatische Gruppen.

Gesichtsvorschauen werden unter `BEARSTACK_PHOTOS_CACHE_DIR/faces/v1` als JPEG zwischengespeichert, auch über Neustarts hinweg. Aktive Gesichtsvorschauen bleiben ohne Größen- oder Anzahlbegrenzung gespeichert. Beim Ignorieren erhalten vorhandene Vorschauen eine feste Ablaufzeit von höchstens 48 Stunden; erneute Aufrufe oder wiederholtes Ignorieren verlängern diese Frist nicht. Abgelaufene Vorschauen werden auch ohne weitere Zugriffe automatisch gelöscht. Ein späterer Aufruf kann eine neue Vorschau erzeugen, die ebenfalls höchstens 48 Stunden gespeichert wird. Gesichtsdaten, Embeddings und die Möglichkeit zur Wiederherstellung bleiben dauerhaft erhalten. Ignorierte Gesichter dienen zu keinem Zeitpunkt als Referenzen für den Gesichtsabgleich. Beim Wiederherstellen entfällt die Ablaufzeit der Vorschau. Die Bereinigung läuft unabhängig von der aktivierten Gesichtserkennung, berücksichtigt Neustarts und verarbeitet nur fällige Cacheeinträge in kleinen, indexgestützten Paketen. Beim Update bleibt der bestehende Cache unter `faces/v1` erhalten. Die Zuordnung zu Gesichts-IDs wird einmalig im Hintergrund in Paketen von höchstens 100 Gesichtern ergänzt und nach Unterbrechungen fortgesetzt. Dazu werden vorhandene Indexdaten und die beiden möglichen Vorschaupfade geprüft; Originalbilder werden weder gelesen noch neu dekodiert. Die Zuordnung allein erhält Dateinamen, Bildinhalt und Änderungszeiten. Ab 0.39.2 verwenden proportional eingepasste Vorschauen einen neuen Rendering-Schlüssel: Beim nächsten Abruf wird die angeforderte Größe einmal neu erzeugt und nur ihre konkrete gestreckte Altvorschau entfernt. Andere Größen und nicht angeforderte Vorschauen bleiben bis zu ihrem Abruf oder einer bestehenden Ablaufzeit erhalten. Es gibt keinen vollständigen Cache-Neuaufbau; fehlgeschlagene Bereinigungen werden beim nächsten Abruf erneut versucht. Ignorierte Altvorschauen erhalten eine feste Frist von höchstens 48 Stunden ab dem Update, die bei Neustarts nicht verlängert wird. Nicht mehr zuordenbare Altdateien werden nicht pauschal gelöscht. Hashbasierte Unterverzeichnisse vermeiden übergroße Einzelordner; Cache-Zugriffe benötigen keinen Verzeichnisscan und keine vollständige Dateiliste im Arbeitsspeicher. Die Größen 160 und 640 Pixel werden getrennt gespeichert; parallele Abrufe desselben Ausschnitts teilen sich eine Erzeugung. Personenübersicht, Personendetailseite, AJAX und Android-/Labeling-API verwenden denselben Datei-Cache: Vorhandene Vorschauen werden direkt daraus ausgeliefert; fehlt eine Vorschau, wird sie erzeugt und dort gespeichert. Der Browser darf Web-Gesichtsvorschauen privat speichern, muss sie aber vor erneuter Verwendung validieren (ETag/304). Authentifizierung, aktuelle Sichtbarkeit und Quelldatei werden weiterhin geprüft. Bei AJAX-Aktualisierungen bleiben vorhandene Bild-Elemente erhalten; Änderungen an Namen oder Fotoanzahlen laden unveränderte Bilder nicht erneut.

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
[OpenAPI-Beschreibung](https://github.com/ringelbaer/BearStack-DMS/blob/main/openapi.yaml).

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

## Android-Personen-App

Ab BearStack 0.30.0 lassen sich unbenannte Gruppen auch mit der [nativen Android-App](android.md) bearbeiten. Die Anleitung beschreibt HTTPS-Anmeldung, Gesten, Statistik und den privaten APK-Build.
