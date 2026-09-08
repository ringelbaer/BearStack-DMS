# Changelog

Alle wesentlichen Änderungen an BearStack werden in dieser Datei dokumentiert.

## Unveröffentlicht

### BearStack 0.46.0

- Gruppenbilder ergänzen „Nur Unbenannte anzeigen“: benannte und ignorierte Gesichtsvorschauen werden unmittelbar ausgeblendet, auch nach Bearbeitungen. Die Auswahl bleibt pro Benutzer im Browser gespeichert; Fotoauswahl und Sammelaktion behalten ihre Bedeutung. Ausgeblendete Zoomziele werden zurückgesetzt; leere Ergebnisse erhalten einen Hinweis.
- MINOR für den zusätzlichen Anzeigefilter. Die vorhandenen Kartendaten werden im Browser gefiltert, ohne zusätzliche Server- oder Bildabrufe. Keine API- oder Schemaänderung. README, Website und Browserprüfungen aktualisiert.

### BearStack 0.45.1

- Im Gruppenbildmodus zeigt ein Thumbnail-Klick jetzt einen Ausschnitt mit dreifacher Breite und Höhe der Bounding Box (300 % statt 200 %). Ein erneuter Klick zeigt das vollständige Foto. Bildränder, Tastaturbedienung und Wiederverwendung der geladenen Vorschau bleiben berücksichtigt.
- PATCH für die Anpassung des bestehenden Ausschnitts; keine API- oder Datenmigration. Browser-Geometrieprüfungen, README und Website auf 300 % aktualisiert.

### BearStack 0.45.0

- Android-App **0.7.1** (versionCode 16): kleineres „×“ samt Schaltflächenhintergrund im Personenbereich bei weiterhin mindestens 48 dp Touchfläche. Kleine UI-Korrektur im bereits vorgesehenen Release 0.45.0; API unverändert.

- Android-App **0.7.0** (versionCode 15): Textsuche im Personenbereich mit 250-ms-Eingabepause über alle benannten Personen. Entfernen einer Zuordnung erfordert einen Bestätigungsdialog mit Name und Bildpfad. Personenliste und Portrait-Raster laden beim Scrollen automatisch nach; die Viererseiten im Personenbereich entfallen.
- Additive Labeling-API: `q` für die Personenliste, `limit` (1–40, Standard 4) und indexierter `after_face`-Cursor für Portraits, `named_search` in der Sitzung und `source_revision` in neuen Aktionsquittungen. Suche behandelt Umlaute tolerant und SQL-Platzhalterzeichen wörtlich. Bestehende Offset-Clients und gespeicherte Quittungen bleiben kompatibel; keine Datenmigration. MINOR für die neuen Bedien- und API-Funktionen.
- Verspätete Suchantworten werden verworfen, widersprüchliche Portrait-Seiten nicht vermischt. Bestätigte Änderungen aktualisieren geladene Bilder ohne Rücksprung zum Listenanfang; offene Dialoge und Originalvorschauen unterbrechen das automatische Nachladen. Regressionen für Suchwechsel, Bestätigung/Abbruch, mehr als 80 Portraits, Scrollposition, Cursor, Revisionen und API-Validierung; README, Website und OpenAPI aktualisiert.

- Einzelne unbenannte Gesichtsvorschauen lassen sich im Gruppenbildmodus über „×“ ignorieren. Nur die gewählte Erkennung wird ausgeblendet; andere Gesichter derselben Person bleiben aktiv. Das aktuelle Foto bleibt geöffnet, die ignorierte Vorschau wird markiert und der Zähler aktualisiert. Zoom und bereits geladene Bilder bleiben erhalten.
- Der bestehende Gruppenbild-Endpunkt akzeptiert optional `face_id` mit atomarer Prüfung von Fotorevision, Zugehörigkeit und Bearbeitungsstand. Ungültige Einzelangaben lösen keine Sammelaktion aus. Ohne JavaScript kehrt die Einzelaktion zum gleichen Foto zurück. Nach erfolgreichem Speichern und fehlgeschlagenem Nachladen wird nur der Lesezugriff wiederholt.
- MINOR für die zusätzliche Bedienfunktion und den kompatiblen API-Parameter; keine Datenmigration. Tests decken Einzel- und Sammelaktionen, Mehrfacherkennungen derselben Person, Rechte, Schutzmarker, Konflikte, Rollback, Tastatur, Bildwiederverwendung und Bedienung ohne JavaScript ab. README, Website und OpenAPI aktualisiert.

### BearStack 0.44.0

- Neuer Filter „Nur unbekannte Personen“ unter `/photos/people`: ausschließlich unbenannte Gruppen mit aktiven Gesichtern. Benannte und vollständig ignorierte Gruppen sind ausgeschlossen; Vorschaubild und Fotoanzahl berücksichtigen keine ignorierten Gesichter. Der Filter bleibt beim Blättern, nach Bearbeitungen und über die im Browser gespeicherte Auswahl erhalten. Bekannte und ignorierte Ansichten schließen den neuen Filter in der Bedienung aus.
- Kleiner Button „Alle Filter aufheben“ leert Namenssuche und alle Personenfilter und öffnet Seite 1, auch ohne JavaScript. Mit JavaScript wird dabei auch die gespeicherte Filterauswahl zurückgesetzt.
- Die kompatible API ergänzt `unknown=1` und `unknown_only`; bei widersprüchlichen Parametern gewinnt `unknown=1`. SQL-Filterung und der bestehende Namensindex liefern korrekte Seitenzahlen ohne nachträgliches Aussortieren im Browser. Sichtbarkeitsprüfungen berücksichtigen auch importierte Namen, deren geschützte Quelle die Person unbenannt werden lässt.
- MINOR für den zusätzlichen Filter ohne Datenmigration. Regressionen prüfen aktive/ignorierte Gesichter, Seitenwechsel und Verkleinerung der Ergebnisliste, Schutzmarker, HTTP-Rechte, gespeicherte Filter, Zurücksetzen und Bedienung ohne JavaScript. README, Website und OpenAPI aktualisiert.

### BearStack 0.43.1

- Android-App **0.6.1** (versionCode 14): Originalfotos werden im gemeinsamen 16-MiB-Arbeitsspeichercache für maximal drei Minuten ab erfolgreichem Laden über unterschiedliche Gesichter hinweg wiederverwendet. Cachetreffer verlängern die Frist nicht; Bounding Box und Zoom bleiben separat. Kein Disk-Cache; Verbindungswechsel leert den Cache. Ältere Server verwenden weiterhin getrennte Gesichts-URLs mit derselben Frist.
- Die Labeling-Detailantwort ergänzt `original_key` aus Originalpfad und indexierten Dateimetadaten. Die Abfrage bleibt auf vier Gesichter begrenzt und benötigt weder zusätzliche Dateizugriffe noch Einzelabfragen. PATCH für die Performance-Verbesserung ohne Datenmigration. Tests prüfen echte HTTPS-Anfragezahlen, Bitmap-Wiederverwendung, Ablauf, Verdrängung, Fehler und geänderte Quelldateien; README, Website und OpenAPI ergänzt.

- Personendetails, Personenübersicht und ignorierte Gesichter zeigen bei genau einer Seite nur noch „Seite 1 von 1“. Die überflüssigen Aktionen „Erste Seite“ und „Letzte Seite“ entfallen auch nach AJAX-Aktualisierungen; mehrseitige Ergebnisse behalten ihre Navigation. UI-Korrektur im bestehenden PATCH 0.43.1 ohne API-Änderung.

- Größere Thumbnails im Gruppenbild-Grid: bis zu 200 × 200 Pixel, breitere Kacheln und eine an die Bildschirmbreite angepasste Spaltenzahl. Zoom- und Modalumrandung folgen weiterhin der Bildfläche. Vorhandene Bildabrufe und Vorschaucache werden weiterverwendet; die UI-Anpassung gehört zum bereits vorgesehenen PATCH 0.43.1.

- Im Gruppenbildmodus erhält das Thumbnail des geöffneten Benenn-/Zuordnungsdialogs eine Umrandung. Die Markierung folgt der tatsächlich angeklickten Vorschau, auch bei mehreren Gesichtern derselben Person, und bleibt bei Speicherfehlern erhalten. Abbrechen, Escape und erfolgreiches Speichern entfernen sie wieder; die Zoomauswahl bleibt erhalten.
- PATCH für die ergänzte visuelle Zuordnung ohne zusätzliche Bildabrufe, API-Änderung oder Datenmigration. Bestehende Browsertests um Dialogwechsel, doppelte Personenzuordnungen, Fehler und Schließen erweitert; README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.43.0

- Die Bounding Box im Gruppenbildmodus verwendet eine dünnere gelbe Linie (1 statt 3 Pixel); die dunkle Kontrastkontur bleibt erhalten. Kleine UI-Korrektur innerhalb des bereits vorgesehenen Releases 0.43.0.

- Die Autovervollständigung im Benenn-/Zuordnungsmodal zeigt neben jeder vorhandenen Person deren Thumbnail aus der Personenübersicht. Bild und Name wählen dieselbe Person; Tastaturbedienung und sofortiges Speichern bleiben erhalten. „Neu anlegen“ bleibt ohne Bild, fehlgeschlagene Bilder blockieren die Auswahl nicht. Die Vorschlags-API ergänzt `face_id`, nutzt vorhandene Indizes und bleibt auf 60 Ergebnisse begrenzt. Kleine Bilder werden bedarfsabhängig aus dem bestehenden Cache geladen. Teil des bereits vorgesehenen MINOR-Releases 0.43.0; keine zusätzliche Migration.

- Android-App **0.6.0** (versionCode 13): Neuer Bereich „Menü → Personen“ mit seitenweise geladenen benannten Personen und Portraits. Details erlauben Umbenennen, Zurücksetzen einzelner Gesichter auf unbenannt einschließlich des letzten Gesichts, Favorisieren, Galeriesuche im Browser und Originalfoto-Vorschau per Halten/Wischen sowie TalkBack. Originaldateien bleiben erhalten.
- Additive Labeling-API: Sitzungskapazität `named_people`, `GET /people?after=…&upper=…` und quittierte Aktionen `rename`, `unassign`, `favorite`. Kleine Abfrage-/Sichtbarkeitspakete, keine Gesamtzählung, Revisionsprüfung, atomare Quittungen und inkrementelle Aktualisierung betroffener Personen. Protokoll 1 und das Android-Room-Schema bleiben erhalten; Foto-Schema 24 ergänzt kompatibel einen partiellen Index für benannte Personen. MINOR wegen kompatibler Funktionen.
- Android-Zuordnungswarteschlange bleibt beim Bereichswechsel erhalten. Entfernte Gesichter folgen als unbenannte Gruppen; unklare Schreibaktionen werden nach Verbindungsfehlern über die gespeicherte Aktions-ID geklärt. Regressionen für große Listen, Rechte, Schutzmarker, Rollback, Konflikte, letzte Gesichter, große Schrift und Vorschaugesten; README, Android-Anleitung, Website und OpenAPI erweitert.

- Im Gruppenbildmodus vergrößert ein Klick auf eine Gesichtsvorschau einen Ausschnitt mit doppelter Breite und Höhe der Bounding Box (200 %). Erneuter Klick zeigt das ganze Foto; ein anderes Thumbnail wechselt direkt zum zugehörigen Gesicht. Enter und Leertaste bedienen denselben Wechsel, die aktive Vorschau ist sichtbar und über `aria-pressed` markiert.
- Bildränder, Seitenverhältnis und Größenänderungen bleiben berücksichtigt. Benennen erhält den Zoom im selben Foto, Navigation setzt ihn zurück. Die vorhandene Fotovorschau wird im Browser wiederverwendet, ohne zusätzliche Bildabrufe oder serverseitige Bildberechnung.
- MINOR für die zusätzliche kompatible Web-Bedienfunktion ohne Datenmigration. Browser-Regressionen für Zoom-Geometrie, Fenstergrößen, Tastatur, Gesichtswechsel, Benennen, Navigation und Bildfehler ergänzt; README, Website und OpenAPI-Version aktualisiert.

### Dependency-Prüfung

- Leere Android-Übergangsabhängigkeit `androidx.room:room-ktx:2.8.4` entfernt. Die verwendeten APIs sind bereits in `room-runtime` enthalten; Room-Compiler, Coroutines, Schema und Versionsnummern bleiben unverändert. Keine Laufzeitänderung.
- Go-Module über Importpfade, `go mod tidy -diff` und `go mod why` geprüft; Browser-/Python-Pakete sowie externe OCR-, Vorschau- und Bildwerkzeuge werden weiterhin benötigt. Android-Dokumentation auf die vorhandenen Versionen Gradle 9.6.0 und AGP 9.4.0 korrigiert; Update-Risiken dokumentiert.

### BearStack 0.42.1

- `.adminonly`-Symlinks schützen Fotoordner auch bei fehlenden, zyklischen oder unzugänglichen Zielen. Galerie, Medienzugriff, Batchabfragen und Gesichtslisten verwenden dieselbe Markerprüfung. Lesefehler werden bei Zugriffs- und Startprüfungen weitergegeben; der Startabgleich bricht bei unauflösbaren Indexpfaden vor Schreibzugriffen ab, statt private Einträge öffentlich zu setzen oder Gesichtsdaten zu löschen.
- Automatisch erzeugte TLS-Zertifikate und Schlüssel werden über temporäre Dateien atomar ersetzt. Private Schlüssel erhalten stets `0600`; vorhandene Dateirechte werden nicht übernommen und Symlink-Ziele nicht überschrieben.
- PATCH für Sicherheitskorrekturen ohne Datenmigration. Regressionen reproduzieren defekte Schutzmarker, verweigerte Verzeichniszugriffe, Indexfreigaben und unsichere TLS-Neuerzeugung. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.42.0

- WebDAV liest ausschließlich die für Dateinamen, Auslieferung und HTTP-Metadaten benötigten Dokumentfelder. Zusatzabfragen für Tags, eigene Felder, Duplikate und Verknüpfungen entfallen; Zwischenordner werden ohne Dokumentlisten aufgelöst. Filter, Reihenfolge, Namenskollisionen und HTTP-Metadaten bleiben erhalten.
- Die Personen-Autovervollständigung verwendet `GET /photos/people?format=suggestions&q=...`: höchstens 60 benannte Gruppen mit ID, Name, Fotoanzahl und `has_next`, ohne Gesamtzählung oder Thumbnail-ID. Suchsemantik, Rechte und aktuelle Prüfungen von Schutzmarkierungen und importierten Namen bleiben erhalten; die vollständige Personenübersicht behält ihr bisheriges Format.
- Mitgelieferte PDF.js-Versionsverzeichnisse erhalten auch ohne Versionsparameter ein Jahr Browser-Cache mit `immutable`, einschließlich Worker, Schriften und Codecs. Versionswechsel verwenden neue Pfade; unversionierte Assets behalten ihre bisherigen Cache-Regeln.
- Die Foto-Initialisierung teilt Symlink- und Pfadprüfungen gemeinsamer Vorfahren innerhalb eines Durchlaufs. Ein neuer Start und spätere Dateizugriffe prüfen das Dateisystem erneut; Fehlerbehandlung und der synchrone Sichtbarkeitsabgleich bleiben erhalten.
- MINOR wegen der zusätzlichen kompatiblen API-Darstellung für Vorschläge; die übrigen Änderungen sind Performance-Optimierungen. Keine Datenmigration. Regressionen und Vergleichsbenchmarks ergänzt; README, Website und OpenAPI aktualisiert.

### BearStack 0.41.4

- Die acht Labeling-Endpunkte sind direkt eigenen Handlern zugeordnet. Die zweite Verteilung anhand von Pfad-Endungen und HTTP-Methode entfällt; Zugriffsrechte, Validierung, Fehlercodes, Cache-Header und Bildauslieferung bleiben erhalten.
- Services werden über einen gemeinsamen, durch `sync.Once` geschützten Aufbau erzeugt. Produktionsstart und teilweise aufgebaute Test-Server verwenden dieselben Instanzen und Worker-Kanäle; vorgegebene Services bleiben erhalten. Die Initialisierung startet keine Hintergrundarbeit.
- Favoriten- und Gruppenbildaktionen teilen den Abschluss von Referenzaktualisierung und Revision innerhalb ihrer Transaktion sowie die anschließende Cache-Synchronisierung. Ein Fehler nach erfolgreichem Commit verwirft den Suchcache für den nächsten Aufbau, ohne die gespeicherte Aktion als fehlgeschlagen auszugeben.
- PATCH für interne Refactors ohne Datenmigration oder Änderung der HTTP-Verträge. Regressionen für parallelen Service-Aufbau, Routen, Rollback und Cache-Wiederherstellung ergänzt; README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.41.3

- Navigation und beschriftete Auswahloptionen aus dem Einstellungsservice ausgelagert. Die Startseitenauflösung erhält Modulstatus und Rechte explizit; atomare Speicherung, Cache-Synchronisierung und Worker-Konfiguration bleiben unverändert.
- Regeln für Benutzerverwaltung und Rechteweitergabe im Account-Modul gebündelt. Handler und Darstellung verwenden dieselben Regeln über einen Adapter vom authentifizierten Principal; Session- und Schreibabläufe bleiben bestehen.
- PATCH für interne Refactors ohne Änderung an HTTP-Verträgen, Datenformaten oder Berechtigungen. Regeltests ins Account-Modul verschoben, Grenzfälle und Startseitenmatrix ergänzt; README, Website und OpenAPI-Version aktualisiert.

### Dokumentation

- Native Gesichtserkennung: macOS-Hinweis für fehlende Python-CA-Bundles beim Modell-Download ergänzt. Das vorhandene System-CA-Bundle lässt sich mit `SSL_CERT_FILE` verwenden; Zertifikats- und Modell-Prüfungen bleiben aktiv. Keine Änderung am Anwendungscode oder an der Version.

### BearStack 0.41.2

- Ausschließlich in Tests verwendete Helfer zum Vorbelegen der Einstellungs-Caches nach `test_helpers_test.go` verschoben; die zusätzliche Server-Weiterleitung für den App-Namen entfernt.
- Unbenutzte PDF-Weiterleitungen im Mailimport entfernt. Bestehende Tests rufen die gemeinsamen Anhangsfunktionen direkt auf und tragen deren Namen; Absenderfilter, Größenlimit und sichere Dateinamen bleiben abgedeckt.
- PATCH für konservatives Aufräumen ohne Änderung des Laufzeitverhaltens oder der HTTP-Verträge. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.41.1

- Fehler beim Aktualisieren eines Foto-Fingerprints werden vor Zugriffen auf gespeicherte Gesichter weitergegeben. Gesichtsvorschauen, Favoriten und Gruppenbilder verwenden dieselbe strikte Quellprüfung; fehlgeschlagene Datenbankänderungen erlauben keine alten Gesichtsregionen auf ersetzten Fotos.
- Mehrere Gesichtsausschnitte teilen die decodierte und ausgerichtete 1.600-Pixel-Fotovorschau. Der Arbeitsspeichercache ist auf 32 MiB und acht Bilder begrenzt, bündelt gleichzeitige Anforderungen und prüft Quell-Fingerprint, Sidecars und Schutzmarkierungen auch bei Cachetreffern. Vollauflösende Originale werden nicht im Cache behalten.
- Das Status-Polling der Gesichtserkennung liest mit `format=json&progress=1` ausschließlich Zähler aus dem Index, ohne globale Verzeichnisprüfung oder Dateifehlerliste. Gleichzeitige Aufrufe teilen einen höchstens fünf Sekunden alten Zählerstand. Die vollständige Statusansicht und konkrete Gesichtsabrufe behalten ihre Sichtbarkeitsprüfungen.
- Personen-Dialog, Suche und Stift-Erzeugung liegen im gemeinsamen Modul `app-person-dialog.js`. Gruppenbilder laden dafür nicht mehr das gesamte Personenübersichtsmodul; Benennen, Zuordnen, Fokusführung und Fehlerbehandlung bleiben erhalten.
- Ungenutzte direkte Android-Abhängigkeit `ui-tooling-preview` entfernt; es gibt keine Preview-Annotationen im Projekt. Keine Änderung der Android-Bedienung oder App-Version.
- PATCH für Fehlerkorrektur, Performance und UI-Refactor ohne neue Bedienfunktion oder Datenmigration. Regressionen und Bildaufbereitungsbenchmark ergänzt; README, Website und OpenAPI aktualisiert.

### BearStack 0.41.0

- Neuer Button „Gruppenbilder“ unter Personen für Fotobearbeiter: links das Foto, rechts alle erkannten Gesichtsvorschauen. Hover, Tastaturfokus und Antippen markieren die zugehörige Gesichtsregion; die Darstellung berücksichtigt Seitenverhältnis und EXIF-Ausrichtung.
- Einstellbare Schwelle von 0–255 (Standard 5), pro Nutzer und Browser gespeichert. Nur Fotos mit mehr als dieser Anzahl unbenannter, nicht ignorierter Gesichter gelangen in den Durchlauf. Ein geöffnetes Foto bleibt nach Bearbeitungen auch unterhalb der Schwelle sichtbar.
- Der gemeinsame Stift-Dialog benennt die gesamte Personengruppe oder führt sie mit einer vorhandenen Person zusammen, einschließlich anderer Fotos. „Verbleibende ignorieren“ betrifft dagegen ausschließlich unbenannte, aktive Gesichter des aktuellen Fotos und wechselt direkt zum nächsten Bild. Änderungen am angezeigten Stand führen zu einem Konflikt statt zu unbeabsichtigten Sammeländerungen.
- Überspringen verändert keine Gesichtsdaten und gilt nur für den aktuellen Durchlauf. Neue Durchläufe berücksichtigen übersprungene Fotos erneut. Fehler beim Laden des nächsten Fotos lassen sich ohne Wiederholung einer bereits erfolgreichen Ignorieraktion beheben.
- Foto-Schema 23 ergänzt einen partiellen Auswahlindex; Cursor-Paginierung und gezielte Sichtbarkeitsprüfungen vermeiden wiederholte Gesamtprüfungen. Bestehende Bild-Caches und eine proportionale 1.600-Pixel-Fotovorschau begrenzen Übertragung und Bilderzeugung. MINOR für die kompatible Web-Funktion und automatische Indexmigration; README, Website und OpenAPI aktualisiert.

### Android 0.5.3

- Das automatische Speichern nach Ablauf einer Rückgängig-Frist wartet auf das Schließen des Namens- oder Duplikatdialogs. Dadurch deaktivieren Änderungen und Ausblenden des Toasts nicht mehr dessen Eingabefeld; Fokus, Tastatur und Namensentwurf bleiben erhalten. Die Rücknahmefrist bleibt bei fünf Sekunden.
- Wartende Schreibvorgänge werden weiterhin einzeln für die ursprünglich ignorierten Gruppen ausgeführt. Beim Hintergrundwechsel bleiben noch nicht gesendete Gruppen wiederherstellbar. Regressionen für mehrere Fristen, Texteingabe und Tastatur, Duplikatdialog sowie Hintergrundwechsel ergänzt.
- Android-PATCH auf 0.5.3 (`versionCode` 12); unabhängige Serverversion und API-Vertrag bleiben unverändert. README, Android-Anleitung und Website aktualisiert.

### BearStack 0.40.0

- Vergleichsbilder werden möglichst über verschiedene Galerieordner verteilt. Innerhalb eines Ordners bleiben manuelle Zuordnung und Erkennungssicherheit ausschlaggebend; bereits durch Favoriten vertretene Ordner werden beim Auffüllen berücksichtigt.
- Einzelne Gesichter lassen sich in benannten und unbenannten Personengruppen der Weboberfläche per Stern favorisieren. Alle aktiven Favoriten werden vollständig verglichen, auch oberhalb der eingestellten Referenzanzahl. Weniger Favoriten werden soweit möglich bis zur Zielanzahl ergänzt. Die Markierung speichert ohne Seitenreload und bleibt nach Neustart, Verschieben und Zusammenführen erhalten. Bei erneuter Erkennung wird sie nur auf eindeutig wiedergefundene Regionen übertragen.
- Additive API für spätere App-Unterstützung: `GET`/`PUT /api/photos/labeling/v1/faces/{id}/favorite`, Favoritenstatus in Personendetails und `face_favorites` in der Labeling-Sitzung. Änderungen benötigen `photos.edit`, setzen explizit den gewünschten Zustand und prüfen die erwartete Personengruppe. Die Android-Oberfläche bleibt unverändert.
- Automatische Migration des Fotoindex auf Schema 22 mit Favoritenstatus, Auswahlindex und fortsetzbarer Referenz-Neuauswahl ohne neue Bildanalyse. Favoritenänderung und Referenzen werden atomar gespeichert; ein aktueller Suchindex wird nur für die betroffene Person inkrementell angepasst. Private, ignorierte und entfernte Gesichter bleiben ausgeschlossen; Favoriten verdrängen keine regulären Konkurrenten aus dem HNSW-Index. Die Kandidatenprüfung verwendet SQL-Pakete mit höchstens 512 IDs.
- Regressionen für Auswahl, Migration, Speicherung, Wiedererkennung, Sichtbarkeit, API-Rechte und Browserbedienung sowie ein Benchmark für große Personengruppen ergänzt. MINOR für kompatible Funktionen und Datenmigration; README, OpenAPI und Website aktualisiert.

### BearStack 0.39.3

- Bereits benannte Personen zeigen in der Übersicht weder das Ignorieren-× noch den Stift für den Benenn-Dialog. Die Darstellung folgt dem vorhandenen Namensstatus der Karte und aktualisiert sich auch nach AJAX-Aktionen sofort. Unbenannte Personen behalten beide Symbole.
- Browser-Regressionen für die Sichtbarkeit nach dem Laden, Benennen und Zusammenführen angepasst. PATCH für die UI-Korrektur; README und Website aktualisiert.

### BearStack 0.39.2

- Gesichtsvorschauen behalten im Web und in der Android-App die Proportionen des vollständigen Ausschnitts. Hoch- und Querformate werden zentriert auf dunkelgrauem Hintergrund eingepasst; die Kacheln bleiben quadratisch. Dies gilt für beide Vorschaugrößen und auch für am Bildrand begrenzte Ausschnitte. Die gemeinsame serverseitige Korrektur benötigt kein App-Update.
- Ein neuer Rendering-Schlüssel ersetzt gestreckte Cachebilder beim nächsten Abruf. Nur die konkret ersetzte alte Vorschau wird entfernt; es gibt keinen vollständigen Cache-Neuaufbau. Bestehende Cachefristen und Bereinigung bleiben für nicht aufgerufene Altbilder erhalten. Neu erzeugte Vorschauen ignorierter Gesichter erhalten weiterhin höchstens 48 Stunden. Fehler beim Aufräumen werden erneut versucht, ohne die Anzeige zu blockieren.
- Bildtests für Seitenverhältnisse, Randbeschnitt und beide Größen sowie Regressionen für Cacheersatz, Fehlerversuche, Web-/Android-Endpunkte und quadratische Browser-Kacheln ergänzt. Server-PATCH ohne Datenmigration; README und Website aktualisiert.

### Android 0.5.2

- Der Ladebalken in der Personenbearbeitung erhält einen dauerhaft reservierten Bereich. Ein- und Ausblenden verändern weder die Höhe noch die Abstände der Inhaltsliste; Überschrift, Gesichtsraster und Navigation springen beim Laden nicht mehr vertikal. Im Ruhezustand wird keine Ladeanimation ausgeführt.
- Compose-Regressionstests für Vor- und Zurückblättern sowie gescrollte Inhalte mit großer Schrift ergänzt. Android-PATCH auf 0.5.2 (`versionCode` 11); die unabhängige BearStack-Serverversion bleibt bei 0.39.1. README, Android-Anleitung und Website aktualisiert.

### BearStack 0.39.1

- DOM-Testfixtures der Personenübersicht enthalten wieder die vollständige Auswahlleiste; der JavaScript-Testlauf führt alle Regressionen aus. Browser-Navigationstests erwarten das bestehende Ziel `/settings/general` des Systemmenüs.
- Allgemeine, Dokument- und Fotoeinstellungen werden pro Formular in einer SQLite-Transaktion gespeichert. Zusammengehörige Einstellungen werden mit einer gemeinsamen Abfrage gelesen; Cache-Ladevorgänge, Speicherung und Übernahme der Foto-Worker-Einstellung sind synchronisiert. Fehlgeschlagene Schreibvorgänge erhalten Datenbank, Cache und Laufzeiteinstellung.
- Personenbezogene Sichtbarkeitsprüfungen beschränken sich auf betroffene Gruppen einschließlich der Herkunft importierter Namen. Reine Personenansichten prüfen keine unbeteiligten Auftragsverzeichnisse. Gemeinsame Vorfahren werden je Prüfung einmal geprüft; wartende gleichartige Anfragen teilen eine anschließend gestartete Prüfung. Fertige Ergebnisse werden nicht zwischengespeichert, damit neue `.adminonly`-Markierungen beim nächsten Zugriff wirken. Gesamtübersichten prüfen weiterhin alle relevanten Gesichtsverzeichnisse. Dateisystemfehler blockieren die Abfrage, ohne bei vorübergehend unerreichbaren Verzeichnissen Gesichtsdaten zu löschen.
- Der Gesichtsabgleich lädt bis zu 101 Referenzkandidaten je Suchlauf in einer SQL-Abfrage und prüft gemeinsame Verzeichnisse einmal. Ignorierte, entfernte oder private Referenzen bleiben ausgeschlossen; Datenbankfehler brechen die Verarbeitung ab.
- Gemeinsame MIME-Helfer für Mailimport und EML-Archivierung vereinheitlichen Transferdecoder, Content-Type, Anhangsnamen und Zeichensatzverarbeitung. MIME-Header mit Windows-1252 werden auch beim Mailimport korrekt dekodiert.
- Regressionen für atomaren Rollback, konkurrierende Einstellungen, Sichtbarkeitsumfang, neue Schutzmarkierungen, Namensherkunft und gebündelte Referenzabfragen ergänzt; neuer Verzeichnisbenchmark `BenchmarkFaceVisibility`. PATCH für Fehlerkorrekturen, Performance und interne Vereinheitlichung ohne Datenmigration.

### BearStack 0.39.0

- Vorschauen ignorierter Gesichter werden höchstens 48 Stunden gespeichert und anschließend auch ohne weitere Aufrufe automatisch entfernt. Zugriffe und wiederholtes Ignorieren verlängern bestehende Fristen nicht; bei Bedarf neu erzeugte Vorschauen erhalten erneut höchstens 48 Stunden. Dies gilt für Web- und Android-/Labeling-Aktionen und beide Vorschaugrößen.
- Gesichtsdaten, Embeddings und Wiederherstellung bleiben dauerhaft erhalten. Wiederherstellen hebt die Ablaufzeit auf. Ignorierte Gesichter bleiben vom Abgleich ausgeschlossen, jetzt auch mit ausdrücklicher Filterung bei inkrementellen Aktualisierungen des Referenzgraphen.
- Automatische Migration des Fotoindex auf Schema 21: Ein dauerhafter Ablaufindex ermöglicht die Bereinigung in begrenzten Paketen ohne regelmäßige Verzeichnisscans. Neustarts und gleichzeitige Bilderzeugung/Zuordnungsänderungen werden berücksichtigt. Der bestehende Vorschau-Cache `faces/v1` bleibt erhalten. Seine Zuordnungen werden ohne Bilddekodierung in fortsetzbaren Paketen von höchstens 100 Gesichtern ergänzt; Aufrufe übernehmen vorhandene Vorschauen auch vor Abschluss direkt. Dateinamen, Inhalt und Änderungszeiten bleiben erhalten. Ignorierte Altvorschauen erhalten eine feste Frist ab dem Update, nicht mehr zuordenbare Altdateien bleiben unangetastet. MINOR wegen der kompatiblen automatischen Migration.
- Regressionstests für Fristgrenzen, Wiederherstellung, Web-/Labeling-Ignorieren, Neustart, Bereinigung ohne Aufrufe, Fehlerwiederholung, Erhalt alter Cachedateien, fortsetzbare Migration ohne Originalbildzugriffe und Ausschluss vom Gesichtsabgleich ergänzt.

### BearStack 0.38.1

- Im Benenn-Modal sendet die Auswahl eines Namensvorschlags oder „Neu anlegen“ das Formular direkt per AJAX ab, auch bei Mehrfachauswahl und Tastaturbedienung. Während des Speicherns werden weitere Auswahlen blockiert.

### BearStack 0.38.0

- Autovervollständigungen für Personen zeigen nur benannte Gruppen; die serverseitige Filterung hält Vorschläge auch bei vielen unbenannten Gruppen nutzbar.
- Das Benennen/Zuordnen an Personenkarten erscheint als dezentes Stiftsymbol in der unteren rechten Ecke.
- Markierte Gruppen lassen sich gemeinsam im Dialog benennen oder einer vorhandenen Person zuordnen, ohne Seitenreload. Benennen und Zusammenführen erfolgen atomar; Fehler erhalten die Auswahl. Der Merge-Endpunkt unterstützt dafür optional `new_name`.

### BearStack 0.37.0

- Personen direkt in der Übersicht per „Benennen / zuordnen“ im Dialog bearbeiten: Namensvorschläge, „Neu anlegen“ und Zusammenführen ganzer Gruppen ohne Seitenreload. Filter und Seitenposition bleiben erhalten, Fehler erscheinen im Dialog.
- Der bestehende Benennen-Endpunkt unterstützt zusätzlich JSON-Antworten für AJAX. Berechtigungen und Schutz vor fremden Origins bleiben erhalten.

### BearStack 0.36.1

- Die Personendetailseite kombiniert Benennen und Zusammenführen in einem Namensfeld mit Autovervollständigung und „Neu anlegen“. Freie Namen benennen die aktuelle Gruppe; ausgewählte Personen werden als Ziel zum Zusammenführen übernommen. Der Button zeigt die jeweilige Aktion an.
- Bestehende Endpunkte und Berechtigungen bleiben erhalten; die verzögerte, abbrechbare Suche und Tastaturbedienung werden weiterverwendet.

### Android 0.5.1

- Zoomrichtung der Originalfoto-Vorschau getauscht: Finger nach unten vergrößert zum Gesicht, Finger nach oben verkleinert zurück zum ganzen Foto. Dies gilt beim Halten und in der dauerhaft geöffneten Vorschau.
- Bedienhinweis, Anleitung und Gestentests angepasst. Android-PATCH auf 0.5.1 (`versionCode` 10); Serverversion und API bleiben unverändert.

### Web-UI

- Kontexthilfen im bestehenden `checkbox-help-label`-Muster für Aktivierung, Bilder pro Lauf, Pausen, Referenzen pro Person und Löschbestätigung der Gesichtserkennung ergänzt. Die Hilfen erklären Leistungsfolgen und den Umfang der Aktionen und sind per Tastatur bedienbar.

### BearStack 0.35.0 / Android 0.5.0

- Die Personenbenennungs-API liefert pro Gesicht einer Detailseite ein normalisiertes `bounds`-Rechteck im gemäß EXIF ausgerichteten Originalfoto. Die vier Koordinaten werden mit derselben paginierten Abfrage geladen; zusätzliche Bilddekodierung oder Datei-/Datenbankabfragen entfallen.
- Android zeigt im Originalfoto einen dünnen, auch beim Zoomen gleichbleibenden Rahmen um das ausgewählte Gesicht. Während des Haltens zoomt Hochwischen zum Gesicht; Herunterwischen zoomt zurück. Loslassen, Abbruch und Mehrfingergesten schließen die gehaltene Vorschau. Ignorieren und Überspringen bleiben währenddessen gesperrt.
- Die dauerhaft geöffnete TalkBack-Vorschau bietet entsprechende Zoomaktionen. Das Original wird einmal mit maximal 2048 Pixeln je Seite geladen, die Vergrößerung ist auf 12-fach begrenzt. Ältere Server bleiben ohne Rahmen und Zoom lesbar.
- Server-MINOR auf 0.35.0; Android-MINOR auf 0.5.0 (`versionCode` 9). API-Vertrag, Android-Anleitung und Website aktualisiert.

### Android 0.4.1

- HTTPS-Verbindungen werden beim Kontowechsel, bei abgelehnten Logins und beim Beenden des ViewModels außerhalb des UI-Threads geschlossen. Dadurch verdeckt eine `NetworkOnMainThreadException` beim TLS-Verbindungsabbau nicht mehr die eigentliche Ursache mit „Die Aktion konnte nicht abgeschlossen werden“.
- Fehler bei falschen Zugangsdaten oder fehlenden Personenrechten bleiben erhalten und sind auch im geöffneten Zertifikatsdialog sichtbar. Die Ressourcenfreigabe läuft unabhängig vom bereits beendeten ViewModel zu Ende.
- Login, Profilwechsel zwischen zwei Konten sowie HTTP 401/403 einschließlich Fehleranzeige im Zertifikatsdialog durch einen HTTPS-Emulatortest abgesichert.
- Android-PATCH auf 0.4.1 (`versionCode` 8); Serverversion und API bleiben unverändert.

### Android 0.3.0

- Ignorieren zeigt sofort die nächste Gruppe. „Rückgängig“ erscheint ausschließlich als klickbare Meldung am oberen Bildschirmrand; die Rücknahmefrist sperrt die Bearbeitung nicht mehr.
- Mehrere ignorierte Gruppen erhalten jeweils fünf Sekunden Rücknahmezeit. Die Meldung nimmt die letzte noch rücknehmbare Gruppe zurück. Verzögerte Schreibanfragen behalten ihre ursprüngliche Quellgruppe und werden mit anderen Aktionen serialisiert.
- Aktionsquittungen im Hintergrund verändern die aktuelle Karte nicht. Statistik zählt weiterhin erst bestätigte Serveraktionen. Noch nicht gesendete Gruppen kehren beim Hintergrundwechsel, Profilwechsel oder Neustart in die Warteschlange zurück.
- Room-Schema 3 ergänzt die dauerhafte Vormerkung vorläufig ignorierter Gruppen mit kompatibler Migration. Android-MINOR auf 0.3.0 (`versionCode` 6); Serverversion und API bleiben unverändert.

### Android 0.2.0

- Rechtswischen und die Menüaktion „Letztes Überspringen zurücknehmen“ holen zuletzt übersprungene Gruppen zurück, auch mehrfach und nach Durchgangsende. Aktuelle Gruppen und Bildseiten bleiben zur weiteren Bearbeitung erhalten.
- Gruppen werden vor dem Zurückholen erneut geprüft. Netzwerkfehler erhalten die aktuelle Ansicht und Rücknahmemöglichkeit; inzwischen bearbeitete oder entfernte Gruppen werden ausgelassen. Offene Serveraktionen sperren die Rücknahme.
- Rücknahmen korrigieren die Überspringen-Statistik atomar mit der Warteschlange. Verlauf und vorgemerkte Bildseiten werden pro Instanz, Datenbestand und Konto gespeichert. Ein neuer Durchgang leert den Rücknahmeverlauf.
- Kompatible Room-Migration von Schema 1 auf 2 erhält Warteschlange, Statistik und offene Aktionen. Android-MINOR auf 0.2.0 (`versionCode` 5); BearStack-Serverversion und API bleiben unverändert.

### Android 0.1.3

- Wischen nach links und oben funktioniert auf der gesamten Inhaltsfläche der Personenbearbeitung, einschließlich Hintergrund und Abständen. Die Wischerkennung liegt zentral außerhalb des Grids; eine Geste löst höchstens eine Aktion aus.
- Bei überlangem Inhalt scrollt Hochwischen zunächst bis zum Ende. Erst ein weiterer Wischer ignoriert die Gruppe. Haltevorschau, Namensdialog, Menüs, Statistik und laufende Aktionen sperren Bearbeitungsgesten.
- Android-PATCH auf 0.1.3 (`versionCode` 4); Serverversion und API bleiben für diese UI-Korrektur unverändert.

### Android 0.1.1

- Verbindungsfehler nennen jetzt Phase und Ursache (DNS, Erreichbarkeit, Timeout, Netzwerkberechtigung oder Zertifikat), statt beim ersten Verbindungsaufbau eine offene Aktion anzudeuten. Debug-Logs enthalten nur Diagnosecodes und keine Zugangsdaten, URLs oder Exception-Texte.
- Gemeinsame AndroidX-Future-Version für App und instrumentierte Tests festgelegt, damit die Tests auch mit AGP 9.4 ohne Versionskonflikt auflösen.
- Die Android-Version steigt unabhängig auf 0.1.1 (`versionCode` 2); Serverversion und API bleiben unverändert.

### Dokumentation

- OpenAPI-Versionsangabe auf den bestehenden Release 0.24.2 korrigiert und den erforderlichen Abgleich mit `VERSION` dokumentiert.
- Schrittweise Einrichtung der Gesichtserkennung ohne Compose ergänzt: Python-Umgebung, Modellinstallation, Token, Dienstprüfung und native BearStack-Konfiguration.

## 0.36.0 - 2026-09-08

### Neu

- „Fotos bearbeiten“ (`photos_editor` / `photos.edit`) reicht für Personenbenennung, Zuordnung, Zusammenführung, Ignorieren und Wiederherstellung sowie alle Endpunkte der Android-/Labeling-API. Die Web-Oberfläche zeigt die passenden Bearbeitungsaktionen auch Fotobearbeitern.
- Einstellungen und Steuerung der Gesichtserkennung sowie das Löschen aller Gesichtsdaten bleiben an `photos.manage` gebunden. Fotoleser dürfen Personen weiterhin nur ansehen; geschützte Fotos bleiben ausgeschlossen.
- Das API-Feld `can_manage` bleibt für bestehende Apps kompatibel und signalisiert die erlaubte Personenbearbeitung. Ein App-Update ist für die Rechtefreigabe nicht nötig.

### Tests

- App-Sitzung, Benennung, Vorschaubilder, Aktionsquittungen und Web-Ignorieren mit Fotobearbeiter-Rechten geprüft; Verwaltungsaktionen und Datenlöschung bleiben gesperrt.

## 0.35.1 - 2026-09-08

### Sicherheit

- „Gesichtsdaten löschen“ verlangt zusätzlich zur Löschbestätigung das aktuelle Passwort des angemeldeten Nutzers im bestehenden Passwortdialog. Die serverseitige Prüfung samt Fehlversuchsdrosselung erfolgt vor dem Abschalten der Verarbeitung und dem Löschen der Daten.
- Fehlendes oder falsches Passwort sowie fehlende Löschbestätigung und unzureichende Rechte lassen Daten und Verarbeitung unverändert. Ablehnung, Passwortdialog und erfolgreiche Löschung sind durch HTTP-Tests abgesichert.

## 0.34.0 - 2026-09-07

### Hinzugefügt

- Personenbenennungs-API liefert für die vier Gesichter einer Detailseite `display_path`: vollständiger Galeriepfad mit allen Ordnerebenen und Dateiname. Ordnernamen und Datumspräfixe folgen den bestehenden Galerie-Breadcrumb-Regeln. Die Pfade werden ohne zusätzliche Datei- oder Datenbankabfragen aufbereitet; absolute Host-Pfade werden nicht ausgegeben.
- Android 0.4.0 (`versionCode` 7) zeigt diesen Pfad unter jedem Gesichtsausschnitt und unter der Originalfoto-Vorschau. Lange Pfade werden umgebrochen und nicht mit Auslassungszeichen gekürzt. Die Anzeige benötigt BearStack 0.34.0.

- Einstellbare Referenzanzahl pro Person (1–100, Standard 30) unter Einstellungen → Gesichtserkennung. Vorhandene Gesichter werden ohne erneute Bildanalyse vor der nächsten Analyse als Referenzen ausgewählt; manuelle Zuordnungen bleiben bevorzugt.
- Foto-Schema 20 speichert Referenzlimit und Fortschritt des stückweisen Referenz-Neuaufbaus. Abbruch und Neustart setzen die Aktualisierung fort. Die Kandidatensuche berücksichtigt das Referenzlimit, damit viele Referenzen derselben Person den Abstand zur zweitbesten Person nicht verdecken.

### Behoben

- Die Auswahlfelder einzelner Gesichter erhalten wieder einen zugänglichen Namen für Screenreader und Tastaturtests.

## 0.33.0 - 2026-09-07

### Hinzugefügt

- Die Personenbenennungs-API liefert über `GET /api/photos/labeling/v1/faces/{id}/original` das unveränderte Quellfoto eines Gesichts, mit Personenverwaltungsrechten, Ausschluss geschützter und ignorierter Gesichter, `no-store` und Unterstützung für Bereichsanfragen. Vorhandene Ausschnitt-Endpunkte bleiben kompatibel.

### Android 0.1.2

- Halten eines Gesichtsausschnitts zeigt das vollständige Originalfoto im ursprünglichen Seitenverhältnis. Loslassen oder Berührungsabbruch schließt die Vorschau. TalkBack bietet „Originalfoto anzeigen“ an.
- Originalfotos werden erst beim Halten geladen; Ladeanzeige und Fehlermeldung geben Rückmeldung. Das Grid verwendet weiterhin kleine Gesichtsausschnitte.
- `versionCode` steigt auf 3; die Originalfoto-Vorschau benötigt BearStack 0.33.0.

## 0.32.2 - 2026-09-07

### Behoben

- Die Option „BearStack PDF-Vorschau verwenden“ zeigt Checkbox und Beschriftung in Konto- und Benutzerverwaltung nebeneinander. Der Text verwendet normale Schreibweise und bricht auf schmalen Bildschirmen neben der Checkbox um.

## 0.32.1 - 2026-09-07

### Behoben

- Beim Zusammenführen per Checkbox gewinnt eine benannte Gruppe gegenüber unbenannten Gruppen unabhängig von der Auswahlreihenfolge. Zielanzeige und AJAX-Anfrage verwenden dieselbe Priorität. Bei mehreren benannten Gruppen bleibt die zuerst ausgewählte benannte Gruppe erhalten.
- Der Server übernimmt bei einem unbenannten Ziel den ersten vorhandenen Quellnamen samt Herkunft und manueller Namensmarkierung innerhalb derselben Transaktion. Benannte Ziele behalten ihren Namen.

### Tests

- Auswahlreihenfolge, Zielanzeige und Merge-Anfrage sowie unbenannte Ziele, benannte zusätzliche Quellen, mehrere Namen und vollständig unbenannte Gruppen abgesichert.

## 0.32.0 - 2026-09-07

### Neu

- Die Web-Personenübersicht merkt sich Seite und Filter im Browser pro Benutzer. Direkte Seitenaufrufe und neue Suchfilter haben Vorrang; gesperrter Browser-Speicher beeinträchtigt die normale Bedienung nicht.
- „Erste Seite“ und „Letzte Seite“ sowie Gesamtseitenzahlen für Personenübersicht, Personendetails und ignorierte Gesichter, auch nach AJAX-Aktionen. Nicht mehr vorhandene Seiten werden auf die letzte gültige Seite begrenzt.
- Die JSON-Personenansicht liefert `total_pages`; Datenbankzählungen berücksichtigen die jeweiligen Filter und zählen Gruppen beziehungsweise Gesichter passend zur Ansicht.

### Tests

- Browser-Seitenwiederherstellung, ausdrückliche Filter, ungültige Speicherwerte, AJAX-Randnavigation, gefilterte Gesamtzahlen, Personendetails und veraltete Seitennummern geprüft.

## 0.31.0 - 2026-09-07

### Neu

- Filter „Ignorierte Gesichter“ in der Personenübersicht, kombinierbar mit Namenssuche und „Nur bekannte Personen“. Zeigt einzelne ignorierte Gesichter auch aus Gruppen ohne aktive Gesichter, mit bis zu 60 Einträgen pro Seite und bestehenden Cache-Vorschauen.
- Fotoverwalter können ignorierte Gesichter direkt benennen und als neue benannte Person wiederherstellen. Andere Gesichter der bisherigen Gruppe bleiben unverändert; Filter und Seite bleiben nach dem Speichern erhalten.
- Partieller Datenbankindex für ignorierte Gesichter; die neue JSON-Filteransicht liefert `ignored_only` und `faces`.

### Tests

- Filter, Namenssuche, Seitengrenzen, vollständig ignorierte Gruppen, Wiederherstellung samt Referenzbild, Schutz privater Fotos, HTTP-Rechte und Formularrückleitung abgesichert.

## 0.30.2 - 2026-09-07

### Behoben

- Gesichtsvorschauen werden im Foto-Cache unter `faces/v1` wiederverwendet statt bei jedem Abruf aus dem Original dekodiert. Der persistente Cache hat keine Größen- oder Anzahlbegrenzung, verwendet hashbasierte Unterverzeichnisse ohne Verzeichnisscans beim Abruf und bündelt parallele Erzeugungen desselben Ausschnitts.
- Web-Gesichtsbilder unterstützen private Browser-Revalidierung mit ETag/304 nach erneuter Zugriffs- und Sichtbarkeitsprüfung. Die Personenübersicht behält bestehende Bild-Elemente auch bei geänderten Namen oder Fotoanzahlen.

### Tests

- Cache-Wiederverwendung über Neustarts, parallele Abrufe, fehlende Cache-Dateien, Quelländerungen, geschützte Fotos, HTTP-Revalidierung und DOM-Wiederverwendung abgesichert.
- HTTP-Integrationstest weist für Web- und Labeling-Endpunkte (160/640 Pixel einschließlich Standardgröße) die Auslieferung vorhandener Cache-Dateien ohne Überschreiben, ihre Wiederverwendung nach Library-Neustart und die Neuerzeugung fehlender Vorschauen nach.

## 0.30.1 - 2026-09-07

### Behoben

- In der Personenübersicht entfällt „Auswählen“ neben den Checkboxen, auch nach AJAX-Aktualisierungen. Die personenbezogene Screenreader-Beschriftung bleibt erhalten.

## 0.30.0 - 2026-09-07

### Neu

- Native Android-Personen-App unter `apps/android` (eigene Version 0.1.0): Vierergrid, Haltevorschau, Namensvorschläge, Zuordnung, Einzelgesicht-Abtrennung, Ignorieren mit Rücknahmefrist und lokale Durchgänge für übersprungene Gruppen.
- Versionierte JSON-API unter `/api/photos/labeling/v1` mit cursorbasierten Kandidaten, vollständigen Gruppenaktionen, Revisionsprüfung und atomar gespeicherten, kontogebundenen Aktionsquittungen. Web- und Hintergrundänderungen erhöhen dieselben Revisionen; geschützte Fotos bleiben ausgeschlossen.
- Lokale Room-Warteschlange und Statistik nach Konto und Datenbestand; Wiederherstellung ungeklärter Aktionen ohne Doppelzählung. HTTPS mit Zertifikatsabgleich vor Anmeldung, Keystore-geschützte Zugangsdaten und begrenzter Bildcache.
- Separater Gradle-Prüfeinstieg, Emulator- und Go-HTTPS-Integrationstests sowie signierbarer privater APK-Build. Go- und Docker-Builds benötigen kein Android-SDK.

### Tests

- Veraltete Menütrennlinien-Erwartungen an die bereits vorhandene Platzierung von API und Log im Footer angepasst.

## 0.29.0 - 2026-09-07

### Neu

- „Zielperson suchen“ vervollständigt Namen beim Zusammenführen und Verschieben direkt im Eingabefeld mit AJAX-Vorschlägen statt einer separaten Auswahlliste. Vorschläge zeigen Namen, IDs und Fotoanzahl und lassen sich mit Maus oder Tastatur auswählen. Beim Zusammenführen wird die aktuelle Person ausgeschlossen.
- Die Suche startet bei Bedarf, verzögert Anfragen bei Eingaben und verwirft überholte Antworten. Freitext kann keine alte Zielauswahl absenden; beim Verschieben bleibt ein leeres Feld die Auswahl für eine neue Person.

## 0.28.1 - 2026-09-07

### Behoben

- Auf Personenseiten brechen lange Foto-Dateipfade innerhalb der Karten um, ohne Nachbarkarten zu überlagern. Vorschaubilder bleiben auch in schmalen Karten quadratisch; beim Überfahren entfällt die dunkle Button-Fläche und Tastaturfokus bleibt sichtbar.

## 0.28.0 - 2026-09-07

### Neu

- Mehrfachauswahl per Checkbox in der Personenübersicht mit unten rechts fixiertem Merge-Button ab zwei ausgewählten Gruppen. AJAX führt die Auswahl atomar in die zuerst ausgewählte Person zusammen und aktualisiert die Übersicht. Fehler werden angezeigt und parallele Aktionen gesperrt.
- Der Merge-Endpunkt akzeptiert zusätzliche Quellpersonen als wiederholtes `person_id` (maximal 60 verschiedene Quellen) und liefert für JSON-Anfragen eine Erfolgsantwort. Klassische Formulare bleiben kompatibel.

## 0.27.0 - 2026-09-07

### Neu

- Die Personenübersicht bietet „Nur bekannte Personen“ für Gruppen mit vergebenem Namen. Der Filter lässt sich mit der Namenssuche kombinieren und bleibt beim Blättern sowie beim Ajax-Ignorieren erhalten; die Datenbank filtert vor der Paginierung.

## 0.26.2 - 2026-09-07

### Behoben

- Die Personenformulare verwenden ein eigenes responsives Raster: Such- und Auswahlfelder sowie Aktionen stehen mobil untereinander. Lange Beschriftungen und Buttons werden nicht mehr in schmale Spalten gedrückt; doppelte Kartenrahmen entfallen.

## 0.26.1 - 2026-09-07

### Behoben

- Im Fotoinfo-Panel trennt ein Mittelpunkt („·“) die verlinkten Personennamen.

## 0.26.0 - 2026-09-07

### Neu

- Fotoverwalter können in der Personenübersicht das angezeigte Gesicht mit „×“ per Ajax ignorieren. Vorschaubild, Fotoanzahl und Paginierung aktualisieren sich ohne Seitenreload; weitere Gesichter derselben Gruppe bleiben erhalten. Fehler werden direkt angezeigt, Mehrfachklicks während einer Aktion gesperrt.
- Der bestehende Gesichts-Bearbeitungsendpunkt liefert für `Accept: application/json` eine JSON-Erfolgsantwort; klassische Formulare behalten ihre Weiterleitung.

## 0.25.13 - 2026-09-07

### Behoben

- Personenübersicht und Personenseiten zeigen die Paginierung mit gestalteten Zurück-/Weiter-Buttons, klaren Abständen und hervorgehobener Seitenzahl; auf schmalen Bildschirmen kann die Navigation umbrechen.

## 0.25.12 - 2026-09-07

### Behoben

- Die Kopfaktionen „Fotos“, „Alle Personen“ und „Gesichtserkennung“ verwenden auf Personenseiten die vorhandene Button-Gestaltung statt einer nicht definierten CSS-Klasse.

## 0.25.11 - 2026-09-07

### Behoben

- Fotos auf Personenseiten öffnen über Gesichtsbild oder Dateiname die gemeinsame Foto-Lightbox mit Navigation, Zoom und Bildinformationen. Die Gesichtsauswahl bleibt unabhängig bedienbar; Fotodetails werden erst beim Öffnen nachgeladen.

## 0.25.10 - 2026-09-07

### Behoben

- Der Einstellungen-Einstieg im Systemmenü und die Startseiten-Fallbacks für Systemverwalter öffnen „Allgemein“ (`/settings/general`).

## 0.25.9 - 2026-09-07

### Geändert

- Die Ähnlichkeitssuche für Feldwerte begrenzt die Distanzberechnung auf die bestehenden Schwellwerte, verwirft unpassende Längen früh und verwendet Unicode-Daten und Arbeitsspeicher mehrfach. Vorschläge und Normalisierung bleiben unverändert.
- Einfache Tagabfragen laden Definitionen ohne Dokumentzählungen; die Berechtigungsprüfung fragt nur übergebene Tagnamen ab. Ansichten und API-Endpunkte mit sichtbaren Nutzungszahlen behalten die Zählungen.
- Die Fotokarte misst ihre Abmessungen und projiziert ihren Mittelpunkt einmal je Renderdurchlauf. Track-Polylinien werden beim Verschieben und Zoomen wiederverwendet.
- Bereits geladene Lightbox-Medien werden pro Auswahl einmal dargestellt; auch das Vorladen benachbarter Medien wird nur einmal angestoßen.
- Die Foto-Startprüfung gleicht `.adminonly`-Sichtbarkeit in begrenzten SQL-Paketen ab und überspringt Updates für unveränderte Ordner. Änderungen an Medien, Blogs und Ordnern bleiben transaktional und werden vor Freigabe der Library abgeschlossen.

### Tests

- Begrenzte Unicode-Distanzberechnung gegen eine vollständige Referenz geprüft und einen Feldwert-Benchmark ergänzt. Tag-Metadatenabfragen ohne Zuordnungstabelle sowie Batchgrenzen abgesichert.
- Foto-Startprüfung auf Abfrageanzahl, Sichtbarkeitswechsel, Vererbung und Rollback geprüft. Browser-Tests sichern einmaliges Lightbox-Rendering und konstante Größenabfragen bei einem Track mit 10.000 Punkten ab.

## 0.25.8 - 2026-09-07

### Geändert

- Die Foto-Lightbox liegt im eigenen Browsermodul `app-photos-lightbox.js`. Die Galerie übergibt Bearbeitungsmodus, gebündeltes Metadaten-Nachladen und Kartenhelfer explizit. Zoom, Touch-Gesten, Vollbild, Diashow und Medienwiedergabe behalten ihr bisheriges Verhalten; Initialisierung und Ladefolge bleiben zentral gesteuert.
- API und Log stehen abhängig von den Benutzerrechten im Footer neben der Versionsnummer. Einstellungen, Konto und Logout bilden eine gemeinsame Icon-Zeile im Systemmenü mit zugänglichen Beschriftungen und Tooltips; die Navigation funktioniert auch ohne JavaScript.

### Tests

- OCR-Ausführung mit kontrollierten Werkzeug-Fixtures abgesichert: Seitenreihenfolge, Fallback-Limit, Fehler, Abbruch und temporäre Dateien. Traversal- und Symlink-Tests für Dateisystemoperationen ergänzt.
- Verspätete Lightbox-Metadaten und Vorschaubilder, fehlgeschlagene Abrufe, optionale Kartenhelfer sowie Tastaturbedienung und weitere Berechtigungskombinationen der Navigation geprüft.

## 0.25.7 - 2026-09-07

### Geändert

- Der eigene Einstellungsreiter „Allgemein“ bündelt Anwendungsname, Design, Startseite, Tag-Darstellung und Favicon. Dokumentbezogene Optionen bleiben unter „Dokumente“; beide Formulare speichern nur ihren jeweiligen Bereich. Favicon-Aktionen führen zurück zu „Allgemein“.

### Behoben

- Einstellungsformulare passen sich auch schmalen Mobilgeräten an. Reiter umbrechen, statt seitlich abgeschnitten zu werden; Formulargruppen, Designauswahl, lange Beschriftungen und Worker-Aktionen bleiben innerhalb der verfügbaren Breite.

## 0.25.6 - 2026-09-07

### Geändert

- Fotoeinstellungen verwenden eine gemeinsame Normalisierung für Wertebereiche. Formularauswertung ist vom Einstellungsservice getrennt; Datenbank-Defaults, Checkbox-Verhalten und gespeicherte Schlüssel bleiben erhalten.
- Gemeinsame JavaScript-Helfer für Fotometadaten und die Auswahl der Bildauflösung liegen in einem eigenen Medienmodul. Galerie und Fotoframe nutzen dieselbe Implementierung; der Fotoframe benötigt dafür das Galerie-Script nicht mehr.
- Dokumentlisten verwenden für HTML und JSON einen gemeinsamen Abfrage-Service ohne HTTP- oder Darstellungstypen. Redirects, Pagination-Links und Sortierlinks entstehen in der Darstellungsschicht. HTML-Seitenkorrekturen benötigen weiterhin nur die Zählung; JSON-Abfragen laden keine zusätzlichen OCR-Daten.

## 0.25.5 - 2026-09-07

### Sicherheit

- Der Betrieb ohne Auth prüft zusätzlich zur Listener-Adresse die tatsächliche Gegenstelle und den HTTP-Host jeder Anfrage. Fremde Hostnamen werden mit `403` abgewiesen, damit DNS-Rebinding keinen Zugriff auf die lokale Dokument- und Fotoverwaltung eröffnet; Forwarded-Header umgehen die Prüfung nicht.
- `golang.org/x/image` auf `v0.45.0` aktualisiert: behebt bekannte WebP-Decoderfehler mit übermäßigem Speicherverbrauch und Abstürzen bei widersprüchlichen Bilddimensionen. Regressionstests decken gültige und präparierte WebP-Dateien in der Gesichtsvorverarbeitung ab.
- Mindestversion der Go-Toolchain auf `1.26.6` angehoben, um relevante Sicherheitskorrekturen der Standardbibliothek beim Build sicherzustellen. Die von `x/image` benötigten Versionen von `x/sys` und `x/text` wurden mitgezogen.

## 0.25.4 - 2026-09-07

### Geändert

- Konservative Bereinigung ohne Änderung des Anwendungsverhaltens: ausschließlich von Tests verwendeten Foto-Schreibhelfer in eine `_test.go`-Datei verschoben und Dokument-URLs auf den bestehenden Query-Helfer umgestellt.
- Exklusiv verwendete Sperren für GPX-Cache und Hintergrundjob-Kontext auf `sync.Mutex` vereinfacht.
- GPX-Benchmarks setzen beim Leeren des Caches auch LRU-Verwaltung und Speicherzähler zurück.

## 0.25.3 - 2026-09-07

### Behoben

- Gültige Foto-Thumbnail-Cachetreffer benötigen keinen SQLite-Schreibaufruf mehr. Fehlende oder unvollständige Queue-Metadaten werden nur bei Bedarf repariert.
- Beim Herunterfahren werden Hintergrundworker sowie manuell gestartete Foto- und Gesichtsjobs abgebrochen und abgewartet. Neue Jobs werden abgewiesen; Datenbanken schließen erst nach Abschluss. Nach insgesamt 60 Sekunden wird ein nicht abgeschlossener Shutdown als Fehler beendet.
- `update.sh` prüft nach dem blockierenden Dienststopp dessen Ergebnis und Prozesszustand. Bei Timeout, Fehler oder verbleibender Hauptprozess-ID wird kein neues Binary installiert. Die systemd-Vorlage erlaubt 75 Sekunden für das Beenden.

### Geändert

- GPX-Verarbeitung ist abbrechbar und auf 16 MiB sowie 100.000 eingelesene Punkte pro Datei begrenzt. Parser laufen je Foto-Library einzeln; Kartenantworten enthalten höchstens 256 Tracks und 250.000 Punkte. Zu große Tracks werden übersprungen.
- Der GPX-Cache verwendet ein Speicherbudget von 32 MiB einschließlich Punktarray-Kapazität und Eintragskosten und verdrängt die am längsten ungenutzten Einträge.
- Unbenutzten Dateisystem-Helfer entfernt; reine Testzugriffe aus dem Produktionscode ausgelagert beziehungsweise auf produktive Einstiegspunkte umgestellt.
- Playwright-Suiten teilen sich Serverstart, Gesundheitsprüfung und vollständige Prozessbeendigung. Ein Testbinary wird pro Lauf einmal gebaut; die bestehende Testabhängigkeit ist über npm-Lockfile fest versioniert.

## 0.25.2 - 2026-09-07

### Behoben

- Vorübergehende Stat- und Blog-Lesefehler verwerfen den betroffenen Ordnerscan vor dem Schreiben. Bestehende Indexeinträge, manuelle Tags und Scan-Signaturen bleiben erhalten; ein späterer vollständiger Scan kann weiterhin echte Löschungen erkennen.
- Dokument-Bildthumbnails prüfen die Dimensionen vor dem Dekodieren und begrenzen die Eingabe auf 40 Megapixel. Ungültige oder zu große Bilder ersetzen keine vorhandene Vorschau.
- Indexbereinigung löscht zusammengehörige Metadaten, Tagzuordnungen, Such- und Vorschauindizes in einer Transaktion je Löschpaket beziehungsweise Ordnerteilbaum. Fehler und Abbrüche hinterlassen keine teilweise gelöschten Pakete.

### Geändert

- Foto-Info-Batchanfragen bündeln Metadaten- und Gesichtsabfragen, teilen sich Ordner-Rechteprüfungen und speichern geänderte Medien gemeinsam. Dateistand, XMP-Sidecars, manuelle Tags und aktuelle Zugriffsbeschränkungen bleiben berücksichtigt.

## 0.25.1 - 2026-09-07

### Behoben

- Foto-Tags, Tagzuordnungen und Volltextindex werden gemeinsam in einer Transaktion gespeichert, auch bei Umbenennungen und beim Entfernen von Tags. Schreibfehler rollen die gesamte Änderung zurück.
- Gleichzeitige Bulk-Tagaktionen lesen den aktuellen Stand innerhalb der Schreibtransaktion. Hinzufügen und Entfernen überschreiben keine parallelen Änderungen; fehlgeschlagene Aktionen hinterlassen keine teilweise bearbeitete Auswahl. Laufende Indexscans tragen inzwischen entfernte Tags nicht wieder ein.
- Endgültige Dateibereinigung nutzt ausschließlich die abbrechbare Sperre je Dokument. Eine laufende Vorschauerzeugung blockiert dadurch keine parallel angeforderte Löschung anderer Dokumente.
- Parallele Aufrufe der Dokumentstatistik teilen sich eine Berechnung. Eine laufende Berechnung kann eine zwischenzeitliche Cache-Invalidierung nicht mehr überschreiben.

### Geändert

- Tag-Umbenennungen und Tag-Löschungen lesen über vorhandene Tagindizes nur betroffene Medien, Ordner und Blogs statt sämtliche getaggten Datensätze.

## 0.25.0 - 2026-09-07

### Behoben

- Endgültiges Löschen legt persistente Dateibereinigungsaufträge in derselben Transaktion wie die Metadatenlöschung an. Quarantäne, reservierte Dateinamen und Wiederholung beim Start sowie minütlich machen die Bereinigung nach Fehlern und Neustarts fortsetzbar. Laufende Vorschauerzeugung und Bereinigung werden pro Dokument koordiniert.
- PDF-Thumbnails werden vollständig geprüft und atomar veröffentlicht; abgebrochene Renderer hinterlassen keine unvollständige veröffentlichte Vorschau.
- Fehlerhafte Dokumente blockieren spätere Batches des Thumbnail-Nachholprozesses nicht mehr.

### Geändert

- Automatische Migration der Dokumentdatenbank auf Schema 17 für persistente Löschaufträge.
- Foto-Cache-Statistiken werden beim Start und alle 30 Minuten im Hintergrund erhoben. Die Statistikseite zeigt den Messzeitpunkt und benötigt keinen vollständigen Dateisystemdurchlauf; parallele Berechnungen werden zusammengeführt.
- Ungenutzte Konto-Präferenz-Löschmethode und ausschließlich von Tests verwendeter einzelner Thumbnail-Helfer entfernt; der Regressionstest nutzt die produktive Batch-Funktion.
- Lokale Python-Umgebungen, Tool-Caches und Website-Verzeichnisse aus dem Docker-Buildkontext ausgeschlossen.

## 0.24.3 - 2026-09-06

### Behoben

- Die Einstellungen der Gesichtserkennung verwenden das gemeinsame Einstellungslayout mit seitlicher Navigation auf Desktop-Geräten und horizontalen Reitern auf kleinen Bildschirmen. Statuszahlen, Eingabefelder und Aktionen sind responsiv angeordnet; die Löschaktion ist räumlich getrennt.

## 0.24.2 - 2026-09-06

### Behoben

- Die Foto-Info erscheint auf kleinen Bildschirmen unter dem Foto über die volle Breite, statt es durch eine seitliche Spalte zusammenzudrücken. Das Panel bleibt separat scrollbar und sein Schließen-Knopf sichtbar.

## 0.24.1 - 2026-09-06

### Behoben

- Die Foto-Lightbox stoppt am ersten und letzten Medium, statt zum anderen Ende zu springen. Nicht verfügbare Navigationspfeile sind deaktiviert; die Diashow endet beim letzten Medium. Auch das Vorladen respektiert diese Grenzen.

## 0.24.0 - 2026-09-06

### Hinzugefügt

- Optionale lokale Gesichtserkennung mit CPU-Dienst, fest geprüften YuNet-/SFace-Modellen und gedrosselter, persistenter Hintergrundwarteschlange.
- Personenansicht mit Benennung, Zusammenführen, Verschieben und Ignorieren von Gesichtern für Foto-Verwalter; Personensuche und Fotoinfo berücksichtigen automatische Zuordnungen.
- XMP-Gesichtsregionen als Namensvorgaben, dauerhafte manuelle Korrekturen, Rechteprüfung für Gesichtsvorschauen und Ausschluss von `.adminonly`-Fotos.
- Einstellungen für Fortschritt, Pause, Fortsetzung, Fehlerwiederholung und bestätigtes Löschen erzeugter Gesichtsdaten; Compose-Profil `faces` und optionale Dienstkonfiguration.

### Geändert

- Foto-Datenbankschema 18 ergänzt automatische Gesichter, Personen und Aufträge ohne Änderungen an Originalbildern oder XMP-Sidecars.
- Personensuchen nutzen indizierte Zuordnungen; der inkrementelle HNSW-Index hält höchstens fünf Referenzen pro Person.

## 0.23.4 - 2026-09-06

### Behoben

- Beim Zurückkehren aus einem Foto-Unterordner wird die vorherige Scrollposition wiederhergestellt, sowohl über den Fotopfad als auch über Browser-Zurück/Vorwärts. Pfadlinks übernehmen die zuvor besuchte Sortierung, Filter und Seite; bei geänderter Fensterbreite bleibt der betretene Ordner an seiner bisherigen Bildschirmposition.

## 0.23.3 - 2026-09-06

### Geändert

- Foto-Ordnerthumbnails zeigen Bilder an den Positionen 20 %, 40 %, 60 % und 80 % in absteigender Datumsreihenfolge. Kleine Ordner zeigen jedes Bild höchstens einmal; Ordner ohne Bilder behalten Medienvorschauen.
- Bestehende Ordner-Vorschauzuordnungen werden automatisch ersetzt. Die Auswahl berücksichtigt die sichtbaren Bilder einschließlich Unterordnern und wird für Ansichten mit und ohne Admin-only-Inhalte getrennt zwischengespeichert.

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
