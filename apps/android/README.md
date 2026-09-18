# BearStack Fotos für Android

Native Android-App für BearStack-Galerie, lokale Fotos und Personenverwaltung.
App-Version **0.19.0** (`versionCode 40`), Android **8.0 oder neuer**.

## Dokumentation

Die Benutzeranleitung wird an einer Stelle gepflegt und daraus für die Website gebaut:

- [Anleitung: von der Einrichtung bis zur Personenverwaltung](../../_site-src/docs/android.md)
- [Technik & Entwicklung: Verbindung, API, Datenhaltung, Build und Tests](../../_site-src/docs/android-technik.md)
- [Versionshistorie](../../CHANGELOG.md)
- [HTTP-Vertrag](../../openapi.yaml)

Für Serverfotos sind BearStack ab 0.50.0, ein aktiviertes Fotomodul und `photos.read`
erforderlich; Personenverwaltung benötigt zusätzlich `photos.edit`. Gerätefotos
lassen sich auch ohne Server verwenden. Die Anleitung enthält die
[Kompatibilitätstabelle](../../_site-src/docs/android.md#serverkompatibilitat).

## Entwicklungsstart

`apps/android/` als eigenes Projekt in Android Studio öffnen. JDK 17 oder 21 und
Android-SDK-Plattform 36 bereitstellen; den SDK-Pfad über `ANDROID_HOME` oder eine
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
