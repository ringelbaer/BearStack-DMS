# Changelog

Alle wesentlichen Änderungen an BearStack werden in dieser Datei dokumentiert.

## Unveröffentlicht

### Dokumentation

- OpenAPI-Versionsangabe auf den bestehenden Release 0.24.2 korrigiert und den erforderlichen Abgleich mit `VERSION` dokumentiert.
- Schrittweise Einrichtung der Gesichtserkennung ohne Compose ergänzt: Python-Umgebung, Modellinstallation, Token, Dienstprüfung und native BearStack-Konfiguration.

## 0.25.3 - 2026-09-07

### Behoben

- Gültige Foto-Thumbnail-Cachetreffer benötigen keinen SQLite-Schreibaufruf mehr. Fehlende oder unvollständige Queue-Metadaten werden nur bei Bedarf repariert.
- Beim Herunterfahren werden Hintergrundworker sowie manuell gestartete Foto- und Gesichtsjobs abgebrochen und abgewartet. Neue Jobs werden abgewiesen; Datenbanken schließen erst nach Abschluss. Nach insgesamt 60 Sekunden wird ein nicht abgeschlossener Shutdown als Fehler beendet.
- `update.sh` prüft nach dem blockierenden Dienststopp dessen Ergebnis und Prozesszustand. Bei Timeout, Fehler oder verbleibender Hauptprozess-ID wird kein neues Binary installiert. Die systemd-Vorlage erlaubt 75 Sekunden für das Beenden.

### Geändert

- GPX-Verarbeitung ist abbrechbar und auf 16 MiB sowie 100.000 eingelesene Punkte pro Datei begrenzt. Parser laufen je Foto-Library einzeln; Kartenantworten enthalten höchstens 256 Tracks und 250.000 Punkte. Zu große Tracks werden übersprungen.
- Der GPX-Cache verwendet ein Speicherbudget von 32 MiB einschließlich Punktarray-Kapazität und Eintragskosten und verdrängt die am längsten ungenutzten Einträge.
- Unbenutzten Dateisystem-Helfer entfernt; reine Testzugriffe aus dem Produktionscode ausgelagert beziehungsweise auf produktive Einstiegspunkte umgestellt.
- Playwright-Suiten teilen sich Serverstart, Gesundheitsprüfung und vollständige Prozessbeendigung. Ein Testbinary wird pro Lauf einmal gebaut; die bestehende Testabhängigkeit ist über npm-Lockfile fest versioniert.

## 0.25.2 - 2026-09-07

### Behoben

- Vorübergehende Stat- und Blog-Lesefehler verwerfen den betroffenen Ordnerscan vor dem Schreiben. Bestehende Indexeinträge, manuelle Tags und Scan-Signaturen bleiben erhalten; ein späterer vollständiger Scan kann weiterhin echte Löschungen erkennen.
- Dokument-Bildthumbnails prüfen die Dimensionen vor dem Dekodieren und begrenzen die Eingabe auf 40 Megapixel. Ungültige oder zu große Bilder ersetzen keine vorhandene Vorschau.
- Indexbereinigung löscht zusammengehörige Metadaten, Tagzuordnungen, Such- und Vorschauindizes in einer Transaktion je Löschpaket beziehungsweise Ordnerteilbaum. Fehler und Abbrüche hinterlassen keine teilweise gelöschten Pakete.

### Geändert

- Foto-Info-Batchanfragen bündeln Metadaten- und Gesichtsabfragen, teilen sich Ordner-Rechteprüfungen und speichern geänderte Medien gemeinsam. Dateistand, XMP-Sidecars, manuelle Tags und aktuelle Zugriffsbeschränkungen bleiben berücksichtigt.

## 0.25.1 - 2026-09-07

### Behoben

- Foto-Tags, Tagzuordnungen und Volltextindex werden gemeinsam in einer Transaktion gespeichert, auch bei Umbenennungen und beim Entfernen von Tags. Schreibfehler rollen die gesamte Änderung zurück.
- Gleichzeitige Bulk-Tagaktionen lesen den aktuellen Stand innerhalb der Schreibtransaktion. Hinzufügen und Entfernen überschreiben keine parallelen Änderungen; fehlgeschlagene Aktionen hinterlassen keine teilweise bearbeitete Auswahl. Laufende Indexscans tragen inzwischen entfernte Tags nicht wieder ein.
- Endgültige Dateibereinigung nutzt ausschließlich die abbrechbare Sperre je Dokument. Eine laufende Vorschauerzeugung blockiert dadurch keine parallel angeforderte Löschung anderer Dokumente.
- Parallele Aufrufe der Dokumentstatistik teilen sich eine Berechnung. Eine laufende Berechnung kann eine zwischenzeitliche Cache-Invalidierung nicht mehr überschreiben.

### Geändert

- Tag-Umbenennungen und Tag-Löschungen lesen über vorhandene Tagindizes nur betroffene Medien, Ordner und Blogs statt sämtliche getaggten Datensätze.

## 0.25.0 - 2026-09-07

### Behoben

- Endgültiges Löschen legt persistente Dateibereinigungsaufträge in derselben Transaktion wie die Metadatenlöschung an. Quarantäne, reservierte Dateinamen und Wiederholung beim Start sowie minütlich machen die Bereinigung nach Fehlern und Neustarts fortsetzbar. Laufende Vorschauerzeugung und Bereinigung werden pro Dokument koordiniert.
- PDF-Thumbnails werden vollständig geprüft und atomar veröffentlicht; abgebrochene Renderer hinterlassen keine unvollständige veröffentlichte Vorschau.
- Fehlerhafte Dokumente blockieren spätere Batches des Thumbnail-Nachholprozesses nicht mehr.

### Geändert

- Automatische Migration der Dokumentdatenbank auf Schema 17 für persistente Löschaufträge.
- Foto-Cache-Statistiken werden beim Start und alle 30 Minuten im Hintergrund erhoben. Die Statistikseite zeigt den Messzeitpunkt und benötigt keinen vollständigen Dateisystemdurchlauf; parallele Berechnungen werden zusammengeführt.
- Ungenutzte Konto-Präferenz-Löschmethode und ausschließlich von Tests verwendeter einzelner Thumbnail-Helfer entfernt; der Regressionstest nutzt die produktive Batch-Funktion.
- Lokale Python-Umgebungen, Tool-Caches und Website-Verzeichnisse aus dem Docker-Buildkontext ausgeschlossen.

## 0.24.3 - 2026-09-06

### Behoben

- Die Einstellungen der Gesichtserkennung verwenden das gemeinsame Einstellungslayout mit seitlicher Navigation auf Desktop-Geräten und horizontalen Reitern auf kleinen Bildschirmen. Statuszahlen, Eingabefelder und Aktionen sind responsiv angeordnet; die Löschaktion ist räumlich getrennt.

## 0.24.2 - 2026-09-06

### Behoben

- Die Foto-Info erscheint auf kleinen Bildschirmen unter dem Foto über die volle Breite, statt es durch eine seitliche Spalte zusammenzudrücken. Das Panel bleibt separat scrollbar und sein Schließen-Knopf sichtbar.

## 0.24.1 - 2026-09-06

### Behoben

- Die Foto-Lightbox stoppt am ersten und letzten Medium, statt zum anderen Ende zu springen. Nicht verfügbare Navigationspfeile sind deaktiviert; die Diashow endet beim letzten Medium. Auch das Vorladen respektiert diese Grenzen.

## 0.24.0 - 2026-09-06

### Hinzugefügt

- Optionale lokale Gesichtserkennung mit CPU-Dienst, fest geprüften YuNet-/SFace-Modellen und gedrosselter, persistenter Hintergrundwarteschlange.
- Personenansicht mit Benennung, Zusammenführen, Verschieben und Ignorieren von Gesichtern für Foto-Verwalter; Personensuche und Fotoinfo berücksichtigen automatische Zuordnungen.
- XMP-Gesichtsregionen als Namensvorgaben, dauerhafte manuelle Korrekturen, Rechteprüfung für Gesichtsvorschauen und Ausschluss von `.adminonly`-Fotos.
- Einstellungen für Fortschritt, Pause, Fortsetzung, Fehlerwiederholung und bestätigtes Löschen erzeugter Gesichtsdaten; Compose-Profil `faces` und optionale Dienstkonfiguration.

### Geändert

- Foto-Datenbankschema 18 ergänzt automatische Gesichter, Personen und Aufträge ohne Änderungen an Originalbildern oder XMP-Sidecars.
- Personensuchen nutzen indizierte Zuordnungen; der inkrementelle HNSW-Index hält höchstens fünf Referenzen pro Person.

## 0.23.4 - 2026-09-06

### Behoben

- Beim Zurückkehren aus einem Foto-Unterordner wird die vorherige Scrollposition wiederhergestellt, sowohl über den Fotopfad als auch über Browser-Zurück/Vorwärts. Pfadlinks übernehmen die zuvor besuchte Sortierung, Filter und Seite; bei geänderter Fensterbreite bleibt der betretene Ordner an seiner bisherigen Bildschirmposition.

## 0.23.3 - 2026-09-06

### Geändert

- Foto-Ordnerthumbnails zeigen Bilder an den Positionen 20 %, 40 %, 60 % und 80 % in absteigender Datumsreihenfolge. Kleine Ordner zeigen jedes Bild höchstens einmal; Ordner ohne Bilder behalten Medienvorschauen.
- Bestehende Ordner-Vorschauzuordnungen werden automatisch ersetzt. Die Auswahl berücksichtigt die sichtbaren Bilder einschließlich Unterordnern und wird für Ansichten mit und ohne Admin-only-Inhalte getrennt zwischengespeichert.

## 0.23.2 - 2026-08-24

### Behoben

- Die Foto-Kartenansicht nutzt auf hohen Browserfenstern den verfügbaren Bereich bis zum Footer aus, statt ihre Höhe bei 640 px zu begrenzen.

## 0.23.1 - 2026-08-24

### Behoben

- Die Info-Seitenleiste der Foto-Vollansicht zeigt neben dem Aufnahmedatum wieder die Aufnahmezeit an.

## 0.23.0 - 2026-08-09

### Hinzugefügt

- Optionaler, selbst gehosteter BearStack-PDF-Viewer mit Seitennavigation, Zoom, Einpassen, Text- und Link-Layer.
- Geräteübergreifende PDF-Vorschaupräferenz für SQLite- und Konfigurationskonten im Selbstservice und in der Nutzerverwaltung.

### Geändert

- PDF.js 6.2.108 wird lokal und verzögert geladen; bei Darstellungsfehlern bleibt der native Browser-Viewer als automatischer Rückfall erhalten.
- Datenbankschema 16 speichert versionierte Konto-Präferenzen unabhängig von Zugangsdaten und Sitzungen.

## 0.22.0 - 2026-08-09

### Hinzugefügt

- Produktionsreife Benutzerverwaltung für SQLite-Konten in der Weboberfläche.
- Rollen, zusätzliche Einzelrechte, Aktivierung, Passwort-Reset und Passwort-Selbstservice.
- Dediziertes Recht `system.users.manage` für delegierbare Benutzerverwaltung.
- Versionierte Sitzungen und begrenzter Schutz vor wiederholten Anmeldeversuchen.

### Geändert

- JSON- und Env-Konten bleiben als schreibgeschützte Konfigurationskonten parallel zu UI-Konten nutzbar.
- Sitzungen werden bei sicherheitsrelevanten Kontoänderungen sofort ungültig; bestehende Cookies im alten Format erfordern nach dem Upgrade eine erneute Anmeldung.
