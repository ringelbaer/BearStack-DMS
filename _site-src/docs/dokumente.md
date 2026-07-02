---
title: Dokumente
description: Dokumentablage, Suche, Mail-Import, Tags und virtuelle Ordner.
icon: lucide/file-text
---

# Dokumente

BearStack organisiert Dokumente um die vorhandenen Dateien herum. Die Dateien bleiben unverändert im lokalen Speicher; Metadaten, Volltext, Vorschaudaten und Indexe liegen getrennt davon in SQLite und Cache-Verzeichnissen.

## Ablage und Verarbeitung

Dokumente können über die Weboberfläche (`POST /upload`), die JSON-API (`POST /api/upload`) oder kompatible `PUT`-Anfragen importiert werden. BearStack speichert die Originaldatei im konfigurierten `storage_dir`, legt Metadaten in SQLite ab und führt Text-, Vorschau- und Thumbnail-Verarbeitung asynchron im Hintergrund aus.

Unterstützt werden PDF, Bilder sowie einfache Text- und Office-Formate. Office-Text und Office-Vorschauen benötigen LibreOffice. Uploads werden nach dem konfigurierten Limit begrenzt, Dateinamen werden normalisiert, unbekannte Dateitypen werden abgelehnt und gespeicherte Pfade immer gegen den Storage-Root aufgelöst. Unerwartete Import- und Vorschaufehler werden für HTTP-Antworten generisch ausgegeben, damit interne Pfade oder Werkzeugdetails nicht im Browser landen.

Weitere Dokumentfunktionen:

- Detailseiten mit Metadaten, Vorschau, Verknüpfungen und gruppierten Dateien
- E-Mail-Import für PDF-Anhänge und EML-Archive aus einem IMAP-Postfach
- Papierkorb, Duplikatübersicht und Exportfunktionen
- Hintergrundverarbeitung mit nachvollziehbaren Statusansichten
- Statistik- und Audit-Ansichten für den Betrieb

## Suche und OCR

- Volltextsuche über extrahierte Inhalte
- OCR-Unterstützung für PDFs und Scans
- Filter nach Tags, Feldwerten und Zeiträumen
- Suchfavoriten für wiederkehrende Filter
- Wortwolke für häufige und fachlich zentrale Tags
- virtuelle Ordner für Tags, Felder und Favoriten

## E-Mail-Import

Der E-Mail-Import ruft PDF-Anhänge und angehängte `.eml`-Dateien aus einem IMAP-Postfach ab und führt sie durch denselben Dokumentimport wie normale Uploads. PDF-Anhänge bleiben dabei unverändert; BearStack übernimmt die PDF-Datei in den Dokumentenspeicher, erkennt Duplikate und startet die weitere Verarbeitung wie Vorschau, Volltext und OCR-Status getrennt davon.

EML-Anhänge werden als eigenständige E-Mail-Archive importiert. BearStack erzeugt dafür ein PDF mit unabhängigem Metadaten-Deckblatt, gerenderter sicherer HTML-/Textabbildung der E-Mail und anschließenden PDF-Anhängen aus der EML. Nicht-PDF-Anhänge innerhalb der EML werden auf dem Deckblatt mit Name, Typ und Größe gelistet, aber nicht eingebettet. Für die gerenderte Mailabbildung wird `chromium` benötigt; für das Zusammenführen von Deckblatt, Mailabbildung und PDF-Anhängen nutzt BearStack `pdfunite` aus `poppler-utils`.

Konfiguriert wird der Import unter `Einstellungen -> E-Mail-Import`. Unterstützt werden SSL/TLS, STARTTLS und unverschlüsselte IMAP-Verbindungen; Standardwerte sind Port `993`, `INBOX` und ein Abrufintervall von 15 Minuten. Ein Verbindungstest prüft die Zugangsdaten, ein manueller Lauf ruft sofort ab, und bei aktiviertem Import prüft BearStack das Postfach regelmäßig im Hintergrund.

Die Absenderliste kann leer bleiben oder einzelne Adressen und Domänen enthalten. Eine leere Liste verarbeitet alle Absender. Domänenregeln passen auch auf Subdomains; nicht erlaubte Absender werden abgelehnt, protokolliert und aus dem IMAP-Postfach gelöscht. Die Prüfung bezieht sich auf die Import-Nachricht im IMAP-Postfach. Erfolgreich verarbeitete E-Mails mit PDF- oder EML-Anhängen werden ebenfalls gelöscht, damit das Postfach als Eingangskorb funktioniert. E-Mails ohne verarbeitbare Anhänge bleiben unberührt.

Alle Importläufe werden im Audit-Log sichtbar: erfolgreiche Importe, Duplikate, abgelehnte Absender, Verbindungstests und Fehler bekommen eigene Einträge. Die Verwaltung des Mail-Imports erfordert Systemverwaltungsrechte.

## Tags

Tags sind kurze, frei wählbare Markierungen für Dokumente. Sie bleiben als Metadaten in BearStack gespeichert und verändern die Dateien nicht. Tags lassen sich in Listen, Detailseiten, Filtern und Batch-Aktionen nutzen, damit ein Dokument gleichzeitig in mehreren fachlichen Zusammenhängen auffindbar bleibt.

Benutzer ohne Struktur-Recht können nur vorhandene Dokument-Tags zuweisen. Neue Tags werden bei Dokument-Metadaten, Batch-Tagging und kompatiblen Uploads abgelehnt, wenn die Berechtigung für Struktur-Daten fehlt.

Dokument-Tags können mehr als nur markieren:

| Funktion | Wirkung |
| --- | --- |
| Farbe und Beschreibung | machen Tags in Listen, Details, Filtern und Auswahlfeldern leichter unterscheidbar |
| Automatisches Tagging | Regelsets vergeben Tags anhand von Dateiname, Textinhalt oder beidem; Regeln können „mindestens eins“ oder „alle“ Treffer verlangen und Ausschlüsse definieren |
| Primärer Tag | markiert fachliche Hauptkategorien für die Wortwolke; primäre Tags bilden dort eigene Bereiche |
| Gruppenmodus | verbindet Dokumente mit demselben Gruppentag automatisch; sie erscheinen auf Detailseiten als gruppierte Dateien, auch ohne manuelle Verknüpfung |
| Nur im Detail anzeigen | hält interne oder sehr technische Tags in Listen kompakt verborgen; sie bleiben suchbar und sind auf Detailseiten sichtbar |
| Löschschutz | verhindert das Löschen von Dokumenten mit diesem Tag; Löschaktionen werden ausgeblendet und direkte Löschversuche serverseitig blockiert |

## Dateien verknüpfen

BearStack kann Dokumente manuell miteinander verknüpfen. In der Dokumentliste werden dafür mehrere Dateien ausgewählt und anschließend verknüpft. Auf der Detailseite erscheinen diese Dateien im Abschnitt „Verknüpfte und gruppierte Dateien“ mit Vorschau, Titel, Originalname, Datum, Größe, Upload-Weg, Detail-Link und Download.

Manuelle Verknüpfungen eignen sich für Anlagen, Nachweise, Verträge mit Nachträgen oder mehrteilige Vorgänge. Sie verändern die Originaldateien nicht; die Beziehung liegt als BearStack-Metadatum in der Datenbank. Mit Dokument-Bearbeitungsrechten kann eine manuelle Verknüpfung auf der Detailseite wieder aufgehoben werden.

Zusätzlich gibt es automatische Gruppierung über Tags im Gruppenmodus. Alle Dokumente mit demselben Gruppentag werden auf der Detailseite zusammen angezeigt. Gruppierte Dateien sind dort erkennbar, lassen sich aber nicht einzeln als Verknüpfung aufheben, weil die Gruppierung aus dem gemeinsamen Tag entsteht. Um die Gruppierung zu ändern, wird der Gruppentag am Dokument angepasst.

## Suchfavoriten

Suchfavoriten speichern wiederkehrende Suchen als benannte Abkürzungen. Ein Favorit kann Suchtext, Tags, benutzerdefinierte Feldwerte und Zeiträume kombinieren, zum Beispiel „Rechnungen dieses Jahr“ oder „Steuer letzte 30 Tage“. Relative Zeiträume werden beim Aufruf neu berechnet, feste Jahresfilter bleiben stabil.

## Wortwolke

Die Dokument-Wolke ist standardmäßig deaktiviert und kann unter `Einstellungen -> Dokument-Wolke` eingeschaltet werden. Danach erscheint `Wolke` in der Hauptnavigation und kann optional als Startseite gewählt werden.

Die Wolke visualisiert Dokument-Tags nach Häufigkeit. Größere Wörter stehen für häufiger verwendete Tags, jedes Wort führt direkt zur passenden Dokumentliste. Wenn keine primären Tags gesetzt sind, zeigt BearStack eine zentrale Wolke der wichtigsten Tags. Wenn primäre Tags gesetzt sind, entstehen eigene Wolkenbereiche: primäre Tags werden hervorgehoben, und verwandte Tags erscheinen in ihrem Umfeld.

Die Wortwolke bleibt eine Navigations- und Suchhilfe. Sie verändert keine Dateien, Tags oder Dokumente, sondern nutzt vorhandene Tag-Metadaten und die gleiche Dokumentfilterung wie Listen, Suche und virtuelle Ordner.

## Benutzerdefinierte Felder

Benutzerdefinierte Felder ergänzen Dokumente um strukturierte Werte wie Kundennummer, Projekt, Vertragspartner oder Aktenzeichen. Felder können mit Autocomplete arbeiten, in der Filterleiste erscheinen und auf Wunsch Wertordner erzeugen, wenn genügend Dokumente denselben Wert teilen. So entsteht Ordnung über Metadaten, ohne die vorhandene Ablage oder die Dateien selbst umzubauen.

## Virtuelle Ordner

Die Ordneransicht ist virtuell: Ordner entstehen aus Dokument-Tags, Feldwerten und Suchfavoriten, nicht aus einem beschreibbaren Dateisystembaum. Sie werden in der Weboberfläche als eigene Ordneransicht angezeigt und führen von dort direkt zu den passenden Dokumentlisten.

Der WebDAV-kompatible Endpunkt bildet dieselbe virtuelle Struktur ab. `PROPFIND`, `GET`, `HEAD` und `PUT` sind unterstützt; `DELETE`, `MKCOL`, `MOVE`, `COPY`, `LOCK`, `UNLOCK`, `PROPPATCH`, `PATCH` und `POST` werden als read-only abgelehnt. `PUT` importiert neue Dateien in den Zielordner und übernimmt vorhandene Tag-Ordner als Initial-Tags, überschreibt aber keine existierenden Ressourcen.

## Berechtigungen

Dokumentrechte sind capability-basiert und werden über Rollen oder einzelne Permissions vergeben:

| Rolle oder Recht | Wirkung |
| --- | --- |
| `documents_read` | Dokumente lesen, herunterladen, suchen, exportieren und WebDAV lesend nutzen |
| `documents_editor` | zusätzlich hochladen, Metadaten bearbeiten, OCR starten, Dokumente verknüpfen und vorhandene Tags zuweisen |
| `documents_manager` | zusätzlich löschen, wiederherstellen und Struktur-Daten wie Tags, Felder und Suchfavoriten pflegen |
| `api_uploader` | Upload über API, ohne Dokumente lesen zu dürfen |
| `admin` | vollständige Verwaltung |

Für Sonderfälle können einzelne Permissions wie `documents.read`, `documents.webdav.read`, `documents.upload`, `documents.edit`, `documents.delete` und `documents.structure` kombiniert werden. Die vollständige Matrix steht unter [Benutzer und Rechte](benutzer-und-rechte.md).
