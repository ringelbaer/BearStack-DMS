---
title: Fotos
description: Erste Schritte mit der Fotogalerie, Suche, Bildinformationen, Tags, Karten und Fotoframe.
icon: lucide/images
---

# Fotos

Mit BearStack durchsuchst und betrachtest du eine vorhandene Fotosammlung im
Browser. Die Ordnerstruktur bleibt erhalten. Bilder werden nicht in die
Dokumentenablage importiert und Originaldateien nicht verändert; Tags,
Personenzuordnungen und Vorschauen speichert BearStack getrennt.

Diese Anleitung beginnt mit der Benutzung einer eingerichteten Galerie.
Für die [Personenverwaltung](fotos-personen.md) und die
[Einrichtung und den Betrieb](fotos-technik.md) gibt es eigene Anleitungen.
Die Bedienung auf dem Smartphone beschreibt die [Android-Anleitung](android.md).

## Erste Schritte

1. Bei BearStack anmelden und in der Navigation **Fotos** öffnen.
2. Einen Ordner und anschließend ein Bild öffnen. Der Bildbetrachter bietet
   Navigation, Zoom und Bildinformationen.
3. Über den Fotopfad oder **Zurück** wieder zur Galerie wechseln.
4. Für eine gezielte Auswahl **Suche und Filter** öffnen und beispielsweise
   `date:2024` oder `tag:urlaub` suchen.

Fehlt **Fotos** in der Navigation, muss das Fotomodul aktiviert und deinem Konto
mindestens **Fotos lesen** zugewiesen sein. Wer BearStack selbst betreibt, beginnt
bei [Fotomodul aktivieren](fotos-technik.md#fotomodul-aktivieren).

## In der Galerie orientieren

| Element | Wofür es da ist |
| --- | --- |
| Fotopfad über der Galerie | Zum übergeordneten Ordner oder zur Fotoübersicht zurückkehren. |
| Ordnerkachel | Einen Teil der Sammlung öffnen; die Vorschaubilder geben einen Eindruck vom Inhalt. |
| **Suche und Filter** | Suchtext, Medientyp und GPS-Filter festlegen. |
| **Galerie sortieren** | Ordnerstandard, Name, Datum oder zufällige Reihenfolge auswählen. |
| **⋯ Weitere Fotoaktionen** | Unter anderem Personen im Ordner, Zufall, Fotoframe und Karte öffnen. |
| **Personen** | Fotos benannter Personen ansehen, unabhängig von ihrem Speicherordner. |

Welche Aktionen erscheinen, hängt von Ansicht, vorhandenem Inhalt und deinen
Rechten ab. Normale Fotoordner und virtuelle Personenordner haben unterschiedliche
Menüs.

### Ordner öffnen und zurückkehren

BearStack bereitet Ordnernamen für die Anzeige auf: Aus
`2026_07_15_Sommer_Urlaub` wird **Sommer Urlaub**, das erkannte Datum wird gesondert
berücksichtigt. Das Foto-Hauptverzeichnis heißt **Fotos**. Technische Pfade für
Dateien und Links bleiben unverändert.

Beim Zurückkehren über den Fotopfad oder Browser-Zurück/Vorwärts bleiben die
vorherige Sortierung, Filter, Seite und Scrollposition erhalten. Diese Positionen
werden im aktuellen Browser-Tab gespeichert. Bei geänderter Fensterbreite bleibt
der zuvor geöffnete Ordner möglichst an derselben Bildschirmposition.

Ordnervorschauen berücksichtigen Bilder aus Unterordnern. Bis zu vier Bilder
zeigen verschiedene Stellen der nach Datum sortierten Sammlung; kleine Ordner
zeigen jedes Bild höchstens einmal. Die genaue Auswahl beschreibt die
[Referenz zu Vorschauen](fotos-technik.md#medienvorschauen).

### Sortieren

Die Sortierung lässt sich über **Galerie sortieren** ändern. **Ordnerstandard**
übernimmt die Vorgabe des jeweiligen Ordners. Bei der Datumssortierung von
Ordnern zählt das im Ordnernamen erkannte Datum.

Administratoren können hier auch **Admin-only anzeigen** einschalten.
Geschützte Inhalte sind selbst für Administratoren zunächst ausgeblendet;
normale Fotoleser können sie nicht freischalten. Details zu den
[Zugriffsrechten](fotos-technik.md#berechtigungen-und-geschutzte-ordner) stehen in
der Betriebsanleitung.

## Fotos suchen und filtern

**Suche und Filter** verbindet ein Suchfeld mit der Auswahl **Bilder**, **Videos**,
**Audios** oder **Alle** und einem GPS-Schalter. Suche innerhalb eines geöffneten
Ordners, um den Bereich einzugrenzen. In der Galerie einer Person bleibt die
Suche auf deren Fotos beschränkt.

Die Suche behandelt Umlaute tolerant. Feldnamen helfen, gezielt nach einer
Eigenschaft zu suchen:

| Beispiel | Gesucht wird nach |
| --- | --- |
| `date:2024` | Fotos mit einem passenden Datum. |
| `directory:Urlaub` | Passenden Fotoordnern beziehungsweise deren Medien. |
| `file_name:IMG` | Dateinamen mit diesem Bestandteil. |
| `type:image` | Bildern. |
| `gps:true` | Medien mit GPS-Daten. |
| `tag:urlaub` | BearStack-Tags. |
| `person:"Marie Curie"` oder `face:Marie` | Gesichtsnamen aus XMP oder benannten Personengruppen. |

Mehrteilige Namen in Anführungszeichen setzen. **Suchen** übernimmt die Auswahl;
**Zurücksetzen** entfernt die Suchfilter. Die Ordnersuche berücksichtigt die
vollständige sichtbare Trefferliste, auch bei kurzen Begriffen und ODER-Suchen.

Ein Gesichtsname ist kein Foto-Tag. Deshalb Personen mit `person:` oder `face:`
suchen; `tag:` durchsucht die vergebenen Tags.

## Bilder, Videos und Bildinformationen

Ein Klick auf eine Medienkachel öffnet den Bildbetrachter. Über die Pfeile lässt
sich innerhalb der geöffneten Auswahl blättern. Am ersten und letzten Medium
stoppt die Navigation; die jeweilige Pfeilrichtung ist deaktiviert.

Die Info-Seitenleiste zeigt vorhandene Metadaten wie Aufnahmezeitpunkt mit Datum
und Uhrzeit, Kamera, GPS, Tags und Gesichter. **Ordner** enthält den formatierten
Ordnernamen, bei Bildern im Hauptverzeichnis **Fotos**. Auf kleinen Bildschirmen
steht die Info unter dem Foto und lässt sich separat scrollen.

Gesichtsnamen verlinken die zugehörigen Personen. Mit Bearbeitungsrecht lassen sich
Gesichter direkt aus der Info zuordnen oder manuell einrahmen. Der Ablauf steht
unter [Gesichter im Foto bearbeiten](fotos-personen.md#gesichter-im-foto-bearbeiten).

Zur Sammlung können auch Videos, Audios, Markdown-Texte und GPX-Dateien gehören.
Welche Vorschauen erzeugt werden können, hängt von Format und Servereinrichtung
ab; die [Format- und Werkzeugreferenz](fotos-technik.md#formate-und-metadaten)
beschreibt die Voraussetzungen.

## Tags vergeben

Mit **Fotos bearbeiten** kannst du Medien und Ordner mit BearStack-Tags
versehen. In der Galerie in den Bearbeitungsmodus wechseln und die Tags am
jeweiligen Ordner oder Medium bearbeiten. Für mehrere Fotos die Bilder
markieren und **Tags ergänzen** oder **Tags entfernen** verwenden.

Tags werden in BearStacks Foto-Datenbank gespeichert, nicht in die Originale
oder XMP-Sidecars geschrieben. Gleichzeitige Tagänderungen bleiben erhalten;
scheitert eine Sammelaktion, wird keine teilweise geänderte Auswahl gespeichert.
Ein laufender Indexscan überschreibt deine Tags nicht.

Personen haben eigene **Personen-Tags**. Sie gelten für die Personengruppe und
werden nicht automatisch auf deren Fotos übertragen. Die
[Personenanleitung](fotos-personen.md#personen-tags) erklärt Einzel- und
Sammeländerungen. Die Foto-Tag-Bibliothek verwaltet ein Konto mit
**Fotos verwalten**.

## Fotos auf der Karte ansehen

1. Einen Fotoordner ab Ebene 2 öffnen, etwa einen Ereignisordner unterhalb eines
   Jahresordners, und bei Bedarf Suchfilter setzen.
2. **⋯ Weitere Fotoaktionen → Karte** wählen, sofern die Kartenansicht verfügbar ist.
3. Mit den Kartensteuerungen zoomen und die vorhandenen Kartenlayer auswählen.
4. Über **Galerie** zur Bildansicht zurückkehren.

Die Karte zeigt Fotostandorte und kann eine zeitlich geordnete Fotoroute sowie
vorhandene GPX-Tracks darstellen. Die Fotoroute berücksichtigt die gesamte
indexierte Auswahl, nicht nur die gerade angezeigte Galerieseite. Sehr lange
Routen werden für die Anzeige vereinfacht; Anfang, Ende und die Kennzeichnung
der angezeigten beziehungsweise vollständigen Punktzahl bleiben erhalten.

Im Foto-Root, auf der ersten Ordnerebene und in virtuellen Personenordnern
steht die Kartenaktion nicht zur Verfügung.

Unter **Einstellungen → Fotos → Foto-Track-Auflösung** legt ein Foto-Verwalter
fest, wie nah Fotos liegen müssen, um zu einem Trackpunkt zusammengefasst zu
werden. Die Stufen reichen von **500 m bis 10 km**. Größere Abstände ergeben
weniger Punkte. Die Einstellung betrifft die aus Fotos erzeugte Route.

Fehlen Fotostandorte, zunächst prüfen, ob die Bilder GPS-Daten besitzen und
bereits indexiert wurden. Eine Karte kann auch über vorhandene GPX-Tracks
verfügbar sein. Größenbegrenzungen und die Aufbereitung stehen in der
[Kartenreferenz](fotos-technik.md#karten-und-gpx).

## Diashow, Fotoframe und Zufallsbild

Die folgenden Schritte gelten für den Browser. Einstellungen und Wiedergabe der
Android-App beschreibt [Diashow und Fotoframe](android.md#diashow-und-fotoframe).

### Diashow

Die Diashow im Bildbetrachter spielt die geöffnete Medienauswahl ab und endet
beim letzten Medium. Das Intervall wird unter **Einstellungen → Fotos →
Slideshow Sekunden** eingestellt; zulässig sind **2 bis 60 Sekunden**.

### Fotoframe

Der Fotoframe eignet sich zur fortlaufenden Wiedergabe eines Ordners
inklusive Unterordnern:

1. Ordner, Filter und gewünschte Sortierung in der Galerie wählen.
2. **⋯ Weitere Fotoaktionen → Fotoframe** öffnen.
3. Bei Bedarf die Informationsleiste mit **×** ausblenden.

Für eine zufällige Reihenfolge vor dem Start **Galerie sortieren → Zufällig**
wählen. Der Browser übernimmt die Auswahl in den Fotoframe. Die Android-App
besitzt dafür einen eigenen gespeicherten Schalter im Zahnrad-Dialog.

Unter **Einstellungen → Fotos → Fotoframe Sekunden** lassen sich **3 bis 300
Sekunden** einstellen. Nach dem letzten Medium beginnt ein neuer Durchlauf.
Wird der Browser-Tab ausgeblendet, pausieren Wiedergabe und Nachladen. Bei der
Rückkehr geht es am aktuellen Medium weiter. Wartet die Verbindung auf die
nächste Seite, bleibt das aktuelle Foto sichtbar.

### Zufallsbild

**⋯ Weitere Fotoaktionen → Zufall** liefert ein zufälliges Bild aus dem gewählten
Bereich. Dieser Link ist auch für externe Anzeigen geeignet. Größenparameter und
die mitgelieferten Metadaten erklärt die
[Referenz zum Zufallsendpunkt](fotos-technik.md#zufallsbild-einbinden).

## Fotos einer Person ansehen

**Fotos → Personen** öffnet virtuelle Ordner für **Alle** benannten Personen
und für zugewiesene Personen-Tags. Eine Person öffnen, um ihre Fotos in der
normalen Galerie anzusehen. Sortierung, Datumsgruppen und Suche stehen dort wie
gewohnt zur Verfügung; ein Foto erscheint nur einmal, auch bei mehreren
Gesichtern derselben Person.

Für Personen aus einem bestimmten Fotoordner **⋯ Weitere Fotoaktionen → Personen
im Ordner** wählen. Personen, Vorschaubilder, Fotoanzahlen und die anschließende
Personengalerie bleiben auf diesen Ordner samt Unterordnern beschränkt.

**Personen ansehen** zeigt Fotos. **Personen verwalten** dient dem Benennen und
Korrigieren von Gesichtern. Mit Bearbeitungsrecht führt **Person bearbeiten** aus
der Personengalerie direkt zur Verwaltung. Eine schrittweise Anleitung bieten
[Personen und Gesichter](fotos-personen.md).

## Extern umbenannte Fotoordner

Ordner und Dateien werden außerhalb von BearStack umbenannt oder verschoben.
Der Foto-Root bleibt schreibgeschützt. Nach einem vollständigen Indexlauf kann
BearStack eindeutige Umzüge innerhalb desselben Roots erkennen und Tags,
Personenzuordnungen sowie Vorschauen erhalten.

Fehlende Inhalte verschwinden aus der Galerie, bleiben aber **sieben Tage** in
der Datenbank aufbewahrt. Alte Links werden nach einem Umzug nicht automatisch
weitergeleitet. Bei mehrdeutigen Kopien kann eine manuelle Ordnerzuordnung nötig
sein. Vorgehen und Grenzen beschreibt
[Ordnerumzüge und Aufbewahrung](fotos-technik.md#ordnerumzuge-und-aufbewahrung).

**Foto geändert / Prüfen** bedeutet, dass die bisher bestätigten Gesichtsdaten
noch nicht zum aktuellen Bild geprüft wurden. Fotobearbeiter können die
[Gesichter geänderter Fotos prüfen](fotos-personen.md#gesichter-geanderter-fotos-prufen).

## Wenn etwas fehlt oder nicht funktioniert

| Beobachtung | Nächster Schritt |
| --- | --- |
| **Fotos** fehlt in der Navigation. | Fotomodul und Recht **Fotos lesen** prüfen lassen. |
| Ein erwartetes Bild oder ein Ordner fehlt. | Suche und Typfilter zurücksetzen; anschließend Indexstand und Ordnerschutz prüfen lassen. |
| Die Vorschau fehlt oder lädt beim ersten Mal langsam. | Sie wird bei Bedarf erzeugt; bei dauerhaften Fehlern Format und Vorschauwerkzeuge auf dem Server prüfen. |
| Ein Foto hat keinen Standort. | GPS-Metadaten prüfen; ohne Koordinaten kann es nicht als Fotostandort erscheinen. |
| Eine Person ist nicht auffindbar. | Die Personengalerie zeigt nur aktive benannte Personen. Unbenannte und ignorierte Gesichter stehen in der Personenverwaltung. |
| Bearbeitungsaktionen fehlen. | **Fotos bearbeiten** ist nötig; Einstellungen benötigen **Fotos verwalten**. |
| Gesichter oder Zuordnungen sind falsch. | Die [Personenanleitung](fotos-personen.md) erklärt Korrektur, Ignorieren und Wiederherstellen. |

Für Einrichtung, Worker, Speicherbedarf und Sicherung bei
[Einrichtung und Betrieb](fotos-technik.md) weiterlesen. Die vollständige
Rechtematrix steht unter [Benutzer und Rechte](benutzer-und-rechte.md).
