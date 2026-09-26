# BearStack Fotos für Android

Native Android-App für BearStack-Galerie, lokale Fotos und Personenverwaltung.
App-Version **0.23.0** (`versionCode 49`), Android **8.0 oder neuer**.

Rücknavigation, Einstellungen und Hilfe sind zwischen den Ansichten vereinheitlicht.
Die Personenverwaltung hat sichtbare Reiter; Auswahlaktionen beziehen sich ausdrücklich
auf ausgewählte Gesichter. Ein Verbindungswechsel erklärt die Abmeldung und lässt
sich vor dem Entfernen der gespeicherten Verbindung abbrechen.

Einzahl-/Mehrzahltexte sind in Deutsch und Englisch korrigiert. Das Galerieraster
verwendet die Fenstergröße, der Scrollgriff vermeidet unnötige Neuberechnungen.
Lint-Warnungen brechen Debug- und Release-Builds ab; begründete Ausnahmen und die
KSP-Buildkonfiguration stehen in der technischen Referenz.

Die Großansicht verwendet deckende Leisten einschließlich der Android-Systemabstände;
das Bild läuft bei eingeblendeten Bedienelementen nicht über die Menüleisten hinaus.

Begrenzte Thumbnail-Listen bieten beim Scrollen rechts einen ziehbaren Scrollgriff;
der Fotostream ist ausgenommen. Seitensprünge laden nur die benötigte Zielseite.

Ab BearStack **1.15.0** unterstützt die App Bildgruppen: Erstellen mit Hauptbildwahl,
Erweitern über die Mehrfachauswahl und Verwalten im Infopanel. Dafür ist `photos.edit`
erforderlich; Lesen und Ordnersprung benötigen `photos.read`. Der Ordnerlink springt
aus Fotostream oder Personenordner direkt zum Bild. Gerätefotos bieten diese Aktionen nicht.

Mehrfachauswahl per langem Drücken unterstützt Teilen, Speichern von Servermedien
und bestätigtes Löschen lokaler Bilder. Serveroriginale bleiben schreibgeschützt.

Nach dem Teilen eines Gerätefotos bleibt die Großansicht geöffnet. Beim Schließen
kehrt die Galerie zum aktuellen Bild zurück; unveränderte lokale Medien behalten
auch nach einem App-Wechsel ihre Rasterposition.

Die Personenverwaltung verwendet getrennte Zustände für Personenliste, Gruppenvergleich
und Ordnerprüfung sowie einen eindeutigen Navigationszustand. Die Ordnerprüfung hat
einen eigenen Controller; dauerhafte Aktionen und Wiederholungen behalten den
gemeinsamen Schreibzugriff.

## Dokumentation

Die Benutzeranleitung wird an einer Stelle gepflegt und daraus für die Website gebaut:

- [Anleitung: von der Einrichtung bis zur Personenverwaltung](../../_site-src/docs/android.md)
- [Technik & Entwicklung: Verbindung, API, Datenhaltung, Build und Tests](../../_site-src/docs/android-technik.md)
- [Versionshistorie](../../CHANGELOG.md)
- [HTTP-Vertrag](../../openapi.yaml)

Für Serverfotos sind BearStack ab 0.50.0, ein aktiviertes Fotomodul und `photos.read`
erforderlich; Personenverwaltung benötigt zusätzlich `photos.edit`. Gerätefotos
lassen sich auch ohne Server verwenden. **Zuordnungen nach Ordner prüfen** einschließlich
Gesichtsvergrößerung und Sammelaktionen benötigt BearStack ab **1.2.0**. Die Anleitung enthält die
[Kompatibilitätstabelle](../../_site-src/docs/android.md#serverkompatibilitat).

## Entwicklungsstart

`apps/android/` als eigenes Projekt in Android Studio öffnen. JDK 17 oder 21 und
Android-SDK-Plattform 37 bereitstellen; den SDK-Pfad über `ANDROID_HOME` oder eine
ignorierte `local.properties` mit `sdk.dir=…` angeben. Den mitgelieferten Gradle
Wrapper verwenden.

Vom Repository-Stamm aus:

```sh
# JVM-Tests, Lint und Debug-APK
./scripts/check-android.sh

# Auf dem verbundenen Testgerät installieren
adb install -r apps/android/app/build/outputs/apk/debug/app-debug.apk

# Geräte- und Integrationstests auf einem gestarteten Emulator
apps/android/gradlew -p apps/android :app:connectedDebugAndroidTest
```

Ein reiner Build ist mit `apps/android/gradlew -p apps/android :app:assembleDebug`
möglich. Go-, Web- und Docker-Builds benötigen kein Android-SDK. APKs werden lokal
gebaut und nicht automatisch veröffentlicht.

Für die private Verteilung einen dauerhaften Signierschlüssel verwenden:
[Release signieren](../../_site-src/docs/android-technik.md#privaten-release-signieren).
Die weiteren [Testziele](../../_site-src/docs/android-technik.md#tests) umfassen
Release-Smoke und einen isolierten Go-HTTPS-Server. Die technischen Abläufe und
Grenzen stehen in der Referenz und werden hier nicht parallel dokumentiert.
