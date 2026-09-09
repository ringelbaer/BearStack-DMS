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

Auch ein Symlink namens `.adminonly` schützt ab 0.42.1 den Ordner, unabhängig davon, ob sein Ziel erreichbar ist. Fehler beim Lesen der Markierung erlauben keinen öffentlichen Zugriff. Kann der Sichtbarkeitsabgleich beim Start einen indizierten Pfad wegen Zugriffsfehlern oder Symlinks nicht sicher prüfen, startet das Fotomodul nicht; Indexflags und Gesichtsdaten bleiben dabei unverändert. In diesem Fall den gemeldeten Pfad beziehungsweise dessen Zugriffsrechte korrigieren und BearStack erneut starten.

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
und Fehlfunde ignorieren. Der Filter „Ignorierte Gesichter“ unter `/photos/people` zeigt einzelne ignorierte Gesichter, auch aus vollständig ausgeblendeten Gruppen. Er lässt sich mit Namenssuche und „Nur bekannte Personen“ kombinieren und bleibt beim Blättern erhalten. Fotobearbeiter können ein Gesicht direkt mit „Benennen und wiederherstellen“ einer neuen benannten Person zuordnen; dabei wird nur dieses Gesicht wieder sichtbar. Die Liste verwendet die vorhandenen Gesichtsvorschauen aus dem Cache und lädt höchstens 60 Gesichter pro Seite. Mit „Nur bekannte Personen“ zeigt die Personenübersicht ausschließlich benannte Gruppen; Namenssuche und Seitenwechsel behalten den Filter bei. Ab **0.44.0** zeigt **„Nur unbekannte Personen“** ausschließlich unbenannte Gruppen mit aktiven, nicht ignorierten Gesichtern. Benannte Gruppen und vollständig ignorierte Gruppen entfallen; bei gemischten Gruppen zählen und erscheinen nur die aktiven Gesichter. Der Filter verwendet `unknown=1`, bleibt beim Blättern, nach AJAX-Aktionen und bei der Rückkehr aus Personendetails erhalten und wird im Browser pro Benutzer gespeichert. Ab **0.47.0** bietet ein gemeinsames Select die Auswahl **„Alle“**, **„Nur bekannte Personen“**, **„Nur unbekannte Personen“** und **„Ignorierte Gesichter“**. Die Auswahl ist exklusiv. Unter „Nur unbekannte Personen“ entfällt die wiederholte sichtbare Beschriftung „Unbenannt“; zugängliche Link- und Aktionsnamen bleiben erhalten. Bei widersprüchlichen URL-Parametern hat `unknown=1` Vorrang. Namenssuche ist weiterhin kombinierbar; ein Suchtext ohne passende unbenannte Namen liefert eine leere Liste. Benannte oder vollständig ignorierte Gruppen verschwinden nach der Bearbeitung sofort aus dieser Ansicht. Der kleine Button **„Alle Filter aufheben“** leert die Namenssuche, setzt die Auswahl auf „Alle“ und öffnet Seite 1; der leere Filterzustand ersetzt auch die gespeicherte Auswahl. Der Button funktioniert ebenfalls ohne JavaScript. Fotobearbeiter können Personen per Checkbox ohne sichtbaren Beschriftungstext auswählen; für Screenreader bleibt die personenbezogene Beschriftung erhalten. Ab zwei ausgewählten Gruppen erscheint unten rechts „Personen zusammenführen“ mit dem Zielnamen. Benannte Gruppen haben beim Zusammenführen Vorrang vor unbenannten: Die zuerst ausgewählte benannte Person bleibt samt Namen erhalten. Sind alle Gruppen unbenannt, bleibt die zuerst ausgewählte Person erhalten; alle ausgewählten Gruppen werden atomar per AJAX zusammengeführt und die Übersicht ohne Seitenreload aktualisiert. Die Auswahl gilt für die aktuelle Seite. Fotobearbeiter können in der Personenübersicht bei unbenannten Gruppen mit „×“ das angezeigte Gesicht ohne Seitenreload ignorieren. Bereits benannte Gruppen zeigen ab 0.39.3 weder dieses × noch den Stift für den Benenn-Dialog. Die Symbole verschwinden auch nach dem Benennen oder Zusammenführen per AJAX sofort. Weitere Gesichter der Gruppe bleiben erhalten; Vorschaubild und Fotoanzahl aktualisieren sich automatisch. Die Fotoinfo verlinkt erkannte Personen und trennt mehrere Namen mit „·“. Die Kopfaktionen der Personenseiten erscheinen als Navigationsbuttons. In der Personenübersicht öffnet bei unbenannten Gruppen das dezente Stiftsymbol in der unteren rechten Kartenecke „Benennen / zuordnen“ für Fotobearbeiter einen Dialog mit Namensvorschlägen und „Neu anlegen“. Ein neuer Name benennt die gesamte Gruppe; die Auswahl einer vorhandenen Person führt beide Gruppen zusammen. Speichern und Aktualisieren erfolgen ohne Seitenreload; Suchfilter und Seitenposition bleiben erhalten. Fehler werden im Dialog angezeigt. Über „Auswahl benennen / zuordnen“ lassen sich markierte Gruppen gemeinsam bearbeiten: Ein neuer Name führt sie unter diesem Namen zusammen; eine vorhandene benannte Person wird zum gemeinsamen Ziel. Eine bereits markierte Person kann ebenfalls Ziel sein. Die gesamte Änderung erfolgt atomar. Ab 0.43.0 zeigt das Benenn-Modal bei vorhandenen Personen zusätzlich ein Thumbnail neben Name, ID und Fotoanzahl. Es verwendet dasselbe aktive Gesicht wie die Personenübersicht. Die kleinen Vorschaubilder werden bei Bedarf aus dem vorhandenen Cache geladen; auch bei einem Bildfehler bleibt der Vorschlag auswählbar. „Neu anlegen“ erhält kein Thumbnail. Die Vorschlagsliste nutzt die freie Höhe neben der Fotovorschau und scrollt bei längeren Trefferlisten innerhalb dieses Bereichs. Im Benenn-Modal speichert ein Klick auf einen Namensvorschlag oder „Neu anlegen“ sofort; dasselbe gilt für die Auswahl mit Pfeiltasten und Enter. Dies funktioniert auch für mehrere markierte Gruppen ohne Seitenreload. Autovervollständigungen zeigen ausschließlich benannte Personen; die Suche filtert diese bereits auf dem Server. Auf Personendetailseiten kombiniert das Feld „Name“ Benennen und Zusammenführen mit AJAX-Vorschlägen (Name, Personen-ID und Fotoanzahl): Ein frei eingegebener Name oder „Neu anlegen“ benennt die aktuelle Gruppe; die Auswahl einer vorhandenen Person wechselt den Button zu „Gruppen zusammenführen“. Die aktuelle Gruppe wird nicht als Ziel angeboten. Beim Verschieben bietet „Zielperson suchen“ weiterhin Autovervollständigung. Eine Person lässt sich per Klick oder mit Pfeiltasten und Enter auswählen; Escape schließt die Vorschläge. Änderungen am Suchtext verwerfen die vorherige Auswahl. Beim Verschieben erzeugt ein leeres Zielfeld eine neue Person. Auf Mobilgeräten stehen die Felder und Aktionen zum Benennen, Zusammenführen und Verschieben untereinander in voller Breite. Die Paginierung bietet „Erste Seite“, „Zurück“, „Weiter“ und „Letzte Seite“ sowie die aktuelle Seite und Gesamtseitenzahl. Nicht verfügbare Randaktionen sind bei mehreren Seiten deaktiviert. Bei nur einer Seite bleibt ab 0.43.1 ausschließlich „Seite 1 von 1“ sichtbar; „Erste Seite“ und „Letzte Seite“ entfallen. Das gilt für Personendetails, Personenübersicht und ignorierte Gesichter sowie nach AJAX-Aktualisierungen. Die Personenübersicht merkt sich Seite und Filter im Browser pro Benutzer, auch über Browser-Neustarts hinweg. Beim erneuten Öffnen von `/photos/people` wird diese Ansicht wiederhergestellt; ausdrücklich angegebene Seiten oder Suchfilter haben Vorrang. Nicht mehr vorhandene Seiten werden auf die letzte verfügbare Seite begrenzt. Dies gilt auch für AJAX-Aktualisierungen; Personendetailseiten und ignorierte Gesichter haben passende Gesamtseitenzahlen. Auf Personenseiten bleiben die Kacheln quadratisch und lange Dateipfade brechen innerhalb der Karte um. Ab BearStack 0.39.2 werden Gesichtsausschnitte proportional eingepasst und zentriert; freie Flächen erhalten einen dunkelgrauen Hintergrund. Die Vorschauen werden bereits auf dem Server unverzerrt erzeugt, sodass dies auch in der Android-App ohne App-Update gilt. Bereits im App-Speicher geladene Altbilder werden nach erneutem Anmelden oder einem App-Neustart neu geladen. Beim Überfahren wird der Dateipfad unterstrichen, die Bildfläche bleibt unverändert. Auf einer Personenseite öffnen Gesichtsbild und Dateiname die Foto-Lightbox; dort stehen Navigation, Zoom und Bildinformationen zur Verfügung. `person:Juergen`
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

    Scheitert der Download unter macOS mit `CERTIFICATE_VERIFY_FAILED`, kann der
    Python-Installation ihr CA-Bundle fehlen. Ist `/etc/ssl/cert.pem` vorhanden,
    lässt sich das System-CA-Bundle für diesen Aufruf verwenden:

    ```sh
    SSL_CERT_FILE=/etc/ssl/cert.pem .venv-faces/bin/python services/faces/download_models.py "$HOME/.local/share/bearstack-face-models"
    ```

    TLS-Zertifikate und Modell-Prüfsummen werden dabei weiterhin geprüft.

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

**Optimierungen ab 0.41.1:** Gesichtsausschnitte desselben Fotos teilen dessen
verkleinerte und ausgerichtete Bildaufbereitung. Der Cache hält höchstens acht Bilder
und 32 MiB im Arbeitsspeicher. Dateiaustausch und geänderte XMP-Sidecars ändern den
Cache-Schlüssel; Quell- und Schutzprüfungen bleiben auch bei Cachetreffern aktiv.
Kann der Index einen geänderten Foto-Fingerprint nicht speichern, wird der Abruf
abgebrochen, statt alte Gesichtsregionen weiterzuverwenden.

Das fünfsekündliche Status-Polling verwendet `/settings/photos/faces?format=json&progress=1`.
Dieser Modus liefert Zähler des zuletzt indexierten Bestands, ohne globale
Dateisystemprüfung und ohne Liste einzelner Dateifehler. Mehrere Aufrufe teilen
höchstens fünf Sekunden lang denselben Zählerstand. Neue Schutzmarkierungen wirken
auf diese Zahlen nach der nächsten Index- oder Sichtbarkeitsaktualisierung; konkrete
Gesichtsabrufe prüfen den Schutz weiterhin sofort. Die vollständige HTML-Ansicht
und JSON ohne `progress=1` prüfen die Sichtbarkeit wie bisher.

Der gemeinsame Personen-Dialog und seine Suche werden unabhängig von der
Personenübersicht geladen; die Gruppenbilder-Ansicht benötigt nur ihr eigenes
Modul und `app-person-dialog.js`.

**Anzeigeeinstellungen der Personenübersicht (ab 0.47.0):** Das **…-Menü**
unter `/photos/people` steuert **Ordnername anzeigen**, **Fotoanzahl anzeigen** und
**Thumbnailgröße S, M oder L**. S entspricht der bisherigen Größe (bis 160 Pixel),
M verwendet bis 224 Pixel und L bis 320 Pixel; kleine Bildschirme begrenzen die
Breite automatisch. Standard: Fotoanzahl sichtbar, Ordner ausgeblendet, Größe S.
Der Ordner steht unter der Fotoanzahl und gehört zum jeweiligen Vorschaubild;
andere Fotos der Person können aus anderen Ordnern stammen. Für das Stammverzeichnis
steht „Fotos“. Die Einstellungen bleiben pro Benutzer im Browser gespeichert und
gelten auch nach AJAX-Aktualisierungen. Das Umschalten verwendet die vorhandenen
Bilder und benötigt keine Serverabfrage. Ohne JavaScript gilt die Standardansicht.

**Benenn-Modal (ab 0.47.0):** Links erscheint das Foto mit einem Ausschnitt von
300 % der Bounding Box des gewählten Vorschaubilds und der Gesichtsmarkierung,
rechts stehen Name und Vorschläge. Auf schmalen Bildschirmen steht das Bild darüber.
Bei Mehrfachauswahl wird das erste ausgewählte Gruppenportrait gezeigt. Im
Gruppenbildmodus entspricht die Vorschau genau dem angeklickten Gesicht. Das Foto
wird erst beim Öffnen geladen; ein Bildfehler blockiert das Benennen nicht. Die
Personenliste liefert Ordner und Bounding Box mit derselben Seitenabfrage, ohne
zusätzliche Einzelabfragen pro Person. Rechte und aktuelle Sichtbarkeitsprüfungen
bleiben auch beim Laden der Fotovorschau wirksam.

Ab **0.47.1** steht im Benenn-Modal **„Ignorieren“** links neben **„Abbrechen“**.
Die Aktion ignoriert das angezeigte Gesicht; bei mehreren ausgewählten unbenannten
Gruppen werden deren angezeigte Vorschaubilder gemeinsam ignoriert. Andere Gesichter
der Gruppen bleiben aktiv. Bereits benannte Gruppen deaktivieren diese Aktion.
Ein eingegebener Name wird dabei nicht gespeichert. Nach Erfolg schließt sich das
Modal und die Ansicht aktualisiert sich. Schreibfehler bleiben im Modal sichtbar;
schlägt nur das Nachladen nach erfolgreichem Speichern fehl, bleibt erneutes
Speichern gesperrt. Im Gruppenbildmodus gilt die vorhandene Revisionsprüfung;
bei einem Konflikt wird das Foto aktualisiert und die Auswahl muss nach Schließen
des Modals erneut geprüft werden.

**Gruppenbilder bearbeiten:** Der Button **Gruppenbilder** unter `/photos/people`
öffnet `/photos/people/groups` für Fotobearbeiter (`photos.edit`). Die Schwelle ist
oben einstellbar (0–255, Standard **5**) und wird pro Nutzer und Browser gemerkt.
Es erscheinen ausschließlich Fotos mit **mehr als** dieser Anzahl unbenannter,
nicht ignorierter Gesichter. Benannte und ignorierte Gesichter zählen nicht für die
Auswahl, werden im geöffneten Foto aber weiterhin als Vorschauen gezeigt.

Ab **0.46.0** blendet **„Nur Unbenannte anzeigen“** im Gesichtsraster benannte
und ignorierte Vorschauen aus. Der Filter wirkt sofort ohne zusätzlichen
Serverabruf und merkt sich die Auswahl pro Benutzer im Browser. Nach Benennen,
Zuordnen oder Ignorieren wird das Raster automatisch angepasst. Wird eine
vergrößerte Vorschau ausgeblendet, erscheint wieder das ganze Foto. Ohne passende
Gesichter erscheint ein Hinweis; das aktuelle Foto bleibt geöffnet. Ausschalten
zeigt wieder alle Vorschauen. Der Anzeigefilter benötigt JavaScript und ändert
weder die Auswahl der Gruppenfotos noch die Aktion „Verbleibende ignorieren“.

Links steht eine proportional eingepasste Fotovorschau, rechts das Gesichtsraster.
Ab **0.43.1** zeigt das Raster größere Gesichtsvorschauen mit bis zu **200 × 200 Pixeln**.
Die Spaltenzahl passt sich der verfügbaren Breite an; auf schmalen Bildschirmen
werden die Kacheln untereinander angeordnet. Vorhandene Vorschaubilder und Cache
werden weiterverwendet.
Hover oder Tastaturfokus einer Vorschau markiert die zugehörige Region im Foto.
Ab **0.45.1** vergrößert ein Klick oder Antippen den Ausschnitt auf **300 % der
Bounding Box**: Ein Bereich mit dreifacher Breite und Höhe der Gesichtsmarkierung
wird proportional in die Fotoansicht eingepasst. Am Bildrand wird der Ausschnitt
ins Foto verschoben; große Gesichtsregionen verkleinern das Foto nicht weiter.
Ein weiterer Klick auf dieselbe Vorschau zeigt wieder das vollständige Foto,
ein Klick auf eine andere Vorschau wechselt direkt zu deren Gesicht. Mit der
Tastatur funktioniert der Wechsel über **Enter** oder **Leertaste**. Die aktive
Vorschau ist hervorgehoben. Beim nächsten Foto wird der Zoom zurückgesetzt;
Benennen im selben Foto erhält ihn. Die Vergrößerung nutzt die bereits geladene
Fotovorschau ohne zusätzliche Bildabrufe. Die Markierung hat eine dünne
1-Pixel-Linie mit dunkler Kontrastkontur und berücksichtigt Bildränder,
Seitenverhältnis und die EXIF-Ausrichtung, auch beim Vergrößern und
Ändern der Fenstergröße. Auf kleinen Bildschirmen stehen Foto und Raster untereinander.
Ab **0.43.1** umrandet eine zusätzliche Markierung das Thumbnail, dessen Stift-Dialog
gerade geöffnet ist. Sie bleibt auch bei Speicherfehlern sichtbar und verschwindet
beim Schließen des Dialogs. Bei mehreren Gesichtern derselben Person wird nur die
tatsächlich angeklickte Vorschau markiert. Die Zoomauswahl bleibt dabei erhalten.

Der bekannte Stift-Dialog benennt die **gesamte Personengruppe** oder führt sie mit
einer vorhandenen Person zusammen, einschließlich anderer Fotos. Benannte oder
ignorierte Gesichter zeigen keinen Stift. Das geöffnete Foto bleibt auch unterhalb
der Schwelle sichtbar, damit die übrigen Gesichter weiter bearbeitet werden können.

Ab **0.45.0** hat jede unbenannte, aktive Gesichtsvorschau ein kleines **×**
(**Dieses Gesicht ignorieren**). Es ignoriert ausschließlich diese Erkennung im
aktuellen Foto. Andere Gesichter derselben Person bleiben erhalten, auch im selben
Foto. Die Vorschau bleibt als „Ignoriert“ sichtbar; Zähler und Aktionen aktualisieren
sich ohne Seitenwechsel, die Zoomauswahl bleibt bestehen. Die Aktion funktioniert
auch ohne JavaScript und kehrt dann zum selben Foto zurück. Sie verwendet dieselbe
Revisions- und Rechteprüfung wie die Sammelaktion. Schlägt nur das Aktualisieren nach
dem Speichern fehl, lädt „Ansicht erneut laden“ die Daten ohne erneute Schreibaktion.

**Verbleibende ignorieren** ignoriert atomar ausschließlich die noch unbenannten,
aktiven Gesichter des angezeigten Fotos und wechselt danach zum nächsten passenden
Foto. Bereits benannte Gesichter und dieselben Personen auf anderen Fotos bleiben
erhalten. Ein zwischenzeitlich geändertes Foto oder eine neue Zuordnung führt zu
`409`; die Ansicht wird aktualisiert, bevor die Auswahl erneut bestätigt werden
kann. Schlägt nur das Laden des nächsten Fotos fehl, wiederholt „Ansicht erneut
laden“ den Bildwechsel, ohne erneut zu ignorieren.

**Überspringen / nächstes Foto** lässt alle Gesichtsdaten unverändert und setzt den
aktuellen Durchlauf fort. **Durchlauf starten** beginnt wieder am Anfang und zeigt
auch zuvor übersprungene, weiterhin passende Fotos. Der Pfad-Cursor steht in der URL;
zum nächsten Foto wird ohne Seitenreload gewechselt. Ignorieren und Überspringen
funktionieren auch ohne JavaScript, der Modal-Dialog und die Markierungen benötigen
JavaScript. Private, fehlende oder ersetzte Quellen werden ausgeschlossen.

Die Auswahl liest einen kleinen Index über aktive Gesichter in Pfadreihenfolge;
sie benötigt weder Offset-Paginierung noch einen vollständigen Dateisystemscan.
Eine Fotoansicht enthält höchstens 256 Gesichter, deren vorhandene Vorschau-Caches
weiterverwendet und bei Bedarf erst beim Scrollen geladen werden. Für das große
Foto wird die Galerie-Vorschau mit maximal 1.600 Pixeln verwendet; ohne passenden
Vorschaugenerator steht das Original als Fallback zur Verfügung. Foto und unveränderte
Gesichtsbilder werden beim Benennen nicht erneut geladen. Die automatische Migration
auf Foto-Schema **23** ergänzt lediglich den Auswahlindex und analysiert keine Bilder neu.
Die JSON-Ansicht sowie der Ignorieren- und Bild-Endpunkt sind in OpenAPI dokumentiert.

**Bessere Gesichtszuordnung (ab 0.48.0):** Der Abgleich bewertet sämtliche
zulässigen Referenzvektoren exakt. Anschließend werden die besten unterschiedlichen
Personen mit aktuellen Sichtbarkeits- und Zuordnungsprüfungen ausgewählt. Viele
ähnliche Referenzen oder bereits im Foto zugeordnete Personen verdrängen damit
keine passenden Vergleichsgruppen. Die bisherigen Grenzwerte für neue Fotos
bleiben bei 0,55 Ähnlichkeit und 0,08 Abstand zur zweitbesten Person.

Unter **Einstellungen → Gesichtserkennung → Vorhandene Zuordnungen verbessern**
arbeitet ein separat pausierbarer Hintergrundlauf mit gespeicherten Vektoren,
auch ohne laufenden Erkennungsdienst. Er ist standardmäßig aktiviert und wird nach
Benennen, manuellen Korrekturen, Favoriten-/Referenzänderungen und erfolgreichen
Analysepaketen vorgemerkt. **Zuordnungen erneut prüfen** startet eine neue Prüfung;
**Abgleich pausieren/fortsetzen** erhält deren Fortschritt. Cursor und Ergebnisse
werden gemeinsam gespeichert; Abbruch und Neustart verlieren keine abgeschlossenen
Pakete. Geprüft werden höchstens 100 Datensätze je Paket mit zusätzlichem Zeitbudget.

Automatisch verschoben werden nur unbenannte, unbestätigte, nicht ignorierte und
nicht favorisierte Gesichter zu ausdrücklich benannten Personen. Dafür gelten
strengere Grenzwerte von 0,62 und 0,10 Abstand. Manuelle Trennungen bleiben erhalten;
eine bereits im selben Foto vorhandene Zielperson ist ausgeschlossen. Schlechte
Aufnahmen mit gemessener unzureichender Qualität treiben keinen automatischen
Nachabgleich an. Die Zähler zeigen geprüfte Datensätze, neue Zuordnungen und im
aktuellen Lauf erzeugte Vorschläge.

**Ähnliche Gruppen** unter `/photos/people/merge-suggestions` zeigt Fotobearbeitern
bis zu 60 gespeicherte Zusammenführungsvorschläge. Auch ähnliche unbenannte
Teilgruppen werden berücksichtigt. Vorschläge ab 0,45 Ähnlichkeit sind keine
Wahrscheinlichkeitsangaben und benötigen eine Prüfung. **Zusammenführen** verwendet
die angezeigten Gruppenrevisionen; zwischenzeitliche Änderungen verlangen eine
neue Prüfung. **Getrennt lassen** bleibt als Entscheidung gespeichert und verhindert
auch künftige automatische Zuordnungen zwischen diesen Gruppen. Aufrufe der Seite
lösen keine neue Vektorsuche aus. Bearbeiten benötigt `photos.edit`, Einstellungen
und Steuerung benötigen `photos.manage`.

Ab Android-App **0.9.0** und BearStack **0.49.0** ist **Ähnliche Gruppen** auch über das App-Menü verfügbar: immer eine Entscheidung mit zwei Portraits und festen Buttons, ohne scrollbare Vorschlagsliste. Nach **Zusammenführen** oder **Getrennt lassen** lädt automatisch das nächste Gruppenpaar. Beide Portraits bieten dieselbe Originalfoto-Vergrößerung per Halten und Wischen wie beim Benennen/Zuordnen. Aktionsquittungen verhindern doppelte Änderungen bei verlorenen Antworten; Konflikte verlangen eine erneute Prüfung. Details stehen in der [Android-Anleitung](android.md#ahnliche-gruppen).

**Kleine Gesichter:** Nach der ersten Analyse des maximal 1.600 Pixel großen Fotos
werden bei Bedarf bis zu acht Originalausschnitte nachanalysiert. Das Original wird
dafür einmal zusätzlich dekodiert; Ausschnitte sind auf 1.600 Pixel und zusammen
16 MiB begrenzt. Nur eindeutig wiedergefundene Gesichter mit ausreichend höherer
Auflösung übernehmen den verbesserten Vektor. Die ursprünglichen Markierungen
bleiben stabil. Ausfälle der optionalen Nachanalyse behalten das erste Ergebnis;
Abbruch, Quelländerungen und Schutzmarkierungen stoppen die Verarbeitung.

Der aktualisierte Gesichtsdienst liefert zusätzlich Gesichtsauflösung und Schärfe.
Neue automatisch ausgewählte Referenzen benötigen mindestens 48 Gesichtspixel und
Schärfewert 20; diese technischen Mindestwerte sind keine allgemeine Genauigkeits-
garantie. Explizite Favoriten dürfen die Qualitätsauswahl überschreiben, niemals
Sichtbarkeitsregeln. Ältere kompatible Dienste und gespeicherte Vektoren ohne
Qualitätswerte bleiben verwendbar. Für gemessene Qualitätsfilter den Gesichtsdienst
mit aktualisieren. Modelle und Protokoll 1 bleiben kompatibel. Foto-Schema 25
migriert automatisch, ohne bestehende Namen oder Gesichter zu löschen.

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
nächsten Analyse in kurzen, fortsetzbaren Schritten übernommen. Die Referenzauswahl selbst führt keine Gruppen zusammen. Der separate
Hintergrundabgleich prüft unbestätigte Zuordnungen und erzeugt Vorschläge. Die Android-Oberfläche erhält
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
migriert automatisch auf Schema 24 (ab 0.43.0 zusätzlich ein partieller Index für benannte Personen). Ihre Sicherung muss die erzeugten Gesichtsdaten
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

Ab BearStack 0.30.0 lassen sich unbenannte Gruppen auch mit der [nativen Android-App](android.md) bearbeiten. Ab App 0.6.0 und BearStack 0.43.0 bietet „Menü → Personen“ zusätzlich alle benannten Personen mit Portraits, Umbenennen, einzelne Zuordnungen zurücksetzen, Favoriten, Galeriesuche im Browser und Originalfoto-Vorschau per Halten und Wischen. Die Anleitung beschreibt HTTPS-Anmeldung, Gesten, Statistik und den privaten APK-Build.

Ab Android-App 0.7.0 und BearStack 0.45.0 durchsucht das Textfeld im Personenbereich alle benannten Personen. Personenliste und Portrait-Raster laden beim Scrollen automatisch nach. Das Entfernen einer Zuordnung verlangt zuvor eine Bestätigung mit Name und Bildpfad.
