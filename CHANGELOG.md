# Changelog

Alle wesentlichen Änderungen an BearStack werden in dieser Datei dokumentiert.

## Unveröffentlicht

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
