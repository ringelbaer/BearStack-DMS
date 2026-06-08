1. Analysiere das Repo und ändere keine Dateien. Erstelle eine priorisierte Liste von Aufräum- und Optimierungspunkten: tote Dateien, doppelte Logik, unklare Modulgrenzen, riskante Stellen, fehlende Tests, unnötige Dependencies. Nenne konkrete Dateien und begründe jede Empfehlung kurz.

2. Räume das Projekt konservativ auf, ohne Verhalten zu ändern. Entferne offensichtlichen Dead Code, vereinheitliche Namen/Strukturen nach bestehenden Patterns und lass größere Refactors weg. Führe danach passende Tests/Builds aus und fasse die Änderungen zusammen.

3. Prüfe, ob Tests, Build und lokale Startbefehle sauber laufen. Wenn etwas fehlschlägt, finde die Ursache und behebe sie möglichst klein. Keine unrelated Refactors.

4. Untersuche die Architektur des Projekts. Wo sind Verantwortlichkeiten vermischt, wo gibt es unnötige Kopplung, und welche 3-5 Refactors hätten den größten Nutzen bei geringem Risiko? Bitte erst nur analysieren, nicht ändern.

5. Suche nach offensichtlichen Performance-Problemen im Backend und Frontend: unnötige IO, ineffiziente Queries, übermäßige Re-Renders, große Assets, langsame Startpfade. Schlage sichere, gut belegbare Optimierungen vor.

6. Prüfe Konfiguration, Auth, Dateizugriffe, Uploads, Secrets, Logging und Fehlerbehandlung auf Sicherheitsrisiken. Ändere nur konkrete Probleme mit klarer Begründung und ergänze Tests, wenn sinnvoll.

7. Prüfe Dependencies auf ungenutzte Pakete, unnötige Komplexität und Update-Risiken. Entferne nur eindeutig ungenutzte Dependencies und verifiziere Build/Tests danach.

8. Bring die Projekt-Dokumentation auf Stand: Setup, lokale Entwicklung, Tests, Build, wichtige Env Vars, typische Troubleshooting-Fälle. Lies zuerst das Repo und dokumentiere nur, was tatsächlich stimmt.

9. Analysiere zuerst den Bereich <Bereich/Feature>. Erstelle einen kurzen Refactor-Plan mit Risiken. Danach setze den Plan um, wenn er klein genug ist; andernfalls stoppe nach dem Plan und warte auf Freigabe.

10. Mach einen gründlichen, aber konservativen Cleanup-Pass für dieses Repo. Starte mit einer kurzen Bestandsaufnahme, ändere nur Dinge mit geringem Risiko, führe relevante Tests/Builds aus und gib am Ende eine knappe Liste der Änderungen plus verbleibende Empfehlungen.

11. Auditiere die Photo Funktion. Prüfe Frontend- und Worker-Prozesse, Thumbnailerstellung und Indexierung, Umsetzung der Berechtigungen und Beschränkungen, Tagging. Hilf dir mit detailiierten Tests für alle Komponenten.

12. Auditiere die Documents Funktion. Prüfe Frontend- und Worker-Prozesse, Dateiverarbeitung, Batch-Editing, Ordner-Funktion, WebDAV. Umsetzung der Berechtigungen und Beschränkungen, Tagging. Hilf dir mit detailiierten Tests für alle Komponenten.

--- PERFORMANCE ---

1. Performance-Audit

Analysiere die Codebase auf Performance-Risiken. Finde die wichtigsten Hotspots in Backend, Storage und UI, belege sie mit konkreten Dateien/Zeilen und schlage priorisierte Verbesserungen vor.

2. Langsamen Workflow untersuchen

Der Workflow [konkreter Ablauf] ist langsam. Reproduziere ihn lokal, miss die Laufzeit, finde den Engpass und implementiere eine minimal-invasive Optimierung mit Test oder Benchmark.

3. Datenbank/Storage optimieren

Prüfe alle Datenzugriffe im Bereich [Feature] auf unnötige Reads/Writes, N+1-Zugriffe, fehlende Indexierung oder Locking-Probleme. Optimiere nur belegte Engpässe.

4. Frontend-Performance verbessern

Untersuche die UI-Performance von [Seite/Komponente]: Ladezeit, Render-Kosten, unnötige Re-Renders, Asset-Größe und Interaktionslatenz. Implementiere konkrete Verbesserungen und verifiziere sie.

5. Regressionen verhindern

Erstelle Performance-Benchmarks oder Smoke-Tests für [kritischer Pfad], damit spätere Änderungen messbar keine Verschlechterung verursachen. Dokumentiere die Baseline und sinnvolle Grenzwerte.


---


1. Prüfe das Photo-Modul gezielt auf Berechtigungsfehler. Erstelle eine Matrix für Rollen, Routen und Aktionen: Galerie, Media, Thumbnail, Tags, Settings, Worker. Ergänze Regressionstests für jede erlaubte/verbotene Kombination. Ändere nur echte Bugs.

2. Prüfe die Thumbnail-Erstellung Ende-zu-Ende: Galerie-Lazy-Loading, Status-Endpoint, Worker, Batch-Größen, Preview-Größe, Video-Thumbnails, Fehlerfälle bei fehlendem ffmpeg/vipsthumbnail. Ergänze fokussierte Tests und behebe kleine Bugs.

3. Auditiere die Photo-Indexierung auf Konsistenz: Rebuild, inkrementelle Änderungen, gelöschte/umbenannte Dateien, AdminOnly-Marker, Tags, Blog-Metadaten, Folder-Counts. Schreibe Tests für Drift zwischen Filesystem-Fallback und Indexpfad.

4. Ergänze minimale Playwright-Smoke-Tests für die Photo-Galerie: Galerie lädt, Thumbnail-Queue startet, Lightbox öffnet, Auswahl/Bulk-Tagging funktioniert, Nicht-Admin sieht keine AdminOnly-Inhalte. Halte Setup klein und passe Makefile/README nur nötig an.

5. Prüfe Photo-Worker auf Race Conditions, Job-Locking, Context-Cancel, lange Laufzeiten und Fehlerbehandlung. Ergänze Tests für parallelen Index- und Thumbnail-Worker sowie manuelle Worker-Starts.

6. Prüfe Photo-Tagging komplett: Normalisierung, Rename/Delete, Media/Folder/Blog-Tags, Suche, Bulk-Aktionen, AdminOnly-Sichtbarkeit, leere Tags. Ergänze Tests für Grenzfälle und behebe nur klare Fehler.

7. Performance gezielt messen
Suche im Photo-Modul nach belegbaren Performance-Problemen. Miss oder begründe Query-Anzahl, IO-Pfade, Thumbnail-Worker-Batches und Frontend-Reflows. Implementiere nur risikoarme Optimierungen mit Tests.

8. Prüfe das Photo-Modul mit diesen lokalen Verzeichnissen: BEARSTACK_PHOTOS_DIR=... und BEARSTACK_PHOTOS_DATA_DIR=.... Starte lokal, teste Indexierung, Galerie, Thumbnails und Tags. Ändere keine Dateien außerhalb des Repos und fasse Auffälligkeiten zusammen.

