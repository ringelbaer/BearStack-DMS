# BearStack Fotos für Android

Native App für Android 8.0 oder neuer, App-Version **0.10.0**. Die Galerie benötigt
**BearStack 0.50.0**, ein aktiviertes Fotomodul und `photos.read`. Personenverwaltung
benötigt zusätzlich `photos.edit`; die bisherigen Abläufe und lokalen Daten bleiben
beim Update erhalten. Auf älteren Servern bleibt der bisherige Personenbereich verfügbar.

## Gesichtssuche beim Benennen

Ab App **0.10.0** startet die **Lupe neben dem Namensfeld** im Benenn-Dialog einen Gesichtsabgleich mit bereits benannten Personen. Sie verwendet das erste Gesicht der angezeigten Seite beziehungsweise das Quellgesicht im Modus „Ähnliche Gruppen“. Die Treffer erscheinen schon während des Abgleichs in Ähnlichkeitsreihenfolge mit dem passenden Referenzportrait. Weitere Zwischenstände aktualisieren die Liste; der Ladehinweis bleibt bis zum Abschluss sichtbar. Treffer lassen sich bereits während der Suche auswählen. Antippen ordnet die aktuelle Gruppe zu; beim gemeinsamen Benennen zweier Gruppen werden beide zugeordnet. Die Suche selbst verändert nichts. Ladezustand, leere Trefferliste und Fehler werden angezeigt; erneutes Antippen wiederholt die Suche. Tippen im Namensfeld, Schließen des Dialogs oder Wechsel in den Hintergrund bricht sie ab. Das separate Umbenennen einer bereits benannten Person bleibt eine Namensänderung.

Der Abgleich nutzt den vorhandenen Endpunkt `GET /photos/faces/{id}/suggestions` (BearStack ab 0.50.0), benötigt keine erneute Bilderkennung und liefert höchstens 20 Treffer. Die App fordert NDJSON-Streaming an, verarbeitet höchstens 64 KiB pro Zwischenstand und hält nur die aktuelle Trefferliste sowie den neuesten noch nicht angezeigten Zwischenstand im Speicher. Ein fehlender Abschluss oder ein Übertragungsfehler leert die vorläufigen Treffer und erlaubt einen erneuten Versuch. Ältere Server mit diesem Endpunkt können weiterhin eine einzelne JSON-Antwort liefern. Bei einem positiven Mindestabstand zwischen erstem und zweitem Treffer wartet auch der Server auf den vollständigen Abgleich. Die App lädt nur bei explizitem Start und lädt die aktuelle Revision ausschließlich der ausgewählten Zielperson nach. Eine inzwischen umbenannte Zielperson erfordert eine neue Entscheidung; Zuordnungen behalten die vorhandenen Revisionsprüfungen und Aktionsquittungen. Ältere Server zeigen eine verständliche Fehlermeldung; die Namenssuche bleibt verfügbar.

## Gruppen manuell kombinieren

**Menü → Gruppen manuell kombinieren** öffnet ein fortlaufendes Gruppenraster. Standardmäßig erscheinen nur unbenannte Gruppen; **Auch benannte Gruppen anzeigen** nimmt benannte Gruppen hinzu. Kurz auf ein Portrait tippen wählt die Gruppe aus oder ab. Langes Drücken öffnet wie gewohnt das Originalfoto mit Gesichtsmarkierung und Vergrößerung durch Wischen. Weitere Gruppen laden automatisch beim Scrollen, auch beim Zurückscrollen.

Ab zwei ausgewählten Gruppen erscheinen **Kombinieren** und **Kombinieren und benennen**. Kombinieren behält den ersten ausgewählten vorhandenen Namen; ohne benannte Auswahl bleibt die Gruppe unbenannt. Der erhaltene Name steht über den Aktionen. Kombinieren und benennen fragt vor dem Speichern nach einem Namen. Abbrechen verändert nichts. Bei einem bereits anderweitig vergebenen Namen kann eine separate gleichnamige Gruppe ausdrücklich bestätigt werden. Ein Filterwechsel oder „Aktualisieren“ leert die Auswahl. Pro Aktion sind höchstens 60 Gruppen wählbar.

Die Funktion benötigt die neue Server-Capability `manual_merge` innerhalb BearStack **0.50.0**; App und Server müssen dafür aktualisiert sein. Der Server prüft die Revision aller ausgewählten Gruppen und führt die komplette Auswahl samt Aktionsquittung atomar zusammen. Eine veraltete Auswahl muss neu getroffen werden; eine verlorene Antwort kann über „Offene Aktion prüfen“ aufgelöst werden. Ignorierte Gesichter bleiben ignoriert, Favoriten und Originaldateien erhalten. Die App lädt Pakete von höchstens 20 Gruppen, hält normalerweise drei Seiten um den sichtbaren Bereich und separat die begrenzte Auswahl. Kleine ID-Marken erlauben das Nachladen früherer Seiten. Nach dem Kombinieren bleibt die Scrollposition erhalten; die betroffenen Seiten werden neu geladen.

## Galerie

**Fotos** zeigt den gesamten Fotobestand nach Datum. Unter **Ordner** öffnest du die
bekannte Verzeichnisstruktur; jede Kachel enthält zwei Vorschauen und die Medienanzahl.
**Suchen** nutzt die Browsersyntax, zum Beispiel `person:Anna` oder `tag:Urlaub`.
Die Suche wird mit der Suchen-Taste der Tastatur gestartet.

Die Ordnersuche hat keine Gesamtgrenze von 50 Treffern. Auch größere Ergebnisse
werden vollständig und sortiert in Paketen von höchstens 24 Ordnern nachgeladen;
Vorschaubilder werden erst für das jeweils angefragte Paket ermittelt.

Die Galerie ist ein fortlaufendes Raster: Einfach weiterscrollen. Weitere Fotos,
Ordner und Texte laden bereits vor dem sichtbaren Ende automatisch nach. Auch
beim Zurückscrollen bleiben alle Inhalte erreichbar. Es gibt keine Seitenwechsel
oder „Mehr laden“-Schaltflächen; bei einem Ladefehler erscheint **Erneut versuchen**.

Ein Foto öffnet den Vollbildbetrachter. Wische zum nächsten Foto oder nutze die
Vor-/Zurück-Schaltflächen. Mit zwei Fingern oder **Vergrößern** lässt sich das Bild
vergrößern. **Informationen** lädt Dateiname, Pfad, Auflösung, Dateigröße sowie
vorhandene Kamera- und GPS-Daten, Bewertung, Personen, Tags und Schlagwörter.
Die Aufnahmezeit erscheint mit Sekunden und gespeicherter Zeitzone; ohne
Aufnahmezeit wird die Änderungszeit ausdrücklich gekennzeichnet. Ladefehler
lassen sich direkt im Informationsblatt über **Erneut versuchen** wiederholen. Markdown- und Textbeiträge stehen im jeweiligen
Ordner unter **Geschichten & Notizen** und werden erst beim Öffnen vollständig geladen.
Ladefehler erscheinen mit **Erneut versuchen** direkt in der Textansicht; leere
Beiträge werden als leer angezeigt und beenden den Ladezustand.

**Weitere Optionen → Personen verwalten** führt zu den bisherigen Funktionen.
Zurück aus dem Benennen-Bereich oder **Menü → Fotos** öffnet die Galerie.
Galerie, Personenverwaltung, Hilfetexte und Fehlermeldungen stehen vollständig auf
Deutsch und Englisch zur Verfügung. Die App folgt der Android-Sprache und dem
System-Hell-/Dunkelmodus. Ab Android 13 lässt sich die Sprache auch einzeln unter
**Android-Einstellungen → Apps → BearStack Fotos → Sprache** wählen; ältere Geräte
verwenden die Systemsprache. Bereits sichtbare Fehler werden bei einem Sprachwechsel
neu übersetzt, ohne die aktuelle Gruppe oder eine offene Aktion zu verlieren.

Fehlermeldungen verwenden feste Meldungsschlüssel statt ungefilterter technischer
Ausnahmetexte. Kamera-, Datei- und Personenbezeichnungen bleiben die Originaldaten.
Die kompakte, abgerundete Navigation verbindet Fotos, Ordner und Suche. Das Raster
zeigt auf breiten Displays sechs statt drei Fotos pro Reihe. Bei großer Schrift
erhalten Ordner im Hochformat die gesamte Breite für ihre Beschriftung. Das
adaptive App-Symbol bleibt auch beim runden Android-Beschnitt vollständig sichtbar.

### Einzelne Bilder teilen

**Teilen** oben im Vollbildbetrachter öffnet die Android-App-Auswahl mit der
Originaldatei des aktuell angezeigten Fotos, auch aus Karten und lokalen Fotoordnern.
Die Diashow pausiert beim Antippen. Serverbilder werden erst dann über die bestehende
HTTPS-Verbindung geladen. Ein Ladehinweis bietet **Abbrechen**; nach einem Fehler
kannst du den Button erneut verwenden. Schließen, Bildwechsel und Wechsel in den
Hintergrund brechen die Vorbereitung ab.

Die ausgewählte App erhält vorübergehenden Lesezugriff auf genau dieses Bild,
keine BearStack-Zugangsdaten oder Serverlinks. Originaldateien enthalten weiterhin
ihre gespeicherten Metadaten, gegebenenfalls einschließlich GPS. Lokale Fotos werden
direkt über ihre MediaStore-URI geteilt, ohne Kopie, Upload oder zusätzliche Berechtigung.

Serveroriginale werden ohne Bilddekodierung mit dem vorhandenen 64-KiB-Übertragungspuffer
in einem privaten, ausschließlich für das Teilen freigegebenen Cache vorbereitet.
Pro Bild sind bis zu 256 MiB möglich; größere Originale lassen sich über **Herunterladen**
speichern. Fehler und Abbruch entfernen die angefangene Datei. Erfolgreich vorbereitete
Dateien bleiben für die Empfänger-App verfügbar; vor dem nächsten Server-Share werden
Dateien ab 24 Stunden sowie ältere Dateien oberhalb der Grenzen bereinigt. Einschließlich
der laufenden Vorbereitung bleiben höchstens acht Dateien und 512 MiB im Cache.
Es werden weder weitere Fotos vorgeladen noch alle Galerieeinträge durchsucht.

### Lokale Fotoordner

Unter **Weitere Optionen → Einstellungen → Lokale Fotoordner anzeigen** lässt sich
zusätzlich **Ordner → Dieses Gerät** einschalten. Die Funktion ist standardmäßig aus
und die Einstellung bleibt auf dem Gerät gespeichert. Unter dem Hauptordner stehen
die von Android erkannten Fotoordner, beispielsweise Camera, Screenshots und Pictures,
mit Medienanzahl und bis zu zwei Vorschauen. Ein Ordner öffnet seine Fotos im
fortlaufenden Raster; Vollbild, Zoom, Diashow und lokale Dateiinformationen sind verfügbar.
Zurück führt über das Gerät wieder zur Servergalerie. Die Tabs Fotos und Suchen
zeigen weiterhin den Serverbestand.

Beim ersten Aktivieren fragt Android nach Fotozugriff. Ab Android 14 kannst du auch
nur einzelne Fotos freigeben; dann erscheinen ausschließlich diese Fotos und ihre
Ordner. **Fotozugriff erlauben / Auswahl ändern** in den App-Einstellungen ermöglicht
eine neue Auswahl. Bei dauerhaft abgelehntem Zugriff führt **Android-App-Einstellungen**
zur Freigabe. Deaktivieren blendet das Gerät aus; vorhandene Android-Berechtigungen
können separat dort entzogen werden.

Die App liest ausschließlich den lokalen Android-Medienindex (MediaStore). Es gibt
keinen Upload und keinen Serveraufruf für lokale Fotos. Videos, private App-Dateien
und nicht im Medienindex enthaltene Ordner gehören nicht zu dieser Ansicht.
SD-Karten-Fotoordner erscheinen, soweit Android sie bereitstellt; gleiche Ordnernamen
auf unterschiedlichen Datenträgern bleiben getrennt. Lokale Bilder werden nur im
Arbeitsspeicher mit maximal 16 MiB Bildcache gehalten, ohne Disk-Cache. Beim Verlassen
der lokalen Ansicht oder Wechsel in den Hintergrund werden Anfragen abgebrochen
und der lokale Bildcache geleert. Nach der Rückkehr werden Fotozugriff und Medienindex
neu geprüft; Änderungen am Medienindex aktualisieren die offene Ansicht mit kurzer
Verzögerung und schließen gegebenenfalls den Betrachter.

Die Ordnerübersicht liest einmal pro geöffnetem beziehungsweise aktualisiertem
Katalog nur IDs und Ordner-Metadaten, hält je Ordner einen Zähler und höchstens zwei
Vorschau-IDs und lädt Ordner in Paketen von 24 nach. Fotos laden in Paketen von 96;
das Raster hält höchstens drei Seiten. Die Abfragen laufen abbrechbar außerhalb
des UI-Threads. Android-Anbieter ohne native Seitengrenzen verwenden einen Cursor
zum angefragten Ausschnitt, weiterhin ohne die komplette Fotoliste in den App-Speicher
zu übernehmen. Bilddekodierung erfolgt erst für sichtbare Vorschauen beziehungsweise
den Betrachter und ist auf dessen Darstellungsgröße begrenzt.

Die Berechtigungen folgen [Androids Regeln für vollständigen und ausgewählten Fotozugriff](https://developer.android.com/about/versions/14/changes/partial-photo-video-access).
Die Server-API und die benötigte BearStack-Version bleiben unverändert. Diese
MINOR-Erweiterung gehört zur noch unveröffentlichten App 0.10.0 / BearStack 0.50.0;
App-VERSION, versionCode und Root-VERSION behalten deshalb ihren Stand.

### Karten

**Weitere Optionen → Karte** zeigt die GPS-Aufnahmen des aktuellen Ordners
inklusive Unterordnern oder der aktuellen Suche. Verschieben und Zwei-Finger-Zoom
ändern den Ausschnitt; Plus/Minus und **Alle Orte anzeigen** sind ebenfalls verfügbar.
Zahlen bündeln benachbarte Aufnahmen. Antippen vergrößert die Umgebung; ein einzelner
Punkt öffnet die Aufnahme. Mehrere Aufnahmen am exakt gleichen Ort oder bei
maximalem Zoom öffnen eine ebenfalls endlos scrollbare Fotoauswahl.
Das Raster lädt automatisch nach und öffnet Fotos im gemeinsamen Vollbildbetrachter.
In **Informationen** erscheint eine Karte zum Foto.

**Ebenen → Fotoroute** verbindet die GPS-Orte der Fotos in zeitlicher Reihenfolge
als gestrichelte Linie. Die aktive Suche und Typauswahl bleiben erhalten. Die
serverseitige Foto-Track-Auflösung bestimmt die Gruppierung naher Orte. Die App
lädt diese Ebene erst beim Einschalten und hält höchstens 4.096 Routenkoordinaten.
Beim Zoomen wird die Geometrie für den Ausschnitt erneut geladen. Die Gruppierung
verwendet immer den vollständigen passenden Bestand, bevor die Route zugeschnitten
wird. Lange Routen können vereinfacht sein; Fehler sind direkt wiederholbar.

**Ebenen → GPX-Tracks** öffnet die Trackauswahl für den Ordner samt Unterordnern.
Bei einer Fotosuche stehen die Tracks der gesamten Sammlung zur Auswahl. Die Liste
lädt beim Scrollen automatisch in beide Richtungen nach. Du kannst mehrere Tracks
zugleich anzeigen; farbige Markierungen und die ausgewählten Namen helfen beim
Vergleichen. Ein ausgewählter Track wird auf der Karte fokussiert. Das funktioniert
auch ohne GPS-Fotos. Erneutes Antippen oder **Alle GPX-Tracks ausblenden** entfernt die
Auswahl. Unlesbare Dateien werden in der Trackauswahl gemeldet und lassen sich erneut laden.

Getrennte GPX-Abschnitte bleiben getrennt, auch an der Datumsgrenze. Lange Tracks
werden für den sichtbaren Ausschnitt vereinfacht; beim Vergrößern lädt die App mehr
Details. Bis zu 256 Tracks teilen sich ein Gesamtlimit von 8.192 dargestellten
Punkten. Zwei GPX-Geometrie-Anfragen laufen gleichzeitig; beim Verschieben, Verlassen
der Karte oder Ändern der Auswahl werden überholte Anfragen abgebrochen. Die
Trackliste hält höchstens 96 Dateieinträge zusätzlich zu den ausgewählten Namen.

Die lesenden Endpunkte `/api/photos/v1/map/tracks` und `/api/photos/v1/map/track`
verwenden denselben GPX-Parser und den auf 32 MiB begrenzten Cache wie der Browser.
GPX-Dateien dürfen höchstens 16 MiB und 100.000 Track-/Routenpunkte enthalten.
Symbolische Links und als privat markierte Ordner werden ausgeschlossen. Schema 26
legt einen Dateimetadatenindex an; vorhandene Installationen ergänzen ihn beim
nächsten normalen Indexlauf. Bis dahin weist die App auf die noch unvollständige
Trackliste hin. Vorhandene Fotokarten bleiben währenddessen verfügbar.
Der lesende Endpunkt `/api/photos/v1/map/route` verwendet dieselbe Gruppierung wie
der Browser und einen chronologischen Index mit normalisierten Zeitzonen. Die
Serverantwort enthält Gesamtzahlen und höchstens 8.192 Koordinaten; die App fordert
4.096 an. GPX und Fotorouten teilen sich serverseitig zwei Geometrie-Arbeitsplätze.
Große Volltextsuchen können ihre Sortierung in temporäre Dateien auslagern.
Komplexe Suchausdrücke melden ein zu breites Ergebnis ausdrücklich.

Die vollständig gruppierte Route wird **auf dem Server** berechnet und als JSON
unter `<Cache-Verzeichnis>/photo-routes/v1/` von Browserkarte und App gemeinsam
wiederverwendet. Die App lädt nur
zugeschnittene und vereinfachte Geometrie. Ordner samt Unterordnern, Medientyp,
Gruppierungsradius und Sichtbarkeit bestimmen den Cache-Schlüssel. Suchabfragen
werden berechnet, aber zunächst nicht dauerhaft gespeichert.

Eine transaktionale Indexrevision erfasst Einfügen, Löschen sowie Änderungen an
GPS, Aufnahme-/Änderungszeit, Ordnerzuordnung, Medientyp und Sichtbarkeit. Änderungen
in Unterordnern invalidieren dadurch auch übergeordnete Routen. Foto-Schema 27 trennt
diese Revisionen nach Ordner, Medientyp und Sichtbarkeit: unbeteiligte Routen bleiben
warm; Verschieben und Löschen invalidieren weiterhin alle betroffenen Auswahlen.
Ordnerzeitstempel
sind keine Grundlage der Invalidierung. Vor jedem Abruf bleiben die aktuellen
Zugriffsprüfungen aktiv; während der Berechnung geänderte Revisionen werden verworfen.

Gleichzeitige Cache-Abrufe derselben Route teilen die nötige Neuberechnung. Erst wenn niemand mehr
wartet, wird sie abgebrochen. Die vollständige JSON-Datei ersetzt die alte atomar;
Format und Gruppierungsalgorithmus haben getrennte Versionsnummern. Unterschiedliche
Zoomstufen und Punktlimits verwenden dieselbe Datei. Beschädigte Dateien werden
neu erzeugt; bei nicht beschreibbarem Cache bleibt die direkte Berechnung möglich.
Der Routencache hält höchstens 256 JSON-Dateien mit insgesamt 512 MiB. Für atomare
Neuberechnungen kommen vorübergehend temporäre Dateien hinzu. Größere Routen
bleiben vollständig berechenbar, werden aber nicht dauerhaft gespeichert. Dateien
sind nur für den Serverbenutzer lesbar; HTTP-Antworten bleiben `private, no-store`.

Kartenmarker, Fotoauswahl und Fotoroute prüfen die aktuellen Zugriffsregeln der
betroffenen GPS-Ordner. Neu private Unterordner bleiben auch vor dem nächsten
Indexlauf verborgen. Die Prüfung liest keine Originalbilder und arbeitet mit
höchstens 256 Ordnern samt gemeinsam geprüften Vorfahren pro Paket.


Die Karten-API bündelt alle passenden indexierten Aufnahmen in höchstens 289
Markern pro Ausschnitt. Sie lädt keine vollständigen Fotolisten in den App-Speicher.
Ist der Fotoindex noch nicht bereit, bietet die Ansicht erneutes Laden an.
OpenStreetMap liefert nur die gerade sichtbaren Kartenbilder. Ein eigener Client
ohne BearStack-Zugangsdaten nutzt einen auf 64 MiB begrenzten HTTP-Cache und beachtet
Cache-Header sowie bedingte Anfragen. Die Quellenangabe bleibt auf der Karte sichtbar.


### Original speichern

Im Vollbild öffnet **Original herunterladen** Androids Speicherdialog. Wähle den
Zielordner und Dateinamen. Die Originaldatei wird stückweise übertragen, ohne sie
vollständig in den Arbeitsspeicher zu laden. Die Fortschrittsanzeige erlaubt
Abbrechen; bei Fehler oder Abbruch wird versucht, die neu angelegte unvollständige
Datei zu entfernen. Es wird keine allgemeine Speicherberechtigung verlangt.

### Diashow und Fotoframe

Im Vollbild startet die Wiedergabetaste die Diashow. **Diashow einstellen** bietet
Anzeigedauern von 3 bis 300 Sekunden und eine Wiederholung am Ende. Die Zeit läuft
erst nach dem Laden des Fotos; Informationen, Einstellungen, Zoom und der Wechsel
in eine andere App pausieren die automatische Wiedergabe.

**Weitere Optionen → Fotoframe starten** spielt die Medien des aktuellen Ordners
mit Unterordnern ab und berücksichtigt die aktive Suche bzw. Typauswahl. Im Frame blendet Antippen die Steuerung ein; Name/Datum und
Bildschirmfüllung mit Beschnitt sind einstellbar. Zurück stellt die vorherige
Galerie wieder her. Während der Wiedergabe bleibt der Bildschirm aktiv.
Videos und Audio verwenden im Vollbild und im Frame den nativen Media3-Player
und dieselbe HTTPS-Verbindung; nur die sichtbare Seite hält einen Decoder bereit.
Die automatische Wiedergabe wartet bis zum Medienende. Auch eine einzelne Datei
wird bei aktivierter Wiederholung erneut abgespielt; ohne Wiederholung stoppt
die Wiedergabe am Ende. Informationen und Einstellungen pausieren auch manuell
gestartete Medien. Die Einstellungen bleiben bei großer Schrift scrollbar.

Die API `/api/photos/v1/` verwendet dieselben Foto-Dienste, Suchregeln und
Zugriffskontrollen wie der Browser. Seiten enthalten bis zu 96 Medien, 24 Ordner
mit jeweils zwei Vorschauen und 20 Textzusammenfassungen. Fotos und Texte werden
bei Bedarf geladen; ein Ansichts- oder Kontowechsel bricht veraltete Anfragen ab.
Die Übertragung erfolgt intern in begrenzten Datenpaketen, ohne sichtbare Seitengrenzen.
Eine Galerieansicht hält pro Bereich höchstens 288 Medien,
72 Ordner mit bis zu zwei Vorschauen und 60 Textzusammenfassungen. Für die Rückkehr
aus dem Fotoframe bleibt zusätzlich die vorherige Galerie mit denselben Grenzen
erhalten. Entfernte Metadaten werden beim Zurückscrollen automatisch erneut geladen. Das sichtbare Element behält seine
Position; Vollbild und Diashow wechseln auch über Seitengrenzen zum richtigen Foto.
Ladefehler lassen sich am betroffenen Bereich wiederholen. Nach dem Schließen des
Vollbilds zeigt das Raster das zuletzt betrachtete Foto; der Wechsel zur
Personenverwaltung und zurück erhält die Galerieposition.

Vollbild und Frame laden über WLAN höchstens die nächste Aufnahme vor; Wechsel
der Ansicht oder Verlust der WLAN-Verbindung brechen dieses Vorladen ab. Anzeige
und Vorladen teilen sich den begrenzten Drei-Minuten-Speichercache, mit höchstens
2048 Pixeln für die decodierte Vorschau. Der Bildcache liegt ausschließlich im
begrenzten Arbeitsspeicher. Die App-ID
`de.bearstack.people` bleibt für bestehende Installationen erhalten; der Bär im
adaptiven Icon liegt jetzt innerhalb des runden Beschnitts.

## Bauen und installieren

`apps/android/` in Android Studio als eigenes Projekt öffnen. Voraussetzungen: JDK 17 oder 21 und Android-SDK mit Plattform 36. Das Projekt verwendet den Gradle Wrapper 9.6.0, AGP 9.4.0 und Kotlin 2.3.21. SDK-Pfad über `ANDROID_HOME` oder eine ignorierte `apps/android/local.properties` mit `sdk.dir=…` setzen.

```sh
./scripts/check-android.sh
adb install -r apps/android/app/build/outputs/apk/debug/app-debug.apk
```

Alternativ im Android-Verzeichnis: `./gradlew :app:assembleDebug`. APKs werden lokal gebaut und nicht automatisch veröffentlicht. Go-, Web- und Docker-Builds benötigen kein Android-SDK. `apps/android` ist aus dem Docker-Buildkontext ausgeschlossen.

## Verbindung und Zertifikat

HTTPS-Adresse (gegebenenfalls mit Reverse-Proxy-Pfad), Benutzername und Passwort eingeben. Die App prüft Protokollversion und Fotoleserechte; Personenfunktionen werden nur bei passenden Bearbeitungsrechten angeboten. HTTP und automatische Weiterleitungen sind ausgeschlossen; bei einer Umleitung die endgültige HTTPS-Adresse verwenden.

Öffentlich vertrauenswürdige Zertifikate werden regulär geprüft. Bei einem selbstsignierten Serverzertifikat erscheint **vor Übermittlung der Zugangsdaten** dessen SHA-256-Fingerabdruck. Auf dem Server den Fingerabdruck der tatsächlich verwendeten Zertifikatsdatei ermitteln:

```sh
openssl x509 -in /pfad/zum/server.crt -noout -fingerprint -sha256
```

Alle Hexadezimalpaare vergleichen, anschließend „Abgeglichen und vertrauen“ wählen. Nur dieses Zertifikat wird für das Profil akzeptiert; Hostname und Gültigkeit bleiben verbindlich. Eine IP-Adresse funktioniert nur mit einem entsprechenden IP-Eintrag im Subject Alternative Name des Zertifikats. Bei einem Zertifikatswechsel die Verbindung erneut einrichten und den neuen Fingerabdruck prüfen. Private Zertifikatsketten mit eigener CA werden in Version 1 nicht als selbstsigniertes Blattzertifikat angeboten.

Es gibt ein aktives Profil. Zugangsdaten und bestätigtes Zertifikat werden mit AES-GCM unter einem Schlüssel im Android Keystore in `noBackupFilesDir` gespeichert. Backups und Gerätetransfers sind ausgeschlossen. Screenshots und die Vorschau im App-Umschalter sind gesperrt. Die App schreibt keine Zugangsdaten in Logs. Bilder werden nur im begrenzten Arbeitsspeichercache gehalten; ein Profilwechsel leert den Cache.

## Ähnliche Gruppen

Ab App **0.10.0** und BearStack **0.50.0** steht der Ähnlichkeitswert des aktuellen Paars klein und mittig über den Entscheidungsbuttons. Zwei Nachkommastellen, keine Prozentwahrscheinlichkeit; bei älteren Servern ohne Wert bleibt die Zeile ausgeblendet.

Sind beide Gruppen unbenannt, erscheint in App und WebUI der **Stift – Zusammenführen und benennen/zuordnen**. Er öffnet die Namenssuche: einen neuen Namen speichern oder eine vorhandene Person auswählen, um beide Gruppen in einem Schritt zusammenzuführen und zu benennen beziehungsweise zuzuordnen. **Abbrechen** verändert nichts. Veränderte Gruppen oder Zielpersonen müssen erneut geprüft werden. Die App zeigt den Stift nur bei Servern mit `merge_naming` (ab BearStack 0.50.0); nach bestätigtem Speichern folgt das nächste Paar.

Ab App **0.9.0** und **BearStack 0.49.0** öffnet **Menü → Ähnliche Gruppen** jeweils eine einzelne Entscheidung. Zwei Portraits zeigen die tatsächlichen Vergleichsgesichter der Gruppen mit Namen und Gesichtsanzahl. Es gibt keine scrollbare Vorschlagsliste; die Portraits passen sich dem verfügbaren Platz an und die beiden Entscheidungsbuttons bleiben unten sichtbar.

- **Zusammenführen:** Alle Gesichter der ersten Gruppe werden der zweiten zugeordnet. Favoriten bleiben erhalten. Ist die zweite Gruppe unbenannt, wird ein vorhandener Name der ersten übernommen.
- **Getrennt lassen:** Die Trennung bleibt gespeichert und verhindert auch künftige automatische Zuordnungen zwischen diesen Gruppen.
- Nach Serverbestätigung lädt automatisch das nächste Gruppenpaar. Wenn keine Vorschläge vorliegen, erscheint ein Hinweis mit „Aktualisieren“.
- **Portrait halten und wischen:** Wie beim Benennen/Zuordnen erscheint das vollständige Original mit Gesichtsmarkierung. Herunterwischen vergrößert zum Gesicht, Hochwischen verkleinert; Loslassen oder Abbrechen schließt die Vorschau. Beide Portraits bieten die TalkBack-Aktion „Originalfoto anzeigen“. Der vollständige Galeriepfad steht in der Originalvorschau; lange Pfade lassen sich dort lesen.

Während des Speicherns und bei einer ungeklärten Antwort sind weitere Entscheidungen gesperrt. „Offene Aktion prüfen“ klärt die dauerhaft gespeicherte Aktionsquittung, auch nach einem App-Neustart. Konflikte durch andere Bearbeitungen verlangen eine neue Prüfung. Schlägt erst das Nachladen fehl, wird ausschließlich der nächste Vorschlag neu geladen. Zurück führt zum Benennen; bei unveränderten Gruppen bleibt auch die bisherige Bildseite erhalten. Diese Entscheidungen erhöhen nicht die Statistik für erstmaliges Benennen/Zuordnen.

Der Server liest höchstens 20 gespeicherte Kandidaten und liefert nur ein Paar mit je einem Vergleichsgesicht. Beim Öffnen wird keine Vektorsuche gestartet. Auch Vergleichsgesichter weit hinten in großen Gruppen werden über den Gesichts-ID-Index abgerufen. Die bestehende Originalvorschau mit maximal 2048 Pixeln, Drei-Minuten-/16-MiB-Speichercache und seriellem WLAN-Vorladen wird wiederverwendet. Auf älteren Servern zeigt die App einen Hinweis auf BearStack 0.49.0; Benennen und Personenverwaltung bleiben verfügbar.

## Personen verwalten

Ab App-Version **0.6.0** öffnet **Menü → Personen** die Liste aller benannten Personen mit Portrait und Gesichtsanzahl. Dieser Bereich benötigt **BearStack 0.43.0**; auf älteren Servern bleibt das bisherige Benennen verfügbar. Die Liste lädt jeweils höchstens 20 Personen in stabiler ID-Reihenfolge. Ab App **0.7.0** lädt die Liste beim Scrollen automatisch weitere Personen nach; „Aktualisieren“ übernimmt neu hinzugekommene Personen. Das Suchfeld durchsucht nach 250 ms Eingabepause den gesamten Serverbestand nach Namen, unabhängig von bereits geladenen Einträgen. Groß-/Kleinschreibung und deutsche Umlautschreibweisen werden tolerant behandelt. Ein Suchwechsel beginnt eine neue Trefferliste; verspätete Antworten ersetzen keine neuere Suche. Die Textsuche benötigt **BearStack 0.45.0**; ältere Server zeigen einen entsprechenden Hinweis.

Eine Person antippen, um ihre Portraits im fortlaufenden Raster zu öffnen. Ab App **0.7.1** ist das „×“ zum Entfernen einer Zuordnung im Personenbereich kleiner; die Touchfläche bleibt mindestens 48 dp groß. Ab App 0.7.0 werden beim Scrollen automatisch weitere Bilder ergänzt; Seitenknöpfe entfallen im Personenbereich:

- **Person umbenennen:** Der neue Name gilt für die ganze Person. Existiert er bereits, muss das separate Speichern ausdrücklich bestätigt werden; Personen werden dadurch nicht zusammengeführt.
- **× / Zuordnung entfernen:** Zunächst erscheint eine Bestätigung mit Personenname und Bildpfad. „Abbrechen“ schließt sie ohne Änderung; erst „Entfernen“ sendet den Auftrag. Nur das ausgewählte Gesicht wird in eine neue unbenannte Gruppe verschoben und seine Favorisierung aufgehoben. Die Originaldatei bleibt erhalten. Das funktioniert auch beim letzten Gesicht; die leere Person verschwindet dann aus der Liste. Zurückgesetzte Gesichter werden im Zuordnungsmodus nach der aktuellen Gruppe angeboten.
- **☆ / ★:** Das Gesicht als Vergleichsbild favorisieren oder die Favorisierung aufheben, entsprechend der Favoritenfunktion im Web.
- **Galeriesuche im Browser:** Öffnet die Galerie mit dem Personen-Namensfilter. Der Reverse-Proxy-Pfad bleibt erhalten. Der Browser verwendet seine eigene Anmeldung; App-Zugangsdaten werden nicht im Link übergeben. Gleichnamige Personen erscheinen gemeinsam entsprechend der Galeriesuche.
- **Portrait halten und wischen:** Dieselbe Originalfoto-Vorschau wie im Zuordnungsmodus, mit Gesichtsmarkierung, Herunterwischen zum Vergrößern und Hochwischen zum Verkleinern. Loslassen oder Abbrechen schließt sie. Die TalkBack-Aktion „Originalfoto anzeigen“ bleibt ebenfalls verfügbar.

Umbenennen, Entfernen und Favorisieren werden erst nach Serverbestätigung angezeigt. Offene Schreibaktionen bleiben lokal gespeichert und werden über ihre Aktionsquittung geklärt. Änderungen an Gruppenzuordnung oder Namen durch andere Clients verlangen eine neue Entscheidung. Die aktuelle unbenannte Gruppe und ihre Bildseite bleiben beim Wechsel in den Personenbereich erhalten. Verwaltungsaktionen erhöhen nicht die Zähler für erstmaliges Benennen oder Zuordnen.

Die Liste verwendet ein Lazy-Layout; Portraits werden nur für sichtbare Einträge geladen. Personen-Metadaten und Sichtbarkeitsprüfungen erfolgen serverseitig in kleinen Paketen ohne Gesamtzählung. Das Raster stellt nur sichtbare Kacheln bereit. BearStack 0.45.0 liefert pro Anfrage bis zu 40 Gesichter über einen indexierten Gesichts-ID-Cursor; tiefes Scrollen benötigt keinen zunehmend großen Offset. Alte Server liefern weiterhin vier Gesichter je Anfrage, die App fügt sie ebenfalls fortlaufend an. Vor dem Anfügen werden Revision und Gesichtsanzahl geprüft; bei zwischenzeitlichen Änderungen wird neu geladen und eine Meldung angezeigt. Bestätigte Favoriten-, Namens- und Zuordnungsänderungen aktualisieren den geladenen Bestand anhand der atomaren Aktionsquittung, sodass Bilder und Scrollposition erhalten bleiben. Während eines Dialogs oder einer Originalvorschau wird nicht automatisch nachgeladen; Ladefehler lösen keine Endlosschleife aus. Ab App 0.8.0 werden Originale der angezeigten Portraits bei WLAN nacheinander vorgeladen; über Mobilfunk werden sie erst beim Öffnen der Vorschau geladen. Der vorhandene begrenzte Bildcache wird weiterverwendet. Änderungen aktualisieren den Gesichtssuchindex nur für betroffene Personen, sofern der Index aktuell ist.

Ab App **0.8.3** erscheinen **…**, **?** und **Stift** in der unteren Benennen-Aktionsleiste als gleich große, vertikal mittig ausgerichtete Symbole. Der Stift hat keinen sichtbaren Schaltflächenhintergrund mehr; die unsichtbaren Touchflächen bleiben jeweils 48 dp groß. In „Personen benennen“ beginnt der Inhalt direkt mit dem Gesichtsraster; „Unbenannte Person“ und die Bildanzahlzeile entfallen.

Ab App **0.8.2** hat der Favoritenstern im Personenbereich dieselbe Schriftgröße wie „×“ und keinen sichtbaren Schaltflächenhintergrund; die unsichtbare Touchfläche bleibt 48 dp groß.

Ab App **0.8.1** bleibt unten eine schlanke Aktionsleiste stehen: links **…** für Ignorieren, Überspringen und dessen Rücknahme, mittig **?** für die scrollbar angezeigte Bedienhilfe, rechts der **Stift** zum Benennen. Die Leiste reserviert Platz unter dem Inhalt und berücksichtigt die Systemnavigation. Das obere Menü enthält die App-Navigation.

## Zuordnungsmodus

- Ab App-Version 0.5.2 bleibt der Platz für den Ladebalken dauerhaft reserviert. Gesichtsraster und Navigation behalten beim Ein- und Ausblenden ihre Position, auch in gescrollten Ansichten mit großer Schrift.
- Unter jedem Gesichtsausschnitt und in der Originalfoto-Vorschau steht der vollständige Galeriepfad mit allen Ordnerebenen und Dateiname. Die Aufbereitung entspricht der Galerie, etwa `Fotos / 11.05.2026 · Urlaub / IMG_1234.jpg`. Lange Pfade werden umgebrochen statt abgeschnitten. Die Pfade kommen vom Server; diese Anzeige benötigt BearStack 0.34.0.
- Das Grid zeigt bis zu vier **Gesichtsausschnitte**, keine ganzen Fotos. Bei größeren Gruppen blättern „Zurück“ und „Weiter“ durch Viererseiten. Benennen, Zuordnen und Ignorieren betreffen immer die gesamte Gruppe.
- Wischen funktioniert auf Bildern, Zwischenräumen und dem freien Hintergrund der Bearbeitungsansicht. Links überspringt die Gruppe; rechts holt die zuletzt übersprungene Gruppe zurück; oben ignoriert sie mit Rücknahmefrist. Bei überlangem Inhalt scrollt Hochwischen zunächst zum Ende; ein weiterer Wischer nach oben ignoriert die Gruppe. Menü, Statistik, Namensdialog und Originalfoto-Vorschau lösen keine Wischaktionen aus.
- Rechtswischen nimmt das letzte Überspringen im aktuellen Durchgang zurück, auch mehrfach und nach dem letzten Datensatz. Alternativ „Letztes Überspringen zurücknehmen“ unter … in der unteren Aktionsleiste wählen. Die bisherige Gruppe bleibt mit ihrer Bildseite zur weiteren Bearbeitung vorgemerkt. Zurückgeholte Gruppen werden erneut am Server geprüft; bereits bearbeitete oder entfernte Gruppen werden ausgelassen. Bei Verbindungsfehlern bleiben aktuelle Gruppe und Rücknahmemöglichkeit erhalten. Die zurückgenommene Überspringen-Zählung wird entfernt. Verlauf und vorgemerkte Gruppen bleiben nach einem App-Neustart erhalten; ein neuer Durchgang beginnt ohne Rücknahmeverlauf.
- Einen Gesichtsausschnitt halten, um das vollständige Originalfoto zu sehen, aus dem er stammt. Das Foto wird mit seinem ursprünglichen Seitenverhältnis vollständig eingepasst. Loslassen oder Abbruch schließt die Vorschau. Eine feine Bounding Box markiert das ausgewählte Gesicht. Bei weiter gedrücktem Finger vergrößert Herunterwischen zum Gesicht, Hochwischen verkleinert zurück zum ganzen Foto. Dabei wird weder ignoriert noch übersprungen. Der Zoom bleibt auf maximal 12-fache Vergrößerung begrenzt; das Original wird einmal mit höchstens 2048 Pixeln je Seite dekodiert. Über die TalkBack-Aktion „Originalfoto anzeigen“ bleibt die Vorschau bis „Vorschau schließen“ geöffnet; dort stehen „Zum Gesicht vergrößern“ und „Ganzes Foto anzeigen“ als Aktionen bereit. Die Markierung benötigt die Gesichtskoordinaten aus BearStack 0.35.0; bei älteren Servern bleibt das Original ohne Markierung und Zoom sichtbar.
- Der Stift öffnet das Namensfeld. Nach 250 ms Eingabepause erscheinen höchstens 20 Vorschläge. Einen Vorschlag antippen, um zuzuordnen; „Speichern“ benennt die aktuelle Gruppe. Bei einem bestehenden gleichen Namen wird zwischen Zuordnen und „Separat benennen“ unterschieden.
- Das **×** links unten trennt genau dieses Gesicht in eine neue unbenannte Gruppe ab. Es ignoriert und löscht nichts. Die neue Gruppe folgt nach Abschluss der aktuellen Gruppe; mehrere Abtrennungen folgen ihrer Reihenfolge. Bei einem einzigen Gesicht entfällt das ×.
- **Nach oben wischen:** die ganze Gruppe ignorieren. Die nächste Gruppe erscheint sofort. Eine klickbare Meldung am oberen Bildschirmrand bietet fünf Sekunden lang „Rückgängig“ an; die Ansicht bleibt währenddessen bedienbar. Bei mehreren rasch ignorierten Gruppen hat jede ihre eigene Frist, die Meldung nimmt jeweils die letzte noch rücknehmbare Gruppe zurück. Ab App-Version 0.5.3 wartet das Speichern nach Fristende, solange ein Namens- oder Duplikatdialog geöffnet ist. Die Meldung läuft weiterhin nach fünf Sekunden aus; Änderungen und Ausblenden der Meldung unterbrechen weder Texteingabe noch Fokus oder Tastatur. Nach dem Schließen des Dialogs werden wartende Schreibanfragen nacheinander ausgeführt. Beim Wechsel in den Hintergrund oder nach Prozessende kehren noch nicht gesendete Gruppen in die Warteschlange zurück.
- **Nach links wischen:** lokal für den aktuellen Durchgang überspringen. Über „Übersprungene bearbeiten“ am Ende des Durchgangs werden diese Gruppen erneut angeboten. „Neuen Durchgang starten“ übernimmt inzwischen hinzugekommene Gruppen; übersprungene bleiben der ausdrücklichen Wiederaufnahme vorbehalten.
- Ignorieren und Überspringen sind auch als beschriftete Menüaktionen verfügbar. Die Oberfläche unterstützt System-Hell-/Dunkelmodus und große Schrift; Inhalte und Dialoge sind scrollbar.

Benennen und Zuordnen schalten nach Serverbestätigung weiter. Ignorieren schaltet bereits während der Rücknahmefrist weiter. Bei abgelehntem Ignorieren wird die betroffene Gruppe zur erneuten Prüfung vorgemerkt; die aktuelle Gruppe bleibt erhalten. „Offene Aktion prüfen“ klärt zuerst die serverseitige Aktionsquittung und sendet nur bei fehlender Quittung denselben gespeicherten Auftrag erneut. Währenddessen sind weitere Entscheidungen gesperrt. Bei fehlenden Rechten oder falschen Zugangsdaten kann die Verbindung gewechselt und mit demselben Konto wiederhergestellt werden.

Ändert die Weboberfläche oder Hintergrundverarbeitung inzwischen eine Gruppe, meldet die API `409`; die App lädt den aktuellen Stand und verlangt eine neue Entscheidung. Sie überträgt die alte Entscheidung nicht automatisch auf eine veränderte Gruppe. Eine Änderung an der Zielperson erfordert ebenfalls eine erneute Auswahl.

## Verbindungsfehler eingrenzen

Ab App-Version 0.4.1 erfolgt auch das Schließen von HTTPS-Verbindungen außerhalb des UI-Threads. Dadurch funktionieren Kontowechsel ohne verdeckende Android-Netzwerkfehler; abgelehnte Anmeldungen zeigen die konkrete Ursache auch im Zertifikatsdialog.

Ab App-Version 0.1.1 zeigt die Fehlermeldung die Phase (HTTPS-Verbindungsprüfung, Anmeldung oder Serveranfrage) und einen Diagnosecode. `DNS` steht für fehlgeschlagene Namensauflösung, `CONNECT` für einen fehlgeschlagenen Verbindungsaufbau, `NO_ROUTE` für eine fehlende Route und `TIMEOUT` für eine Zeitüberschreitung. `TLS`, `TLS_IDENTITY`, `CERT_EXPIRED` und `CERT_NOT_YET_VALID` unterscheiden Zertifikatsprobleme. `NETWORK_PERMISSION` weist auf verweigerten Socket-Zugriff hin.

Bei einer LAN-Adresse dieselbe HTTPS-Adresse auf demselben Handy im Browser testen. Funktioniert der Zugriff nur außerhalb der App, die Netzwerkfreigabe und die appbezogenen VPN-Regeln prüfen. Eine Zeitüberschreitung allein beweist weder einen Zertifikats- noch einen Passwortfehler. Bei der ersten HTTPS-Prüfung werden keine Zugangsdaten gesendet; die Meldung über eine offene Aktion erscheint nur, wenn tatsächlich eine solche Aktion gespeichert ist.

Debug-Builds schreiben unter dem Logcat-Tag `BearStackConnection` ausschließlich Phase und Diagnosecode. Exception-Texte, vollständige URLs, Benutzernamen und Passwörter werden nicht protokolliert.

## Warteschlange und Statistik

Room trennt lokalen Zustand nach Instanz, Datenbestand und Konto. Gespeichert werden aktueller Datensatz und Bildseite, Cursor mit fester oberer Gruppen-ID, abgetrennte und übersprungene Gruppen, ungeklärte Aktionen und bestätigte Ereignisse. Rücknahmetimer laufen ausschließlich im Arbeitsspeicher. Die IDs und Bildseiten vorläufig ignorierter Gruppen werden in Room gespeichert, damit sie nach einem Neustart wieder angeboten werden. Ungeklärte, möglicherweise bereits gesendete Aktionen werden zuerst über die Aktionsquittung aufgelöst. Es gibt keine allgemeine Offline-Warteschlange.

Die Statistik zählt **Gesichter und Gruppen**, jeweils heute (lokale Zeitzone) und insgesamt: Benannt, Zugeordnet, Ignoriert, Übersprungen. Serveraktionen zählen nach Bestätigung genau einmal pro Aktions-ID. Überspringen zählt einmal je Gruppe und Durchgang. Abtrennen hat keinen eigenen Statistikzähler. Die Zahlen sind gerätelokal; sie sind kein vollständiges Server-Audit. App-Daten löschen oder Deinstallation entfernt die lokalen Zahlen.

Ein Durchgang lädt Metadaten in Seiten von maximal 20 Gruppen. Jede Gruppe wird vor Anzeige erneut geprüft. Benannte, leere oder gelöschte Gruppen werden ausgelassen. Vier Bilder werden angezeigt, höchstens die nächste Viereransicht wird vorgeladen. Ab App 0.8.0 öffnet die Originalvorschau nach 250 ms Halten und schließt beim Loslassen. „Zurück“ und „Weiter“ erscheinen im Benennen-Modus nur bei mehr als vier Fotos. Originale angezeigter Portraits werden bei aktiver WLAN-Verbindung nacheinander in den vorhandenen Cache vorgeladen, mit derselben begrenzten Dekodiergröße wie die Vorschau. Beim Verlassen der Ansicht, Wechsel auf Mobilfunk oder im App-Hintergrund wird das Vorladen abgebrochen. Der Bildcache ist auf 16 MiB begrenzt und hat keinen Disk-Cache.

Ab App **0.6.1** und BearStack **0.43.1** wird das dekodierte Originalfoto bis zu **drei Minuten ab erfolgreichem Laden** über unterschiedliche Gesichter desselben Fotos hinweg wiederverwendet, auch zwischen Zuordnungsmodus und Personenbereich. Weitere Aufrufe verlängern die Frist nicht. Die individuelle Bounding Box und der Zoom werden separat gezeichnet. Der gemeinsame LRU-Bildcache bleibt auf **16 MiB** begrenzt; bei Speicherdruck können Bilder früher verdrängt werden. Ein Verbindungswechsel leert ihn. Es gibt keinen Disk-Cache. WLAN-Vorladen ab App 0.8.0 nutzt dieselben Cache-Schlüssel und dieselbe feste Frist. Eine bereits geöffnete Vorschau bleibt nach Fristablauf sichtbar; erneutes Öffnen lädt wieder vom Server. Auf älteren Servern gilt die Frist ebenfalls, die Wiederverwendung bleibt dort auf dieselbe Gesichts-URL beschränkt.

Der Server liefert dazu je Gesicht die optionale, undurchsichtige `original_key`-Kennung aus Originalpfad und indexierten Dateimetadaten. Unterschiedliche Gesichter und Personen desselben Fotos teilen diese Kennung; erkannte Dateiänderungen erzeugen eine neue Kennung. Anzeigeformatierte Pfade werden nicht als Cache-Schlüssel verwendet. Cachetreffer benötigen weder einen erneuten Bildabruf noch eine erneute Dekodierung; ein neuer Abruf nach Ablauf durchläuft wieder die serverseitigen Zugriffsprüfungen. HTTP-Antworten bleiben `private, no-store`; wiederverwendet wird ausschließlich das dekodierte Bild im App-Arbeitsspeicher.

## API und Datenmigration

Der gemeinsame Vertrag steht in [`../../openapi.yaml`](../../openapi.yaml), unter `/api/photos/labeling/v1`. Anfragen verwenden HTTP Basic über HTTPS und JSON. Die Sitzung meldet `named_people` ab BearStack 0.43.0. `GET /people?after=…&upper=…` liefert benannte Personen; die Aktionen `rename`, `unassign` und `favorite` erweitern den bestehenden Aktionsendpunkt ohne Protokollwechsel. Ab BearStack 0.49.0 meldet die Sitzung `merge_suggestions`; `GET /merge-suggestions/next` liefert ein Paar oder `suggestion: null`. `accept_merge` und `reject_merge` am Aktionsendpunkt benötigen zusätzlich `suggestion_id`, `target_id` und `target_revision`. Operationen tragen eine zufällige ID, Datenbestandskennung, Quellrevision und bei Zuordnung oder Gruppenentscheidung eine Zielrevision. Die Antwort enthält tatsächliche Gesichtszahlen und betroffene Gruppen-IDs.

Die Labeling-Tabellen bestehen seit Foto-Schema 19. BearStack 0.43.0 migriert kompatibel auf Schema 24 und ergänzt einen partiellen Index für benannte Personen; Namen, Gesichter, Revisionen und Quittungen bleiben erhalten. Das Room-Schema der App bleibt unverändert. Datenbanktrigger erhöhen Revisionen auch bei Web- und Hintergrundänderungen. Mutation und Quittung werden in derselben SQLite-Transaktion gespeichert. Quittungen sind kontogebunden und bleiben bis zum Löschen der Gesichtserkennungsdaten erhalten. Dieser Reset erzeugt eine neue Datenbestandskennung. Die Foto-Datenbank einschließlich dieser Tabellen gemeinsam sichern und wiederherstellen. Bestehende Web-Endpunkte bleiben erhalten; ausgeschlossene geschützte Fotos werden auch über diese API nicht angeboten.

Die Anwendungssitzung liegt in `connection/AppSession.kt`: Anmeldung, gespeichertes Profil,
Zertifikatsbestätigung, HTTP-Client, Bildcache und Galerie werden dort gemeinsam verwaltet.
`PeopleViewModel` verwaltet die Personenwarteschlange und ihre Aktionen. Die Personendatenbank
wird erst für eine Sitzung mit Personenrechten geöffnet; reine Lesekonten benötigen sie nicht. Gemeinsame
Originalbild-Caches und WLAN-Vorlader liegen im Paket `media`. Kontowechsel stoppt
zuerst die alten Feature-Aufgaben und schließt anschließend deren Sitzungsressourcen;
eine fehlgeschlagene Anmeldung ersetzt keine bestehende Verbindung.

## Tests

Die Build-Abhängigkeiten enthalten keine separate `ui-tooling-preview`-Deklaration,
da die App keine `@Preview`-Annotationen verwendet. Die Debug-Werkzeuge für Compose
und die instrumentierten UI-Tests bleiben erhalten; die App-Version ändert sich dadurch nicht.

Auch die leere Übergangsabhängigkeit `androidx.room:room-ktx` entfällt: Seit Room 2.7 liegen ihre APIs in `room-runtime`, das hier bereits explizit mit 2.8.4 eingebunden ist. Room-Compiler, Coroutines und Datenbankschema bleiben unverändert. Die Entfernung enthält keine Laufzeitänderung und erfordert keinen Versionssprung. Vor einem Upgrade auf AGP 10 müssen die Legacy-DSL-/Kotlin-Optionen in `gradle.properties` und die kapt-Anbindung migriert werden; sie sind für den aktuellen Aufbau weiterhin erforderlich. Siehe [Room-Releases](https://developer.android.com/jetpack/androidx/releases/room) und [AGP-Migrationsplan](https://developer.android.com/build/releases/gradle-plugin-roadmap).

```sh
# JVM-Tests, Lint und Debug-APK
make test-android

# JVM-Tests, Lint und minimierte Release-APK (R8)
make test-android-release

# Release-UI-Smoke gegen die minimierte App und den temporären Go-Server
make test-android-release-integration

# Mit gestartetem Emulator: Gesten, Room, Keystore
apps/android/gradlew -p apps/android :app:connectedDebugAndroidTest

# Mit genau einem gestarteten Emulator und adb im PATH:
# isolierter echter Go-HTTPS-Server, Zertifikat, Aktionen, parallele Webänderung
make test-android-integration

# Server inkl. Revisionen, Rechte, geschützte Fotos, Rollback, >500 Gesichter
make test-go
```

Compose-Regressionstests prüfen beim Vor- und Zurückblättern, dass Gesichtsraster und Navigation vor, während und nach dem Laden an derselben Position bleiben, auch bei doppelter Schriftgröße und gescrolltem Inhalt. Während des Ladens bleiben die Blätteraktionen gesperrt. Weitere Regressionen prüfen ablaufende Rücknahmen bei geöffnetem Namens- und Duplikatdialog, das anschließende Speichern jeder ursprünglichen Gruppe sowie die Wiederherstellung beim Hintergrundwechsel. Ein UI-Test ab Android 11 prüft außerdem Dialogidentität, Eingabefokus, sichtbare Bildschirmtastatur und weiteren Textinput beim Ändern und Ausblenden der Rückgängig-Meldung.

Cachetests prüfen tatsächliche HTTPS-Anfragezahlen und die Wiederverwendung desselben dekodierten Bitmaps für unterschiedliche Gesichter, den festen Drei-Minuten-Ablauf, Fehlerantworten, Cacheleeren, Speicherbegrenzung und alte Server ohne Bildkennung.

Zusätzliche Regressionen prüfen die vollständige Personenliste über mehrere Seiten, Umbenennen, Favoritenwechsel, Entfernen des letzten Gesichts, große Schrift, Halten/Wischen/Abbruch und TalkBack-Vorschau. Repository- und ViewModel-Tests sichern verlorene Antworten, Konflikte und den Erhalt der Zuordnungswarteschlange ab. Der echte HTTPS-Integrationstest führt die neuen Verwaltungsaktionen gegen den Go-Server aus. Ab 0.7.0 prüfen zusätzliche Tests Textsuche jenseits der ersten Seite, verspätete Suchantworten, Bestätigen/Abbrechen, fortlaufendes Nachladen über 80 Gesichter, Scrollposition nach Favorisieren und Revisionskonflikte beim Anfügen.

Ab App 0.9.0 prüfen Regressionen einzelne Gruppenentscheidungen, sichtbare Buttons bei doppelter Schriftgröße, beide Portrait-Vorschauen, doppelte Klicks, Konflikte, verlorene Antworten und Fehler beim Nachladen. Repository-Neustarts erhalten offene Entscheidungen und die Benennen-Bildseite. Die HTTPS-Integration prüft beide Entscheidungen samt Originalen, Vorschaubildern und Quittungswiederholung gegen isolierte Go-Instanzen hinter URL-Präfixen. Servertests prüfen zusätzlich Sichtbarkeit, Rollback, Vergleichsgesichter jenseits der ersten Bildseite und gleichzeitiges Annehmen/Ablehnen.

Der Release-Smoke benötigt Python 3, adb und einen dedizierten Testemulator. Er prüft
Anmeldung, Galerie, Fotoinformationen, Profilwiederherstellung und Kontowechsel über
die normale Oberfläche. Die Testwerkzeuge laufen außerhalb des App-Prozesses;
R8 benötigt dafür keine Keep-Regeln oder zusätzlichen Testbibliotheken in der App.
Der Lauf setzt die App-Daten im Testemulator vor und nach der Prüfung zurück und
schreibt `app/build/reports/release-smoke.xml`. Physische Geräte werden abgewiesen.

Nur `-Pbearstack.releaseSmoke=true` erlaubt ohne privaten Keystore eine Signierung
mit dem lokalen Debug-Schlüssel. Normale Release-Builds behalten die konfigurierte
Produktionssignierung. Die vollständigen Compose-/Repository-Gerätetests laufen
über das Debug-Ziel; fehlende oder ausschließlich übersprungene Tests sowie
Fehler werden zusätzlich anhand der XML-Ergebnisse zurückgewiesen.

Der Integrationstest öffnet ausschließlich `127.0.0.1:18787`, nutzt temporäre Daten und führt `adb reverse` aus. Er greift auf keine installierte BearStack-Instanz zu. Ohne Testadresse wird dieser zusätzliche instrumentierte Test übersprungen.

Vor einer privaten Verteilung zusätzlich die komplette Bedienung auf dem eigenen Gerät mit TalkBack prüfen, besonders Namensdialog/Tastatur und die Originalfoto-Vorschau. Ein Gerätewechsel, Energiesparmodus und herstellerspezifische Prozessbeendigung lassen sich nicht vollständig durch einen Emulator ersetzen.

## Privaten Release signieren

Einen dauerhaften Signierschlüssel **außerhalb des Repositories** anlegen und separat sichern. Updates derselben App-ID benötigen denselben Schlüssel. Passwörter interaktiv eingeben:

```sh
keytool -genkeypair -v -keystore /sicherer/pfad/bearstack-android.jks \
  -alias bearstack -keyalg RSA -keysize 4096 -validity 10000
```

Für den Build diese Umgebungsvariablen setzen (Passwörter beispielsweise mit `read -rs` interaktiv einlesen und anschließend exportieren):

- `BEARSTACK_ANDROID_KEYSTORE`: absoluter Pfad zur JKS-Datei.
- `BEARSTACK_ANDROID_KEY_ALIAS`: `bearstack`.
- `BEARSTACK_ANDROID_STORE_PASSWORD` und `BEARSTACK_ANDROID_KEY_PASSWORD`.

```sh
apps/android/gradlew -p apps/android :app:assembleRelease
adb install -r apps/android/app/build/outputs/apk/release/app-release.apk
```

Ohne Signiervariablen entsteht eine **unsignierte** Release-APK (`app-release-unsigned.apk`) zur Buildprüfung. Eine Debug-Installation hat einen anderen Schlüssel; sie muss vor dem ersten privaten Release deinstalliert werden, wobei ihre lokalen Daten verloren gehen. Bei jeder APK-Aktualisierung `versionCode` erhöhen und `apps/android/VERSION` pflegen; die Android-Version wird unabhängig von BearStack geführt.

## Struktur

```text
apps/android/
  VERSION                 # unabhängige App-Version
  gradlew, gradlew.bat     # eigener Build-Einstieg
  app/
    schemas/              # exportiertes Room-Schema
    src/main/java/de/bearstack/people/
      connection/         # TLS, Zertifikatsabgleich, Keystore
      data/local/         # Room-Zustand, Ereignisse, Statistikabfragen
      data/remote/        # JSON-Modelle und HTTP-Vertrag
      people/             # Repository, ViewModel, Gestenregeln
      statistics/         # lokale Gesichts- und Gruppenzähler
      ui/                 # Compose-Oberfläche und Gestaltung
    src/test/             # JVM-Tests
    src/androidTest/      # Emulator- und Go-Integrationstests
```

Die Personenlogik bleibt im Go-Backend unter `internal/photos`; HTTP-Adapter und Rechte liegen unter `internal/server`.
