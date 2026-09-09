# BearStack Fotos – Umsetzung und Abnahme

Dieses Dokument hält den vollständigen Ausbauauftrag fest. Ein abgehakter
Teilbereich ersetzt nicht die Abnahme der gesamten App.

## Architektur

- Die bestehende App-ID `de.bearstack.people` bleibt für Updates erhalten.
- Galerie als Einstieg; sämtliche Personenfunktionen bleiben erreichbar.
- Gemeinsame HTTPS-Verbindung, Zertifikatsprüfung und Bild-Cache je Konto.
- Nur lesende Galerie unter `/api/photos/v1/`; Personenaktionen bleiben unter
  `/api/photos/labeling/v1/`. Beide verwenden die bestehenden Foto-Dienste.
- Begrenzte Seiten und Bildgrößen; Vorschauen nur für die angeforderte Seite.
  Abgebrochene Ansichten dürfen keine veralteten Antworten übernehmen.
- Native Compose-Ansichten einschließlich Text, Karten und Medienbetrachter.

## Offene Umsetzung und Nachweise

- [ ] Lesende Galerie-API, Rechte- und Pfadtests, dokumentiertes OpenAPI-Schema
- [ ] Galerie als Startansicht, Ordnernavigation, zwei Vorschauen und Beschriftung
- [ ] Fotogrid mit lokalisierten Datumsgruppen, Suche und zuverlässigem Nachladen
- [ ] Vollbild mit Wischen und Zoom, Foto-Infos und gestreamtem Download
- [ ] Einstellbare Diashow und Fotoframe mit korrektem Lebenszyklus
- [ ] Blog-, Markdown- und Textansicht mit gemeinsamer sicherer Inhaltsaufbereitung
- [ ] Karten in Foto-Infos und je Ordner, Attribution und begrenzter Tile-Cache
- [ ] Bestehende Personenfunktionen inklusive Zuordnen, Ignorieren, Favoriten,
      Statistik, Wiederaufnahme und ähnlichen Gruppen unverändert verfügbar
- [ ] Deutsch und Englisch für gesamte App, Hilfe, Fehler und Barrierefreiheit
- [ ] Hell-/Dunkelmodus, konsistente Navigation und Google-Fotos-inspirierte Gestaltung
- [ ] Adaptive Icons vollständig im runden Beschnitt sichtbar
- [ ] Begrenzter Speicher, intelligente Vorschauplanung, Konto- und Cache-Isolation
- [ ] JVM-, Server-, Geräte- und HTTPS-Integrationstests, Build und Lint
- [ ] Visuelle Abnahme an Smartphone und Querformat; große Sammlung und Lesekonto
- [ ] VERSION, App-Version, README, CHANGELOG, Website und OpenAPI aktuell

## Bekannte Optimierungspunkte

Die Browser-Library sortiert Ordnermetadaten vollständig und nutzt für Medien
LIMIT/OFFSET. Native Seiten müssen vor der Vorschauabfrage begrenzt werden.
Tiefe Medienseiten, große Ordnerlisten und Such-Postfilter sind vor Abschluss
mit realistischen Datenmengen zu prüfen und gegebenenfalls weiter zu optimieren.

## Zwischenstand 2026-09-09

Implementiert: lesende API und Bibliotheksseiten, nativer Galerieeinstieg,
Lesekonten/Alte-Server-Fallback, Ordner, Suche, Datumsgruppen, Texte, Vollbild/Zoom,
Foto-Infos, Download, Diashow, Fotoframe und nativer Video-/Audio-Player.
Die Checkboxen oben verlangen auch die vollständige Abnahme und bleiben bis dahin offen.

Nachweise bisher:

- Vollständiges `go test ./...` erfolgreich nach API-, Text- und Seitentests.
- Android-Build, Lint und 25 JVM-Tests erfolgreich, darunter Streaming, Abbruch,
  Anfragewechsel und Wiederherstellung nach dem Frame.
- Erster kompletter Emulatorlauf: bestehende Personenabläufe und reale HTTPS-API
  geprüft; ein neuer UI-Test hatte einen mehrdeutigen Dateinamen-Selektor, korrigiert.
- Galerie-/Diashow-UI anschließend erfolgreich. Klassenlisten im Gradle-Filter
  führten tatsächlich nur die erste Klasse aus: Einzelberichte prüfen, nicht die
  beabsichtigte Filterliste als Abdeckung werten.
- Vollständiger Gerätelauf mit 66 Tests: 65 erfolgreich, ein echter Fehler in der
  Download-Abbruchanzeige gefunden und korrigiert. Der gezielte Dateiablage-Test
  (Erfolg, Fehler, Abbruch) besteht danach; Build, Lint und 25 JVM-Tests ebenfalls.
- Vollbild/Frame laden inzwischen höchstens die nächste Aufnahme über die
  vorhandene WLAN-Logik vor. Anzeige und Vorladen verwenden denselben auf
  2048 Pixel begrenzten Request und den bestehenden Drei-Minuten-Speichercache.

Noch wichtig für die Fertigstellung:

- Native Karten pro Foto/Ordner mit Attribution sind implementiert. GPX- und
  fotobasierte Routen sowie die visuelle Abnahme mit echten Kartenbildern fehlen noch.
- Galerie, Personenansichten, Hilfen, Fehler und zugängliche Aktionen sind auf
  Deutsch/Englisch lokalisiert. Die visuelle Gesamtprüfung bleibt offen.
- Die App hält jetzt höchstens drei Metadatenseiten je Bereich und lädt in beide
  Richtungen nach. Tiefe serverseitige Seiten/Ordner sowie Account-/Cache-Verhalten
  müssen weiter mit großen Sammlungen geprüft und gegebenenfalls optimiert werden.
- Fotoframe filtert aktuell auf Bilder; Browser-Medienverhalten bei Videos abgleichen.
- Navigation/Scrollposition beim Wechsel zwischen Galerie und Personen, Querformat,
  große Schrift, Bildfehler und visuelle Gestaltung prüfen und vervollständigen.
- Icon-Skalierung ist implementiert; tatsächlichen runden Launcher-Beschnitt visuell prüfen.
- Website nach weiteren Dokumentationsänderungen neu generieren.

Technische Referenzen für die weitere Umsetzung:
[Storage Access Framework](https://developer.android.com/training/data-storage/shared/documents-files),
[Media3 1.11.0](https://developer.android.com/jetpack/androidx/releases/media3),
[OSM-Tile-Regeln](https://operations.osmfoundation.org/policies/tiles/).
Maps benötigen einen eigenen Client ohne BearStack-Zugangsdaten, sichtbare
Attribution, identifizierbaren User-Agent, begrenzten HTTP-Cache mit Beachtung der
Cache-Header und dürfen nur sichtbare Tiles anfragen (kein Karten-Prefetch).

## Karten – weiterer Zwischenstand 2026-09-09

- `/api/photos/v1/map` nutzt die gemeinsamen Index-, Such- und Sichtbarkeitsregeln.
  Alle indexierten GPS-Aufnahmen im Ausschnitt werden in maximal 289 Markern
  zusammengefasst. Keine Begrenzung auf die ersten 10.000 Medien. Die Datumsgrenze
  funktioniert beim Verschieben und bei der automatischen Erstansicht.
- `/api/photos/v1/map/media` liefert höchstens 96 Aufnahmen pro Seite mit denselben
  Filtern. Identische Orte und Gruppen bei maximalem Zoom öffnen diese Auswahl;
  ein Seitenwechsel ersetzt den Inhalt. Der bestehende Viewer wird wiederverwendet.
- Wiederverwendbare native Karte in Foto-Infos und Ordner-/Suchansicht: Verschieben,
  Zoom, Zurücksetzen, Marker, Fehler-/Wiederholen-Anzeige, zugängliche Aktionen und
  dauerhaft sichtbare OpenStreetMap-Attribution.
- Eigener öffentlicher Kartenclient: feste HTTPS-Zielprüfung, keine BearStack-
  Zugangsdaten/Cookies, keine Weiterleitungen, identifizierbarer User-Agent,
  64-MiB-HTTP-Cache, ETag/Cache-Control/Expires, sieben Tage Ersatzfrist bei fehlenden
  Cache-Headern. Nur sichtbare Tiles; Decodierung 256 px, keine Karten-Vorladejobs.
- Go-Kartentests mit 20.000 indexierten Medien, Seiten, privaten Positionen,
  Such-/Typfiltern, ungültigen Grenzen und Abbruch bestehen. HTTP-Cache-Tests prüfen
  echte lokale Requests einschließlich ETag, no-store und Header-Isolation.
- Karten-UI auf dem Emulator in Deutsch und Englisch geprüft: Einzelmarker, Zoom,
  dichte Gruppen und Attribution. Layout-Screenshot mit lokalen Test-Tiles gesehen;
  dies belegt noch keine vollständige visuelle Abnahme mit echten Straßenkarten.
- Lint-Absturz bei gleichzeitiger KAPT-Stub-Erzeugung und Testanalyse nachvollzogen:
  zuerst APKs/Testartefakte bauen, danach Lint separat; separater Lauf erfolgreich.
  Noch keine Änderung an Gradle-Plugins oder Abschaltung von Prüfungen.

Aktuelle Nachweise:

- Vollständige Go-Suite nach der gemeinsamen räumlichen Abfrage und Korrektur der
  Erstansicht an der Datumsgrenze erfolgreich (`go test ./...`).
- Vollständige Geräte-/HTTPS-Suite: **69 Tests, 0 Fehler, 0 übersprungen**.
  Enthalten sind die bisherigen Personenfunktionen, Download-Abbruch, neue Karten
  in Deutsch/Englisch und das Ersetzen dichter Ortsauswahl-Seiten samt Viewer.
- Der vorherige Download-Abbruchfehler ist damit auch in einem kompletten Lauf
  nachgeprüft. Die Android-Prüfung wurde mit isoliertem Go-HTTPS-Testserver ausgeführt.
- Abschließender Android-Build, **35 JVM-Tests** und Lint nach der Anpassung
  des Karten-Antwortlimits erfolgreich. Website neu erzeugt; `git diff --check`
  ohne Befund.

Der Gesamtauftrag bleibt offen. Als nächstes folgen gemeinsame GPX-/Fotorouten,
serverseitige Seitenskalierung und die visuelle Gesamtprüfung.

## Lokalisierung – weiterer Zwischenstand 2026-09-09

- Galerie und Personenverwaltung einschließlich Hilfe, Statistik, ähnlicher Gruppen,
  Originalvorschau und TalkBack-Aktionen nutzen deutsche und englische Ressourcen.
  Einzahl/Mehrzahl und Zertifikatsdatum sind lokalisiert. Ordner- und Dateinamen
  bleiben unverändert; nur die feste Foto-Wurzelbeschriftung wird übersetzt.
- Ab Android 13 sind Deutsch/Englisch in den Spracheinstellungen der App verfügbar;
  ältere Geräte folgen der Systemsprache. Vorhandene Fehlermeldungen wechseln die
  Sprache mit, ohne die aktive Gruppe oder gespeicherte Aktionen zurückzusetzen.
- Gemeinsame strukturierte Fehlertexte ersetzen ungefilterte Ausnahmemeldungen.
  Serveradressen, Zugangsdaten und Tokens werden nicht als Fehlertext angezeigt.
- Textbeiträge zeigen Fehler mit einer Wiederholen-Aktion direkt im Dialog. Leere
  Beiträge beenden den Ladezustand; verspätete Antworten öffnen geschlossene
  Dialoge nicht erneut.
- Bestehende deutsche UI-Tests verwenden einen auf die jeweilige Composition
  begrenzten Sprachkontext. Dadurch hängen sie nicht von der Emulator-Sprache ab
  und verändern die Sprache anderer Testfälle nicht.

Abschließende Nachweise für diesen Zwischenstand:

- Android-Build, **36 JVM-Tests** und separat ausgeführtes Lint erfolgreich.
- Vollständige Geräte-/HTTPS-Suite: **74 Tests, 0 Fehler, 0 übersprungen**.
  Enthalten sind alle bisherigen Personenabläufe sowie Sprachwechsel, englische
  Gruppenentscheidungen mit Originalvorschau und Textfehler-/Leerzustände.
- Go-Gesamtsuite nach der letzten Serveränderung erfolgreich. Website erzeugt;
  OpenAPI-YAML lesbar und `git diff --check` ohne Befund.
- Der Commit hält den implementierten Zwischenstand fest. Die oben genannten
  offenen Ausbaupunkte und die visuelle Gesamtprüfung bleiben bestehen.

## Begrenzte Galerie-Metadaten – Zwischenstand 2026-09-09

- Gemeinsames Seitenfenster für Medien, Ordner und Texte: maximal drei API-Seiten
  bzw. 288 Medien, 72 Ordner und 60 Textzusammenfassungen je Ansicht. Der Frame
  bewahrt zusätzlich die vorherige Galerie mit denselben Grenzen für die Rückkehr.
- Seiten können an beiden Rändern nachgeladen werden. Ein sichtbarer Inhaltsanker
  erhält seine Pixelposition beim Einfügen und Entfernen von Seiten, auch wenn die
  Ladezeile selbst am Rand steht. Verdeckte Galerien laden nicht automatisch nach.
- Vollbild verwendet stabile Fotopfade; Nachbarn und Wiederholungsanfang werden nach
  dem Laden erneut aufgelöst. Die Positionsanzeige zählt die gesamte Sammlung.
  Nach dem Schließen zeigt die Galerie das zuletzt betrachtete Foto.
- Fehler bleiben beim betroffenen Bereich, Wiederholen lädt dieselbe Seite. Eine
  andere Ansicht oder ein Kontowechsel kann keine verspäteten Seitendaten übernehmen.
  Der Wechsel zur Personenverwaltung und zurück erhält die Galerieposition.
- JVM-Tests mit 250 Medien-/Ordner-/Textseiten in beide Richtungen, einem virtuellen
  Millionenbestand, Ladefehlern, Wiederholung, Seitenauslagerung und verspäteten
  Antworten bestehen. Die Gerätetests prüfen zusätzlich sichtbare Anker und Dialoge.
- Bei der Geräteprüfung behoben: eine direkte Pager-Layoutbeobachtung verursachte
  fortlaufende Neuberechnungen. Die Beobachtung meldet jetzt nur Änderungen der
  sichtbaren Foto-ID. Test-Sprachkontexte gelten ausdrücklich auch für die Ressourcen
  von Dialogen und Menüs, unabhängig von der Emulator-Sprache.
- Performance-/Bedienkorrektur innerhalb des unveröffentlichten Gesamtfeatures:
  BearStack bleibt 0.50.0, Android 0.10.0 (22). Keine Änderung am API-Vertrag;
  README, Android-Anleitung, Fotodokumentation und Website werden mitgeführt.

Zusätzlicher Schutz bei langsamen Anfragen: Die aktuelle sichtbare Auswahl wird
vor dem Entfernen älterer Seiten erneut geprüft. Würde eine inzwischen überholte
Antwort sichtbare Einträge entfernen, wird diese Antwort verworfen. Scrollt der
Benutzer während des Ladens weiter, wird sein neuer Inhaltsanker verwendet.
Ein verspäteter Klick auf ein bereits entferntes Foto öffnet kein anderes Foto.
Sechs gezielte Galerie-Gerätetests bestehen, einschließlich pixelgenauer Ankerprüfung,
Richtungswechsel während einer blockierten Anfrage, Seitenauslagerung im Vollbild,
Fehler/Wiederholen, Wiederaufnahme sowie Ordner- und Textseiten.

Abschließende Nachweise für diesen Schritt:

- **40 JVM-Tests: 0 Fehler, 0 übersprungen.**
- **80 Geräte-/HTTPS-Tests: 0 Fehler, 0 übersprungen**, vollständig mit dem isolierten
  Go-Testserver ausgeführt. Ein zunächst mehrdeutiger „Zurück“-Selektor wurde auf
  den Textdialog eingegrenzt; der Test prüft auch den weiterhin geöffneten Ordner.
- Build und Lint erfolgreich; Website neu erzeugt und `git diff --check` sauber.
  Server und HTTP-Schemas wurden in diesem Schritt nicht verändert.

Geprüfte Anschlusspunkte für die weitere Arbeit:

- GPX-Verarbeitung in `internal/photos/library_gpx.go` ist bereits geteilt und
  begrenzt (16 MiB/100.000 Punkte pro Track, 32 MiB Cache, eigener Parser-Zugang).
  `internal/photos/route.go` liefert die gemeinsame Fotorouten-Aggregation mit
  der konfigurierten Entfernung. Die native Erweiterung muss diese Regeln nutzen
  und große Routen vor der Übertragung und Darstellung sinnvoll begrenzen.
- Der Browser-Fotoframe (`internal/server/static/app-photos-frame.js`) unterstützt
  Videos. Der native Frame filtert bisher auf Bilder; Medienwechsel und Wiederholung
  müssen einschließlich einzelner Videos vervollständigt und geprüft werden.
- Beide Bildreferenzen erneut angesehen: kompakter Dreispaltenraster, schwebende
  Navigation und Foto-Informationen als Blatt über dem Bild sind die visuelle
  Orientierung. Der aktuelle native Aufbau benötigt weiterhin die visuelle Abnahme.
- In Foto-Informationen werden aktuell Datum, Datei, Größe, Kamera/Objektiv und GPS
  dargestellt. Vollständige Uhrzeit und weitere vom Browser bzw. der API angebotene
  Metadaten sind gegen die tatsächliche Browseransicht zu prüfen und zu ergänzen.

Der Gesamtauftrag bleibt aktiv; dieser Schritt schließt die übrigen offenen
Funktionen, serverseitigen Skalierungsprüfungen und visuellen Nachweise nicht ab.
