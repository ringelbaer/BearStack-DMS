---
title: Android-App – Technik & Entwicklung
description: Build, Signierung, Verbindung, API, Datenhaltung und Tests der Android-App.
---

# Android-App – Technik & Entwicklung

Diese Referenz richtet sich an Betreiber und Entwickler. Die
[Android-Anleitung](android.md) erklärt Einrichtung und Bedienung vom ersten Start
bis zur Personenverwaltung. Stand: App **0.20.0** (`versionCode 41`), BearStack **1.2.0**.

## Bauen und installieren

Öffne `apps/android/` in Android Studio als eigenes Projekt. Benötigt werden JDK 17
oder 21 und das Android-SDK mit Plattform 36. Gradle Wrapper, Android Gradle Plugin
und Kotlin sind im Projekt festgelegt; verwende den mitgelieferten Wrapper.
Setze den SDK-Pfad über `ANDROID_HOME` oder eine ignorierte
`apps/android/local.properties` mit `sdk.dir=…`.

Vom Repository-Stamm aus:

```sh
# JVM-Tests, Lint und Debug-APK
./scripts/check-android.sh

# Auf einem verbundenen Testgerät installieren
adb install -r apps/android/app/build/outputs/apk/debug/app-debug.apk
```

Für einen reinen Build:

```sh
apps/android/gradlew -p apps/android :app:assembleDebug
```

Die APKs werden lokal gebaut und nicht automatisch veröffentlicht. Go-, Web- und
Docker-Builds benötigen kein Android-SDK; `apps/android` ist aus dem
Docker-Buildkontext ausgeschlossen. Die App-ID lautet `de.bearstack.people`.

## Privaten Release signieren

Updates derselben App-ID benötigen denselben Signierschlüssel. Lege einen dauerhaften
Schlüssel außerhalb des Repositories an und sichere ihn separat. Passwörter werden
interaktiv eingegeben:

```sh
keytool -genkeypair -v -keystore /sicherer/pfad/bearstack-android.jks \
  -alias bearstack -keyalg RSA -keysize 4096 -validity 10000
```

Der Release-Build verwendet diese Umgebungsvariablen:

| Variable | Inhalt |
| --- | --- |
| `BEARSTACK_ANDROID_KEYSTORE` | Absoluter Pfad zur JKS-Datei. |
| `BEARSTACK_ANDROID_KEY_ALIAS` | Alias, beispielsweise `bearstack`. |
| `BEARSTACK_ANDROID_STORE_PASSWORD` | Passwort des Keystores. |
| `BEARSTACK_ANDROID_KEY_PASSWORD` | Passwort des Schlüssels. |

Passwörter beispielsweise mit `read -rs` einlesen und anschließend exportieren.

```sh
apps/android/gradlew -p apps/android :app:assembleRelease
adb install -r apps/android/app/build/outputs/apk/release/app-release.apk
```

Ohne Signiervariablen entsteht `app-release-unsigned.apk` zur Buildprüfung. Eine
Debug-Installation hat einen anderen Schlüssel und muss vor dem ersten privaten
Release deinstalliert werden; dabei gehen ihre lokalen Daten verloren.
`apps/android/VERSION` und `versionCode` in `app/build.gradle.kts` werden für
APK-Aktualisierungen gepflegt. Die Android-Version ist unabhängig von BearStack.

## Verbindung und Zertifikat

Die App verwendet HTTPS einschließlich optionalem Reverse-Proxy-Pfad. HTTP und
automatische Weiterleitungen sind ausgeschlossen; einzutragen ist die endgültige
HTTPS-Adresse. Vor der Anmeldung werden Serverprotokoll und anschließend die
verfügbaren Funktionen und Rechte geprüft.

Öffentlich vertrauenswürdige Zertifikate werden regulär geprüft. Bei einem
selbstsignierten Blattzertifikat zeigt die App vor Übermittlung der Zugangsdaten
den SHA-256-Fingerabdruck an. Ermittle ihn für die tatsächlich ausgelieferte
Zertifikatsdatei:

```sh
openssl x509 -in /pfad/zum/server.crt -noout -fingerprint -sha256
```

Vergleiche alle Hexadezimalpaare. **Abgeglichen und vertrauen** bindet genau dieses
Zertifikat an das Profil. Hostname und Gültigkeitszeitraum werden weiter geprüft.
Eine IP-Adresse benötigt einen entsprechenden IP-Eintrag im Subject Alternative
Name. Nach einem Zertifikatswechsel wird die Verbindung mit erneutem Abgleich
eingerichtet. Private Ketten mit eigener CA werden nicht über diese Freigabe für
selbstsignierte Blattzertifikate akzeptiert.

Es gibt ein aktives Profil. Zugangsdaten und bestätigtes Zertifikat werden mit
AES-GCM und einem Schlüssel im Android Keystore in `noBackupFilesDir` gespeichert.
Backups und Gerätetransfers sind ausgeschlossen. Screenshots, Bildschirmaufnahmen
und die Vorschau im App-Umschalter sind möglich.

Debug-Builds protokollieren unter `BearStackConnection` nur Phase und Diagnosecode,
keine vollständigen URLs, Exception-Texte, Benutzernamen oder Passwörter.
[Diagnosecodes und erste Prüfschritte](android.md#probleme-beheben) stehen in der Anleitung.

## API und Datenhaltung

Der verbindliche HTTP-Vertrag steht in
[`openapi.yaml`](https://github.com/ringelbaer/BearStack-DMS/blob/main/openapi.yaml).
Die App verwendet HTTP Basic über HTTPS; Galerie und Personenverwaltung teilen
sich die Foto-Dienste und Zugriffsregeln des Browsers.

| API-Bereich | Aufgabe |
| --- | --- |
| `/api/photos/v1/` | Sitzung, Galerie, Datumssprung, Medieninformationen, Karten und Textbeiträge. |
| `/api/photos/labeling/v1/` | Personenlisten, Benennen, Zuordnen, Favoriten, Gruppenvergleiche und Aktionsquittungen. |
| `/photos/faces/{id}/suggestions` | Gesichtsabgleich mit bereits benannten Personen. |

Funktionen werden anhand von Serverfähigkeiten eingeblendet, etwa `named_people`,
`merge_suggestions`, `merge_side_actions`, `merge_naming`, `named_face_batch`, `person_folders` und
`people_count_sort`. Die [Kompatibilitätstabelle](android.md#serverkompatibilitat)
nennt die zugehörigen Mindestversionen. Ältere Server behalten die jeweils
unterstützten Bedienwege; fehlende Funktionen führen nicht zu einem Protokollwechsel.

### Native Ordnerprüfung

`GET /api/photos/labeling/v1/people/{id}/folders?page=1` liefert höchstens 40 exakte
Ordner und acht Gesichter pro Ordner. Die App hält nur die aktuelle Seite. Anzeigen
verwenden ausschließlich `display_path`; `directory` bleibt unverändert im Auftrag.
Vorschauen enthalten die vorhandenen Original-Cachekennungen, Gesichtsrahmen und
Prüfmarkierungen. Die bekannte Bildansicht nutzt dieselben geschützten
Thumbnail-/Originalendpunkte und lädt Originale nur im begrenzten bestehenden Cache.

`folder_move`, `folder_unnamed`, `folder_ignore`, `folder_exclude` und `folder_include`
verwenden den bestehenden `people/{id}/actions`-Endpunkt und die persistente
Room-Auftragsablage. Die gemeinsame Serverimplementierung für Web und Android
ändert den ganzen exakten Ordner atomar, ohne eine vollständige Gesichtsliste an
den Client zu übertragen. Quittung und Änderung werden zusammen geschrieben;
Wiederholungen prüfen die Quittung vor der inzwischen geänderten Quellrevision.
Die Neuzuweisung an eine vorhandene Person prüft zusätzlich deren Revision.
Die Personenliste fordert `include_excluded=1` an, damit benannte Personen mit
ausschließlich sichtbaren Sperren nach einem Neustart wieder erreichbar bleiben.

### Aktionen, Revisionen und Wiederanlauf

Schreibaktionen tragen eine zufällige Aktions-ID, Datenbestandskennung und
Quellrevision. Zuordnungen und Gruppenentscheidungen benötigen zusätzlich die
Zielrevision; Gruppenvergleiche enthalten auch die Vorschlags-ID. Der Server
speichert Änderung und Quittung in derselben SQLite-Transaktion. Quittungen sind
kontogebunden; bestätigte Aktionen werden dadurch auch nach verlorenen Antworten
nicht doppelt ausgeführt.

Web- und Hintergrundänderungen erhöhen dieselben Revisionen. Ein Konflikt verlangt
eine neue Benutzerentscheidung. **Offene Aktion prüfen** liest zuerst die Quittung
und sendet nur bei fehlender Quittung denselben gespeicherten Auftrag erneut.
Die Foto-Datenbank einschließlich Quittungen gemeinsam sichern und wiederherstellen.
Das Löschen der Gesichtserkennungsdaten erzeugt eine neue Datenbestandskennung und
beendet die Gültigkeit der bisherigen Quittungen.

Room hält die aktuelle Gruppe und Bildseite, Durchgangsgrenze, übersprungene und
abgetrennte Gruppen, offene Aktionen und bestätigte Statistikereignisse getrennt
nach Instanz, Datenbestand und Konto. Das aktuelle Room-Schema **4** speichert die
Warteschlangen als einzelne indizierte Einträge; Migrationen übernehmen vorhandene
Reihenfolgen und offene Aktionen. Wiederherstellung liest höchstens 256 Einträge
je Block, normales Blättern einzelne Einträge.

Rücknahmetimer existieren nur im Arbeitsspeicher. Noch nicht gesendete Ignorieraktionen
kehren bei Hintergrundwechsel oder Neustart in die Warteschlange zurück. Ein offener
Namens- oder Duplikatdialog verzögert das Senden abgelaufener Ignorieraktionen, ohne
die Rücknahmefrist zu verlängern. Bereits möglicherweise gesendete Aktionen werden
über ihre Quittung geklärt. Dies ist keine allgemeine Offline-Warteschlange.

### Gesichtsabgleich

Die Lupe fordert NDJSON-Zwischenstände an; ältere passende Server können eine
JSON-Antwort liefern. Es werden höchstens 20 Treffer und 64 KiB pro Zwischenstand
verarbeitet. Im Speicher liegen nur die aktuelle Rangliste und der neueste wartende
Zwischenstand. Ein fehlender Abschluss oder Übertragungsfehler verwirft vorläufige
Treffer und erlaubt einen erneuten Versuch.

Beim Vergleich zweier unbenannter Gruppen laufen höchstens zwei Abgleiche
nacheinander: zuerst für die erste Gruppe, nur bei leerem Endergebnis für die zweite.
Fertige Ergebnisse gelten für das aktuelle Paar; Gruppenwechsel oder bestätigte
Einzelaktionen verwerfen sie. Ein Hintergrundwechsel bricht laufende Suchen ab;
unvollständige Ergebnisse werden beim Fortsetzen neu ermittelt. Die Zuordnung
lädt nur die aktuelle Zielperson nach und verwendet die bestehenden Revisionsprüfungen.

## Speicher und Ladeverhalten

Die Fotoframe-Einstellung `frame_random` wird lokal gespeichert. Die native Session
meldet Unterstützung über `frame_random_sort`; fehlt die Fähigkeit, bleibt die
bisherige Wiedergabe aktiv. Der Server verarbeitet `sort=random` über den bereits
indexierten stabilen Zufallswert. Die Reihenfolge bleibt über Seitenwechsel und
Wiederholungen hinweg gleich; es gibt keinen vollständigen Metadatenabruf.

Bei Gerätefotos wird für den gewählten Ordner einmal eine primitive `LongArray`
mit Bild-IDs gelesen und gemischt (8 Byte pro Bild, etwa 0,8 MB für 100.000 Fotos).
Metadaten werden anschließend nur für die jeweils bis zu 96 IDs einer Seite
abgefragt. Das funktioniert auch bei Anbietern ohne native Offset-Pagination.
Schließen, Verbindungswechsel und Wechsel der Reihenfolge geben die ID-Liste frei.
Der nächste Fotoframe-Start mischt Gerätefotos neu.

### Metadatengrenzen

| Bereich | Seitengröße und Begrenzung |
| --- | --- |
| Galerie | 96 Medien, 24 Ordner und 20 Textzusammenfassungen je Seite; höchstens drei Seiten je Bereich. |
| Ordnervorschauen | Bis zu vier je normalem Ordner, bei virtuellen Personenordnern bis zu acht. |
| Rückkehr aus dem Fotoframe | Zusätzlich bleibt die vorherige Galerie innerhalb derselben Grenzen erhalten. |
| Lokale Fotos | 96 Fotos beziehungsweise 24 Ordner je Seite; höchstens drei Seiten im Raster. |
| Personenliste | Bis zu 20 Personen je Anfrage; Namenssuche nach 250 ms Eingabepause. |
| Gesichter einer benannten Person | Bis zu 40 je Anfrage über einen Gesichts-ID-Cursor; ältere Server liefern Viererseiten. |
| Benennungsdurchgang | Bis zu 20 Gruppen je Metadatenseite; Anzeige in Viererseiten. |
| Mehrfachauswahl | Höchstens 500 Gesichter pro Aktion. |

Entfernte Galerie-Metadaten werden beim Zurückscrollen erneut geladen. Ansichts- und
Kontowechsel brechen überholte Anfragen ab. Der native Katalog ergänzt Identität,
Inhaltsrevision und Prüfstatus gemeinsam für Medien und Ordnervorschauen in
Abfragen mit höchstens 200 unterschiedlichen Pfaden. Normale Ordnerseiten mit
Namenssortierung werden direkt in SQLite paginiert; komplexe Suchen,
Datumssortierung und unvollständige Sichtbarkeitszähler behalten den bisherigen Pfad.

Der Datumssprung ermittelt Nachbartage über höchstens vier Bereichsabfragen und
zählt die Einträge vor der Zielseite. Position und Ziel stammen aus derselben
Lesetransaktion. Die App lädt danach die Zielseite, keine vollständige Medienliste.

Lokale Ordner lesen nur Androids MediaStore. Je Ordner bleiben ein Zähler und
höchstens zwei Vorschau-IDs erhalten. Abfragen laufen abbrechbar außerhalb des
UI-Threads; Anbieter ohne native Seitengrenzen werden über Cursor gelesen.
Reihenfolge ist die absteigende Medien-ID. Es gibt keine lokalen Uploads oder
Serveranfragen für Gerätefotos.

### Bild- und Dateicaches

| Cache | Grenze und Lebensdauer |
| --- | --- |
| Server-Galerie-Thumbnails | Einstellbar 64–2048 MiB, Standard 256 MiB; der geschützte Pflichtbestand darf das Budget überschreiten. |
| Große Vorschauen | 16 MiB Arbeitsspeicher; höchstens drei Minuten ab erfolgreichem Laden, ohne Verlängerung bei Zugriff. Kein Disk-Cache. |
| Lokale Fotos | 16 MiB Arbeitsspeicher; beim Verlassen der lokalen Ansicht oder Hintergrundwechsel geleert. |
| Vorbereitete Serverbilder zum Teilen | Höchstens 256 MiB je Datei sowie acht Dateien und 512 MiB insgesamt; Bereinigung beim nächsten Teilen, auch für Dateien ab 24 Stunden. |
| OpenStreetMap-Kartenbilder | 64 MiB HTTP-Cache; Cache-Header und bedingte Anfragen werden berücksichtigt. |

Galerie-Thumbnails liegen privat in `noBackupFilesDir`, getrennt nach Server,
Instanz, Datenbestand, Konto, Bildversion und Vorschaugröße. Die neuesten 50
ungefilterten Stream-Vorschauen und alle Vorschauen der ersten Ordnerebene sind
geschützt. Beim Vorladen werden vorhandene Dateien nur anhand ihrer Metadaten
geprüft; unveränderte Schutzlisten werden nicht erneut geschrieben. Die Nutzung
beim Anzeigen bestimmt die Verdrängungsreihenfolge ungeschützter Einträge.

Große Gesichtsvorschauen verwenden den serverseitigen Wert `large_preview_size`
(Standard 3840 Pixel längste Kante); die App dekodiert mit einem Ziel von 2048 Pixeln.
Ältere Server ohne diesen Modus liefern über denselben Endpunkt das Original.
Die optionale Kennung `original_key` erlaubt verschiedenen Gesichtern desselben
Fotos, ein dekodiertes Bild gemeinsam zu verwenden. Rahmen und Zoom werden separat
gezeichnet. Bei Speicherdruck können Bilder vor Ablauf der drei Minuten verdrängt werden.

Große Vorschauen werden nur über WLAN vorgeladen: in der Galerie höchstens die
nächste Aufnahme, in Personenansichten die angezeigten Portraits nacheinander.
Wechsel auf Mobilfunk oder in den Hintergrund bricht dies ab. Das eigenständige
Vorladen der geschützten Galerie-Thumbnails darf dagegen mobile Daten nutzen.
Ein ausdrücklicher Verbindungswechsel leert die Server-Bildcaches.

Zum Teilen werden Serveroriginale ohne Bilddekodierung mit einem 64-KiB-Puffer
übertragen. Fehler und Abbruch entfernen angefangene Dateien. Die empfangende App
bekommt vorübergehenden Zugriff über einen FileProvider; lokale Fotos werden über
ihre MediaStore-URI ohne zusätzliche Kopie freigegeben.

### Karten und Geometrien

Die App hält höchstens 289 Fotomarker je Ausschnitt, fordert bis zu 4.096
Fotoroutenkoordinaten an und zeigt bis zu 256 GPX-Tracks mit gemeinsam höchstens
8.192 Punkten. Die Trackliste hält zusätzlich zu ausgewählten Namen höchstens
96 Einträge. Zwei GPX-Geometrieanfragen laufen gleichzeitig; überholte Anfragen
werden abgebrochen. Abschnitte bleiben getrennt, auch an der Datumsgrenze.

GPX-Dateien dürfen serverseitig höchstens 16 MiB und 100.000 Track-/Routenpunkte
enthalten; der Parsercache ist auf 32 MiB begrenzt. Fehlende Indexdaten werden beim
normalen Indexlauf ergänzt. Foto- und GPX-Routen verwenden gemeinsame Parser,
Zugriffsprüfungen und Geometrie-Arbeitsplätze des Servers.

Der Servercache für vollständig gruppierte Fotorouten liegt unter
`<Cache-Verzeichnis>/photo-routes/v1/`: höchstens 256 JSON-Dateien und 512 MiB,
zuzüglich temporärer Dateien bei atomarem Ersatz. Ordner, Medientyp,
Gruppierungsradius und Sichtbarkeit bestimmen den Schlüssel; Zoomstufen verwenden
dieselbe Grundlage. Suchrouten werden berechnet, aber nicht dauerhaft gespeichert.
Revisionen invalidieren betroffene Routen bei Änderungen; Zugriffsprüfungen bleiben
vor jedem Abruf aktiv. Details beschreibt die [Kartenreferenz](fotos-technik.md#karten-und-gpx).
Kartenbilder lädt ein eigener Client ohne BearStack-Zugangsdaten.

## Tests

Alle Befehle werden vom Repository-Stamm aus ausgeführt:

```sh
# JVM-Tests, Lint und Debug-APK
make test-android

# JVM-Tests, Lint und minimierte Release-APK (R8)
make test-android-release

# Mit gestartetem Emulator: Compose, Gesten, Room, Keystore und Caches
apps/android/gradlew -p apps/android :app:connectedDebugAndroidTest

# Mit genau einem Testemulator und adb im PATH: echter temporärer Go-HTTPS-Server
make test-android-integration

# Bedienung der minimierten App gegen einen temporären Go-Server
make test-android-release-integration

# Serverseitige Verträge, Rechte, Revisionen und Rollback
make test-go
```

Die Tests decken unter anderem Navigation und große Schrift, Vorschaugesten,
Tastatureingabe während Rücknahmefristen, Seitenwechsel, verlorene Antworten,
Konflikte, Neustarts, Statistik und die Grenzen der Bildcaches ab. Cachetests
prüfen auch echte HTTPS-Anfragezahlen, unveränderte Dateien beim erneuten Vorladen
und Reparatur gelöschter oder beschädigter Einträge.

Die HTTPS-Integration verwendet temporäre Daten, `127.0.0.1:18787` und `adb reverse`;
sie greift auf keine installierte BearStack-Serverinstanz zu. Ohne Testadresse wird
dieser zusätzliche instrumentierte Test übersprungen.

Der Release-Smoke benötigt Python 3, adb und einen dedizierten Emulator. Er setzt
dessen App-Daten vor und nach dem Lauf zurück; physische Geräte werden abgewiesen.
Der Bericht liegt unter `app/build/reports/release-smoke.xml`. Nur
`-Pbearstack.releaseSmoke=true` erlaubt ohne privaten Keystore eine Signierung mit
dem lokalen Debug-Schlüssel. Die normale Produktionssignierung bleibt davon getrennt.

Vor privater Verteilung die Bedienung zusätzlich auf dem eigenen Gerät prüfen,
insbesondere TalkBack, Namensdialog/Tastatur und die Originalfoto-Vorschau.

## Projektstruktur

```text
apps/android/
  VERSION                 # eigenständige App-Version
  gradlew, gradlew.bat     # Build-Einstieg
  app/
    schemas/              # exportierte Room-Schemata
    src/main/java/de/bearstack/people/
      connection/         # Sitzung, TLS, Zertifikatsabgleich, Keystore
      data/local/         # Room-Zustand, Ereignisse, Statistik
      data/remote/        # JSON-Modelle und HTTP-Vertrag
      media/              # Bildcaches und Vorladen
      photos/             # Galerie, Gerätefotos, Karten und Wiedergabe
      people/             # Repository, ViewModel und Gestenregeln
      statistics/         # lokale Gesichts- und Gruppenzähler
      ui/                 # Compose-Oberfläche
    src/test/             # JVM-Tests
    src/androidTest/      # Geräte- und Integrationstests
```

`connection/AppSession.kt` verwaltet Anmeldung, Profil, HTTP-Client, Bildcaches und
Galerie. `PeopleViewModel` verwaltet Personenwarteschlange und Aktionen. Die lokale
Personendatenbank wird erst für eine Sitzung mit Personenrechten geöffnet. Ein
Kontowechsel stoppt zuerst die Feature-Aufgaben und schließt dann die Sitzungsressourcen.
Die Personenlogik liegt im Go-Backend unter `internal/photos`, HTTP-Adapter und
Rechte unter `internal/server`.

Für die Buildpflege: Die App verwendet `room-runtime` und den Room-Compiler;
`room-ktx` und `ui-tooling-preview` sind keine zusätzlichen Abhängigkeiten. Vor einem
Upgrade auf AGP 10 müssen die Legacy-DSL-/Kotlin-Optionen in `gradle.properties` und
die kapt-Anbindung migriert werden. Die bestehenden Optionen gehören zum aktuellen
Projektaufbau. Verbindlich sind die Gradle-Dateien im Repository.
