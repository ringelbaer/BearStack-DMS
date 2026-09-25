---
title: Einrichtung und Betrieb
description: Fotomodul und Gesichtserkennung einrichten, Index und Caches betreiben sowie Daten und Zugriffe verwalten.
icon: lucide/settings
---

# Einrichtung und Betrieb

Diese Referenz richtet sich an Betreiber einer BearStack-Installation. Für die
tägliche Benutzung gibt es die [Foto-Anleitung](fotos.md) und die
[Personenanleitung](fotos-personen.md). Die allgemeine Servereinrichtung steht
unter [Installation](installation.md).

## Fotomodul aktivieren

Ein vorhandenes Fotoverzeichnis und ein getrenntes, beschreibbares Datenverzeichnis
bereitstellen. Anschließend BearStack konfigurieren:

```env
BEARSTACK_PHOTOS_ENABLED=true
BEARSTACK_PHOTOS_DIR=/srv/photos
BEARSTACK_PHOTOS_DATA_DIR=/var/lib/bearstack/photos
```

`BEARSTACK_PHOTOS_DIR` ist der bestehende Foto-Root. BearStack liest ihn, schreibt
aber keine Tags, Vorschauen oder Gesichtsdaten hinein. Bei Containerbetrieb muss
der konfigurierte Pfad innerhalb des Containers erreichbar sein.

`BEARSTACK_PHOTOS_DATA_DIR` enthält die erzeugten Daten, standardmäßig
Thumbnail-Cache und `photos.db`. Bei Bedarf können `BEARSTACK_PHOTOS_CACHE_DIR`
und `BEARSTACK_PHOTOS_DB_PATH` diese Pfade getrennt vorgeben. Die Foto-Datenbank
ist unabhängig von der Dokumentenablage.

BearStack neu starten und einem Benutzer **Fotos lesen** zuweisen. Danach
erscheint **Fotos** in seiner Navigation. Unter **Einstellungen → Fotos** lassen
sich Darstellung, Intervalle und Worker konfigurieren; dafür ist **Fotos
verwalten** erforderlich. Mit **Jetzt indexieren** den Bestand erfassen und
anschließend einen Ordner, ein Foto und dessen Informationen prüfen.

## Berechtigungen und geschützte Ordner

| Rolle | Berechtigung |
| --- | --- |
| `photos_read` | Galerie, Suche, Medien, Vorschauen, Zufallsbild, Fotoframe und Personen ansehen. |
| `photos_editor` | Zusätzlich Foto-/Ordner-Tags und Personen bearbeiten. |
| `photos_manager` | Zusätzlich Fotoeinstellungen, Foto-Tag-Bibliothek, Index-/Thumbnail-Worker und Gesichtserkennung verwalten. |
| `admin` | Vollständige Verwaltung und Zugriff auf geschützte `.adminonly`-Ordner. |

Die Einzelrechte heißen `photos.read`, `photos.edit` und `photos.manage`.
Die [vollständige Rechtematrix](benutzer-und-rechte.md) beschreibt ihre Vergabe.

Eine Datei namens `.adminonly` schützt einen Fotoordner. Auch ein Symlink mit
diesem Namen schützt ihn, unabhängig davon, ob sein Ziel erreichbar ist.
Lesefehler der Markierung geben den Ordner nicht öffentlich frei.

Geschützte Inhalte bleiben an die Rolle **admin** gebunden. Auch für Admins sind
sie in Galerie, Suche, Zufall, Fotoframe, Karte und Foto-Tag-Listen zunächst
ausgeblendet; **Galerie sortieren → Admin-only anzeigen** schaltet sie für die
aktuelle Sitzung ein. Direkte Medien- und Thumbnail-URLs erfordern weiterhin
Adminrechte.

Die automatische Gesichtserkennung und virtuelle Personengalerie schließen
geschützte Fotos aus. Nachträgliche Schutzmarkierungen entfernen automatische
Ergebnisse und Referenzen aus dem aktiven Bestand. Aktuelle Sichtbarkeitsprüfungen
gelten auch beim Abruf vorhandener Vorschauen.

Kann der Sichtbarkeitsabgleich beim Start einen indizierten Pfad wegen fehlender
Rechte oder Symlinks nicht sicher prüfen, startet das Fotomodul nicht.
Indexflags und Gesichtsdaten bleiben unverändert. Den gemeldeten Pfad und seine
Rechte korrigieren und BearStack erneut starten.

## Formate und Metadaten

Die Galerie unterstützt unter anderem `jpg`, `jpeg`, `png`, `gif`, `webp`, `svg`,
`mp4`, `webm`, `ogv` und `ogg`. JPEG, PNG und GIF erhalten Vorschauen bei Bedarf.
Externe Werkzeuge wie `ffmpeg` und optional `vipsthumbnail` werden nur für die
jeweiligen Medienfunktionen benötigt. Wiedergabe hängt außerdem von den Formaten
ab, die der verwendete Browser unterstützt.

Aus JPEGs liest BearStack EXIF-Aufnahmedatum und -zeit, Kamera und GPS sowie
eingebettete Adobe/MWG-XMP-Gesichtsregionen. Zusätzlich werden Sidecars mit den
Namen `photo.jpg.xmp`, `photo.jpg.XMP`, `photo.xmp` und `photo.XMP` berücksichtigt.

XMP-Sidecars müssen reguläre Dateien von höchstens **4 MiB** sein. Symlinks,
Spezialdateien, zu große oder beim Öffnen ausgetauschte Dateien werden
übersprungen; die feste Grenze gilt auch während des Lesens. Das Foto bleibt
ohne diese optionalen Metadaten verfügbar.

Namen und normalisierte Gesichtsboxen liegen im Fotoindex. Sie sind über
`person:` beziehungsweise `face:` suchbar, werden aber nicht zu Foto-Tags.
Originale und Sidecars bleiben unverändert. Markdown-Blogtexte und GPX-Dateien
können den Fotoordner ergänzen.

### Standardreihenfolge eines Ordners

Eine leere Steuerdatei im jeweiligen Ordner bestimmt dessen **Ordnerstandard**:

| Datei | Reihenfolge |
| --- | --- |
| `.order_descending_name.pg2conf` | Name absteigend. |
| `.order_ascending_name.pg2conf` | Name aufsteigend. |
| `.order_descending_date.pg2conf` | Datum absteigend. |
| `.order_ascending_date.pg2conf` | Datum aufsteigend. |
| `.order_random.pg2conf` | Zufällig. |

Die Dateien werden außerhalb von BearStack angelegt. Anzeigedatum und formatierter
Ordnername werden aus dem Verzeichnisnamen abgeleitet; technische Pfade ändern
sich dadurch nicht.

## Index und Hintergrundverarbeitung

Die beiden Foto-Worker sind standardmäßig **ausgeschaltet**. Ohne
Thumbnail-Worker entstehen fehlende Vorschauen weiterhin bei Bedarf.

| Worker | Standard bei Aktivierung | Aufgabe |
| --- | --- | --- |
| Index-Worker | Alle 60 Minuten, 250 ms Pause pro gescanntem Ordner. | Bestand, Metadaten, Ordner, Blogs und GPX-Dateiindex aktualisieren. |
| Thumbnail-Worker | Alle 15 Minuten, bis zu 15 Thumbnails je Lauf, Parallelität 1. | Fehlende Galerie-Thumbnails vorab erzeugen. |

Unter **Einstellungen → Fotos** lassen sich Intervalle und Pakete anpassen.
**Jetzt indexieren** beziehungsweise **Fehlende Thumbs jetzt erstellen** startet
die jeweilige Arbeit gezielt. Die Thumbnail-Parallelität begrenzt auch die
Erzeugung beim ersten Abruf. Größere Galerieseiten und Vorschauen erhöhen
Ladeaufwand und Speicherbedarf; für kleine Server mit den Vorgaben beginnen.

Die WebUI lädt Foto-Skripte passend zur Ansicht: Der Frame benötigt nur
Medienhelfer und Frame-Steuerung; Personenübersichten und Personenordner
benötigen keine Galerie-Skripte. Personen-Details und Bildgruppen behalten die
Großansicht. Kartenprojektion und Kacheldarstellung werden von der kleinen
GPS-Karte in der Großansicht wiederverwendet; die vollständige Kartensteuerung
wird ausschließlich auf der Kartenseite geladen. Bildgruppenaktionen werden
nur in den passenden Bearbeitungsansichten geladen.

Die WebUI fragt den Thumbnail-Status für höchstens 200 Medien je Anfrage ab.
Auch Netzwerkfehler, ungültige oder unvollständige Antworten zählen zur Grenze
von 40 Versuchen mit wachsendem Abstand. Jede Anfrage hat ein Zeitlimit von
15 Sekunden. Dauerhafte HTTP-Clientfehler und Anmeldeweiterleitungen stoppen
die Abfrage sofort; 408 und 429 werden begrenzt wiederholt. Ein späteres
Neuladen der Galerie startet die Prüfung erneut.

Jeder Thumbnail-Konverter (`vipsthumbnail` oder `ffmpeg`) erhält höchstens
30 Sekunden Laufzeit. Timeout und Abbruch stoppen weitere Konverterversuche
und das aktuelle Worker-Paket. Der Fehler wird gespeichert; das bestehende
Wiederholungsintervall gilt weiter und die Job-Sperre wird freigegeben.

HTTP-Originalabrufe und Fotoübertragungen öffnen Originale ausschließlich
lesend über festgehaltene Verzeichnis-Handles. Der Foto-Root bleibt während der
Library-Laufzeit gebunden. Ausgetauschte Symlink-Pfade und abweichende
Dateiidentitäten werden abgelehnt; Schutzmarker werden auch an den geöffneten
Verzeichnissen geprüft. Nach einem Austausch des Foto-Root-Verzeichnisses ist
ein Neustart erforderlich, um den neuen Root zu verwenden.

Der Indexscan arbeitet ordnerweise, überspringt unveränderte Ordner anhand des
Scanstands und nutzt eine niedrige I/O-Priorität, sofern unterstützt. Bei Stat-
oder Blog-Lesefehlern bleibt der Index des betroffenen Ordners einschließlich
manueller Tags erhalten; ein späterer Lauf versucht ihn erneut.

Erfolgreiche vollständige Scans bestätigen fehlende Inhalte. Für deren
Aufbewahrung gelten die [Regeln für Ordnerumzüge](#ordnerumzuge-und-aufbewahrung).
Metadaten, Tags sowie Such- und Vorschauindizes werden je Löschpaket oder
Teilbaum gemeinsam aktualisiert und bei Fehler oder Abbruch zurückgerollt.

Foto-Tags und Volltextsuche werden gemeinsam geändert. Sammelaktionen sind atomar;
parallele Änderungen und Indexscans erhalten den aktuellen Tagstand. Das globale
Umbenennen oder Entfernen eines Tags bearbeitet über Tagindizes nur betroffene
Medien, Ordner und Blogs.

## Vorschauen und Caches

### Medienvorschauen

Ordnermosaike wählen Bilder bei **20 %, 40 %, 60 % und 80 %** der sichtbaren
Bilder einschließlich Unterordnern. Grundlage ist die absteigende Datumsfolge
nach Aufnahmezeit, ersatzweise Dateiänderungszeit; Bruchteile werden aufgerundet.
Bei 100 Bildern sind es die Bilder 20, 40, 60 und 80.

Ordner mit bis zu vier Bildern zeigen jedes höchstens einmal. Eine kleinere
eingestellte Vorschauanzahl nutzt die ersten Positionen. Ordner ohne Bilder
behalten Video-/Audiovorschauen. Ausgeblendete Admin-only-Bilder zählen in der
öffentlichen Auswahl nicht mit. Die Zuordnung wird zwischengespeichert und bei
Änderungen aktualisiert.

Virtuelle Personen- und Tag-Ordner zeigen doppelt so viele Gesichtsvorschauen
wie normale Ordner, also **2 bis 8**. Die Auswahl bevorzugt benannte aktive
Personen mit den meisten unterschiedlichen sichtbaren Fotos. Aktive
Stern-Favoriten bestimmen bevorzugt das Porträt; bei mehreren der mit der
kleinsten Gesichts-ID. Die Vorschauen verwenden den Gesichts-Cache.

Ein gültiger Thumbnail-Cachetreffer benötigt keinen SQLite-Schreibzugriff.
Nur fehlende oder reparaturbedürftige Queue-Metadaten werden nachgetragen.
Metadaten-Batchanfragen bündeln Foto- und Gesichtsabfragen sowie gemeinsame
Rechteprüfungen, prüfen Dateistand, Sidecars und aktuelle Schutzmarkierungen aber
weiterhin.

### Cache-Statistik

Anzahl und Größe der Thumbnail-Dateien werden beim Start und anschließend alle
**30 Minuten** im Hintergrund gemessen, auch bei deaktivierter Erzeugung. Ein
Seitenaufruf liest den letzten vollständigen Stand mit Messzeitpunkt und löst
keinen Dateisystemdurchlauf aus.

Vor der ersten Messung erscheint **Thumbnail-Cache wird ermittelt**. Fehler oder
Abbruch erhalten den vorherigen Stand. Gleichzeitige Aktualisierungen teilen
sich einen Durchlauf; temporäre Dateien und Symlinks zählen nicht mit.

### Gesichtsvorschauen

Der gemeinsame Datei-Cache liegt unter
`BEARSTACK_PHOTOS_CACHE_DIR/faces/v1`. Weboberfläche und APIs verwenden daraus
proportional eingepasste JPEGs in getrennten Größen von **160 und 640 Pixeln**.
Hashbasierte Unterordner vermeiden große Einzelverzeichnisse; parallele Abrufe
desselben Ausschnitts teilen sich die Erzeugung.

Aktive Gesichtsvorschauen bleiben ohne feste Anzahl- oder Größenbegrenzung
gespeichert. Beim Ignorieren erhalten vorhandene Vorschauen eine feste Frist
von höchstens **48 Stunden**. Erneute Aufrufe oder wiederholtes Ignorieren
verlängern sie nicht. Abgelaufene Dateien werden automatisch entfernt; ein
späterer Abruf kann eine neue Vorschau mit neuer begrenzter Frist erzeugen.
Die Gesichtsdaten und die Möglichkeit zum Wiederherstellen bleiben bestehen.
Wiederherstellen hebt die Ablaufzeit der Vorschau auf.

Die Bereinigung läuft unabhängig von der Gesichtserkennung in kleinen,
indexgestützten Paketen und berücksichtigt Neustarts. Bestehende Vorschauen
bleiben bei Updates erhalten; noch fehlende Cachezuordnungen werden fortsetzbar
in Paketen von höchstens 100 Gesichtern ergänzt, ohne Originalbilder zu lesen.
Geänderte Rendering-Schlüssel erneuern nur die angeforderte Größe beim Abruf;
es gibt keinen pauschalen Neuaufbau und keine pauschale Löschung unbekannter
Altdateien.

Web-Vorschauen dürfen privat mit ETag/304 zwischengespeichert werden; Rechte,
Sichtbarkeit und Quelle werden erneut geprüft. Benennen und AJAX-Aktualisierungen
erhalten unveränderte Bildelemente. Für mehrere Gesichter desselben Fotos teilen
sich Abrufe die ausgerichtete, verkleinerte Vorlage in einem Speicher-Cache von
höchstens **8 Bildern und 32 MiB**. Datei- oder Sidecaränderungen ändern seinen
Schlüssel. Kann ein geänderter Fingerabdruck nicht gespeichert werden, bricht
der Abruf ab.

## Karten und GPX

Die Karte verwendet Fotokoordinaten und vorhandene GPX-Tracks. Die
**Foto-Track-Auflösung** unter **Einstellungen → Fotos** gruppiert nahe Fotos in
Stufen von **500 m bis 10 km**. Browser und Android verwenden dieselbe
zeitlich-räumliche Routengruppierung. Auch Routen über die geografische
Datumsgrenze (±180° Längengrad) werden bei der Aufbereitung berücksichtigt.

Der reguläre Indexscan erfasst GPX-Dateien, ohne sie dabei vollständig zu parsen.
Rechte und Pfade einschließlich Symlinks werden vor der Ausgabe geprüft. Die
Trackauswahl arbeitet in begrenzten Indexpaketen; während eines noch unvollständigen
Indexaufbaus bleibt ein Dateisystem-Fallback verfügbar.

| Grenze | Wert |
| --- | --- |
| Größe einer GPX-Datei | 16 MiB. |
| Eingelesene Track-/Routenpunkte je Datei | 100.000. |
| Gleichzeitige Parser je Fotobibliothek | 1. |
| Tracks je Kartenantwort | Höchstens 256. |
| Punkte je Kartenantwort | Höchstens 250.000. |
| GPX-Speicher-Cache | 32 MiB einschließlich Punktarrays und Eintragskosten. |

Zu große oder fehlerhafte Dateien werden übersprungen. Tracks, die das
Antwortbudget überschreiten, werden ausgelassen. Lesen und Warten reagieren auf
Anfrageabbruch; der Cache verdrängt die am längsten ungenutzten Tracks.

Ein optionaler Kartenindex wird im Hintergrund aufgebaut. Währenddessen bleibt
die Galerie bedienbar; Kartenanfragen warten abbrechbar auf den Index. Ein
Fehler meldet die Karte als noch nicht verfügbar und erlaubt einen späteren
Versuch. Große Sortierungen können temporäre Daten auf Datenträger auslagern.

### Fotorouten-Cache

Browser und native API teilen JSON-Dateien unter
`<Cache>/photo-routes/v1/`. Die vollständige gruppierte Route wird pro Ordner,
Radius, Medientyp und Sichtbarkeit gespeichert. Ausschnitt und Antwortlimit
werden anschließend angewendet. Beliebige Textsuchen erhalten keinen dauerhaften
Routencache.

Transaktionale Indexrevisionen machen betroffene Routen nach Änderungen an GPS,
Aufnahmezeit oder Sichtbarkeit ungültig. Ordnerumzüge berücksichtigen alte und
neue Vorfahren sowie gelöschte Teilbäume. Änderungen außerhalb des Bereichs
erhalten den Cache. Gleichzeitige Erzeugungen werden zusammengefasst; Dateien
werden atomar mit versioniertem Format geschrieben.

Die Browserroute umfasst die gesamte indexierte Auswahl. Für die HTML-Ausgabe
werden höchstens **8.192 gruppierte Orte** dargestellt, bei längeren Routen über
deren gesamte Länge vereinfacht. Anfang, Ende und ursprüngliche Zeit-/Fotozahlen
der verbleibenden Orte bleiben erhalten. Die Cachedatei enthält weiterhin alle
gruppierten Orte. Vor dem ersten vollständigen Indexlauf bleibt die direkte
Berechnung aus dem Dateisystem verfügbar.

Die native API begrenzt Kartenmarker auf 289, Tracklisten auf 32 Einträge je
Cursorseite und einzelne Trackantworten auf 8.192 Koordinaten. Appseitige Speicher-
und Darstellungsgrenzen stehen in der [Android-Referenz](android-technik.md).

## Gesichtserkennung einrichten

Die optionale automatische Erkennung ist standardmäßig ausgeschaltet. Sie läuft
in einem separaten lokalen Dienst mit **OpenCV, YuNet und SFace**. Dieser hat
weder Zugriff auf den Foto-Root noch auf die Datenbank. BearStack überträgt
ausgerichtete JPEGs mit höchstens **1.600 Pixeln** Kantenlänge an ihn.

Bilder bleiben in der eigenen Infrastruktur. Die Modelle werden bei Einrichtung
oder Image-Build mit festen SHA-256-Prüfsummen geladen und beim Start geprüft;
im laufenden Betrieb gibt es keine Modelldownloads.

### Mit Docker Compose

1. Mit `openssl rand -hex 32` einen zufälligen gemeinsamen Token erzeugen.
2. In `.env` `BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN` auf diesen Wert setzen.
3. Das Fotomodul konfigurieren und
   `docker compose --profile faces up -d --build` starten.
4. Unter **Einstellungen → Gesichtserkennung** die Verarbeitung einschalten.
   Dafür muss der Dienst erreichbar und kompatibel sein.

Compose verwendet intern `http://faces:8091` ohne veröffentlichten Dienstport.
Für eine eigene Dienstadresse stehen folgende Optionen zur Verfügung:

| JSON-Feld unter `photos` | Umgebungsvariable | Bedeutung |
| --- | --- | --- |
| `face_service_url` | `BEARSTACK_PHOTOS_FACE_SERVICE_URL` | HTTP(S)-Adresse des Erkennungsdienstes. |
| `face_service_token` | `BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN` | Gemeinsamer geheimer Token mit mindestens 32 Zeichen. |

### Ohne Compose

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
    **Fotos → Personen verwalten** bereit. Der vorhandene Fotobestand wird schrittweise verarbeitet.

Für dauerhaften Betrieb muss der Python-Prozess ebenfalls durch einen Dienstmanager
wie systemd gestartet werden. Dabei einen absoluten Pfad zu `.venv-faces/bin/python`,
`services/faces/server.py` und den Modellen verwenden sowie die drei Dienstvariablen
aus Schritt 3 setzen. BearStack startet diesen Prozess nicht selbst. Die Begrenzungen
auf einen halben CPU-Kern und 1 GiB RAM gelten nur für die Compose-Konfiguration;
bei nativem Betrieb müssen entsprechende Ressourcenlimits im Dienstmanager gesetzt werden.
Der einzelne Inferenzthread und die BearStack-Pausen gelten auch ohne Compose.

## Erkennung steuern und Ergebnisse beurteilen

### Ressourcen und Fortschritt

Der Erkennungs-Worker verarbeitet standardmäßig **100 Bilder pro Lauf**, mit
**1 Sekunde Pause pro Bild** und **15 Minuten zwischen Läufen**. Einstellbar sind
1–1.000 Bilder, 100–60.000 ms Pause und 1–1.440 Minuten zwischen Läufen.
Der Dienst nutzt einen Inferenzthread; Compose begrenzt ihn auf **0,5 CPU-Kerne
und 1 GiB RAM**. Native Dienste brauchen eigene Ressourcenlimits.

Der Erstlauf arbeitet die Sammlung schrittweise ab. Ist der separate Index-Worker
aus, nutzt die Erkennung dessen Scanmechanismus stündlich. Anschließend entstehen
Aufträge direkt bei Indexänderungen. Unveränderte Bilder, auch solche ohne
erkannte Gesichter, werden nicht erneut analysiert. Fehler werden mit zunehmender
Wartezeit bis zu fünfmal versucht und können manuell zurückgesetzt werden.
Die persistente Warteschlange setzt nach Neustarts fort.

Die Einstellungen trennen Status, Verarbeitung und Löschaktion. Das Status-Polling
alle fünf Sekunden verwendet zuletzt indexierte Zähler statt eines globalen
Dateisystemscans. Neue Schutzmarkierungen wirken hier nach dem nächsten Index-
oder Sichtbarkeitsabgleich; konkrete Gesichtsabrufe prüfen den Schutz sofort.

### Referenzen und Qualität

Die Erkennung unterstützt JPEG, PNG, WebP und das erste GIF-Bild. Videos und SVG
werden nicht analysiert; Bilder über **40 Megapixel** werden zur Speicherbegrenzung
zurückgewiesen. Sehr kleine, verdeckte oder unscharfe Gesichter können fehlen.

Ein zweiter Suchlauf mit höchstens **320 Pixeln** Kantenlänge ergänzt große
Porträtgesichter, die in der 1.600-Pixel-Vorlage übersehen wurden. Für kleine
Gesichter können bis zu **acht Originalausschnitte** nachanalysiert werden,
jeweils höchstens 1.600 Pixel und zusammen **16 MiB**. Das Original wird dafür
einmal zusätzlich dekodiert. Nur eindeutig wiedergefundene Regionen mit besserer
Auflösung übernehmen den verbesserten Vektor; ein Fehler der optionalen
Nachanalyse erhält das erste Ergebnis.

Die Zielanzahl der **Referenzen pro Person** ist auf **1 bis 100** einstellbar,
Standard **30**. Die Auswahl verteilt sich möglichst über mehrere Galerieordner.
Innerhalb eines Ordners haben manuelle Zuordnungen Vorrang, danach die
Erkennungssicherheit. Neue automatisch ausgewählte Referenzen benötigen bei
vorhandenen Qualitätswerten mindestens **48 Gesichtspixel** und **Schärfewert 20**.
Das sind technische Filter, keine Genauigkeitsgarantie. Ältere kompatible Dienste
und gespeicherte Vektoren ohne diese Werte bleiben verwendbar.

Alle aktiven Stern-Favoriten werden vollständig verglichen, auch oberhalb des
Referenzlimits. Bei weniger Favoriten füllen weitere Gesichter bis zur Zielanzahl
auf; bisher nicht vertretene Ordner haben Vorrang. Favoriten dürfen die
Qualitätsauswahl übersteuern, niemals Sichtbarkeit oder einen veralteten
Quellstand. Ignorierte, geschützte und manuell gezeichnete Gesichter dienen nicht
als automatische Vergleichsreferenzen.

Änderungen der Zielanzahl werden vor der nächsten Analyse in kurzen,
fortsetzbaren Schritten übernommen. Die Referenzauswahl selbst führt keine
Gruppen zusammen.

### Zuordnungen verbessern

Neue Gesichter werden zuerst mit **benannten Gruppen** verglichen. Nur ohne
eindeutigen Treffer folgt ein eigener Vergleich mit **unbenannten Gruppen**.
Pro Gruppe zählt die beste zulässige Referenz; der Mindestabstand bezieht sich
auf eine andere Gruppe im selben Bereich.

Im Expertenbereich sind drei Wertepaare einstellbar:

| Abgleich | Ähnlichkeit | Mindestabstand |
| --- | --- | --- |
| Automatische Zuordnung bei der Erkennung | 0,55 | 0,08 |
| Automatische Zuordnung im Hintergrund | 0,62 | 0,10 |
| Vorschläge zur manuellen Prüfung | 0,45 | 0,00 |

Die Tabelle zeigt die Standardwerte. Zulässig sind **0,4 bis 0,7** für Ähnlichkeit
und **0,0 bis 0,2** für Mindestabstand. Höhere Werte filtern strenger; die Werte
sind keine Wahrscheinlichkeiten und lernen nicht automatisch aus Entscheidungen.
Bei manuellen Vorschlägen erlaubt Abstand 0 mehrere Alternativen, ein positiver
Abstand nur einen eindeutig besten Treffer.

Ab **1.6.0** werden für **Ähnliche Gruppen** pro unbenanntem Gesicht zuerst bis
zu drei passende benannte Personen ausgewählt. Nur wenn keine davon die
Vorschlagsgrenzwerte erfüllt, folgen bis zu drei unbenannte Gruppen. Der
Mindestabstand wird jeweils innerhalb desselben Bereichs geprüft, auch gegen
den zweitbesten Treffer unterhalb der Ähnlichkeitsgrenze. Abgelehnte Paare,
gemeinsame aktive Fotos, Ordnerausschlüsse und ungültige Referenzen bleiben
ausgeschlossen. Die Regeln für automatische Einzelzuordnungen bleiben bestehen.

Web und Android lesen gespeicherte Paare mit mindestens einer benannten Person
zuerst, danach rein unbenannte Paare; innerhalb beider Bereiche gilt absteigende
Ähnlichkeit und bei Gleichstand die Vorschlags-ID. Bereits gespeicherte
unbenannte Vorschläge bleiben erhalten. Die neue Auswahl gilt beim nächsten
Hintergrundabgleich; **Zuordnungen erneut prüfen** stößt ihn ausdrücklich an.

**Vorhandene Zuordnungen verbessern** arbeitet mit gespeicherten Vektoren und
benötigt keinen laufenden Erkennungsdienst. Der Hintergrundabgleich ist
standardmäßig aktiviert und wird unter anderem nach Benennungen, manuellen
Korrekturen, Referenzänderungen und erfolgreichen Analysepaketen vorgemerkt.
**Zuordnungen erneut prüfen** startet eine neue Prüfung; **Abgleich pausieren /
fortsetzen** erhält den Fortschritt. Cursor und Ergebnisse werden gemeinsam
in Paketen von höchstens 100 Datensätzen mit zusätzlichem Zeitbudget gespeichert.

Automatisch verschoben werden nur unbenannte, unbestätigte, nicht ignorierte
und nicht favorisierte Gesichter zu ausdrücklich benannten Personen.
Manuelle Trennungen bleiben erhalten. Eine im selben Foto bereits vorhandene
Zielperson ist ausgeschlossen; unzureichende gemessene Qualität treibt keinen
automatischen Nachabgleich an.

**Unbenannte Gruppen automatisch zusammenführen** ist zusätzlich und
standardmäßig **ausgeschaltet**. Aktiviert kann sie die ganze unbenannte
Quellgruppe atomar verbinden, zunächst mit benannten, danach mit unbenannten
Gruppen. Für beide gelten die Hintergrundgrenzwerte. Gruppen mit manuellen
Zuordnungen, Favoriten, ignorierten oder gezeichneten Gesichtern, ungeeigneten
Merkmalen, geschützten Mitgliedern oder widersprechenden XMP-Namen sind als
automatische Quellgruppe ausgeschlossen. Abgelehnte Paare, gemeinsame aktive
Fotos, manuelle Trennungen und Familienkonflikte verhindern die Verbindung.
Tags, Beziehungen und Ablehnungen gegenüber anderen Gruppen bleiben erhalten.

Geänderte Grenzwerte oder das Umschalten der Gruppenoption verwerfen offene
Vorschläge und planen einen neuen Abgleich. Ein pausierter Lauf bleibt pausiert;
manuelle Entscheidungen und bereits erfolgte Zuordnungen werden nicht
zurückgesetzt. Es erfolgt keine neue Bildanalyse.

### Statistik richtig lesen

**Bestehender Gruppe zugeordnet**, **Neue Gruppe gebildet** und **Zuordnungsanteil**
beschreiben die Entscheidung bei der Bildanalyse für aktuell vorhandene,
nicht ignorierte Gesichter, einschließlich XMP-Zuordnungen.
Der Anteil ist `zugeordnet / (zugeordnet + neu) × 100`, ohne Basis 0 %.

Spätere manuelle Änderungen oder Hintergrundzuordnungen ändern diese ursprüngliche
Einteilung nicht. Gelöschte und ignorierte Gesichter entfallen. Bei neuer Analyse
wird die Entscheidung neu erfasst; übernommene Korrekturen behalten ihre Einteilung.
Altbestände ohne gespeicherte Entscheidung stehen unter **Noch nicht erfasst**
und zählen nicht zum Prozentwert.

## Ordnerumzüge und Aufbewahrung

Medien, Ordner, Markdown-Blogs und GPX-Dateien besitzen dauerhafte interne
Identitäten. Umbenennen, Verschieben sowie Kopieren und späteres Löschen finden
außerhalb von BearStack statt. Der nächste vollständige Indexlauf kann eindeutige
Umzüge **innerhalb desselben Foto-Roots** zuordnen.

Bei erfolgreicher Zuordnung bleiben Tags, Personen-/Gesichts-IDs und manuelle
Entscheidungen erhalten. Vorhandene Gesichtsvorschauen und unveränderte
Foto-/Video-Thumbnails werden wiederverwendet.

### Ablauf bei fehlenden Ordnern

1. Den externen Umzug abschließen und einen vollständigen Indexlauf abwarten
   oder unter **Einstellungen → Fotos** anstoßen.
2. **Ordnerumzüge und aufbewahrte Fotodaten** öffnen. Dort stehen erfasste Umzüge,
   fehlende Einträge mit Ablaufdatum und der Fingerabdruckfortschritt.
3. Falls keine eindeutige automatische Zuordnung möglich ist, die fehlende
   Ordneridentität einem vorhandenen Ziel zuordnen und Konflikte prüfen.

Fehlende Inhalte werden **sieben Tage** aufbewahrt und aus normalen Ansichten
ausgeblendet. Scans und Neustarts verlängern die Frist nicht. Ein unvollständiger
oder fehlgeschlagener Scan bestätigt keine Löschung; ein ausgefallener oder
verdächtig leerer Root löst keine endgültige Bereinigung aus. Die endgültige
Löschung betrifft nur BearStacks aufbewahrte Daten und Caches.

### Wann die Zuordnung eindeutig ist

Vergleichsdateien müssen unverändert an denselben relativen Pfaden liegen.
Automatisch genügen mindestens **zwei unterschiedliche, nichtleere
SHA-256-Fingerabdrücke** oder die eindeutige Übereinstimmung zweier
Ein-Datei-Ordner. Identische Kopien bleiben eigenständig, solange beide existieren.

Leere Ordner, fehlende historische Fingerabdrücke und mehrdeutige Kopien können
eine manuelle Zuordnung benötigen. Unabhängige manuelle Änderungen am Ziel
verhindern eine automatische Übernahme. Zusätzliche oder gelöschte Dateien
schließen die Zuordnung nicht aus, wenn genug eindeutige Vergleichsdateien bleiben.
Markdown/GPX und XMP werden unabhängig aktualisiert; ein Dateitypwechsel gilt als
neuer Eintrag.

### Geänderte Bilder und Datenmigration

Ändern sich die Bytes eines Fotos am bisherigen Pfad, bleiben bestätigte
Gesichtsdaten und Vorschauen zunächst erhalten. **Foto geändert / Prüfen**
kennzeichnet sie, alte Embeddings dienen auch bei Favoriten nicht als Referenzen.
Die [Quellenprüfung](fotos-personen.md#gesichter-geanderter-fotos-prufen) benötigt
`photos.edit`, die Ordnerverwaltung `photos.manage`.

Personenzusammenführungen berücksichtigen auch aufbewahrte und durch Ordnerschutz
verborgene Gesichter. Nach Rückkehr des Fotos gilt die Zuordnung zur verbleibenden
Person. Aktuelle Rechte gelten ebenso für gespeicherte Vorschauen und Referenzen.
Alte pfadbasierte URLs werden nach Umzügen nicht automatisch weitergeleitet.

Migrationen erhalten vorhandene IDs und Cachedateien. SHA-256-Erfassung und
Cacheinventar werden fortsetzbar aufgebaut; unveränderte Folgeläufe lesen keine
Dateiinhalte, Galerieaufrufe berechnen keine vollständigen Hashes. Für noch nicht
erfasste Altbestände ist automatische Wiedererkennung nicht zugesichert.
Foto-Schema 37 synchronisiert aufbewahrte Spalten nach versionierten Migrationen;
Umbenennungen, Entfernungen oder Typwechsel benötigen explizite Datenmigrationen.

## Daten sichern und Gesichtsdaten löschen

Den Foto-Root und BearStacks Foto-Datenbank getrennt in die Sicherung aufnehmen.
Die Datenbank enthält auch Tags, erzeugte Gesichter, manuelle Korrekturen,
Favoriten, Personenstammdaten und Aufbewahrungsstände. Sie ist nicht bloß ein
beliebig ersetzbarer Vorschaubereich. Die allgemeinen
[Betriebshinweise](installation.md) ergänzen die Sicherung der übrigen Installation.

Ein Index-Neuaufbau erhält Korrekturen unveränderter Bilder. Für ersetzte oder
fehlende Quellen gelten die Prüf- und Aufbewahrungsregeln oben. Bei Modellwechseln
werden manuelle Zuordnungen nur auf eindeutig wiedergefundene Regionen übertragen;
unsichere Treffer bleiben getrennt. XMP und automatische Gesichter werden getrennt
gespeichert, manuelle Zuordnungen haben Vorrang.

**Pausieren oder Ausschalten** der Erkennung erhält die bisherigen Ergebnisse.
Die separate Löschaktion unter **Einstellungen → Gesichtserkennung** entfernt
erzeugte Gesichtsdaten, Gruppen und Korrekturen und schaltet die Verarbeitung aus.
Importierte XMP-Gesichtsdaten bleiben erhalten. Erforderlich sind `photos.manage`,
die ausdrückliche Löschbestätigung und das aktuelle Passwort des angemeldeten
Benutzers. Fehlende oder falsche Bestätigungen verändern nichts; wiederholte
Fehlversuche werden gedrosselt.

Merkmalsvektoren werden weder an den Browser ausgegeben noch in Logs geschrieben.

## Schnittstellen und technische Grenzen

Die vollständigen Verträge stehen in der
[OpenAPI-Beschreibung](https://github.com/ringelbaer/BearStack-DMS/blob/main/openapi.yaml).
Diese Übersicht ordnet die wichtigsten Schnittstellen ihren Aufgaben zu.

### Zufallsbild einbinden

`GET /photos/random` liefert standardmäßig das Original direkt aus.
`size=original` behält dieses Verhalten bei. Mit `size=ordner`, `size=galerie`,
`size=gross` beziehungsweise `size=groß` oder `size=hd` wird die jeweilige
konfigurierte Vorschaugröße verwendet. Authentifizierung und Sichtbarkeit gelten
auch für diesen Abruf.

Zusätzliche Response-Header liefern den Kontext:

- `X-BearStack-Photo-Title` und `X-BearStack-Photo-Path`
- `X-BearStack-Photo-Folder-Path` und `X-BearStack-Photo-Folder-URL`
- `X-BearStack-Photo-Folder-Title`
- `Link: <https://…/photos?…>; rel="up"` als standardisierter Parent-Link

### Galerie, Personen und Aktionen

| Schnittstelle | Zweck und Besonderheit |
| --- | --- |
| `/photos/frame/items` | Paginiertes Nachladen für den Web-Fotoframe. Der Browser hält höchstens zwei Metadatenseiten; neue Durchläufe beginnen wieder auf Seite 1. |
| `/photos/people` | HTML und JSON verwenden dieselbe Sortierung: `name_asc/desc`, `count_asc/desc`, `folder_asc/desc`, `date_asc/desc`. Ungültige Werte liefern 400; die JSON-Seite enthält `sort`. |
| `GET /photos/faces?path=…` | Vorhandene Gesichter; ohne bisherige Analyse eine leere Liste. Benötigt `photos.edit`. |
| `POST /photos/faces/analyze` | Einzelnes Foto analysieren; Warten und Analyse zusammen höchstens zwei Minuten. Benötigt `photos.edit`, startet keinen globalen Lauf. |
| `GET /photos/faces/drawing-image` und `POST /photos/faces/manual` | Manuelle Gesichtsrahmen mit `photos.edit`, ohne Erkennungsdienst oder Hintergrundlauf. |
| `POST /photos/faces/unignore` | Mit `path` und `revision` alle ignorierten Gesichter eines Fotos wiederherstellen. Liefert die Anzahl; benötigt `photos.edit`. |
| `POST /photos/faces/reset-ignored-directory` | Mit `path` ignorierte unbenannte Gesichter eines zulässigen Ordnerteilbaums zurücksetzen. Benötigt `photos.edit`; JSON enthält `ok` und `restored`, Formulare erhalten eine Weiterleitung. |
| `GET /photos/faces/{id}/suggestions` | Vergleich mit benannten Referenzen; benötigt `photos.edit`, verändert keine Zuordnung. JSON oder `application/x-ndjson` mit abschließendem `done=true`. |
| `GET` / `PUT /api/photos/labeling/v1/faces/{id}/favorite` | Favorit lesen oder gezielt setzen; PUT mit `person_id` und booleschem `favorite`, benötigt `photos.edit`. Wiederholung schaltet nicht erneut um. |
| `/settings/photos/faces?format=json&progress=1` | Kurzer Fortschrittsstand mit maximal fünf Sekunden geteiltem Zählercache, ohne globale Dateisystemprüfung oder einzelne Dateifehler. |

Die Ordner-Rücksetzung verwendet exakte indizierte Pfadgrenzen, auch bei `%` und
`_`, und eine gemeinsame Transaktion für Gruppen und Referenzen. Sie dekodiert
keine Bilder, funktioniert auch mit mehr als 500 betroffenen Gesichtern und kann
ohne zusätzliche Änderungen wiederholt werden.

Labeling-Seiten verwenden Cursorpagination mit Sichtbarkeitsprüfung in kleinen
Paketen. Neue Schutzmarkierungen und entfallende XMP-Namen werden berücksichtigt.
Personen-Tags und Stammdaten werden bei Bedarf geladen, statt jede Übersicht
mit allen Beziehungen zu belasten.

### Begrenzte Arbeit bei großen Sammlungen

Der Lupenvergleich verwendet unveränderliche Referenzstände benannter Gruppen
und blockiert während Vergleich und Streaming keine weiteren Änderungen oder
Suchen. Er prüft alle passenden Gruppen und Favoriten; **512 Gesichts-IDs sind
eine Datenbankpaketgröße, kein Bestandslimit**. Zwischenstände erscheinen höchstens
alle 100 ms, das Endergebnis immer. Die Lupe startet keinen globalen Cache-Neuaufbau.
Messungen beschreibt der [Performance-Bericht](tests-und-audit.md#performance-des-lupen-gesichtabgleichs).

Gruppenbilder werden über einen Index in Pfadreihenfolge ausgewählt. Die
Bilderleiste hält höchstens 96 Einträge im Browser, die Fotoansicht höchstens
256 Gesichter. Die große Vorschau ist auf 1.600 Pixel begrenzt; fehlt ein
passender Generator, steht das Original als Fallback bereit. Schreib- und
Nachladeabrufe beim Ignorieren enden nach spätestens 20 Sekunden; unbestätigte
Antworten führen zu einer Leseprüfung statt automatischer Wiederholung.

Die Personenordneransicht liest 40 Pfade pro Seite und höchstens acht Vorschau-IDs
je Pfad über einen Index auf Person, Ordner und Status. Originalbilder werden für
Vorschauen nicht dekodiert. Sammelaktionen prüfen die Revision und alle betroffenen
Quelldateien und schreiben in einer Transaktion, unabhängig von der Vorschauzahl.
Foto-Schema 38 ergänzt exakte personenbezogene Ordnersperren. Datenbank-Trigger
verhindern verbotene Neuzuordnungen; Erkennung und Hintergrundabgleich filtern
solche Ziele bereits vor der Kandidatenauswahl. Sperren bleiben beim Zurücksetzen
von Gesichtsergebnissen erhalten und werden bei Personenzusammenführungen übernommen.


### Stammbäume

Foto-Schema 39 ergänzt eine revisionsgeschützte Auswahl der Ausgangspersonen.
`GET /settings/photos/family-tree?format=json` liefert Auswahl und Revision;
`POST /settings/photos/family-tree` ersetzt sie atomar mit `revision` und wiederholten
`person_id`-Formularfeldern. Beide erfordern `photos.manage`. Ohne `person_id`
wird die Ansicht deaktiviert. Zusammenführungen übertragen die Auswahl innerhalb
derselben Transaktion; konkurrierende Änderungen liefern 409.

`GET /photos/family-tree?format=json` benötigt `photos.read` und liefert die
zusammenhängenden Stammbäume als Personen und gerichtete Eltern- beziehungsweise
ungerichtete Geschwister-/Eheverbindungen. Ohne Auswahl liefert der Endpunkt 404.
Alle Ehe-Datensätze einschließlich Scheidungen bleiben erhalten. Die Graphsuche
liest einen konsistenten Datenbankstand und folgt beiden Richtungen über bestehende
Endpunktindizes in Paketen von höchstens 200 Personen. Sie verarbeitet keine Bilder
oder Gesichtsvektoren. Live-Verzeichnisschutz und Herkunft importierter Namen werden
geprüft; versteckte Personen können keine Brücke zwischen öffentlichen Personen bilden.

Die gemeinsame Grenze beträgt 10.000 geprüfte Personen und 50.000 Beziehungen;
bei Überschreitung folgt 422, bei mehr als 30 Sekunden 503. Es gibt keine stille
Kürzung. Antworten sind `private, no-store`. Die Navigation verwendet lediglich
eine indizierte Existenzprüfung der Auswahl. Der Browser zeichnet Verbindungslinien
auf einer Canvas in Sichtbereichsgröße und baut nur die sichtbaren Personen-Karten
auf. Suche und Stammdaten sind unabhängig vom Zoom erreichbar. Es werden keine
externen Grafikdienste oder JavaScript-Bibliotheken geladen.


Das Stammbaumlayout trennt Generationen von den kleineren Partner-/Elterneinheiten
für die horizontale Anordnung. Begrenzte Durchläufe richten Einheiten an den
wirklichen Eltern- und Kinderkoordinaten aus und lösen Kollisionen innerhalb einer
Zeile. Zeilen werden nicht unabhängig zentriert; damit bleiben auch unterschiedlich
breite Familienzweige ausgerichtet. Geburtsdatum, Name und zuletzt ID sorgen für
stabile Gleichstände. Der Generationenabstand beträgt 400 Pixel bei 144 Pixel
Kartenhöhe. Lange horizontale Verbindungen nutzen den Raum oberhalb der Karten.
Das Layout durchläuft Familien iterativ und benötigt keine rekursive Ahnensuche.

## Bildgruppen und API

Seit **1.9.0**, Foto-Schema **40**, speichert BearStack Bildgruppen ausschließlich
in `photo_image_groups` und `photo_image_group_members`. Mitglieder beziehen sich
auf beständige `photo_entities`-IDs. Extern verschobene Ordner behalten ihre
Gruppen nach erfolgreicher Identitätszuordnung; Originale und Sidecars werden
nicht geschrieben. Ein fehlendes Hauptbild erhält ein temporäres Ersatzhauptbild.

`media_index.image_group_hidden` blendet andere Mitglieder bereits vor
Sortierung und Pagination aus. Bestehende schnelle Zähler ziehen ausgeblendete
Mitglieder über einen Teilindex ab. Gruppenzuordnungen werden stapelweise geladen;
Kartenrouten-Revisionen berücksichtigen Hauptbildwechsel. Vollständige
Gruppenansichten sind auf 500 Mitglieder begrenzt. Auch das Erstellen und
nachträgliche Hinzufügen prüfen die Mitgliedschaften in begrenzten Stapeln
innerhalb derselben Schreibtransaktion, ohne Einzelabfrage pro Bild.

| Endpunkt | Verhalten |
| --- | --- |
| `POST /photos/image-groups` | `photos.edit`; Formular mit wiederholten `ids` und `primary`, 2–500 noch nicht gruppierte Bilder. Optionales `return` zur bisherigen `/photos`-Galerie einschließlich Filter und Sortierung. |
| `GET /photos/image-groups/{id}` | `photos.read`; Gruppenansicht, mit `format=json` oder `Accept: application/json` aktuelle Revision und sichtbare Mitglieder. |
| `POST /photos/image-groups/{id}` | `photos.edit`; `revision`, `action=primary/remove/dissolve/add`. Für `primary/remove` zusätzlich `entity_id`, für `add` wiederholte `ids` mit noch nicht gruppierten Bildpfaden. Zugriff auf alle bisherigen und neuen Mitglieder erforderlich. |

Schreibanfragen sind same-origin-geschützt. Mit JSON-Accept liefert Anlegen HTTP
201 mit `{id,url}`, Ändern HTTP 200 mit `{ok,exists,url}`; sonst erfolgt eine
303-Weiterleitung. Die JSON-Antwort beim Anlegen enthält weiterhin die Gruppen-URL;
das HTML-Formular führt ab 1.10.0 zur bisherigen Galerie zurück. Ohne gültiges
lokales `/photos`-Rücksprungziel wird der Ordner des Hauptbilds verwendet. Beim
Auflösen oder automatischen Auflösen nach `remove` führen HTML-Weiterleitung und
JSON-`url` in den Ordner des bisherigen Hauptbilds.

`action=add` ist ab 1.10.0 verfügbar, erhält das Hauptbild und erlaubt mindestens
ein neues Bild bis zu einer Gesamtgröße von 500 Mitgliedern. Bestehende Gruppen
lassen sich nicht zusammenführen. Veraltete Revisionen oder bereits gruppierte Bilder ergeben
409. Fehlerhafte Eingaben ergeben 400, fehlende Rechte 403 und nicht vorhandene
Bilder/Gruppen 404. Die [OpenAPI-Beschreibung](https://github.com/ringelbaer/BearStack-DMS/blob/main/openapi.yaml) enthält die Schemata.
Metadaten und native Katalogantworten ergänzen optional `image_group_id`.
Gruppenfilter gelten auch für native Galerie-, Karten- und Frame-Listen; die
Gruppenverwaltung erfolgt im Browser.
