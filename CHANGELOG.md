# Changelog

Alle wesentlichen Änderungen an BearStack werden in dieser Datei dokumentiert.

## 0.22.0 - 2026-08-09

### Hinzugefügt

- Produktionsreife Benutzerverwaltung für SQLite-Konten in der Weboberfläche.
- Rollen, zusätzliche Einzelrechte, Aktivierung, Passwort-Reset und Passwort-Selbstservice.
- Dediziertes Recht `system.users.manage` für delegierbare Benutzerverwaltung.
- Versionierte Sitzungen und begrenzter Schutz vor wiederholten Anmeldeversuchen.

### Geändert

- JSON- und Env-Konten bleiben als schreibgeschützte Konfigurationskonten parallel zu UI-Konten nutzbar.
- Sitzungen werden bei sicherheitsrelevanten Kontoänderungen sofort ungültig; bestehende Cookies im alten Format erfordern nach dem Upgrade eine erneute Anmeldung.
