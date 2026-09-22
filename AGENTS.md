# BearStack Agent Instructions

## Fetch & Pull

Warne mich und verweigere die Ausführung von Befehlen, wenn nicht aktuell gepullt ist.

## Versioning

BearStack uses semantic versioning from the root `VERSION` file. Any agent or maintainer changing tracked project behavior must decide whether the version changes and update `VERSION` in the same change when needed.

- `PATCH`: bug fixes, security fixes, performance work, refactors without new behavior, small UI corrections, and behavior corrections that make an existing endpoint match its intended behavior.
- `MINOR`: backward-compatible features, new optional settings, non-breaking API or UI capabilities, automatic compatible migrations.
- `MAJOR`: intentional incompatible changes to documented HTTP/API/WebDAV contracts, configuration, data formats, permissions, or manual/incompatible migrations that reasonably require users, operators, or client code to change.
- When multiple categories apply, use the highest required bump.
- Do not treat every HTTP status/header/body change as `MAJOR`. If a change fixes a bug, removes an unnecessary redirect while preserving the endpoint's purpose, corrects headers, or otherwise improves an existing response without requiring client rework, prefer `PATCH`.
- Static project website-only changes do not require a BearStack version bump. This covers the Zensical website sources under `_site-src/` and generated website output under `_site/` when the BearStack application, its shipped web UI, API, configuration, data formats, and runtime behavior are otherwise unchanged.
- Docs-only and test-only changes do not require a version bump.
- While BearStack is in `0.x`, the first true major change bumps to `1.0.0`.
- Aktualisiere README, _site und CHANGELOG.

## Performance,

- Achte bei allen Änderungen und Funktionsergänzungen auf die Performance-Implikationen, prüfe und suche die beste Umsetzung für eine herausragende Performance, auch bei vielen Dateien, Ordnern oder Bildern.

## Galerie

- Die Originalfotos bleiben zu jedem Zeitpunkt readonly durch BearStack.

## Productionreadiness

- Achte bei alle Änderungen und Funktionen auf eine sichere und Production-ready Umsetzung. Sichere durch ausführliche Tests ab. 

## Doku und API yaml

- Halte die Dokumentation / Website, die README und die API Dokumentation (openapi) stets aktuell.

## Anzeige von Fotoordnern und Pfaden

- Sichtbare Fotoordnernamen müssen die zentrale Galerie-Formatierung `photos.MediaFolderName` verwenden; sichtbare Fotopfade verwenden `photos.MediaDisplayPath`. Das gilt auch für Tooltips, Dialoge und nachgeladene Kacheln.
- Rohe Verzeichniswerte wie `directory`, `path.Base` oder JavaScript-`split` dürfen nicht als sichtbare Ordnerbeschriftung verwendet werden. Technische Pfade bleiben für URLs, Dateizugriffe und Sortierung unverändert.
- Für JavaScript-Ansichten stellt der Server formatierte Anzeigefelder bereit. Keine eigene Formatierungslogik und kein Rückfall auf Rohpfade im Browser.
- Änderungen an solchen Anzeigen müssen Regressionen für Datumspräfixe, Unterstriche, verschachtelte Ordner und das Stammverzeichnis enthalten. HTML-Erstausgabe sowie Aktualisierung bestehender und neu eingefügter AJAX-Kacheln sind abzudecken.
