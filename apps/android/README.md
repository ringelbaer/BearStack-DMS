# BearStack Personen für Android

Native, deutschsprachige App für Android 8.0 oder neuer. App-Version **0.1.0**; erforderlich sind **BearStack 0.30.0**, aktiviertes Fotomodul, vorhandene erkannte Gesichter und ein Konto mit `photos_manage` (beispielsweise `photos_manager` oder Administrator). Die App arbeitet online und spricht ausschließlich mit BearStack, niemals direkt mit dem Python-Gesichtsdienst.

## Bauen und installieren

`apps/android/` in Android Studio als eigenes Projekt öffnen. Voraussetzungen: JDK 17 oder 21 und Android-SDK mit Plattform 36. Das Projekt verwendet den Gradle Wrapper 8.13, AGP 8.13.2 und Kotlin 2.3.21. SDK-Pfad über `ANDROID_HOME` oder eine ignorierte `apps/android/local.properties` mit `sdk.dir=…` setzen.

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

## Bearbeitung

- Das Grid zeigt bis zu vier **Gesichtsausschnitte**, keine ganzen Fotos. Bei größeren Gruppen blättern „Zurück“ und „Weiter“ durch Viererseiten. Benennen, Zuordnen und Ignorieren betreffen immer die gesamte Gruppe.
- Einen Ausschnitt halten, um ihn zu vergrößern. Loslassen oder Abbruch schließt die Vorschau. Während des Haltens gibt es keine Wischaktion. Über die TalkBack-Aktion „Vergrößern“ bleibt die Vorschau bis „Vorschau schließen“ geöffnet.
- Der Stift öffnet das Namensfeld. Nach 250 ms Eingabepause erscheinen höchstens 20 Vorschläge. Einen Vorschlag antippen, um zuzuordnen; „Speichern“ benennt die aktuelle Gruppe. Bei einem bestehenden gleichen Namen wird zwischen Zuordnen und „Separat benennen“ unterschieden.
- Das **×** links unten trennt genau dieses Gesicht in eine neue unbenannte Gruppe ab. Es ignoriert und löscht nichts. Die neue Gruppe folgt nach Abschluss der aktuellen Gruppe; mehrere Abtrennungen folgen ihrer Reihenfolge. Bei einem einzigen Gesicht entfällt das ×.
- **Nach oben wischen:** die ganze Gruppe ignorieren. Für fünf Sekunden ist „Rückgängig“ verfügbar; weitere Bearbeitungen sind gesperrt. Erst nach Ablauf wird gesendet. Wechsel in den Hintergrund oder Prozessende während dieser Frist verwirft die noch nicht gesendete Aktion.
- **Nach links wischen:** lokal für den aktuellen Durchgang überspringen. Über „Übersprungene bearbeiten“ am Ende des Durchgangs werden diese Gruppen erneut angeboten. „Neuen Durchgang starten“ übernimmt inzwischen hinzugekommene Gruppen; übersprungene bleiben der ausdrücklichen Wiederaufnahme vorbehalten.
- Ignorieren und Überspringen sind auch als beschriftete Menüaktionen verfügbar. Die Oberfläche unterstützt System-Hell-/Dunkelmodus und große Schrift; Inhalte und Dialoge sind scrollbar.

Nach einer bestätigten Aktion wird weitergeschaltet. Bei einem Fehler bleibt die Gruppe erhalten. „Offene Aktion prüfen“ klärt zuerst die serverseitige Aktionsquittung und sendet nur bei fehlender Quittung denselben gespeicherten Auftrag erneut. Währenddessen sind weitere Entscheidungen gesperrt. Bei fehlenden Rechten oder falschen Zugangsdaten kann die Verbindung gewechselt und mit demselben Konto wiederhergestellt werden.

Ändert die Weboberfläche oder Hintergrundverarbeitung inzwischen eine Gruppe, meldet die API `409`; die App lädt den aktuellen Stand und verlangt eine neue Entscheidung. Sie überträgt die alte Entscheidung nicht automatisch auf eine veränderte Gruppe. Eine Änderung an der Zielperson erfordert ebenfalls eine erneute Auswahl.

## Warteschlange und Statistik

Room trennt lokalen Zustand nach Instanz, Datenbestand und Konto. Gespeichert werden aktueller Datensatz und Bildseite, Cursor mit fester oberer Gruppen-ID, abgetrennte und übersprungene Gruppen, ungeklärte Aktionen und bestätigte Ereignisse. Eine noch nicht gesendete Rücknahmefrist ist ausschließlich im Arbeitsspeicher. Es gibt keine allgemeine Offline-Warteschlange.

Die Statistik zählt **Gesichter und Gruppen**, jeweils heute (lokale Zeitzone) und insgesamt: Benannt, Zugeordnet, Ignoriert, Übersprungen. Serveraktionen zählen nach Bestätigung genau einmal pro Aktions-ID. Überspringen zählt einmal je Gruppe und Durchgang. Abtrennen hat keinen eigenen Statistikzähler. Die Zahlen sind gerätelokal; sie sind kein vollständiges Server-Audit. App-Daten löschen oder Deinstallation entfernt die lokalen Zahlen.

Ein Durchgang lädt Metadaten in Seiten von maximal 20 Gruppen. Jede Gruppe wird vor Anzeige erneut geprüft. Benannte, leere oder gelöschte Gruppen werden ausgelassen. Vier Bilder werden angezeigt, höchstens die nächste Viereransicht wird vorgeladen. Vergrößerungen werden erst beim Halten angefordert. Der Bildcache ist auf 16 MiB begrenzt und hat keinen Disk-Cache.

## API und Datenmigration

Der gemeinsame Vertrag steht in [`../../openapi.yaml`](../../openapi.yaml), unter `/api/photos/labeling/v1`. Anfragen verwenden HTTP Basic über HTTPS und JSON. Operationen tragen eine zufällige ID, Datenbestandskennung, Quellrevision und bei Zuordnung eine Zielrevision. Die Antwort enthält tatsächliche Gesichtszahlen und betroffene Gruppen-IDs.

BearStack migriert die Foto-Datenbank kompatibel auf Schema 19. Datenbanktrigger erhöhen Revisionen auch bei Web- und Hintergrundänderungen. Mutation und Quittung werden in derselben SQLite-Transaktion gespeichert. Quittungen sind kontogebunden und bleiben bis zum Löschen der Gesichtserkennungsdaten erhalten. Dieser Reset erzeugt eine neue Datenbestandskennung. Die Foto-Datenbank einschließlich dieser Tabellen gemeinsam sichern und wiederherstellen. Bestehende Web-Endpunkte bleiben erhalten; ausgeschlossene geschützte Fotos werden auch über diese API nicht angeboten.

## Tests

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

Der Integrationstest öffnet ausschließlich `127.0.0.1:18787`, nutzt temporäre Daten und führt `adb reverse` aus. Er greift auf keine installierte BearStack-Instanz zu. Ohne Testadresse wird dieser zusätzliche instrumentierte Test übersprungen.

Vor einer privaten Verteilung zusätzlich die komplette Bedienung auf dem eigenen Gerät mit TalkBack prüfen, besonders Namensdialog/Tastatur und die Vergrößerung. Ein Gerätewechsel, Energiesparmodus und herstellerspezifische Prozessbeendigung lassen sich nicht vollständig durch einen Emulator ersetzen.

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
