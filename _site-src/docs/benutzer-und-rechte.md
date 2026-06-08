---
title: Benutzer und Rechte
description: Benutzerkonfiguration, Rollen, Einzelrechte und typische Zugriffsszenarien.
icon: lucide/shield-check
---

# Benutzer und Rechte

BearStack schützt Weboberfläche, API und WebDAV mit derselben Basic-Auth-Konfiguration. Die Rechte sind capability-basiert: Eine Rolle ist nur ein vordefiniertes Bündel aus Einzelrechten. Für spezielle Konten können zusätzlich oder stattdessen einzelne `permissions` gesetzt werden.

Ohne Auth ist BearStack nur für lokale Entwicklung gedacht. Auf Loopback-Adressen wie `127.0.0.1:8080` darf BearStack ohne Login starten; nicht-lokale Listener wie `0.0.0.0:8080` oder `:8080` erfordern Auth. Wenn Auth deaktiviert ist, behandelt BearStack alle Anfragen wie vollständig berechtigt.

## Benutzer anlegen

Die einfache Env-Konfiguration legt genau einen Admin-Benutzer an:

```env
BEARSTACK_AUTH_USER=admin
BEARSTACK_AUTH_PASSWORD_HASH='$2a$10$...'
```

Mehrere Benutzer werden in der JSON-Konfiguration unter `auth.credentials` gepflegt. Sobald diese Liste gesetzt ist, ignoriert BearStack die einfachen Felder `auth.username`, `auth.password` und `auth.password_hash`.

```json
{
  "auth": {
    "realm": "BearStack",
    "credentials": [
      {
        "username": "admin",
        "password_hash": "$2a$10$...",
        "role": "admin"
      },
      {
        "username": "dokumente",
        "password_hash": "$2a$10$...",
        "role": "documents_manager"
      },
      {
        "username": "scanner",
        "password_hash": "$2a$10$...",
        "role": "api_uploader"
      },
      {
        "username": "familie",
        "password_hash": "$2a$10$...",
        "role": "photos_read"
      }
    ]
  }
}
```

`password_hash` hat Vorrang vor `password`. Für produktive Installationen ist ein bcrypt-Hash empfehlenswert; Klartextpasswörter eignen sich vor allem für lokale Tests. Fehlt bei einem Eintrag sowohl `role` als auch `permissions`, wird der Benutzer als `admin` behandelt.

## Rollen

| Rolle | Enthaltene Rechte | Geeignet für |
| --- | --- | --- |
| `admin` | alle Rechte | Vollständige Verwaltung, Systemkonfiguration, Audit-Log und `.adminonly`-Fotoordner |
| `documents_read` | `documents.read`, `documents.webdav.read` | Dokumente suchen, ansehen, herunterladen und per WebDAV lesen |
| `documents_editor` | `documents_read` plus `documents.upload`, `documents.edit` | Dokumente hochladen, Metadaten bearbeiten, OCR starten, Dokumente verknüpfen und vorhandene Tags zuweisen |
| `documents_manager` | `documents_editor` plus `documents.delete`, `documents.structure` | Dokumente löschen/wiederherstellen und Struktur-Daten wie Tags, Suchfavoriten und benutzerdefinierte Felder verwalten |
| `photos_read` | `photos.read` | Foto-Galerie, Suche, Medien, Thumbnails, Zufallsbild und Fotoframe lesen |
| `photos_editor` | `photos_read` plus `photos.edit` | Foto- und Ordner-Tags zuweisen oder entfernen |
| `photos_manager` | `photos_editor` plus `photos.manage` | Fotoeinstellungen, Foto-Tag-Bibliothek, Index-Worker und Thumbnail-Worker verwalten |
| `api_uploader` | `documents.upload` | Scanner, Automationen und Importjobs, die Dateien hochladen, aber keine Dokumente lesen sollen |

Die Rolle `admin` ist mehr als nur eine Summe sichtbarer Menüpunkte: Admin-only-Fotoordner mit `.adminonly` sind nur für Benutzer mit der Rolle `admin` zugänglich. Ein Benutzer mit einzeln gesetzten Vollrechten, aber ohne Rolle `admin`, sieht diese Inhalte nicht.

## Einzelrechte

Einzelrechte werden als `permissions` gesetzt. Wenn `role` und `permissions` gemeinsam verwendet werden, werden die Rechte addiert; `permissions` können Rechte aus einer Rolle nicht entfernen.

| Permission | Wirkung |
| --- | --- |
| `documents.read` | Dokumentlisten, Suche, Detailseiten, Vorschau, Download, Export, Statistik und Dokument-API lesen |
| `documents.webdav.read` | virtuelle Dokumentordner per WebDAV lesen; für WebDAV-`PUT` zusätzlich `documents.upload` erforderlich |
| `documents.upload` | Dokumente über Upload-Endpunkte wie `/upload` oder `/api/upload` hochladen; allein kein Leserecht |
| `documents.edit` | Titel, Notizen, Dokumentdatum, Feldwerte, vorhandene Tags, Verknüpfungen, OCR und Batch-Bearbeitung ändern |
| `documents.delete` | Papierkorb sehen, Dokumente löschen, wiederherstellen, endgültig entfernen und Papierkorb leeren |
| `documents.structure` | Dokument-Tags, Tag-Regeln, Suchfavoriten, benutzerdefinierte Felder und Feldwert-Vorschläge verwalten |
| `photos.read` | Foto-Galerie, Medien, Thumbnails, Suche, Zufallsbild, Fotoframe und Foto-Metadaten lesen |
| `photos.edit` | Tags auf Fotos und Fotoordnern setzen oder entfernen |
| `photos.manage` | Fotoeinstellungen ändern, Index- und Thumbnail-Worker starten und Foto-Tag-Bibliothek pflegen |
| `system.manage` | Systemeinstellungen, Mail-Import, Spalten, Seitengrößen und Favicon verwalten |
| `system.audit` | Audit-Log unter `/log` lesen |

Benutzer ohne `documents.structure` können bei Dokumenten nur bereits vorhandene Tags zuweisen. Neue Dokument-Tags werden bei Metadatenbearbeitung, Batch-Tagging und kompatiblen Uploads abgelehnt. So können Redakteure arbeiten, ohne die globale Struktur des Archivs ungeplant zu verändern.

## Beispiele

Ein reiner WebDAV-Lesezugang:

```json
{
  "username": "dav",
  "password_hash": "$2a$10$...",
  "role": "documents_read"
}
```

Ein Scanner-Zugang für `POST /api/upload`, ohne Leserechte:

```json
{
  "username": "scanner",
  "password_hash": "$2a$10$...",
  "role": "api_uploader"
}
```

Ein Konto, das Dokumente hochladen und per WebDAV-`PUT` importieren darf, aber keine Metadaten bearbeitet:

```json
{
  "username": "eingang",
  "password_hash": "$2a$10$...",
  "permissions": [
    "documents.webdav.read",
    "documents.upload"
  ]
}
```

Ein Foto-Admin für das Fotomodul, aber ohne Systemverwaltung:

```json
{
  "username": "foto-team",
  "password_hash": "$2a$10$...",
  "role": "photos_manager"
}
```

Ein Audit-Konto, das nur das Betriebslog lesen darf:

```json
{
  "username": "audit",
  "password_hash": "$2a$10$...",
  "permissions": [
    "system.audit"
  ]
}
```

## Sessions und Betrieb

Nach erfolgreichem Login setzt BearStack ein signiertes HttpOnly-Session-Cookie. Normale Sessions laufen nach 12 Stunden ab; mit „Eingeloggt bleiben“ nach 30 Tagen. Basic Auth funktioniert weiterhin für API, WebDAV und Automationen. WebDAV antwortet ohne gültige Session mit einem Basic-Auth-Challenge.

Konfigurationsänderungen an Benutzern und Rechten werden beim Start geladen. Nach einem Neustart sind bestehende Sessions ungültig, weil BearStack den Session-Schlüssel neu erzeugt. Für entfernte Benutzer gilt außerdem: Eine vorhandene Session wird nur akzeptiert, wenn der Benutzer in der aktuellen Auth-Konfiguration noch existiert.

## Praxisempfehlungen

- Für öffentlich erreichbare Installationen immer Auth aktiv lassen, auch hinter einem Reverse Proxy, sofern der Proxy keine eigene Zugriffskontrolle erzwingt.
- Für Menschen eher Rollen verwenden; für Scanner, Dashboards und Spezialfälle gezielte `permissions` setzen.
- `admin` sparsam vergeben, weil diese Rolle auch Systemverwaltung, Audit-Zugriff und `.adminonly`-Fotoordner umfasst.
- Passworthashes in JSON oder Env-Dateien sorgfältig quoten, weil bcrypt-Hashes `$` enthalten.
- Für Backups die Konfigurationsdatei mit sichern; dort liegen bei mehreren Benutzern die Accounts und Rechte.
