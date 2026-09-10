---
title: Tests und Audit
description: Wie BearStack mit KI-Unterstützung, Tests und Audit-Vorkehrungen entwickelt wird.
icon: lucide/shield-check
---

# Tests und Audit

BearStack wird bewusst als kleines, lokal betreibbares Archivsystem entwickelt. KI und Codex helfen dabei als Entwicklungswerkzeug: Sie beschleunigen Recherche, Refactoring, Testergänzungen und Dokumentation, ersetzen aber nicht die technischen Sicherheitsgrenzen im Projekt. Änderungen sollen aus dem vorhandenen Code heraus entstehen, klein bleiben und durch Tests, Benchmarks oder manuelle Prüfung belegbar sein.

Die Regressionen zur Dokumentverarbeitung vergleichen SQL-Thumbnail-Kandidaten und Renderer für alle unterstützten Formate, MIME-Parameter und Dateiendungen, einschließlich Pagination, Papierkorb und bereits erzeugter Vorschauen. Ein Benchmark prüft einen Bestand von 50.000 Dokumenten mit wenigen passenden Formaten. Mail-Service-Tests verwenden ein isoliertes Postfach-Doppel und prüfen, dass Import-/Abruffehler keine Löschung auslösen. OCR-Service-Tests prüfen Textübernahme, Fehler, Abbruch, Zeitlimit, gelöschte Dokumente und begrenzte Wecksignale ohne HTTP-Server.

`internal/testutil` stellt die gemeinsame Soffice-Fixture für Import-, Vorschau- und Thumbnail-Tests bereit. Jeder Test erhält ein eigenes temporäres Programmverzeichnis und einen wiederhergestellten `PATH`. Die Fixture prüft die Integration und ersetzt keine Prüfung realer LibreOffice-Konvertierungen.

## KI und Codex im Projekt

Codex wird als Repo-naher Assistent eingesetzt. Der Arbeitsstil ist konservativ: zuerst Code lesen, bestehende Patterns übernehmen, keine unnötigen Abstraktionen einführen und keine fremden Änderungen zurückrollen. Für BearStack ist das besonders wichtig, weil Dokumente, Fotos, Metadaten und Berechtigungen eng zusammenhängen.

Die interne Datei `.codex/AUDIT.md` enthält Prüfaufträge für wiederkehrende Audits. Dazu gehören Cleanup-Pässe, Architektur-Reviews, Performance-Audits, Dependency-Prüfungen, Dokumentationsabgleich, Sicherheitsprüfungen sowie gezielte Audits für Foto- und Dokumentfunktionen. Diese Prompts sind kein Ersatz für Tests, sondern eine wiederholbare Checkliste, damit kritische Bereiche nicht nur spontan geprüft werden.

## Sicherheitsprinzipien

Die wichtigsten Vorkehrungen entstehen direkt in der Anwendung:

- Auth ist für nicht-lokale Listener Pflicht. Ohne Auth müssen sowohl die Gegenstelle als auch der HTTP-Host auf Loopback begrenzt sein; fremde Hostnamen werden als Schutz vor DNS-Rebinding abgewiesen.
- Passwort-Hashes mit bcrypt werden gegenüber Klartextpasswörtern bevorzugt.
- Logins nutzen signierte HttpOnly-Session-Cookies.
- Rollen und einzelne Permissions begrenzen Dokumente, Fotos, Systemverwaltung und Audit-Zugriff.
- Upload-Limits, Dateinamen-Normalisierung und Storage-Root-Prüfung schützen Dateioperationen.
- Unerwartete Import- und Vorschaufehler werden im Browser generisch ausgegeben.
- Dokumente bleiben unverändert; BearStack speichert Metadaten, Volltext und Vorschaudaten getrennt.
- Das Fotomodul kann vorhandene Fotoverzeichnisse read-only anzeigen.
- Admin-only-Fotoordner sind für Nicht-Admins nicht sichtbar und bleiben auch über direkte Medien- und Thumbnail-URLs geschützt.

## Tests

Nur von Tests benötigte Mail-Nachrichtenhelfer liegen in `_test.go`; sie erweitern die Produktionsschnittstellen nicht. Browser-Regressionen für Bildfehler fordern eine eigene Bild-URL an und prüfen die tatsächliche 404-Antwort, damit bereits dekodierte Portraits den Fehlerfall nicht verdecken. Der Login-Helfer der Einstellungs-Suite setzt ein ausdrückliches Rücksprungziel; ein zuvor gespeicherter Startseitenwert beeinflusst dadurch spätere Tests nicht.

Die Standardprüfung kombiniert Go-Tests und JavaScript-Syntaxchecks:

```sh
make test
```

Nach den Syntaxchecks führt `make test-js` auch die DOM-Regressionen aus. Die Personen-Fixtures verwenden eine gemeinsame vollständige Auswahlleiste, damit ein neuer Bedienknopf den restlichen Testlauf nicht durch veraltete Fixtures blockiert.

Regressionen prüfen atomare Einstellungen bei Schreibfehlern, konsistente Datenbank-Snapshots und konkurrierende Cache-Ladevorgänge. Fototests sichern die Beschränkung auf betroffene Personen, importierte Namensquellen, neue Schutzmarkierungen, wartende Prüfungen und eine gebündelte Referenzabfrage ab. Gemeinsame MIME-Fixtures decken Transferdecoder sowie UTF-8-, Windows-1252- und RFC-2231-Anhangsnamen ab. Bildtests prüfen proportionale Gesichtsausschnitte in quadratischen Vorschauen für Hochformat, Querformat, quadratische Regionen und Randbeschnitt bei 160 und 640 Pixeln. Weitere Regressionen sichern den bedarfsweisen Ersatz gestreckter Cachebilder, Fehlerwiederholungen und dieselbe Auslieferung über Web- und Android-/Labeling-Endpunkte ab; Browser-Tests prüfen Hintergrund und quadratische Kacheln auf Desktop und Mobilgeräten.

Einzeln:

```sh
go test ./...
make test-go
make test-js
make test-playwright
```

Die Go-Tests decken Repository-Migrationen, Suche, Tags, benutzerdefinierte Felder, Suchfavoriten, Uploads, Auth, Berechtigungen, Audit-Logs, Dokumentverarbeitung, Fotoindex, Thumbnails, Worker und viele HTTP-Handler ab. JavaScript wird per `node --check` geprüft. Playwright-Smokes starten BearStack mit temporärer Konfiguration und prüfen zentrale Browser-Flows wie Dokumenten-Upload und Foto-Galerie.

Reine Testhelfer bleiben in `_test.go`-Dateien. Ab 0.41.2 liegen auch die Helfer zum Vorbelegen der Einstellungs-Caches in `internal/server/test_helpers_test.go`. Mailimport-Tests rufen die gemeinsamen Anhangsfunktionen direkt auf; die ausschließlich in Tests genutzten PDF-Weiterleitungen sind entfernt. Absenderfilter, Größenlimit, Decodierung und sichere Dateinamen bleiben durch dieselben Prüfungen abgedeckt.

## Testmatrix

| Bereich | Befehl | Voraussetzung / Umfang |
| --- | --- | --- |
| Backend und Browser-DOM | `make test` | Go und Node; einschließlich fester Parserkorpora |
| Browser-Integration | `make test-playwright` | Node, Playwright und Browser; temporärer Go-Server |
| Parser-Korpora | `make test-parsers` | EXIF-IFDs, GPX-Abschnitte und XMP einschließlich defekter Eingaben |
| Parser-Fuzzing | `make fuzz-parsers FUZZTIME=30s` | Zeitbudget je Parser, begrenzte Eingaben; gefundene Fehler als feste Regression übernehmen |
| Faces-Dienst | `make test-faces PYTHON=/pfad/venv/bin/python` | Hashgesicherte `services/faces/requirements.txt`; echte Modelle optional über `BEARSTACK_TEST_FACE_MODELS_DIR` |
| Android Debug | `make test-android` | JDK 17 und Android-SDK; JVM-Tests, Lint, APK |
| Android Release | `make test-android-release` | JVM-Tests, Release-Lint und minimierte R8-APK |
| Android Debug / Go | `make test-android-integration` | Testemulator, `adb`, Python 3; vollständige Instrumentierung gegen temporären HTTPS-Server, Prüfung auf leere Ergebnisse |
| Android Release / Go | `make test-android-release-integration` | Dedizierter Testemulator, `adb`, Python 3; externer UI-Smoke der R8-App für Login, Galerie, Fotoinfo, Wiederherstellung und Kontowechsel |

Android- und Python-Prüfungen bleiben eigene Jobs und machen normale Go-Builds nicht
von diesen Laufzeitumgebungen abhängig. Der Release-Smoke verwendet die Property
`bearstack.releaseSmoke=true`; ohne Produktions-Keystore nutzt nur diese Testvariante
den lokalen Debug-Schlüssel. Die Testwerkzeuge laufen über adb/UIAutomator außerhalb
des App-Prozesses, sodass die App mit den normalen R8-Regeln optimiert wird. Der Smoke
setzt die App-Daten im dedizierten Testemulator zurück und schreibt ein JUnit-XML-Ergebnis;
physische Geräte werden abgewiesen. Der Debug-Gerätelauf lehnt fehlende, leere,
fehlgeschlagene und ausschließlich übersprungene Testergebnisse zusätzlich ab.

Neue Kartenregressionen prüfen die erste Browserantwort nach einer vererbten Ordnersperre,
den abbrechbaren und wiederaufnehmbaren Aufbau optionaler Indizes auf einer gefüllten
Datenbank, die Nutzung des GPX-Inventars sowie Revisionen für Verschieben, Löschen,
Rechte-, Zeit- und GPS-Änderungen. Unbeteiligte Ordner, Typen und Sichtbarkeiten müssen
weiter aus dem warmen Cache bedient werden. Der Benchmark
`go test ./internal/photos -run '^$' -bench '^BenchmarkPopulatedPhotoMapIndexUpgrade$' -benchtime=1x -benchmem`
misst den Indexaufbau mit 100.000 vorhandenen GPS-Medien. `AppSessionTest` prüft
Lesekonten und Kontowechsel ohne Personendatenbank samt getrennten Bildcaches und
vollständigem Schließen alter HTTP-Ressourcen.

Die Go-Anweisungsabdeckung lässt sich reproduzierbar messen:

```sh
go test ./... -coverprofile=/tmp/bearstack-coverage.out
go tool cover -func=/tmp/bearstack-coverage.out
```

Diese Messung umfasst die Go-Tests; Browser-Tests werden separat ausgeführt. Eine hohe Prozentzahl ersetzt keine Prüfung von Fehler- und Abbruchpfaden.

OCR-Tests führen kontrollierte Ersatzprogramme für Tesseract und Poppler aus. Sie prüfen Argumentübergabe, Seitenreihenfolge, Fortschritt, das 50-Seiten-Limit im Fallback, Werkzeugfehler, Abbruch und die Bereinigung temporärer Dateien. Sie benötigen keine installierten OCR-Werkzeuge und bewerten nicht die Erkennungsqualität echter Scans. Dateisystemtests prüfen Traversal sowie interne, externe und ungültige Symlinks beim Auflösen und Anlegen von Verzeichnissen.

Browser-Regressionen prüfen, dass verspätete Metadaten oder Vorschaubilder nach einem Bildwechsel das aktuelle Foto nicht überschreiben. Auch fehlgeschlagene Metadaten- und Vorschauabrufe, die Lightbox ohne Kartenhelfer und die Tastaturnavigation zu den Menü-Icons und Footer-Links sind abgedeckt. Die Berechtigungsmatrix der Navigation umfasst unter anderem reine WebDAV-Rechte und die Einstellungsziele bei deaktiviertem Fotomodul.

Regressionstests prüfen den Zugriff ohne Auth über lokale und fremde Hostnamen einschließlich gefälschter Forwarded-Header. Präparierte WebP-Dateien mit widersprüchlichen Bild- und Alpha-Dimensionen müssen bei der Gesichtsvorverarbeitung einen Fehler statt eines Absturzes auslösen; gültige WebP-Bilder bleiben verarbeitbar. Die Mindestversion Go `1.26.6` und `golang.org/x/image` ab `v0.45.0` enthalten die zugehörigen Sicherheitskorrekturen.

Zusätzliche Regressionstests vergleichen die Normalisierung von Fotoeinstellungen aus Datenbank und HTTP-Formular, prüfen die Abfrageanzahl für HTML- und API-Dokumentlisten und erhalten deren unterschiedliche Behandlung zu hoher Seitenzahlen. Ein Browser-Test lädt den Fotoframe mit leerem Galerie-Script und prüft, dass das gemeinsame Medienmodul für die Anzeige ausreicht.

Performance-Regressionen vergleichen die begrenzte Unicode-Distanzberechnung der Feldwertsuche mit einer vollständigen Referenz. Tagdefinitionen bleiben ohne Dokumentzuordnungstabelle lesbar; Abfragen einzelner Tagnamen werden an Batchgrenzen geprüft. Die Foto-Startprüfung benötigt bei 401 unveränderten Ordnern vier SQL-Abfragen und keine Updates. Weitere Tests prüfen geänderte und vererbte `.adminonly`-Sichtbarkeit sowie das Zurückrollen bei Datenbankfehlern. Browser-Tests sichern bei 10.000 Trackpunkten eine Größenmessung je Renderdurchlauf, wiederverwendete Polylinien und einmalige Darstellung bereits geladener Lightbox-Medien ab.

Für Änderungen an riskanten Bereichen gilt: fokussierte Regressionstests vor breit angelegten Refactors. Wenn eine Änderung Performance berührt, sind Benchmarks oder nachvollziehbare Messungen sinnvoller als reine Einschätzung.

Bei Versionsänderungen muss `info.version` in `openapi.yaml` mit der Root-Datei `VERSION` übereinstimmen. Der Go-Test `TestOpenAPISpecMatchesApplicationVersion` prüft diesen Abgleich für die eingebettete API-Beschreibung.

Playwright baut einmal pro Testlauf ein temporäres BearStack-Binary. Alle Suiten verwenden denselben Helfer für Start, Gesundheitsprüfung und geordnetes Beenden; temporäre Daten werden erst nach Prozessende entfernt. Die Testabhängigkeit ist in `package-lock.json` festgelegt und wird bei Bedarf mit `npm ci --ignore-scripts` installiert. Ein vorhandener `GOCACHE` wird weiterverwendet.

Sicherheitsregressionen ab 0.42.1 prüfen defekte `.adminonly`-Symlinks über Ordner-, Medien-, Batch- und Gesichtsprüfungen sowie die HTTP-Routen. Verweigerte Verzeichniszugriffe dürfen keine Gesichtsdaten löschen; unauflösbare Indexpfade dürfen private Einträge beim Start nicht veröffentlichen. TLS-Tests prüfen bei erneuter automatischer Erzeugung die Schlüsselrechte `0600`, unveränderte Symlink-Ziele und das Entfernen temporärer Dateien.

Browser-Regressionen ab 0.43.0 prüfen den Gruppenbild-Zoom auf 200 % der Bounding Box bei Desktop- und Mobilgrößen sowie Hoch- und Querformaten. Sie vergleichen die sichtbare Markierung mit dem tatsächlich skalierten Foto, prüfen Bildränder, Tastaturbedienung, Wechsel zwischen Gesichtern, Erhalt beim Benennen und Rücksetzen bei Navigation. Ungültige Regionen und fehlgeschlagene Bilder dürfen keinen Zoom aktivieren; Zoomwechsel dürfen keine zusätzlichen Bild- oder Datenabrufe auslösen.

Android-App 0.6.0 ergänzt Tests für den Personenbereich: mehrseitige Listen, Umbenennen, Favoriten, Entfernen des letzten Gesichts, Konflikte und verlorene Quittungsantworten. Instrumentierte Tests prüfen große Schrift und die Originalfoto-Geste einschließlich Abbruch und TalkBack. Der HTTPS-Test mit echtem Go-Server führt die Verwaltungsaktionen vollständig aus. Backendtests prüfen Schutzmarker, Rollback, Duplikatnamen und die wiederholbare Migration auf Foto-Schema 24; ein Abfrageplan-Test mit 20.000 unbenannten Gruppen sichert die Nutzung des partiellen Index ab.

Die Personen-Modaltests prüfen übereinstimmende Thumbnails in Übersicht und Vorschlagsliste, feste Bildgrößen auf Mobilgeräten und Desktop, Auswahl über Bildklick und Tastatur sowie bedienbare Vorschläge bei Bildfehlern. API- und Bibliothekstests sichern aktive Ersatzportraits nach Ignorieren und erneute Schutzmarkerprüfungen beim Bildabruf ab. Eine lokale SQL-Vergleichsmessung mit 10.000 Personen und je 3.000 Gesichtern für die ersten 61 Gruppen ergab rund 14 ms für die begrenzte indexgestützte Portraitabfrage gegenüber 56 ms für eine gemeinsame Aggregation über materialisierte Kandidaten; dies misst ausschließlich synthetische SQL-Abfragen, keine HTTP-Gesamtlatenz.

## Dependency-Prüfung

Die Prüfung vom 8. September 2026 entfernt nur `androidx.room:room-ktx:2.8.4` aus dem Android-Build. Das Artefakt enthält keine Klassen; seine APIs sind bereits im explizit eingebundenen `room-runtime` enthalten ([Room-Releases](https://developer.android.com/jetpack/androidx/releases/room)). Es gibt keine Änderung am Datenbankschema oder am Laufzeitverhalten und keinen zusätzlichen Versionssprung.

Ein Vorher-/Nachher-Vergleich der aufgelösten Abhängigkeiten umfasst sechs Android-Konfigurationen: Debug-Compile-/Runtime-Classpath, Release-Runtime, JVM-Test-Runtime sowie Compile-/Runtime-Classpath der instrumentierten Tests. Alle übrigen Module behalten dieselben Versionen; `room-ktx` entfällt dort, wo es zuvor enthalten war.

Alle acht direkten Go-Module haben Importpfade; `golang.org/x/sys` wird beispielsweise im Linux-Indexworker verwendet. `go mod tidy -diff` bleibt leer, `go mod why -m all` erklärt die Abhängigkeitsketten. Einträge in `go.sum` werden nicht allein aufgrund fehlender direkter Imports gelöscht. Playwright ist ausschließlich ein Testwerkzeug. NumPy und OpenCV werden vom Gesichtsdienst verwendet; Zensical und Pygments bauen die Website einschließlich Syntaxhervorhebung. Auch Chromium, LibreOffice, Poppler, Tesseract und FFmpeg werden für Mailarchivierung, Vorschauen, OCR oder Thumbnails aufgerufen.

Bei Updates verdienen diese Punkte besondere Aufmerksamkeit:

- **Android-Build:** `apps/android/gradle.properties` aktiviert Legacy-DSL und externes Kotlin. Gradle meldet bereits abgekündigte Optionen; vor AGP 10 sind DSL-/Kotlin- und kapt-Migrationen nötig ([AGP-Migrationsplan](https://developer.android.com/build/releases/gradle-plugin-roadmap)). Das ist ein eigener Umbau, keine Entfernung ungenutzter Plugins.
- **Aufgelöste Versionen:** Die Android-Deklarationen für Lifecycle nennen 2.8.7, tatsächlich wird über den Abhängigkeitsgraphen 2.9.4 aufgelöst. Gradle-Lockfiles und Verifikationsmetadaten sind nicht vorhanden. Website-Dependencies haben teilweise offene transitive Versionsbereiche; Docker-Basisimages und apt-Pakete sind ebenfalls nicht vollständig eingefroren. Bei Updates deshalb den effektiven Graphen und die erzeugten Artefakte vergleichen.
- **Gebündelte Komponenten:** PDF.js liegt versioniert unter `internal/server/static/vendor` und wird nicht über `package.json` gepflegt. Viewer, Worker und Zusatzdateien müssen zusammen aktualisiert werden. Die Go-SQLite-Abhängigkeiten und der Python-Gesichtsdienst sind funktional erforderlich; Updates brauchen insbesondere Datenbank-, Gesichtsabgleich-, Modell- und Plattformtests. Die Modellkennung und Prüfsummen gehören zum internen Gesichtsprotokoll.

## Performance-Benchmarks

BearStack enthält Benchmarks für Dokumentlisten, Feldwertvorschläge und das Fotomodul. Sie messen Suche, Tag-Filter, Pagination, Ähnlichkeitssuche mit 1.500 Feldwerten, große Fotoindexe, GPX-Daten, Thumbnail-Status und Index-Neuaufbau.

```sh
go test ./internal/repository -bench=BenchmarkList -benchmem
go test ./internal/server -run '^$' -bench=BenchmarkSimilarCustomFieldValues -benchmem
go test ./internal/photos -bench=BenchmarkPhoto -benchmem
go test ./internal/photos -bench=BenchmarkMillionPhotoRebuildIndexScenarios -benchmem
```

Die Benchmarks sind als Regressionsschutz gedacht. Absolute Zahlen hängen stark von CPU, Speicher, Dateisystem, SQLite-I/O und Go-Version ab; vergleichbar werden sie erst auf demselben System mit mehreren Wiederholungen.

Die GPX-Benchmarks setzen für Messungen ohne Cache auch die LRU-Verwaltung und den Speicherzähler zurück. Reine Go-Testhelfer liegen in `_test.go`-Dateien und werden nicht in das Anwendungsbinary übernommen.

### Performance des Lupen-Gesichtabgleichs

Audit und Optimierung vom **10. September 2026**, Ausgangsstand `9dbf9d3` /
BearStack 0.50.0. Geprüft wurde der Abgleich im Benenn-Modal über
`/photos/faces/{id}/suggestions`, den WebUI und Android gemeinsam verwenden.
Die erste Tabelle und die drei Befunde dokumentieren den **Ausgangsstand**;
die anschließend umgesetzten Optimierungen und neuen Messwerte folgen darunter.

Die Messung verwendet einen Ryzen 5 9600X, Go 1.26.6 und temporäre SQLite-Datenbanken
auf Linux-tmpfs. Je Gruppe sind 30 normalisierte Referenzvektoren mit 128 Dimensionen
und zehn Fotodatensätze vorhanden, insgesamt bis zu 1.000 echte Testverzeichnisse.
Nur das Quellfoto ist eine echte Bilddatei. Kandidatenbilder, Netzwerk, Thumbnail-
Dekodierung, NAS-Latenz und Hintergrundverarbeitung sind nicht Teil der Backend-Zeiten.
Die Werte sind lokale Vergleichsmessungen, keine Zusage für Produktionsbestände.

| Szenario | 1.000 Gruppen / 30.000 Referenzen | 10.000 Gruppen / 300.000 Referenzen |
| --- | ---: | ---: |
| Warmer Stream, frühe Treffer, ohne SQLite-Statistiken | 1.636 ms | Abbruch erst nach 15.320 ms trotz 5-s-Deadline |
| Erster Zwischenstand dabei | 0,68 ms | 2,69 ms |
| Warmer Stream, frühe Treffer, nach `ANALYZE` der Testdatenbank | 4,34 ms | 50,54 ms |
| Warmer Stream ohne Treffer | 2,11 ms | 45,98 ms |
| Ständig bessere Treffer, Stream, nach `ANALYZE` | 59,00 ms / 34 Antworten | 670,05 ms / 315 Antworten |
| Dieselben ständig besseren Treffer, einzelnes JSON | 6,53 ms | 56,22 ms |
| Im Referenzcache zusätzlich belegter Go-Heap | 19,46 MiB | 187,75 MiB |
| Ausstehende Neuauswahl aller Referenzen | 227 ms | 2.646 ms |

Erfolgreiche warme Fälle enthalten drei bis zehn Wiederholungen und zeigen den
Median; Neuauswahl und Cache-Speicher wurden einmal gemessen. Der große Aufruf ohne
Statistiken wurde wegen des Fehlplans nach einem Versuch abgebrochen: 15.320 ms sind
die beobachtete Rückkehr mit Fehler, keine erfolgreiche Suchzeit. Die bereits
gesetzte 5-s-Deadline begrenzte die laufende SQLite-Operation nicht rechtzeitig.
Die Stream-Antwortzahlen schließen den Abschluss ein. Gemessene JSON-Nutzlasten
enthalten die Ranglisten ohne HTTP-Header und zusätzliche NDJSON-Statusfelder.

**Befund 1 – SQL-Abfragereihenfolge, hohe Priorität:**
`validateFacePersonCandidates` in `internal/photos/faces_candidates.go` prüft
höchstens 512 bekannte Gesicht-IDs pro Abfrage. Ohne passende Statistiken beginnt
SQLite trotzdem bei allen öffentlichen Medien über
`idx_media_index_admin_directory_random`, statt bei diesen IDs. Bei 1.000 Gruppen
entfallen 1.618 ms auf diese Prüfung, rund 3,3 ms auf das Ranking und 0,4 ms auf
die Ergebnisaufbereitung. Der CPU-Profiler bestätigt SQLite als Hauptverbraucher.
Ein ausschließlich im Test erzwungener Start bei den Gesicht-IDs mittels
`CROSS JOIN` liefert dieselben Kandidaten in 1,65 ms. Eine künftige Korrektur sollte
die kleine ID-Menge zuerst abfragen und den Plan mit und ohne Statistiken absichern;
sämtliche aktuellen Sichtbarkeits- und Zuordnungsprüfungen müssen erhalten bleiben.
`ANALYZE` wurde hier nur als Diagnose auf der Testdatenbank ausgeführt.

**Befund 2 – gemeinsame Sperre und verzögerter Abbruch, hohe Priorität:**
`suggestPeopleForFace` hält `faceRuntime.mu` von der Quellprüfung bis zum letzten
Zwischenstand. Der HTTP-Handler schreibt und flusht Zwischenstände innerhalb dieses
Bereichs. Ein langsamer Empfänger kann weitere Suchen und Gesichtsänderungen damit
aufhalten. Das fünfsekündige Schreiblimit gilt pro Ausgabe, nicht für die gesamte
Suche. Ein zweiter Aufruf mit 20-ms-Deadline kehrte im kontrollierten Test auch
100 ms nach Ablauf nicht zurück, solange der erste Stream blockierte. Empfohlen
sind abbrechbares Warten und eine Trennung von Cache-Zugriff und Netzwerkausgabe,
unter Beibehaltung der Revisions- und Sichtbarkeitskontrollen.

**Befund 3 – viele Zwischenstände und vollständiger Cache, mittlere Priorität:**
Das Streaming prüft bei laufend besseren Treffern viele Gruppen erneut gegen SQL
und stellt jede geänderte Top-20-Liste neu zusammen. Bei 10.000 Gruppen waren das
315 Antworten, etwa 396 kB Ranglisten-JSON und 52,76 MiB kumulierte Go-Allokationen
pro Suche; das einzelne JSON benötigte 1,19 MiB. Die Allokationen sind kein dauerhaft
belegter Speicher. Der Browser verarbeitet jeden Zwischenstand: 315 bereits
gepufferte Antworten brauchten mit dem echten Modal-Skript rund 178 ms bei 1.440 px
und 185 ms bei 390 px Fensterbreite, mit durchgehend höchstens 20 Optionen. Diese
isolierte Chromium-Messung enthält keine echten Vorschaubilder und ist kein
Android-Gerätebenchmark. Android hält bereits nur den neuesten wartenden Stand.
Eine zeitliche Begrenzung der Zwischenstände und ein zusammengefasstes Rendern pro
Browser-Frame können den Aufwand reduzieren, ohne den ersten Treffer zu verzögern.

Der kalte Cache lädt auch Referenzen unbenannter Gruppen. Bei 90 % unbenannten
Gruppen dauerte der erste Zwischenstand mit 300.000 Referenzen noch etwa 521 ms;
der gesamte Aufruf allokierte rund 551 MiB, obwohl nur 10 % der Gruppen verglichen
wurden. Ein ausstehender Referenzneuaufbau läuft vor dem ersten Treffer. Favoriten
können die normale Zielanzahl von 30 Referenzen zusätzlich überschreiten. Weitere
Optimierungen sollten deshalb Cache-Aufbau und Referenzvorbereitung gesondert messen.
Ein positiver Vorschlagsabstand wartet absichtlich auf das vollständige Ranking.

Reproduzieren (opt-in, ausschließlich temporäre Daten):

```sh
BEARSTACK_FACE_SUGGESTION_PERF=1 go test ./internal/photos -run '^TestFaceSuggestionPerformance$' -count=1 -v -timeout=8m
BEARSTACK_FACE_SUGGESTION_PERF=1 BEARSTACK_FACE_SUGGESTION_CPU=/tmp/face-suggestions.cpu go test ./internal/photos -run '^TestFaceSuggestionPerformanceStages$' -count=1 -v
go tool pprof -top /tmp/face-suggestions.cpu
BEARSTACK_FACE_SUGGESTION_PERF=1 PLAYWRIGHT_BROWSER_CHANNEL=chromium npm exec -- playwright test tests/playwright/face-suggestions-performance.spec.mjs
```

Der Stufentest vergleicht die feste Abfragereihenfolge mit der bisherigen frei
planbaren Variante. Die Umfangsmessung protokolliert Cache-Zustände, erste Ausgabe,
Laufzeit, Antwortzahl, Allokationen und Abbruchverhalten. In normalen Testläufen
werden die aufwendigen Messungen übersprungen.

#### Umgesetzte Optimierung

Die Kandidatenprüfung startet jetzt ausdrücklich bei höchstens **512 Gesicht-IDs
pro SQL-Paket**. Alle infrage kommenden benannten Gruppen bleiben im Vergleich;
512 ist keine Obergrenze für Personen, Referenzgesichter oder den Suchbestand.
Der Plan wird mit und ohne `ANALYZE` geprüft. Die Namen-, Medien-, Rechte- und
Revisionsprüfungen bleiben erhalten.

Ein eigener unveränderlicher Referenzstand lädt nur benannte Gruppen. Aktuelle
Gruppenrevisionen ermöglichen die Wiederverwendung unveränderter Gruppen nach
Bearbeitungen; Referenz-/Modelländerungen invalidieren betroffene Cache-Stände.
Bereits verfügbare Vektoren des Hintergrundabgleichs können ohne wartende Sperre
geteilt werden. Fehlt der globale Referenzaufbau noch, wird die Auswahl nur für die
benötigten benannten Gruppen gelesen. Favoriten, Qualitätsregeln und Verteilung auf
Ordner entsprechen der bestehenden globalen Auswahl. Die temporäre Sortierung kann
auf Datenträger auslagern; die Lupe löst keinen globalen Neuaufbau aus. Der bestehende
Hintergrundlauf bleibt für dessen Fortsetzung zuständig, sofern aktiviert.

Cache-Aufbau wird zwischen Anfragen koordiniert, wartende Aufrufe sind abbrechbar.
Vergleich und Netzwerkausgabe halten keine Schreibsperre der Gesichtsverarbeitung.
Quellen, Gruppenrevisionen und Einstellungen werden vor Ausgaben aktuell geprüft;
bei Konflikten muss die Suche gegebenenfalls wiederholt werden. Das Löschen der
Gesichtsdaten und Schließen der Bibliothek leeren den Cache. Bereits laufende alte
Ladevorgänge dürfen ihn danach nicht erneut veröffentlichen.

Der erste geprüfte Treffer bleibt sofort verfügbar. Weitere Zwischenstände werden
höchstens alle 100 ms geprüft und ausgegeben; das vollständige Endergebnis folgt
immer. Ein positiver Vorschlagsabstand wartet weiterhin auf das gesamte Ranking,
einschließlich eines zweitbesten Kandidaten unterhalb der Ähnlichkeitsschwelle.
Im Browser löst nur der neueste wartende Stand pro Bildschirmaktualisierung einen
Neuaufbau der Liste aus. Endergebnis, Fehler und Abbruch verwerfen ausstehende
Zwischenstände sofort. Das funktioniert auch bei einem pausierten Browser-Frame.

Neue Messung auf derselben Maschine und mit denselben synthetischen Beständen:

| Szenario | Vorher | Nachher |
| --- | ---: | ---: |
| Warmer Stream, 1.000 Gruppen, ohne `ANALYZE` | 1.636 ms | 5,01 ms |
| Warmer Stream, 10.000 Gruppen, ohne `ANALYZE` | Abbruch erst nach 15.320 ms | erfolgreich nach 56,07 ms |
| Warmer Stream, 10.000 Gruppen, nach `ANALYZE` | 50,54 ms | 55,64 ms |
| Ständig bessere Treffer, 10.000 Gruppen | 670,05 ms / 315 Antworten | 57,38 ms / 2 Antworten |
| Allokationen bei ständig besseren Treffern | 52,76 MiB | 1,48 MiB |
| Kalter Stream, 10.000 Gruppen, davon 90 % unbenannt | 590,57 ms | 60,18 ms |
| Erster Zwischenstand in diesem kalten Fall | 521,08 ms | 52,61 ms |
| Allokationen in diesem kalten Fall | 551,24 MiB | 51,17 MiB |
| Go-Heap für 300.000 benannte Referenzen | 187,75 MiB | 164,40 MiB |
| Browser-Burst mit 315 Antworten, 1.440 px | 177,6 ms | 4,5 ms / ein Listenaufbau |
| Derselbe Browser-Burst, 390 px | 184,8 ms | 2,1 ms / zwei Listenänderungen |

Der neue Aufwand für revisionssichere Snapshots ist bei bereits günstigem SQL-Plan
leicht höher; bei 10.000 benannten Gruppen bleibt der warme vollständige Vergleich
hier unter 60 ms. Mit ausstehender Referenzauswahl und 90 % unbenannten Gruppen
dauert der neue Modal-Aufruf 142,94 ms, während die separat gemessene globale
Neuauswahl 2.754 ms benötigt. Der kalte Aufbau für ausschließlich benannte Gruppen
benötigt weiterhin etwa 565 ms. Ein zweiter Aufruf kann während eines blockierten
Stream-Empfängers fertig werden; zusätzliche Tests prüfen abbrechbare Cache-Wartezeit
und parallele Favoritenänderungen. Die Browserwerte stammen aus einem isolierten
Chromium-Lauf ohne echte Vorschaubilder, nicht von einem Android-Gerät.

Die vollständigen Go-Tests, gezielte Race-Prüfungen, JavaScript-Checks und
Browser-Regressionen sichern den Ablauf ab. Dies ist eine PATCH-Optimierung innerhalb
der unveröffentlichten 0.50.0; VERSION und API-Schema bleiben unverändert, eine
Migration ist nicht erforderlich. Die OpenAPI-Beschreibung dokumentiert die
Zwischenstände und das Verhalten bei konkurrierenden Änderungen.

## Audit-Log in BearStack

BearStack protokolliert schreibende Aktionen im Laufzeit-Audit-Log. Erfasst werden unter anderem Zeitpunkt, Benutzer, HTTP-Methode, Pfad, Route, Aktion, Ziel, Status, Remote-Adresse und User-Agent. Dazu gehören auch das Anlegen, Ändern, Aktivieren, Deaktivieren und Löschen von Benutzern sowie Passwortänderungen; Passwörter und Hashes werden niemals übernommen. Das Log ist über `/log` erreichbar und erfordert die Permission `system.audit`; die Rolle `admin` enthält diese Berechtigung.

Das Audit-Log speichert Schreibmethoden wie `POST`, `PUT`, `PATCH` und `DELETE`. Handler können das Ziel genauer benennen, zum Beispiel ein Dokument, einen Tag, ein Feld oder einen Suchfavoriten. Einträge werden auf die letzten 30 Tage und zusätzlich auf höchstens 50.000 Datensätze begrenzt. Abgewiesene Kontoaktionen werden nur mit einem begrenzten In-Memory-Budget protokolliert, damit fehlerhafte oder anonyme Requests weder SQLite noch den Datenträger fluten können; erfolgreiche Kontoänderungen werden unabhängig davon erfasst.

## Manuelle Audits

Die Audits in `.codex/AUDIT.md` sind nach Themen gruppiert:

- Repo-Cleanup und tote Dateien
- Architektur und Modulgrenzen
- Performance-Hotspots in Backend, Storage und UI
- Konfiguration, Auth, Dateizugriffe, Uploads, Secrets, Logging und Fehlerbehandlung
- Dependency-Risiken
- Dokumentationsstand
- Foto-Berechtigungen, Thumbnail-Erstellung, Indexierung, Worker und Tagging
- Dokument-Berechtigungen, Verarbeitung, Batch-Editing, virtuelle Ordner und Tagging

Ein Audit sollte konkrete Dateien, Risiken und Tests nennen. Wenn ein Befund umgesetzt wird, bleibt die Änderung klein, bekommt passende Tests oder Benchmarks und wird danach mit den relevanten Befehlen verifiziert.
