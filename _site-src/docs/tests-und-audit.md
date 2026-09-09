---
title: Tests und Audit
description: Wie BearStack mit KI-Unterstützung, Tests und Audit-Vorkehrungen entwickelt wird.
icon: lucide/shield-check
---

# Tests und Audit

BearStack wird bewusst als kleines, lokal betreibbares Archivsystem entwickelt. KI und Codex helfen dabei als Entwicklungswerkzeug: Sie beschleunigen Recherche, Refactoring, Testergänzungen und Dokumentation, ersetzen aber nicht die technischen Sicherheitsgrenzen im Projekt. Änderungen sollen aus dem vorhandenen Code heraus entstehen, klein bleiben und durch Tests, Benchmarks oder manuelle Prüfung belegbar sein.

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
