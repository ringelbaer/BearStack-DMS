---
title: Fotos
description: Foto-Galerie, Index, Suche, Worker und Metadaten.
icon: lucide/images
---

# Fotos

Ab BearStack **0.50.0** steht die lesende Galerie auch nativ in **BearStack Fotos für
Android** zur Verfügung: Datumsgruppen, Ordner mit konfigurierbaren Vorschauen, Suche,
Vollbild/Zoom, Foto-Informationen sowie Markdown- und Textbeiträge. Die App nutzt
denselben Fotoindex und dieselben Zugriffsregeln. Die gesamte App-Oberfläche, Hilfen und Fehlermeldungen sind deutsch/englisch. Details zum laufenden Ausbau
stehen in der [Android-Anleitung](android.md).

Die nativen Foto-Informationen zeigen Aufnahmezeit samt Zeitzone, Bewertung,
Personen und Schlagwörter. Der Fotoframe spielt auch Videos und Audio bis zum Ende;
eine aktivierte Wiederholung funktioniert auch bei nur einer Mediendatei.

Die Android-App **0.15.0** bietet ein eigenes **Sortieren**-Menü. Der Fotostream nutzt Datumssortierung; normale Ordner bieten zusätzlich Namen. In Personenlisten ist ab BearStack **0.58.0** auch **Anzahl Bilder** auf-/absteigend verfügbar. Der Server zählt unterschiedliche sichtbare Fotos und sortiert vor der Seitenaufteilung. Bei **Personen im Ordner** bleibt die Zählung auf dessen Unterbaum begrenzt.

Die native Galerie hält höchstens drei Metadatenseiten je Bereich. Beim Zurückscrollen lädt sie ältere Seiten erneut; Vollbild und Diashow behalten die Fotoreihenfolge, und Ladefehler sind direkt im betroffenen Bereich wiederholbar.

Das Fotomodul ist optional und nutzt einen directory-first Ansatz: BearStack importiert Fotos nicht in die Dokumentenablage, sondern rendert ein vorhandenes, read-only Fotoverzeichnis als Galerie. Die Mediendateien bleiben unverändert; BearStack legt Index, Tags, Vorschaubilder und Einstellungen getrennt davon ab.

Im Browser bietet auch **Zusammenführen und benennen/zuordnen** die Lupe für ähnliche benannte Personen. Sie prüft das erste Vergleichsgesicht; beim Benennen einer einzelnen Gruppe wird deren Vergleichsgesicht verwendet. Ein Treffer übernimmt die Zielperson mit Versionsprüfung für die atomare Zuordnung.

## Ignorierte Gesichter eines Fotoordners zurücksetzen

Ab **0.63.0** steht im **…**-Menü eines normalen Fotoordners die Funktion
**Alle ignorierten Gesichter zurücksetzen** bereit. Sie benötigt **Fotos bearbeiten**
(`photos.edit`) und ist erst ab Ordnerebene 2 verfügbar: nicht unter `/photos`,
nicht im Jahresordner `2011`, aber beispielsweise in
`2011/20111015-Silberhochzeit-Onkel-Hermann` und tieferen Ordnern.

Die Bestätigung nennt den aktuellen Ordner. Die Aktion schließt sämtliche Unterordner
ein; Suchbegriffe, Medientyp- und Anzeigefilter begrenzen sie nicht. Ignorierte
Gesichter ohne Personennamen werden wieder als **Unbenannt** sichtbar. Benannte
Personen bleiben vollständig erhalten, einschließlich ihrer eventuell ignorierten
Gesichter, Zuordnungen, Tags, Favoriten und Stammdaten. Andere aktive Gesichter und
Nachbarordner werden nicht verändert. Virtuelle Personenordner und Admin-only-Inhalte
sind von der Rücksetzung ausgeschlossen; die vorhandenen Sichtbarkeitsprüfungen gelten weiter.
Nach Abschluss zeigt die Galerie die Anzahl zurückgesetzter Gesichter an. Abbrechen
führt keine Rücksetzung aus.

Der Server prüft Rechte, Pfad und Mindesttiefe unabhängig vom Menü. Er verwendet den
vorhandenen Pfadindex mit exakten Unterordnergrenzen, auch für Ordnernamen mit `%` oder
`_`. Er liest keine Bilder ein und startet keine Gesichtserkennung. Die Rücksetzung
und die Aktualisierung der Referenzen betroffener Gruppen erfolgen in einer Transaktion;
bei Fehlern bleibt keine teilweise Rücksetzung zurück. Im Arbeitsspeicher werden nur
die betroffenen Gruppen-IDs gehalten, keine vollständigen Gesichts- oder Bildlisten.
Auch mehr als 500 Gesichter sind möglich. Eine Wiederholung ohne neue Ignorierungen
ändert nichts; nach einer unklaren Antwort zuerst den Bestand prüfen.

Der Formularendpunkt ist `POST /photos/faces/reset-ignored-directory` mit `path`.
Mit `Accept: application/json` antwortet er mit `ok` und `restored`, andernfalls
führt er zurück zum Ordner mit einer Ergebnismeldung. Einzelheiten stehen in OpenAPI.

## Personen ansehen und taggen

Ab BearStack **0.53.0** öffnet die virtuelle Kachel **Personen** unter **Fotos** eine
Ansicht zum Durchstöbern. **Alle** (`/photos?path=.people%2Fall`) enthält ab
**0.57.1** ausschließlich aktive benannte Personengruppen; Anzahl, Seitenaufteilung
und Vorschauen berücksichtigen denselben Filter. Daneben stehen ausschließlich Foto-Tags, denen sichtbare Personen
zugeordnet sind. Es gibt genau eine Tag-Ebene; auch Tags mit einem Schrägstrich
bleiben eine einzelne Kachel. Im Tag-Ordner stehen alle zugeordneten Personen.

Eine Person öffnet ihre vollständigen Fotos in der normalen Galerie, mit
Datumsgruppen, Sortierung und seitenweisem Laden. Mehrere Erkennungen derselben
Person in einem Foto erzeugen nur einen Bildeintrag. Die Suche innerhalb dieser
Galerie bleibt auf die Person begrenzt. Mit der Berechtigung **Fotos bearbeiten** führt **Person bearbeiten** aus der Personengalerie direkt zur Bearbeitungsansicht dieser Person.

Ab **0.57.3** berücksichtigt die Hauptkachel **Personen** bei Anzahl und
Vorschaubildern ausschließlich benannte aktive Personen. Ohne benannte Personen
zeigt sie null Personen und keine Gesichtsvorschauen.

Die Personen-Kachel sowie **Alle** und die Tag-Kacheln zeigen bis zur doppelten
konfigurierten Anzahl der Ordner-Vorschaubilder (2–8). Die Auswahl bevorzugt
Personen mit den meisten unterschiedlichen sichtbaren Fotos. Pro Person erscheint
das gewohnte Porträt: ein aktiver Stern-Favorit, bei mehreren der mit der kleinsten
Gesichts-ID, sonst das bisherige Übersichtsbild. Die bestehenden Gesichtsthumbnails
und deren Cache werden wiederverwendet. Listen laden keine Originalbilder.
Ignorierte und geschützte Gesichter bleiben ausgeschlossen, auch bei Admin-Konten.

**Personen verwalten** führt weiterhin nach `/photos/people`. Bei der Namenssuche
leert der **×-Button** mit einem Klick den Suchtext und springt zur ersten Seite.
Er hat dieselbe Höhe wie das Suchfeld;
der gewählte Personenfilter und die Sortierung bleiben erhalten. In der geöffneten
Personengruppe vergibt **Personen-Tags** bestehende oder neue Foto-Tags. **Keine
Tags → Übernehmen** entfernt die Auswahl. Dafür ist `photos.edit` erforderlich,
zum Ansehen genügt `photos.read`. Die Tags gehören der Gruppe und werden nicht auf
die einzelnen Fotos übertragen. Beim Zusammenführen werden die Tags vereinigt;
globales Umbenennen und Löschen eines Foto-Tags aktualisiert auch Personengruppen.
Foto-Schema **32** ergänzt automatisch einen indizierten Zuordnungsspeicher.

### Markierte Personen gemeinsam taggen

Ab **0.59.0** erscheint nach dem Markieren von Personen unter `/photos/people`
**Personen-Tags ergänzen** in der Auswahlleiste. Im bekannten Tag-Dialog vorhandene
Tags wählen oder neue anlegen und **Übernehmen** drücken. Die Tags werden allen
markierten Personen der aktuellen Seite hinzugefügt; bestehende Personen-Tags
bleiben erhalten und die Tags ihrer Fotos unverändert. Abbrechen speichert nichts.
Auch unbenannte aktive Personen können getaggt werden.

Die gesamte Auswahl wird atomar gespeichert. Ist eine Person zwischenzeitlich
entfernt, ignoriert oder geschützt, wird nichts hinzugefügt. Bei einem Fehler bleibt
der Dialog für einen erneuten Versuch offen; wiederholtes Ergänzen erzeugt keine
Duplikate. Erfordert `photos.edit`; höchstens 500 Personen pro Anfrage und 100 Tags
pro Person. Keine neue Datenbankmigration.

### Personenstammdaten

Ab **0.60.0** öffnet **⋯ → Stammdaten** in der Detailansicht einer benannten Person
einen Dialog mit Geburts- und Sterbedatum, mehreren Geschwistern und mehreren Ehen.
Zu jeder Ehe gehören eine benannte Person als Ehepartner sowie Hochzeits- und
optional Scheidungsdatum. Auch mehrere Ehen mit derselben Person sind möglich,
sofern die Datumsangaben die Einträge unterscheiden. Geschwister und Ehen werden
bei beiden Personen angezeigt; Änderungen und Entfernen gelten für beide Seiten.
Ab **0.61.0** gehören **Mutter und Vater** zum selben Stammdatendialog und werden
mit den übrigen Angaben gemeinsam gespeichert. Geschwister und Ehepartner erscheinen
als kompakte Eingabezeilen mit **×** zum Entfernen, ohne wiederholte Überschriften.
Leere **Sterbe- und Scheidungsdaten** bleiben zunächst ausgeblendet. Ein kleines **+**
mit der Beschriftung „Sterbedatum ergänzen“ beziehungsweise „Scheidungsdatum ergänzen“
öffnet das jeweilige Feld. Gespeicherte Daten bleiben beim erneuten Öffnen sichtbar.
Geschwister werden ausdrücklich zugeordnet und nicht automatisch aus gemeinsamen
Eltern abgeleitet.

Personen über die vorhandene Namenssuche auswählen. Unbekannte Datumsfelder dürfen
leer bleiben. Vollständige Kalenderdaten werden ohne Uhrzeit gespeichert; Sterbedatum
vor Geburtsdatum und Scheidung vor Hochzeit werden abgewiesen. **Abbrechen** verwirft
alle Änderungen. Ein veralteter Dialog überschreibt keine zwischenzeitliche Änderung,
auch wenn die Beziehung auf der anderen Personenseite bearbeitet wurde.
**Stammdaten neu laden** lädt den aktuellen Stand und verwirft den offenen Entwurf.

Lesen benötigt `photos.read`, Bearbeiten `photos.edit`. Nur benannte aktive sichtbare
Personen sind auswählbar; verborgene Angehörige und deren Ehedaten erscheinen nicht.
Bearbeiten sichtbarer Angaben erhält vorhandene verborgene Beziehungen. Pro Person
sind bis zu 100 Geschwister und 100 Ehen möglich. Die Daten gehören zur Personengruppe,
nicht zu einzelnen Bildern; beim Löschen der Gruppe werden ihre Beziehungen entfernt.

Beim Zusammenführen werden passende Stammdaten und Beziehungen übernommen;
identische Beziehungen werden zusammengefasst. Widersprüchliche Geburts-/Sterbedaten
oder entstehende Selbstbeziehungen verhindern die Zusammenführung atomar.
Automatische Abgleiche überspringen solche Konflikte. Foto-Schema **35** ergänzt
indizierte Tabellen und Aufräumregeln ohne Neuindexierung oder Bilderkennung.
Die Daten laden erst beim Öffnen des Dialogs. Die API dokumentiert
`GET/PUT /photos/people/{id}/details` einschließlich Versionsprüfung und Schreibgrenzen.

### Mutter und Vater zuordnen

Unter **⋯ → Stammdaten → Eltern** lassen sich Mutter und Vater zuordnen.
Tippe einen Namen und wähle eine vorhandene benannte Gruppe aus den Vorschlägen;
danach **Stammdaten speichern**. Ein leeres Feld oder **×** entfernt die sichtbare
Zuordnung. In der Leseansicht öffnen die verknüpften Namen die zugehörige Person.
Die Bearbeitung ist zunächst im Web verfügbar und benötigt `photos.edit`.
Leseberechtigte sehen die zugeordneten, sichtbaren benannten Personen.

Stabile IDs erhalten Beziehungen beim Umbenennen. Selbstzuordnungen, dieselbe
Person in beiden Elternrollen und Kreise in der Abstammung sind ausgeschlossen.
Zusammenführen übernimmt Eltern und Verweise auf die Quellperson atomar; bei
widersprüchlichen Eltern muss zuerst die Zuordnung korrigiert werden. Geschützte,
inaktive oder unbenannte Eltern werden nicht angezeigt. Foto-Schema **33** wird
automatisch angelegt. Die Namenssuche lädt höchstens 60 Vorschläge pro Anfrage.

### Personen eines Fotoordners ansehen

Ab **0.55.0** steht in jedem normalen Fotoordner unter **Weitere Fotoaktionen** der
Eintrag **Personen im Ordner**, neben den bisherigen Fotoaktionen wie der Karte.
Die Übersicht berücksichtigt den aktuellen Ordner und alle Unterordner. Sie zeigt
ab **0.57.1** ausschließlich benannte aktive Gruppen, mit der Anzahl unterschiedlicher Fotos in
diesem Bereich. Auch das Gesichtsporträt stammt aus dem Bereich; dessen Stern-
Favoriten haben Vorrang. Der bestehende Gesichtscache wird wiederverwendet.

Eine Person öffnet die normale, datumsgruppierte und sortierbare Galerie ihrer
Fotos aus genau diesem Bereich. Suche und Blättern behalten die Begrenzung bei.
Der Fotopfad führt zur Personenübersicht oder zum ursprünglichen Fotoordner zurück.
Geschützte Unterordner und ignorierte Gesichter bleiben ausgeschlossen. Die
Übersicht benötigt nur Fotoleserechte und lädt Personen seitenweise. Es werden
bestehende Pfadindizes genutzt und keine Originalbilder für die Liste dekodiert.

## Aktivierung

Minimal in `.env`:

```env
BEARSTACK_PHOTOS_ENABLED=true
BEARSTACK_PHOTOS_DIR=/srv/photos
BEARSTACK_PHOTOS_DATA_DIR=/var/lib/bearstack/photos
```

`BEARSTACK_PHOTOS_DIR` ist das vorhandene read-only Fotoverzeichnis. `BEARSTACK_PHOTOS_DATA_DIR` ist der BearStack-eigene Fotobereich für erzeugte Dateien; standardmäßig liegen darunter `thumbnails/` und die separate Foto-Indexdatenbank `photos.db`. Bei Bedarf können `BEARSTACK_PHOTOS_CACHE_DIR` und `BEARSTACK_PHOTOS_DB_PATH` einzeln überschrieben werden.

Nach dem Neustart erscheint `Fotos` in der Hauptnavigation.

Die native Android-Karte unter **Weitere Optionen → Karte** berücksichtigt Ordner samt Unterordnern oder die aktuelle Suche. `/api/photos/v1/map` bündelt alle passenden indexierten GPS-Aufnahmen je Ausschnitt in maximal 289 Marker. Die App bietet außerdem eine Karte in den Foto-Informationen; Über **Ebenen → GPX-Tracks** lassen sich mehrere Tracks des Ordners samt Unterordnern einblenden, auch ohne GPS-Fotos. Die App lädt begrenzte Geometrie für den sichtbaren Ausschnitt, erhält getrennte Abschnitte und berücksichtigt die Datumsgrenze. **Ebenen → Fotoroute** ergänzt eine gestrichelte Verbindung der Fotoorte mit denselben Zeit- und Entfernungsregeln wie im Browser. Suche und Typfilter gelten weiterhin.

Die gemeinsame Gruppierung von Fotorouten verarbeitet die chronologisch geordneten Fotoorte mit konstantem Zwischenspeicher. Die bestehenden Zeitfenster und die konfigurierte Entfernung gelten auch für nahe Orte beiderseits der Datumsgrenze; ungültige Koordinaten werden ausgelassen. Der neue lesende Endpunkt `/api/photos/v1/map/route` liefert begrenzte Geometrie und die vollständigen Gesamtzahlen. Kartenmarker, Fotoauswahl und Route prüfen GPS-Ordner in Paketen von höchstens 256 Einträgen gegen aktuelle private Markierungen; ein Rescan ist dafür nicht erforderlich.

Der GPX-Dateiindex wird ab Schema 26 automatisch beim regulären Foto-Indexlauf ergänzt, ohne Tracks zu parsen oder den vorhandenen Fotoindex zurückzusetzen. Die API `/api/photos/v1/map/tracks` liefert maximal 32 Dateimetadaten pro Cursor-Abfrage; `/api/photos/v1/map/track` verwendet den gemeinsamen GPX-Parser und liefert pro Ausschnitt höchstens 8.192 Koordinaten. Neu privat markierte Ordner und symbolische Links bleiben geschützt. Die App hält höchstens 96 Trackeinträge, bis zu 256 ausgewählte Namen und insgesamt 8.192 angezeigte GPX-Koordinaten sowie optional 4.096 Fotorouten-Koordinaten; überholte Anfragen werden abgebrochen.


Die Browserkarte prüft aktuelle Schutzmarkierungen bereits vor dem Lesen ihrer Medienliste. So bleiben auch Namen, Pfade und Koordinaten einzelner Marker in der ersten Antwort nach einer Ordnersperre verborgen. Für GPX verwendet sie denselben Dateiindex wie die native API und lädt Kandidaten in Paketen von höchstens 32 Einträgen. Nur diese Tracks werden bei Bedarf geöffnet; die bisherigen Byte-, Punkt- und Antwortlimits gelten weiter. Solange der GPX-Backfill noch läuft, bleibt der bestehende Dateisystemlauf erhalten.

Große optionale Kartenindizes werden bei vorhandenen Fotobeständen nach dem Start im Hintergrund aufgebaut. Die 30-Sekunden-Frist für das Basisschema begrenzt diesen Aufbau nicht; SQLite lagert Sortierdaten in temporäre Dateien aus. Galerie-Lesezugriffe bleiben verfügbar, Kartenanfragen warten mit ihrem Anfragekontext. Beim Herunterfahren wird der Aufbau abgebrochen und beim nächsten Start sicher fortgesetzt. Ein Aufbaufehler wird bei Kartenanfragen als nicht verfügbar gemeldet und beim nächsten Start erneut versucht.

### Serverseitiger Fotorouten-Cache

Browserkarte und native Karten-API verwenden denselben serverseitigen Cache der
vollständigen gruppierten Fotoroute als JSON unter `<Cache-Verzeichnis>/photo-routes/v1/`. Erst beim
Abruf werden Kartenausschnitt und Punktlimit angewendet. Ordner, Radius, Medientyp
und Sichtbarkeit erhalten getrennte Cache-Schlüssel; beliebige Suchabfragen bleiben
zunächst ohne dauerhafte Cache-Datei. Eine transaktionale Indexrevision invalidiert
auch Änderungen an GPS, Aufnahmezeit und Sichtbarkeit in Unterordnern. Ab Foto-Schema 27
werden Revisionen nach Ordner, Medientyp und Sichtbarkeit getrennt geführt. Betroffene
Vorfahren sowie beide Seiten einer Verschiebung werden invalidiert; Änderungen in
anderen Teilbäumen, an anderen Medientypen oder ausschließlich privaten Fotos lassen
unbeteiligte Routen weiter im Cache. Löschrevisionen bleiben erhalten, auch wenn der
Ordner danach leer ist. Die bisherige globale Revisionskennung wird einmalig ersetzt. Gleiche
Berechnungen werden zusammengefasst und Cache-Dateien atomar ersetzt. Format und
Algorithmus haben eigene Versionen. Die [Android-Anleitung](android.md#karten)
beschreibt Speichergrenzen, Fehlerbehandlung und Zugriffsprüfung.

Die Browserroute umfasst die gesamte indexierte Auswahl, unabhängig von der
angezeigten Medienseite. Für eine begrenzte HTML-Ausgabe werden höchstens 8.192
gruppierte Orte dargestellt; längere Routen werden über ihre gesamte Länge
vereinfacht und mit angezeigter/vollständiger Punktzahl gekennzeichnet. Anfang,
Ende sowie die ursprünglichen Zeiten und Fotoanzahlen der verbleibenden Orte
bleiben erhalten. Die JSON-Datei enthält weiterhin alle gruppierten Orte.
Vor dem ersten vollständigen Indexlauf bleibt die bestehende direkte Berechnung
aus dem Dateisystem verfügbar.


## Galerie und Suche

Die Ordnersuche berücksichtigt alle passenden sichtbaren Ordner statt nur der
ersten 50. Gesamtzahl und Sortierung gelten für die vollständige Trefferliste;
die Android-App lädt sie in Paketen von höchstens 24 Ordnern mit bis zu vier
Vorschauen (virtuelle Personenordner bis zu acht) weiter. Auch kurze Suchbegriffe und ODER-Suchen behalten alle Treffer.

Die native Android-Galerie lässt sich endlos durchscrollen. Fotos, Ordner, Texte und die Fotoauswahl auf der Karte laden automatisch nach, ohne Seitenwechsel oder Ladebuttons. Die Scrollposition bleibt beim Nachladen und bei der Rückkehr aus dem Vollbild erhalten.

Auf kleinen Bildschirmen steht die Foto-Info unter dem Foto über die volle Breite. Das Panel ist separat scrollbar; sein Schließen-Knopf bleibt sichtbar.

Die Foto-Lightbox stoppt am ersten und letzten Medium. Die jeweiligen Navigationspfeile sind dort deaktiviert; auch die Diashow endet beim letzten Medium.

Unterstützt werden:

- Ordnernavigation, Bild- und Videowiedergabe
- Medienformate `jpg`, `jpeg`, `png`, `gif`, `webp`, `svg`, `mp4`, `webm`, `ogv` und `ogg`
- On-demand-Thumbnails für JPEG, PNG und GIF
- separate SQLite-Foto-DB für den Metadatenindex
- EXIF-Aufnahmedatum und -zeit, Kamera und GPS für JPEGs
- Adobe/MWG-XMP-Gesichtsregionen
- BearStack-Tags auf Ordnern und Medien
- Umlaut-tolerante Suche mit Feldfiltern wie `date:2024`, `directory:Urlaub`, `file_name:IMG`, `type:image`, `gps:true`, `tag:urlaub`, `person:"Marie Curie"` und `face:Marie`
- Kartenansicht mit konfigurierbarer Foto-Track-Auflösung
- Markdown-Blogdateien, GPX-Hinweise, Zufallslink und Fotoframe

## Foto-Tags

Tagänderungen aktualisieren Metadaten, Tagzuordnungen und Volltextsuche gemeinsam. Beim Hinzufügen oder Entfernen von Tags auf mehreren Fotos bleiben gleichzeitige Änderungen erhalten. Scheitert eine Bulk-Aktion, wird keine teilweise geänderte Auswahl gespeichert. Auch laufende Indexscans erhalten den aktuellen Tagstand.

Umbenennen und Entfernen eines Tags verwenden die vorhandenen Tagindizes, um nur die betroffenen Medien, Ordner und Blogs zu bearbeiten. Der Aufwand hängt damit von den betroffenen Einträgen statt von allen getaggten Dateien ab.

## Foto-Zufall und Metadaten

Die Info-Seitenleiste zeigt bei den Metadaten unter **Ordner** den normalisierten Namen des enthaltenden Fotoordners, etwa **Sommer Urlaub** für `2026_07_15_Sommer_Urlaub`. Für Medien direkt im Foto-Hauptverzeichnis steht dort **Fotos**. Der Wert wird mit den vorhandenen Metadaten als `folder_name` geladen.

Foto-Info-Batchanfragen bündeln Metadaten- und Gesichtsabfragen in Paketen und teilen sich die Rechteprüfung gemeinsamer Elternordner. Dateistand und XMP-Sidecars werden weiterhin geprüft; aktuelle `.adminonly`-Markierungen und geschützte Gesichtsnamen bleiben berücksichtigt.

Der Zufallsendpunkt `/photos/random` liefert standardmäßig das Original direkt aus. Mit `size=original` bleibt es beim Original; mit `size=ordner`, `size=galerie`, `size=gross`, `size=groß` oder `size=hd` wird stattdessen die jeweilige konfigurierte Thumbnailgröße ausgeliefert.

Zusätzlich liefert der Endpunkt Metadaten als Response-Header: `X-BearStack-Photo-Title`, `X-BearStack-Photo-Path`, `X-BearStack-Photo-Folder-Path`, `X-BearStack-Photo-Folder-URL`, `X-BearStack-Photo-Folder-Title` sowie `Link: <https://.../photos?...>; rel="up"` als standardisierter Parent-Link.

## Gesichter und XMP

Ab 0.52.0 werden nur reguläre XMP-Sidecars bis **4 MiB** gelesen. Symlinks, Spezialdateien, zu große oder während des Öffnens ausgetauschte Dateien werden übersprungen; auch beim Lesen gilt die feste Grenze. Das Foto bleibt ohne diese optionalen Metadaten verfügbar.

Die Labeling-API verwendet für unbenannte, benannte und zusammenzuführende Gruppen dieselbe Cursorpagination mit Sichtbarkeitsprüfungen in kleinen Blöcken. Neue Schutzmarkierungen und dadurch entfallende importierte Namen werden berücksichtigt, ohne den gesamten restlichen ID-Bereich vor jeder Seite zu prüfen. Foto-Schema **31** ergänzt den dazugehörigen partiellen Kandidatenindex automatisch; Gesichtsidentitäten bleiben erhalten.

Gesichtsdaten werden aus eingebettetem JPEG-XMP sowie XMP-Sidecars gelesen: `photo.jpg.xmp`, `photo.jpg.XMP`, `photo.xmp` und `photo.XMP`. BearStack speichert Namen und normalisierte Gesichtsboxen im Fotoindex und liefert sie in der Foto-JSON-API aus. Sie bleiben eigene Metadaten: Gesichtsnamen werden nicht automatisch zu Foto-Tags, erscheinen nicht in Tag-Listen und sind gezielt über `person:` oder `face:` suchbar.

In der Vollansicht zeigt die Info-Seitenleiste den vollständigen Aufnahmezeitpunkt mit Datum und Uhrzeit an.

## Index und Worker

Unter `Einstellungen -> Fotos` kann die Foto-Track-Auflösung der Karte in sinnvollen Stufen von 500 m bis 10 km eingestellt werden. Sie legt fest, wie nah GPS-Fotos liegen müssen, um im fotobasierten Karten-Track zu einem Trackpunkt zusammengefasst zu werden.

Die Kartenansicht passt ihre Höhe an das Browserfenster an und nutzt den zwischen Navigation, Filtern und Footer verfügbaren Bereich vollständig aus. Auf kleinen oder sehr kurzen Fenstern bleibt eine bedienbare Mindesthöhe erhalten.

Dort kann auch ein Index-Worker aktiviert werden. Er crawlt den Foto-Root ordnerweise im Hintergrund, überspringt unveränderte Ordner anhand ihres letzten Scan-Zeitpunkts und entfernt nicht mehr vorhandene Foto-, Ordner- und Blogeinträge ordnerlokal aus dem Index. Standardmäßig ist er deaktiviert; bei Aktivierung läuft er alle 60 Minuten mit niedriger I/O-Priorität, falls vom System unterstützt, und 250 ms Pause pro gescanntem Ordner.

Bei Stat- oder Blog-Lesefehlern bleibt der Index des betroffenen Ordners einschließlich manueller Tags unverändert. Der Scan wird später erneut versucht. Vollständige Scans entfernen tatsächlich verschwundene Einträge; zusammengehörige Metadaten, Tagzuordnungen und Such-/Vorschauindizes werden je Löschpaket beziehungsweise Ordnerteilbaum gemeinsam zurückgerollt, falls die Bereinigung scheitert oder abgebrochen wird.

Der separate Thumbnail-Worker ist ebenfalls standardmäßig deaktiviert. Bei Aktivierung läuft er alle 15 Minuten, erzeugt standardmäßig bis zu 15 fehlende Thumbnails pro Lauf und nutzt standardmäßig eine Thumbnail-Parallelität von 1.

GPX-Dateien werden bis 16 MiB und 100.000 eingelesene Track-/Routenpunkte verarbeitet; größere oder fehlerhafte Dateien werden übersprungen. Wartezeiten und Lesen reagieren auf einen Anfrageabbruch. Je Foto-Library läuft höchstens ein Parser gleichzeitig. Eine Kartenantwort enthält höchstens 256 Tracks und 250.000 Punkte; weitere Tracks, die das Budget überschreiten, werden ausgelassen. Der GPX-Cache hat ein Speicherbudget von 32 MiB für Punktarrays und Eintragskosten und verdrängt die am längsten ungenutzten Tracks.

Gültige Foto-Thumbnail-Cachetreffer lesen ihre Metadaten ohne SQLite-Schreibaufruf. Nur fehlende oder zu reparierende Queue-Metadaten werden nachgetragen; ein paralleler Datenbank-Writer blockiert reguläre Cachetreffer nicht.

## Cache-Statistik

Anzahl und Größe der Thumbnail-Dateien werden beim Start und danach alle 30 Minuten im Hintergrund ermittelt, auch wenn die Thumbnail-Erzeugung deaktiviert ist. Seitenaufrufe lesen den letzten vollständigen Stand und lösen keinen Dateisystemdurchlauf aus. Die Statistik nennt den Messzeitpunkt; vor der ersten Messung erscheint „Thumbnail-Cache wird ermittelt“. Bei Fehler oder Abbruch bleibt der vorherige Stand erhalten. Gleichzeitige Aktualisierungen teilen sich einen Durchlauf; temporäre Dateien und symbolische Links werden nicht mitgezählt.

## Foto-Ordner

Beim Zurückkehren aus einem Unterordner über den Fotopfad oder Browser-Zurück/Vorwärts erscheint die Ordnerliste wieder an der vorherigen Scrollposition. Pfadlinks behalten die zuvor besuchte Sortierung, Filter und Seite bei. Ändert sich die Fensterbreite, bleibt der betretene Ordner an seiner bisherigen Bildschirmposition. Die Positionen werden innerhalb des aktuellen Tabs gespeichert.

Ordnerthumbnails zeigen Bilder bei **20 %, 40 %, 60 % und 80 %** der sichtbaren Bilder einschließlich Unterordnern. Die Auswahl verwendet absteigende Datumsreihenfolge: Aufnahmedatum, ersatzweise Dateiänderungsdatum. Bruchteile einer Bildposition werden aufgerundet; bei 100 Bildern sind es Bild 20, 40, 60 und 80.

Ordner mit bis zu vier Bildern zeigen jedes Bild höchstens einmal. Eine kleinere konfigurierte Vorschauanzahl nutzt die ersten dieser Positionen. Ordner ohne Bilder behalten Video-/Audiovorschauen. Ausgeblendete Admin-only-Bilder zählen nicht zu den Positionen der öffentlichen Ansicht. Bestehende Vorschauzuordnungen werden automatisch erneuert und die Auswahl wird zwischengespeichert.

Ordner können nach Ordnerstandard, Name, Datum oder zufällig sortiert werden. Die Datumssortierung von Ordnern nutzt das aus dem Ordnernamen erkannte Anzeigedatum. Die Ordnerstandard-Sortierung wird über eine leere Steuerdatei im Ordner gesetzt:

- `.order_descending_name.pg2conf`
- `.order_ascending_name.pg2conf`
- `.order_descending_date.pg2conf`
- `.order_ascending_date.pg2conf`
- `.order_random.pg2conf`

Ein Ordner mit der Datei `.adminonly` ist nur für Benutzer mit der Rolle `admin` zugänglich. Admin-only-Inhalte sind auch für Admins standardmäßig in Galerie, Suche, Zufall, Fotoframe, Kartenansicht und Foto-Tag-Listen ausgeblendet. Admins können sie im Sortieren-Menü der Galerie per Schalter einblenden; die Auswahl bleibt in der aktuellen Session gespeichert. Direkte Medien- und Thumbnail-URLs bleiben weiterhin nur Admins vorbehalten.

Auch ein Symlink namens `.adminonly` schützt ab 0.42.1 den Ordner, unabhängig davon, ob sein Ziel erreichbar ist. Fehler beim Lesen der Markierung erlauben keinen öffentlichen Zugriff. Kann der Sichtbarkeitsabgleich beim Start einen indizierten Pfad wegen Zugriffsfehlern oder Symlinks nicht sicher prüfen, startet das Fotomodul nicht; Indexflags und Gesichtsdaten bleiben dabei unverändert. In diesem Fall den gemeldeten Pfad beziehungsweise dessen Zugriffsrechte korrigieren und BearStack erneut starten.

## Berechtigungen

Fotorechte sind capability-basiert und werden über Rollen oder einzelne Permissions vergeben:

| Rolle oder Recht | Wirkung |
| --- | --- |
| `photos_read` | Galerie, Suche, Medien, Thumbnails, Zufallsbild und Fotoframe lesen |
| `photos_editor` | zusätzlich Foto- und Ordner-Tags sowie Personen bearbeiten und die Personen-App nutzen |
| `photos_manager` | zusätzlich Fotoeinstellungen, Foto-Tag-Bibliothek, Index-Worker und Thumbnail-Worker verwalten |
| `admin` | vollständige Verwaltung und Zugriff auf `.adminonly`-Ordner |

Einzelrechte heißen `photos.read`, `photos.edit` und `photos.manage`. `.adminonly`-Ordner bleiben bewusst an die Rolle `admin` gebunden. Die vollständige Matrix steht unter [Benutzer und Rechte](benutzer-und-rechte.md).

Externe Werkzeuge wie `ffmpeg` und optional `vipsthumbnail` werden nur für Medienfunktionen benötigt, die sie tatsächlich brauchen.

### Optionale lokale Gesichtserkennung

Ab 0.24.0 kann BearStack Gesichter automatisch erkennen und ähnliche Gesichter zu
Personen gruppieren. Die Funktion ist standardmäßig aus. Unter **Fotos → Personen**
sehen Fotoleser die Gruppen; Fotobearbeiter (`photos.edit`, Rolle „Fotos bearbeiten“) können Namen vergeben,
Gruppen zusammenführen, ausgewählte Gesichter in andere oder neue Gruppen verschieben
und Fehlfunde ignorieren. Der Filter „Ignorierte Gesichter“ unter `/photos/people` zeigt einzelne ignorierte Gesichter, auch aus vollständig ausgeblendeten Gruppen. Er bleibt beim Blättern erhalten. Die Weboberfläche zeigt dafür keine Namenssuche; bestehende API-Kombinationen mit `q` und `known=1` bleiben unterstützt. Ab 0.50.0 holt **„Wiederherstellen“** ein ignoriertes Gesicht als neue unbenannte Gruppe zurück, auch wenn es vorher einer benannten Person angehörte. Der **Stift** öffnet den gemeinsamen Dialog mit Originalvorschau: Ein neuer Name benennt das Gesicht und stellt es wieder her; die Auswahl einer vorhandenen Person ordnet ausschließlich dieses Gesicht zu und stellt es wieder her. Andere aktive oder ignorierte Gesichter der früheren Gruppe bleiben unverändert. Abbrechen verändert nichts. Die direkte Wiederherstellung funktioniert auch ohne JavaScript. Mit JavaScript verschwinden bestätigte Wiederherstellungen ohne Seitenreload aus der Liste; Filter, Seitenposition und unveränderte Thumbnails bleiben erhalten. Bei unbestätigten Antworten muss die Ansicht vor einer weiteren Aktion erneut geladen werden. Bereits wiederhergestellte Gesichter werden durch eine alte Wiederherstellungsanfrage nicht erneut verschoben. Die Liste verwendet die vorhandenen Gesichtsvorschauen aus dem Cache und lädt höchstens 60 Gesichter pro Seite. Mit „Nur bekannte Personen“ zeigt die Personenübersicht ausschließlich benannte Gruppen; Namenssuche und Seitenwechsel behalten den Filter bei. Ab **0.44.0** zeigt **„Nur unbekannte Personen“** ausschließlich unbenannte Gruppen mit aktiven, nicht ignorierten Gesichtern. Benannte Gruppen und vollständig ignorierte Gruppen entfallen; bei gemischten Gruppen zählen und erscheinen nur die aktiven Gesichter. Der Filter verwendet `unknown=1`, bleibt beim Blättern, nach AJAX-Aktionen und bei der Rückkehr aus Personendetails erhalten und wird im Browser pro Benutzer gespeichert. Ab **0.50.0** wechseln die Filterbuttons **„Alle“**, **„Benannt“**, **„Unbenannt“** und **„Ignoriert“** direkt zwischen den Ansichten, auch ohne JavaScript. Der aktive Filter ist hervorgehoben. Der Wechsel beginnt auf Seite 1 und erhält die Sortierung. Unter „Unbenannt“ und „Ignoriert“ entfallen Namenssuchfeld und Suchbutton; ein vorheriger Suchtext wird beim Wechsel verworfen. Die Auswahl bleibt exklusiv. Unter „Nur unbekannte Personen“ entfällt die wiederholte sichtbare Beschriftung „Unbenannt“; zugängliche Link- und Aktionsnamen bleiben erhalten. Bei widersprüchlichen URL-Parametern hat `unknown=1` Vorrang. Die Namenssuche steht in den Ansichten „Alle“ und „Benannt“ zur Verfügung. Bestehende API-Aufrufe mit `q` behalten ihr bisheriges Verhalten. Benannte oder vollständig ignorierte Gruppen verschwinden nach der Bearbeitung sofort aus dieser Ansicht. Die Filterbuttons wechseln die Ansicht direkt; eine zusätzliche Rücksetzaktion entfällt. Einen Namenssuchtext kann man im Suchfeld löschen und mit „Suchen“ übernehmen. Die Sortierung wird unabhängig davon ausgewählt. Fotobearbeiter können Personen per Checkbox ohne sichtbaren Beschriftungstext auswählen; für Screenreader bleibt die personenbezogene Beschriftung erhalten. Ab zwei ausgewählten Gruppen erscheint unten rechts „Personen zusammenführen“ mit dem Zielnamen. Benannte Gruppen haben beim Zusammenführen Vorrang vor unbenannten: Die zuerst ausgewählte benannte Person bleibt samt Namen erhalten. Sind alle Gruppen unbenannt, bleibt die zuerst ausgewählte Person erhalten; alle ausgewählten Gruppen werden atomar per AJAX zusammengeführt und die Übersicht ohne Seitenreload aktualisiert. Die Auswahl gilt für die aktuelle Seite. Fotobearbeiter können in der Personenübersicht bei unbenannten Gruppen mit „×“ das angezeigte Gesicht ohne Seitenreload ignorieren. Bereits benannte Gruppen zeigen ab 0.39.3 weder dieses × noch den Stift für den Benenn-Dialog. Die Symbole verschwinden auch nach dem Benennen oder Zusammenführen per AJAX sofort. Weitere Gesichter der Gruppe bleiben erhalten; Vorschaubild und Fotoanzahl aktualisieren sich automatisch. Die Fotoinfo verlinkt erkannte Personen und trennt mehrere Namen mit „·“. Die Kopfaktionen der Personenseiten erscheinen als Navigationsbuttons. In der Personenübersicht öffnet bei unbenannten Gruppen das dezente Stiftsymbol in der unteren rechten Kartenecke „Benennen / zuordnen“ für Fotobearbeiter einen Dialog mit Namensvorschlägen und „Neu anlegen“. Ein neuer Name benennt die gesamte Gruppe; die Auswahl einer vorhandenen Person führt beide Gruppen zusammen. Speichern und Aktualisieren erfolgen ohne Seitenreload; Suchfilter und Seitenposition bleiben erhalten. Fehler werden im Dialog angezeigt. Über „Auswahl benennen / zuordnen“ lassen sich markierte Gruppen gemeinsam bearbeiten: Ein neuer Name führt sie unter diesem Namen zusammen; eine vorhandene benannte Person wird zum gemeinsamen Ziel. Eine bereits markierte Person kann ebenfalls Ziel sein. Die gesamte Änderung erfolgt atomar. Ab 0.43.0 zeigt das Benenn-Modal bei vorhandenen Personen zusätzlich ein Thumbnail neben Name, ID und Fotoanzahl. Es verwendet dasselbe aktive Gesicht wie die Personenübersicht. Die kleinen Vorschaubilder werden bei Bedarf aus dem vorhandenen Cache geladen; auch bei einem Bildfehler bleibt der Vorschlag auswählbar. „Neu anlegen“ erhält kein Thumbnail. Die Vorschlagsliste nutzt die freie Höhe neben der Fotovorschau und scrollt bei längeren Trefferlisten innerhalb dieses Bereichs. Im Benenn-Modal speichert ein Klick auf einen Namensvorschlag oder „Neu anlegen“ sofort; dasselbe gilt für die Auswahl mit Pfeiltasten und Enter. Dies funktioniert auch für mehrere markierte Gruppen ohne Seitenreload. Autovervollständigungen zeigen ausschließlich benannte Personen; die Suche filtert diese bereits auf dem Server. Ab 0.50.0 zeigt die Personeneinzelansicht `/photos/people/{id}` direkt kompakte Gesichtskarten. „Person benennen / zuordnen“ im Kopf bearbeitet die gesamte Gruppe; der Stift an einer Karte ausschließlich dieses Gesicht. Beide öffnen den gemeinsamen Dialog mit Originalvorschau, aufbereitetem Bildpfad, Namensvorschlägen und Lupe für ähnliche benannte Personen. Markierte Gesichter bekommen unten eine Aktionsleiste zum gemeinsamen Benennen, Zuordnen oder Ignorieren. Ein leerer Name im Auswahldialog trennt die Auswahl als neue unbenannte Gruppe ab. „Alle auf dieser Seite auswählen“ umfasst höchstens die 60 sichtbaren Gesichter. Auf jeder Gesichtskarte sitzt die Auswahlcheckbox oben links, Ignorieren (×) oben rechts, der Vergleichsstern unten links und der Stift unten rechts. Die Kennzeichnung manueller Zuordnung steht unten mittig; die Bedienflächen sind mindestens 44 Pixel groß. Unter „Anzeige“ lassen sich die Thumbnailgröße S/M/L und der zusätzliche Bildpfad einstellen; der Browser merkt sich dies pro Benutzer. Lange Dateinamen erscheinen gekürzt, der vollständige aufbereitete Pfad bleibt im Tooltip und Dialog erreichbar. Eine ausklappbare Hilfe erklärt die Aktionen und Vergleichssterne. Bestätigte Änderungen aktualisieren die Ansicht ohne Seitenreload und erhalten unveränderte Bildknoten. Bei einer unbestätigten Verschiebe- oder Ignorieraktion bleibt die Bearbeitung bis zur erneuten Aktualisierung gesperrt. Die Paginierung bietet „Erste Seite“, „Zurück“, „Weiter“ und „Letzte Seite“ sowie die aktuelle Seite und Gesamtseitenzahl. Auf schmalen Bildschirmen erscheinen diese Aktionen als Pfeilbuttons in einer kompakten Zeile mit verkürzter Seitenangabe; Tooltips und Screenreader behalten die ausgeschriebenen Bezeichnungen. Dies gilt auch nach AJAX-Aktualisierungen. Nicht verfügbare Randaktionen sind bei mehreren Seiten deaktiviert. Bei nur einer Seite bleibt ab 0.43.1 ausschließlich „Seite 1 von 1“ sichtbar; „Erste Seite“ und „Letzte Seite“ entfallen. In Personendetails entfällt ab 0.50.0 die Paginierung bei nur einer Seite vollständig. Personenübersicht und ignorierte Gesichter behalten die Seitenangabe, auch nach AJAX-Aktualisierungen. Die Personenübersicht merkt sich Seite und Filter im Browser pro Benutzer, auch über Browser-Neustarts hinweg. Beim erneuten Öffnen von `/photos/people` wird diese Ansicht wiederhergestellt; ausdrücklich angegebene Seiten oder Suchfilter haben Vorrang. Nicht mehr vorhandene Seiten werden auf die letzte verfügbare Seite begrenzt. Dies gilt auch für AJAX-Aktualisierungen; Personendetailseiten und ignorierte Gesichter haben passende Gesamtseitenzahlen. Auf Personenseiten bleiben die Bildflächen quadratisch; lange Dateinamen und optionale Pfade werden platzsparend gekürzt. Ab BearStack 0.39.2 werden Gesichtsausschnitte proportional eingepasst und zentriert; freie Flächen erhalten einen dunkelgrauen Hintergrund. Die Vorschauen werden bereits auf dem Server unverzerrt erzeugt, sodass dies auch in der Android-App ohne App-Update gilt. Bereits im App-Speicher geladene Altbilder werden nach erneutem Anmelden oder einem App-Neustart neu geladen. Beim Überfahren wird der Dateipfad unterstrichen, die Bildfläche bleibt unverändert. Auf einer Personenseite öffnen Gesichtsbild und Dateiname die Foto-Lightbox; dort stehen Navigation, Zoom und Bildinformationen zur Verfügung. `person:Juergen`
und `face:"Marie Curie"` suchen sowohl XMP-Namen als auch benannte automatische Gruppen.

Gesichtsvorschauen werden unter `BEARSTACK_PHOTOS_CACHE_DIR/faces/v1` als JPEG zwischengespeichert, auch über Neustarts hinweg. Aktive Gesichtsvorschauen bleiben ohne Größen- oder Anzahlbegrenzung gespeichert. Beim Ignorieren erhalten vorhandene Vorschauen eine feste Ablaufzeit von höchstens 48 Stunden; erneute Aufrufe oder wiederholtes Ignorieren verlängern diese Frist nicht. Abgelaufene Vorschauen werden auch ohne weitere Zugriffe automatisch gelöscht. Ein späterer Aufruf kann eine neue Vorschau erzeugen, die ebenfalls höchstens 48 Stunden gespeichert wird. Gesichtsdaten, Embeddings und die Möglichkeit zur Wiederherstellung bleiben dauerhaft erhalten. Ignorierte Gesichter dienen zu keinem Zeitpunkt als Referenzen für den Gesichtsabgleich. Beim Wiederherstellen entfällt die Ablaufzeit der Vorschau. Die Bereinigung läuft unabhängig von der aktivierten Gesichtserkennung, berücksichtigt Neustarts und verarbeitet nur fällige Cacheeinträge in kleinen, indexgestützten Paketen. Beim Update bleibt der bestehende Cache unter `faces/v1` erhalten. Die Zuordnung zu Gesichts-IDs wird einmalig im Hintergrund in Paketen von höchstens 100 Gesichtern ergänzt und nach Unterbrechungen fortgesetzt. Dazu werden vorhandene Indexdaten und die beiden möglichen Vorschaupfade geprüft; Originalbilder werden weder gelesen noch neu dekodiert. Die Zuordnung allein erhält Dateinamen, Bildinhalt und Änderungszeiten. Ab 0.39.2 verwenden proportional eingepasste Vorschauen einen neuen Rendering-Schlüssel: Beim nächsten Abruf wird die angeforderte Größe einmal neu erzeugt und nur ihre konkrete gestreckte Altvorschau entfernt. Andere Größen und nicht angeforderte Vorschauen bleiben bis zu ihrem Abruf oder einer bestehenden Ablaufzeit erhalten. Es gibt keinen vollständigen Cache-Neuaufbau; fehlgeschlagene Bereinigungen werden beim nächsten Abruf erneut versucht. Ignorierte Altvorschauen erhalten eine feste Frist von höchstens 48 Stunden ab dem Update, die bei Neustarts nicht verlängert wird. Nicht mehr zuordenbare Altdateien werden nicht pauschal gelöscht. Hashbasierte Unterverzeichnisse vermeiden übergroße Einzelordner; Cache-Zugriffe benötigen keinen Verzeichnisscan und keine vollständige Dateiliste im Arbeitsspeicher. Die Größen 160 und 640 Pixel werden getrennt gespeichert; parallele Abrufe desselben Ausschnitts teilen sich eine Erzeugung. Personenübersicht, Personendetailseite, AJAX und Android-/Labeling-API verwenden denselben Datei-Cache: Vorhandene Vorschauen werden direkt daraus ausgeliefert; fehlt eine Vorschau, wird sie erzeugt und dort gespeichert. Der Browser darf Web-Gesichtsvorschauen privat speichern, muss sie aber vor erneuter Verwendung validieren (ETag/304). Authentifizierung, aktuelle Sichtbarkeit und Quelldatei werden weiterhin geprüft. Bei AJAX-Aktualisierungen bleiben vorhandene Bild-Elemente erhalten; Änderungen an Namen oder Fotoanzahlen laden unveränderte Bilder nicht erneut.

Unter **Einstellungen → Gesichtserkennung** stehen Statuszahlen, Verarbeitung und
Löschaktion in getrennten Bereichen. Die Einstellungsnavigation steht auf großen
Bildschirmen links und ist auf kleinen Bildschirmen als horizontale Reiterleiste erreichbar.

Die Verarbeitung erfolgt in einem separaten lokalen Dienst mit OpenCV/YuNet/SFace.
Bilder verlassen die eigene Infrastruktur nicht. Der Dienst hat weder Zugriff auf
das Fotoverzeichnis noch auf die Datenbank. BearStack sendet ausgerichtete, auf
höchstens 1.600 Pixel verkleinerte JPEGs. Modelle werden bei der Einrichtung beziehungsweise beim Image-Build mit festen
SHA-256-Prüfsummen geladen und beim Start geprüft; im Betrieb gibt es keine Downloads.

**Einrichtung mit Compose:**

1. Einen zufälligen Token erzeugen, beispielsweise mit `openssl rand -hex 32`.
2. In `.env` `BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN` auf diesen Token setzen.
3. Fotomodul und Fotoverzeichnis wie oben konfigurieren und
   `docker compose --profile faces up -d --build` starten.
4. Unter **Einstellungen → Gesichtserkennung** die Verarbeitung einschalten.
   Aktivierung ist nur bei erreichbarem, kompatiblem Dienst möglich.

Compose verwendet intern `http://faces:8091`, ohne veröffentlichten Dienstport.
Eine eigene Dienstadresse lässt sich über folgende optionale Werte setzen:

| JSON-Feld unter `photos` | Umgebungsvariable | Bedeutung |
| --- | --- | --- |
| `face_service_url` | `BEARSTACK_PHOTOS_FACE_SERVICE_URL` | HTTP(S)-Adresse des eigenen Erkennungsdienstes. |
| `face_service_token` | `BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN` | Gemeinsamer geheimer Token, mindestens 32 Zeichen. |

##Ab BearStack **0.52.4** verwenden Personen-Übersichtskarten und die Treffer-Thumbnails im Benennen-Dialog in WebUI und Android ausschließlich sichtbare, aktive **Stern-Favoriten**, sofern die Gruppe welche besitzt. Bei mehreren Favoriten wird stabil der mit der kleinsten Gesichts-ID gezeigt. Ohne Favoriten bleibt die bisherige Bildauswahl erhalten. Das gilt auch für die Portraits der Lupentreffer; Personenerkennung, Vergleichsreferenzen, Such-Ausgangsgesichter und Trefferreihenfolge bleiben unverändert. Die Auswahl erfolgt serverseitig über den vorhandenen Favoritenindex, ohne zusätzliche Bildabrufe oder Datenmigration. Die Android-App erhält die korrigierten Portraits nach dem Serverupdate über die bestehenden Antworten.

## Personen sortieren

Personenübersicht und Personendetailseite verwenden dieselbe Kacheldarstellung mit
einheitlichen Größen S/M/L, Bildflächen, Abständen, Rahmen und Aktionspositionen.
Die Übersicht zeigt Personennamen und Fotoanzahl, die Detailseite den Dateinamen
und zusätzlich den Vergleichsstern.

Die Kopfzeile der Personenübersicht bietet **← Fotos**, **Gruppenbilder**,
**Ähnliche Gesichter** und ein Zahnrad für die Einstellungen zur Gesichtserkennung.
Textbuttons und Zahnrad haben eine einheitliche Mindesthöhe von 44 Pixeln.
Unterseiten führen mit **← Alle Personen** zurück zur Übersicht.

Ein dünner gemeinsamer Rahmen fasst die Filter **Alle**, **Benannt** und **Unbenannt**
zusammen. **Ignoriert** steht außerhalb: **Alle** enthält keine ignorierten Gesichter.
Sortierung und Dreipunkt-Menü stehen in derselben Zeile wie die Filterbuttons und
brechen auf schmalen Bildschirmen passend um. Nur das Namenssuchfeld mit **Suchen**
erhält einen zusätzlichen Kasten; bei **Unbenannt** und **Ignoriert** entfällt dieser.

Unter **`/photos/people` → Sortieren** stehen **Name**, **Anzahl Fotos**, **Ordner** und
**Datum** jeweils auf- und absteigend zur Verfügung. Standard ist **Name: A–Z**;
Umlaute werden wie bei der Namenssuche behandelt. Mit JavaScript übernimmt ein
Auswahlwechsel die Sortierung sofort und öffnet Seite 1. Ohne JavaScript übernimmt
**Suchen** die Auswahl; unter „Unbenannt“ und „Ignoriert“ steht dafür ein **Sortieren**-Button bereit.

**Anzahl Fotos** zählt unterschiedliche sichtbare Fotos mit aktiven Gesichtern einer
Person, auch wenn diese mehrfach im selben Bild vorkommt. **Ordner** verwendet den
angezeigten Ordner des Vorschaubilds; der Hauptordner heißt „Fotos“. Andere Bilder
der Person können in weiteren Ordnern liegen. **Datum** verwendet das neueste ihrer
aktiven Fotos: die Aufnahmezeit, ersatzweise die Dateiänderungszeit. Unterschiedliche
Zeitzonen werden vor dem Vergleichen normalisiert. Private Fotos und ignorierte
Gesichter beeinflussen die aktiven Gruppen nicht.

Beim Filter **Ignorierte Gesichter** beziehen sich Ordner und Datum auf das jeweilige
Foto; die Anzahl zählt unterschiedliche Fotos mit ignorierten Gesichtern derselben
Gruppe. Die Namenssortierung verwendet deren Gruppennamen. Auch diese Ansicht
sortiert standardmäßig nach Name und bleibt mit Namenssuche kombinierbar.

Die Datenbank sortiert **vor** der Seiteneinteilung. Gleichstände werden über die ID
stabil aufgelöst. Pro Seite werden weiterhin höchstens 60 Einträge ausgeliefert;
Vorschaugeometrie wird nur für diese Seite ermittelt. Die Namenssortierung nutzt den
vorhandenen Namensindex. Andere Sortierungen verwenden die vorhandenen Gesichts-
und Medienindizes und können temporäre Sortierdaten auf Datenträger auslagern. Es
werden keine Originalbilder gelesen und kein dauerhafter Sortiercache angelegt;
Änderungen wirken beim nächsten Abruf ohne erneute Bilderkennung.

Auswahl und Richtung bleiben beim Blättern, nach AJAX-Bearbeitungen und bei der
Rückkehr aus Personendetails erhalten. Der Browser speichert sie zusammen mit Seite
und Filtern pro Benutzer. Ausdrückliche URL-Parameter haben Vorrang. **Alle Filter
aufheben** setzt auch die Sortierung auf **Name: A–Z** zurück. Personendetails und
Namensvorschläge behalten ihre bisherige Reihenfolge.

Die kompatible Erweiterung verwendet `sort=name_asc`, `name_desc`, `count_asc`,
`count_desc`, `folder_asc`, `folder_desc`, `date_asc` oder `date_desc`; ungültige Werte
liefern HTTP 400. HTML und JSON verwenden dieselbe Reihenfolge. Die JSON-Seite enthält
zusätzlich `sort`. Die Änderung gehört zur unveröffentlichten MINOR-Version 0.50.0;
es ist keine Schemaänderung oder Migration erforderlich.

#### Einrichtung ohne Compose (native Installation)

BearStack und der Erkennungsdienst können auf demselben Rechner direkt laufen.
Voraussetzungen sind ein eingerichtetes Fotomodul, die BearStack-Quelldateien passend
zur installierten Version, **Python 3.12 oder neuer mit `venv`**, OpenSSL und curl.
Eine GPU ist nicht erforderlich. Die folgenden Befehle werden im BearStack-Projektordner
ausgeführt; für die Installation der Python-Pakete und Modelle wird Internetzugang benötigt.

1. **Python-Umgebung und Modelle installieren:**

    ```sh
    python3 --version
    python3 -m venv .venv-faces
    .venv-faces/bin/python -m pip install --only-binary=:all: -r services/faces/requirements.txt
    .venv-faces/bin/python services/faces/download_models.py "$HOME/.local/share/bearstack-face-models"
    ```

    Fehlt `venv`, das zur Python-Version passende Paket des Betriebssystems installieren
    (unter Debian/Ubuntu beispielsweise `python3-venv`). Das Downloadskript lädt die
    festgelegten Modelle samt Lizenzhinweisen und prüft ihre SHA-256-Prüfsummen.
    Die Modelldateien bleiben lokal erhalten; dieser Schritt ist nur bei der Einrichtung
    oder einem vorgesehenen Modellupdate nötig.

    Scheitert der Download unter macOS mit `CERTIFICATE_VERIFY_FAILED`, kann der
    Python-Installation ihr CA-Bundle fehlen. Ist `/etc/ssl/cert.pem` vorhanden,
    lässt sich das System-CA-Bundle für diesen Aufruf verwenden:

    ```sh
    SSL_CERT_FILE=/etc/ssl/cert.pem .venv-faces/bin/python services/faces/download_models.py "$HOME/.local/share/bearstack-face-models"
    ```

    TLS-Zertifikate und Modell-Prüfsummen werden dabei weiterhin geprüft.

2. **Gemeinsamen Token erzeugen:**

    ```sh
    openssl rand -hex 32
    ```

    Den erzeugten Wert in den folgenden Beispielen anstelle von `TOKEN_HIER_EINTRAGEN`
    verwenden. Erkennungsdienst und BearStack müssen denselben Token erhalten.

3. **Erkennungsdienst in einem eigenen Terminal starten:**

    ```sh
    export BEARSTACK_FACE_MODELS_DIR="$HOME/.local/share/bearstack-face-models"
    export BEARSTACK_FACE_SERVICE_TOKEN='TOKEN_HIER_EINTRAGEN'
    export BEARSTACK_FACE_BIND=127.0.0.1
    .venv-faces/bin/python services/faces/server.py
    ```

    Der Dienst lauscht auf `127.0.0.1:8091`. Er benötigt Leserechte auf die Modelle,
    aber keine Rechte auf Fotoverzeichnis oder BearStack-Datenbank. Das Terminal bleibt
    während des Betriebs geöffnet. Der Python-Dienst liest `.env` **nicht** automatisch;
    seine Variablen müssen in der Prozessumgebung gesetzt sein.

4. **Erreichbarkeit in einem zweiten Terminal prüfen:**

    ```sh
    export BEARSTACK_FACE_SERVICE_TOKEN='TOKEN_HIER_EINTRAGEN'
    curl --fail --silent --show-error \
      -H "Authorization: Bearer $BEARSTACK_FACE_SERVICE_TOKEN" \
      http://127.0.0.1:8091/health
    ```

    Die JSON-Antwort muss `"ready": true`, `"protocol": 1` und die Modellkennung
    `yunet-2023mar-sface-2021dec-v1` enthalten. Bei HTTP 401 stimmt der Token nicht;
    bei einem Verbindungsfehler zuerst das Dienstterminal prüfen.

5. **BearStack verbinden und neu starten:** Die folgenden Werte in die von BearStack
    gelesene `.env`-Datei beziehungsweise seine Prozessumgebung eintragen:

    ```env
    BEARSTACK_PHOTOS_FACE_SERVICE_URL=http://127.0.0.1:8091
    BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN=TOKEN_HIER_EINTRAGEN
    ```

    Bei der systemd-Beispielinstallation können stattdessen die Felder
    `face_service_url` und `face_service_token` im vorhandenen `photos`-Objekt in
    `/etc/bearstack/bearstack.json` ergänzt werden. Danach `sudo systemctl restart bearstack`
    ausführen. Die übrige Foto-Konfiguration beibehalten und die Datei mit dem Token
    nur für den Administrator und den jeweiligen Dienst lesbar halten.

6. **Verarbeitung aktivieren:** Als Foto-Verwalter unter **Einstellungen → Gesichtserkennung**
    einschalten. Dort erscheinen Fortschritt und Fehler; die Ergebnisse stehen unter
    **Fotos → Personen** bereit. Der vorhandene Fotobestand wird schrittweise verarbeitet.

Für dauerhaften Betrieb muss der Python-Prozess ebenfalls durch einen Dienstmanager
wie systemd gestartet werden. Dabei einen absoluten Pfad zu `.venv-faces/bin/python`,
`services/faces/server.py` und den Modellen verwenden sowie die drei Dienstvariablen
aus Schritt 3 setzen. BearStack startet diesen Prozess nicht selbst. Die Begrenzungen
auf einen halben CPU-Kern und 1 GiB RAM gelten nur für die Compose-Konfiguration;
bei nativem Betrieb müssen entsprechende Ressourcenlimits im Dienstmanager gesetzt werden.
Der einzelne Inferenzthread und die BearStack-Pausen gelten auch ohne Compose.

#### Verarbeitung, Metadaten und Datenschutz

**Schonender Betrieb:** Ein Worker verarbeitet standardmäßig 100 Bilder, mit einer
Sekunde Pause pro Bild und 15 Minuten Wartezeit zwischen Läufen. Der Dienst nutzt
einen Inferenzthread; Compose begrenzt ihn auf 0,5 CPU-Kerne und 1 GiB RAM. Die
Einstellungen erlauben 1–1.000 Bilder pro Lauf, 100–60.000 ms Pause und 1–1.440 Minuten
Wartezeit. Der Erstlauf holt die Sammlung schrittweise nach. Wenn der separate
Fotoindex-Worker aus ist, nutzt die Gesichtserkennung dessen Scanmechanismus stündlich.
Nach dem Erstlauf entstehen neue Aufträge direkt bei Indexänderungen. Unveränderte
Bilder einschließlich Ergebnissen ohne Gesicht werden nicht erneut analysiert.
Fehler werden mit zunehmender Wartezeit bis zu fünfmal versucht und können manuell
zurückgesetzt werden. Neustarts setzen die persistente Warteschlange fort.

Die Info-Symbole in den Einstellungen der Gesichtserkennung erklären Aktivierung, Bilder pro Lauf, Pausen, Referenzlimit und das Löschen der Gesichtsdaten. Die Kontexthilfen lassen sich per Klick oder Tastatur öffnen. Zum Löschen aller Gesichtsdaten sind die Löschbestätigung und das aktuelle Passwort des angemeldeten Nutzers erforderlich. Das Passwort wird vor jeder Zustandsänderung serverseitig geprüft; fehlende oder falsche Bestätigungen lassen Gesichtsdaten und Verarbeitung unverändert. Wiederholte Fehlversuche werden gedrosselt.

**Personenrechte:** Seit BearStack 0.36.0 reicht „Fotos bearbeiten“ (`photos_editor` bzw. `photos.edit`) zum Benennen, Zuordnen, Zusammenführen, Ignorieren und Wiederherstellen von Gesichtern sowie für die Android-App. Einstellungen der Gesichtserkennung und das Löschen aller Gesichtsdaten benötigen weiterhin „Fotos verwalten“ (`photos.manage`).

**Optimierungen ab 0.41.1:** Gesichtsausschnitte desselben Fotos teilen dessen
verkleinerte und ausgerichtete Bildaufbereitung. Der Cache hält höchstens acht Bilder
und 32 MiB im Arbeitsspeicher. Dateiaustausch und geänderte XMP-Sidecars ändern den
Cache-Schlüssel; Quell- und Schutzprüfungen bleiben auch bei Cachetreffern aktiv.
Kann der Index einen geänderten Foto-Fingerprint nicht speichern, wird der Abruf
abgebrochen, statt alte Gesichtsregionen weiterzuverwenden.

Das fünfsekündliche Status-Polling verwendet `/settings/photos/faces?format=json&progress=1`.
Dieser Modus liefert Zähler des zuletzt indexierten Bestands, ohne globale
Dateisystemprüfung und ohne Liste einzelner Dateifehler. Mehrere Aufrufe teilen
höchstens fünf Sekunden lang denselben Zählerstand. Neue Schutzmarkierungen wirken
auf diese Zahlen nach der nächsten Index- oder Sichtbarkeitsaktualisierung; konkrete
Gesichtsabrufe prüfen den Schutz weiterhin sofort. Die vollständige HTML-Ansicht
und JSON ohne `progress=1` prüfen die Sichtbarkeit wie bisher.

Der gemeinsame Personen-Dialog und seine Suche werden unabhängig von der
Personenübersicht geladen; die Gruppenbilder-Ansicht benötigt nur ihr eigenes
Modul und `app-person-dialog.js`.

Auf Smartphones stehen die Navigationslinks der Personenansicht kompakt in zwei
Spalten. Das Anzeigemenü sitzt neben den Filteraktionen, damit mehr Platz für
die Personenbilder bleibt.

Die Einzelpersonenansicht hat eine gemeinsame Aktionsleiste mit **Alle Personen**,
**Fotos ansehen**, dem **Stift für die gesamte Gruppe** und **Alle auswählen**.
Auf schmalen Bildschirmen bricht diese Leiste platzsparend um. Bei benannten
Personen zeigt der Gruppenbutton nur das Stiftsymbol; Tooltip und
Screenreader-Beschriftung bleiben verfügbar. **Alle auswählen** markiert nur die
Gesichter der aktuellen Seite.

Die **Personen-Tags** stehen direkt hinter dem Namen in derselben Zeile und lassen
sich dort mit Bearbeitungsrecht ändern. Bei wenig Platz darf die Zeile umbrechen.

Das **Mehr-Menü (⋯)** bündelt **Stammdaten**, **Anzeige**, **Hilfe** und
weitere Navigation. Die Einträge verwenden einen einheitlichen Menüstil.
Es öffnet über der Galerie, ohne Gesichtskarten zu verschieben.
**Hilfe** öffnet einen Dialog; **Schließen** oder Escape schließt ihn und setzt den
Tastaturfokus zurück auf den Mehr-Menü-Button. Thumbnailgröße und Ordnerpfade bleiben
gespeichert. Kleine Gesichtskarten füllen die verfügbare Breite gleichmäßig; auf
schmalen Smartphones passen zwei Spalten nebeneinander.

Unter der Fotovorschau im Benennen-Dialog steht der aufbereitete Bildpfad,
zum Beispiel **Fotos / 11.05.2026 · Urlaub / IMG_1234.jpg**. Er gehört immer zum
angezeigten Gesicht, auch nach dem Wechsel des Vorschaubilds oder Gruppenfotos.
Lange Pfade brechen mobil um; zusätzliche Serverabfragen sind nicht erforderlich.

**Gesichter direkt aus der Foto-Info bearbeiten:** Fotobearbeiter können im
Infopanel des geöffneten Bildes die Lupe **Gesichter erkennen und zuordnen** wählen.
Die kompakte Symbolleiste zeigt in einer Zeile das Gesichtsicon, die Lupe,
den Auswahlrahmen für **Gesicht einrahmen**, den Toggle **Beschriftete Gesichtsrahmen
anzeigen**, die Wiederherstellung ignorierter
Gesichter und den Refresh-Button. Alle Buttons
haben Tooltips, zugängliche Namen und 44 Pixel große Bedienflächen. Die Gesichtsliste
erscheint ohne zusätzlichen Zähl- und Bedienhinweis.
Bearbeitet wird nur dieses Foto, auch bei pausierter automatischer Verarbeitung;
ein konfigurierter lokaler Gesichtsdienst ist erforderlich. Der Vorgang wartet bei
Bedarf auf die gerade laufende Bildanalyse und nutzt dieselbe Erkennung und
Zuordnung wie der Hintergrundprozess. Erkannte Gesichter erscheinen mit Vorschaubild
und Name. Über den Stift lassen sie sich im bekannten Dialog benennen oder einer
vorhandenen Person zuordnen, einschließlich Live-Abgleich per Lupe. **Bei bereits
benannten Personen gilt eine Korrektur hier nur für das angeklickte Gesicht im
aktuellen Foto.** Ein neuer Name erstellt eine eigene Person für dieses Gesicht;
die Auswahl eines vorhandenen Namens verschiebt nur dieses Gesicht dorthin.
Andere Gesichter derselben Person – auch im selben Foto – behalten ihren Namen
und ihre Zuordnung. Unbenannte Gruppen werden weiterhin gemeinsam benannt oder
zugeordnet; der Dialog zeigt den jeweiligen Geltungsbereich an. Die gesamte
benannte Person lässt sich über die Personenübersicht umbenennen.
Nach dem Speichern werden die
Gesichter und Namen im Infopanel aktualisiert. **Ignorieren** im Dialog blendet
das angezeigte unbenannte Gesicht aus; andere Gesichter derselben Gruppe bleiben
erhalten. Bei zwischenzeitlichen Änderungen wird die aktuelle Gesichtsliste
geladen und die Auswahl muss erneut geprüft werden. Das kleine **Refresh-Symbol**
am Ende der Symbolleiste lädt bestehende Zuordnungen ohne neue Bildanalyse
(Tooltip: **Gesichter aktualisieren**). Videos und Audios bieten diese
Aktionen nicht an. Manuelle Korrekturen, ignorierte Gesichter und Favoriten werden
bei eindeutiger Wiedererkennung übernommen. Beim Bildwechsel wird die laufende
Anfrage abgebrochen; bereits gespeicherte Ergebnisse bleiben bestehen. Der neue
POST-Endpunkt `/photos/faces/analyze` benötigt `photos.edit`, aktiviert keinen
globalen Erkennungslauf und begrenzt Warten und Analyse zusammen auf zwei Minuten.
`GET /photos/faces?path=…` liefert die vorhandenen Gesichter mit denselben Rechten;
ein noch nicht analysiertes Foto liefert eine leere Gesichtsliste.

**Gesichtsrahmen im normalen Bild:** Der Toggle **Beschriftete Gesichtsrahmen
anzeigen** blendet vorhandene Rahmen samt vollständigen Namen oberhalb der Boxen
ein oder aus. Unbenannte und ignorierte Gesichter sind entsprechend gekennzeichnet.
Die Einstellung gilt für die gesamte Sitzung im Browser-Tab, auch beim Bild- oder
Seitenwechsel, Neuladen und bei geschlossener Info-Sidebar, bis der Toggle wieder
deaktiviert wird. Zoom und Verschieben bewegen Rahmen und Bild gemeinsam.
Bereits geladene Gesichtsdaten werden wiederverwendet; bei aktivem Toggle lädt ein
Bildwechsel die vorhandenen Gesichter auch ohne geöffnete Sidebar. Es wird keine
neue Gesichtserkennung gestartet. Videos und Audio zeigen keine Gesichtsrahmen.
Ignorierte Gesichtsrahmen einschließlich ihrer Labels erscheinen in diesem Modus
mit **20 % Deckkraft** (80 % transparent), damit sie das Foto weniger verdecken.

Der Button **Alle ignorierten Gesichter dieses Fotos wiederherstellen** entfernt ab
0.50.0 den Ignoriert-Status für alle betroffenen Gesichter im geöffneten Foto.
Vorhandene Namen, Personengruppen und manuelle Markierungen bleiben erhalten;
Gesichter anderer Fotos bleiben unverändert. Der Button ist nur bei ignorierten
Gesichtern aktiv und benötigt keinen Gesichtserkennungsdienst. Eine Prüfung des
angezeigten Datenstands verhindert Änderungen anhand veralteter Daten. Nach einer
unbestätigten Antwort zuerst **Gesichter aktualisieren** wählen; die Aktion wird
nicht automatisch wiederholt. Der neue POST-Endpunkt `/photos/faces/unignore`
nimmt `path` und `revision` entgegen, benötigt `photos.edit` und liefert die Anzahl
der wieder sichtbaren Gesichter. Danach aktualisiert die Infobar ihre Gesichtsliste.

**Große Gesichter in Porträts:** YuNet kann sehr große Gesichter in der
1600-Pixel-Vorlage übersehen. Der Gesichtsdienst ergänzt deshalb einen zweiten
Suchlauf mit höchstens 320 Pixeln Kantenlänge, auch wenn die erste Suche bereits
andere Personen gefunden hat. Bereits gefundene Regionen bleiben erhalten;
überlappende zusätzliche Treffer werden zusammengefasst. Rahmen und Augen-/Nasen-/
Mundpunkte werden auf die größere Vorlage zurückgerechnet. Zuordnung und
Qualitätsbewertung verwenden deren Bilddetails, nicht die verkleinerte Übersicht.
Modell, Vergleichsvektoren und Erkennungsschwelle bleiben kompatibel. Nach einem
Update muss der Gesichtsdienst neu gebaut bzw. gestartet und das betroffene Foto
über **Gesichter erkennen und zuordnen** erneut verarbeitet werden.

**Fehlendes Gesicht manuell einrahmen:** In der Foto-Info öffnet **Gesicht
einrahmen** das ausgerichtete Foto in einem nahezu fenstergroßen Dialog. Am Desktop
stehen die Eingaben neben der großen Zeichenfläche; mobil nutzt der Dialog den
ganzen Bildschirm mit scrollbarem Inhalt und stets erreichbaren Speicherbuttons.
Vorhandene Gesichtsrahmen zeigen die vollständigen Namen in kleiner Schrift
außerhalb oberhalb des Rahmens, ohne Begrenzung auf dessen Breite;
unbenannte Gesichter heißen **Unbenannt**, ignorierte Rahmen sind gestrichelt und
entsprechend beschriftet. Die Rahmen bleiben auch beim Ändern der Fenstergröße
am ausgerichteten Foto und behindern das Zeichnen nicht.
Mit Maus oder Finger einen Rahmen ziehen und die gemeinsame Namensvervollständigung
wie im Benennen-Dialog nutzen, einschließlich Gesichtsvorschau der vorhandenen
Personen. Die Auswahl einer Person oder von **Neu anlegen** speichert den Rahmen
direkt und schließt den Dialog nach erfolgreichem Speichern. Alternativ einen
neuen Namen eingeben und **Gesicht speichern** wählen. Ohne Rahmen wird nichts
gespeichert; bei einem Fehler bleibt der Entwurf zur Prüfung geöffnet.
**Rahmen mittig setzen** ermöglicht die Tastaturbedienung:
Pfeiltasten verschieben den Rahmen, Umschalt + Pfeiltasten ändern seine Größe.
Abbrechen verwirft den Entwurf vollständig. Die Markierung funktioniert ohne
Erkennungsdienst und erscheint sofort mit Vorschaubild und Name in der Info.

Manuell eingezeichnete Rahmen bleiben bei erneuter Erkennung erhalten; deutlich
überlappende automatische Treffer werden nicht doppelt angelegt. Die Region hat
keinen berechneten Gesichtsvektor und dient deshalb nicht als automatische
Vergleichsreferenz; die Namenssuche funktioniert weiterhin. Manuelle Regionen
werden nicht in der Statistik automatischer Zuordnungen mitgezählt. Änderungen
an der Bilddatei machen einen geöffneten Entwurf ungültig. Bereits gespeicherte
Regionen werden bei ersetzten oder entfernten Quelldateien wie andere Gesichter
verworfen. Pro Foto sind insgesamt höchstens 256 Gesichter zulässig. Schema 29
wird automatisch migriert. Die zusätzlichen Endpunkte
`GET /photos/faces/drawing-image` und `POST /photos/faces/manual` benötigen
`photos.edit`; sie aktivieren keinen Hintergrundlauf.

**Gesichtsabgleich im Benennen-Dialog:** Die Lupe rechts neben dem Namensfeld
vergleicht das aktuell angezeigte Gesicht auf Klick mit den gespeicherten
Referenzgesichtern benannter Personen. Bis zu 20 Kandidaten ab der konfigurierten Ähnlichkeit (Standard 0,45)
erscheinen bereits während des Abgleichs in derselben Vervollständigung, mit Name,
Fotoanzahl und dem passenden Vergleichsgesicht. Erste geprüfte Treffer sind sofort
auswählbar; bessere Treffer können die nach Ähnlichkeit sortierte Liste noch
verändern. „Abgleich läuft …“ kennzeichnet die vorläufigen Ergebnisse. Die aktuelle Gruppe ist
ausgeschlossen. Ein Treffer ist ein Vorschlag, keine sichere Identifikation;
die Auswahl ordnet wie bisher die gesamte Gruppe zu. Bei Mehrfachauswahl wird
das Gesicht aus der Dialogvorschau verglichen. Tippen startet wieder die
Namenssuche; verspätete Antworten werden verworfen. Die Suche nutzt den vorhandenen
Referenzcache einschließlich Favoriten, prüft Sichtbarkeit und Quellfoto aktuell
und benötigt weder eine erneute Bildanalyse noch einen laufenden Erkennungsdienst.
`GET /photos/faces/{id}/suggestions` benötigt `photos.edit` und ändert keine Zuordnung.
Der Dialog fordert `application/x-ndjson` an: Jeder Datensatz ersetzt die bisherige
Trefferliste, `done=true` schließt den Stream ab. Die normale JSON-Antwort bleibt
verfügbar. Abbruch, neue Eingaben und Dialogschließen stoppen die Anfrage; aktive
Tastaturauswahl und unveränderte Vorschaubilder bleiben bei Updates erhalten.

Der optimierte Vergleich lädt unveränderliche Referenzstände ausschließlich benannter
Gruppen und verwendet unveränderte Gruppen erneut. Eine laufende Suche blockiert
während Vergleich und Streaming keine Gesichtsänderungen oder weitere Suchen;
Warten auf einen Cache-Aufbau ist abbrechbar. Bei ausstehender Referenzvorbereitung
liest das Modal nur die aktuelle Auswahl der benötigten benannten Gruppen. Der
globale Neuaufbau bleibt beim vorhandenen Hintergrundlauf und wird nicht durch die
Lupe ausgelöst. Nach dem ersten zulässigen Treffer werden weitere Zwischenstände
höchstens alle 100 ms ausgegeben; das Endergebnis folgt immer. Der Browser stellt
pro Bildschirmaktualisierung nur den neuesten wartenden Stand dar. Änderungen an
Quelle, Referenzen oder Einstellungen werden aktuell geprüft und können einen
erneuten Aufruf erfordern. Alle infrage kommenden benannten Gruppen und Favoriten
werden weiterhin verglichen: **512 Gesicht-IDs sind nur ein Datenbankpaket**, keine
Begrenzung des Suchbestands. Den ausführlichen Vergleich beschreibt der
[Performance-Bericht](tests-und-audit.md#performance-des-lupen-gesichtabgleichs).

Die Dreipunkt-Menüs verwenden vertikal mittig ausgerichtete SVG-Punkte. Im Anzeigemenü stehen die Checkboxen direkt neben ihren Beschriftungen; die Größenauswahl bleibt kompakt. Bei ähnlichen Gruppen stehen Zusammenführen und gegebenenfalls der Stift in der ersten Zeile, Getrennt lassen durchgehend in der zweiten.

**Anzeigeeinstellungen der Personenübersicht (ab 0.47.0):** Das **…-Menü**
unter `/photos/people` steuert **Ordnername anzeigen**, **Fotoanzahl anzeigen** und
**Thumbnailgröße S, M oder L**. S entspricht der bisherigen Größe (bis 160 Pixel),
M verwendet bis 224 Pixel und L bis 320 Pixel; kleine Bildschirme begrenzen die
Breite automatisch. Standard: Fotoanzahl sichtbar, Ordner ausgeblendet, Größe S.
Der Ordner steht unter der Fotoanzahl und gehört zum jeweiligen Vorschaubild;
andere Fotos der Person können aus anderen Ordnern stammen. Für das Stammverzeichnis
steht „Fotos“. Die Einstellungen bleiben pro Benutzer im Browser gespeichert und
gelten auch nach AJAX-Aktualisierungen. Das Umschalten verwendet die vorhandenen
Bilder und benötigt keine Serverabfrage. Ohne JavaScript gilt die Standardansicht.

**Benenn-Modal (ab 0.47.0):** Links erscheint das Foto mit einem Ausschnitt von
300 % der Bounding Box des gewählten Vorschaubilds und der Gesichtsmarkierung,
rechts stehen Name und Vorschläge. Auf schmalen Bildschirmen steht das Bild darüber.
Bei Mehrfachauswahl wird das erste ausgewählte Gruppenportrait gezeigt. Im
Gruppenbildmodus entspricht die Vorschau genau dem angeklickten Gesicht. Das Foto
wird erst beim Öffnen geladen; ein Bildfehler blockiert das Benennen nicht. Die
Personenliste liefert Ordner und Bounding Box mit derselben Seitenabfrage, ohne
zusätzliche Einzelabfragen pro Person. Rechte und aktuelle Sichtbarkeitsprüfungen
bleiben auch beim Laden der Fotovorschau wirksam.

Ab **0.47.1** steht im Benenn-Modal **„Ignorieren“** links neben **„Abbrechen“**.
Die Aktion ignoriert das angezeigte Gesicht; bei mehreren ausgewählten unbenannten
Gruppen werden deren angezeigte Vorschaubilder gemeinsam ignoriert. Andere Gesichter
der Gruppen bleiben aktiv. Bereits benannte Gruppen deaktivieren diese Aktion.
Ein eingegebener Name wird dabei nicht gespeichert. Nach Erfolg schließt sich das
Modal und die Ansicht aktualisiert sich. Schreibfehler bleiben im Modal sichtbar;
schlägt nur das Nachladen nach erfolgreichem Speichern fehl, bleibt erneutes
Speichern gesperrt. Im Gruppenbildmodus gilt die vorhandene Revisionsprüfung;
bei einem Konflikt wird das Foto aktualisiert und die Auswahl muss nach Schließen
des Modals erneut geprüft werden.

**Gruppenbilder bearbeiten:** Der Button **Gruppenbilder** unter `/photos/people`
öffnet `/photos/people/groups` für Fotobearbeiter (`photos.edit`). Die Schwelle ist
im standardmäßig eingeklappten Bereich **Optionen** einstellbar (0–255, Standard **5**)
und wird pro Nutzer und Browser gemerkt. Dort steht auch **Nur Unbenannte anzeigen**.
Es erscheinen ausschließlich Fotos mit **mehr als** dieser Anzahl unbenannter,
nicht ignorierter Gesichter. Benannte und ignorierte Gesichter zählen nicht für die
Auswahl, werden im geöffneten Foto aber weiterhin als Vorschauen gezeigt.

Das **Fragezeichen-Icon** neben **Alle Personen** öffnet die Hilfe zu Auswahl,
Vergrößerung und Gesichtsaktionen. **Verbleibende ignorieren** und **Überspringen**
stehen direkt oberhalb und unterhalb der Gesichtskarten. Beide Aktionsleisten
verwenden denselben Fotostand und dieselbe Sperre während eines laufenden Abrufs;
ein Wechsel der Leiste löst keine doppelte Speicherung aus. Optionen, Hilfe,
Überspringen und Ignorieren funktionieren auch ohne JavaScript.

Beim Ignorieren eines einzelnen Gesichts oder aller verbleibenden Gesichter endet
jeder Speicher- und Nachladeabruf nach spätestens **20 Sekunden**, einschließlich
des Lesens der Antwortdaten. Eine nicht bestätigte Speicherung sperrt weitere
Änderungen an der alten Ansicht, bis **Ansicht erneut laden** den aktuellen Stand
abgerufen hat. Dieser Button wiederholt nur den Leseabruf, niemals die Schreibaktion.
Dasselbe gilt, wenn die Speicherung bestätigt wurde, aber das anschließende Nachladen
scheitert, oder nach einem Revisionskonflikt. Fotovergrößerung bleibt bedienbar;
nach Ende des Abrufs sind auch Filter, Überspringen und die Bilderleiste wieder
nutzbar. Es gibt keine automatischen Wiederholungsschleifen.

Ab **0.50.0** bleibt unten eine **horizontal scrollbare Bilderleiste** sichtbar. Mit Wischen, Mausrad oder den Pfeiltasten der Leiste lassen sich frühere und spätere Gruppenbilder durchsuchen. Ein Klick auf eine Vorschau öffnet genau dieses Foto und setzt den Durchlauf dort fort, ohne Gesichter zu verändern. Das aktuelle Foto ist markiert; **Aktuelles Foto** zentriert die Leiste wieder darauf. Kleine Vorschauen werden erst in Sichtnähe geladen. Die Liste nutzt dieselbe Schwelle und Pfadsortierung wie der Durchlauf, lädt in beide Richtungen nach und hält höchstens 96 Einträge im Browser. Bereits bearbeitete Bilder verschwinden beim Nachladen aus der Auswahl; das gerade geöffnete Foto bleibt auch unterhalb der Schwelle erreichbar. Ladefehler der Leiste lassen sich getrennt wiederholen.

Fehlende kleine Vorschauen werden aus passenden größeren Cache-Bildern erzeugt. Dabei gibt die Thumbnail-Suche ihre Datenbankverbindung vor der weiteren Cache-Prüfung frei, damit paralleles Nachladen weder andere Fotoabrufe noch das Ignorieren von Gesichtern durch einen Datenbank-Deadlock blockiert.

Ab **0.46.0** blendet **„Nur Unbenannte anzeigen“** im Gesichtsraster benannte
und ignorierte Vorschauen aus. Der Filter wirkt sofort ohne zusätzlichen
Serverabruf und merkt sich die Auswahl pro Benutzer im Browser. Nach Benennen,
Zuordnen oder Ignorieren wird das Raster automatisch angepasst. Wird eine
vergrößerte Vorschau ausgeblendet, erscheint wieder das ganze Foto. Ohne passende
Gesichter erscheint ein Hinweis; das aktuelle Foto bleibt geöffnet. Ausschalten
zeigt wieder alle Vorschauen. Der Anzeigefilter benötigt JavaScript und ändert
weder die Auswahl der Gruppenfotos noch die Aktion „Verbleibende ignorieren“.

Links steht eine proportional eingepasste Fotovorschau, rechts das Gesichtsraster.
Ab **0.43.1** zeigt das Raster größere Gesichtsvorschauen mit bis zu **200 × 200 Pixeln**.
Die Spaltenzahl passt sich der verfügbaren Breite an; auf schmalen Bildschirmen
werden die Kacheln untereinander angeordnet. Vorhandene Vorschaubilder und Cache
werden weiterverwendet.
Hover oder Tastaturfokus einer Vorschau markiert die zugehörige Region im Foto.
Ab **0.45.1** vergrößert ein Klick oder Antippen den Ausschnitt auf **300 % der
Bounding Box**: Ein Bereich mit dreifacher Breite und Höhe der Gesichtsmarkierung
wird proportional in die Fotoansicht eingepasst. Am Bildrand wird der Ausschnitt
ins Foto verschoben; große Gesichtsregionen verkleinern das Foto nicht weiter.
Ein weiterer Klick auf dieselbe Vorschau zeigt wieder das vollständige Foto,
ein Klick auf eine andere Vorschau wechselt direkt zu deren Gesicht. Mit der
Tastatur funktioniert der Wechsel über **Enter** oder **Leertaste**. Die aktive
Vorschau ist hervorgehoben. Beim nächsten Foto wird der Zoom zurückgesetzt;
Benennen im selben Foto erhält ihn. Die Vergrößerung nutzt die bereits geladene
Fotovorschau ohne zusätzliche Bildabrufe. Die Markierung hat eine dünne
1-Pixel-Linie mit dunkler Kontrastkontur und berücksichtigt Bildränder,
Seitenverhältnis und die EXIF-Ausrichtung, auch beim Vergrößern und
Ändern der Fenstergröße. Auf kleinen Bildschirmen stehen Foto und Raster untereinander.
Ab **0.43.1** umrandet eine zusätzliche Markierung das Thumbnail, dessen Stift-Dialog
gerade geöffnet ist. Sie bleibt auch bei Speicherfehlern sichtbar und verschwindet
beim Schließen des Dialogs. Bei mehreren Gesichtern derselben Person wird nur die
tatsächlich angeklickte Vorschau markiert. Die Zoomauswahl bleibt dabei erhalten.

Der bekannte Stift-Dialog benennt die **gesamte Personengruppe** oder führt sie mit
einer vorhandenen Person zusammen, einschließlich anderer Fotos. Benannte oder
ignorierte Gesichter zeigen keinen Stift. Das geöffnete Foto bleibt auch unterhalb
der Schwelle sichtbar, damit die übrigen Gesichter weiter bearbeitet werden können.

Ab **0.45.0** hat jede unbenannte, aktive Gesichtsvorschau ein kleines **×**
(**Dieses Gesicht ignorieren**). Es ignoriert ausschließlich diese Erkennung im
aktuellen Foto. Andere Gesichter derselben Person bleiben erhalten, auch im selben
Foto. Die Vorschau bleibt als „Ignoriert“ sichtbar; Zähler und Aktionen aktualisieren
sich ohne Seitenwechsel, die Zoomauswahl bleibt bestehen. Die Aktion funktioniert
auch ohne JavaScript und kehrt dann zum selben Foto zurück. Sie verwendet dieselbe
Revisions- und Rechteprüfung wie die Sammelaktion. Schlägt nur das Aktualisieren nach
dem Speichern fehl, lädt „Ansicht erneut laden“ die Daten ohne erneute Schreibaktion.

**Verbleibende ignorieren** ignoriert atomar ausschließlich die noch unbenannten,
aktiven Gesichter des angezeigten Fotos und wechselt danach zum nächsten passenden
Foto. Bereits benannte Gesichter und dieselben Personen auf anderen Fotos bleiben
erhalten. Ein zwischenzeitlich geändertes Foto oder eine neue Zuordnung führt zu
`409`; die Ansicht wird aktualisiert, bevor die Auswahl erneut bestätigt werden
kann. Schlägt nur das Laden des nächsten Fotos fehl, wiederholt „Ansicht erneut
laden“ den Bildwechsel, ohne erneut zu ignorieren.

**Überspringen / nächstes Foto** lässt alle Gesichtsdaten unverändert und setzt den
aktuellen Durchlauf fort. **Durchlauf starten** beginnt wieder am Anfang und zeigt
auch zuvor übersprungene, weiterhin passende Fotos. Der Pfad-Cursor steht in der URL;
zum nächsten Foto wird ohne Seitenreload gewechselt. Ignorieren und Überspringen
funktionieren auch ohne JavaScript, der Modal-Dialog und die Markierungen benötigen
JavaScript. Private, fehlende oder ersetzte Quellen werden ausgeschlossen.

Die Auswahl liest einen kleinen Index über aktive Gesichter in Pfadreihenfolge;
sie benötigt weder Offset-Paginierung noch einen vollständigen Dateisystemscan.
Eine Fotoansicht enthält höchstens 256 Gesichter, deren vorhandene Vorschau-Caches
weiterverwendet und bei Bedarf erst beim Scrollen geladen werden. Für das große
Foto wird die Galerie-Vorschau mit maximal 1.600 Pixeln verwendet; ohne passenden
Vorschaugenerator steht das Original als Fallback zur Verfügung. Foto und unveränderte
Gesichtsbilder werden beim Benennen nicht erneut geladen. Die automatische Migration
auf Foto-Schema **23** ergänzt lediglich den Auswahlindex und analysiert keine Bilder neu.
Die JSON-Ansicht sowie der Ignorieren- und Bild-Endpunkt sind in OpenAPI dokumentiert.

**Bessere Gesichtszuordnung (ab 0.48.0):** Der Abgleich bewertet sämtliche
zulässigen Referenzvektoren exakt. Anschließend werden die besten unterschiedlichen
Personen mit aktuellen Sichtbarkeits- und Zuordnungsprüfungen ausgewählt. Viele
ähnliche Referenzen oder bereits im Foto zugeordnete Personen verdrängen damit
keine passenden Vergleichsgruppen. Die Standardgrenzwerte für neue Fotos
liegen bei 0,55 Ähnlichkeit und 0,08 Abstand zur zweitbesten Person im jeweiligen Bereich.

Ab **0.50.0** werden neu erkannte Gesichter zuerst ausschließlich mit **benannten
Gruppen** abgeglichen. Erfüllt dort keine Gruppe beide Grenzwerte, folgt ein eigener
Abgleich mit **unbenannten Gruppen**. Erst wenn auch dort kein eindeutiger Treffer
vorliegt, entsteht eine neue unbenannte Gruppe. Der Mindestabstand gilt dabei nur
zwischen unterschiedlichen Gruppen innerhalb desselben Bereichs; benannte und
unbenannte Gruppen konkurrieren nicht miteinander. Pro Gruppe zählt das ähnlichste
gültige Referenzgesicht, einschließlich Favoriten. Weitere Treffer derselben Gruppe
verringern den Abstand nicht. Die benannten Gruppen werden einmal pro Foto über
ihren vorhandenen Datenbankindex ermittelt; bei erfolgreicher benannter Zuordnung
entfällt der Vektorvergleich mit unbenannten Gruppen.

Unter **Einstellungen → Gesichtserkennung → Vorhandene Zuordnungen verbessern**
arbeitet ein separat pausierbarer Hintergrundlauf mit gespeicherten Vektoren,
auch ohne laufenden Erkennungsdienst. Er ist standardmäßig aktiviert und wird nach
Benennen, manuellen Korrekturen, Favoriten-/Referenzänderungen und erfolgreichen
Analysepaketen vorgemerkt. **Zuordnungen erneut prüfen** startet eine neue Prüfung;
**Abgleich pausieren/fortsetzen** erhält deren Fortschritt. Cursor und Ergebnisse
werden gemeinsam gespeichert; Abbruch und Neustart verlieren keine abgeschlossenen
Pakete. Geprüft werden höchstens 100 Datensätze je Paket mit zusätzlichem Zeitbudget.

Automatisch verschoben werden nur unbenannte, unbestätigte, nicht ignorierte und
nicht favorisierte Gesichter zu ausdrücklich benannten Personen. Dafür gelten
standardmäßig strengere Grenzwerte von 0,62 und 0,10 Abstand. Manuelle Trennungen bleiben erhalten;
eine bereits im selben Foto vorhandene Zielperson ist ausgeschlossen. Schlechte
Aufnahmen mit gemessener unzureichender Qualität treiben keinen automatischen
Nachabgleich an. Die Zähler zeigen geprüfte Datensätze, neue Zuordnungen und im
aktuellen Lauf erzeugte Vorschläge.

Ab **0.56.0** bietet der Expertenbereich unter **Automatische Zuordnung im Hintergrund**
die standardmäßig ausgeschaltete Checkbox **Unbenannte Gruppen automatisch zusammenführen**.
Aktiviert führt ein eindeutiger Treffer die **gesamte unbenannte Quellgruppe** atomar zusammen.
Zuerst werden bestätigte benannte Personen geprüft; nur ohne passenden Treffer folgen
unbenannte Gruppen. Für beide Schritte gelten **Hintergrund-Ähnlichkeit und -Mindestabstand**
(Standard 0,62 / 0,10), jeweils gegenüber anderen Gruppen desselben Bereichs.
Der Gruppenwert ist der beste Vergleich zwischen den gespeicherten Referenzgesichtern
beider Gruppen. Ein benannter Treffer hat Vorrang, auch wenn eine unbenannte Gruppe ähnlicher ist.

Eine Quellgruppe mit manuellen Zuordnungen, Favoriten, ignorierten oder gezeichneten
Gesichtern, ungeeigneten/alten Gesichtsmerkmalen, geschützten Mitgliedern oder
widersprechenden XMP-Namen wird nicht automatisch als Ganzes verschoben.
Manuell getrennte Gruppen, abgelehnte Paare und Gruppen mit aktiven Gesichtern im selben
Foto werden nicht automatisch verbunden. Tags und Familienbeziehungen bleiben erhalten;
Familienkonflikte erfordern manuelle Prüfung. Ablehnungen gegenüber weiteren Gruppen
werden auf die verbleibende Gruppe übertragen. Automatisch verschobene Gesichter bleiben
unbestätigt. Ohne die Option bleibt der bisherige Einzelgesichtsabgleich erhalten.
Die automatische Migration auf **Foto-Schema 34** ergänzt nur die ausgeschaltete Option.
Umschalten verwirft offene Vorschläge und plant einen neuen Abgleich; ein pausierter Lauf
bleibt pausiert. Gespeicherte Referenzen, begrenzte Arbeitspakete und indizierte
Gruppenabfragen werden wiederverwendet; es erfolgt keine erneute Bildanalyse.

**Expertenbereich (ab 0.50.0):** Unter **Einstellungen → Gesichtserkennung**
lassen sich im ausklappbaren Expertenbereich drei getrennte Wertepaare einstellen:

| Abgleich | Ähnlichkeit (Standard) | Mindestabstand (Standard) |
| --- | --- | --- |
| Automatische Zuordnung bei der Erkennung | 0,55 | 0,08 |
| Automatische Zuordnung im Hintergrund | 0,62 | 0,10 |
| Vorschläge zur manuellen Prüfung | 0,45 | 0,00 |

Für jede Ähnlichkeit sind **0,4 bis 0,7**, für jeden Mindestabstand **0,0 bis 0,2**
zulässig. Höhere Werte filtern strenger; der Abstand bezeichnet die Differenz zum
besten Treffer einer anderen Gruppe, niemals zu weiteren Gesichtern derselben Gruppe.
Bei der Erstzuordnung werden benannte und unbenannte Gruppen getrennt bewertet.
Manuelle Vorschläge umfassen **Ähnliche Gesichter**
und die Gesichtssuche nach benannten Personen. Bei Abstand 0 dürfen mehrere
Alternativen erscheinen; ein positiver Abstand lässt nur einen eindeutig besten
Treffer zu. In der Gesichtssuche wird dann die vollständige begrenzte Rangliste
abgewartet, bevor Treffer ausgegeben werden. Die Werte sind keine Wahrscheinlichkeiten
und werden durch menschliche Bewertungen nicht automatisch angepasst.

Speichern geänderter Werte entfernt offene Gruppenvorschläge und plant einen neuen,
paketweisen Abgleich der gespeicherten Gesichtsvektoren. Bei pausiertem Hintergrundlauf
entstehen neue Gruppenvorschläge erst nach dem Fortsetzen. Abgelehnte Paare und manuelle
Zuordnungen bleiben erhalten; bereits erfolgte automatische Zuordnungen werden nicht
zurückgesetzt. Die Regeln zum Schutz bestätigter Zuordnungen gelten weiterhin.
Bestehende Installationen erhalten durch die automatische Foto-Schema-Migration **30**
die bisherigen Standardwerte; eine erneute Bildanalyse ist nicht nötig.

**Gruppierungsstatistik der Erkennung:** Im normalen Status stehen die Zahlen
**Bestehender Gruppe zugeordnet**, **Neue Gruppe gebildet** und der
**Zuordnungsanteil**. Gezählt wird die bei der Bildanalyse gespeicherte Entscheidung
für die aktuell vorhandenen, nicht ignorierten Gesichter, einschließlich
XMP-Zuordnungen. Der Anteil ist `zugeordnet / (zugeordnet + neu) × 100`, bei leerer
Basis 0 %. Spätere manuelle Änderungen und der Hintergrundabgleich ändern die
Einteilung nicht; eine damals neue Gruppe kann inzwischen gewachsen sein.
Gelöschte und ignorierte Gesichter entfallen aus den Zahlen. Bei erneuter Analyse
wird die Entscheidung neu erfasst; übernommene Korrekturen behalten ihre bisherige
Einteilung. Foto-Schema 28 ergänzt das Feld automatisch. Altbestände bleiben
**Noch nicht erfasst** und zählen nicht zum Prozentwert: Die ursprüngliche
Entscheidung lässt sich aus heutigen Gruppengrößen nicht zuverlässig ableiten.
Die Statusabfrage nutzt einen schmalen Index und den vorhandenen gemeinsamen
Fünf-Sekunden-Cache, ohne Bilder oder Verzeichnisse zusätzlich zu lesen.

Ab **BearStack 0.50.0 / Android-App 0.10.0** erhalten ausschließlich **unbenannte Gruppen** auf jeder Vergleichsseite eigene Buttons **Ignorieren** und **Benennen/Zuordnen** (Stift). Jede Aktion betrifft alle aktiven Gesichter dieser einen Gruppe, nicht nur das angezeigte Portrait. Benannte Gruppen haben keine Einzelseitenbuttons. Nach einer bestätigten Einzelaktion bleibt das Paar sichtbar; eine weitere unbenannte Seite lässt sich noch bearbeiten. Unten ersetzt dann ausschließlich **Weiter** (App) beziehungsweise **Ausblenden** (Web) die gemeinsamen Entscheidungsbuttons samt gemeinsamem Stift. Abbrechen im Namensdialog verändert nichts.

**Weiter** lädt das nächste Paar und überspringt das gerade bearbeitete Paar in beiden Richtungen. **Ausblenden** entfernt es aus der aktuellen Webansicht. Beides speichert keine Trennung und verbietet keinen späteren automatischen Abgleich. Einzelaktionen verwenden die bestehenden revisionsgeprüften Aktionen `ignore`, `name` und `assign`. Bei verlorener Antwort bleibt das Paar bis zur Klärung der Aktionsquittung gesperrt; anschließend kann die andere Seite bearbeitet werden. Die App zeigt die zusätzlichen Buttons nur bei Servern mit `merge_side_actions`. Die Vorschauen und der bestehende Cache werden weiterverwendet; es erfolgt keine neue Gesichtserkennung.

Sind beide Gruppen unbenannt, erscheint in App und WebUI der **Stift – Zusammenführen und benennen/zuordnen**. Er öffnet die Namenssuche: einen neuen Namen speichern oder eine vorhandene Person auswählen, um beide Gruppen in einem Schritt zusammenzuführen und zu benennen beziehungsweise zuzuordnen. **Abbrechen** verändert nichts. Veränderte Gruppen oder Zielpersonen müssen erneut geprüft werden. Die App zeigt den Stift nur bei Servern mit `merge_naming` (ab BearStack 0.50.0); nach bestätigtem Speichern folgt das nächste Paar.

Sind **beide Gruppen bereits benannt**, warnt **Zusammenführen** in App und WebUI mit beiden Namen und verlangt eine zusätzliche Bestätigung – auch bei gleichen Namen. Alle Gesichter der ersten Gruppe wechseln zur zweiten; deren Name bleibt erhalten. **Abbrechen** lässt beide Gruppen und den Vorschlag unverändert. **Getrennt lassen** und Zusammenführungen mit mindestens einer unbenannten Gruppe benötigen diese Zusatzbestätigung nicht. Ohne JavaScript verlangt die WebUI vor dem Zusammenführen eine ausdrückliche Bestätigungs-Checkbox. Veränderte Gruppen müssen weiterhin erneut geprüft und bestätigt werden. Die Warnung verwendet die bereits geladenen Namen und verursacht keine zusätzlichen Serverabfragen oder Gesichtsvergleiche.

Bei **Ähnliche Gesichter** (Web) beziehungsweise **Ähnliche Gruppen** (Android-App) zeigen beide Oberflächen den Ähnlichkeitswert des aktuellen Gruppenpaars klein und mittig über den Entscheidungsbuttons, auf zwei Nachkommastellen gerundet. Der Wert beschreibt die Ähnlichkeit der angezeigten Vergleichsgesichter, keine Wahrscheinlichkeit. Nach einer Entscheidung erscheint der Wert des nächsten Paars. Ältere Server ohne `score` liefern in der App keine Wertanzeige.

**Ähnliche Gesichter** unter `/photos/people/merge-suggestions` zeigt Fotobearbeitern
bis zu 60 gespeicherte Zusammenführungsvorschläge. Auch ähnliche unbenannte
Teilgruppen werden berücksichtigt. Vorschläge ab der konfigurierten Schwelle (Standard 0,45) sind keine
Wahrscheinlichkeitsangaben und benötigen eine Prüfung. **Zusammenführen** verwendet
die angezeigten Gruppenrevisionen; zwischenzeitliche Änderungen verlangen eine
neue Prüfung. **Getrennt lassen** bleibt als Entscheidung gespeichert und verhindert
auch künftige automatische Zuordnungen zwischen diesen Gruppen. Beide Aktionen
aktualisieren mit JavaScript die Vorschläge ohne Seitenneuladen; unveränderte
Karten bleiben erhalten. Während einer Entscheidung sind nur die Buttons des
betroffenen Vorschlags gesperrt; andere Gruppenpaare lassen sich weiter bearbeiten.
Parallele Entscheidungen werden vor dem Nachladen gesammelt, überholte Antworten
verworfen. Fehler werden direkt angezeigt. Bei unbestätigten Entscheidungen bleibt
nur der betroffene Vorschlag bis zur erfolgreichen Aktualisierung gesperrt;
veraltete Vorschläge müssen erneut geprüft werden. Ohne JavaScript bleiben die normalen Formulare
nutzbar. Aufrufe der Seite
lösen keine neue Vektorsuche aus. Bearbeiten benötigt `photos.edit`, Einstellungen
und Steuerung benötigen `photos.manage`.

Ab **0.61.2** zeigt `/photos/people/merge-suggestions` zu jedem Vergleichsbild den
Ordnernamen (im Hauptordner „Fotos“). Ein Klick auf das Bild öffnet die vorhandene
Lightbox mit dem vollständigen Foto; nur der Personenname öffnet die Personengruppe.
Das gilt auch nach dem Nachladen und Bearbeiten von Vorschlägen. Die Bildpfade
kommen aus der bestehenden begrenzten Abfrage; zusätzliche Einzelabfragen entfallen.

Ab **0.61.3** bleiben die Vergleichsgesichter mit Ordnernamen auch im Dialog
**Zusammenführen und benennen/zuordnen** sichtbar. Beim Benennen einer einzelnen
Gruppe erscheint nur deren Gesicht. Die Vorschauen im Dialog übernehmen keine
Aktionsbuttons oder Personenlinks aus den Vorschlagskarten.

Ab Android-App **0.9.0** und BearStack **0.49.0** ist **Ähnliche Gruppen** auch über das App-Menü verfügbar: immer eine Entscheidung mit zwei Portraits und festen Buttons, ohne scrollbare Vorschlagsliste. Nach **Zusammenführen** oder **Getrennt lassen** lädt automatisch das nächste Gruppenpaar. Beide Portraits bieten dieselbe Originalfoto-Vergrößerung per Halten und Wischen wie beim Benennen/Zuordnen. Aktionsquittungen verhindern doppelte Änderungen bei verlorenen Antworten; Konflikte verlangen eine erneute Prüfung. Die Android-Portraits stehen gleich groß und auf derselben Höhe; Namen, Gesichtsanzahlen und Einzelaktionen sind kompakt darunter angeordnet. Bei wenig Platz oder großer Schrift scrollt der Vergleichsbereich bei weiterhin sichtbaren Entscheidungsbuttons. Details stehen in der [Android-Anleitung](android.md#ahnliche-gruppen).

**Kleine Gesichter:** Nach der ersten Analyse des maximal 1.600 Pixel großen Fotos
werden bei Bedarf bis zu acht Originalausschnitte nachanalysiert. Das Original wird
dafür einmal zusätzlich dekodiert; Ausschnitte sind auf 1.600 Pixel und zusammen
16 MiB begrenzt. Nur eindeutig wiedergefundene Gesichter mit ausreichend höherer
Auflösung übernehmen den verbesserten Vektor. Die ursprünglichen Markierungen
bleiben stabil. Ausfälle der optionalen Nachanalyse behalten das erste Ergebnis;
Abbruch, Quelländerungen und Schutzmarkierungen stoppen die Verarbeitung.

Der aktualisierte Gesichtsdienst liefert zusätzlich Gesichtsauflösung und Schärfe.
Neue automatisch ausgewählte Referenzen benötigen mindestens 48 Gesichtspixel und
Schärfewert 20; diese technischen Mindestwerte sind keine allgemeine Genauigkeits-
garantie. Explizite Favoriten dürfen die Qualitätsauswahl überschreiben, niemals
Sichtbarkeitsregeln. Ältere kompatible Dienste und gespeicherte Vektoren ohne
Qualitätswerte bleiben verwendbar. Für gemessene Qualitätsfilter den Gesichtsdienst
mit aktualisieren. Modelle und Protokoll 1 bleiben kompatibel. Foto-Schema 25
migriert automatisch, ohne bestehende Namen oder Gesichter zu löschen.

**Referenzen pro Person:** Unter **Einstellungen → Gesichtserkennung** lässt sich
die Zielanzahl auf 1–100 einstellen; der Standard ist **30**. Seit 0.40.0 werden
Vergleichsbilder möglichst über verschiedene Galerieordner verteilt. Innerhalb
eines Ordners haben manuelle Zuordnungen Vorrang, danach die Erkennungssicherheit.
Die Auswahl ist bei unveränderten Daten stabil.

**Favorisierte Gesichter:** In der Web-Detailansicht einer benannten oder unbenannten
Person können Fotobearbeiter einzelne Gesichter mit einem Stern favorisieren.
Alle aktiven Favoriten werden bei jedem Abgleich vollständig verglichen, auch wenn
ihre Anzahl das eingestellte Limit übersteigt. Bei weniger Favoriten füllen
unfavorisierte Gesichter bis zur Zielanzahl auf, sofern vorhanden. Dabei haben
bisher nicht vertretene Ordner Vorrang; Favoriten zählen bereits für ihren Ordner.
Mehr Favoriten benötigen mehr Arbeitsspeicher und Rechenzeit pro neu erkanntem Gesicht.
Die Aktion speichert sofort, ohne Seitenreload oder erneute Bildanalyse.
Favoriten bleiben nach Neustart, Zuordnen und Zusammenführen erhalten.
Ignorierte und private Gesichter werden weiterhin ausgeschlossen. Wiederhergestellte
Gesichter behalten ihren Stern; bei erneuter Erkennung wird er nur auf eine eindeutig
wiedergefundene Region übertragen. Gelöschte oder ersetzte Fotos verlieren ihre
veralteten Gesichtsdaten einschließlich der Favoriten.

Änderungen der Zielanzahl und das Update bestehender Referenzen werden vor der
nächsten Analyse in kurzen, fortsetzbaren Schritten übernommen. Die Referenzauswahl selbst führt keine Gruppen zusammen. Der separate
Hintergrundabgleich prüft unbestätigte Zuordnungen und erzeugt Vorschläge. Die Android-Oberfläche erhält
vorerst keine Sterne. Für spätere Clients stehen `GET` und `PUT` unter
`/api/photos/labeling/v1/faces/{id}/favorite` bereit (`photos.edit`). Der PUT-Body
enthält `person_id` und den gewünschten booleschen Wert `favorite`; Wiederholungen
schalten den Zustand nicht erneut um. Eine inzwischen geänderte Gruppenzuordnung
oder ein ignoriertes Gesicht führt zu `409`. Personendetails liefern `faces[].favorite`,
die Sitzung meldet Unterstützung über `face_favorites: true`. Alte Server ohne dieses
Feld unterstützen die Erweiterung nicht. Details und Fehlerantworten stehen in der
[OpenAPI-Beschreibung](https://github.com/ringelbaer/BearStack-DMS/blob/main/openapi.yaml).

**Metadaten und Korrekturen:** Eindeutige XMP-Gesichtsregionen liefern Namen und
Referenzen. Manuelle Zuordnungen haben Vorrang. XMP und automatische Gesichter werden
getrennt gespeichert; Originale und Sidecars werden nicht verändert. Die Foto-DB
migriert automatisch auf Schema 24 (ab 0.43.0 zusätzlich ein partieller Index für benannte Personen). Ihre Sicherung muss die erzeugten Gesichtsdaten
sowie manuelle Korrekturen und Favoriten einschließen. Ein Index-Neuaufbau erhält die Korrekturen
unveränderter Bilder; Dateiaustausch und Löschung entfernen veraltete Analysen.
Bei einem Modellwechsel werden manuelle Zuordnungen nur auf eindeutig wiedergefundene
Regionen übertragen. Unsichere Treffer bleiben getrennt. Das Zusammenführen bestehender
Gruppen erfolgt ausschließlich manuell; die Erkennung stellt keine sichere Identitätsfeststellung dar.

**Sichtbarkeit und Aufbewahrung:** `.adminonly`-Fotos sind von automatischer Analyse
und Personengruppen ausgeschlossen. Nachträgliche Schutzmarkierungen entfernen deren
automatische Ergebnisse und Referenzen. Gesichtsvorschauen prüfen die aktuellen Rechte.
Pausieren oder Ausschalten behält bisherige Ergebnisse. Die separate, ausdrücklich
zu bestätigende Löschaktion entfernt erzeugte Gesichtsdaten, Gruppen und Korrekturen
und schaltet die Verarbeitung aus; importierte XMP-Gesichtsdaten bleiben erhalten.
Merkmalsvektoren erscheinen weder in HTTP-Antworten an Browser noch in Logs.

V1 unterstützt JPEG, PNG, WebP und das erste GIF-Bild. Videos und SVG werden nicht
analysiert. Bilder über 40 Megapixel werden zur Begrenzung des Speichers zurückgewiesen;
sehr kleine, verdeckte oder durch die Verkleinerung zu kleine Gesichter können fehlen.

### Gesichtsketten gemeinsam prüfen

Ab **0.57.0** führt **Ähnliche Gesichter → Gesichtsketten prüfen** zur neuen
Webansicht unter `/photos/people/chains`. Die Funktion benötigt **Fotos bearbeiten**.
Sie sucht ausschließlich beim Aufruf dieser Ansicht, beim Blättern zur nächsten
Kette oder beim Neustart des Durchlaufs; im Hintergrund werden keine Ketten berechnet.
Ein laufender Erkennungsdienst ist dafür nicht erforderlich.

**Maximale Sprünge** begrenzt die Entfernung von der Ausgangsgruppe auf **1 bis 5**
(Standard **2**). A → F → Y sind zwei Sprünge. Verzweigungen gehören dazu: Passt
ein Gesicht von A zu B und ein anderes zu F, erscheinen B und F bereits bei einem
Sprung; eine Verbindung von F zu Y ergänzt Y beim zweiten Sprung. Die kürzeste
Entfernung zur Ausgangsgruppe steht an jeder Gesichtskarte.

Ab **0.62.0** bietet `/photos/people/chains` neben **Maximale Sprünge** das Feld
**Mindestähnlichkeit** (0 bis 1, Schritte von 0,01). Höhere Werte verlangen
ähnlichere Gesichter für jede direkte Verbindung; der Wert ist keine Wahrscheinlichkeit.
Vorgabe ist die eingestellte Mindestähnlichkeit manueller Vorschläge (standardmäßig 0,45).
Mit **Durchlauf neu starten** werden der neue Wert übernommen und übersprungene
Gruppen wieder berücksichtigt. Die globale Einstellung bleibt unverändert.
Der Wert gilt auch für weitere Ketten, Wiederholungen und die Fortsetzung nach einer
unbestätigten Speicherung. Beim normalen erneuten Öffnen gilt wieder die globale Vorgabe.

Eine direkte Verbindung benötigt mindestens ein geeignetes gespeichertes
Gesichtspaar mit der gewählten Mindestähnlichkeit. Auch geeignete Gesichter außerhalb
des Referenzlimits werden verglichen. **Der Mindestabstand wird hier nicht angewendet**:
Mehrere mögliche Treffer sind ausdrücklich Teil der manuellen Kettenprüfung.
Benannte Gruppen sind weder Quellen noch Zwischenstationen. Ignorierte,
geschützte, gezeichnete und ungeeignete Gesichter erzeugen keine Verbindungen;
Vergleichssterne behalten ihre Referenz-Eignung. Abgelehnte Gruppenpaare und Gruppen,
die aktiv im selben Foto vorkommen, bilden keine direkte Verbindung. Ein indirekter
Weg über andere Gruppen kann sie dennoch in dieselbe Prüfauswahl bringen.

Die Ansicht zeigt **alle aktiven Gesichter** der erreichten Gruppen einschließlich
manuell zugeordneter und gezeichneter Gesichter, jeweils mit Ordnernamen. Es gibt
**60 Gesichter pro Seite**. Zunächst ist die **gesamte Kette über alle Seiten**
angekreuzt; Abwahlen bleiben beim Blättern erhalten. Die Auswahlzahl unten umfasst
auch unsichtbare Seiten. **Seite auswählen/abwählen** betrifft nur die sichtbare Seite.
**Auswahl zuordnen** öffnet den bekannten Namensdialog für eine vorhandene benannte
Person oder einen neuen Namen. Ein leerer Name wird nicht gespeichert.

Die Speicherung ordnet alle ausgewählten Gesichter atomar und manuell bestätigt
zu; nicht ausgewählte, ignorierte und bereits benannte Gesichter bleiben unverändert.
Gruppen-Tags und Familienbeziehungen werden bei dieser Gesichtsauswahl nicht
zusammengeführt. Nach erfolgreichem Speichern folgt die nächste Kette.
**Kette überspringen** ändert keine Daten. Übersprungene und bearbeitete Gruppen
werden nur für diesen Durchlauf ausgelassen; **Durchlauf neu starten** bezieht
sie wieder ein. Änderungen an Gruppen, Zielen oder Fotos erfordern eine erneute
Prüfung. Bei einer verlorenen Speicherantwort bleibt die Auswahl gesperrt;
**Speicherung prüfen** klärt den Vorgang über eine Aktionsquittung, auch nach einem
Neuladen im selben Browser-Tab.

Die Suche verarbeitet Vergleichsvektoren in kleinen Blöcken und hält keinen
bibliotheksweiten Kettengraphen im Speicher. Pro Bibliothek läuft höchstens eine
Kettensuche gleichzeitig. Eine Anfrage hat 30 Sekunden Zeit; **Suche anhalten**
bricht die laufende Suche ab. Bei mehr als **1.000 verbundenen Gruppen** erscheint
eine Aufforderung, eine höhere Mindestähnlichkeit oder weniger Sprünge zu wählen; die Kette wird niemals still gekürzt.
Pro Auswahl können höchstens **10.000 Gesichter** abgewählt werden, pro Durchlauf
höchstens **10.000 Gruppen** ausgelassen werden. Für die insgesamt zugeordneten
Gesichter gilt nicht das übliche 500er-Limit einzelner Stapelaktionen.
Die Funktion benötigt keine neue Datenbankmigration.


## Android-Personen-App

Ab BearStack 0.30.0 lassen sich unbenannte Gruppen auch mit der [nativen Android-App](android.md) bearbeiten. Ab App 0.6.0 und BearStack 0.43.0 bietet „Menü → Personen“ zusätzlich alle benannten Personen mit Portraits, Umbenennen, einzelne Zuordnungen zurücksetzen, Favoriten, Galeriesuche im Browser und Originalfoto-Vorschau per Halten und Wischen. Die Anleitung beschreibt HTTPS-Anmeldung, Gesten, Statistik und den privaten APK-Build.

Ab Android-App 0.7.0 und BearStack 0.45.0 durchsucht das Textfeld im Personenbereich alle benannten Personen. Personenliste und Portrait-Raster laden beim Scrollen automatisch nach. Das Entfernen einer Zuordnung verlangt zuvor eine Bestätigung mit Name und Bildpfad.

### Lokale Fotoordner in der Android-App

Die optionale Einstellung **Lokale Fotoordner anzeigen** ergänzt **Ordner → Dieses Gerät** um die freigegebenen Fotoordner des Smartphones, inklusive Vorschauen und lokalem Vollbildbetrachter. Standardmäßig deaktiviert, ohne Upload; Fotos und Suche bleiben auf den Serverbestand bezogen. Freigabe, ausgewählte Fotos ab Android 14 und Speichergrenzen beschreibt die [Android-Anleitung](android.md#lokale-fotoordner).

## Extern umbenannte Fotoordner

Ab **BearStack 0.65.0** behält die Bibliothek dauerhafte interne Identitäten für
Medien, Ordner, Markdown-Blogs und GPX-Dateien. BearStack schreibt weiterhin
**nicht in den Foto-Root**: Umbenennen, Verschieben oder Kopieren und späteres
Löschen erfolgt außerhalb der Anwendung. Der nächste vollständige Indexlauf
ordnet eindeutige Umzüge innerhalb desselben Roots zu.

- Foto-, Ordner- und Blogtags, Personen- und Gesichts-IDs sowie manuelle
  Entscheidungen bleiben bei einer Zuordnung erhalten. Existierende
  Gesichtsvorschauen und unveränderte Foto-/Video-Thumbnails werden wiederverwendet.
- Fehlende Inhalte verschwinden aus den normalen Ansichten und bleiben **sieben
  Tage** in BearStacks Datenbank aufbewahrt. Wiederholte Scans und Neustarts
  verlängern diese Frist nicht. Ein fehlgeschlagener oder unvollständiger Scan
  bestätigt keine Löschung; ein ausgefallener oder verdächtig leerer Root löst
  keine endgültige Bereinigung aus.
- Unter **Einstellungen → Fotos → Ordner und Aufbewahrung** erscheinen erfasste
  Umzüge, fehlende Einträge mit Ablaufdatum und der Fingerabdruckfortschritt.
  Eine fehlende Ordneridentität lässt sich einem vorhandenen Ziel zuordnen.
  Unabhängige manuelle Änderungen am Ziel verhindern eine automatische Übernahme.
  Die endgültige Löschung entfernt nur BearStacks aufbewahrte Daten und Caches.
- Unveränderte Vergleichsdateien müssen an denselben relativen Pfaden liegen.
  Mindestens zwei unterschiedliche, nichtleere SHA-256-Fingerabdrücke oder die
  eindeutige Übereinstimmung zweier Ein-Datei-Ordner erlauben eine automatische
  Zuordnung. Identische Kopien bleiben eigenständig, solange beide existieren.
  Leere Ordner, fehlende historische Fingerabdrücke und mehrdeutige Kopien
  können eine manuelle Zuordnung benötigen.
- Zusätzliche und gelöschte Dateien verhindern die Zuordnung nicht, wenn genug
  eindeutige Vergleichsdateien verbleiben. Markdown/GPX und XMP werden unabhängig
  aktualisiert. Ein Wechsel des Dateityps gilt als neuer Eintrag.

Ändern sich die Bytes eines Fotos am bisherigen relativen Pfad, bleiben die
Gesichtsdaten und vorhandenen Gesichtsvorschauen als letzter bestätigter Stand
bestehen. **Foto geändert / Prüfen** kennzeichnet diese Regionen im Web und ab
**Android 0.17.0** in den Gesichtslisten. Alte Embeddings werden auch bei
Favoriten nicht mehr als Erkennungsreferenzen verwendet. Unter **Geänderte Fotos
prüfen** zeigt das Web die bisherige Vorschau neben dem aktuellen Bild; Rahmen
und Person lassen sich bestätigen oder korrigieren. Die Prüfung erfordert
`photos.edit`, die Ordnerverwaltung `photos.manage`. Ohne verfügbaren
Gesichtsdienst kann eine Region bestätigt werden; der historische Vektor wird
dadurch nicht zur Erkennungsreferenz. Veraltete Prüfentscheidungen werden mit
HTTP 409 abgewiesen.

Die SQLite-Migration übernimmt vorhandene IDs und Cachedateien. SHA-256-Erfassung
und Cacheinventar werden anschließend fortsetzbar aufgebaut. Unveränderte
Folgeläufe lesen keine Dateiinhalte; Galerieaufrufe berechnen keine vollständigen
Hashes. Für noch nicht erfasste Altbestände ist keine automatische Wiedererkennung
zugesichert. Bestehende pfadbasierte URLs bleiben unterstützt; alte Pfade werden
nach einem Umzug nicht automatisch weitergeleitet. Aktuelle Zugriffsrechte gelten
auch für gespeicherte Vorschauen und Erkennungsreferenzen.

Ab **0.66.0** folgen auch vorübergehend aufbewahrte und durch Ordnerschutz
verborgene Gesichter einer manuellen oder automatischen Personen-Zusammenführung.
Nach Rückkehr des Fotos bleibt die Zuordnung zur überlebenden Person erhalten;
IDs, Embeddings, Favoriten und gespeicherte Vorschauen werden dabei bewahrt.
Ordnerumzüge aktualisieren die betroffenen Teilbäume und Vorfahren, ohne fremde
Scanstände oder Vorschauzuordnungen zu löschen.

Foto-Schema **37** synchronisiert aufbewahrte Spalten zentral nach jeder
versionierten Migration. Kopien verwenden explizite Spaltennamen; zusätzliche
Spalten übernehmen ihre definierten Standardwerte. Umbenennungen, Entfernungen
oder Typwechsel benötigen eine explizite Datenmigration. Der zuvor ungenutzte
Identitätszustand entfällt; Fristen und Einzelrevisionen bleiben an den Einträgen.
