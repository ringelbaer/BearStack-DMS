---
title: Personen und Gesichter
description: Personen benennen, Gesichter zuordnen, Fehler korrigieren und größere Fotobestände gemeinsam prüfen.
icon: lucide/users
---

# Personen und Gesichter

BearStack fasst Gesichter zu Personengruppen zusammen. Du kannst diesen Gruppen
Namen geben, falsche Zuordnungen korrigieren und Fotos einer Person unabhängig
von ihrem Ordner finden. Ein automatischer Treffer ist ein Vorschlag und keine
sichere Identitätsfeststellung.

Diese Anleitung beschreibt die Weboberfläche. Zum bloßen Ansehen von Personenfotos
genügt die [Fotogalerie](fotos.md#fotos-einer-person-ansehen). Die
[Android-Anleitung](android.md#personen-verwalten) erklärt die mobile Bearbeitung;
die [Betriebsanleitung](fotos-technik.md#gesichtserkennung-einrichten) beschreibt
die Einrichtung des optionalen Erkennungsdienstes.

## Voraussetzungen und erster Ablauf

Zum Ansehen brauchst du **Fotos lesen** (`photos.read`), zum Benennen, Zuordnen,
Zusammenführen, Ignorieren und Wiederherstellen **Fotos bearbeiten**
(`photos.edit`). Die Erkennung einrichten oder alle erzeugten Gesichtsdaten
löschen darf ein Konto mit **Fotos verwalten** (`photos.manage`).

1. **Fotos → Personen verwalten** öffnen (`/photos/people`).
2. **Unbenannt** wählen, um noch nicht benannte Gruppen zu sehen.
3. Über das Stiftsymbol einer Kachel **Benennen / zuordnen** öffnen.
4. Die Fotovorschau prüfen. Einen neuen Namen eingeben und **Neu anlegen** wählen
   oder eine bereits vorhandene Person aus den Vorschlägen auswählen.
5. Die benannte Person öffnen und ihre Gesichter auf falsche Zuordnungen prüfen.

Bei einzelnen Gesichtern führt ein Vorschlag wie **„Anna“ zuordnen** die Änderung
sofort aus. Für ganze Gruppen oder Ordner wählst du zunächst das Ziel und bestätigst
anschließend mit dem beschrifteten Speichern-Button. **Abbrechen** verändert nichts. Ist die Liste leer, können noch keine Gesichter erkannt worden sein;
die automatische Verarbeitung ist standardmäßig ausgeschaltet.

## Gemeinsame Navigation

Die Bereichsnavigation bietet **Personen**, **Gruppenbilder**, **Ähnliche Gruppen**
und **Geänderte Fotos** entsprechend deinen Rechten. Der aktuelle Bereich ist markiert.
Darüber führen die Pfadlinks zurück zur Übersicht oder Person. Hilfe und die
administrativen Erkennungseinstellungen stehen rechts. Auf schmalen Bildschirmen
lässt sich die Bereichsnavigation horizontal scrollen.

## Übersicht filtern und sortieren

| Filter | Inhalt |
| --- | --- |
| **Alle** | Benannte und unbenannte Gruppen mit aktiven Gesichtern. |
| **Benannt** | Gruppen mit einem Personennamen. |
| **Unbenannt** | Gruppen ohne Namen und mit mindestens einem aktiven Gesicht. |
| **Ignoriert** | Einzelne ignorierte Gesichter, auch aus vollständig ausgeblendeten Gruppen. |

**Alle** enthält keine ignorierten Gesichter. Die Namenssuche steht bei **Alle**
und **Benannt** zur Verfügung. Der Wechsel zu **Unbenannt** oder **Ignoriert**
verwirft den Suchtext. Über **×** am Suchfeld lässt sich eine Namenssuche
zurücksetzen, ohne Filter und Sortierung zu verlieren.
Suchfeld, **×** und **Suchen** sind gleich hoch ausgerichtet; auf Mobilgeräten
sind die Bedienelemente mindestens 44 Pixel hoch.

### Personen sortieren

**Sortieren** bietet jeweils auf- und absteigend:

| Auswahl | Bedeutung |
| --- | --- |
| **Name** | Personenname; Standard ist **Name: A–Z**. Umlaute werden tolerant behandelt. |
| **Anzahl Fotos** | Unterschiedliche sichtbare Fotos mit aktiven Gesichtern dieser Person. |
| **Ordner** | Angezeigter Ordner des Vorschaubilds, nicht aller Fotos der Person. |
| **Datum** | Neuestes aktives Foto nach Aufnahmezeit, ersatzweise Dateiänderungszeit. |

Unter **Ignoriert** beziehen sich Ordner und Datum auf das jeweilige Foto; die
Anzahl zählt die unterschiedlichen Fotos mit ignorierten Gesichtern derselben
Gruppe. Private Bilder beeinflussen die öffentlich sichtbare Auswahl nicht.

Die Sortierung gilt für die gesamte Trefferliste, vor der Seiteneinteilung.
Pro Seite erscheinen höchstens **60 Einträge**. Ein Sortierwechsel beginnt auf
Seite 1. Browser und Benutzerkonto bestimmen den gespeicherten Stand aus Seite,
Filter und Sortierung; explizite Angaben in einem Link haben Vorrang. Ohne
JavaScript wird die Sortierung über **Suchen** beziehungsweise **Sortieren**
übernommen.

### Darstellung anpassen

Unter **Anzeigeeinstellungen** rechts in der Werkzeugleiste lassen sich Thumbnailgröße **S/M/L**, Fotoanzahl und bei benannten
Gruppen die Ordneranzeige ändern. Unter **Unbenannt** und **Ignoriert** steht der
Ordner immer über dem Bild. Im Hauptverzeichnis heißt er **Fotos**; Datumspräfixe
und Unterstriche werden wie in der Galerie aufbereitet.

Die Einstellungen bleiben pro Benutzer im Browser gespeichert. Ein aktiver
Vergleichsstern kann das Gruppenporträt bestimmen; bei mehreren Sternen wird
stabil das Gesicht mit der kleinsten ID verwendet.

## Benennen, zuordnen und zusammenführen

Vor dem Speichern den Geltungsbereich im Dialog beachten:

| Einstieg | Was geändert wird |
| --- | --- |
| Stift einer unbenannten Gruppe in der Übersicht | Die gesamte Gruppe wird benannt oder mit einer vorhandenen Person zusammengeführt. |
| Gruppen-Stift in den Personendetails | Die gesamte Personengruppe. |
| Stift an einer einzelnen Gesichtskarte in den Personendetails | Nur dieses Gesicht. |
| Markierte Gesichter in den Personendetails | Nur die ausgewählten Gesichter. |
| Bereits benanntes Gesicht in der Foto-Info | Nur das angeklickte Gesicht im aktuellen Foto. |
| Unbenannte Gruppe in der Foto-Info oder im Gruppenbildmodus | Die Gruppe; den angezeigten Geltungsbereich im Dialog prüfen. |

Der Dialog zeigt einen vergrößerten Ausschnitt des Originalfotos mit Rahmen und
formatiertem Fotopfad. Die Vorschlagsliste enthält ausschließlich benannte
Personen mit Porträts. Bei einzelnen Gesichtern speichert ein Vorschlag oder **Neu anlegen** direkt;
bei Gruppen und Ordnern ist anschließend die beschriftete Hauptaktion zu bestätigen.
Pfeiltasten und Enter funktionieren ebenfalls. Ein fehlendes Vorschaubild
verhindert die Namensauswahl nicht.

### Mehrere Gruppen bearbeiten

Übersicht, Personendetails und ignorierte Gesichter verwenden denselben **Auswahlmodus**:
Ein Klick auf eine Kachel markiert sie, ein weiterer hebt die Markierung auf.
Enter und Leertaste funktionieren ebenfalls. Ohne Auswahlmodus bleiben die
Checkboxen nutzbar. **Alle Gruppen dieser Seite auswählen** markiert in der Übersicht ganze Gruppen;
in Personendetails und bei ignorierten Gesichtern heißt die Aktion **Alle Gesichter dieser Seite auswählen**.
Beide ändern nur die Auswahl auf der aktuellen Seite, noch keine Zuordnung.
Ein Seitenwechsel hebt die Auswahl auf; das Ausschalten des Modus erhält sie.
Die gemeinsame Leiste unten nennt Anzahl und Art der ausgewählten Einträge und
bietet **Auswahl aufheben**. Einzelaktionen sind im Auswahlmodus ausgeblendet.

Ab **1.8.0** markiert **Shift + Klick** im Auswahlmodus alle Einträge zwischen dem
zuletzt ausgewählten und dem angeklickten Eintrag einschließlich beider Endpunkte.
Die Auswahl folgt der sichtbaren Sortierung von links nach rechts, Zeile für Zeile;
sie funktioniert auch rückwärts. Vorhandene Markierungen außerhalb des Bereichs bleiben erhalten.
Das angeklickte Ende ist der Ausgangspunkt für den nächsten Shift-Klick. Ohne Ausgangspunkt
wird nur der angeklickte Eintrag umgeschaltet. Checkboxen und **Shift + Enter/Leertaste**
funktionieren ebenso. **Auswahl aufheben**, **Alle auswählen**, ein Modus- oder Seitenwechsel
setzen den Ausgangspunkt zurück; entfernte oder abgewählte Einträge dienen nicht mehr als Ausgangspunkt.
Die Bereichsauswahl bleibt auf die aktuelle Seite begrenzt und speichert keine Änderungen.

**Ausgewählte Gruppen zusammenführen** verbindet die ausgewählten Gruppen. Die zuerst
ausgewählte benannte Person bleibt mit ihrem Namen erhalten; sind alle Gruppen
unbenannt, bleibt die zuerst gewählte Gruppe erhalten. **Ausgewählte Gruppen benennen / zuordnen** erlaubt stattdessen einen neuen gemeinsamen Namen oder eine vorhandene
benannte Zielperson. Eine bereits markierte Person kann ebenfalls Ziel sein.

Die Auswahl gilt für die aktuelle Seite. Zusammengehörige Änderungen werden
gemeinsam gespeichert. Personen-Tags bleiben beim Zusammenführen erhalten;
widersprüchliche Stammdaten müssen vorher geklärt werden.

Seit **1.7.1** trennt die Werkzeugleiste in Personendetails **Gesamte Person · alle Seiten**,
**Ansicht** und **Gesichter dieser Seite · nur auswählen**. **Gesamte Gruppe benennen / zusammenführen**
betrifft auch Gesichter auf anderen Seiten. Navigation und Auswahl sind neutral gestaltet;
Ignorieren, Auflösen von Zuordnungen und Entfernen eines Namens sind rot abgesetzt.
Bei ähnlichen Gruppen unterscheidet sich die dauerhaft gespeicherte Trennung auch optisch
vom bloßen Ausblenden eines Vorschlags. Geöffnete **Anzeigeeinstellungen** sind wie
das Menü **Weitere Personenaktionen** hervorgehoben.

### Einzelne Zuordnungen korrigieren

Eine Person öffnen und den Stift am falschen Gesicht verwenden. Eine vorhandene
Zielperson auswählen oder einen neuen Namen vergeben. Markierte Gesichter lassen
sich gemeinsam bearbeiten oder ignorieren. Ein leerer Name im Auswahldialog
trennt diese Auswahl als neue unbenannte Gruppe ab.

**Alle Gesichter dieser Seite auswählen** markiert nur die Gesichter der aktuellen Seite. Gesicht und
Dateiname öffnen das vollständige Foto im Bildbetrachter. **Stammdaten** und **Anzeigeeinstellungen** stehen direkt in der Werkzeugleiste.
Das **⋯-Menü** enthält die optionale **Ordner-Pfade prüfen**-Aktion. **Hilfe** steht
rechts neben der Bereichsnavigation und erklärt auch die Vergleichssterne.

### Mit der Lupe passende Personen suchen

Die Lupe neben dem Namensfeld vergleicht das angezeigte Gesicht mit gespeicherten
Referenzen benannter Personen. Bis zu **20 Kandidaten** erscheinen nach
Ähnlichkeit sortiert. Solange **Abgleich läuft …** angezeigt wird, können bessere
Treffer hinzukommen. Die aktuelle Gruppe wird nicht vorgeschlagen.

Ein Treffer bleibt eine zu prüfende Alternative. Seine Auswahl verwendet den
Geltungsbereich des geöffneten Dialogs. Tippen wechselt zurück zur Namenssuche;
Schließen oder neue Eingaben brechen den laufenden Abruf ab. Ein laufender
Erkennungsdienst ist für diesen Vergleich nicht erforderlich.

## Ignorieren und wiederherstellen

**Ignorieren** blendet eine Erkennung aus, ohne das Foto zu löschen. Je nach
Ansicht unterscheidet sich der Umfang:

| Aktion | Umfang |
| --- | --- |
| **Gesicht ignorieren** (durchgestrichener Kreis) an einer unbenannten Kachel | Das angezeigte Gesicht; weitere Gesichter der Gruppe bleiben aktiv. |
| **Ignorieren** bei markierten unbenannten Gruppen | Jeweils das angezeigte Gesicht der ausgewählten Kacheln. |
| Auswahl in den Personendetails | Die markierten Gesichter. |
| Einzelaktion einer Gruppe unter **Ähnliche Gruppen** | Alle aktiven Gesichter dieser einen unbenannten Gruppe. |

Unter **Ignoriert → Als unbenannt wiederherstellen** wird ein Gesicht als neue unbenannte
Gruppe sichtbar, auch wenn es vorher einer benannten Person angehörte. Der
Stift kann es stattdessen direkt benennen oder einer vorhandenen Person
zuordnen. Andere Gesichter seiner früheren Gruppe bleiben unverändert. Mehrere markierte
ignorierte Gesichter lassen sich gemeinsam **Als unbenannt wiederherstellen** oder
**Zuordnen und wiederherstellen**. Beim Wiederherstellen ohne Namen entsteht je Gesicht eine eigene unbenannte Gruppe.
Die Auswahl wird in einer Transaktion verarbeitet;
eine ausbleibende Antwort sperrt erneute Änderungen bis **Ansicht erneut laden**.

Zum Wiederherstellen **mit den bisherigen Zuordnungen** die Aktion für alle
ignorierten Gesichter eines Fotos verwenden, beschrieben im nächsten Abschnitt.

### Ignorierte Gesichter eines Fotoordners zurücksetzen

1. In der Galerie einen normalen Ordner ab Ebene 2 öffnen, etwa einen
   Ereignisordner innerhalb eines Jahresordners.
2. **⋯ Weitere Fotoaktionen → Alle ignorierten Gesichter zurücksetzen** wählen.
3. Den im Bestätigungsdialog genannten Ordner prüfen und bestätigen.

Die Aktion gilt für den **gesamten Ordner samt Unterordnern**, unabhängig von
Suche, Medientyp und Anzeigefiltern. Ignorierte Gesichter ohne Personennamen
werden wieder unbenannt sichtbar. Benannte Gruppen einschließlich ihrer
ignorierten Gesichter, Tags, Favoriten und Stammdaten bleiben erhalten.
Nachbarordner sind nicht betroffen.

Root, erste Ordnerebene wie Jahresordner und virtuelle Personenordner bieten
die Aktion nicht an. Sie benötigt **Fotos bearbeiten** und startet keine neue
Erkennung. Die Rückmeldung nennt die Anzahl wiederhergestellter Gesichter.

## Gesichter im Foto bearbeiten

Ein Foto öffnen und die Info-Seitenleiste anzeigen. Mit Bearbeitungsrecht bietet
die Gesichtsleiste diese Aktionen. **Weitere Gesichtsaktionen** bündelt die seltenen
Aktionen zur Neuanalyse, Wiederherstellung und Aktualisierung. Das Menü bleibt
innerhalb der Info-Seitenleiste und liegt über den darunterliegenden
Bedienelementen; bei wenig Höhe lässt sich die Info-Seitenleiste scrollen:

| Aktion | Wirkung |
| --- | --- |
| **Gesichter erkennen und zuordnen** (Lupe) | Nur dieses Foto mit dem konfigurierten Erkennungsdienst analysieren, auch bei pausiertem Hintergrundlauf. |
| **Gesicht einrahmen** | Ein fehlendes Gesicht manuell markieren; funktioniert ohne Erkennungsdienst. |
| **Beschriftete Gesichtsrahmen anzeigen** | Gespeicherte Rahmen und Namen ein- oder ausblenden, ohne neue Analyse. |
| **Alle ignorierten Gesichter dieses Fotos wiederherstellen** | Ignoriert-Status entfernen; Namen, Gruppen und manuelle Markierungen erhalten. |
| **Gesichter aktualisieren** | Gespeicherte Zuordnungen neu laden, ohne neue Bildanalyse. |

Bei einem bereits benannten Gesicht betrifft der Stift nur dieses Gesicht.
**Auf unbenannt setzen** trennt es als eigene unbenannte Gruppe ab, auch wenn
es das letzte Gesicht der Person ist. Andere Gesichter behalten ihre Zuordnung.
Ein Entwurf im Namensfeld wird bei dieser Aktion nicht übernommen.

Die Rahmenanzeige gilt für den Browser-Tab bis zum Ausschalten, auch nach
Bildwechsel oder Neuladen und bei geschlossener Seitenleiste. Zoom und
Verschieben bewegen Bild und Rahmen gemeinsam. Ignorierte Rahmen sind stark
abgeblendet. Videos und Audios bieten diese Gesichtsaktionen nicht an.

### Ein fehlendes Gesicht manuell einrahmen

1. **Gesicht einrahmen** öffnen.
2. Mit Maus oder Finger einen Rahmen um das Gesicht ziehen. Für die Tastatur
   **Rahmen mittig setzen** verwenden: Pfeiltasten verschieben, Umschalt mit
   Pfeiltasten verändert die Größe.
3. Eine vorhandene Person oder **Neu anlegen** auswählen; diese Auswahl speichert
   direkt. Alternativ einen neuen Namen eingeben und **Gesicht speichern** wählen.

Ohne Rahmen wird nichts gespeichert. **Abbrechen** verwirft den Entwurf;
bei Fehlern bleibt er zur Prüfung offen. Manuelle Rahmen bleiben bei einer
erneuten Erkennung erhalten und deutlich überlappende automatische Treffer
werden nicht doppelt angelegt.

Ein manuell gezeichneter Rahmen besitzt keinen berechneten Gesichtsvektor und
dient deshalb nicht als automatische Vergleichsreferenz. Namenssuche und
Personenzuordnung funktionieren trotzdem. Pro Foto sind insgesamt höchstens
**256 Gesichter** zulässig.

## Personen-Tags

In den Details einer Person stehen **Personen-Tags** direkt am Namen. Mit
Bearbeitungsrecht vorhandene Tags wählen oder neue ergänzen. **Keine Tags →
Übernehmen** entfernt die Tags dieser Gruppe.

Für mehrere Gruppen die Personen auf der aktuellen Übersichtsseite markieren
und **Personen-Tags ergänzen** wählen. Diese Aktion **ergänzt** die Auswahl;
bestehende Tags werden nicht ersetzt. Auch aktive unbenannte Gruppen lassen sich
so bearbeiten. Höchstens **500 Personen pro Anfrage** und **100 Tags pro Person**
sind möglich. Kann eine ausgewählte Gruppe nicht mehr bearbeitet werden, wird
die gesamte Aktion abgelehnt.

Personen-Tags verändern keine Foto-, Ordner- oder Blogtags. Beim Zusammenführen
werden die Tags beider Gruppen vereinigt. Das globale Umbenennen oder Entfernen
eines Tags aktualisiert auch seine Personenzuordnungen.

In der Galerie erscheinen Personen-Tags als virtuelle Ordner unter **Personen**.
Ein Schrägstrich im Tagnamen erzeugt keine weiteren Ordnerebenen.

## Stammdaten und Familie pflegen

In den Details einer **benannten** Person **Stammdaten** öffnen. Dort stehen
Geburts- und Sterbedatum, Mutter, Vater, Geschwister und Ehen zur Verfügung.
Lesen benötigt **Fotos lesen**, Speichern **Fotos bearbeiten**. Die Eingabe dieser
Angaben erfolgt in der Weboberfläche.

Vorhandene benannte Personen werden über die Namenssuche zugeordnet. **×** oder
ein leeres Elternfeld entfernt die jeweilige Zuordnung. Unbekannte Daten dürfen
leer bleiben; leere Sterbe- und Scheidungsdaten werden über **+** eingeblendet.
**Stammdaten speichern** übernimmt alle Angaben gemeinsam, **Abbrechen** verwirft
den Entwurf.

- Geschwister und Ehen erscheinen bei beiden Personen. Geschwister werden
  ausdrücklich gepflegt, nicht automatisch aus den Eltern abgeleitet.
- Eine Ehe enthält Partner, Hochzeitsdatum und optional Scheidungsdatum.
  Mehrere Ehen desselben Paars sind mit unterschiedlichen Hochzeitsdaten möglich.
- Datumswerte sind Kalendertage ohne Uhrzeit. Sterbe- und Scheidungsdatum dürfen
  nicht vor Geburt beziehungsweise Hochzeit liegen.
- Selbstzuordnungen, dieselbe Person in beiden Elternrollen und Kreise in der
  Elternfolge sind nicht zulässig. Umbenennen erhält die Beziehungen.
- Unsichtbare Beziehungen werden nicht angezeigt oder zum Bearbeiten angeboten;
  das Speichern sichtbarer Angaben entfernt sie nicht nebenbei.

Zwischenzeitliche Änderungen erfordern ein erneutes Laden. Dabei den aktuellen
Stand prüfen, bevor der Entwurf verworfen wird. Pro Person sind höchstens
**100 Geschwister und 100 Ehen** möglich. Beim Zusammenführen werden kompatible
Stammdaten übernommen; widersprüchliche Daten oder dadurch entstehende
Selbstbeziehungen verhindern die gesamte Zusammenführung.

## Vergleichssterne verwenden

Der Stern an einer Gesichtskarte in den Web-Personendetails markiert einen
bevorzugten Vergleichsausschnitt. Er wird sofort gespeichert. Aktive Favoriten
werden bei jedem Abgleich berücksichtigt, auch oberhalb der eingestellten
Zielanzahl von Referenzen. Mehr Sterne erhöhen den Rechen- und Speicherbedarf.

Favoriten bestimmen außerdem bevorzugt das Porträt in Übersichten und
Namensvorschlägen. Ignorierte und geschützte Gesichter dienen nicht als Referenz.
Wiederherstellen erhält den Stern; nach Änderungen der Quelldatei muss die
Region zunächst erneut geprüft werden. Die
[Referenzregeln](fotos-technik.md#referenzen-und-qualitat) beschreiben Auswahl und
Grenzen im Detail.

## Gruppenbilder zügig bearbeiten

**Personenverwaltung → Gruppenbilder** zeigt Fotos mit vielen noch unbenannten
Gesichtern. Unter **Durchlaufoptionen** die Schwelle wählen: Standard ist **5**, es erscheinen
Fotos mit **mehr als fünf** unbenannten, aktiven Gesichtern. Möglich sind Werte
von 0 bis 255.

1. Das Foto und die zugehörigen Gesichtskarten prüfen. Eine Vorschau anklicken,
   um ihren Ausschnitt zu vergrößern; ein zweiter Klick zeigt wieder das ganze Foto.
2. Unbenannte Gruppen über den Stift benennen oder zuordnen. Diese Aktion gilt
   für die **gesamte Gruppe**, auch in anderen Fotos.
3. Mit **Gesicht ignorieren** (durchgestrichener Kreis) nur das betreffende Gesicht ignorieren oder mit
   **Verbleibende ignorieren** alle noch unbenannten, aktiven Gesichter des Fotos
   ausblenden und zum nächsten Foto wechseln.
4. **Nächstes Foto** ändert keine Zuordnung. **Durchlauf starten**
   beginnt wieder am Anfang und berücksichtigt auch übersprungene Fotos.

**Nur Unbenannte anzeigen** blendet benannte und ignorierte Vorschauen aus. Das
ändert weder die Fotoauswahl noch den Umfang von **Verbleibende ignorieren**.
Ein geöffnetes Foto bleibt nach der Bearbeitung auch unterhalb der Schwelle
sichtbar. **Mit Zuordnungen wiederherstellen** stellt alle ignorierten Gesichter dieses Fotos mit
ihren bisherigen Namen und Zuordnungen wieder her, auch die vom Filter verborgenen.

Die horizontal scrollbare Bilderleiste ermöglicht das direkte Öffnen früherer
und späterer Gruppenfotos. **Aktuelles Foto** zentriert sie wieder. **Hilfe** rechts in der Bereichsnavigation erklärt die Bedienung. Grundaktionen wie
Überspringen und Ignorieren funktionieren auch ohne JavaScript; Vergrößerung,
Dialog und Wiederherstellen in dieser Ansicht benötigen JavaScript.

## Ähnliche Gruppen prüfen

**Ähnliche Gruppen** zeigt gespeicherte Vorschläge zum Zusammenführen.
Das Foto anklicken, um es vollständig zu sehen; der Personenname öffnet die
Gruppe. Der angezeigte Ähnlichkeitswert ist keine Wahrscheinlichkeit.

Ab BearStack **1.6.0** sucht der Hintergrundabgleich für ein unbenanntes Gesicht
zuerst passende **benannte Personen**. Nur wenn kein benannter Treffer die
Grenzwerte erfüllt, werden unbenannte Gruppen vorgeschlagen. Vorhandene Paare
mit mindestens einer benannten Person stehen zuerst; innerhalb beider Bereiche
entscheidet die Ähnlichkeit. Das gilt auch in Android.

Die Beschriftung **Gruppe ignorieren** bricht bei wenig Platz oder vergrößerter
Schrift innerhalb des Buttons um. Der Stift daneben bleibt separat bedienbar.

| Entscheidung | Folge |
| --- | --- |
| **Zusammenführen** | Die Gruppen werden verbunden. Bei zwei bereits benannten Gruppen ist eine zusätzliche Bestätigung nötig; der Name der zweiten Gruppe bleibt erhalten. |
| **Gruppen dauerhaft getrennt lassen** | Die Ablehnung wird gespeichert und verhindert auch zukünftige automatische Zuordnungen zwischen diesen Gruppen. |
| Gemeinsamer Stift bei zwei unbenannten Gruppen | Beide gemeinsam benennen oder einer vorhandenen Person zuordnen. |
| **Ignorieren** oder Stift an einer unbenannten Seite | Nur diese gesamte Gruppe bearbeiten; die andere Seite bleibt unverändert. |

Nach einer Einzelaktion ersetzt **Vorschlag nur aus Ansicht ausblenden** die gemeinsamen Entscheidungsbuttons.
Es entfernt das Paar aus der aktuellen Ansicht, speichert aber keine Trennung.
In Android heißt die entsprechende Fortsetzung **Weiter**.

Die Webansicht zeigt bis zu **60 Vorschläge** und startet beim Öffnen keine neue
Vektorsuche. Wurden Gruppen inzwischen verändert, ist eine erneute Prüfung nötig.
Einstellungen und Hintergrundabgleich beschreibt die
[Betriebsreferenz](fotos-technik.md#zuordnungen-verbessern).

### Stammdaten und Ordner einer Person

Unter der Überschrift stehen nur hinterlegte Stammdaten: Geburts- und Sterbedatum,
Eltern, Geschwister und Ehen ohne Scheidungsdatum. Die vollständigen Stammdaten
bleiben im Dialog bearbeitbar.

**Ordner-Pfade prüfen** öffnet `/photos/people/{id}/folder`. Ab Android 0.20.0 mit
BearStack 1.2.0 steht die [vollständige native Ordnerprüfung](android.md#ordner-pfade-prufen)
mit der bekannten Gesichtsvergrößerung auch in der App bereit. Jeder exakte Ordner erscheint
einmal mit seinem vollständigen formatierten Galeriepfad und darunter höchstens
acht Gesichtsvorschauen. Weitere Ordner folgen auf der nächsten Seite (40 pro Seite).
Die zentrierte Seitennavigation zeigt die Seitenzahl mit Abstand zu **Zurück**
und **Weiter** und bleibt auch auf schmalen Bildschirmen bedienbar.

**Gesichter zuordnen …** öffnet das gemeinsame Personenmodal für den jeweiligen Ordner.
Dort steht der vollständig formatierte Pfad zur Kontrolle; wähle eine vorhandene
Person oder gib einen neuen Namen ein. **Abbrechen** schließt ohne Änderung.
Escape schließt zunächst offene Suchvorschläge, danach den Dialog. Die Ordnerliste
selbst enthält keine Suchfelder.

Unter **Weitere Ordneraktionen** stehen die selteneren Eingriffe. Vor der Ausführung
zeigt eine Bestätigung Aktion und exakten Ordner; Unterordner sind nicht betroffen.

Mit **Fotos bearbeiten** stehen folgende Aktionen für alle aktiven Gesichter der
Person im jeweiligen Ordner bereit, auch für Gesichter außerhalb der Vorschau:

- **Gesichter zuordnen …**: vorhandene Person wählen oder neuen Namen eingeben.
- **Gesichter auf unbenannt setzen**: als neue unbenannte Gruppe abtrennen.
- **Gesichter ignorieren**: aus den aktiven Ansichten und dem Abgleich entfernen.
- **Pfad ausschließen und Zuordnungen auflösen**: abtrennen und weitere
  Zuordnungen dieses exakten Ordners zur bisherigen Person dauerhaft sperren.
- **Pfad wieder freigeben**: Sperre aufheben; keine automatische Rückzuweisung.

Unterordner sind eigenständige Pfade. Ausgeschlossene Pfade bleiben in dieser
Ansicht sichtbar. Ab **1.7.0** laufen diese Aktionen im Hintergrund, ohne die
Seite neu zu laden. Die Ordnerliste, Vorschauen und Revisionen werden gemeinsam
aktualisiert; die aktuelle Seite bleibt nach Möglichkeit erhalten. Entfällt die
letzte Seite, erscheint Seite 1. Ohne verbleibende Ordner bleibt eine leere Ansicht
mit Rückweg zur Personenübersicht stehen.

Sammelaktionen sind atomar. Bei zwischenzeitlichen Änderungen, einer verlorenen
Antwort oder fehlgeschlagenem Nachladen bleibt die bisherige Liste sichtbar.
**Ansicht aktualisieren** liest den aktuellen Stand, ohne die Schreibaktion zu
wiederholen. Danach eine gewünschte weitere Aktion ausdrücklich neu auswählen.
Ohne JavaScript behalten die übrigen Ordneraktionen ihre Formularweiterleitung;
**Gesichter zuordnen …** benötigt JavaScript.

Manuelle und automatische Zuordnungen sowie
Neuanalysen beachten die Sperren. Personenzusammenführungen übernehmen Sperren;
Konflikte mit vorhandenen Gesichtern verhindern die gesamte Zusammenführung.

Die frühere Gesichtskettenfunktion und ihre HTTP-Endpunkte entfallen ab 1.0.0.

## Gesichter geänderter Fotos prüfen

Wenn sich der Inhalt eines Fotos am bisherigen Pfad ändert, zeigt BearStack
**Foto geändert / Prüfen**. Die letzten bestätigten Gesichtsdaten und Vorschauen
bleiben zunächst erhalten, ihre alten Vergleichsvektoren werden aber nicht weiter
zur Erkennung verwendet.

Unter **Personenverwaltung → Geänderte Fotos** die alte
Vorschau mit dem aktuellen Bild vergleichen. Rahmen und Person lassen sich
bestätigen oder korrigieren. Die Namenssuche verwendet dieselben Vorschläge wie die
übrige Personenverwaltung. Exakte Koordinaten stehen unter **Rahmen präzise einstellen**.
**Bestätigen und weiter** öffnet das nächste offene Gesicht und kehrt am Ende zur
Prüfliste zurück. Dafür reicht **Fotos bearbeiten**. Auch ohne
verfügbaren Gesichtsdienst kann eine Region bestätigt werden; der historische
Vektor wird dadurch nicht wieder zur Vergleichsreferenz.

## Fehler und unbestätigte Änderungen

Wenn eine Speicherantwort ausbleibt, die Aktion nicht blind wiederholen. Die
jeweilige Ansicht bietet **Ansicht erneut laden**, **Gesichter aktualisieren**
oder **Speicherung prüfen** an. Diese Aktionen klären den gespeicherten Stand,
bevor weitere Änderungen zugelassen werden.

Bei einem Konflikt die neu geladenen Gesichter und die Auswahl erneut prüfen.
Die Bildvergrößerung bleibt zur Kontrolle nutzbar. Fehlt eine erwartete Person,
auch Filter, ignorierte Gesichter und Ordnerschutz prüfen.

Originalfotos und XMP-Sidecars werden durch diese Arbeit nicht verändert.
Sicherung, Aufbewahrung und das gezielte Löschen erzeugter Gesichtsdaten stehen
unter [Einrichtung und Betrieb](fotos-technik.md#daten-sichern-und-gesichtsdaten-loschen).

## Stammbäume einrichten und erkunden

Bei aktiviertem Fotomodul öffnet ein Konto mit **Fotos verwalten** unter
**Einstellungen → Stammbaum** die Auswahl der Ausgangspersonen. Suche benannte
Personen, wähle die passenden Vorschläge und speichere die Auswahl. Bis zu
200 Ausgangspersonen sind möglich. Verbundene Ausgangspersonen gehören zum
selben Stammbaum; unabhängige Familien erscheinen getrennt in der Familienauswahl.

BearStack verfolgt sämtliche gespeicherten Eltern-, Kinder-, Geschwister- und
Eheverbindungen in beide Richtungen. Auch geschiedene und wiederholte Ehen zählen
als Familienverbindung. Es werden keine Verwandtschaften aus Namen oder Bildern
abgeleitet. Änderungen an den Stammdaten erscheinen beim nächsten Öffnen der Ansicht.

Mit **Fotos lesen** öffnet **Stammbaum** in der Hauptnavigation die grafische
Ansicht unter `/photos/family-tree`:

- Personen-Karten zeigen Porträt, Namen sowie vorhandene Geburts- und Sterbedaten.
- Ein Klick zeigt Stammdaten, Personen-Tags und sämtliche direkten Verbindungen;
  bei Ehen auch hinterlegte Hochzeits- und Scheidungsdaten. Beziehungsschaltflächen
  führen direkt zur verbundenen Person, **Person und Fotos öffnen** zur Personenansicht.
- Familienzweige richten sich an ihren Eltern und Kindern aus. Partner und gemeinsame
  Eltern stehen zusammen; bei gleichwertiger Position bestimmen Geburtsdatum und
  Name die Reihenfolge. Größere Abstände zwischen Generationen lassen den Linien
  Raum. Lange Verbindungen innerhalb einer Generation laufen oberhalb der Karten.
- Farben und Linien unterscheiden Eltern/Kinder, Geschwister, Ehen und geschiedene
  Ehen. Ausgangspersonen tragen eine Markierung. Eine frühere Ehe bleibt hier sichtbar,
  auch wenn sie in der kompakten Stammdatenzeile der Personenansicht ausgeblendet ist.
- Mit **+ / −**, **100 %** und **Alles anzeigen** passt du den Zoom an. Die freie
  Fläche lässt sich mit der Maus ziehen; Scrollleisten und Touch-Scrollen bewegen
  die Ansicht. **Strg + Mausrad** zoomt am Mauszeiger.
- Die Personensuche findet auch gerade nicht sichtbare Karten. Mit der Tastatur
  wählst du einen Treffer über Enter; im fokussierten Stammbaum scrollen die
  Pfeiltasten, **+ / −** zoomen und **0** zeigt alles.

Ohne ausgewählte Ausgangsperson gibt es keine Stammbaumansicht und keinen Link
dorthin. Der Einstellungspunkt bleibt zur Einrichtung verfügbar. **Auswahl leeren**
und **Auswahl speichern** deaktivieren die Ansicht. Die Auswahl bleibt über Neustarts
bestehen und wird bei Zusammenführungen auf die Zielperson übertragen.

Private, unbenannte oder ausschließlich ignorierte Personen erscheinen nicht und
werden auch nicht als Verbindung zu weiteren Verwandten verwendet. Eine bereits
gewählte, später nicht mehr verfügbare Person bleibt in den Einstellungen ohne
Namensausgabe entfernbar. Sind alle Ausgangspersonen nicht mehr verfügbar, zeigt
die Ansicht einen entsprechenden Hinweis.

Bei großen Familien werden nur die sichtbaren Karten aufgebaut; in der Übersicht
vereinfachen sich die Karten. Pro Abruf gelten 10.000 geprüfte Personen und 50.000
Verbindungen sowie 30 Sekunden Ladezeit als Grenzen. Überschreitet die Auswahl eine
Grenze, erscheint ein Fehler statt eines unvollständigen Baums. Verkleinere dann
die Auswahl, sofern sie mehrere unabhängige Familien enthält.
