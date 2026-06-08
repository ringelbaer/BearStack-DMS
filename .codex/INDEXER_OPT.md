1. Profile den Foto-Indexer und finde die Top-5 Bottlenecks bei 1M Bildern / 10k Ordnern. Ändere noch nichts, liefere Messwerte und konkrete Optimierungsvorschläge.

2. Optimiere den Indexer für Raspberry Pi 4/5: niedrige CPU- und I/O-Last ist wichtiger als maximale Geschwindigkeit. Baue adaptive Pausen/Batchgrößen ein und teste, dass der Worker abbrechbar bleibt.

3. Prüfe die SQLite-Nutzung des Fotoindexers: Transaktionen, Indizes, FTS, PRAGMAs, Write-Amplification. Optimiere nur Stellen mit messbarem Effekt.

4. Optimiere den Fall „fast alles unverändert“: Ein erneuter Crawl über 1M Bilder soll möglichst wenig FS/DB-Arbeit machen. Benchmarke Cold Run vs Warm Run.

5. Schreibe Tests für Indexer-Randfälle: gelöschte Ordner, umbenannte Dateien, neue Datei in unverändertem Parent, Tags, Blogs, Symlinks, Abbruch per Context.

6. Erweitere den Benchmark um mehrere Szenarien: leerer Index, warmer Index, 1 geänderter Ordner, 100 geänderte Ordner, gelöschter Teilbaum, viele Tags, viele Blogs.
 
7. Vergleiche BearStack Fotoindexierung erneut mit PiGallery2 und übernimm nur die Muster, die zu Go/SQLite/BearStack passen. Priorisiere ressourcenschonende Strategien.

8. Prüfe den Zusammenspiel von Index-Worker und Thumbnail-Worker. Verhindere, dass beide gleichzeitig den Pi belasten, und baue eine gemeinsame Lastbegrenzung.

9. Reduziere Allocations und Peak-RAM im Indexer. Benchmarke vorher/nachher mit -benchmem und erkläre die größten Verbesserungen.

10. Baue leichte Indexer-Telemetrie ein: gescannte Ordner, übersprungene Ordner, Dateien/s, DB-Writes, Dauer, letzte Fehler. Zeige das in den Settings an.