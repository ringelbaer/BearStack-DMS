---
title: Architektur
description: Wie BearStack Daten, Suche, Medien und Konfiguration strukturiert.
icon: lucide/blocks
---

# Architektur

BearStack hält die Architektur bewusst direkt: eine Go-Webanwendung, SQLite für strukturierte Daten und das Dateisystem für Dokumente und Medien.

## Lokale Daten

| Bereich | Standardpfad |
| --- | --- |
| Basisdaten | `data` |
| Dokumente | `data/documents` |
| Dokumentdatenbank | `data/bearstack.db` |
| Fotos | `data/photos` |
| Fotodaten | `data/photos-data` |
| Thumbnail-Cache | `data/photos-data/thumbnails` |

## Konfiguration

Die Konfiguration wird aus Defaults, JSON-Datei, Env-Dateien und Prozessumgebung zusammengeführt. Die effektive Priorität ist:

```text
Defaults < JSON < Env-Dateien < bereits gesetzte Prozess-Env
```

Die JSON-Datei wird über `BEARSTACK_CONFIG` angegeben. Sie ist besonders nützlich für feste Pfade, TLS, Fotokonfiguration und mehrere Auth-Zugänge. Unbekannte JSON-Felder werden ignoriert; dadurch startet BearStack auch dann weiter, wenn eine Installation zusätzliche oder künftige Einstellungen enthält.

Beim Start werden zuerst `.env` und danach `BEARSTACK_ENV_FILE` in die Prozessumgebung geladen, ohne bereits gesetzte Variablen zu überschreiben. Danach wird die JSON-Datei geladen und zum Schluss überschreiben Umgebungsvariablen die JSON-Werte. Innerhalb der Env-Dateien gewinnt `.env` vor `BEARSTACK_ENV_FILE`, wenn beide denselben Key setzen. Env-Dateien unterstützen einfache `KEY=value`-Zeilen, optional mit `export`, einfachen oder doppelten Quotes sowie Kommentare nach einem Leerzeichen.

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
    "data_dir": "/var/lib/bearstack/photos-data",
    "cache_dir": "/var/lib/bearstack/photos-data/thumbnails",
    "db_path": "/var/lib/bearstack/photos-data/photos.db",
    "page_size": 120
  },
  "webdav": {
    "path": "/webdav"
  },
  "auth": {
    "realm": "BearStack",
    "credentials": [
      {
        "username": "admin",
        "password_hash": "$2a$10$...",
        "role": "admin",
        "permissions": []
      }
    ]
  }
}
```

Die vollständige Liste der Laufzeit- und Docker-Umgebungsvariablen steht im Abschnitt [Docker und Compose](installation.md#docker-und-compose). So bleibt die variable Referenz an einer Stelle gepflegt; diese Architektur-Seite konzentriert sich auf Struktur, Priorität und JSON-Form.

Zusätzlich zur Startkonfiguration speichert BearStack über die Weboberfläche angelegte Konten in der Hauptdatenbank. Beim Start werden beide Quellen validiert und zu einem unveränderlichen Auth-Snapshot zusammengeführt. Authentifizierung und Session-Prüfung lesen diesen Snapshot ohne Datenbankzugriff; Kontoänderungen ersetzen ihn nach erfolgreicher SQLite-Transaktion atomar. Konfigurationskonten bleiben schreibgeschützt, und doppelte Benutzernamen über beide Quellen werden abgelehnt.

## Abfragen und Darstellung

Ab 0.41.3 trennt `internal/server/navigation.go` erreichbare Startseiten und URLs vom Einstellungsservice. Die reine Startseitenauflösung erhält Berechtigungen und Modulstatus explizit; `settings_presenter.go` enthält die beschrifteten Auswahloptionen. Speicherung, Normalisierung und synchronisierte Caches bleiben im Einstellungsservice.

Die fachlichen Regeln zur Rechteweitergabe und Benutzerverwaltung liegen in `internal/account/management.go`. Der Server übergibt Identität, Rolle und effektive Fähigkeiten über einen kleinen Principal-Adapter. Handler und HTML-Darstellung verwenden dieselben Regeln; Routenberechtigungen, Session-Prüfung, Passwortverarbeitung und synchronisierte Schreibabläufe bleiben Aufgaben des Servers.

Ab 0.41.4 registriert die Foto-Routentabelle die acht Labeling-Endpunkte direkt auf einzelne Handler. Die Bild-Endpunkte teilen die Prüfung des aktuellen Gesichts; JSON-Validierung, Größenlimits, Fehlercodes und Cache-Header bleiben unverändert. Ein zweiter Router innerhalb des Handlers ist nicht mehr nötig.

`internal/server/services.go` bündelt den Aufbau von Dokumentimport, Nachverarbeitung, OCR, Thumbnails, Office-Vorschauen, Mailimport und Papierkorb. `Server.New` initialisiert diese Services vor der Freigabe des Servers. Teilweise aufgebaute Test-Server verwenden beim ersten Zugriff denselben durch `sync.Once` geschützten Aufbau. Vorgegebene Services werden vor dem ersten Zugriff eingesetzt und bleiben erhalten. Konstruktion erzeugt nur Instanzen und Kanäle; Start und Shutdown der Worker bleiben im vorhandenen Hintergrund-Lifecycle.

Dokumentlisten für HTML und JSON nutzen denselben Abfrage-Service. Er erhält Filter und Optionen für OCR-Daten beziehungsweise das Überspringen von Seiten außerhalb des gültigen Bereichs und liefert Dokumente, Gesamtzahl und optional OCR-Jobs. HTTP-Anfragen, Redirects und Navigationslinks bleiben in der Darstellungsschicht. HTML-Seiten werden wie bisher auf die letzte vorhandene Seite umgeleitet; die API darf eine leere Seite zurückgeben. Vor einer HTML-Umleitung wird nur gezählt, und API-Abfragen benötigen keine OCR-Abfrage.

Fotoeinstellungen werden an ihrer Quelle eingelesen und anschließend gemeinsam normalisiert. Zahlenbereiche stehen dadurch an einer Stelle. Fehlende oder ungültige Datenbankwerte behalten ihre Defaults; Formular-Checkboxen sind weiterhin nur mit dem Wert `1` aktiviert. Allgemeine, Dokument- und Fotoeinstellungen werden je Formular in einer SQLite-Transaktion gespeichert. Zusammengehörige Werte werden mit einer gemeinsamen Abfrage gelesen. Cache-Ladevorgänge und Schreibvorgänge sind synchronisiert; nur erfolgreiche Commits aktualisieren Caches und die Foto-Worker-Einstellung.

Mailimport und EML-Archivierung teilen MIME-Helfer für Transfer-Encoding, Content-Type, Anhangsnamen und Zeichensätze. Welche Anhänge verarbeitet werden, ihre Größenlimits und die Archivdarstellung bleiben Aufgaben der jeweiligen Module.

Das Browsermodul `app-photos-media.js` stellt die gemeinsam verwendeten Medien-Helfer bereit, darunter die Übernahme von Fotometadaten und die Auswahl einer passenden Bildauflösung. Galerie und Fotoframe verwenden diese Helfer; der Fotoframe kann unabhängig vom Galerie-Script arbeiten. Das gemeinsame Modul wird vor seinen Verbrauchern geladen.

Die Lightbox mit Zoom, Touch-Gesten, Vollbild, Diashow und Video-/Audiowiedergabe liegt in `app-photos-lightbox.js`. Die Galerie startet sie über `BearStack.photos.lightbox.init` und übergibt Bearbeitungsmodus, gebündeltes Metadaten-Nachladen und Kartenhelfer explizit. Der Dialogzustand bleibt innerhalb des Lightbox-Moduls; Galerie-Layout, Auswahl und Scroll-Wiederherstellung bleiben im Galerie-Modul. Die Asset-Liste lädt das neue Modul vor der Galerie. Ein Browser-Test prüft die Lightbox ohne Galerie-Script mit separat übergebenen Funktionen.

## Performance

BearStack trennt Dokumente und Fotodaten, nutzt Caches für aufwendige Medienarbeit und führt OCR sowie Vorschau-Erzeugung im Hintergrund aus. Das ist besonders wichtig, wenn Archive über die Zeit wachsen oder viele Bilder in einem bestehenden Fotoverzeichnis liegen.

Ab 0.42.0 verwendet WebDAV `ListDocumentFiles` für schlanke Dateimetadaten ohne nachgeladene Tags, eigene Felder, Duplikat- und Verknüpfungszähler. Filter und Sortierung stammen aus demselben Abfrageaufbau wie normale Dokumentlisten; die bestehende Namensvergabe und HTTP-Metadaten bleiben erhalten. Beim Auflösen von Zwischenordnern werden ausschließlich Ordnerkandidaten geladen.

Ab 0.44.0 filtert `unknown=1` die Personenübersicht auf unbenannte Gruppen mit aktiven Gesichtern. Zählung und Seitenabfrage verwenden denselben SQL-Filter; `name_fold=''` nutzt den bestehenden Namensindex. Ignorierte Gesichter tragen weder zum Vorschaubild noch zur Fotoanzahl bei. Der Sichtbarkeitsabgleich prüft unbekannte Gruppen sowie importierte Namen einschließlich ihrer Gesichts- und Quellverzeichnisse: Wird ein importierter Name durch eine neue Schutzmarkierung entfernt, kann die Gruppe sicher als unbenannt erscheinen. Unbeteiligte manuell benannte Gruppen werden dabei nicht geprüft. Es sind keine zusätzlichen Bildabrufe oder Schemaänderungen erforderlich.

Die Web-Autovervollständigung fragt `GET /photos/people?format=suggestions&q=...` ab. Die kompatible zusätzliche JSON-Darstellung liefert bis zu 60 benannte Gruppen mit `id`, `name`, `count` (verschiedene Fotos) sowie `has_next`. Eine zusätzliche Ergebniszeile ersetzt die Gesamtzählung. Ab 0.43.0 ergänzt `face_id` dasselbe aktive Portrait wie die Personenübersicht. Der bestehende Index auf Person, Ignorierstatus, Pfad und ID begrenzt die zusätzliche Thumbnail-Auswahl auf die höchstens 61 Abfragekandidaten. Es gibt keine separaten Detailabfragen pro Person. Das Modal lädt die kleinen Bilder bedarfsabhängig über den vorhandenen Thumbnail-Endpunkt und dessen Cache; dieser prüft die Sichtbarkeit auch beim Bildabruf erneut. Der Modus berücksichtigt `q`; `page`, `known` und `ignored` sind für Vorschläge ohne Wirkung. Rechte und aktuelle Sichtbarkeitsprüfungen entsprechen der bekannten Personenliste. Die vollständige Übersicht mit `format=json` bleibt unverändert. Wegen dieser zusätzlichen API-Darstellung trägt das Release die MINOR-Version 0.42.0; eine Datenmigration ist nicht erforderlich.

Der Startabgleich der Foto-Sichtbarkeit teilt Pfad- und Symlinkprüfungen gemeinsamer Vorfahren über einen lokalen `RootPathBatch`. Er verwendet dieselbe Pfadvalidierung und Fehlerbehandlung wie einzelne Aufrufe und existiert nur während des synchronen Startdurchlaufs. Spätere Zugriffe und neue Durchläufe prüfen das Dateisystem erneut.

Statische Dateien unter mitgelieferten `vendor/pdfjs-X.Y.Z/`-Verzeichnissen erhalten `Cache-Control: public, max-age=31536000, immutable`, auch für relative Worker-, Schrift- und Codec-URLs ohne `?v=`. Eine Änderung dieser Dateien muss einen neuen Versionspfad erhalten. Unversionierte Assets behalten fünf Minuten Cache-Laufzeit; Inhaltsversionen aus Templates behalten ihre bisherigen langfristigen Cache-Header.

Personendetails, gefilterte Personenlisten und Bearbeitungsaktionen prüfen die Verzeichnisse der betroffenen Gruppen einschließlich der Herkunft importierter Namen. Reine Personenansichten beziehen keine unbeteiligten Auftragsverzeichnisse ein. Gesamtübersichten mit exakten Zählwerten prüfen weiterhin alle relevanten Gesichtsverzeichnisse. Gemeinsame Vorfahren werden pro Prüfung einmal geprüft; wartende gleichartige Anfragen können eine anschließend gestartete Prüfung teilen. Fertige Prüfergebnisse werden nicht zwischengespeichert, damit neue `.adminonly`-Markierungen beim nächsten Zugriff berücksichtigt werden. Fehlende Verzeichnisse, Symlinks und Zugriffsfehler blockieren die betroffene Abfrage. Ein vorübergehend nicht erreichbares Fotoverzeichnis löscht dabei keine gespeicherten Gesichtsdaten.

Der Gesichtsabgleich verteilt die Referenzen einer Person möglichst über verschiedene Galerieordner. Alle aktiven Favoriten sind verbindlich, auch oberhalb der Zielanzahl; freie Plätze werden ordnerweise ergänzt. SQLite ordnet ausschließlich Metadaten mithilfe eines Indexes für Person, Modell, Ordner und Priorität. Schema 22 ergänzt den Favoritenstatus kompatibel mit Standardwert `false` und plant die fortsetzbare Neuauswahl bestehender Referenzen ohne erneute Inferenz.

Seit 0.48.0 ersetzt ein inkrementeller Vektorcache den approximativen HNSW-Index. Alle Referenzen werden mit exaktem Skalarprodukt bewertet und pro Person zusammengefasst. Nur Personen, die noch in die besten Ergebnisse gelangen können, werden in SQL-Paketen mit höchstens 512 Referenz-IDs gegen aktuelle Zuordnung, Qualitätsfreigabe und Sichtbarkeit geprüft. Ungültige Treffer werden durch nachfolgende Personen ersetzt. Favoriten bleiben vollständig berücksichtigt. Die Laufzeit der Vektorbewertung wächst linear mit der Referenzzahl; die Datenbankabfragen betreffen überwiegend die besten Gruppen.

Foto-Schema 25 ergänzt nullable Qualitätswerte, eine fortsetzbare Abgleichwarteschlange und gespeicherte Zusammenführungsvorschläge. Der separate Worker nutzt vorhandene Vektoren und speichert Cursor und Ergebnisse atomar in begrenzten Paketen. Benutzerkorrekturen merken einen Folgelauf vor, ohne einen laufenden Cursor zurückzusetzen. Automatische Verschiebungen sind auf unbestätigte unbenannte Gesichter zu bestätigten benannten Gruppen beschränkt; Konflikte im selben Foto, ignorierte Gesichter und dauerhafte Ablehnungen werden berücksichtigt. Zusammenführungen nach Bestätigung prüfen beide Gruppenrevisionen innerhalb der Schreibtransaktion.

Kleine Gesichter können über bis zu acht Originalausschnitte pro Foto verbessert werden. Ein gemeinsames Originaldekodieren, eine begrenzte Ausschnittgröße und ein Gesamtbudget von 16 MiB begrenzen Speicher und Anfragen. EXIF-Ausrichtung wird beim Ausschnitt angewandt; Quell- und Schutzprüfungen laufen vor und nach den Anfragen. Qualitätsmetadaten sind additive Felder des bestehenden Inferenzprotokolls; Modellkennung und Vektordimension bleiben gleich.

Favoritenänderung, Neuauswahl für die betroffene Person und Referenzrevision werden atomar gespeichert. Ein aktueller Suchindex wird nur für diese Person inkrementell angepasst; ein veralteter Index wird vor dem nächsten Abgleich neu aufgebaut. Fehler bei der Cache-Aktualisierung ändern den bereits gespeicherten Favoritenstatus nicht. Die explizite gewünschte Markierung ist idempotent; die erwartete Personen-ID schützt vor veralteten Gruppenzuordnungen. Die additive Labeling-API bleibt auf Protokoll 1 und meldet `face_favorites` in der Sitzung. Ab App 0.6.0 nutzt der Personenbereich die Favoritenfunktion. BearStack 0.43.0 meldet zusätzlich `named_people` und bietet eine seitenweise Liste sowie quittierte Aktionen `rename`, `unassign` und `favorite` für benannte Personen. Foto-Schema 24 ergänzt dafür einen partiellen Personen-ID-Index, der unbenannte Gruppen beim Blättern auslässt. Diese Aktionen verwenden denselben Transaktions- und Revisionsschutz wie das Benennen; die neuen Verwaltungsaktionen aktualisieren einen aktuellen Suchindex für die betroffenen Personen inkrementell.

Ab 0.41.4 verwenden Favoriten- und Gruppenbildaktionen gemeinsame Helfer aus `faces_mutation.go`: `refreshFaceMutationTx` aktualisiert betroffene Referenzen und Revision innerhalb der bestehenden Transaktion. Erst nach erfolgreichem Commit synchronisiert `syncFaceMutation` den Suchcache unter dem vorhandenen Runtime-Lock. Eine veraltete Cache-Revision oder ein Fehler beim Nachladen verwirft den Cache; die gespeicherte Aktion bleibt erfolgreich. Es entstehen keine zusätzlichen Dateisystemprüfungen oder Datenbankabfragen.

## Performance-Benchmark

Die Go-Benchmarks decken zwei zentrale Lastprofile ab: Dokumentlisten mit Suche, Tag-Filtern und Pagination sowie Fotolisten mit großen Indexen, Thumbnail-Status, GPX-Daten und Index-Neuaufbau. Die Dokument-Benchmarks arbeiten mit 1.000, 10.000 und 50.000 Dokumenten. Die Foto-Benchmarks simulieren unter anderem 300.000 Medien in bis zu 5.000 Galerien, geänderte Ordner, viele Tags und Blog-Dateien im Fotoverzeichnis.

```sh
go test ./internal/repository -bench=BenchmarkList -benchmem
go test ./internal/photos -bench=BenchmarkPhoto -benchmem
go test ./internal/photos -bench=BenchmarkMillionPhotoRebuildIndexScenarios -benchmem
go test ./internal/photos -run '^$' -bench '^BenchmarkFaceVisibility$' -benchmem
```

Für vergleichbare Messungen sollten Benchmarks auf einem ruhigen System laufen, idealerweise mit derselben Go-Version, demselben Datenträger und mehreren Wiederholungen. Die Zahlen hängen stark von CPU, Speicher, Dateisystem und SQLite-I/O ab; die Benchmarks sind deshalb vor allem als Regressionsschutz und Größenordnung für eigene Installationen gedacht.

## Versionierung

Die Anwendungs-Version steht zentral in `VERSION` und wird in der Weboberfläche dezent im Footer angezeigt. BearStack nutzt semantische Versionierung:

- `PATCH`: Bugfixes, Security-Fixes, Performance, Refactors ohne neues Verhalten und kleine UI-Korrekturen.
- `MINOR`: neue rückwärtskompatible Funktionen, neue optionale Einstellungen, nicht-brechende API- oder UI-Fähigkeiten und automatische kompatible Migrationen.
- `MAJOR`: brechende Änderungen an HTTP/API/WebDAV, Konfiguration, Datenformaten, Berechtigungen oder manuelle inkompatible Migrationen.

Bei mehreren Änderungstypen gewinnt die höchste Kategorie. Docs-only- und Test-only-Änderungen erhöhen die Version nicht. Solange BearStack in `0.x` ist, führt der erste echte Major Change auf `1.0.0`.

Für diese Website wurde keine BearStack-Version erhöht, weil sie Dokumentations- und Website-Quellen ergänzt und keine Laufzeitfunktion der Anwendung ändert.

### Gruppenbilder-Bearbeitung

Ab 0.41.0 verwendet die Gruppenbilder-Ansicht einen Pfad-Cursor und den partiellen
Index `idx_face_group_candidates(path, person_id) WHERE ignored=0` (Foto-Schema 23).
Die Datenbank zählt unbenannte Gesichter pro Foto; bei importierten Namen wird deren
aktuelle Sichtbarkeit vor der Ausgabe geprüft. Nur Kandidaten und Namensquellen
benötigen Dateisystemprüfungen, Embedding-Blobs werden bei der Auswahl nicht gelesen.
Die Detailabfrage ist auf die maximale Detektionsanzahl von 256 Gesichtern begrenzt.

Ein Hash der angezeigten Gesichter sichert die Sammelaktion ab. Nach Reservierung
des SQLite-Schreibzugriffs vergleicht der Server den aktuellen Stand und ignoriert
nur unbenannte, aktive Gesichter dieses Fotos. Änderungen durch Benennen, Verschieben
oder neue Analyse verhindern die Mutation mit `409`; Teiländerungen werden zurückgerollt.
Referenzauswahl und Revision werden in derselben Transaktion aktualisiert. Ein aktueller
Suchindex wird anschließend nur für betroffene Personen angepasst. Ein fehlgeschlagener
Bildwechsel wird unabhängig von der bereits gespeicherten Mutation wiederholt.

Die bestehende Modal-Vorlage und Personensuche werden gemeinsam genutzt. Eine
Bearbeitung im Modal wirkt wie in der Personenübersicht auf die gesamte Gruppe.
Die Fotovorschau nutzt den Galerie-Cache mit 1.600 Pixeln und prüft die Quell-Gesichts-ID
vor der Ausgabe. Die Markierung wird aus normalisierten Koordinaten innerhalb des
proportional eingepassten Bildes berechnet und bei Größenänderungen aktualisiert.

Ab 0.41.1 liegt die gemeinsame Bildaufbereitung im begrenzten `faceImageCache`.
Der Schlüssel umfasst Quellpfad, Größe, Änderungszeit und XMP-Fingerprint. Gleichzeitige
Decodierungen derselben Quelle werden zusammengefasst; ein abgebrochener Aufrufer
verhindert keinen späteren Versuch eines anderen Aufrufers. Gespeichert werden nur
unveränderlich verwendete, ausgerichtete NRGBA-Raster bis 1.600 Pixeln; die bisherigen
JPEG-Ausschnittscaches bleiben bestehen. Beim Schließen der Library wird der
Arbeitsspeichercache geleert.

`FaceProgress` liest ausschließlich Datenbankzähler und bündelt Aufrufe für fünf
Sekunden. `FaceStatus` bleibt für die vollständige, frisch geprüfte Ansicht zuständig.
Die strikte Quellprüfung `refreshFaceSource` wird von einzelnen Gesichtern und
Gruppenbildern gemeinsam verwendet und gibt Fehler bei Fingerprint-Schreibvorgängen weiter.

Das Webmodul `app-person-dialog.js` kapselt Personensuche, Formularübermittlung und
Fokusführung. Die aufrufende Ansicht stellt ihren Ladezustand und eine asynchrone
Aktualisierungsfunktion bereit. Dadurch hängt der Dialog weder von der Auswahlleiste
der Personenübersicht noch von der Navigation durch Gruppenbilder ab.
