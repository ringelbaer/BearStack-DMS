---
title: Android-App – Anleitung
description: BearStack Fotos einrichten, Bilder ansehen und Personen Schritt für Schritt verwalten.
---

# BearStack Fotos für Android

Mit BearStack Fotos kannst du deine Sammlung auf dem BearStack-Server ansehen,
Bilder suchen und teilen sowie erkannte Personen benennen und zuordnen. Du kannst
auch ausschließlich die Fotos auf deinem Android-Gerät öffnen.

Diese Anleitung beschreibt **Android-App 0.22.1**. Beginne mit den
[ersten Schritten](#erste-schritte); die folgenden Kapitel erklären die einzelnen
Arbeitsabläufe. Für Installation aus dem Quellcode, API und Speicherverwaltung gibt
es die [technische Referenz](android-technik.md).

## Erste Schritte

### Was du brauchst

- Ein Gerät mit **Android 8.0 oder neuer** und die BearStack-Fotos-APK.
- Für Serverfotos: die **HTTPS-Adresse**, einen Benutzernamen und das zugehörige
  Passwort. Die Adresse kann einen Reverse-Proxy-Pfad enthalten, etwa
  `https://fotos.example.org/bearstack/`.
- Einen BearStack-Server ab **0.50.0** mit aktiviertem Fotomodul und dem Recht
  **Fotos lesen** (`photos.read`). Für die Personenverwaltung brauchst du zusätzlich
  **Fotos bearbeiten** (`photos.edit`).

Bitte die Person, die deinen Server betreibt, um APK, Adresse und Zugangsdaten.
Die APK wird derzeit lokal gebaut; es gibt keine automatische Veröffentlichung.
Wenn du sie selbst bauen möchtest, folge [Bauen und installieren](android-technik.md#bauen-und-installieren).
Für lokale Gerätefotos brauchst du weder einen Server noch ein BearStack-Konto.

### App installieren und verbinden

1. Öffne die bereitgestellte APK auf deinem Android-Gerät und führe die Installation
   aus. Falls Android danach fragt, erlaube die Installation für die verwendete Quelle.
2. Starte **BearStack Fotos**.
3. Trage **HTTPS-Adresse**, **Benutzername** und **Passwort** ein und tippe auf **Verbinden**.
   Verwende die endgültige Serveradresse: Die App folgt keinen Weiterleitungen und
   unterstützt keine unverschlüsselten HTTP-Adressen.
4. Bei einem selbstsignierten Zertifikat erscheint eine zusätzliche Prüfung.
   Vergleiche den angezeigten SHA-256-Fingerabdruck mit dem Fingerabdruck, den dir
   der Serverbetreiber nennt. Erst danach wählst du **Abgeglichen und vertrauen**.
   [Details zur Zertifikatsprüfung](android-technik.md#verbindung-und-zertifikat).
5. Nach erfolgreicher Anmeldung öffnet sich auf einem passenden Server die Galerie.
   Tippe auf ein Foto, um es groß anzusehen; mit Android-Zurück gelangst du ins Raster.

Die App merkt sich eine Verbindung und meldet sich beim nächsten Start automatisch
an. Eine andere Adresse oder ein anderes Konto richtest du in der Galerie unter
**… → Einstellungen → Verbindung wechseln** ein.

### Ohne Server beginnen

Tippe auf dem Anmelde- oder Ladebildschirm auf **Lokale Fotos öffnen**. Android
fragt beim ersten Zugriff nach einer Fotofreigabe. Anschließend kannst du unter
**Dieses Gerät** einen Fotoordner öffnen. Die Einzelheiten stehen unter
[Lokale Fotoordner](#lokale-fotoordner).

## Orientierung in der App

Die schwebende Leiste am unteren Rand wechselt zwischen den drei Galerieansichten:

| Bereich | Dafür verwendest du ihn |
| --- | --- |
| **Fotos** | Den gesamten Serverbestand nach Datum durchsehen. |
| **Ordner** | In der Verzeichnisstruktur, in Personengalerien oder unter **Dieses Gerät** navigieren. |
| **Suchen** | Serverfotos und Ordner anhand von Suchbegriffen finden. |

Listen laden beim Scrollen automatisch nach. Bei einem Ladefehler steht
**Erneut versuchen** direkt im betroffenen Bereich. Nach dem Schließen eines Fotos
zeigt das Raster das zuletzt betrachtete Bild wieder an.

Die Symbole haben unterschiedliche Aufgaben:

| Symbol oder Menü | Bedeutung |
| --- | --- |
| **Sortieren** | Reihenfolge der aktuellen Galerie oder Personenübersicht wählen. |
| **…** | Aktionen für die aktuelle Ansicht, etwa Karte, Fotoframe, Personenverwaltung oder Einstellungen. |
| **Zahnrad** im Bildbetrachter oder Fotoframe | Anzeigedauer, Wiederholung und Wiedergabedarstellung einstellen. |
| **… → Personen verwalten** in der Galerie | In den Bearbeitungsbereich zum Benennen und Zuordnen wechseln. |
| **… → Personenliste** im Bearbeitungsbereich | Bereits benannte Personen suchen und bearbeiten. |

Der Inhalt von **…** hängt von der Ansicht und deinen Rechten ab. In der
Personenliste und in einer geöffneten Person liegen auch **Zurück**, **Hilfe** und
**Verbindung wechseln** in diesem Menü. Beim Benennen hat die untere Aktionsleiste
links **…** für Gruppenaktionen, mittig **?** für Hilfe und rechts den **Stift**.

Die App folgt dem hellen oder dunklen Android-Design und unterstützt Deutsch und
Englisch. Auf Android 13 oder neuer kannst du die Sprache unter
**Android-Einstellungen → Apps → BearStack Fotos → Sprache** einzeln wählen.
Auf älteren Geräten gilt die Systemsprache. Große Schrift und TalkBack werden
unterstützt; längere Inhalte und Dialoge lassen sich scrollen.

## Galerie

### Fotos finden und sortieren

Öffne **Fotos** für den zeitlichen Überblick oder **Ordner** für einen bestimmten
Teil deiner Sammlung. Ordnerkacheln zeigen Vorschauen und die Medienanzahl.
Unter **Geschichten & Notizen** findest du vorhandene Markdown- und Textbeiträge;
der vollständige Text wird beim Öffnen geladen.

Unter **Suchen** gibst du einen Ausdruck ein und startest ihn mit der Suchen-Taste
der Tastatur. Es gilt dieselbe [Fotosuche wie im Browser](fotos.md#fotos-suchen-und-filtern), beispielsweise:

| Suchausdruck | Ergebnis |
| --- | --- |
| `person:Anna` | Fotos mit der Person Anna. |
| `tag:Urlaub` | Fotos und passende Ordner mit dem Tag Urlaub. |
| `file_name:IMG_1234` | Medien mit passendem Dateinamen. |

Auch große Ordnersuchergebnisse werden beim Scrollen weitergeladen. Meldet der
Server eine zu breite Suche, grenze den Suchausdruck weiter ein.

Über **Sortieren** wählst du Datum auf- oder absteigend. Normale Ordner bieten
zusätzlich Name auf- oder absteigend. In Personengalerien werden Fotos nach Datum
sortiert; Personenübersichten bieten Name und auf passenden Servern **Anzahl Bilder**.
Die Bildanzahl zählt unterschiedliche sichtbare Fotos, nicht einzelne Gesichtsausschnitte.

### Zu einem Datum springen

1. Öffne **Fotos** in der Reihenfolge **Datum: Neueste zuerst**.
2. Tippe auf das **Kalendersymbol links neben …**.
3. Wähle einen Tag. Du kannst auch die Jahresauswahl oder die Texteingabe verwenden.
4. Bestätige mit **Zum Datum springen**.

Die Galerie springt zum Anfang dieses Tages. Gibt es dort keine Aufnahme, wählt sie
den zeitlich nächsten vorhandenen Tag, bei gleichem Abstand den älteren. Der Sprung
setzt keinen Filter: Du kannst anschließend in beide Richtungen weiterscrollen.
Maßgeblich ist der Aufnahmetag mit gespeicherter Zeitzone, ersatzweise die Dateiänderung.
Ordner, lokale Fotos und Suchergebnisse haben keinen solchen Datumssprung.

### Bilder groß ansehen und Informationen öffnen

Tippe auf ein Foto und wische zum nächsten oder vorherigen Bild. Alternativ nutzt
du die Vor-/Zurück-Schaltflächen. Zwei Finger vergrößern oder verkleinern das Bild;
ein vergrößertes Bild lässt sich verschieben.

Ein einfacher Tipp auf das Foto blendet Bedien- und Systemleisten aus, ein weiterer
Tipp blendet sie wieder ein. Das gilt auch beim Bildwechsel. TalkBack bietet eigene
Aktionen für Zoom, Zurücksetzen und das Ein-/Ausblenden der Bedienelemente.

Bei sichtbaren Bedienelementen bleiben die Status- und Navigationsbereiche der
Großansicht deckend dunkel. Das Foto scheint dort auch im Querformat nicht durch.

**Informationen** zeigt die vorhandenen Daten: Dateiname und Pfad, Auflösung,
Dateigröße, Kamera, Aufnahmezeit samt Zeitzone, Bewertung, Personen, Tags,
Schlagwörter und GPS-Position. Fehlt die Aufnahmezeit, ist die ersatzweise angezeigte
Änderungszeit entsprechend bezeichnet. Ein Fehler beim Laden lässt sich im
Informationsblatt über **Erneut versuchen** wiederholen.

### Schnell durch Listen scrollen

Beim Scrollen durch eine begrenzte Thumbnail-Liste erscheint rechts ein Griff mit
zwei Pfeilen. Ziehe ihn nach oben oder unten, um schnell zu einer anderen Stelle
zu springen. Nach kurzer Ruhe verschwindet er wieder. Das gilt für Ordner,
Suchergebnisse, Karten-Treffer und **Dieses Gerät**. Im unbegrenzten Fotostream
unter **Fotos** wird er nicht angezeigt. TalkBack kann die Position am Griff
ebenfalls verändern.

### Mehrfachauswahl

Drücke ein Medium im Raster lange, um die Mehrfachauswahl zu aktivieren. Kurze
Tipps markieren weitere Medien oder entfernen ihre Markierung. Die Auswahl bleibt
beim Scrollen und Nachladen erhalten; maximal 100 Medien sind gleichzeitig
auswählbar. **Auswahl beenden** oder Zurück beendet den Modus. Ein Ordner-, Such-
oder Sortierwechsel verwirft die Auswahl. Das funktioniert auch in Karten-Trefferlisten.

- **Teilen:** Öffnet die Android-App-Auswahl mit allen markierten Medien. Lokale
  Bilder werden direkt freigegeben; Serveroriginale werden nacheinander vorbereitet.
- **Auswahl speichern** bei Servermedien: Wähle einen Zielordner im Android-Dialog.
  Die Originale werden nacheinander als neue Dateien gespeichert. Bei Fehlern oder
  Abbruch bleiben bereits vollständig gespeicherte Dateien erhalten; die gerade
  geschriebene unvollständige Datei wird entfernt.
- **Löschen** unter **Dieses Gerät**: Nach deiner Bestätigung fordert Android bei
  Bedarf die Freigabe an. Auf Android 10 kann dies für jedes Bild einzeln nötig
  sein; ältere Versionen benötigen Schreibzugriff. Die Löschung ist dauerhaft.
  Bei Abbruch können bereits bestätigte Einzelbilder gelöscht sein. Einzelne
  lokale Bilder lassen sich auch direkt in der Großansicht löschen.

Serveroriginale bleiben schreibgeschützt; dort wird keine Löschaktion angeboten.
Das Teilen ist auf 256 MiB pro Serverdatei und 512 MiB insgesamt begrenzt.

### Einzelne Bilder teilen

Tippe im Vollbildbetrachter auf **Teilen** und wähle die empfangende Android-App.
Geteilt wird die Originaldatei des aktuellen Fotos. Bei Serverfotos lädt BearStack
sie zunächst herunter; diese Vorbereitung lässt sich abbrechen. Lokale Fotos
werden direkt freigegeben. Die Diashow pausiert währenddessen.

Nach dem Teilen oder Abbrechen der App-Auswahl bleibt das aktuelle Foto in der
Großansicht geöffnet, auch unter **Dieses Gerät**. Beim Schließen kehrst du im
Raster zum aktuellen Bild zurück. Das gilt, solange Medienbestand und Fotozugriff
unverändert bleiben.

Die empfangende App bekommt vorübergehenden Lesezugriff auf genau dieses Bild.
Zugangsdaten und Serverlinks werden nicht weitergegeben. Im Original gespeicherte
Metadaten bleiben erhalten, gegebenenfalls einschließlich GPS.
Serverbilder dürfen zum Teilen bis zu **256 MiB** groß sein. Größere Dateien kannst
du über **Original herunterladen** speichern.

### Original speichern

Die Bestätigung nach einem erfolgreichen Download verschwindet automatisch nach
fünf Sekunden; ein längeres Zeitlimit deiner Android-Bedienungshilfen wird beachtet.

Wähle im Vollbild **Original herunterladen**. Im Android-Speicherdialog bestimmst
du Zielordner und Dateinamen. Der Transfer zeigt seinen Fortschritt und lässt sich
abbrechen. Bei Fehler oder Abbruch versucht die App, die angefangene Datei wieder
zu entfernen. Eine allgemeine Speicherfreigabe ist dafür nicht erforderlich.

### Diashow und Fotoframe

Für eine Diashow öffnest du ein Foto und tippst auf die Wiedergabetaste. Über das
**Zahnrad** öffnest du **Diashow einstellen**. Dort legst du eine Anzeigedauer von **3 bis 300 Sekunden**
und **Am Ende wiederholen** fest. Die Zeit beginnt erst, wenn das Foto geladen ist.

Für einen Bilderrahmen öffnest du in der Galerie **… → Fotoframe starten**. Der
Fotoframe verwendet den aktuellen Ordner mit Unterordnern und berücksichtigt die
aktive Suche beziehungsweise Typauswahl. Antippen blendet die Steuerung ein;
Android-Zurück stellt die vorherige Galerie wieder her.

Über das **Zahnrad** öffnest du **Fotoframe einstellen**. Dort kannst du zusätzlich festlegen:

| Einstellung | Wirkung |
| --- | --- |
| **Zufällige Reihenfolge** | Spielt Server- oder Gerätefotos gemischt ab. Die Einstellung ist zunächst ausgeschaltet. |
| **Bildschirm füllen (mit Beschnitt)** | Das Bild füllt den Bildschirm; dabei können Ränder abgeschnitten werden. |
| **Name und Datum anzeigen** | Die Beschriftung wird eingeblendet. |
| **Dateiname / Ordnername** | Wählt den Inhalt der Namenszeile. Der Ordnername wird nach den Galerieregeln formatiert. |

**Zufällige Reihenfolge** wird beim Speichern übernommen und startet den Fotoframe
von vorn. Ausschalten verwendet wieder die normale Wiedergabereihenfolge. Ordner,
Unterordner, Such- und Typfilter bleiben erhalten. Solange sich der Bestand nicht
ändert, werden in einem Durchlauf alle Medien einmal abgespielt. Die Reihenfolge
bleibt beim Nachladen und Wiederholen stabil; Serverfotos verwenden die feste
Zufallssortierung des Index, Gerätefotos werden bei jedem neuen Fotoframe-Start
neu gemischt. Serverfotos benötigen dafür **BearStack 0.69.0** oder neuer; auf
älteren Servern ist der Schalter mit einem Hinweis deaktiviert. Gerätefotos
benötigen keinen Server.

Die Auswahl bleibt nach einem Neustart erhalten. Der formatierte Serverordnername
benötigt BearStack **0.68.0** oder neuer. Auf älteren Servern zeigt diese Einstellung
„Ordnername nicht verfügbar“; du kannst weiter den Dateinamen verwenden. Lokale
Fotos verwenden den Anzeigenamen ihres Gerätealbums.

Videos und Audio werden im Serverbetrachter und Fotoframe bis zum Ende abgespielt.
Erst danach wechselt die automatische Wiedergabe weiter. Wiederholung funktioniert
auch bei einer einzelnen Datei. Informationen, Einstellungen, Fotozoom oder ein
Wechsel in eine andere App pausieren die automatische Wiedergabe. Während der
aktiven Wiedergabe bleibt der Bildschirm eingeschaltet.

### Karten

Öffne **… → Karte**, um GPS-Aufnahmen des aktuellen Ordners einschließlich
Unterordnern oder der aktuellen Suche auf einer Karte zu sehen. Verschieben,
Zwei-Finger-Zoom, Plus/Minus und **Alle Orte anzeigen** ändern den Ausschnitt.

Eine Zahl bündelt mehrere benachbarte Aufnahmen. Tippe sie an, um näher heranzugehen.
Ein einzelner Punkt öffnet das Foto. Aufnahmen am gleichen Ort oder bei maximalem
Zoom erscheinen in einer Fotoauswahl, die du wie die Galerie durchblättern kannst.
Auch die **Informationen** eines Fotos enthalten bei vorhandenen GPS-Daten eine Karte.

Über **Ebenen** blendest du weitere Informationen ein:

- **Fotoroute** verbindet Fotoorte in zeitlicher Reihenfolge als gestrichelte Linie.
  Die aktuelle Suche bleibt berücksichtigt. Lange Routen können vereinfacht sein.
- **GPX-Tracks** öffnet eine Auswahl vorhandener Tracks. Wähle einen oder mehrere
  Tracks; die Karte fokussiert den gewählten Track. Erneutes Antippen oder
  **Alle GPX-Tracks ausblenden** entfernt die Auswahl. Tracks funktionieren auch ohne
  GPS-Fotos. In einem Ordner stammen sie aus dessen Unterbaum, bei einer Fotosuche
  aus der gesamten Sammlung.

Beim Vergrößern werden bei Bedarf mehr Details geladen. Unlesbare Tracks und
Ladefehler werden in der Auswahl angezeigt und lassen sich erneut laden.
Karten benötigen einen erreichbaren Server und Kartenbilder von OpenStreetMap.
Für lokale Gerätefotos ist diese Kartenansicht nicht verfügbar.

### Personen unter Ordner ansehen

Zum reinen Ansehen genügt **Fotos lesen**. Öffne **Ordner → Personen** und dann
**Alle** oder einen angebotenen Foto-Tag. Eine Person öffnet ihre Fotos als Galerie.
Die Personenkacheln zeigen bevorzugt ein favorisiertes Gesicht. Foto-Tags werden
in der Web-Personenverwaltung vergeben.

In einem normalen Serverordner führt **… → Personen im Ordner** zu den Personen
aus diesem Ordner und seinen Unterordnern. Auch die anschließend geöffneten Fotos
bleiben auf diesen Bereich beschränkt. Zurück führt über die Personenübersicht
wieder zum ursprünglichen Ordner. Die Einträge erscheinen nur, wenn der Server
sie unterstützt; die [Kompatibilitätstabelle](#serverkompatibilitat) nennt die Mindeststände.

## Lokale Fotoordner

### Gerätefotos freigeben

Du erreichst lokale Fotos direkt beim App-Start über **Lokale Fotos öffnen**.
In einer verbundenen Galerie aktivierst du unter
**… → Einstellungen → Lokale Fotoordner anzeigen** die Kachel
**Ordner → Dieses Gerät**. Diese Einstellung ist standardmäßig ausgeschaltet.

1. Öffne **Dieses Gerät** und erlaube den von Android angefragten Fotozugriff.
2. Auf Android 14 oder neuer kannst du auch nur ausgewählte Fotos freigeben.
   Dann erscheinen ausschließlich diese Bilder und ihre Ordner.
3. Öffne einen Ordner, beispielsweise Camera oder Screenshots, und tippe auf ein Foto.

Die Auswahl änderst du über **Fotozugriff erlauben / Auswahl ändern** in den
App-Einstellungen. Bei dauerhaft verweigertem Zugriff führt
**Android-App-Einstellungen** zur Freigabe. Das Ausschalten der lokalen Kachel
blendet sie aus; eine Android-Berechtigung entziehst du separat in den Systemeinstellungen.

### Was lokal verfügbar ist

Lokale Fotos unterstützen Raster, Vollbild, Zoom, Informationen, Teilen, Diashow
und Fotoframe in einem geöffneten Fotoordner sowie bestätigtes Löschen. Es gibt keinen Upload. Videos,
private App-Dateien und Ordner außerhalb des Android-Medienindex gehören nicht zu
dieser Ansicht. SD-Karten-Ordner erscheinen, soweit Android sie bereitstellt;
gleichnamige Ordner verschiedener Datenträger bleiben getrennt.

Die zuletzt in den Android-Medienindex aufgenommenen Fotos stehen zuerst. Das
entspricht den Ordnervorschauen und kann von der Reihenfolge nach Aufnahmedatum
abweichen. Nach Änderungen am Medienbestand oder den Freigaben aktualisiert sich
die Ansicht; ein inzwischen unzugängliches Foto wird geschlossen.

Bei aktiver Verbindung zeigen **Fotos** und **Suchen** weiterhin den Serverbestand.
Ohne Server bleiben die lokalen Ordner benutzbar; die Statusleiste bietet eine
erneute Anmeldung an.

## Personen verwalten

### Gruppe, Gesicht und Foto unterscheiden

Die Bearbeitung arbeitet mit **Gesichtern**, die der Server erkannt und zu
**Personengruppen** zusammengefasst hat. Ein Foto kann mehrere Gesichter enthalten.
Eine Gruppe kann Gesichter aus vielen Fotos enthalten und zunächst unbenannt sein.
Benennen, Zuordnen oder Ignorieren ändert die Gesichtsdaten; es löscht keine Fotos.

Du brauchst **Fotos bearbeiten**. Öffne in der Galerie **… → Personen verwalten**.
Für die Arbeit an bereits benannten Personen wechselst du dort über
**… → Personenliste**. Unter **… → Fotos** gelangst du zur Galerie zurück.

### Zuordnungsmodus

Die Ansicht **Personen benennen** zeigt bis zu vier Gesichtsausschnitte der aktuellen
Gruppe. Bei größeren Gruppen blättern **Zurück** und **Weiter** durch die Bilder.
Eine Benennung, Zuordnung oder Gruppen-Ignorieraktion betrifft die gesamte Gruppe,
auch die gerade nicht sichtbaren Gesichter.

1. Prüfe die Gesichter. Halte bei Bedarf ein Portrait, um das zugehörige ganze Foto
   zu sehen; die [Fotovorschau](#groe-fotovorschau-beim-halten-eines-gesichts) ist unten erklärt.
2. Tippe auf den **Stift** und gib einen Namen ein.
3. Wähle einen Namensvorschlag, um die Gruppe einer vorhandenen Person zuzuordnen.
   Mit **Speichern** benennst du die Gruppe neu. Bei einem bereits vorhandenen Namen
   unterscheidet der Dialog zwischen Zuordnen und **Separat benennen**.
4. Nach Serverbestätigung erscheint die nächste Gruppe.

Wenn du eine Gruppe noch nicht sicher einordnen kannst, nutze diese Aktionen:

| Geste in der Bearbeitungsansicht | Entsprechende Gruppenaktion |
| --- | --- |
| Nach links wischen | Gruppe für diesen Durchgang **überspringen**. |
| Nach rechts wischen | Das letzte Überspringen zurücknehmen. |
| Nach oben wischen | Die ganze Gruppe **ignorieren**, mit kurzer Rücknahmefrist. |

Dieselben Aktionen stehen unter **…** in der unteren Leiste. Bei langem Inhalt
scrollt Hochwischen zunächst zum Ende; erst ein weiterer Wischer ignoriert die
Gruppe. Gesten im Namensdialog oder in der Fotovorschau lösen keine Gruppenaktion aus.

Nach dem Ignorieren bietet die Meldung **Rückgängig** für **fünf Sekunden** eine
Rücknahme an. Bei mehreren rasch ignorierten Gruppen gilt die Frist je Gruppe;
die Meldung nimmt die letzte noch rücknehmbare Aktion zurück. Noch nicht gesendete
Ignorieraktionen werden nach einem Hintergrundwechsel oder Neustart wieder angeboten.

Das **×** an einem Gesicht trennt nur dieses Gesicht in eine neue unbenannte Gruppe
ab. Sie folgt nach Abschluss der aktuellen Gruppe. Bei einem einzigen Gesicht
entfällt hier das ×. Ein neuer Durchgang übernimmt inzwischen hinzugekommene
Gruppen; **Übersprungene bearbeiten** nimmt am Ende die zuvor ausgelassenen Gruppen
gezielt wieder auf.

### Große Fotovorschau beim Halten eines Gesichts

Halte ein Portrait kurz gedrückt, um das ganze Foto mit markiertem Gesicht zu sehen.
Solange du hältst, vergrößert Wischen nach unten zum Gesicht; Wischen nach oben
verkleinert wieder. Loslassen schließt die Vorschau. Diese Bedienung gilt beim
Benennen, in der Personenliste und unter **Ähnliche Gruppen**.

TalkBack bietet **Originalfoto anzeigen**. Die Vorschau bleibt dann geöffnet, bis
du **Vorschau schließen** verwendest; **Zum Gesicht vergrößern** und
**Ganzes Foto anzeigen** ersetzen die Wischgesten. Der angezeigte Pfad hilft dir,
das Bild in der Sammlung wiederzufinden. Die Vorschau verwendet auf aktuellen
Servern ein verkleinertes Bild; zum Speichern des Originals nutze den Galeriebetrachter.

### Gesichtssuche beim Benennen

Die **Lupe neben dem Namensfeld** vergleicht das Ausgangsgesicht mit bereits
benannten Personen. Treffer erscheinen mit Portrait und Namen und lassen sich
bereits während des Abgleichs auswählen. Ein Stern-Favorit wird bevorzugt als
Portrait verwendet. Die Suche allein ändert keine Zuordnung.

Tippe einen Treffer an, um die Gruppe dieser Person zuzuordnen. Fehlt ein passender
Treffer, kannst du weiter den Namen eingeben oder den Abgleich erneut starten.
Tippen im Namensfeld, Schließen des Dialogs oder ein Hintergrundwechsel bricht eine
laufende Suche ab. Beim bloßen Umbenennen einer schon benannten Person gibt es
keinen Gesichtsabgleich.

### Benannte Personen bearbeiten

Öffne **… → Personenliste**, suche nach einem Namen und tippe auf die Person.
Die Suche berücksichtigt den gesamten Serverbestand, auch noch nicht geladene
Einträge. Beim Scrollen werden weitere Personen beziehungsweise Gesichter geladen.
**Aktualisieren** übernimmt neu hinzugekommene Personen.

| Aktion | Ergebnis |
| --- | --- |
| **Person umbenennen** | Ändert den Namen der ganzen Person. Ein gleicher vorhandener Name führt nicht automatisch zu einer Zusammenführung. |
| **× / Zuordnung entfernen** | Verschiebt nach Bestätigung genau dieses Gesicht in eine neue unbenannte Gruppe und hebt seine Favorisierung auf. |
| **☆ / ★** | Markiert ein Gesicht als Vergleichsfavorit oder nimmt die Markierung zurück. |
| **Galeriesuche im Browser** | Öffnet eine Suche nach dem Personennamen. Der Browser benötigt seine eigene Anmeldung; gleichnamige Personen können gemeinsam erscheinen. |

Wird das letzte Gesicht entfernt, verschwindet die leere Person aus der Liste.
Bestätigte Änderungen werden nach der Serverantwort angezeigt. Unter **… → Hilfe**
findest du die Bedienhinweise auch direkt in der Personenliste und in einer geöffneten Person.

### Ordner-Pfade prüfen

Öffne eine Person und wähle **… → Ordner-Pfade prüfen**. Der Eintrag ist auch beim
Benennen einer unbenannten Gruppe verfügbar. Benötigt werden BearStack **1.2.0**
oder neuer und **Fotos bearbeiten**; bei älteren Servern ist der Eintrag deaktiviert.

Jeder vollständige, wie in der Galerie formatierte Ordnerpfad erscheint einmal.
Die Ansicht enthält **40 Ordner pro Seite** und höchstens **acht Gesichtsvorschauen
je Ordner**. Wische die Vorschauen seitlich. Halte ein Gesicht gedrückt und ziehe
zum Vergrößern wie gewohnt nach oben oder unten. Die Originalansicht zeigt den
Fotopfad und den passenden Gesichtsrahmen; TalkBack bietet **Originalfoto anzeigen**.

| Aktion | Wirkung |
| --- | --- |
| **Alle neu zuweisen** | Öffnet den gemeinsamen Namensdialog mit Suche nach vorhandenen Personen, Gesichtssuche und Eingabe eines neuen eindeutigen Namens. |
| **Alle auf unbenannt setzen** | Verschiebt die Gesichter gemeinsam in eine neue unbenannte Gruppe und entfernt ihre Vergleichsfavoriten. |
| **Alle ignorieren** | Ignoriert alle aktiven Gesichter dieser Person im Ordner. |
| **Pfad ausschließen und Gesichter auf unbenannt setzen** | Setzt die Gesichter unbenannt und sperrt neue manuelle sowie automatische Zuordnungen zu dieser Person im Ordner. |
| **Pfad wieder freigeben** | Hebt die Sperre auf; bestehende unbenannte Gesichter werden nicht automatisch zurückverschoben. |

Alle Aktionen gelten für **sämtliche aktiven Gesichter der Person im exakten
Ordner**, auch bei mehr als acht Vorschauen oder 500 Gesichtern. Unterordner und
andere Personen bleiben unverändert. Ausschlüsse bleiben auch ohne aktive Gesichter erreichbar. Benannte Personen
mit ausschließlich gesperrten Pfaden stehen weiter in der Personenliste als
**Ausgeschlossene Pfade** und öffnen direkt die Ordnerprüfung. Änderungen verlangen eine
Bestätigung beziehungsweise die ausdrückliche Zielauswahl. Bei einem
Revisionskonflikt wird die Liste aktualisiert und eine neue Entscheidung benötigt.
Nach einem Verbindungsabbruch löst **Offene Aktion prüfen** die gespeicherte Aktion
über ihre Quittung auf. **Zurück** aktualisiert die vorherige Personenansicht.

### Mehrfachauswahl in „Benannte Personen“

1. Öffne eine Person und wähle Gesichter über die Checkbox oben links in ihren Kacheln.
   Die Auswahl bleibt beim Nachladen erhalten; höchstens **500 Gesichter pro Aktion**
   sind möglich.
2. Öffne **…** rechts in der Auswahlleiste.
3. Wähle die gewünschte Aktion und prüfe gegebenenfalls den Bestätigungsdialog.

| Aktion | Wirkung auf die ausgewählten Gesichter |
| --- | --- |
| **Auf unbenannt zurücksetzen** | Bildet gemeinsam eine neue unbenannte Gruppe und entfernt ihre Vergleichsfavoriten. |
| **Gruppe zuordnen** | Ordnet sie über den Namensdialog einer neuen oder vorhandenen Person zu. Die Lupe verwendet das erste ausgewählte Gesicht. |
| **Ignorieren** | Entfernt sie nach Bestätigung aus der Personenansicht. Im Browser unter **Ignoriert** lassen sie sich wiederherstellen. |

Während der Auswahl sind ×, Vergleichsstern und Umbenennen der ganzen Person
gesperrt. **Aufheben** leert die Auswahl; Android-Zurück hebt sie zunächst auf.
Das Abbrechen eines Dialogs erhält die Auswahl. Hat sich die Person zwischenzeitlich
geändert, wird sie aktualisiert und du musst die Auswahl neu treffen.

### Ähnliche Gruppen

Öffne im Personenbereich **… → Ähnliche Gruppen**. Die Ansicht zeigt ein Paar
möglicherweise zusammengehöriger Gruppen mit je einem Vergleichsgesicht,
Ordnerangabe, Namen und Gesichtsanzahl. Der Ähnlichkeitswert ist ein Vergleichswert,
keine Prozentwahrscheinlichkeit. Prüfe bei Bedarf beide Portraits in der Fotovorschau.

- **Zusammenführen** ordnet die Gesichter der ersten Gruppe der zweiten zu.
  Favoriten bleiben erhalten. Ist die zweite unbenannt, wird ein vorhandener Name
  der ersten übernommen.
- Sind beide Gruppen benannt, musst du **Trotzdem zusammenführen** zusätzlich
  bestätigen. Dabei bleibt der Name der zweiten Gruppe erhalten.
- **Getrennt lassen** speichert die Trennung und verhindert auch künftige
  automatische Zuordnungen zwischen diesen Gruppen.

Nach einer bestätigten Entscheidung erscheint das nächste Paar. Ein Personenname
öffnet die zugehörige Detailansicht; die Benennungswarteschlange bleibt erhalten.

**Nur eine Seite bearbeiten:** Unbenannte Gruppen haben eigene Buttons
**Ignorieren** und **Benennen/Zuordnen**. Sie betreffen die ganze Gruppe dieser Seite.
Solange eine unbenannte, unbearbeitete Seite übrig ist, bleibt sie bedienbar.
**Weiter** überspringt dann das Paar, ohne eine dauerhafte Trennung zu speichern.
Sobald beide Seiten bearbeitet oder benannt sind, erscheint das nächste Paar.

**Beide unbenannten Gruppen gemeinsam benennen:** Der gemeinsame Stift öffnet den
Namensdialog für beide Gruppen. Zusätzlich sucht die App automatisch nach einer
passenden benannten Person: zuerst anhand der ersten Gruppe, bei leerem Endergebnis
anhand der zweiten. Ein Treffer unter **Benennungsvorschläge für beide Gruppen**
ordnet beide Gruppen gemeinsam zu. **Abbrechen** im Dialog verändert nichts;
**Erneut abgleichen** startet die Suche erneut. Nach einer Einzelaktion entfällt
die gemeinsame Trefferliste.

### Offene Aktionen und geänderte Fotos

Bei einer verlorenen Serverantwort kann eine Änderung bereits gespeichert sein.
Die App sperrt deshalb weitere Entscheidungen und bietet **Offene Aktion prüfen**
an. Damit klärst du den Ausgang, auch nach einem App-Neustart, ohne dieselbe Aktion
doppelt auszuführen. Wenn nur das Laden der nächsten Gruppe fehlschlägt, wiederholt
**Erneut versuchen** ausschließlich diesen Abruf.

Hat die Weboberfläche oder ein Hintergrundlauf inzwischen eine Gruppe verändert,
lädt die App den neuen Stand. Du musst erneut entscheiden; eine alte Entscheidung
wird nicht automatisch auf die geänderte Gruppe übertragen.

**Foto geändert · im Web prüfen** bedeutet, dass das Original zu gespeicherten
Gesichtsdaten geändert wurde. Die alte Gesichtsvorschau bleibt sichtbar, alte
Rahmen werden auf dem neuen Foto nicht eingezeichnet. Prüfe Rahmen und Person
zunächst in der Weboberfläche.

### Warteschlange und Statistik

Aktuelle Gruppe, Bildseite, übersprungene Gruppen und ungeklärte Aktionen werden
auf dem Gerät für das jeweilige Konto aufbewahrt. Die App hat keine allgemeine
Offline-Warteschlange für neue Personenentscheidungen.

**… → Statistik** zählt Gesichter und Gruppen für heute und insgesamt: Benannt,
Zugeordnet, Ignoriert und Übersprungen. Bestätigte Serveraktionen zählen einmal;
das Zurücknehmen eines Überspringens korrigiert dessen Zählung. Abtrennen und
Gruppenvergleiche zählen nicht als erstmaliges Benennen. Die Statistik gilt nur für
dieses Gerät; App-Daten löschen oder Deinstallation entfernt sie.

## Einstellungen und Offlinebetrieb

### Einstellbarer Thumbnail-Cache

Unter **… → Einstellungen → Thumbnail-Cache** legst du **64 bis 2048 MiB**
Speicherbudget fest; Standard sind **256 MiB**. Die Änderung gilt sofort und bleibt
nach einem Neustart erhalten. Die Anzeige zeigt Belegung, geschützten Bestand und
Ladefortschritt; bei Fehlern steht **Erneut versuchen** bereit.

Automatisch geschützt sind die neuesten **50 Vorschauen des ungefilterten
Fotostreams** und alle Ordnervorschauen der **ersten Ebene**, einschließlich weiterer
Ordnerseiten. Der übrige Platz dient bereits besuchten Galerie-Thumbnails; länger
nicht verwendete, ungeschützte Vorschauen werden zuerst verdrängt. Ist der
Pflichtbestand größer als das eingestellte Budget, darf er dieses überschreiten.

Das Vorladen beginnt nach der Anmeldung und wird beim Öffnen einer Galerie oder
bei Rückkehr in den Vordergrund höchstens einmal pro Minute aktualisiert. Es kann
**mobile Daten** verwenden. Davon getrennt werden große Vorschauen für die nächste
Aufnahme beziehungsweise angezeigte Gesichter nur über **WLAN** vorgeladen.

Der Thumbnail-Cache überlebt App-Neustarts und wird beim ausdrücklich gewählten
**Verbindungswechsel** gelöscht. Originale und große Vollbildvorschauen werden nicht
dauerhaft darin gespeichert. Die [technische Referenz](android-technik.md#speicher-und-ladeverhalten)
erläutert Speichergrenzen und Dateiverwaltung.

### Start und Offlinebetrieb

Bei gespeicherter Verbindung versucht die App beim Start automatisch die Anmeldung.
Antwortet der Server nicht innerhalb von acht Sekunden oder schlägt die Anmeldung
fehl, öffnet sich **Dieses Gerät** mit einem Fehlerhinweis und **Erneut versuchen**.
Die lokale Ansicht bleibt während eines erneuten Versuchs geöffnet. Es gibt keine
fortlaufende automatische Wiederholung; Zugangsdaten und Zertifikatsbindung bleiben
erhalten. Ein geändertes Zertifikat wird nicht automatisch akzeptiert.

| Funktion | Ohne Server nutzbar? |
| --- | --- |
| Freigegebene Fotos auf **Dieses Gerät** ansehen und teilen | Ja. |
| Serverordner und Serverfotos durchsuchen | Nein; der Katalog benötigt eine Verbindung. |
| Serveroriginal öffnen, herunterladen oder neu zum Teilen vorbereiten | Benötigt eine Verbindung. |
| Neue Personenentscheidungen speichern | Benötigt eine Verbindung. |
| Gespeicherte Galerie-Thumbnails behalten | Ja; daraus entsteht aber keine vollständige Offlinegalerie. |

## Probleme beheben

| Beobachtung | Was du prüfen kannst |
| --- | --- |
| Anmeldung schlägt fehl | HTTPS-Adresse einschließlich Proxy-Pfad, Benutzername und Passwort prüfen. Dieselbe Adresse auf demselben Gerät im Browser testen. |
| Die Adresse leitet um | Die endgültige HTTPS-Zieladresse eintragen. |
| Server ist nur außerhalb der App erreichbar | Android-Netzwerkfreigaben und appbezogene VPN-Regeln prüfen. |
| Zertifikat wurde geändert | Den neuen Fingerabdruck beim Serverbetreiber prüfen und die Verbindung erneut einrichten. |
| **Personen verwalten** fehlt | Das Konto benötigt zusätzlich **Fotos bearbeiten**. |
| Personen, Karten oder Datumssprung fehlen | Serverversion, angebotene Funktionen und fertigen Fotoindex prüfen. |
| Lokale Bilder fehlen | Fotozugriff und gegebenenfalls die Auswahl freigegebener Bilder prüfen. Nur Fotos aus Androids Medienindex werden angezeigt. |
| Thumbnail-Cache belegt mehr als eingestellt | Der geschützte Pflichtbestand darf das Budget überschreiten. |
| **Offene Aktion prüfen** erscheint | Zuerst die gespeicherte Aktion klären; danach weiterarbeiten. |
| **Foto geändert · im Web prüfen** erscheint | Gesicht und Rahmen in der Weboberfläche prüfen. |

Fehlermeldungen nennen die Phase und einen Diagnosecode. **DNS** betrifft die
Namensauflösung, **CONNECT** und **NO_ROUTE** den Verbindungsweg, **TIMEOUT** eine
Zeitüberschreitung. **TLS**, **TLS_IDENTITY**, **CERT_EXPIRED** und
**CERT_NOT_YET_VALID** betreffen Zertifikat, Serveridentität oder Gültigkeit.
**NETWORK_PERMISSION** weist auf verweigerten Netzwerkzugriff hin. Eine
Zeitüberschreitung allein sagt nichts über die Richtigkeit des Passworts aus.
Weitere Prüfschritte stehen unter [Verbindung und Zertifikat](android-technik.md#verbindung-und-zertifikat).

## Serverkompatibilität

Die App blendet Funktionen nach den vom Server gemeldeten Fähigkeiten und Rechten
ein. Für den vollständigen aktuellen Funktionsumfang verwende einen aktuellen
Server; mit älteren Versionen können einzelne Einträge fehlen.

| Funktion | BearStack mindestens |
| --- | --- |
| Galerie, Datumssprung und Gesichtssuche über die Lupe | 0.50.0 |
| Benannte Personen verwalten / nach Namen suchen | 0.43.0 / 0.45.0 |
| Ähnliche Gruppen / zusätzliche Aktionen je Vergleichsseite | 0.49.0 / 0.50.0 |
| Mehrfachauswahl bei benannten Personen | 0.51.0 |
| Native Ordnerprüfung mit Vergrößerung, Sammelaktionen und Pfadsperren | 1.2.0 |
| Personen unter **Ordner** / **Personen im Ordner** | 0.53.0 / 0.55.0 |
| Personen nach **Anzahl Bilder** sortieren | 0.58.0 |
| Hinweis auf geänderte Gesichtsquellen | 0.65.0 |
| Formatierter Serverordnername im Fotoframe | 0.68.0 |
| Zufällige Reihenfolge im Fotoframe für Serverfotos | 0.69.0 |

Build, Signierung, API, Datenhaltung und Tests beschreibt
[Android-App – Technik & Entwicklung](android-technik.md). Die Versionshistorie
steht im [Changelog](https://github.com/ringelbaer/BearStack-DMS/blob/main/CHANGELOG.md).
