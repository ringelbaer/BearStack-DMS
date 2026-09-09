# BearStack Fotos – Umsetzung und Abnahme

Stand: 9. September 2026. BearStack **0.50.0**, Android **0.10.0 (22)**,
zusammengehöriges unveröffentlichtes MINOR-Feature. App-ID und bestehende Konten,
Personengruppen, Warteschlangen und gespeicherte Aktionsquittungen bleiben erhalten.

## Funktionsabnahme

- [x] Lesende Galerie-API, Rechte-/Pfadprüfung und OpenAPI-Vertrag
- [x] Galerie als Einstieg, Ordnernavigation, zwei Vorschauen und Beschriftung
- [x] Datumsgruppen, Suche und endloses Nachladen in beide Richtungen
- [x] Vollbild, Wischen, Zoom, Foto-Infos und gestreamter Original-Download
- [x] Einstellbare Diashow und Fotoframe einschließlich Video/Audio und Lebenszyklus
- [x] Native Blog-, Markdown- und Textansicht mit gemeinsamer sicherer Aufbereitung
- [x] Karten pro Foto und Ordner/Suche, GPX-Ebenen und Fotoroute
- [x] Serverseitiger JSON-Cache vollständiger gruppierter Fotorouten
- [x] Bestehende Personenfunktionen einschließlich ähnlicher Gruppen und Vergrößerung
- [x] Deutsch/Englisch, Hilfen, strukturierte Fehler und zugängliche Aktionen
- [x] Gemeinsamer Hell-/Dunkelmodus und abgerundete Galerienavigation
- [x] Runder Android-Icon-Beschnitt vollständig sichtbar
- [x] Begrenzte Metadaten-/Bildspeicher, Vorladen und Konto-/Cache-Isolation
- [x] Skalierungsprüfung mit 300.000 Fotos, 5.000 Ordnern und tiefen Medienabfragen
- [x] Hochformat, Querformat und doppelte Schriftgröße mit echten Bilddateien geprüft
- [x] Abschließender vollständiger Geräte-/HTTPS-Lauf nach der Gestaltung
- [x] Abschließende Website-/Dokumentationsprüfung

## Architektur und Grenzen

Die App nutzt `/api/photos/v1/` für die lesende Galerie und weiterhin
`/api/photos/labeling/v1/` für Personenaktionen. Gemeinsame Dienste, HTTPS-Prüfung,
Bildlader, kontoabhängige Cache-Schlüssel und der native Vollbildbetrachter werden
wiederverwendet. Lesekonten erreichen keine Personenbearbeitung. Alte Server behalten
den bisherigen Personenbereich als Einstieg.

**Endlos scrollen:** Fotos, Ordner, Texte und Karten-Fotoauswahl verwenden dasselbe
fortlaufende Raster ohne Seitenschalter oder Ladebuttons. Vorladen beginnt mehrere
Reihen vor dem sichtbaren Ende. Intern bleiben höchstens drei Datenpakete pro Bereich:
288 Medien, 72 Ordner, 60 Textzusammenfassungen. Sichtbare Anker und Wischimpulse
bleiben erhalten; veraltete Antworten dürfen keine sichtbaren Einträge entfernen.
Auch die Ordnersuche liefert über 50 Treffer hinaus die vollständige sichtbare
Trefferliste mit korrekter Gesamtzahl und Sortierung.
Vollbild und Fotoframe laden Nachbarn bedarfsgerecht; die Rückkehr erhält die Position.

**Informationen und Wiedergabe:** Datum, Sekunden und gespeicherter UTC-Offset,
Datei/Pfad, Abmessungen/Größe, Kamera/Objektiv, GPS, Bewertung, Personen und Tags.
Ohne Aufnahmezeit wird die Änderungszeit gekennzeichnet. Informationen, Einstellungen
und Hintergrundwechsel pausieren auch manuell gestartete Medien. Einzelvideo-
Wiederholung, Abschalten der Wiederholung und Rückkehr zur Galerie sind mit echtem
Decoder über den isolierten HTTPS-Testserver geprüft.

**Karten:** Öffentlicher OSM-Client ohne BearStack-Zugangsdaten oder Cookies,
sichtbare Attribution, nur sichtbare Kacheln, 64-MiB-HTTP-Cache und Beachtung der
Cache-Header. Kartenmarker fassen den vollständigen indexierten GPS-Bestand in
höchstens 289 Marker zusammen. Neu private GPS-Ordner werden vor jedem Abruf in
Paketen von 256 Verzeichnissen geprüft, auch vor dem nächsten regulären Scan.
GPX und Fotorouten teilen serverseitig zwei Geometrie-Arbeitsplätze. GPX: maximal
16 MiB/100.000 Quellpunkte pro Datei, 32-MiB-Parsercache; in der App insgesamt 8.192
GPX-Koordinaten und zusätzlich 4.096 Fotorouten-Koordinaten. Abschnitte und Datumsgrenzen
bleiben erhalten. Der GPX-Inventarindex wird bei Schema 26 kompatibel ergänzt.

**Fotorouten-Cache:** Die gesamte Gruppierung geschieht auf dem Server. JSON-Dateien
unter `<Cache-Verzeichnis>/photo-routes/v1/` enthalten alle gruppierten Orte mit
Koordinaten, Anfangs-/Endzeit und Fotoanzahl. Erst beim Abruf greifen Ausschnitt und
Punktlimit. Schlüssel unterscheiden Bibliothek, rekursiven Ordner, Radius, Typ und
Sichtbarkeit; Suchabfragen werden nicht dauerhaft gespeichert. Format und Algorithmus
haben eigene Versionen. Eine globale transaktionale Indexrevision invalidiert auch
Änderungen in Unterordnern, unabhängig von Ordnerzeiten. GPS, Aufnahme-/Änderungszeit,
Ordnerzuordnung, Typ, Sichtbarkeit, Einfügen und Löschen werden berücksichtigt.
Unveränderte Updates, Tags und Bewertungen invalidieren die Route nicht.
Browser und native API nutzen denselben Cache-Abruf. Die Browserroute ist unabhängig
von der Medienseite vollständig; nur die Ausgabe wird bei mehr als 8.192 gruppierten
Orten über die gesamte Route vereinfacht und mit der Gesamtpunktzahl gekennzeichnet.
Der gemeinsame Karten-/GPX-Reducer bewahrt Zeiten und Fotoanzahlen der verbleibenden
Orte. Der Arbeitsbereich bleibt auf höchstens 16.385 Punkte begrenzt. Vor dem ersten
Indexlauf bleibt die bisherige direkte Dateisystem-Berechnung verfügbar.

Parallele gleiche Berechnungen werden zusammengefasst. Abbruch des letzten wartenden
Abrufs beendet die Berechnung; beim Schließen der Bibliothek werden die Jobs beendet.
Vollständige Dateien ersetzen alte Revisionen atomar mit Modus 0600. Während der
Berechnung geänderte Revisionen werden verworfen. Defekte Dateien werden erneuert;
bei Schreibfehlern oder einer Route über dem Cachebudget bleibt die vollständige
ungecachte Berechnung verfügbar. Maximal 256 JSON-Dateien/512 MiB; Revisionen erzeugen
keine zusätzlichen Dateinamen. Schreiben und Lesen streamen die Route ohne eine
vollständige Punktliste im Arbeitsspeicher. Auch beschädigte JSON-Werte sind beim
Lesen begrenzt. HTTP-Antworten bleiben `private, no-store`.

## Nachweise

- Nach beiden Korrekturen erneut erfolgreich: vollständiges `go test ./...`,
  gezielte Race-Tests für Cache, Browserroute und Ordnersuche sowie vier
  Playwright-Prüfungen für Desktop-/Mobilfilter und die Browserkarte.

- Nachprüfung der beiden Einschränkungen: API-Test mit 60 öffentlichen und einem
  privaten Ordner, drei Suchformen und beiden Namenssortierungen; alle 60 Treffer
  über vier Abrufe erreichbar. Browser und native Route funktionieren nach Entzug
  des GPS-Quellindexes weiterhin mit derselben Cache-Datei, auch im HTTP-Test.
  Eine Browserroute mit 20.000 getrennten Orten bleibt vollständig im Cache;
  Anzeige und Arbeitsbereich sind begrenzt, Anfang/Ende und Metadaten bleiben erhalten.
- Breite Ordnersuche mit 10.000 indexierten Ordnern, drei Wiederholungen: erste
  Seite ca. 57 ms, Seite 417 ca. 76 ms, jeweils ca. 22 MB Gesamtallokationen.
  Vollständige Metadatensortierung bleibt serverseitig bestehen; nur Vorschauen
  und API-Antwort sind auf das angefragte Paket begrenzt.

- Alle Go-Pakete nach dem Routencache erfolgreich: `go test ./...`.
- Cache-/Revisionsregressionen zusätzlich mit `go test -race` erfolgreich.
  Geprüft: vollständige Speicherung trotz kleinem Antwortlimit, verschiedene
  Ausschnitte, Neustart, Versionen, Rechte, private Unterordner, Metadatenänderungen
  ohne Ordnerzeitänderung, Transaktionsrollback, defekte Dateien, Schreibfehler,
  parallele Abrufe, Abbruch, Änderung während der Berechnung und Cacheverdrängung.
- Ein Cache-Test entfernt nach dem ersten Abruf den GPS-Abfrageindex. Weitere
  Abrufe funktionieren aus derselben JSON-Datei; die Quellabfrage läuft nicht erneut.
- Lokaler Benchmark, 100.000 Fotoorte mit wiederkehrenden Aufenthaltsorten,
  drei Wiederholungen: Cache ca. 10,7 ms, Neuberechnung ca. 129 ms. Gesamtallokationen
  ca. 91 kB gegenüber 9,2 MB; diese Werte sind keine universelle Laufzeitgarantie.
- Galerie-Benchmark mit 300.000 Fotos und 5.000 Ordnern: tiefe Medienabfrage bei
  Position 287.904 ca. 68 ms, 24 Ordner mit je zwei Vorschauen ca. 35 ms.
  Komplexe zu breite Suchausdrücke liefern den dokumentierten Fehler. Ordner werden
  serverseitig weiterhin vollständig sortiert; nur die ausgewählten Vorschauen
  werden abgefragt. Dies ist für die gemessene Sammlung geprüft, kein harter
  konstanter Speicheranspruch für beliebig große serverseitige Ordnerlisten.
- Abschließend **53 JVM-Tests und 92 Geräte-/HTTPS-Tests erfolgreich**, jeweils
  keine Fehler oder übersprungenen Tests. Enthalten sind vier Layouttests:
  Deutsch/hell/Hochformat, Englisch/dunkel/Querformat, doppelte Schrift und die
  Prüfung aller gezeichneten Icon-Pixel gegen die runde Maske.
- Android-Build und Lint erfolgreich. Website neu erzeugt, OpenAPI-YAML geprüft,
  `git diff --check` ohne Befund. README, Changelog und Anleitungen sind aktuell.
- Screenshots von Raster, Ordnern, Informationsblatt, großer Schrift, Querformat
  und rundem Icon wurden angesehen. Bilddateien stammen aus dem vorhandenen
  Website-Fixture. Die Live-Kartenprüfung ist von den deterministischen
  Regressionen getrennt: beide Sprachtests bestehen mit echten OSM-Kacheln und
  ihre Screenshots wurden geprüft. Da der isolierte Emulator keinen direkten
  Internetzugang hat, verwendete dieser manuelle Lauf einen temporären lokalen
  CONNECT-Tunnel ausschließlich zu OSM. HTTPS-Hostname und Zertifikat blieben
  geprüft; Tunnel und Portweiterleitung sind anschließend entfernt.

## Reproduzierbare Prüfungen

- `go test ./...`
- `go test -race ./internal/photos -run 'Test(PhotoRouteCache|PhotoRouteRevision|BrowserPhotoRoute)'`
- `go test ./internal/photos -run '^$' -bench '^BenchmarkNativeGallery$' -benchtime=1x`
- `go test ./internal/photos -run '^$' -bench '^BenchmarkPhotoRouteCache$' -benchtime=3x`
- `apps/android/gradlew -p apps/android :app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest`
- `scripts/test-android-integration.sh --console=plain`
- Lint separat nach der Erzeugung der APK-/Testartefakte: `:app:lintDebug`
- Website: `zensical build -f zensical.toml --clean`; anschließend `git diff --check`
