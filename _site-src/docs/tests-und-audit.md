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

- Auth ist für nicht-lokale Listener Pflicht.
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

Einzeln:

```sh
go test ./...
make test-go
make test-js
make test-playwright
```

Die Go-Tests decken Repository-Migrationen, Suche, Tags, benutzerdefinierte Felder, Suchfavoriten, Uploads, Auth, Berechtigungen, Audit-Logs, Dokumentverarbeitung, Fotoindex, Thumbnails, Worker und viele HTTP-Handler ab. JavaScript wird per `node --check` geprüft. Playwright-Smokes starten BearStack mit temporärer Konfiguration und prüfen zentrale Browser-Flows wie Dokumenten-Upload und Foto-Galerie.

Für Änderungen an riskanten Bereichen gilt: fokussierte Regressionstests vor breit angelegten Refactors. Wenn eine Änderung Performance berührt, sind Benchmarks oder nachvollziehbare Messungen sinnvoller als reine Einschätzung.

## Performance-Benchmarks

BearStack enthält Benchmarks für Dokumentlisten und das Fotomodul. Sie messen Suche, Tag-Filter, Pagination, große Fotoindexe, GPX-Daten, Thumbnail-Status und Index-Neuaufbau.

```sh
go test ./internal/repository -bench=BenchmarkList -benchmem
go test ./internal/photos -bench=BenchmarkPhoto -benchmem
go test ./internal/photos -bench=BenchmarkMillionPhotoRebuildIndexScenarios -benchmem
```

Die Benchmarks sind als Regressionsschutz gedacht. Absolute Zahlen hängen stark von CPU, Speicher, Dateisystem, SQLite-I/O und Go-Version ab; vergleichbar werden sie erst auf demselben System mit mehreren Wiederholungen.

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
