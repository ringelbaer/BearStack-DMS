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

## Performance

BearStack trennt Dokumente und Fotodaten, nutzt Caches für aufwendige Medienarbeit und führt OCR sowie Vorschau-Erzeugung im Hintergrund aus. Das ist besonders wichtig, wenn Archive über die Zeit wachsen oder viele Bilder in einem bestehenden Fotoverzeichnis liegen.

## Performance-Benchmark

Die Go-Benchmarks decken zwei zentrale Lastprofile ab: Dokumentlisten mit Suche, Tag-Filtern und Pagination sowie Fotolisten mit großen Indexen, Thumbnail-Status, GPX-Daten und Index-Neuaufbau. Die Dokument-Benchmarks arbeiten mit 1.000, 10.000 und 50.000 Dokumenten. Die Foto-Benchmarks simulieren unter anderem 300.000 Medien in bis zu 5.000 Galerien, geänderte Ordner, viele Tags und Blog-Dateien im Fotoverzeichnis.

```sh
go test ./internal/repository -bench=BenchmarkList -benchmem
go test ./internal/photos -bench=BenchmarkPhoto -benchmem
go test ./internal/photos -bench=BenchmarkMillionPhotoRebuildIndexScenarios -benchmem
```

Für vergleichbare Messungen sollten Benchmarks auf einem ruhigen System laufen, idealerweise mit derselben Go-Version, demselben Datenträger und mehreren Wiederholungen. Die Zahlen hängen stark von CPU, Speicher, Dateisystem und SQLite-I/O ab; die Benchmarks sind deshalb vor allem als Regressionsschutz und Größenordnung für eigene Installationen gedacht.

## Versionierung

Die Anwendungs-Version steht zentral in `VERSION` und wird in der Weboberfläche dezent im Footer angezeigt. BearStack nutzt semantische Versionierung:

- `PATCH`: Bugfixes, Security-Fixes, Performance, Refactors ohne neues Verhalten und kleine UI-Korrekturen.
- `MINOR`: neue rückwärtskompatible Funktionen, neue optionale Einstellungen, nicht-brechende API- oder UI-Fähigkeiten und automatische kompatible Migrationen.
- `MAJOR`: brechende Änderungen an HTTP/API/WebDAV, Konfiguration, Datenformaten, Berechtigungen oder manuelle inkompatible Migrationen.

Bei mehreren Änderungstypen gewinnt die höchste Kategorie. Docs-only- und Test-only-Änderungen erhöhen die Version nicht. Solange BearStack in `0.x` ist, führt der erste echte Major Change auf `1.0.0`.

Für diese Website wurde keine BearStack-Version erhöht, weil sie Dokumentations- und Website-Quellen ergänzt und keine Laufzeitfunktion der Anwendung ändert.
