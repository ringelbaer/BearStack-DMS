# Changelog

Alle wesentlichen Änderungen an BearStack werden in dieser Datei dokumentiert.

## Unveröffentlicht

### BearStack 1.2.0 / Android 0.20.0

- Android: **Ordner-Pfade prüfen** im Menü benannter und unbenannter Personen. Native Liste mit formatierten vollständigen Pfaden, 40 Ordnern pro Seite und maximal acht Gesichtsvorschauen je Ordner.
- Gewohnte Originalvergrößerung per Gedrückthalten und Ziehen, Gesichtsmarkierung, formatierter Fotopfad, Bildcache und TalkBack-Aktion. Geänderte Bildquellen zeigen keine veralteten Gesichtsrahmen.
- Benannte Personen mit ausschließlich ausgeschlossenen Pfaden bleiben in der nativen Personenliste erreichbar und öffnen direkt die Ordnerprüfung; geschützte Pfade und Namen bleiben verborgen.
- Alle Ordneraktionen integriert: vorhandener Person oder neuem Namen zuweisen, unbenannt setzen, ignorieren, Pfad ausschließen und wieder freigeben. Aktionen erfassen alle aktiven Gesichter der Person im exakten Ordner, auch über 500; Unterordner bleiben separat.
- Neue native Fähigkeit `person_folders`, begrenzter Ordnerendpunkt und atomare `folder_*`-Aktionen mit Quell-/Zielrevisionen, Datenbestandsprüfung und kontogebundenen Quittungen. Offene Aktionen werden bei Wiederanlauf sicher aufgelöst; ältere Server behalten die bisherige Bedienung.
- MINOR für die zusätzliche native Funktion und kompatible API-Erweiterung; Android `versionCode 41`, keine Datenmigration. README, Android-Anleitung, technische Referenz, Website und OpenAPI aktualisiert.
- Validierung: vollständige Go-Suite, gezielte Race-/HTTP-/OpenAPI-Tests, 115 JVM-Tests, 34 Android-Gerätetests einschließlich vier Tests gegen den isolierten Go-HTTPS-Server, Android-Lint ohne Fehler, Debug-/Test-APK, zwei Ordner-Browserregressionen und Website-Build erfolgreich. Geprüft sind mehr als 500 Gesichter, atomarer Quittungs-Rollback, gleichzeitige Wiederholung, veraltete Revisionen, Pfadsperren, Datenschutz, Seitenwechsel, Wiederanlauf, große Schrift sowie Halte-/Zoom- und TalkBack-Vorschauen; 357 lokale Dokumentationsverweise ohne Fehler.

### BearStack 1.1.2

- Ordnerprüfung: Seitennavigation zentriert und mit klaren Abständen zwischen Seitenzahl, Zurück und Weiter; auf Mobilgeräten bleiben die Elemente nebeneinander, bei Platzmangel umbrechend.
- Personensuche: Suchfeld, Zurücksetzen und Suchen gleich hoch ausgerichtet; mobile Bedienelemente mindestens 44 Pixel hoch.
- PATCH für Layoutkorrekturen ohne zusätzliche Abfragen oder JavaScript. README, Website und OpenAPI-Version aktualisiert; HTTP-Verträge unverändert.
- OpenAPI: bestehende Fehlerbeschreibung der Stammbaum-Einstellungen korrekt in Anführungszeichen gesetzt, damit enthaltene Kommas keine zusätzlichen YAML-Felder erzeugen.
- Validierung: gezielte Go- und OpenAPI-Tests, vier Browserregressionen und Website-Build erfolgreich. Geprüft sind Suchfeld-/Buttonhöhen von 320 bis 1440 Pixeln mit und ohne JavaScript sowie Abstand, Zentrierung und Navigation auf erster, mittlerer und letzter Ordnerseite.

### BearStack 1.1.1

- Stammbaum: Familienzweige richten sich an den tatsächlichen Eltern- und Kinderpositionen aus. Partner und gemeinsame Eltern bleiben zusammen; stabile Geburts-/Namenssortierung ersetzt die ID-Reihenfolge. Generationenabstand von 250 auf 400 Pixel erhöht; längere horizontale Beziehungen verlaufen oberhalb der Karten.
- Layout-Regressionen prüfen ungleich breite Familienzweige, chronologische Geschwister, gemeinsame Eltern ohne Eheeintrag, wiederholte Ehen, Kollisionsfreiheit und identische Ausgabe bei vertauschter Eingabereihenfolge. JavaScript-Suite, Stammbaum-Browserprüfung und Website-Build erfolgreich; verzweigter Testbaum mit 10.000 Personen lokal in rund 75 ms angeordnet.

- Ordnerprüfung: **Alle neu zuweisen** verwendet das etablierte Personenmodal statt eines dauerhaften Suchfelds je Ordner. Ein gemeinsamer Dialog zeigt den formatierten Pfad und erlaubt vorhandene Personen oder neue Namen; die übrigen Ordneraktionen bleiben direkt erreichbar.
- Personenmenü: **Ordner-Pfade prüfen** als normaler Menülink; **Anzeige** heißt jetzt **Anzeigeeinstellungen**.
- README und Website aktualisiert; HTTP-Verträge bleiben unverändert.
- Validierung: gezielte Go-Tests, JavaScript-Syntax-/DOM-Suite, Browserregressionen für Personenansicht und Ordneraktionen sowie Website-Build erfolgreich. Geprüft sind ein gemeinsamer Dialog, neue und vorhandene Zielpersonen, formatierte Stamm-/Datums-/Unterordnerpfade, Fokus-Rückkehr, Menügestaltung und 320-Pixel-Darstellung.

### BearStack 1.1.0

- Neue Stammbaum-Einstellungen bei aktiviertem Fotomodul: bis zu 200 Ausgangspersonen auswählen. Verbundene ausgewählte Personen erzeugen einen gemeinsamen Baum; ohne Auswahl entfallen Ansicht und deren Navigation.
- Vollständige Suche in beiden Richtungen durch hinterlegte Eltern-, Geschwister- und Eheverbindungen, einschließlich geschiedener und wiederholter Ehen. Stammdaten, Personen-Tags und Porträts sind in der grafischen Ansicht verfügbar.
- Interaktive Personen-Karten, Beziehungslinien, Suche, Detailbereich, Zoom, Mausziehen und native Scrollfunktion. Große Stammbäume zeichnen nur sichtbare Karten; kleine Zoomstufen zeigen eine vereinfachte Übersicht.
- Automatische Migration auf Foto-Schema 39. Auswahl bleibt über Neustarts und Personenzusammenführungen erhalten; Revisionen schützen gleichzeitige Änderungen. Konfiguration benötigt `photos.manage`, Ansicht `photos.read`; private Personen werden auch als indirekte Verbindung ausgeschlossen.
- Begrenzte, indizierte Graphsuche ohne Bildanalyse: maximal 10.000 Personen und 50.000 Beziehungen pro Abruf, mit Zeitlimit und ausdrücklichem Fehler statt unvollständiger Bäume. README, Website und OpenAPI ergänzt.
- Validierung: vollständige Go-Suite, gezielte Race-Tests, JavaScript-Syntax-/DOM-Tests und Website-Build erfolgreich. Browserprüfung mit Desktop und Mobilansicht, Porträts, Zoom, Scrollen, Suche, Ladefehler/Wiederholung und 2.000 Personen. Regressionen sichern Migration, Neustart, Zusammenführung/Rollback, Sichtbarkeit, Rechte, CSRF, Revisionen und Größenlimit; das Layout verarbeitet 10.000 Generationen ohne Rekursion.

### BearStack 1.0.0

- BREAKING: Gesichtsketten einschließlich WebUI, JavaScript, Suche, Sammelzuordnung und dokumentierter HTTP-Endpunkte entfernt. Andere Personenansichten bleiben verfügbar. Erster inkompatibler HTTP-Vertragswechsel in 0.x, daher 1.0.0.
- Personenansicht: Stammdatenzeile direkt unter der Überschrift, ausschließlich hinterlegte Werte und ungeschiedene Ehen.
- Neue Ordneransicht `/photos/people/{id}/folder`: eindeutige vollständige Galeriepfade, bis zu acht Gesichtsvorschauen pro Ordner, 40 Ordner pro Seite und atomare Sammelaktionen für alle aktiven Gesichter im exakten Ordner.
- Personenbezogene Pfadsperren verhindern manuelle und automatische Neuzuordnungen. Ausschließen setzt bestehende aktive Gesichter unbenannt; Sperren können wieder aufgehoben werden und werden bei konfliktfreien Personenzusammenführungen übernommen.
- Automatische Migration auf Foto-Schema 38 mit indizierten Ordnersperren und Datenbank-Schutz gegen gesperrte Zuordnungen.
- Validierung: vollständige Go-Suite, gezielte Race-Tests, JavaScript-Syntax- und DOM-Prüfungen, Browserregressionen für Personenansicht und Ordneraktionen sowie Website-Build erfolgreich. Regressionen prüfen unter anderem mehr als 500 Gesichter, acht Vorschauen, formatierte Pfade, Rechte, CSRF, veraltete Revisionen, atomaren Rollback, Neuanalyse, Hintergrundabgleich, Migration und Neustart.

### BearStack 0.69.1

- PATCH: Web-Diashow lädt die nächste Galerie-Seite automatisch nach und endet erst am Ende der Medienauswahl. Filter, Sortierung und Vollbild bleiben erhalten; der Speicherbedarf bleibt auf eine Medienseite begrenzt.
- Stoppen und Schließen brechen laufende Seitenabfragen ab. Ladefehler pausieren die Wiedergabe mit einem Hinweis und erlauben einen erneuten Start. Die ursprüngliche Galerie bleibt beim Schließen erhalten.
- README und Website aktualisiert; keine API- oder Datenformatänderung.
- Validierung: Go-Tests für Server und Fotos, JavaScript-DOM-Suite, sechs gezielte Browser-Regressionen einschließlich Seitenwechsel, Abbruch und erneutem Start nach Ladefehler sowie Website-Build erfolgreich.

### Dokumentation

- Foto-Dokumentation vom ersten Galerieaufruf bis zu Suche, Tags, Karte und Fotoframe neu strukturiert. Eigene Anleitungen für Personen/Gesichter und Einrichtung/Betrieb trennen Arbeitsabläufe von Konfiguration, Ressourcenlimits und API-Referenz. Versionschronik und doppelte Android-Beschreibungen durch aktuelle Bedienhinweise und gezielte Verweise ersetzt; Website-Navigation und README aktualisiert.
- Foto-Doku geprüft: Website-Build ohne Warnungen, 1.172 lokale Verweise ohne neue Fehler sowie alle drei Seiten in Chrome bei 1440 und 390 Pixeln Breite mit vollständiger Navigation und ohne horizontalen Seitenüberlauf.
- Android-Anleitung nach Arbeitsabläufen neu aufgebaut: Einstieg und Verbindung, Orientierung, Galerie, lokale Fotos, Personenverwaltung, Einstellungen, Offlinebetrieb und Fehlerhilfe. Veraltete Menüpfade und widersprüchliche Bedienhinweise korrigiert; Kompatibilitätsangaben zentral zusammengefasst.
- Technische Referenz für Build, Signierung, API, Datenhaltung, Speichergrenzen und Tests ergänzt. Android-README dient als Entwicklungsstart und verweist auf die gemeinsame Dokumentationsquelle; Website-Navigation und bestehende Verweise aktualisiert.
- Validierung: Website-Build ohne Warnungen, 290 lokale Dokumentationsverweise geprüft sowie beide Seiten in Chrome bei 1440 und 390 Pixeln Breite ohne horizontalen Seitenüberlauf kontrolliert.
- Reine Dokumentationsänderung ohne Versionssprung oder Änderung der App/API.

### BearStack 0.69.0 / Android 0.19.0 – in Entwicklung

- Fotoframe: gespeicherter Schalter **Zufällige Reihenfolge** im Zahnrad-Dialog; zunächst ausgeschaltet. Speichern startet die Wiedergabe mit der gewählten Reihenfolge neu. Filter und vorherige Galerieansicht bleiben erhalten.
- Native Foto-API meldet `frame_random_sort` und akzeptiert `sort=random` über die vorhandene stabile Indexsortierung. Ältere Server behalten die normale Wiedergabe und zeigen einen deaktivierten Schalter mit Versionshinweis.
- Gerätefotos werden über eine kompakte, temporäre Liste ihrer Bild-IDs gemischt. Metadaten bleiben auf 96 Fotos je Seite und drei Seiten im Viewer begrenzt; kein vollständiger Bild- oder Metadatenkatalog. Wiederholungen und Rückwärtsnavigation behalten die Reihenfolge, ein neuer Fotoframe-Start mischt Gerätefotos neu.
- Regressionen für Paging, Sichtbarkeit, Wiederholung, Abbruch veralteter Abfragen, gespeicherte Einstellungen, Dialogbedienung und Geräteanbieter mit und ohne native Pagination ergänzt.
- MINOR für die zusätzliche Einstellung und API-Fähigkeit; Android `versionCode 40`. README, Website und OpenAPI aktualisiert, keine Datenmigration.
- Validierung: vollständige Go-Suite und 111 JVM-Tests, 15 Emulatorprüfungen für Gerätefotos, Einstellungen und Wiedergabe sowie Lint, Debug-/Test-APK und Website-Build erfolgreich. Schnelles Umschalten setzt den Viewer zuverlässig auf die neue Reihenfolge zurück; lokale Dokumentationsverweise ohne neue Fehler geprüft.

### BearStack 0.68.3 – in Entwicklung

- Upload-Antworten geben bei Duplikaten Dokument-ID, Bestandsdateiname und Dokumentlink nur noch mit `documents.read` zurück. Reine Upload-Konten erhalten weiterhin die Duplikatmeldung mit dem von ihnen eingereichten Dateinamen.
- Dokument-Tags erfordern auch bei deaktiviertem Fotomodul Dokument-Leserecht. Der gemeinsame Tag-Handler lädt nur die jeweils erlaubten Bereiche; die Vorlage zeigt ohne Leserecht keine Dokument-Tags an.
- IMAP-Abruf prüft die gemeldete Nachrichtengröße vor dem Download und fordert den Nachrichteninhalt begrenzt an. Das bisher erst bei der MIME-Verarbeitung greifende Größenlimit schützt damit bereits den Abruf über die puffernde IMAP-Bibliothek.
- Mail-Import und EML-Archivierung begrenzen MIME-Verschachtelung auf 32 Container. Fehler brechen die Verarbeitung ab; abgewiesene Nachrichten werden nicht gelöscht, temporäre Archivdateien werden entfernt.
- Regressionen für beide Upload-Endpunkte, Rollen und Zusatzrechte, Tag-Zugriff ohne Fotomodul, IMAP-Protokoll und Größen-Grenzwerte, MIME-Grenzwerte und Fehlerbereinigung ergänzt. PATCH ohne Datenmigration; README, Website und OpenAPI aktualisiert.
- Validierung: `go test ./...` und gezielte Sicherheitsregressionen mit Race-Detector erfolgreich; Website ohne Build-Warnungen erzeugt, keine neuen defekten lokalen Dokumentationsverweise.

### BearStack 0.68.2 – in Entwicklung

- Native Ordnerseiten mit Namenssortierung direkt in SQLite paginieren; Gesamtzahl und Seite aus demselben Lesesnapshot. Unicode-Sortierung, zentrale Anzeigenamen, Sichtbarkeit und die virtuelle Personenkachel bleiben erhalten.
- Komplexe Suchabfragen, Datum-/Zufallssortierung und unvollständige Sichtbarkeitszähler behalten den bisherigen Pfad; keine Schemaänderung.
- Web-Fotoframe auf zwei Metadatenseiten begrenzt, verbrauchte Einträge freigegeben und neuer Durchlauf ab Seite 1. Ausgeblendete Seiten pausieren Wiedergabe und Laden; Rückkehr setzt am aktuellen Medium fort. Langsame Seiten überspringen keine Bilder.
- Validierung: vollständige Go-Suite, JavaScript-/DOM-Tests, 31 Galerie-Browserszenarien und Website-Build erfolgreich. Ordnerregressionen vergleichen Seitengrenzen, Unicode, Datumspräfixe, Unterstriche, Verschachtelung, Rechte und virtuelle Ordner; Fotoframe-Regressionsläufe prüfen mehrere Zyklen, Speichergrenze, langsame Antworten, Abbruch und Fortsetzen.
- Lokaler Vergleich bei 10.000 Ordnern: erste Namensseite rund 59 → 10 ms und 20,2 → 0,7 MiB Allokationen; tiefe Seite rund 49 → 19 ms. Synthetischer Benchmark, keine Produktionsmessung.
- PATCH für Performance und Lebenszykluskorrekturen; Android bleibt 0.18.1. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.68.1 / Android 0.18.1 – in Entwicklung

- Fotosuche: automatische Gesichtsdaten im nachgelagerten Filter nur bei Personenbedingungen laden.
- Native Foto-API: Inhaltsrevision, Identität und Prüfstatus gemeinsam für Medien und Ordnervorschauen in deduplizierten 200er-Abfragen ergänzen.
- Android: vorhandene Thumbnails beim Vorladen nur anhand von Dateimetadaten prüfen; unveränderte Schutzlisten nicht erneut schreiben. Anzeige-LRU, Cache-Reparatur und Download-Koaleszierung bleiben erhalten.
- Validierung: vollständige Go-Tests für Fotos, Server und API-Verträge; JVM-Tests, Android-Lint, Debug-/Test-Build, sechs Cache-Emulatortests und Website-Build erfolgreich. Regressionen prüfen Abfragebudgets, doppelte Pfade, Prüfstatus, unveränderte Cache-Dateien und Reparatur.
- PATCH für Performance-Optimierungen; Android `versionCode 39`. Keine API- oder Datenmigration. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.68.0 / Android 0.18.0 – in Entwicklung

- Android: „Lokale Fotos öffnen“ aus dem Verwaltungsmenü entfernt; Ordner → Dieses Gerät und der lokale Einstieg beim Anmelden bleiben verfügbar. „Personen verwalten“ und „Personenliste“ eindeutig getrennt; Personenliste und Personendetails bündeln Zurück, Hilfe und Verbindungswechsel im …-Menü.
- Zahnrad statt … für Großbild-, Diashow- und Fotoframe-Einstellungen. Neue gespeicherte Fotoframe-Auswahl zwischen Dateiname und formatiertem Ordnername; ältere Server zeigen bei fehlendem Ordnernamen einen Hinweis, lokale Gerätefotos ihren Album-Anzeigenamen.
- Kompaktere Navigation mit 18-dp-Icons links vom Text; Einzelmarkierungen auf Karten nur noch 16 dp. Berührungsflächen bleiben 48 dp groß, Paging und Caches bleiben erhalten.
- Optionales Katalogfeld `folder_name` aus `photos.MediaFolderName`, ohne zusätzliche Datenbank- oder Netzwerkabfragen. README, Android-Dokumentation, Website und OpenAPI aktualisiert.
- MINOR für die zusätzliche Einstellung und das kompatible API-Feld; Android `versionCode 38`. Keine Datenmigration.
- Validierung: vollständige Go-Tests für Server, Fotoverwaltung und OpenAPI; 107 JVM-Tests, 36 Emulatorregressionen und beide abschließenden Layouttests erfolgreich. Lint, Debug-/Test-APK und Website gebaut; Navigation mit normaler und doppelter Schriftgröße visuell geprüft.

### BearStack 0.67.6 – in Entwicklung

- Personenübersicht: Unter **Unbenannt** steht **Ignorieren** direkt in der Batch-Leiste, auch im Auswahlmodus. Je ausgewählter Kachel wird ausschließlich das angezeigte Gesicht gesammelt über den bestehenden Endpunkt ignoriert. Weitere Gesichter und Fotos bleiben erhalten.
- Gemeinsame Aktionssperre, begrenzte Anfragezeit und reine Lese-Wiederholung nach unbestätigten Antworten oder fehlgeschlagener Aktualisierung verhindern versehentliche Folgeaktionen. Filter, Sortierung, Auswahlmodus und unveränderte Vorschaubilder bleiben bei AJAX-Aktualisierungen erhalten.
- Validierung: Go-Tests für Server, Fotoverwaltung und OpenAPI, JavaScript-/DOM-Prüfungen, 25 Personen-/Gesichts-Browserszenarien und Website-Build erfolgreich. Regressionen für Rechte, Mehrfachauswahl, übrige Gruppengesichter, doppelte Klicks, Fehlerantworten, Verbindungsabbrüche und Zeitüberschreitungen; bestehende Sichtbarkeitsprüfungen an die aktuelle Oberfläche angepasst.
- PATCH **0.67.6** für den direkten Zugang zur bestehenden Ignorieraktion. README, Website und OpenAPI aktualisiert; keine API- oder Datenmigration.

### BearStack 0.67.5 / Android 0.17.2 – in Entwicklung

- Android-Galerie: **Fotos · Ordner · Suchen** schweben als abgerundete Leiste über dem Inhalt. Das Raster läuft hinter der Leiste weiter, ohne durch einen festen unteren Bereich verkürzt zu werden.
- Scrollbarer Endabstand hält die letzte Reihe erreichbar; Systemnavigation und seitliche Displayausschnitte werden berücksichtigt. Bestehendes Lazy-Grid, Paging und Bildcache bleiben erhalten.
- Layoutregressionen für alle drei Ansichten, letzte Bildreihe und große Schrift ergänzt. JVM-Tests, Lint, Debug- und Website-Build erfolgreich; UI-Tests kompiliert, mangels laufendem Testemulator nicht ausgeführt.
- PATCH für die Layoutkorrektur; Android `versionCode 37`. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.67.4 / Android 0.17.1 – in Entwicklung

- Android: Im Benennmodus stehen in der unteren Aktionsleiste wieder **…** links, **?** mittig und der **Stift** rechts. Aktionen und Touchflächen bleiben unverändert; keine zusätzlichen Abfragen.
- Vorhandenen Layouttest für die Reihenfolge und überlappungsfreie Bedienflächen bei großer Schrift angepasst.
- PATCH für die UI-Korrektur; Android `versionCode 36`. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.67.3 – in Entwicklung

- Foto-Lightbox: Im Benennen-/Zuordnen-Dialog erscheint bei benannten Gesichtern **Auf unbenannt setzen**. Der Button löst nur das angeklickte Gesicht als neue unbenannte Gruppe ab, auch das letzte Gesicht einer Person. Andere Gesichter derselben Person bleiben unverändert; Entwürfe im Namensfeld werden nicht gespeichert.
- Vorhandener Endpunkt und gemeinsame Aktionssperren; keine zusätzliche Gesichtserkennung oder Abfrage beim Öffnen. Nach unbestätigtem Speichern oder fehlgeschlagener Aktualisierung verhindert eine Lesewiederholung doppelte Schreibaktionen.
- Validierung: 39 Chromium-Szenarien für Foto-Lightbox, Personen, Gruppenbilder und Zusammenführung sowie gezielte Go-/OpenAPI-, JavaScript-/DOM-Prüfungen und Website-Build erfolgreich. Regressionen decken Einzelgesicht, letztes Gesicht, Fehlerantworten, verlorene Antworten und Schutz vor doppeltem Speichern ab.
- PATCH **0.67.3** für den direkten Zugang zur bereits vorhandenen Einzelgesichtsaktion; keine API- oder Datenmigration. README, Website und OpenAPI aktualisiert.

### BearStack 0.67.2 – in Entwicklung

- Personenübersicht: Auswahlmodus unter Unbenannt direkt links neben dem Drei-Punkte-Menü angeordnet, nach der Sortierung.
- PATCH für die kleine UI-Korrektur; README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.67.1 – in Entwicklung

- Personenübersicht: Ordnernamen in Alle, Benannt und Unbenannt verwenden dieselbe zentrale Galerie-Formatierung wie Ignoriert. Datumspräfixe und Unterstriche werden korrekt verarbeitet; Tooltips zeigen den formatierten Fotopfad. HTML, vorhandene AJAX-Kacheln und neu eingefügte Kacheln verwenden dieselben Serverwerte.
- Das ergänzende JSON-Anzeigefeld `folder_name` korrigiert die Darstellung; technische Verzeichnisse und Sortierung bleiben unverändert. Keine zusätzlichen Datenbankabfragen.
- Verbindliche Vorgabe in `AGENTS.md`: zentrale Formatierung für Namen, Pfade und Tooltips, keine Rohpfad-Fallbacks im Browser sowie Regressionen für HTML und AJAX.
- PATCH **0.67.1** für die Darstellungskorrektur; keine Datenmigration. README, Website und OpenAPI aktualisiert.

### BearStack 0.67.0 – in Entwicklung

- Personenübersicht: Unter **Unbenannt** und **Ignoriert** steht der Ordnername immer über dem Vorschaubild, auch ohne JavaScript. Im Stammverzeichnis erscheint „Fotos“; lange Namen werden mit vollständigem Tooltip gekürzt.
- Neuer **Auswahlmodus** für unbenannte Personen mit Fotobearbeitungsrechten: Die gesamte Kachel wählt per Klick, Enter oder Leertaste aus beziehungsweise ab. Personenlinks sind deaktiviert, Stift und Einzel-Ignorieren ausgeblendet. Die bisherigen Batch-Aktionen bleiben verfügbar; Ausschalten erhält die Auswahl und stellt die Einzelaktionen wieder her.
- AJAX-Aktualisierungen erhalten den Modus und unveränderte Bild-Elemente. Ordneranzeige und Auswahl benötigen keine zusätzlichen Serverabfragen; bestehende Seitengrößen und Berechtigungen bleiben erhalten.
- **Ähnliche Gesichter** nutzt im Benennen-/Zuordnen-Modal die gemeinsame 300%-Fotovorschau mit Gesichtsrahmen und formatiertem Pfad. Einzelgruppen zeigen ihr Vergleichsgesicht, Zusammenführungen das erste Gesicht wie bei normaler Mehrfachauswahl. Ordnerbeschriftungen verwenden die Galerie-Formatierung statt roher Namen. Koordinaten kommen aus der bestehenden Abfrage, ohne zusätzliche Metadatenanfragen; JSON-Verträge bleiben unverändert.
- Validierung: vollständige Go-Suite, JavaScript-/DOM-Prüfungen, 37 Chromium-Szenarien für Personenansicht, Vorschläge, Gruppenbilder und Foto-Lightbox sowie Website-Build erfolgreich. Regressionen prüfen Ordnerformatierung, Rechte, Kachel-/Tastaturauswahl, AJAX-Nachladen, 300%-Ausschnitt, Vorschaufehler und Wechsel zwischen Dialogaufrufern ohne zusätzliche Schreibaktionen. Die Vorschlagsseite bindet Dialog und Picker nur einmal ein.
- **Gruppenbilder**: Rückgängig-Symbol in beiden Aktionsleisten zum Wiederherstellen aller ignorierten Gesichter des aktuellen Fotos, auch bei aktivem Unbenannt-Filter. Namen, Zuordnungen und Favoriten bleiben erhalten; das Foto bleibt geöffnet. Bestehender revisionsgeschützter Endpunkt, gemeinsame Aktionssperren und reine Lese-Wiederholung nach verlorenen Antworten oder fehlgeschlagenem Nachladen. Alle 11 Gruppenbilder-Browsertests sowie gezielte Go-/OpenAPI- und JavaScript-Prüfungen erfolgreich.
- MINOR **0.67.0** für die optionalen UI-Funktionen; keine API- oder Datenmigration. Android bleibt **0.17.0**. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.66.2 – in Entwicklung

- Gemeinsamer Gesichtsanalyse-Service für Hintergrundlauf, manuelle Analyse und Quellenprüfung. Der Service besitzt die gemeinsame abbrechbare Sperre und die Client-Erzeugung; Bildaufbereitung, Ausschnittverbesserung und Speichern folgen weiterhin demselben Ablauf. Die Quellenprüfung ersetzt keine ungeprüften Bestandsdaten. HTTP-Verbindungen werden nach jeder Aktion beziehungsweise jedem Batch freigegeben.
- Der Ordnerdienst verwendet eine schmale lesende Repository-Schnittstelle statt des konkreten SQLite-Repositories. Root-Tag-Cache, Feldwert-Cache und gebündelte Favoritenzählung bleiben erhalten; keine zusätzlichen Abfragen.
- `app-person-picker.js` kapselt Personensuche, gestreamte Gesichtsvorschläge, Vorschauanzeige und Tastatur-/Touch-Auswahl. Formularziele, Validierung, Buttontexte und Absenden liegen im Dialogadapter. Der Zusammenführungsdialog benötigt keine künstliche Rename-URL mehr.
- Regressionen prüfen fehlgeschlagene Analysen ohne Speichern, getrennte Client-Lebenszyklen, Ordner-Abfragebudgets sowie eigenständige Personenauswahl ohne Formularänderung und veraltete Suchantworten. PATCH **0.66.2**, keine API-, Schema- oder Dependency-Änderungen; Android bleibt **0.17.0**. README, Website und OpenAPI-Version aktualisiert.
- Validierung: vollständiges `make test` mit Go, JavaScript/DOM, 95 Browser- und 13 Python-Tests, Android/JVM/Lint/Debug-Build und tatsächlichem Read-only-Mount erfolgreich. Gezielte Race-Tests für Service-Aufbau und Gesichtsanalyse, `go vet`, Go-Build und Website-Build ebenfalls erfolgreich.

### BearStack 0.66.1 – in Entwicklung

- Konservatives Cleanup: ungenutzte Felder aus kompilierten Foto-Suchausdrücken und aufbereiteten Dokumentspalten-Einstellungen entfernt; die Tabellenliste beim Ordnerumzug verwendet das bestehende Muster einer einfachen String-Liste.
- Nicht verwendeten Parameter und überflüssige Variablenbindung in der Benutzerverwaltung entfernt. Suchverhalten, Spaltenreihenfolge, Rollenvergabe und Berechtigungsprüfungen bleiben unverändert.
- PATCH **0.66.1** gemäß `AGENTS.md`; keine API-, Datenbank- oder Dependency-Änderung. Android bleibt **0.17.0**. README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.66.0 – in Entwicklung

- Manuelle und automatische Personen-Zusammenführungen übertragen aufbewahrte und durch Ordnerschutz ausgeblendete Gesichter atomar zur Zielperson. Gesichts-IDs, Embeddings, Favoriten und Vorschaubytes bleiben auch bei Rückkehr nach Neustart erhalten; verborgene Namensquellen werden ohne Veröffentlichung übernommen.
- Ordnerumzüge und Rückkehr invalidieren nur betroffene Ordner und Vorfahren. Gesichts-/Jobzähler folgen Verzeichnisänderungen inkrementell; Referenzen werden für betroffene Personen aktualisiert. Unbeteiligte Scanstände, Vorschauzuordnungen und Referenzen bleiben erhalten.
- Foto-Schema 37: zentrale Synchronisierung aktiver und aufbewahrter Tabellenspalten nach versionierten Migrationen, explizite Spaltenzuordnung für Kopien und Sichtbarkeitstrigger, indexgestützte Pfad-/Personenzuordnung. Nichtadditive Schemaänderungen verlangen eine explizite Migration. Die ungenutzte Tabelle `photo_identity_state` entfällt.
- `make test` umfasst jetzt Go, JavaScript/DOM, Playwright, Python-Gesichtsdienst, Android/JVM/Lint/Debug-Build und einen tatsächlichen Read-only-Foto-Mount. `make test-fast` bietet Go/JavaScript für die lokale Schleife; `make test-release` ergänzt Race-Tests und den minimierten Android-Release-Build. Fehlende Voraussetzungen brechen die jeweilige Prüfung ab.
- Regressionen für Zusammenführen während Aufbewahrung/Ordnerschutz, Transaktionsrollback, Wiederanlauf nach Zuordnung, unveränderte fremde Indexbereiche und additive Migrationen mit abweichender Spaltenreihenfolge. Browserprüfungen warten auf das Dialog-Schließen und pausieren für Einzelaktionsprüfungen den unabhängig getesteten Hintergrundabgleich. MINOR **0.66.0** aufgrund der automatischen kompatiblen Migration gemäß `AGENTS.md`; Android bleibt **0.17.0**, HTTP-Verträge bleiben kompatibel.

### BearStack 0.65.0 / Android 0.17.0 – in Entwicklung

- Stabile Fotoidentitäten und fortsetzbare SHA-256-Erfassung: eindeutige externe Ordnerumzüge sowie Kopieren mit anschließendem Löschen erhalten Tags, Gesichts-/Personen-IDs und bestehende Vorschauen. Der Foto-Root bleibt ausschließlich lesbar.
- Fehlende Medien, Ordner, Blogs und GPX-Einträge werden sieben Tage verborgen aufbewahrt. Unvollständige Scans und nicht verfügbare Roots bestätigen keine Löschung. Verwaltung zeigt Fristen, Umzüge und Fingerabdruckfortschritt; manuelle Zuordnung und endgültige Bereinigung benötigen `photos.manage` und aktuelle Revisionen.
- Geänderter Originalinhalt erhält Gesichtsdaten als prüfbedürftigen Analysestand. Webprüfung mit alter Vorschau, aktuellem Bild und korrigierbarem Rahmen/Person; alte Embeddings bleiben bis zu einer geeigneten Neuberechnung von Referenzen ausgeschlossen. Android zeigt den Prüfstatus und blendet veraltete Rahmen aus.
- Cachezuordnungen bleiben bei Umzügen stabil; Inhaltsänderungen und endgültige Löschungen verwenden ein persistentes Bereinigungsprotokoll. Geschützte Quellen entziehen auch gespeicherten Gesichtsvorschauen und Referenzen die öffentliche Sichtbarkeit.
- Validierung: Go-Suite, gezielte Race-Prüfungen, Android-Unit-Tests/Lint/Debug-Build und JavaScript-Prüfungen; Chromium-Ablauf für Prüfung bei Desktop- und Mobilbreite. Migration aus Schema 35, Copy/Delete, mehrdeutige Teilbäume, gleiche Größe/Zeitstempel, XMP-Neuanalyse, Cache-Aufbewahrung und Wiederanlauf geprüft. Index/Fingerabdrücke auf einem tatsächlich read-only eingebundenen APFS-Testimage geprüft; synthetischer Folgelauf mit 10.000 Dateien gemessen.
- Kompatible SQLite-Migration; additive API-Felder und Verwaltungs-/Prüfendpunkte. MINOR **0.65.0**, Android **0.17.0**, `versionCode 35`. README, Website und OpenAPI aktualisiert.

### BearStack 0.64.0 / Android 0.16.0 – in Entwicklung

- Android: dauerhafter Thumbnail-Cache mit einstellbarem Budget von 64–2048 MiB (Standard 256 MiB). Die neuesten 50 Stream-Vorschauen und alle Kachelvorschauen der ersten Ordnerebene bleiben geschützt; übrige Einträge werden nach letzter Nutzung verdrängt. Der Pflichtbestand hat Vorrang vor einem kleineren Budget. Einstellungen zeigen Belegung, Fortschritt und Wiederholung bei Fehlern.
- Privater Speicher ohne Backup, getrennte Schlüssel nach Server/Instanz/Datenbestand/Konto/Bildversion/Größe, atomare Dateien und persistenter Pflichtbestand. Verbindungswechsel löscht den Cache. Begrenzte, abbrechbare Downloads teilen sich Anfragen pro Bild; Originale, große Vorschauen und lokale Gerätefotos werden nicht dauerhaft gespeichert.
- Einheitliche Optionsmenüs als horizontales **… rechts**: Galerie, Benennen/Statistik, Gruppenaktionen und Mehrfachauswahl. Gemeinsame Menükomponente mit unveränderten Aktionssperren und zugänglichen Beschriftungen; Stift beim Benennen nun links.
- Validierung: 105 JVM-Tests und 48 unterschiedliche Emulatorprüfungen erfolgreich; Cache-/Sitzungstests zuletzt auf der finalen APK wiederholt. Android-Lint für Debug und Release, Debug-/Test-APK, minimierter Release-Build, Versions-/OpenAPI-Prüfungen und Website-Build erfolgreich.
- MINOR **0.64.0** / Android **0.16.0**, `versionCode 34`. Bestehende API-Endpunkte unverändert; README, Android-Anleitung, Website und OpenAPI-Versionsangabe aktualisiert.

### BearStack 0.63.0 – in Entwicklung

- WebUI: **Alle ignorierten Gesichter zurücksetzen** im Dreipunkt-Menü echter Fotoordner ab Ebene 2. Nach Bestätigung gilt die Aktion rekursiv für den aktuellen Ordner, unabhängig von Such-/Anzeigefiltern. Root, Jahresordner und virtuelle Personenordner sind ausgeschlossen.
- Nur ignorierte Gesichter ohne Personennamen werden wieder unbenannt sichtbar. Benannte Personen einschließlich ihrer ignorierten Gesichter, Zuordnungen, Tags, Favoriten und Stammdaten sowie andere aktive Gesichter bleiben unverändert. Bestehende Admin-only-Sichtbarkeitsprüfungen gelten weiterhin.
- Serverseitige Rechte-, Pfad- und Tiefenprüfung; gleicher CSRF-Schutz wie bei vorhandenen Gesichtsaktionen. Atomare Rücksetzung und Referenzaktualisierung über den vorhandenen Pfadindex, ohne Bilder einzulesen oder Erkennung zu starten; kein 500-Gesichter-Limit. Unveränderte Wiederholungen bleiben ohne Wirkung.
- Validierung: vollständige Go-Suite und JavaScript-/DOM-Prüfungen erfolgreich; Foto-/Serverpakete nach den letzten Änderungen erneut geprüft. Zwei Chromium-Szenarien prüfen Bestätigung, Abbruch, rekursiven Umfang trotz Medientypfilter, Namens-/Tag-Erhalt, Leserrechte und Menübreiten bei 320, 390 und 1440 Pixeln. Regressionen decken Pfadgrenzen, SQL-Sonderzeichen, CSRF, Symlinks, Admin-only, Rollback, Wiederholungen und 654 Gesichter ab. Website-Build erfolgreich.
- MINOR **0.63.0**: neuer Formular-/JSON-Endpunkt `/photos/faces/reset-ignored-directory`. Keine Datenmigration; Android-App weiterhin **0.15.2**. README, Website und OpenAPI aktualisiert.

### BearStack 0.62.1 / Android 0.15.2 – in Entwicklung

- Android: gemeinsames Dreipunkt-Menü für Fotos, Ordner, Suchen und Dieses Gerät. Lokale Fotos öffnen entfällt dort; Verbindung wechseln steht in den Einstellungen. Lokal öffnet das Dreipunkt-Symbol das Menü, mit deaktivierter Karte und Fotoframe für geöffnete Fotoordner.
- Lokale Ordner sortieren wie ihre Vorschaukacheln nach absteigender Medien-ID. Neue Screenshots oder importierte Bilder mit fehlendem beziehungsweise älterem Aufnahmedatum bleiben so am Anfang sichtbar. Eindeutige, indexgestützte Reihenfolge; bestehende Seiten- und Speichergrenzen bleiben erhalten.
- Validierung: 93 JVM-Tests und 38 Emulatorprüfungen erfolgreich. Regressionen prüfen Menüeinträge auf Deutsch und Englisch, Personenrechte, Verbindungswechsel aus den Einstellungen, lokalen Fotoframe, native und alternative Seitennavigation mit fehlenden/älteren Aufnahmedaten sowie Galerie und Offline-Start. Android-Lint, Debug-/Test-APK, Versions-/OpenAPI-Prüfungen und Website-Build erfolgreich.
- PATCH **0.62.1** / App **0.15.2**, `versionCode 33`. Keine API-Vertrags- oder Datenformatänderung; README, Android-Anleitung, Website und OpenAPI-Version aktualisiert.

### BearStack 0.62.0 – in Entwicklung

- Personenansicht: Tags direkt neben der Namensüberschrift, mit Umbruch auf schmalen Bildschirmen. Stammdaten verwenden denselben Stil wie die anderen Einträge im Mehr-Menü; dynamische Aktualisierung beim Benennen bleibt erhalten. Für diese Layoutkorrektur bestehen die JavaScript-/DOM-Prüfungen und 13 Chromium-Szenarien zu Personenansicht, Tags und Stammdaten; Website neu gebaut.

- Gesichtsketten: Mindestähnlichkeit direkt unter `/photos/people/chains` von 0 bis 1 einstellbar. Ein neuer Durchlauf übernimmt den Wert und setzt übersprungene Gruppen zurück. Vorgabe bleibt die globale Schwelle für manuelle Vorschläge, ohne sie zu verändern.
- Der gewählte Wert gilt über weitere Ketten und Wiederholungen hinweg, auch nach Wiederherstellung einer unbestätigten Zuordnung. Hinweise bei zu großen oder zu langsamen Ketten empfehlen auch eine höhere Mindestähnlichkeit. Bestehende Speicher- und Laufzeitgrenzen bleiben erhalten; keine neue Vektorsuche im Hintergrund.
- Validierung: vollständige Go-Suite, JavaScript-/DOM-Prüfungen, drei Chromium-Szenarien und Website-Build erfolgreich. Tests prüfen strengere und lockerere Schwellen, Grenzwerte und ungültige Eingaben, globale Vorgaben, Neustart, Zuordnungen und Wiederherstellung sowie Darstellung bei 320, 390 und 1440 Pixeln.
- MINOR **0.62.0**: optionales `similarity` im Kettensuch-Endpunkt, ohne Wert weiterhin bisherige globale Vorgabe. Keine Migration, Android unverändert. README, Website und OpenAPI aktualisiert.

### BearStack 0.61.3 – in Entwicklung

- Fehlende Vergleichsbilder im Dialog **Zusammenführen und benennen/zuordnen** sowie beim Benennen einer einzelnen Vorschlagsgruppe korrigiert. Die Dialogvorschau behält Bilder, Ordnernamen und Personennamen; die Bildbuttons der Vorschlagskarten werden ohne ihren Bildinhalt entfernt.
- Validierung: Fehler durch Browserregression reproduziert, anschließend vier Chromium-Szenarien erfolgreich. Geladene Vorschauen, Ordnernamen, Suche, Benennen/Zuordnen, Lightbox, Fokus-Rückkehr und Dialogbreiten von 320, 390 und 1440 Pixeln geprüft. JavaScript-/DOM- und OpenAPI-/Versionsprüfungen sowie Website-Build erfolgreich.
- Bestehende Vorschaubilder werden wiederverwendet, keine zusätzlichen API- oder Datenbankabfragen. PATCH **0.61.3**, keine API-/Schemaänderung, Android unverändert; README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.61.2 – in Entwicklung

- Web-Zusammenführungsvorschläge zeigen den Ordnernamen unter jedem Vergleichsbild. Bilder öffnen die bestehende Foto-Lightbox; ausschließlich der Personenname führt zur Personengruppe. Die Lightbox berücksichtigt auch nachgeladene oder entfernte Vorschläge.
- Validierung: vollständige Go-Suite, JavaScript-/DOM-Prüfungen, vier Chromium-Browserszenarien und Website-Build erfolgreich. Regressionen prüfen Foto-/Namensklicks, Ordnernamen mit Sonderzeichen, Fokus-Rückkehr, aktualisierte Vorschläge und Darstellung bei 320, 390 und 1440 Pixeln.
- Bildpfade werden aus den bereits verknüpften Gesichtseinträgen mitgelesen, ohne zusätzliche Abfrage je Bild. Bestehende Sichtbarkeitsprüfung bleibt erhalten. PATCH **0.61.2**, keine API-/Schemaänderung, Android unverändert; README, Website und OpenAPI-Version aktualisiert.

### BearStack 0.61.1 / Android 0.15.1 – in Entwicklung

- Android: Ein Tipp auf das geöffnete Foto blendet Bedienelemente und Systemleisten aus, der nächste wieder ein. Das Foto behält Größe und Position; Wischen und Pinch-Zoom bleiben unabhängig davon nutzbar. Der sichtbare Umschalter „Vergrößern / Ganzes Foto“ entfällt, zugängliche Zoom-Aktionen bleiben erhalten.
- Android: Titel von Personen, Kategorien und Ordnern stehen bereits während des Ladens bereit, einschließlich Sortieren, Fehlerwiederholung und Zurücknavigation. Maximal 256 Namen je Verbindung, keine zusätzlichen Serveranfragen. Unbekannte virtuelle Ziele zeigen verständliche Platzhalter statt interner Pfade oder IDs.
- Validierung: 93 JVM-Tests und 34 Emulatorprüfungen erfolgreich, darunter Tippen, Pinch-Zoom, Wischen, langsame Vor-/Zurücknavigation, lokale Fotos, Personenverwaltung, Nachladen und Videowiedergabe. Android-Lint, Debug-/Test-APK, OpenAPI-/Versionstests und Website-Build erfolgreich.
- Personenmenü: Bezeichnung „Personen verwalten“ übernommen. PATCH **0.61.1**, Android **0.15.1** (`versionCode 32`); keine API- oder Schemaänderung. README und Website aktualisiert, OpenAPI-Version synchronisiert.

### BearStack 0.61.0 – in Entwicklung

- Personenstammdaten enthalten jetzt auch Mutter und Vater. Ein gemeinsamer Speichervorgang mit Versionsprüfung umfasst Eltern, Lebensdaten, Geschwister und Ehen. Der separate Elternbereich im Mehr-Menü entfällt.
- Kompakte Geschwister- und Ehepartnerzeilen mit Platzhaltern und ×-Buttons statt wiederholter Überschriften und langer Entfernen-Buttons. Leere Sterbe- und Scheidungsdaten bleiben hinter einem kleinen + verborgen; gespeicherte Daten sind dauerhaft sichtbar.
- Validierung: vollständige Go-Suite, JavaScript-/DOM-Prüfungen und Website-Build erfolgreich. Chromium prüft Elternauswahl, gemeinsames Speichern, Entfernen, ausgeblendete optionale Daten, Leserechte und kompakte Zeilen bei 320, 390 und 1440 Pixeln. Backendtests sichern Kompatibilität, versteckte Eltern, Versionskonflikte und atomaren Rollback ab.
- MINOR **0.61.0** wegen der kompatiblen API-Erweiterung: optionale `mother_id`/`father_id` im Stammdaten-Endpunkt, bestehende Clients erhalten Eltern unverändert. Verborgene Eltern bleiben geschützt; ungültige Abstammungen verhindern den gesamten Speichervorgang. Keine Migration, Android-Version unverändert. README, Website und OpenAPI aktualisiert.

### BearStack 0.60.0 – in Entwicklung

- Personen: Dialog **Stammdaten** mit Geburts-/Sterbedatum, mehreren Geschwistern und mehreren Ehen einschließlich Ehepartner, Hochzeits- und optionalem Scheidungsdatum. Benannte Personen über die bekannte Suche wählen; unbekannte Daten bleiben leer. Eltern bleiben separat erreichbar.
- Geschwister und Ehen sind beidseitige Beziehungen. Atomare Speicherung mit Versionsprüfung, Schutz vor veralteten Änderungen, Datums- und Duplikatprüfung. Leserechte sehen ausschließlich sichtbare Angehörige; verborgene Beziehungen bleiben beim Bearbeiten erhalten.
- Zusammenführungen übernehmen passende Daten und Beziehungen; widersprüchliche Lebensdaten und Selbstbeziehungen verhindern den gesamten Vorgang. Automatische Gruppenzuordnungen überspringen Konflikte. Datensätze werden beim Löschen einer Person aufgeräumt.
- MINOR **0.60.0**, Foto-Schema **35** mit automatischer kompatibler Migration. Stammdaten laden erst bei Bedarf; indizierte Abfragen und maximal 100 Geschwister/100 Ehen je Person. GET/PUT-Endpunkt mit `photos.read`/`photos.edit`, Same-Origin-Schutz und Audit. README, Website und OpenAPI aktualisiert; Android-Version unverändert.
- Validierung: vollständige Go-Suite einschließlich HTTP-/API-Vertrag, zusätzliche Regressionen für beidseitige Speicherung, Mehrfachehen, Sichtbarkeit, Datenübernahme, atomaren Rollback, Versionskonflikte und Migration. JavaScript-/DOM-Prüfungen, dreizehn Chromium-Browserszenarien und Website-Build erfolgreich; Stammdatendialog bei 320, 390 und 1440 Pixeln geprüft.

### BearStack 0.59.0 – in Entwicklung

- Personenübersicht: **Personen-Tags ergänzen** für markierte Personen in der vorhandenen Auswahlleiste. Gemeinsamer Tag-Dialog mit Suche, vorhandenen und neuen Tags; Ergänzen erhält bestehende Personen-Tags und verändert keine Foto-Tags. Abbrechen ohne Änderung, Wiederholen nach Fehler möglich.
- Neuer POST-Endpunkt `/photos/people/tags/add` mit `photos.edit`, Same-Origin-Schutz, Sichtbarkeitsprüfung und Audit. Eine Transaktion für die gesamte Auswahl, idempotente Ergänzung, bis zu 500 Personen und 100 Tags pro Person. Mengenbegrenzte Indexabfragen und vorbereitete Schreibanweisung statt einzelner HTTP-Aufrufe je Person.
- MINOR **0.59.0**; README, Website und OpenAPI aktualisiert. Keine Migration; Android-Version unverändert.
- Validierung: vollständige Go-Suite einschließlich Rechte-, Sichtbarkeits-, Rollback-, Limit- und API-Vertragsprüfungen; JavaScript-/DOM-Prüfungen, zwölf Chromium-Browsertests und Website-Build erfolgreich. Browserregression prüft mehrere Personen, Erhalt bisheriger Tags, unveränderte unmarkierte Personen, Abbrechen, Fehlerwiederholung und Filtererhalt.

### BearStack 0.58.0 / Android 0.15.0 – in Entwicklung

- Android: eigenes **Sortieren**-Menü mit aktiv markierter Auswahl neben dem Mehr-Menü. Der Fotostream und Personengalerien bieten Datum auf-/absteigend, normale Ordner zusätzlich Name. Kategorien nutzen Namen; Personenlisten zusätzlich **Anzahl Bilder** auf-/absteigend.
- Native Galerie-API: `ascending_count`/`descending_count` für Alle-, Tag- und ordnerbezogene Personenlisten. Unterschiedliche sichtbare Bilder zählen einmal; im Ordner gilt dessen Unterbaum. Sortierung vor der Seitenaufteilung, stabile Gleichstände nach Name und ID, Porträts nur für die geladene Seite; große Sortierungen können auf Datenträger auslagern.
- Optionale Session-Capability `people_count_sort`; auf älteren Servern zeigt die App weiterhin nur unterstützte Namenssortierungen. Wechsel zur Fotogalerie setzt eine dort unpassende Sortierung zurück.
- MINOR **0.58.0**, Android **0.15.0** (`versionCode 31`). README, Website und OpenAPI aktualisiert; keine Migration.
- Validierung: vollständige Go-Suite, Android-JVM-Tests, Lint und Debug-Build, elf Android-Galerietests im Emulator, JavaScript-/DOM-Prüfungen und Website-Build erfolgreich. Regressionen für getrennte Menüs, passende Sortieroptionen, ältere Server, Seitenwechsel, doppelte Erkennungen, Sichtbarkeit und Ordnergrenzen. Benchmark mit 10.000 Personen, 50.000 Fotos und 100.000 Gesichtern ergänzt.

### BearStack 0.57.3 – in Entwicklung

- Die Hauptkachel **Personen** zählt ausschließlich benannte aktive Personen und zeigt nur deren Gesichtsvorschauen. Gilt für Web und native Galerie; ohne benannte Personen bleiben Anzahl und Vorschauen leer beziehungsweise null. Bestehende Sichtbarkeitsregeln und Vorschaugrenzen bleiben erhalten.
- WebUI: Die Hilfe der Personendetailansicht öffnet als modaler, auf kleinen Bildschirmen scrollbar begrenzter Dialog. Schließen und Escape führen den Fokus zum Mehr-Menü zurück; die Galerie wird nicht verschoben.
- WebUI: Eine gemeinsame Aktionsleiste bündelt Zurück, Fotos, Benennen und Auswahl. Personen-Tags, Eltern, Anzeige, Hilfe und weitere Navigation liegen im Mehr-Menü. Galerie- und Metadatenlinks folgen auch nach einer Zuordnung der aktuellen Person.
- PATCH **0.57.3**; README, Website und OpenAPI aktualisiert. Keine Migration.
- Validierung: vollständige Go-Testsuite und Website-Build erfolgreich; Regressionen prüfen gemischte Gruppen sowie Anzahl und Vorschauen vor und nach dem Benennen. JavaScript-/DOM-Prüfungen und Chromium-Browserregressionen für Personenansicht, Tags, Eltern und Galerie erfolgreich; Hilfedialog, Escape, Fokus-Rückkehr, kompakte Leiste und stabile Galerieposition bei fünf Bildschirmbreiten geprüft.

### BearStack 0.57.2 – in Entwicklung

- Personengalerien bieten mit **Fotos bearbeiten** einen direkten Button **Person bearbeiten** zur jeweiligen Personenansicht. Gilt für Alle-, Tag- und ordnerbezogene Personengalerien; reine Lesekonten sehen den Button nicht. Keine zusätzlichen Datenbankabfragen.
- WebUI: Die Lupe im Dialog **Zusammenführen und benennen/zuordnen** sowie beim Benennen einer einzelnen Vergleichsgruppe ist verfügbar. Wiederverwendung der abbrechbaren Streaming-Suche; Gesichtstreffer liefern die Zielrevision für konfliktgeprüfte atomare Zuordnungen.
- PATCH **0.57.2**: fehlende Navigation ergänzt; README und Website aktualisiert. OpenAPI ergänzt die optionale Zielrevision in Gesichtssuchtreffern.
- Validierung: vollständige Go-Testsuite einschließlich Berechtigungsprüfungen für den neuen Link, JavaScript-/DOM-Prüfungen, drei Browserregressionen zur Zusammenführung und Website-Build erfolgreich. Lupen-Leerergebnisse, Auswahl mit Zielrevision und Gesichtsauswahl der Einzelgruppe geprüft.

### BearStack 0.57.1 – in Entwicklung

- Fotos → Personen → Alle (`/photos?path=.people%2Fall`) zeigt ausschließlich benannte aktive Personen. Datenbankfilter vor der Seitenaufteilung; Anzahl und Vorschaubilder der Alle-Kachel berücksichtigen denselben Filter, auch in der nativen Galerie-API. Tag-Ordner, Personenverwaltung und direkte Personengalerien bleiben erhalten.
- WebUI: Der Gruppenbutton zum Benennen / Zuordnen zeigt bei bereits benannten Personen nur das Stiftsymbol, auch nach einer Benennung ohne Seitenwechsel. Tooltip und barrierefreie Beschriftung bleiben erhalten.
- Auch **Personen im Ordner** zeigt ausschließlich benannte aktive Personen aus dem Ordner und seinen Unterordnern. Der Datenbankfilter greift vor Fotozählung, Porträtauswahl und Seitenaufteilung; Web und native API verwenden dieselbe Abfrage.
- WebUI: Der ×-Button zum Zurücksetzen der Personensuche passt sich der Höhe des Suchfelds an; das Symbol bleibt mittig ausgerichtet. In Chromium bei 320, 390 und 1440 Pixeln Breite in allen drei Designs auf gleiche Höhe und bündige Ausrichtung geprüft.
- WebUI: **Anzeige** und **Hilfe** in der Personendetailansicht erhalten einheitliche Buttons. Geöffnete Inhalte stehen auf allen Bildschirmgrößen unter der Aktionszeile, ohne den Hilfe-Button zu verschieben. Bestehender Chromium-Browsertest sowie Layoutmessungen bei fünf Bildschirmbreiten in allen drei Designs erfolgreich.
- PATCH **0.57.1**; README, Website und OpenAPI aktualisiert. Keine Migration.
- Validierung: vollständige Go-Suite einschließlich API-Verträgen und Website-Build erfolgreich. Regressionen prüfen gemischte benannte/unbenannte Gruppen, leere Ergebnisse, Seitenaufteilung, Suche, Sortierung, Vorschauen und Sichtbarkeit. Die Button-Anpassung wurde zusätzlich mit JavaScript-/DOM-Prüfungen und dem bestehenden Chromium-Browsertest der Personenansicht geprüft.

### BearStack 0.57.0 – in Entwicklung

- WebUI: **Ähnliche Gesichter → Gesichtsketten prüfen** durchsucht auf Aufruf zusammenhängende unbenannte Gruppen mit einstellbaren 1–5 Sprüngen (Standard 2). Alle geeigneten gespeicherten Gesichter können Verbindungen bilden, auch außerhalb des Referenzlimits. Es gilt die manuelle Vorschlagsähnlichkeit; der Mindestabstand entfällt zugunsten verzweigter Alternativen. Benannte Gruppen werden weder eingeschlossen noch als Zwischenstation verwendet.
- Alle aktiven Gesichter der Kette erscheinen mit Ordnernamen, seitenweise und zunächst vollständig ausgewählt. Abwahlen bleiben über Seiten erhalten; der übliche Dialog ordnet die Auswahl atomar einer vorhandenen oder neuen benannten Person zu. Bestehende benannte Gesichter und abgewählte Gesichter behalten ihre Zuordnung. Nach Zuordnung folgt die nächste Kette; Überspringen und Neustart sind möglich.
- Suche mit kleinen Vektorblöcken, einer gleichzeitigen Suche pro Bibliothek und Zeitlimit. Mehr als 1.000 Gruppen verursachen einen ausdrücklichen Fehler statt einer still gekürzten Auswahl. Revisionen und Originaldateiprüfungen schützen vor veralteten Entscheidungen; Aktionsquittungen klären verlorene Antworten auch nach Neuladen. Keine neue Migration oder Laufzeitabhängigkeit.
- Additive Web-API, README, Website und OpenAPI aktualisiert. MINOR **0.57.0**; Android bleibt **0.14.0**.
- Validierung: vollständige Go-Suite einschließlich API-Verträgen, gezielte Race-Tests, JavaScript-/DOM-Prüfungen, sieben Browserregressionen und Website-Build erfolgreich. Geprüft werden unter anderem 607 Gesichter, die Kettengrößenbegrenzung, veränderte Originale auch auf ungesehenen Seiten, atomarer Rollback und Quittungswiederherstellung.

### BearStack 0.56.0 – in Entwicklung

- Experteneinstellungen: neue, standardmäßig ausgeschaltete Checkbox „Unbenannte Gruppen automatisch zusammenführen“. Aktiviert führt der Hintergrundabgleich ganze unbenannte Quellgruppen atomar zusammen. Bestätigte benannte Personen haben Vorrang; ohne passenden benannten Treffer folgen unbenannte Gruppen. Beide Schritte verwenden die strengen Hintergrundgrenzwerte und den Abstand innerhalb ihres jeweiligen Bereichs.
- Vergleich über gespeicherte Gruppenreferenzen, ohne erneute Bildanalyse. Geschützte Gruppenmitglieder, manuelle Trennungen, abgelehnte Paare und gemeinsames Auftreten im Foto verhindern unerlaubte automatische Zusammenführungen. Tags, Familienbeziehungen und Ablehnungen gegenüber weiteren Gruppen bleiben erhalten; automatisch verschobene Gesichter werden nicht als manuell bestätigt markiert. Ohne Option bleibt der bisherige Einzelgesichtsabgleich erhalten.
- Kompatible Migration auf Foto-Schema 34, additives API-Feld `reconcile_unnamed_groups`, README und Website aktualisiert. MINOR **0.56.0**; Android bleibt **0.14.0**.
- Validierung: vollständige Go-Suite einschließlich OpenAPI, gezielte Race-Prüfungen, drei Browserregressionen und Website-Build erfolgreich. Zusätzliche Regressionen prüfen eine ganze Gruppe mit 2.000 Gesichtern, atomaren Rollback bei Schreib- und Familienkonflikten, Quelldateiänderungen sowie erhaltene Ablehnungen nach Neustart.

### BearStack 0.55.0 / Android 0.14.0 – in Entwicklung

- Normale Fotoordner bieten im Fotoaktionen-Menü eine Personenübersicht einschließlich ihrer Unterordner, im Web und in Android. Personen, unterschiedliche Fotoanzahlen, Stern-Porträts und anschließende Fotogalerien bleiben auf diesen Bereich beschränkt. Navigation zurück zum ursprünglichen Ordner, Namenssuche, Sortierung und Blättern bleiben verfügbar.
- Wiederverwendung von Pfadindizes, begrenzten Ordner-/Medienseiten und Gesichtscache. Ignorierte und geschützte Unterordner bleiben ausgeschlossen; echte Präfixgrenzen verhindern Treffer aus ähnlich benannten Nachbarordnern. Keine zusätzliche Migration.
- Native API ergänzt optional `people_path`; ältere Server zeigen den neuen Menüpunkt nicht. MINOR **0.55.0**, Android **0.14.0**, `versionCode` **30**. README, Website und OpenAPI aktualisiert. Vollständige Go-Suite, gezielte Race-Tests, JavaScript-Prüfungen, elf Browserregressionen, 87 Android-JVM-Tests, Lint, Debug-Build, neun Emulator-UI-Tests und Website-Build erfolgreich.

### BearStack 0.54.0 – in Entwicklung

- Web: Mutter und Vater für benannte Personengruppen aus der vorhandenen Namenssuche wählen, entfernen und als verlinkte Personen ansehen. Keine neuen Gruppen durch die Elternauswahl; Bearbeitung mit `photos.edit`, Ansicht mit `photos.read`.
- Indizierte, atomare Elternzuordnungen und automatische Migration auf Foto-Schema 33. Selbstverweise, identische Eltern und Abstammungskreise werden verhindert. Umbenennungen bleiben verknüpft; Zusammenführungen übernehmen Beziehungen oder scheitern atomar bei widersprüchlichen Familien.
- Begrenzte Suchvorschläge, aktuelle Sichtbarkeitsprüfung und Wiederverwendung des bestehenden Personenwählers. README, Website und OpenAPI aktualisiert. MINOR **0.54.0**; Android bleibt **0.13.0**. Vollständige Go-Suite, gezielte Race-Tests, JavaScript-Prüfungen, zehn Browserregressionen und Website-Build erfolgreich.

### BearStack 0.53.0 / Android 0.13.0 – in Entwicklung

- Neuer virtueller Personenordner unter Fotos und in Android unter Ordner: Alle sowie eine Ebene belegter Foto-Tags führen zu Personengruppen und deren normaler, datumsgruppierter, sortierbarer Fotogalerie. Doppelte Erkennungen im selben Bild werden dedupliziert.
- Personen- und Tag-Kacheln zeigen doppelt so viele Gesichtsthumbnails wie normale Ordner nach der Einstellung (höchstens acht): Personen nach unterschiedlicher sichtbarer Fotoanzahl, Porträts mit bestehender Stern-Priorität und Gesichtscache. Abfragen und Bildabrufe bleiben seitenweise begrenzt; keine Originaldekodierung für Listen.
- Personengruppen lassen sich in der Web-Detailansicht mit bestehenden oder neuen Foto-Tags versehen. Zusammenführungen erhalten die Vereinigungsmenge; Tag-Umbenennung und -Löschung aktualisieren Zuordnungen atomar. Automatische kompatible Migration auf Foto-Schema 32.
- Namenssuche in der Personenverwaltung per Klick zurücksetzen; Filter und Sortierung bleiben erhalten. Ansehen benötigt nur Fotoleserechte, Tagging Fotobearbeitungsrechte. Private und ignorierte Gesichter bleiben ausgeschlossen.
- Native API mit optionalem `people=1`, virtuellen Ordnernamen und Gesichtsvorschau-IDs; ältere Apps behalten die bisherige Ordnerantwort. Android ergänzt variable Vorschaukacheln und Galerie-Sortierung. MINOR: BearStack **0.53.0**, Android **0.13.0**, `versionCode` **29**. README, Website und OpenAPI aktualisiert.
- Geprüft mit vollständiger Go-Suite einschließlich API-Verträgen, gezielten Race-Detector-Tests, JavaScript-Prüfungen, neun Browserregressionen sowie Android-JVM-Tests, Lint, Debug-Build und acht UI-Tests auf dem Emulator.

### BearStack 0.52.5 / Android 0.12.3 – in Entwicklung

- Android / Ähnliche Gruppen: Personennamen öffnen die zugehörige Detailansicht unter Benannte Personen, einschließlich neu benannter oder zugeordneter Seiten. Zuordnungen zeigen „Zugeordnet: Name“ mit der gewählten Zielperson; der Name bleibt bei Quittungsprüfung nach verlorener Antwort erhalten. Details laden erst beim Antippen seitenweise; die Benennungswarteschlange bleibt erhalten.
- Nach einer bestätigten Einzelaktion automatisch zur nächsten Kombination wechseln, sobald jede Seite bereits benannt oder durch Benennen, Zuordnen oder Ignorieren erledigt ist. Eine verbleibende unbenannte Seite bleibt bearbeitbar. Verlorene Antworten, erneuter Abruf und Konflikte behalten die bestehenden Quittungs- und Revisionsprüfungen.
- Validierung: 59 Emulatorprüfungen (47 Gruppen-, 8 Personen- und 4 Sprachtests), 84 JVM-Tests jeweils für Debug und Release, Lint, Debug-/minimierter Release-Build, OpenAPI-Prüfungen und Website-Build erfolgreich. Veraltete Testerwartungen für automatisches Weitergehen und Ordnerüberschriften angepasst; den Test des Paarausschlusses von der Tastaturanimation entkoppelt.
- PATCH-Bedienkorrektur: BearStack **0.52.5**, Android **0.12.3**, `versionCode` **28**. README, Android-Anleitung, deutsche/englische Hilfe, Website und OpenAPI-Beschreibung aktualisiert; keine API-Vertragsänderung oder Migration.

### BearStack 0.52.4 – in Entwicklung

- Personen-Übersicht und Benennen-Dialog in WebUI und Android: Gruppen mit sichtbaren aktiven Stern-Favoriten verwenden ausschließlich diese als Vorschau, stabil nach kleinster Gesichts-ID. Gilt auch für Lupentreffer und Favoriten außerhalb der geladenen Detailseite. Ohne Favoriten bleibt die bisherige Bildauswahl erhalten. Darstellung und Vergleichsgesichter sind getrennt; Erkennung, Referenzauswahl, Such-Ausgangsgesichter und Trefferrangfolge bleiben unverändert. Vorhandener partieller Favoritenindex, keine neue Datenmigration. PATCH: BearStack **0.52.4**, Android weiterhin **0.12.2**; Serverupdate genügt. README, Website und OpenAPI aktualisiert.

- Validierung: vollständige Go-Suite, gezielte Race-Tests, sechs Browserregressionen und alle drei Android-Serverintegrationstests erfolgreich. Große Gruppen mit 20.003 Gesichtern, mehrere/entfernte/ignorierte/geschützte Favoriten, Vorschaumetadaten, Streaming und unveränderte Abgleichreferenzen geprüft; Website-Build erfolgreich.

### BearStack 0.52.3 / Android 0.12.2 – in Entwicklung

- Android / Ähnliche Gruppen: Über den Portraits steht der enthaltende Ordner des jeweiligen Vergleichsfotos aus dem bereits geladenen Bildpfad. Ohne Bildpfad bleibt die bisherige Gruppenbeschriftung. Bei zwei unbenannten Gruppen prüft der automatische Personenabgleich nach einem erfolgreich abgeschlossenen leeren Ergebnis auch das Vergleichsgesicht der zweiten Gruppe. Höchstens zwei aufeinanderfolgende Anfragen; Treffer der ersten Gruppe ersparen die zweite. Gemeinsame Zuordnung, Streaming-Grenzen, Abbruch und explizite Wiederholung nach Fehlern bleiben erhalten. Controller- und UI-Regressionen ergänzt; README, Website, Hilfe und OpenAPI-Beschreibung aktualisiert. PATCH: BearStack **0.52.3**, App **0.12.2**, `versionCode` **27**; keine API-Vertragsänderung oder Migration.

### BearStack 0.52.2 / Android 0.12.1 – in Entwicklung

- Android, Personen: Bedienhinweise zu Einzelaktionen, Vorschau, Mehrfachauswahl und Serverkompatibilität unter **Hilfe** in einem scrollbaren Dialog gesammelt, entsprechend „Ähnliche Gruppen“. In Personenliste und Detailansicht erreichbar; Schließen erhält Ansicht und Auswahl. Keine zusätzlichen Serveranfragen oder API-Änderungen. UI-Regressionen für Liste, große Schrift und Auswahl; README und Website aktualisiert. PATCH: BearStack **0.52.2**, App **0.12.1**, `versionCode` **26**; OpenAPI-Versionsangabe aktualisiert.

### BearStack 0.52.1 – in Entwicklung

- Zentrale Labeling-SQL-Abfragen, konsistente Detail-Lesesnapshots und vollständige Aktions-Transaktionen in den `photoIndexStore` verschoben. Die Bibliothek behält Validierung, Sichtbarkeitsprüfungen und die Sperre über Commit und Cacheabgleich. Transaktionen melden Cachearbeit ausschließlich nach erfolgreichem Commit; Quittungswiederholungen bleiben ohne erneute Mutation. Gemeinsame Aufbereitung von Gesichtspfaden und Originalschlüsseln nach dem Schließen der Datenbankabfrage; finale Personenabfragen laden nur die noch fehlenden Seiteneinträge.
- Reines Paket `internal/photos/blogcontent` für Suchtext, Datum und maskiertes HTML. Dateilesen und Blogcache-Zuordnung aus dem Index-Orchestrator in `library_blog.go` verschoben; HTML- und Textsicherheit durch gezielte Regressionen abgesichert.
- Ausschließlich von Tests aufgerufene Helfer `nearestPerson`, `decodeGPX` und `normalizeRenderSettingsSnapshot` unverändert in `_test.go` verschoben.
- Validierung: vollständige Go-Suite, gezielte Race-Tests für Labeling/Blogs/HTTP, separater Produktionsbuild, alle drei Android-Serverintegrationstests und Website-Build erfolgreich.
- PATCH-Refactor ohne neue API oder Migration: BearStack **0.52.1**, Android weiterhin **0.12.0**. README, Architektur-/Testdokumentation, Website und OpenAPI-Version aktualisiert.

### BearStack 0.52.0 / Android 0.12.0 – in Entwicklung

- XMP-Sidecars auf reguläre Dateien bis 4 MiB begrenzt; Symlinks, Spezialdateien, Dateiaustausch und Wachstum werden abgefangen. Ungültige optionale Metadaten verhindern keinen Fotoabruf. Grenzwert- und Unix-FIFO-Regressionen.
- Gemeinsame Cursorpagination für unbenannte, benannte und zusammenzuführende Personen mit kleinen Sichtbarkeitsblöcken. Importierte Namensquellen bleiben berücksichtigt; Schema 31 ergänzt einen partiellen Kandidatenindex ohne Änderung der Gesichtsidentitäten. Regressionen für entfernte Verzeichnisse, neue Schutzmarkierungen, Abbruch und SQL-Plan mit 20.000 Personen.
- Android: indizierte Room-Warteschlangeneinträge ersetzen serialisierte Listen. Transaktionale Migration 3→4 erhält Reihenfolge, Kartenpositionen, offene Aktionen und Statistik; Migrationen ab Schema 1, Kontentrennung und 10.000 Einträge sind durch Gerätetests abgedeckt. Einzelabrufe und begrenzte Wiederherstellungsblöcke vermeiden das fortlaufende Lesen und Schreiben vollständiger Listen.
- Automatische OpenAPI-3.1-Validierung beider Verträge samt JSON-Schemas und lokalen Referenzen in `go test ./...` / `make test`; eigener `make test-openapi`. Reale HTTP-Erfolgs- und Fehlerantworten sowie Negativtests. Fehlerhafte YAML-Flussnotation einer Konfliktbeschreibung korrigiert. Validatoren ausschließlich in Tests, offizielles Prüfschema samt Lizenz lokal gespeichert.
- UI-Test des Suchfortschritts mit expliziter Freigabe des Testdienstes statt eines flüchtigen 500-ms-Fensters stabilisiert; alle Ladezustandsprüfungen bleiben erhalten.
- Android-HTTPS-Testfixture auf 30 Minuten begrenzt, damit vollständige Gerätesuiten einschließlich Gradle-Vorbereitung den Testserver nicht vorzeitig verlieren.
- Validierung: vollständige Go-Suite, gezielte Race-Prüfungen, JavaScript-DOM-Tests, Android-Unit-Tests, Lint und Release-Build. Alle zehn Queue-Repositorytests und drei Migrationstests bestehen im Emulator. Die bestehenden UI-Tests `smallScreenLargeFontScrollsDetailsAndKeepsDecisionsVisible` und `batchSelectionConfirmCancelAndIgnoreAtLargeFont` scheitern auch im separat gebauten Ausgangsstand `91810e5`; ihre Oberfläche ist von diesem Änderungspaket nicht betroffen.
- MINOR gemäß Projektregeln für kompatible automatische Migrationen: BearStack **0.52.0**, Android **0.12.0**, `versionCode` **25**. README, Website und OpenAPI aktualisiert.

### BearStack 0.51.0 / Android 0.11.0 – in Entwicklung

- Android, Ähnliche Gruppen: automatisch zur nächsten Kombination wechseln, sobald beide Gruppen bestätigt ignoriert wurden. Der Wechsel wartet bei verlorenen Antworten auf die Quittungsprüfung; Ladefehler und leere Vorschlagslisten werden ohne doppelte Schreibaktionen behandelt. Compose-Regressionen für beide Reihenfolgen, verlorene Antwort, erneuten Abruf und letzte Kombination. README, Website und API-Beschreibung aktualisiert. PATCH-Bedienkorrektur innerhalb der laufenden unveröffentlichten BearStack 0.51.0 / App 0.11.0; Versionen und API-Vertrag bleiben unverändert.

- Android, Benannte Personen: Mehrfachauswahl über nachgeladene Gesichtseiten, feste Auswahlleiste und Sammelaktionen für Zurücksetzen in eine neue unbenannte Gruppe, Zuordnen zu einer neuen/vorhandenen Person über den bestehenden Namensdialog einschließlich Lupenabgleich und Ignorieren mit Bestätigungsdialog. Bis zu 500 Gesichter; keine Fotolöschung. Das × behält die bestätigte Einzelentfernung der Zuordnung. Atomare, revisionsgeprüfte Serveraktionen mit dauerhaften Quittungen, Wiederherstellung nach verlorenen Antworten und Erhalt der Benennungswarteschlange. Neue Capability `named_face_batch`; ältere Server behalten die Einzelverwaltung. Server-, Repository-/ViewModel- und Compose-Regressionen für Auswahl, Grenzen, Konflikte, Rechte, Abbruch, verlorene Antworten und letzte Gesichter. MINOR-Erweiterung: VERSION **0.51.0**, App **0.11.0**, `versionCode` **24**. Keine Migration; README, Website und OpenAPI aktualisiert.

### BearStack 0.50.1 / Android 0.10.1 – in Entwicklung

- Beschriftete Gesichtsrahmen im Bildbetrachter: ignorierte Rahmen einschließlich ihrer Labels auf 20 % Deckkraft reduziert. Reine CSS-Anpassung ohne zusätzliche Abfragen; bestehender Overlay-Browsertest und Screenshots auf Desktop/Mobil geprüft. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.1; VERSION und API unverändert. README und Website aktualisiert.

- Gruppenbilder: Hilfetexte hinter einem zugänglichen Fragezeichen-Icon, Optionen standardmäßig eingeklappt und „Verbleibende ignorieren“ / „Überspringen“ direkt oberhalb und unterhalb der Gesichtskarten. Gemeinsamer Formular- und Sperrzustand verhindert doppelte Schreibaktionen; beide Sprunglinks folgen jedem Foto-Wechsel. Native Klappbereiche und Formularzuordnung funktionieren ohne JavaScript. Mobile Größen, Tastaturbedienung, Fehlerzustände und beide Aktionsleisten durch Browserregressionen geprüft. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.1; VERSION unverändert. README, Website und OpenAPI-Beschreibung aktualisiert; API-Vertrag unverändert.

- PATCH-Versionen: BearStack `VERSION` und OpenAPI auf **0.50.1**, Android-App auf **0.10.1** mit `versionCode` **23**. README, App-Dokumentation und Website einschließlich Versionsangabe aktualisiert. Keine API- oder Datenmigration.

- Thumbnail-Erzeugung: Datenbank-Deadlock beim Ableiten fehlender kleiner Vorschauen aus größeren Cache-Bildern behoben. Die Kandidatenabfrage wird vor weiteren Cache- und Indexprüfungen geschlossen; parallele Abrufe können dadurch nicht mehr beide Verbindungen mit verschachtelten Abfragen blockieren und das Ignorieren unter `/photos/people/groups` aufhalten. Regressionen für einen gleichzeitig aktiven SQLite-Schreiber, vollständige Erzeugung mit nur einer Verbindung sowie fehlende, leere, veraltete und ältere Cache-Dateien. PATCH-Korrektur in 0.50.1. README und Website aktualisiert; API und Cache-Format unverändert.

- Gruppenbilder: Ignorieren und anschließendes Nachladen begrenzen jeden Abruf einschließlich JSON-Antwort auf 20 Sekunden, damit ein ausbleibender Server die Bedienung nicht dauerhaft sperrt. Unbestätigte Schreibantworten und fehlgeschlagene Konflikt-Aktualisierungen erlauben weitere Änderungen erst nach frischem Lesen; „Ansicht erneut laden“ wiederholt ausschließlich GET. Fotovergrößerung bleibt nutzbar, Navigation nach Ablauf wieder verfügbar. Abbrüche geben den UI-Status frei und räumen Timer auf; keine automatischen Wiederholungen. Browser-Regressionen für ausbleibende Header, unvollständige Antwortdaten, bestätigte Speicherung und Konflikt mit hängendem Nachladen ergänzt. PATCH-Robustheitskorrektur in 0.50.1. README, Website und OpenAPI-Beschreibung aktualisiert, Serververtrag unverändert.

- Android / Ähnliche Gruppen: luftigere vertikale Abstände zwischen Portraits, Details und Aktionen, deutlicher Abstand zu den gemeinsamen Benennungsvorschlägen sowie größere Trefferkarten mit mehr Innenabstand. Scrollbarer Vergleich und feste Entscheidungsbuttons bleiben erhalten. PATCH-UI-Korrektur in BearStack 0.50.1 / App 0.10.1. README und Website aktualisiert; API unverändert.

### BearStack 0.50.0

- Android: Vorschau beim Halten eines Gesichts und zugehöriges WLAN-Vorladen verwenden `large_preview_size` statt der vollständigen Originaldatei. Optionaler Modus `size=large_preview_size` am bestehenden Labeling-Originalendpunkt liefert das ganze Foto als WebP aus dem vorhandenen Thumbnail-Cache; ohne Parameter bleibt die Originalauslieferung unverändert. Gesichtsrechte, Seitenverhältnis, Range-Requests, 2048-Pixel-Dekodierziel und begrenzter App-Cache bleiben erhalten; eigener Cache-Schlüssel für die große Vorschau. Größenwechsel, Cache-Wiederverwendung, Zugriffsschutz und Android-URL getestet. MINOR-API-Erweiterung mit Performance-Korrektur innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION und versionCode unverändert. README, Website und OpenAPI aktualisiert. App und Server gemeinsam aktualisieren; ältere Server liefern weiterhin Originale.

- Android / Ähnliche Gruppen: gemeinsamer automatischer Lupenabgleich unter Ignorieren und Stift ausschließlich bei zwei unbenannten Gruppen. Nur das Vergleichsgesicht der ersten Gruppe wird abgeglichen; bis zu 20 Streaming-Treffer dienen als globale Benennungsvorschläge für beide Gruppen. Ein Klick ordnet beide atomar per `name_merge` zu. Eine Suchanfrage, gemeinsamer Lade-/Leer-/Fehlerzustand samt Wiederholung, Wiederverwendung fertiger Treffer für das aktuelle Paar und Abbruch unvollständiger Suchen bei Hintergrund, Dialogen und Navigation. Einzelaktionen verwerfen die gemeinsame Liste. Zielrevision wird nur für den angeklickten Treffer geladen; beide Gruppenrevisionen, verlorene Antworten und Doppelaktionen behalten die vorhandenen Revisions- und Quittungsprüfungen. Controller-, UI-, Streaming-, Konflikt- und Wiederaufnahme-Regressionen ergänzt; README, Website und OpenAPI-Beschreibung aktualisiert. MINOR-Erweiterung einschließlich Bedienkorrektur innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. Kein geänderter Serververtrag und keine Migration.

- Android: Start mit gespeichertem Profil über einen gestalteten Ladebildschirm statt des Anmeldeformulars. Lokale Fotos sind bereits während des Verbindungsaufbaus und ohne Servereinrichtung erreichbar; nach fehlgeschlagener Anmeldung automatisch lokale Galerie mit Fehlerhinweis und Wiederholung. Acht Sekunden Zeitlimit für die Sitzungsaushandlung, Erhalt von Profil und Zertifikatsbindung, keine automatischen Wiederholungsschleifen; erneutes Verbinden erhält die lokale Ansicht. Die Funktion „Gruppen manuell kombinieren“ samt Auswahloberfläche, Controller und Gruppenabrufen entfernt; alte offene Aktionen bleiben auflösbar. Regressionen für Start, Serverausfall, Zeitlimit, Anmeldung, Zertifikate, lokale Originale und Wiederverbindung ergänzt. README, Website und API-Beschreibung aktualisiert. PATCH-Bedien- und Robustheitskorrektur innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. Kein geänderter Serververtrag und keine Migration.

- Android / Ähnliche Gruppen: gleich große, bündig ausgerichtete Portraits direkt unter den Gruppenüberschriften statt unabhängig zentrierter Bilder mit großen Leerflächen. Namen und Gesichtsanzahlen stehen in gemeinsamen Zeilen darunter; Ignorieren und Stift kompakt nebeneinander mit mindestens 48 dp großen Touchflächen. Lange Namen, einseitige Aktionen und Ergebnisanzeigen verschieben die Portraits nicht. Bei großer Schrift umbrechende Einzelaktionen und bei Platzmangel scrollbarer Vergleichsbereich mit weiterhin festen Entscheidungsbuttons. Layout- und Bedienregressionen ergänzt; README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. Keine zusätzlichen Abfragen, API-Änderung oder Migration.

- Ähnliche Gesichter / Ähnliche Gruppen: WebUI und Android-App warnen vor dem Zusammenführen zweier bereits benannter Gruppen und verlangen eine zusätzliche Bestätigung, auch bei identischen Namen. Die Warnung nennt beide Gruppen und den erhaltenen Namen der zweiten Gruppe; Abbrechen verändert nichts. Ohne JavaScript verlangt die WebUI eine Bestätigungs-Checkbox; Getrennt lassen bleibt direkt möglich. Bestehende Revisionsprüfungen und Aktionsquittungen bleiben erhalten, ohne zusätzliche Abfragen oder Gesichtsvergleiche. Browser- und Android-Regressionen, README, Website und OpenAPI-Beschreibung aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert, keine Migration oder API-Vertragsänderung.

- Ähnliche Gruppen: Nur unbenannte Gruppen erhalten je Vergleichsseite Ignorieren und einen Benennen-/Zuordnen-Stift. Einzelaktionen ändern ausschließlich diese Gruppe; das Paar bleibt für die Bearbeitung der anderen Seite sichtbar. Nach einer bestätigten Einzelaktion ersetzen Weiter (Android) beziehungsweise Ausblenden (Web) die gemeinsamen Entscheidungsbuttons. Bestehende revisionsgeprüfte Labeling-Aktionen und Aktionsquittungen, Wiederaufnahme nach verlorenen Antworten und unveränderte Vorschaubilder. Additive Capability `merge_side_actions` sowie optionale `exclude_source`/`exclude_target` beim nächsten App-Vorschlag; überspringt das aktuelle Paar ohne Ablehnung oder Vektorvergleich. Backend-, Browser- und Android-Regressionen ergänzt; README, Website und OpenAPI aktualisiert. MINOR-Erweiterung innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert, keine Migration.

- Automatische Gesichtszuordnung bei der Erkennung: benannte Gruppen werden zuerst geprüft; nur ohne Treffer oberhalb von Ähnlichkeit und Mindestabstand folgt der Abgleich mit unbenannten Gruppen. Der Abstand gilt jeweils zwischen unterschiedlichen Gruppen desselben Bereichs. Mehrere Referenzgesichter derselben Gruppe, einschließlich Favoriten, konkurrieren nicht miteinander. Aktuelle Namen werden einmal pro Foto indexgestützt gelesen; bei benanntem Treffer entfällt der unbenannte Vektorvergleich. Regressionen für Priorität, Mehrdeutigkeit, Favoriten, Sichtbarkeit, Umbenennen und mehrere Gesichter pro Foto; README, Website und OpenAPI aktualisiert. PATCH-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION bleibt unverändert, keine Migration oder Änderung der API-Struktur.

- Foto-Info: Korrekturen bereits benannter Personen ändern ausschließlich das angeklickte Gesicht im aktuellen Foto. Neue Namen trennen nur dieses Gesicht in eine eigene Person ab; vorhandene Zielpersonen erhalten nur dieses Gesicht. Andere Fotos und weitere Gesichter derselben Person im selben Bild bleiben unverändert. Der Dialog kennzeichnet den Geltungsbereich; ein unverändert gespeicherter Name erzeugt keine doppelte Person. Unbenannte Gruppen werden weiterhin gemeinsam benannt. Bestehender atomarer Einzelgesicht-Endpunkt, keine neuen Abfragen oder Migrationen. Browserregressionen für Mehrfachvorkommen, unveränderte Geschwister, Namensauswahl und Fehler/Wiederholung; README, Website und OpenAPI-Beschreibung aktualisiert. PATCH-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API-Verträge unverändert.

- Foto-Info: neuer Sitzungs-Toggle für beschriftete Gesichtsrahmen im normalen Bild, auch bei geschlossener Sidebar. Bleibt bei Bild-/Seitenwechsel und Neuladen im Browser-Tab aktiv, bis er ausgeschaltet wird. Gemeinsame Rahmenbeschriftung mit dem Einrahmen-Dialog, synchroner Zoom und Verschieben; Wiederverwendung geladener Daten ohne neue Gesichtserkennung. Fokusrahmen und Rundung des Namensfeldes im Einrahmen-Dialog werden nicht mehr vom scrollbaren Eingabebereich beschnitten. Browserregressionen für Desktop/Mobil, Geometrie, Bildwechsel, verspätete Antworten und gesperrten Sitzungsspeicher; README und Website aktualisiert. MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0; VERSION und API-Verträge unverändert.

- Gesicht einrahmen: Namen stehen vollständig außerhalb oberhalb der Bounding Box, in kleinerer Schrift und ohne Abschneiden auf die Rahmenbreite. Desktop- und Mobilansicht durch Browserregression abgesichert; README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Gesicht einrahmen: gemeinsame Namensvervollständigung mit Gesichtsvorschauen wie im Benennen-Dialog. Die Auswahl einer Person oder eines neuen Namenseintrags bestätigt den gezeichneten Rahmen und schließt nach erfolgreichem Speichern. Touch-Auswahl funktioniert auch nach dem Zeichnen; Scrollgesten bestätigen nichts. Alle vorhandenen Rahmen erscheinen mit kleinen Namen am Rand, einschließlich gekennzeichneter unbenannter und ignorierter Gesichter; korrekte Ausrichtung bei Fenstergrößenwechsel ohne zusätzliche Serverabfragen. Browserregressionen für Maus, Touch, Tastatur, Fehlerfälle und Bildwechsel ergänzt. README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API-Verträge unverändert.

- Foto-Infosidebar: Die Metadaten zeigen unter „Ordner“ den normalisierten Namen des enthaltenden Fotoordners nach den Regeln der Galerie; im Hauptverzeichnis „Fotos“. Das additive JSON-Feld `folder_name` wird über die vorhandenen Metadatenanfragen ohne zusätzliche Datei- oder Datenbankzugriffe geliefert. Normalisierung, Einzel-/Batch-API, DOM, Bildwechsel und mobile Sidebar durch Tests abgesichert. README, Website und OpenAPI aktualisiert. MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0; VERSION bleibt unverändert.

- Personennavigation: Textbuttons und Einstellungszahnrad verwenden einheitlich mindestens 44 Pixel Höhe, auch auf dem Desktop. README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Personenübersicht und Personendetail verwenden gemeinsame CSS-Regeln für Kachelgrößen S/M/L, Raster, Bilder, Rahmen, Abstände und Auswahlmarkierung. Checkbox, Ignorieren und Stift haben einheitliche Positionen und 44-Pixel-Bedienflächen; der Vergleichsstern bleibt in der Detailansicht. README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Personendetail: Gesichtskarten zeigen die Auswahlcheckbox oben links, Ignorieren (×) oben rechts, den Vergleichsstern unten links und den Stift unten rechts. 44-Pixel-Bedienflächen; die Kennzeichnung manueller Zuordnung steht kollisionsfrei unten mittig. README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Personennavigation: „← Fotos“, „Gruppenbilder“, „Ähnliche Gesichter“ und ein Zahnrad mit Tooltip und zugänglichem Namen für die Gesichtserkennungs-Einstellungen. Rücklinks heißen „← Alle Personen“. README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Personenansicht: Sortierung und Dreipunkt-Menü in die Filterbutton-Zeile verschoben, mit Umbruch auf schmalen Bildschirmen. Ein zusätzlicher Kasten erscheint nur für die Namenssuche in „Alle“ und „Benannt“. Bedienung mit und ohne JavaScript bleibt erhalten. README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Personenansicht: dünner gemeinsamer Rahmen um „Alle“, „Benannt“ und „Unbenannt“; „Ignoriert“ steht außerhalb, da ignorierte Gesichter nicht in „Alle“ enthalten sind. Gruppierung auch auf schmalen Bildschirmen und für Screenreader. README und Website aktualisiert. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Android: Datumswähler im Tab Fotos links neben dem Dreipunkt-Menü. Sprung zum ersten Eintrag des gewählten Tages oder zum zeitlich nächsten vorhandenen Tag; gleicher Abstand bevorzugt den älteren Tag. Weiterscrollen in beide Richtungen, unveränderter Stream und begrenzter Seitenspeicher. Abbrechbarer neuer GET-Endpunkt `/api/photos/v1/browse/date` mit vorhandenen Datumsindizes, konsistenter Lesetransaktion, Rechteprüfung und höchstens einer Zielseite im App-Abruf. Aufnahmetag in gespeicherter Zeitzone, Änderungstag als Ersatz; verständliche Fehler und Wiederholen bei zwischenzeitlich verschobenem Ziel. Index-/HTTP-/API-/Navigations-/UI-Tests und Benchmark bis 300.000 Einträge. MINOR-Erweiterung innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. Keine Migration; README, Website und OpenAPI aktualisiert.

- Personenansicht: den nach Einführung der direkten Filterbuttons überflüssigen Link „Alle Filter aufheben“ entfernt, einschließlich ungenutzter CSS-Regeln. Bestehende Filter- und Suchtests angepasst; README und Website aktualisiert. PATCH-Korrektur innerhalb der unveröffentlichten 0.50.0, VERSION und API unverändert.

- Android: zentralen Screenshot-Schutz entfernt. Screenshots, Bildschirmaufnahmen und die Vorschau im App-Umschalter sind in der gesamten App wieder möglich, einschließlich Bildbetrachter und Dialogen. Keine zusätzlichen Abfragen oder Änderungen am API-Vertrag. PATCH-Bedienkorrektur innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. README und Website aktualisiert.

- Android: Teilen-Button im Einzelbildbetrachter für das aktuelle Originalfoto, einschließlich Karten und lokaler Fotoordner. Android-App-Auswahl mit temporärem Lesezugriff, unveränderten Originalmetadaten und ohne Weitergabe von Serverlinks oder Zugangsdaten. Serveroriginale erst auf Anfrage, begrenzter Übertragungspuffer und privater FileProvider-Cache (256 MiB je Bild, höchstens acht Dateien/512 MiB); lokale MediaStore-URIs ohne Kopie. Abbrechbare Vorbereitung, lokalisierte Fehler, Diashow-Pause und Bereinigung bei Fehler/Abbruch sowie alter Cache-Dateien vor dem nächsten Share. Cache-, Provider-, Intent-, Abbruch-, Lokaldatei- und UI-Regressionen ergänzt. MINOR-Erweiterung innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. README, Website und OpenAPI-Beschreibung aktualisiert; kein geänderter Serververtrag.

- Personenansicht: direkte Filterbuttons „Alle“, „Benannt“, „Unbenannt“ und „Ignoriert“ mit sichtbarem Aktivzustand und ohne zusätzlichen Suchklick, auch ohne JavaScript. Wechsel erhält die Sortierung und beginnt auf Seite 1; unbenannte und ignorierte Ansichten zeigen keine Namenssuche und übernehmen keinen Suchtext beim Filterwechsel. Kompakte mobile Paginierung mit 44-Pixel-Pfeilbuttons und zugänglichen Seitennamen, auch nach AJAX-Aktionen. Checkboxen und Größenauswahl im Anzeigemenü korrekt ausgerichtet; Dreipunkt-Menüs verwenden vertikal zentrierte SVG-Punkte. Zusammenführungsvorschläge ordnen Zusammenführen/Stift und Getrennt lassen in einheitlichen Zeilen an. Browser-Regressionen für Filterwechsel mit/ohne JavaScript, Mobilgrößen und Bedienbarkeit. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION unverändert. README, Website und OpenAPI-Beschreibung aktualisiert; API-Parameter unverändert.

- Lupen-Gesichtabgleich optimiert: SQLite beginnt die Kandidatenprüfung bei höchstens 512 Gesicht-IDs pro Paket, unabhängig von Optimiererstatistiken. Unveränderliche Referenzstände ausschließlich benannter Gruppen, Wiederverwendung unveränderter Gruppen und abbrechbares Warten auf Cache-Aufbau; kein Halten der Schreibsperre während Vergleich oder Streaming. Ausstehende Referenzauswahl wird im Modal nur für benötigte benannte Gruppen gelesen, ohne globalen Neuaufbau. Erste Treffer sofort, weitere Zwischenstände höchstens alle 100 ms und Abschluss immer; Browser bündelt Zwischenstände pro Frame und verwirft sie bei Abbruch/Fehlern. Alle infrage kommenden benannten Gruppen und Favoriten bleiben berücksichtigt; aktuelle Rechte, Revisionen und Schwellen werden geprüft. Cache-Bereinigung beim Löschen der Gesichtsdaten und Schließen der Bibliothek. SQL-Plan-, Vergleichs-, Parallelitäts-, Race- und Browser-Regressionen sowie Vorher-/Nachher-Messungen bis 300.000 Referenzen. PATCH-Optimierung innerhalb der unveröffentlichten 0.50.0; VERSION unverändert, keine Migration oder Änderung des API-Schemas. README, Website und OpenAPI aktualisiert.

- Personenübersicht: Sortierung nach Name, Anzahl unterschiedlicher Fotos, Vorschauordner und Datum des neuesten sichtbaren Fotos, jeweils auf-/absteigend. Zeitzonen normalisiert, Dateiänderungszeit als Ersatz; ignorierte/private Fotos aus aktiven Gruppen ausgeschlossen. Ignorierte Gesichter unterstützen dieselben Sortierarten mit Bezug auf ihr Foto beziehungsweise ihre ignorierte Gruppe. Serverseitige Sortierung vor der Pagination mit stabilen ID-Gleichständen und weiterhin maximal 60 Ergebnissen. Seitenauswahl, Filter, AJAX-Aktionen, Rückkehr aus Personendetails und Browser-Speicherung erhalten die Sortierung; ohne JavaScript über das Suchformular nutzbar. Namensindex bleibt erhalten, größere Sortierungen können temporär auf Datenträger auslagern. SQL-, HTTP-, DOM-, Browser- und Performancetests ergänzt. Additiver `sort`-Parameter und JSON-Metadaten; keine Schemaänderung. MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0, VERSION bleibt unverändert. README, Website und OpenAPI aktualisiert.

- Android: optionaler Hauptordner „Dieses Gerät“ unter „Ordner“, aktivierbar über „Weitere Optionen → Einstellungen → Lokale Fotoordner anzeigen“. Lokale Fotoordner aus MediaStore mit Zählern und zwei Vorschauen, paginiertem Raster, Vollbild, Zoom, Diashow und Dateiinformationen ohne Upload oder Serverzugriff. Vollständige/ausgewählte Fotofreigabe, Android-Einstellungslink, Aktualisierung bei Medienänderungen und erneute Berechtigungsprüfung im Vordergrund. Abbrechbare Hintergrundabfragen, begrenzte Fotoseiten und separater 16-MiB-Bildcache ohne Disk-Cache; Datenträger bleiben getrennt. Berechtigungs-, Provider-, Paging-, Abbruch- und UI-Regressionen; vorhandenen Uhrzeit-Test für CLDR-Leerzeichen aktueller JDKs portabel gemacht. MINOR-Erweiterung innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. README, Website und OpenAPI-Beschreibung aktualisiert; keine Serververtragsänderung.

- Android: manuelles Kombinieren beliebiger Personengruppen mit fortlaufendem Raster, Filter für ausschließlich unbenannte oder alle Gruppen, Mehrfachauswahl per kurzem Tippen und Originalvergrößerung per Halten. Ab zwei, bis höchstens 60 Gruppen: Kombinieren oder Kombinieren und benennen. Begrenzter Seitenspeicher, Nachladen beim Vor-/Zurückscrollen und Erhalt der Scrollposition nach Aktionen. Neuer Labeling-Gruppenendpunkt und Capability `manual_merge`; `merge_groups`/`name_groups` prüfen sämtliche Revisionen und speichern alle Gruppenänderungen samt Quittung atomar, ohne Migration. Favoriten und ignorierte Gesichter bleiben erhalten. Regressionen für Cursor, Speichergrenzen, Gesten, Namenskonflikte, Rollback und verlorene Antworten. MINOR-Erweiterung innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION, App-VERSION und versionCode bleiben unverändert. README, Website und OpenAPI aktualisiert.

- Android: Die Lupe beim Benennen nutzt das vorhandene NDJSON-Streaming. Zwischenstände ersetzen die Trefferliste bereits während des Abgleichs; Ladehinweis und Trefferzahl bleiben bis zum Abschluss sichtbar. Auswahl während der Suche, Abbruch und Revisionsprüfung bleiben erhalten. Begrenzte Einzelantworten und nur der neueste wartende Zwischenstand verhindern wachsenden Speicherbedarf; normale JSON-Antworten bleiben kompatibel. Unvollständige oder fehlgeschlagene Streams verwerfen vorläufige Treffer. Netzwerk-, ViewModel- und UI-Regressionen ergänzt. MINOR-Erweiterung innerhalb der unveröffentlichten App 0.10.0; App-VERSION/versionCode und BearStack-VERSION 0.50.0 bleiben unverändert. README, Website und OpenAPI aktualisiert.

- Konservativer Cleanup: nur für Tests benötigte Mail-/OCR-Methoden aus den Server-Schnittstellen entfernt, Mail-Testhelfer nach `_test.go` verschoben sowie ein ungenutztes Testfeld und die nicht referenzierte Go-Konstante `DefaultFaceReferenceLimit` entfernt. Der SQL-Standardwert von 30 bleibt unverändert. Browser-Tests simulieren fehlende Portraits über einen garantiert neuen, mit 404 beantworteten Bildabruf und verwenden beim Login ein ausdrückliches Rücksprungziel statt des veränderlichen Startseitenwerts. Kein geändertes Anwendungsverhalten, keine API- oder Schemaänderung; Refactoring innerhalb der unveröffentlichten 0.50.0, VERSION bleibt unverändert.

- Dokumentverarbeitung: gemeinsame Formatklassifikation für Renderer und SQL-Thumbnail-Auswahl; MIME-Parameter, Schreibweise und Leerzeichen werden konsistent behandelt. Filterung bleibt vor der ID-basierten Pagination, ohne Schemaänderung. Mail- und OCR-Abläufe in eigene Servicepakete mit schmalen Schnittstellen ausgelagert; HTTP, Audit-Anbindung und Worker-Lifecycle bleiben im Server. Gemeinsame Soffice-Test-Fixture, Format-/Service-Regressionen und Benchmark für 50.000 Dokumente ergänzt. PATCH-Korrektur und Refactoring innerhalb der unveröffentlichten 0.50.0; VERSION bleibt unverändert. README, Website und OpenAPI-Beschreibungen aktualisiert.

- Mobile Einzelpersonenansicht: kompakte Kopfzeile mit Gruppenstift, kurze Auswahlaktion und gleichmäßig gefülltes Gesichtsraster. Anzeige, Hilfe und Mehr-Menü öffnen sich einzeln über die volle Breite im Seitenfluss, ohne Überlagerungen. Desktop-Funktionen und gespeicherte Anzeigeoptionen bleiben erhalten. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert. README und Website aktualisiert.

- Ähnliche Gruppen: WebUI und Android-App zeigen den Ähnlichkeitswert des aktuellen Vergleichspaars klein und mittig über den Entscheidungsbuttons, mit zwei Nachkommastellen. Additives `score` im Labeling-Endpunkt; ältere Server ohne Wert bleiben kompatibel. Keine zusätzliche Vektorsuche. MINOR-Erweiterung innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0; VERSION und versionCode unverändert. README, Website und OpenAPI aktualisiert.

- Gesichtserkennung: ausklappbarer Expertenbereich mit getrennten Ähnlichkeits- und Abstandswerten für erste automatische Zuordnung, automatische Hintergrundzuordnung und manuelle Vorschläge einschließlich Gesichtssuche. Ähnlichkeit 0,4–0,7, Mindestabstand 0,0–0,2; bisherige Standards bleiben erhalten. Ein positiver Vorschlagsabstand verlangt einen eindeutig besten Treffer. Serverseitige Validierung, persistente Einstellungen und automatische Foto-Schema-Migration 30. Änderungen verwerfen offene Vorschläge und planen einen neuen Abgleich; Ablehnungen bleiben erhalten. MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0, daher VERSION unverändert. README, Website und OpenAPI aktualisiert.

- Foto-Infobar: neuer Button hebt das Ignorieren aller Gesichter des geöffneten Fotos auf. Namen, Gruppen und manuelle Markierungen bleiben erhalten; keine neue Bildanalyse und keine Änderungen an anderen Fotos. Atomarer, revisionsgeprüfter Endpunkt `POST /photos/faces/unignore`; bei unbestätigter Antwort ist eine Aktualisierung vor der nächsten Änderung erforderlich. MINOR-Erweiterung im bereits unveröffentlichten Release 0.50.0; VERSION unverändert. README, Website und OpenAPI aktualisiert.

- Ignorierte Gesichter: „Wiederherstellen“ holt das gewählte Gesicht ohne Namen zurück; der neue Stift öffnet den gemeinsamen Benenn-/Zuordnungsdialog und stellt zugleich ausschließlich dieses Gesicht wieder her. AJAX behält Filter und unveränderte Thumbnails; einfache Wiederherstellung funktioniert ohne JavaScript. Neue kompatible Formularaktion `restore` prüft atomar, dass alle gewählten Gesichter noch ignoriert sind, und weist veraltete Anfragen mit HTTP 409 zurück. MINOR-Erweiterung im bereits unveröffentlichten Release 0.50.0; VERSION unverändert. README, Website und OpenAPI aktualisiert.

- Android: Lupe im Benenn-Modal sucht anhand des angezeigten Gesichts nach ähnlichen benannten Personen, auch beim gemeinsamen Benennen ähnlicher Gruppen. Begrenzte Treffer mit Referenzportraits, Lade-/Leer-/Fehlerzustände und Abbruch bei Texteingabe, Schließen oder Hintergrundwechsel. Zielrevision wird nur für den gewählten Treffer geladen; bestehende Zuordnungsquittungen bleiben erhalten. MINOR-Erweiterung innerhalb der unveröffentlichten App 0.10.0; App-VERSION/versionCode und BearStack-VERSION bleiben unverändert. Bestehender Gesichtsabgleich-Endpunkt ohne Vertragsänderung; README, Website und OpenAPI erläutern die App-Nutzung.

- Personen-Einzelansicht renoviert: kompakter Kopf, einheitliche Gesichtskarten mit Stern, Ignorieren und Stift; gemeinsame Dialoge für ganze Gruppen, einzelne Gesichter und Auswahl. Aktionsleiste nur bei Auswahl, gespeicherte Thumbnailgröße und optionale Pfade, einklappbare Hilfe und keine Ein-Seiten-Paginierung. AJAX-Aktualisierung erhält unveränderte Bildknoten; unbestätigte Verschiebe-/Ignorieraktionen sperren bis zum erneuten Laden. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API-Verträge unverändert. README und Website aktualisiert.

- Gruppenbilder: feste horizontale Bilderleiste am unteren Rand, markiertes aktuelles Foto und direkter Sprung zu einer beliebigen geladenen Vorschau. Cursorbasiertes Nachladen in beide Richtungen, kleine Galerie-Thumbnails nur in Sichtnähe und maximal 96 Einträge im Browser. Neuer lesender Modus `format=strip` liefert höchstens 33 Metadateneinträge mit denselben Sichtbarkeits- und Schwellenprüfungen wie die Warteschlange. MINOR-Erweiterung im bereits unveröffentlichten Release 0.50.0; VERSION unverändert. README, Website und OpenAPI aktualisiert.

- Ähnliche Gruppen in WebUI und Android: Sind beide Gruppen unbenannt, öffnet ein Stift die Namenssuche zum gemeinsamen Benennen oder Zuordnen. Abbrechen bleibt ohne Änderung; Speichern prüft beide Gruppen und die Zielperson und führt alle Änderungen samt Aktionsquittung atomar aus. Neue kompatible Labeling-Aktion `name_merge` mit Capability `merge_naming`, Revisions- und Duplikatprüfung; keine Migration. MINOR-Erweiterung innerhalb der unveröffentlichten BearStack 0.50.0 / App 0.10.0, daher VERSION und App-versionCode unverändert. README, Website und OpenAPI aktualisiert.

- Gesicht einrahmen: 420-Pixel-Begrenzung der allgemeinen Dialogregel aufgehoben. Nahezu fenstergroße Zeichenansicht am Desktop mit seitlichen Eingaben, mobil Vollbild mit großer Zeichenfläche und festen Speicherbuttons. Beim erneuten Öffnen startet die Ansicht wieder oben. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Ähnliche Personengruppen: Aktionen sperren nur den betroffenen Vorschlag. Andere Karten bleiben während Speichern und Nachladen bedienbar. Parallele Entscheidungen teilen sich eine Aktualisierung; überholte Listenstände werden verworfen. Unbestätigte Aktionen bleiben pro Karte bis zur erfolgreichen Aktualisierung gesperrt. PATCH-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Foto-Info: alle Gesichtsfunktionen in einer kompakten Zeile mit Gesichtsicon, Lupe, Auswahlrahmen und überarbeitetem Refresh-Pfeil. Einheitliche 44-Pixel-Bedienflächen, Tooltips und zugängliche Namen; der zusätzliche Zähl- und Bedienhinweis unter der Symbolleiste entfällt. PATCH-UI-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API unverändert.

- Gesichtserkennung findet auch große Porträtgesichter: zusätzlicher 320-Pixel-Suchlauf mit Rückrechnung aller Rahmen und Landmarken, stabiler Bevorzugung vorhandener Treffer und unveränderter Erkennungsschwelle. Merkmalsextraktion weiterhin auf der größeren Vorlage; Modell und Vektoren kompatibel. Reproduziert mit einem zuvor übersehenen Porträt und abgesichert durch öffentliche Modelltests für große und gemischte Gesichtsgrößen. Gesichtsdienst neu bauen/starten und betroffene Fotos erneut analysieren. PATCH-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION unverändert.

- Fehlende Gesichter lassen sich in der Foto-Info mit Maus, Touch oder Tastatur einrahmen und atomar benennen oder einer vorhandenen Person zuordnen. Ohne Erkennungsdienst; manuelle Regionen bleiben bei erneuter Analyse erhalten, ohne künstliche Vergleichsvektoren oder doppelte überlappende Treffer. Quellrevision, Rechte-, Geometrie- und Mengenprüfungen; additive API und automatische Foto-Schema-Migration 29. MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0; VERSION unverändert.

- Foto-Info: kompaktes Refresh-Symbol rechts neben der Erkennung; der Benennen-Dialog kann das angezeigte unbenannte Gesicht ignorieren. Bestehende Einzelgesicht-API mit Revisionsprüfung, Konfliktbehandlung und Aktualisierung ohne Neuladen. PATCH-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION und API-Verträge unverändert.

- Foto-Infopanel bietet gezielte Gesichtserkennung und automatische Zuordnung für das aktuelle Bild, unabhängig vom pausierten Hintergrundlauf. Erkannte Gesichter lassen sich direkt über den bestehenden Benennen-Dialog samt Live-Lupe bearbeiten. Gemeinsame serielle Bildanalyse, Quell- und Rechteprüfungen, Erhalt manueller Korrekturen und abbrechbare Requests. Additive Endpunkte `POST /photos/faces/analyze` und `GET /photos/faces` (auch für Fotos ohne erkannte Gesichter); MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0, VERSION unverändert.

- Modal-Lupe zeigt geprüfte Kandidaten bereits während des Gesichtsabgleichs und ergänzt oder sortiert die Liste fortlaufend. Abbrechbarer NDJSON-Stream, frühe Auswahl, stabile Tastaturauswahl und wiederverwendete Vorschaubilder; bestehende JSON-Antwort bleibt kompatibel. Kleine Vergleichspakete begrenzen zusätzliche Datenbankarbeit. MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0; VERSION unverändert.

- Benennen-Dialog zeigt unter der Fotovorschau den nach Galerieregeln aufbereiteten Bildpfad mit lesbaren Ordnerdaten und unverändertem Dateinamen. Personen- und Gruppenbilderansicht aktualisieren ihn mit dem angezeigten Gesicht, ohne zusätzliche Abfragen. Ergänzendes JSON-Feld `people[].display_path`; Erweiterung innerhalb der unveröffentlichten 0.50.0, VERSION unverändert.

- Lupe im Web-Benennen-Dialog gleicht das angezeigte Gesicht auf Klick live mit benannten Referenzgruppen ab. Bis zu 20 nach Ähnlichkeit sortierte Kandidaten erscheinen in der vorhandenen Vervollständigung. Gemeinsamer Referenzcache, aktuelle Sichtbarkeitsprüfungen, abbrechbare Anfragen und additiver GET-Endpunkt; keine erneute Bildanalyse. MINOR-Erweiterung innerhalb der unveröffentlichten 0.50.0, VERSION unverändert.

- Erkennungsstatus zeigt Zuordnungen zu bestehenden Gruppen, neu gebildete Gruppen und den prozentualen Zuordnungsanteil, unabhängig vom nachträglichen Hintergrundabgleich. Foto-Schema 28 speichert die Erkennungsentscheidung; Altbestände bleiben ausdrücklich nicht erfasst. Additive JSON-Statusfelder, indexbasierte Zählung und automatische Migration. MINOR-Funktion innerhalb der unveröffentlichten 0.50.0; VERSION bleibt 0.50.0.

- Ähnliche Personengruppen lassen sich ohne Seitenneuladen zusammenführen oder getrennt lassen. Vorschläge rücken nach, unveränderte Karten bleiben erhalten; Konflikte und Netzwerkfehler erscheinen direkt auf der Seite. Bestehende JSON-Endpunkte und Revisionsprüfungen bleiben unverändert. PATCH innerhalb der unveröffentlichten 0.50.0, kein zusätzlicher Versionssprung.

- Mobile Personenansicht: zweispaltige Navigation, kompaktere Filter und Anzeigemenü direkt neben den Filteraktionen statt einer eigenen Zeile. Keine zusätzlichen Abfragen oder Änderungen an API und Datenformaten. PATCH-Korrektur innerhalb der unveröffentlichten 0.50.0; VERSION bleibt unverändert.

- Browserkarten prüfen neue Ordnersperren vor dem Lesen der Marker-Metadaten; Pfade, Namen und GPS-Koordinaten geschützter Unterordner bleiben bereits in der ersten Kartenantwort verborgen.
- Große optionale Kartenindizes entstehen außerhalb des 30-Sekunden-Startlimits im Hintergrund mit dateibasierter Sortierung. Kartenaufrufe warten abbrechbar auf den Aufbau; normale Galerie-Lesezugriffe bleiben verfügbar. Ein unterbrochener Aufbau wird beim nächsten Start wiederholt.
- Browser-GPX-Inventar nutzt den vorhandenen Index in begrenzten Paketen und parst Tracks erst bei Bedarf. Der bisherige Dateisystemlauf bleibt bis zum vollständigen GPX-Backfill erhalten.
- Foto-Schema 27 ergänzt transaktionale Revisionen je Ordner, Medientyp und Sichtbarkeit. Verschieben und Löschen invalidieren betroffene Teilbäume und Vorfahren; Änderungen außerhalb der Auswahl lassen warme Routencaches bestehen. Bestehende Daten bleiben erhalten; alte globale Cache-Revisionen werden einmalig ersetzt.
- Android trennt Anmeldung, Zertifikatsbestätigung, HTTP-Verbindung, Bildcache und Galerie-Lebenszyklus in `AppSession` von der Personenwarteschlange. Gemeinsame Bildhilfen liegen im Paket `media`; reine Lesekonten öffnen auch über die App-Fassade keine Personendatenbank.
- Testmatrix um minimierte Android-Release-Builds und einen externen Release-UI-Smoke sowie EXIF-/GPX-/XMP-Fuzz-Prüfungen und ein eigenes Python-Faces-Testziel ergänzt. Regressionen sichern Sichtbarkeit, unterbrochene Upgrades, gezielte Revisionen, GPX-Inventarnutzung und kontogebundene Sitzungsressourcen ab. Leere oder abgestürzte Geräte-Testläufe werden anhand ihrer XML-Ergebnisse abgelehnt.
- Versionsentscheidung: zusätzliche PATCH-Korrekturen und Refactoring innerhalb der noch unveröffentlichten Versionen BearStack 0.50.0 und Android 0.10.0; kein zusätzlicher Versionssprung. HTTP-Verträge und das Android-Room-Schema bleiben kompatibel.

- Ordnersuche ohne die bisherige Begrenzung auf 50 Treffer: vollständige Gesamtzahlen, korrekte Sortierung und automatisch nachladbare Android-Pakete, auch für kurze Begriffe und ODER-Suchen. Sichtbarkeit und die Begrenzung auf 24 Ordner mit je zwei Vorschauen pro API-Abruf bleiben erhalten.
- Browserkarte und Android verwenden denselben JSON-Fotorouten-Cache samt Revisionen, Such- und Sichtbarkeitsregeln. Browserrouten berücksichtigen die gesamte indexierte Auswahl unabhängig von der Medienseite; nur die HTML-Darstellung wird bei mehr als 8.192 gruppierten Orten mit dem gemeinsamen Kartenverfahren vereinfacht und gekennzeichnet. Der Cache bleibt vollständig.
- PATCH-Korrekturen und Performance-Arbeit innerhalb des unveröffentlichten MINOR-Releases 0.50.0; VERSION bleibt 0.50.0, die unveränderte Android-App bei 0.10.0. Regressionen für vollständige Ordnersuche, Browser-/API-Cachewiederverwendung, Rechte, Invalidierung und lange Routen ergänzt.

- Vollständige Fotorouten werden serverseitig als JSON im bestehenden Cache-Verzeichnis gespeichert. Eigene Format-/Algorithmusversionen, transaktionale Indexrevision, getrennte Ordner-/Radius-/Typ-/Sichtbarkeitsschlüssel, zusammengefasste Berechnungen und atomarer Dateiersatz. Ausschnitt und Punktlimit greifen erst beim Abruf; Suchabfragen bleiben ohne dauerhaften Cache. Maximal 256 Dateien/512 MiB; beschädigte Dateien werden erneuert und Schreibfehler erlauben die direkte Berechnung.
- Gemeinsame warme Hell-/Dunkelfarben, kompakte abgerundete Galerienavigation, sechs Fotospalten auf breiten Displays und einspaltige Ordner bei großer Schrift. Hochformat, Querformat, Informationsblatt, Ordnernavigation und runder Icon-Beschnitt sind mit echten Bilddateien auf dem Emulator geprüft.

- Native Kartenmarker, Fotoauswahl und Fotorouten verbergen neu private GPS-Ordner bereits vor dem nächsten Indexlauf. Gemeinsame Zugriffsprüfung in begrenzten Verzeichnispaketen; keine Originalbilder werden gelesen.
- Gemeinsame Fotorouten-Gruppierung mit konstantem Zwischenspeicher statt vollständiger Zwischenlisten je Zeitfenster. Die bestehenden Browserregeln bleiben erhalten; benachbarte Orte beiderseits der Datumsgrenze werden korrekt zusammengefasst, ungültige Koordinaten ausgeschlossen. Die native Kartenebene **Fotoroute** und `/api/photos/v1/map/route` nutzen dieselben Regeln, aktive Filter und chronologische Indexabfragen mit normalisierten Zeitzonen. Die App hält höchstens 4.096 Routenkoordinaten zusätzlich zu den GPX-Tracks. Große Volltextsuchen werden einmal ausgewertet und können ihre Sortierung in temporäre Dateien auslagern.
- Android-Foto-Infos mit vollständiger Aufnahmezeit und Zeitzone, gekennzeichneter Änderungszeit als Ersatz, Bewertung, Personen, Tags und Schlagwörtern sowie direktem Wiederholen bei Ladefehlern. Der Fotoframe berücksichtigt Videos/Audio und aktive Filter; einzelne Medien werden korrekt wiederholt. Info-/Einstellungsdialoge pausieren auch manuelle Wiedergabe; Einstellungen sind scrollbar und ihre Schalter vollständig beschriftet.
- Native GPX-Kartenebenen für Android: mehrere Tracks auswählen, automatisch zum Track springen und beim Zoomen detailliertere Linien laden. Gemeinsamer GPX-Parser/Cache, getrennte Abschnitte und korrekte Datumsgrenzen; maximal 8.192 dargestellte Punkte und zwei gleichzeitige Geometrie-Anfragen. Die neue lesende API bietet Dateimetadaten mit Cursor und begrenzte Geometrie pro Ausschnitt. Schema 26 ergänzt den GPX-Dateiindex automatisch beim normalen Scan, ohne bestehende Fotokarten zu entwerten. Bestandteil des unveröffentlichten Minor-Features 0.50.0 / App 0.10.0; keine weitere Versionsanhebung.

- Android-Galerie als fortlaufendes Raster ohne Seitenwechsel oder Ladebuttons: Fotos, Ordner und Texte werden vor dem sichtbaren Ende automatisch nachgeladen. Auch die Karten-Fotoauswahl verwendet dasselbe Raster mit Vollbild und Nachladen in beide Richtungen. Speichergrenzen und wiederholbare Ladefehler bleiben erhalten. Bedienkorrektur innerhalb des unveröffentlichten Features 0.50.0 / App 0.10.0; kein zusätzlicher Versionssprung und keine Änderung am API-Vertrag.

- Galerie-Metadaten bleiben auf drei Seiten je Bereich begrenzt. Vorwärts- und Zurückscrollen laden Seiten bei Bedarf erneut; sichtbare Elemente, absolute Fotopositionen und die Rückkehr aus Vollbild/Personenverwaltung bleiben erhalten. Seitenspezifische Fehler sind ohne Neustart der Galerie wiederholbar. Performance-/Bedienkorrektur im geplanten Release 0.50.0 / App 0.10.0; API-Vertrag und Versionsstände bleiben gleich.

- Gesamte Android-Oberfläche auf Deutsch/Englisch: Personenverwaltung, Benennen, ähnliche Gruppen, Originalvorschau, Statistik, Hilfen und TalkBack-Aktionen. Spracheinstellungen ab Android 13 deklariert; Einzahl/Mehrzahl und Zertifikatsdatum lokalisiert. Sprachwechsel übersetzen auch vorhandene Fehler, ohne die Warteschlange zurückzusetzen.
- Gemeinsame strukturierte Fehlermeldungen für Personen- und Galerieabläufe vermeiden die Anzeige ungefilterter Ausnahmen. Ladefehler von Textbeiträgen sind direkt im Dialog wiederholbar; leere Beiträge bleiben nicht im Ladezustand hängen.

- Native Karten in Foto-Informationen und pro Ordner/Suche mit Zoom, Verschieben, GPS-Markern und sichtbarer OpenStreetMap-Quellenangabe. Die neue lesende Route `/api/photos/v1/map` bündelt vollständige indexierte GPS-Bestände in maximal 289 Marker je Ausschnitt, einschließlich Datumsgrenze; Kartenbilder verwenden einen getrennten HTTP-Cache ohne BearStack-Zugangsdaten.

- BearStack Personen wird **BearStack Fotos 0.10.0** (versionCode 22). Native Galerie als Einstieg mit Datumsgruppen, Ordnern mit zwei Vorschauen, globaler Suche, Vollbild/Zoom und Foto-Informationen. Personenfunktionen bleiben im Menü erhalten; Lesekonten können die Galerie ohne Bearbeitungsrechte öffnen.
- Lesende API unter `/api/photos/v1/`, gemeinsame Dienste und HTTPS-Verbindung mit der vorhandenen Personenverwaltung. Begrenzte Medien-, Ordner- und Textseiten; Ordnervorschauen erst nach Seitenauswahl, Blog-Inhalte erst beim Öffnen. `.txt` wird neben `.md` als Textbeitrag indexiert.
- Original-Download über Androids Speicherdialog mit begrenzten Übertragungspuffern und Abbruchbereinigung. Einstellbare Diashow und Fotoframe mit Anzeigedauer, Wiederholung, Beschriftung und Bildschirmfüllung; Wiedergabe pausiert in Info-/Einstellungsdialogen und im Hintergrund. Nativer Media3-Player für Video und Audio.
- Galerie- und Personenansichten auf Deutsch/Englisch mit System-Hell-/Dunkelmodus. Adaptives Icon für runden Beschnitt angepasst. Umsetzung und Prüfnachweise stehen in `apps/android/PHOTOS_PLAN.md`.
- MINOR für abwärtskompatible App- und API-Funktionen, ohne manuelle Datenmigration. Rechte-, Pfad-, Seitengrenzen-, Such- und Anfrageabbruchtests ergänzt; OpenAPI und Android-Dokumentation erweitert.

### BearStack 0.49.0

- Android-App **0.9.0** (versionCode 21): **Menü → Ähnliche Gruppen** zeigt jeweils ein Gruppenpaar ohne Vorschlagsliste zum Scrollen. **Zusammenführen** und **Getrennt lassen** bleiben unten sichtbar; nach bestätigter Entscheidung lädt das nächste Paar. Beide Portraits bieten die bekannte Originalfoto-Vorschau per Halten und Wischen mit Gesichtsmarkierung und TalkBack-Aktionen.
- Die Labeling-API liefert genau einen gespeicherten Vorschlag mit den tatsächlichen Vergleichsgesichtern, Pfaden, Original-Cache-Schlüsseln und Gruppenrevisionen. Beide Entscheidungen werden atomar mit einer Aktionsquittung gespeichert. Verlorene Antworten, doppelte Klicks, Konflikte und Fehler beim Nachladen sind abgesichert. Ablehnen verändert keine Gesichter und verhindert künftige automatische Zuordnungen zwischen den Gruppen.
- Die Benennen-Warteschlange einschließlich Bildseite bleibt bei unveränderten Gruppen erhalten. Nur die beiden aktuellen Portraits werden geladen; WLAN-Vorladen nutzt den vorhandenen begrenzten Originalbildcache. Ältere Server zeigen den erforderlichen Versionsstand an.
- MINOR für die zusätzliche App- und API-Funktion, ohne Datenmigration. README, Android-Anleitung, Website und OpenAPI aktualisiert; Regressionen für Entscheidungen, Quittungen, Rechte, Sichtbarkeit, große Gruppen, Vergrößerung und Bedienung ergänzt.

### BearStack 0.48.1

- Im Benenn-Modal nutzt die Vorschlagsliste die freie Höhe neben der Fotovorschau. Längere Listen scrollen innerhalb dieses Bereichs; die reservierte Fläche hält die Dialogaktionen beim Ein- und Ausblenden stabil.

- Android-App **0.8.3** (versionCode 20): In der unteren Benennen-Aktionsleiste entfällt der sichtbare Schaltflächenhintergrund am Stift. **…**, **?** und **Stift** erscheinen als gleich große, vertikal mittig ausgerichtete Symbole. Die unsichtbaren 48-dp-Touchflächen und bestehenden Aktionen bleiben erhalten. In „Personen benennen“ entfallen „Unbenannte Person“ und die Bildanzahlzeile über dem Gesichtsraster.
- PATCH für die kleine UI-Korrektur ohne API- oder Datenänderung. README, Android-Anleitung, Website und Versionsmetadaten aktualisiert.

### BearStack 0.48.0

- Android-App **0.8.2** (versionCode 19): Favoritenstern im Personenbereich so klein wie „×“, ohne sichtbaren Schaltflächenhintergrund und mit weiterhin 48 dp Touchfläche. UI-PATCH im vorgesehenen Release 0.48.0; API unverändert.

- Android-App **0.8.1** (versionCode 18): feste, schlanke Aktionsleiste im Benennen-Modus mit Gruppenaktionen links, aufklappbarer Hilfe mittig und Stift rechts. Gruppenaktionen wandern aus dem oberen Menü, der permanente Hilfetext entfällt. Scrollbarer Inhalt berücksichtigt Leiste und Systemnavigation. UI-PATCH innerhalb des vorgesehenen Releases 0.48.0, ohne API-Änderung.

- Exakte personenbezogene Kandidatensuche ersetzt den HNSW-Index. Alle Referenzen einschließlich Favoriten werden berücksichtigt; ausgeschlossene oder inzwischen private Kandidaten werden nachgeladen. Grenzwerte für neue Fotos bleiben erhalten. Inkrementelle Cache-Aktualisierung erhält verschobene Referenzen unabhängig von der Reihenfolge betroffener Gruppen.
- Separat pausierbarer, nach Neustart fortsetzbarer Hintergrundabgleich gespeicherter Vektoren mit kurzen Transaktionen, Fortschrittsanzeige und automatischer Vormerkung nach Referenzverbesserungen. Nur unbestätigte Gesichter werden eindeutig bestätigten benannten Personen zugeordnet; manuelle Entscheidungen und Fotos mit bereits vorhandener Zielperson bleiben geschützt.
- Neue Seite für gespeicherte Zusammenführungsvorschläge ähnlicher Teilgruppen, einschließlich unbenannter Gruppen. Annehmen prüft beide Gruppenrevisionen atomar; Ablehnen verhindert weitere automatische Zuordnungen zwischen den Gruppen. Neue HTTP-Endpunkte benötigen Fotobearbeitungsrechte.
- Kleine Gesichter werden mit begrenzten Originalausschnitten nachanalysiert. Auflösung und Schärfe steuern die automatische Referenzauswahl; explizite Favoriten bleiben möglich. Bestehende Vektoren ohne Qualitätsmetadaten und Protokoll 1 bleiben kompatibel. Gesichtsdienst für Qualitätsmetadaten mit aktualisieren.
- MINOR für kompatible Funktionen und automatische Migration auf Foto-Schema 25. README, Website und OpenAPI aktualisiert; Regressionen für Suchqualität, Wiederaufnahme, Konflikte, Datenschutz, Qualitätsfilter, Originalausschnitte und Bedienung ergänzt.

### BearStack 0.47.1

- „Ignorieren“ im Benenn-Modal links neben „Abbrechen“ verwendet die bestehende Einzelgesichtsaktion. Bei mehreren unbenannten Gruppen werden ausschließlich deren angezeigte Vorschaubilder gemeinsam ignoriert; benannte Gruppen deaktivieren die Aktion. Namenseingaben werden nicht mitgespeichert.
- Fehler bleiben im Dialog sichtbar, erfolgreiche Aktionen schließen ihn nach Aktualisierung der Ansicht. Gespeicherte Aktionen mit fehlgeschlagenem Nachladen können nicht direkt wiederholt werden. Gruppenbild-Konflikte aktualisieren das Foto und verlangen erneute Prüfung der Auswahl. Mobile Anordnung und Fokus nach dem Ausblenden von Vorschauen berücksichtigt.
- PATCH für den zusätzlichen Zugang zur vorhandenen Ignorierfunktion, ohne API- oder Datenmigration. Browserprüfungen für Einzel-/Mehrfachauswahl, Fehler, Konflikte und Buttonanordnung; README und Website aktualisiert.

### BearStack 0.47.0

- Die Personenübersicht verwendet eine exklusive Filterauswahl mit „Alle“, bekannten, unbekannten und ignorierten Personen/Gesichtern. Bei unbekannten Personen entfällt die wiederholte sichtbare Beschriftung „Unbenannt“. `filter` ergänzt die API kompatibel und hat Vorrang vor bisherigen Filterparametern.
- Das …-Menü speichert Ordneranzeige, Fotoanzahl und Thumbnailgrößen S/M/L pro Benutzer im Browser. Der Ordner des Vorschaubilds steht unter der Fotoanzahl; die Standardeinstellung bleibt S mit Fotoanzahl und ohne Ordner. Änderungen gelten sofort und nach AJAX-Aktionen, ohne Bilder erneut abzurufen.
- Das gemeinsame Benenn-Modal zeigt links eine 300-%-Bounding-Box-Fotovorschau, auf Mobilgeräten darüber. Personengruppen liefern repräsentativen Ordner und Gesichtsgeometrie direkt mit der Seitenabfrage. Bildfehler lassen Benennen und Zuordnen weiter zu.
- MINOR für neue Anzeigeoptionen und additive API-Felder ohne Datenmigration. README, Website, OpenAPI und Tests für Filter, Sichtbarkeit, Vorschauen und gespeicherte Darstellung aktualisiert.

### BearStack 0.46.0

- Android-App **0.8.0** (versionCode 17): Seitennavigation im Benennen-Modus nur bei mehr als vier Fotos; Originalvorschau nach 250 ms Halten. Aktive WLAN-Verbindungen erlauben serielles Vorladen der angezeigten Originale im bestehenden Drei-Minuten-/16-MiB-Cache. Ansichts-, Netzwerk- und Hintergrundwechsel brechen Vorladen ab. MINOR für die kompatible WLAN-Funktion im vorgesehenen Release; API unverändert. Tests für Navigation, Gesten, WLAN-Abbruch, Serialisierung und HTTPS-Cache-Wiederverwendung.

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
