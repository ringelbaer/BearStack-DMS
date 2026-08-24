# Changelog

Alle wesentlichen Änderungen an BearStack werden in dieser Datei dokumentiert.

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
