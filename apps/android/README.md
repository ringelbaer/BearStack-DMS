# BearStack Personen für Android

Native, deutschsprachige App für Android 8.0 oder neuer. App-Version **0.8.3**; erforderlich sind **BearStack 0.35.0**, aktiviertes Fotomodul, vorhandene erkannte Gesichter und ein Konto mit `photos.edit` (Rolle „Fotos bearbeiten“/`photos_editor`, `photos_manager` oder Administrator; Freigabe für Fotobearbeiter ab BearStack 0.36.0). Bestehende lokale Daten werden beim Update automatisch erhalten; Rücknahmen stehen für ab Version 0.2.0 übersprungene Gruppen bereit. Die App arbeitet online und spricht ausschließlich mit BearStack, niemals direkt mit dem Python-Gesichtsdienst.

## Bauen und installieren

`apps/android/` in Android Studio als eigenes Projekt öffnen. Voraussetzungen: JDK 17 oder 21 und Android-SDK mit Plattform 36. Das Projekt verwendet den Gradle Wrapper 9.6.0, AGP 9.4.0 und Kotlin 2.3.21. SDK-Pfad über `ANDROID_HOME` oder eine ignorierte `apps/android/local.properties` mit `sdk.dir=…` setzen.

```sh
./scripts/check-android.sh
adb install -r apps/android/app/build/outputs/apk/debug/app-debug.apk
```

Alternativ im Android-Verzeichnis: `./gradlew :app:assembleDebug`. APKs werden lokal gebaut und nicht automatisch veröffentlicht. Go-, Web- und Docker-Builds benötigen kein Android-SDK. `apps/android` ist aus dem Docker-Buildkontext ausgeschlossen.

## Verbindung und Zertifikat

HTTPS-Adresse (gegebenenfalls mit Reverse-Proxy-Pfad), Benutzername und Passwort eingeben. Die App prüft Protokollversion und Personenrechte. HTTP und automatische Weiterleitungen sind ausgeschlossen; bei einer Umleitung die endgültige HTTPS-Adresse verwenden.

Öffentlich vertrauenswürdige Zertifikate werden regulär geprüft. Bei einem selbstsignierten Serverzertifikat erscheint **vor Übermittlung der Zugangsdaten** dessen SHA-256-Fingerabdruck. Auf dem Server den Fingerabdruck der tatsächlich verwendeten Zertifikatsdatei ermitteln:

```sh
openssl x509 -in /pfad/zum/server.crt -noout -fingerprint -sha256
```

Alle Hexadezimalpaare vergleichen, anschließend „Abgeglichen und vertrauen“ wählen. Nur dieses Zertifikat wird für das Profil akzeptiert; Hostname und Gültigkeit bleiben verbindlich. Eine IP-Adresse funktioniert nur mit einem entsprechenden IP-Eintrag im Subject Alternative Name des Zertifikats. Bei einem Zertifikatswechsel die Verbindung erneut einrichten und den neuen Fingerabdruck prüfen. Private Zertifikatsketten mit eigener CA werden in Version 1 nicht als selbstsigniertes Blattzertifikat angeboten.

Es gibt ein aktives Profil. Zugangsdaten und bestätigtes Zertifikat werden mit AES-GCM unter einem Schlüssel im Android Keystore in `noBackupFilesDir` gespeichert. Backups und Gerätetransfers sind ausgeschlossen. Screenshots und die Vorschau im App-Umschalter sind gesperrt. Die App schreibt keine Zugangsdaten in Logs. Bilder werden nur im begrenzten Arbeitsspeichercache gehalten; ein Profilwechsel leert den Cache.

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

Der gemeinsame Vertrag steht in [`../../openapi.yaml`](../../openapi.yaml), unter `/api/photos/labeling/v1`. Anfragen verwenden HTTP Basic über HTTPS und JSON. Die Sitzung meldet `named_people` ab BearStack 0.43.0. `GET /people?after=…&upper=…` liefert benannte Personen; die Aktionen `rename`, `unassign` und `favorite` erweitern den bestehenden Aktionsendpunkt ohne Protokollwechsel. Operationen tragen eine zufällige ID, Datenbestandskennung, Quellrevision und bei Zuordnung eine Zielrevision. Die Antwort enthält tatsächliche Gesichtszahlen und betroffene Gruppen-IDs.

Die Labeling-Tabellen bestehen seit Foto-Schema 19. BearStack 0.43.0 migriert kompatibel auf Schema 24 und ergänzt einen partiellen Index für benannte Personen; Namen, Gesichter, Revisionen und Quittungen bleiben erhalten. Das Room-Schema der App bleibt unverändert. Datenbanktrigger erhöhen Revisionen auch bei Web- und Hintergrundänderungen. Mutation und Quittung werden in derselben SQLite-Transaktion gespeichert. Quittungen sind kontogebunden und bleiben bis zum Löschen der Gesichtserkennungsdaten erhalten. Dieser Reset erzeugt eine neue Datenbestandskennung. Die Foto-Datenbank einschließlich dieser Tabellen gemeinsam sichern und wiederherstellen. Bestehende Web-Endpunkte bleiben erhalten; ausgeschlossene geschützte Fotos werden auch über diese API nicht angeboten.

## Tests

Die Build-Abhängigkeiten enthalten keine separate `ui-tooling-preview`-Deklaration,
da die App keine `@Preview`-Annotationen verwendet. Die Debug-Werkzeuge für Compose
und die instrumentierten UI-Tests bleiben erhalten; die App-Version ändert sich dadurch nicht.

Auch die leere Übergangsabhängigkeit `androidx.room:room-ktx` entfällt: Seit Room 2.7 liegen ihre APIs in `room-runtime`, das hier bereits explizit mit 2.8.4 eingebunden ist. Room-Compiler, Coroutines und Datenbankschema bleiben unverändert. Die Entfernung enthält keine Laufzeitänderung und erfordert keinen Versionssprung. Vor einem Upgrade auf AGP 10 müssen die Legacy-DSL-/Kotlin-Optionen in `gradle.properties` und die kapt-Anbindung migriert werden; sie sind für den aktuellen Aufbau weiterhin erforderlich. Siehe [Room-Releases](https://developer.android.com/jetpack/androidx/releases/room) und [AGP-Migrationsplan](https://developer.android.com/build/releases/gradle-plugin-roadmap).

```sh
# JVM-Tests, Lint und Debug-APK
make test-android

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
