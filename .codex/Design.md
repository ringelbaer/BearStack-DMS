# BearStack Design Prompt

Nutze diese Designdeklaration, um neue interne Tools in der gleichen Optik wie BearStack zu gestalten. Das Ergebnis soll wie ein nativer Teil von BearStack wirken: ruhig, dicht, funktional, lokal-first und fuer wiederholte Admin- und Verwaltungsarbeit optimiert.

## Kopierbarer Prompt

Erstelle ein Web-Tool im BearStack-Design. Die UI ist eine helle, sachliche Verwaltungsoberflaeche fuer Dokumente, Fotos, Ordner, Metadaten, Suche und Einstellungen. Sie wirkt nicht wie eine Marketing-Seite, sondern wie ein robustes Arbeitswerkzeug fuer Menschen, die Daten schnell scannen, filtern, bearbeiten und pruefen muessen.

Verwende eine helle Grundflaeche, weisse Panels, feine graublaue Linien, Petrol als primaere Akzentfarbe, kleine Radien, dezente Schatten, systemnahe Sans-Serif-Typografie und kompakte Abstaende. Baue die erste Ansicht als echte Arbeitsoberflaeche mit Navigation, Seitentitel, Aktionen, Filtern und Datenbereich. Keine Hero-Section, keine grossen dekorativen Illustrationen, keine Farbverlaeufe als Hintergrund, keine uebermaessig runden Cards.

Die Oberflaeche besteht aus einer sticky Topbar, einem zentrierten Hauptbereich bis 1600px Breite, klaren Page-Heads, Formularen, Tabellen, Filterleisten, Tags, Menues, Dialogen und optionalen Vorschau- oder Medienbereichen. Interaktionen sind sichtbar und pragmatisch: Buttons haben klare Prioritaeten, Eingaben haben feste Hoehen, Tabellen sind dicht aber lesbar, ausgewaehlte Zeilen erhalten einen hellen Petrol-Hintergrund und links einen Akzentstrich.

## Design-DNA

- Charakter: leise, utilitaristisch, vertrauenswuerdig, lokal, unaufgeregt.
- Ziel: schnelle Orientierung, wiederholbare Arbeitsablaeufe, gute Scanbarkeit.
- Komposition: keine Landingpage-Anmutung. Die Anwendung startet direkt mit dem Tool.
- Dichte: kompakte Controls, Tabellen und Listen statt luftiger Marketing-Karten.
- Kanten: kleine Radien von 6px bis 8px. Pillen nur fuer Tags, Badges, Chips und Toggles.
- Farbe: neutral-graue Flaechen mit Petrol-Akzent. Gruen nur fuer Erfolg/Tags, Rot nur fuer Gefahr, Gelb nur fuer Warnung.
- Bewegung: sparsam, kurz, funktional. Hover, Loader, Menue-Oeffnungen und Lightbox-Controls duerfen sanft reagieren.

## Design Tokens

Verwende diese Werte als Basis. Namen duerfen angepasst werden, die visuellen Werte sollen erhalten bleiben.

```css
:root {
  color-scheme: light;

  --bg: #f5f7f9;
  --surface: #ffffff;
  --surface-2: #eef2f5;
  --surface-muted: #f9fbfc;
  --surface-translucent: rgba(255, 255, 255, 0.92);
  --field-bg: #ffffff;

  --text: #172026;
  --muted: #65727e;
  --line: #d9e0e6;

  --primary: #176b87;
  --primary-light: #1e7f9f;
  --primary-dark: #0f4d61;
  --on-primary: #ffffff;

  --danger: #b42318;
  --danger-bg: #fef3f2;
  --danger-border: #fecdca;

  --success-bg: #ecfdf3;
  --success-border: #abefc6;
  --success-text: #05603a;
  --success-strong: #2e7d32;

  --warning-bg: #fff1d6;
  --warning-text: #805300;
  --warning-strong: #a56b2b;

  --tag-bg: #e8f3ea;
  --tag-text: #285b34;
  --active-row-bg: #eef8fb;
  --highlight-bg: #fff8df;

  --chart-bg: #edf3f6;
  --chart-border: #d6e2e8;
  --chart-secondary: #52738a;
  --chart-accent: #5d7f48;
  --chart-axis: #9eacb7;
  --chart-grid: rgba(158, 172, 183, 0.35);

  --folder-bg: #dceadf;
  --folder-border: #bed5c4;
  --folder-favorite-bg: #dde9f7;
  --folder-favorite-border: #b8cce5;
  --folder-favorite-mark: #2f6fb2;

  --preview-bg: #111820;
  --code-text: #f8fafc;

  --shadow: 0 10px 30px rgba(23, 32, 38, 0.08);
  --modal-shadow: 0 20px 70px rgba(23, 32, 38, 0.26);
  --modal-backdrop: rgba(23, 32, 38, 0.44);
  --modal-backdrop-subtle: rgba(23, 32, 38, 0.28);
  --modal-backdrop-strong: rgba(23, 32, 38, 0.56);
  --drag-overlay-bg: rgba(23, 107, 135, 0.16);
  --drag-shadow: 0 8px 22px rgba(23, 32, 38, 0.16);

  --radius-sm: 6px;
  --radius-md: 8px;
  --control-height: 38px;
}
```

## Typografie

- Schrift: `ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`.
- Body: 15px, line-height 1.45, Farbe `--text`.
- Seitentitel: 28px, line-height 1.15, font-weight 700 bis 800.
- Mobile Seitentitel: 24px.
- Abschnittstitel in Panels: 16px bis 18px.
- Tabellenkoepfe und Labels: 12px, uppercase, font-weight 700 bis 800, Farbe `--muted`.
- Kleine Metadaten: 12px bis 13px, Farbe `--muted`.
- Keine negative Laufweite. Letter-spacing bleibt 0, ausser uppercase Labels benoetigen die native Browser-Darstellung.

## Layout

- Body: Hintergrund `--bg`, keine Seitenraender.
- Topbar: sticky, 62px hoch auf Desktop, 54px auf Tablet/Mobile, weisse Flaeche, untere Linie.
- Hauptbereich: `.page` mit `max-width: 1600px`, horizontal zentriert, Padding `28px 24px 56px`.
- Mobile Hauptbereich: Padding `20px 14px 44px`.
- Foto- oder Medienseiten duerfen volle Breite nutzen, behalten aber Page-Head, Chips und Pagination mit 24px bzw. 14px Seitenabstand.
- Hauptabstaende: 18px fuer vertikale Gruppen, 12px bis 14px fuer Formularraster, 8px bis 10px fuer inline Controls.
- Verwende CSS Grid fuer Filter, Formulare, Tabellenersatz auf Mobile, Kartenraster und Einstellungsseiten.

## Navigation

- Topbar enthaelt Brand links, Hauptnavigation daneben und Systemmenue rechts.
- Brand: Textfarbe, 20px, font-weight 800, keine dekorative Logo-Karte.
- Nav-Links: muted Text, 6px Radius, Padding 8px 10px. Active/Hover nutzt `--surface-2` und `--text`.
- Systemmenue: Icon-Button 42px x 38px mit drei Linien. Dropdown rechtsbuendig, weisse Flaeche, Linie, Radius 8px, Shadow, 6px Padding.
- Menuepunkte: 9px 10px Padding, 6px Radius, Hover `--surface-2`.

## Buttons und Controls

- Primaerbutton: Petrol-Hintergrund `--primary`, Border `--primary`, weisser Text, 38px Mindesthoehe, 14px horizontaler Innenabstand, 6px Radius, font-weight 650.
- Hover Primaerbutton: `--primary-dark`.
- Sekundaerbutton: weisse Flaeche, Petrol-Text, Petrol-Border. Hover `--surface-2`.
- Danger-Button: `--danger` als Hintergrund und Border.
- Link-Button: transparent, ohne Border, Petrol-Text, Unterstreichung nur bei Hover.
- Icon-Buttons: quadratisch oder nahezu quadratisch, 34px bis 42px, transparente oder weisse Flaeche, feine Linie, Hover mit `--surface-2` und Petrol-Akzent.
- Inputs, Selects, Textareas: 100% Breite, 38px Mindesthoehe, Border `--line`, Radius 6px, weisse Flaeche, 8px 10px Padding.
- Checkboxen: 18px x 18px. Checkbox-Zeilen sind inline-flex mit 8px Gap.
- Disabled Controls: Opacity 0.55 und not-allowed Cursor.

## Page Head

- Jede Arbeitsansicht startet mit `.page-head`: flex, `align-items: flex-start`, `justify-content: space-between`, 18px Gap, 18px Margin unten.
- Links stehen Titel und kurze kontextuelle Beschreibung in muted Text.
- Rechts stehen `.page-actions`: inline-flex, flex-wrap, 10px Gap, rechtsbuendig.
- Keine Feature-Erklaertexte oder grosse Willkommenstexte innerhalb der App.

## Panels, Karten und Empty States

- Panels sind Arbeitscontainer, keine dekorativen Kartenlandschaften.
- Standardpanel: weisse Flaeche, 1px `--line`, 8px Radius, `--shadow`, 16px Padding.
- Tabellencontainer und Formulare koennen dieselbe Panel-Optik nutzen.
- Wiederholte Items duerfen Karten sein, aber nur wenn sie echte Einheiten darstellen, z. B. Suchfavorit, Statistikwert, Foto, Ordner.
- Keine Cards in Cards.
- Empty State: weisse Flaeche, Border, Shadow, 8px Radius, 32px Padding, zentrierter muted Text, Titel in `--text`.

## Tabellen und Listen

- Desktop-Tabellen bleiben echte Tabellen mit `border-collapse: collapse`, 100% Breite und sinnvoller Mindestbreite.
- Header: `--surface-muted`, muted, 12px uppercase, Border unten.
- Zellen: 12px 14px Padding, Border unten `--line`, vertikal mittig.
- Letzte Zeile ohne untere Border.
- Wichtige Links in Tabellen: Textfarbe `--text`, font-weight 700. Hover/aktive Vorschau darf Petrol nutzen.
- Aktive oder ausgewaehlte Zeile: `--active-row-bg`; optional links ein inset Akzentstrich `4px` in `--primary`.
- Mobile Tabellen werden zu kompakten Kartenzeilen: Header ausblenden, Zeile als Grid darstellen, Zelllabels via `data-label` in 10px uppercase muted.

## Filter und Suche

- Filterleisten sind weisse, gerahmte Container mit 8px Radius, 14px Padding, 12px Gap.
- Desktop: Grid mit Suchfeld als groesserer Spalte und weiteren Filterspalten.
- Mobile: Suchfeld zuerst, erweiterte Filter einklappbar oder einspaltig.
- Suchfavoriten, Filtermenues und Sortiermenues sind kleine Icon- oder Summary-Controls mit Dropdowns in Panel-Optik.
- Filter-Chips sind Pillen mit weisser Flaeche, Linie, 28px Mindesthoehe, 3px 9px Padding. Label uppercase muted, Wert fett und ellipsiert.

## Tags, Badges und Status

- Tags und Badges sind kompakte Pillen.
- Standard: 12px, font-weight 700, 3px 8px Padding, 999px Radius.
- Tags: `--tag-bg` und `--tag-text`, optional tag-spezifische Farben ueber CSS Variablen.
- Warnbadges: `--warning-bg` und `--warning-text`.
- Status-Pills: 12px, font-weight 700 bis 800, Pillenradius, farblich nach Zustand.
- Versteckte oder deaktivierte Tags: `--surface-2`, `--line`, `--muted`.

## Dialoge, Modals und Overlays

- Dialoge nutzen keine Browser-Default-Optik: Border 0 oder 1px `--line`, Radius 8px, `--modal-shadow`, Padding 0.
- Header und Footer: flex, 12px 14px Padding, Border unten bzw. oben.
- Body: 14px Padding, Grid mit 12px Gap.
- Dialogbreite: kleine Dialoge 420px bis 440px, grosse Vorschau bis 1120px oder 92vw.
- Backdrops: `--modal-backdrop` fuer Standard, `--modal-backdrop-strong` fuer Vorschau/Lightbox.
- Vorschau-Inhalte liegen auf `--preview-bg`; Iframes und Bilder sind 100% gross und `object-fit: contain`.

## Dokument- und Vorschau-Muster

- Dokumentlisten koennen eine sticky Aktionsleiste unter der Topbar verwenden.
- Dokument-Thumbnails: 96px x 72px auf Desktop, 70px x 54px auf Mobile, 6px Radius, Border `--line`, `object-fit: contain`.
- Ladezustand fuer Thumbnails: dezenter Skeleton-Shimmer auf `--surface-2`.
- Detailseiten: zweispaltiges Grid mit Hauptvorschau und Metadatenpanel, Gap 18px. Unter 1050px einspaltig.
- Vorschaupanels: dunkler Hintergrund `--preview-bg`, keine dekorativen Rahmen innerhalb der Vorschau.

## Foto- und Medien-Muster

- Medienseiten duerfen voller wirken, bleiben aber funktional.
- Foto-Galerien: Grid mit 6px Gap, `repeat(auto-fill, minmax(210px, 1fr))`, Medien mit 4:3 Aspect Ratio.
- Ordner-Tiles fuer Fotos: Preview-Mosaik 4:3 mit dunklem Caption-Gradient unten. Der Gradient ist inhaltlich begruendet, nicht dekorativ.
- Edit-Modus: Medienkarten erhalten weisse Flaeche und 1px Border. Auswahl: 3px Outline in `--primary`.
- Lightbox: fullscreen, Hintergrund `#06090d`, weisser Text, Controls erst bei Hover/Fokus sichtbar, Infopanel rechts 280px bis 340px.
- Medien-Controls in dunkler Lightbox: halbtransparente oder fast weisse Buttons, klare Kontraste, kleine Radien.

## Ordner und Breadcrumbs

- Breadcrumbs sind kompakte Pillen mit 32px Mindesthoehe, 6px bis 11px Padding, 999px Radius.
- Aktueller Breadcrumb: helles Petrol `rgba(23, 107, 135, 0.1)`, Border `rgba(23, 107, 135, 0.3)`, Text `--primary-dark`.
- Ordner-Tiles: weisse Flaeche, Border, 8px Radius, Grid mit 42px Iconspalte, 10px Padding.
- Ordner-Icon: flache CSS-Form, Gruenton `#dceadf`, Border `#bed5c4`, 4px Radius.
- Suchfavoriten-Ordner duerfen hellblau markiert werden.

## Statistik und Diagramme

- Statistikwerte als kompakte Cards: weisse Flaeche, Border, Radius 8px, Shadow, 16px Padding.
- Label: 12px uppercase muted. Wert: 28px, line-height 1.1.
- Balken und Charts: hellgrauer Track `--chart-bg` bzw. `#edf3f6`, Border `#d6e2e8`, Fill primaer Petrol.
- Sekundaerfarben sparsam: Blau-Grau `#52738a`, Gruen `#5d7f48`, Warnung `--warning-strong`.

## Einstellungen und Management-Seiten

- Einstellungsseiten nutzen ein Panel mit linkem Tab-Menue und rechtem Inhalt.
- Desktop: Grid `190px bis 220px` Sidebar plus flexible Content-Spalte.
- Sidebar: `--surface-muted`, rechte Border, 12px Padding, 6px Gap.
- Tab-Button: 38px Mindesthoehe, 6px Radius, muted; aktiv/hover weiss mit Border.
- Inhalt: 20px Padding, 18px Gap.
- Fieldsets in Settings: `--surface-muted`, Border, 8px Radius, 14px Padding.

## Responsive Verhalten

- Breakpoints: etwa 1050px fuer komplexe Grids, 960px fuer Topbar/Page/Tabellen, 640px fuer sehr schmale Layouts.
- Unter 960px:
  - Topbar 54px, 12px Padding, Brand 18px.
  - Navigation horizontal scrollbar statt Umbruch-Chaos.
  - Page Padding 20px 14px 44px.
  - Page-Head darf umbrechen oder als kleines Grid laufen.
  - Filter werden ein- bis zweispaltig.
  - Seitliche Vorschau wird ausgeblendet, modale Vorschau bleibt verfuegbar.
- Unter 640px:
  - Buttons duerfen umbrechen und volle Breite nutzen, wenn sie sonst nicht passen.
  - Panels, Empty States und Filter bekommen `calc(100vw - 28px)`.
  - Tabellen werden zu Kartenzeilen mit festen Mini-Spalten fuer Auswahl und Thumbnail.

## Inhalt und Sprache

- Sprache der App: Deutsch.
- Ton: kurz, konkret, handlungsorientiert.
- Beschriftungen: Substantive und klare Verben, z. B. `Dokumente`, `Ordner`, `Fotos`, `Tags`, `Einstellungen`, `Uebernehmen`, `Abbrechen`, `Loeschen`.
- Hilfetexte: muted, knapp und nur dort, wo sie eine Entscheidung erklaeren.
- Keine sichtbaren Texte, die das Design selbst erklaeren.
- Fehlermeldungen: direkt, rot, ohne Humor.

## Dos

- Baue die echte Arbeitsansicht als ersten Screen.
- Nutze Tabellen fuer tabellarische Daten und kompakte Cards nur fuer wiederholte Objekte.
- Halte Aktionen nahe an dem Objekt, das sie betreffen.
- Verwende sticky Topbar und bei Bedarf sticky Aktionsleisten.
- Nutze klare Hover-, Active-, Disabled- und Loading-Zustaende.
- Ellipsiere lange Dateinamen, Tags und Pfade sinnvoll oder erlaube `overflow-wrap: anywhere`.
- Pruefe Mobile explizit auf Textueberlaeufe und unbrauchbare Button-Zeilen.

## Don'ts

- Keine Marketing-Heroes, keine Value-Proposition-Sektionen, keine dekorativen Orbs.
- Keine grossen runden Karten, keine Cards in Cards.
- Keine einfarbige Petrol-Welt. Petrol ist Akzent, nicht Flaechenfarbe fuer alles.
- Keine dominanten dunklen Dashboards ausser fuer Vorschau, Code und Lightbox.
- Keine grossen Illustrationen, wenn echte Tabellen, Listen oder Medien gebraucht werden.
- Keine ueberlangen Texte in Buttons oder Chips.
- Keine versteckten Primaeraktionen ohne klaren visuellen Hinweis.

## Minimaler CSS-Startpunkt

```css
* {
  box-sizing: border-box;
}

body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-size: 15px;
  line-height: 1.45;
}

a {
  color: var(--primary);
  text-decoration: none;
}

a:hover {
  text-decoration: underline;
}

.topbar {
  align-items: center;
  background: var(--surface);
  border-bottom: 1px solid var(--line);
  display: flex;
  gap: 22px;
  min-height: 62px;
  padding: 0 24px;
  position: sticky;
  top: 0;
  z-index: 20;
}

.page {
  margin: 0 auto;
  max-width: 1600px;
  padding: 28px 24px 56px;
}

.panel,
.table-form,
.empty-state {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow);
}

.panel {
  padding: 16px;
}

button,
.secondary-button {
  align-items: center;
  background: var(--primary);
  border: 1px solid var(--primary);
  border-radius: var(--radius-sm);
  color: var(--on-primary);
  cursor: pointer;
  display: inline-flex;
  font: inherit;
  font-weight: 650;
  justify-content: center;
  min-height: var(--control-height);
  padding: 0 14px;
  white-space: nowrap;
}

button:hover {
  background: var(--primary-dark);
  border-color: var(--primary-dark);
}

.secondary-button {
  background: var(--surface);
  color: var(--primary);
}

.secondary-button:hover {
  background: var(--surface-2);
}

input,
select,
textarea {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--radius-sm);
  color: var(--text);
  font: inherit;
  min-height: var(--control-height);
  padding: 8px 10px;
  width: 100%;
}
```
