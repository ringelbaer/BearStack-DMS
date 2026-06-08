# UI-Design: Backup Manager

Dieses Dokument beschreibt ausschließlich die Optik und das Interaktionsdesign der Browseroberfläche von `backup-manager`. Es ist als Vorlage gedacht, um denselben visuellen Stil in einem anderen Projekt nachzubauen, unabhängig von Programmiersprache oder Framework.

## Gesamtwirkung

Die Oberfläche wirkt wie ein ruhiges, lokales Administrationswerkzeug: warm, hell, aufgeräumt und arbeitsorientiert. Sie ist keine Marketingseite und keine dekorative Landingpage. Der erste Eindruck soll sein: "Hier kann ich Jobs verwalten und Backups kontrolliert starten."

Der Stil kombiniert:

- helle Creme- und Sandtöne als Grundfläche
- dunkles, leicht bläuliches Textgrau
- Orange/Braun als primäre Aktionsfarbe
- dezente Glasflächen mit leichter Transparenz
- große, aber nicht verspielte Radien
- weiche Schatten
- klare Formular- und Statuskomponenten

Die UI soll hochwertig, aber nicht laut wirken. Dekoration ist sehr zurückhaltend: keine Illustrationen, keine Icons, keine Kartenstapel, keine starken Farbflächen. Die visuelle Hierarchie entsteht aus Layout, Typografie, Weißraum, Konturen und Zustandsfarben.

## Farbpalette

Die Palette ist warm-neutral mit einem erdigen Orange als Akzent.

```css
:root {
  --bg: #f3efe8;
  --panel: rgba(255, 250, 243, 0.92);
  --panel-strong: #fffaf3;
  --line: #dbcdbb;
  --text: #182832;
  --muted: #64727a;
  --accent: #c95e27;
  --accent-dark: #9a4218;
  --green: #2f7d58;
  --red: #a33f3a;
  --amber: #9a6a1a;
  --shadow: 0 18px 60px rgba(201, 94, 39, 0.08);
  --radius: 22px;
}
```

Verwendung:

- `--bg`: Basis für Seite und Hintergrundverlauf.
- `--panel`: Standardfläche für Hauptcontainer, leicht transparent.
- `--panel-strong`: stärkere helle Fläche für Logs oder dichte Inhalte.
- `--line`: Rahmen, Trenner, Eingabekonturen.
- `--text`: primärer Text.
- `--muted`: Labels, Hilfetext, Metadaten.
- `--accent`: primäre Buttons, aktive Auswahl, Fokus.
- `--accent-dark`: Hover auf primären Buttons und dunkle Akzentdetails.
- `--green`: Erfolg/Bereit.
- `--red`: Fehler/Abbruch/Gefahr.
- `--amber`: laufender Zustand.

Sekundäre Buttonfarbe:

```css
#ebe1d1
```

Danger-Buttonfläche:

```css
#f2d8d3
```

Danger-Buttontext:

```css
#7c2323
```

## Hintergrund

Der Seitenhintergrund ist kein einfarbiger Block, sondern ein sehr weicher, warmer Verlauf mit zwei subtilen radialen Farbfeldern.

```css
body {
  margin: 0;
  color: var(--text);
  background:
    radial-gradient(circle at top left, rgba(201, 94, 39, 0.13), transparent 32%),
    radial-gradient(circle at right 20%, rgba(88, 132, 118, 0.12), transparent 28%),
    linear-gradient(180deg, #f6f2eb, #efe8dc);
  min-height: 100vh;
}
```

Wichtig: Die radialen Verläufe sind nur atmosphärisch. Sie dürfen nicht wie sichtbare Deko-Orbs wirken. Die Fläche muss weiterhin ruhig und flächig erscheinen.

## Typografie

Primäre Schriftfamilie:

```css
font-family: "Avenir Next", "Segoe UI", sans-serif;
```

Monospace für Logs:

```css
font-family: "SFMono-Regular", Menlo, monospace;
```

Typografische Regeln:

- Normale UI-Texte sind 13-15 px groß.
- Abschnittstitel sind 18 px und fett.
- Die Sticky-Bar-App-Bezeichnung ist 20 px.
- Haupttitel, falls verwendet, liegen bei 28-40 px.
- Labels sind klein, uppercase und mit weiter Laufweite.
- Letterspacing wird nur für kleine Labels und einzelne Titel genutzt.

Label-Stil:

```css
font-size: 12px;
text-transform: uppercase;
letter-spacing: 0.08em;
color: var(--muted);
```

Normale Metatexte:

```css
font-size: 12px oder 13px;
color: var(--muted);
```

## Seitenrahmen

Die gesamte Anwendung liegt in einer zentrierten Shell.

```css
.shell {
  width: min(1520px, calc(100vw - 40px));
  margin: 22px auto;
  display: grid;
  gap: 18px;
}
```

Auf kleinen Screens:

```css
.shell {
  width: calc(100vw - 20px);
  margin: 10px auto;
}
```

Die App nutzt keine volle Browserbreite. Der seitliche Rand ist Teil des Designs und lässt die Oberfläche wie ein lokales Werkzeugfenster wirken.

## Flächen und Panels

Hauptflächen teilen einen gemeinsamen Glas-Panel-Stil:

```css
background: rgba(255, 250, 243, 0.92);
border: 1px solid rgba(255,255,255,0.7);
border-radius: 22px;
box-shadow: 0 18px 60px rgba(201, 94, 39, 0.08);
backdrop-filter: blur(10px);
```

Dieser Stil gilt für:

- Sticky-Bar
- obere Einstellungsfläche
- Hauptgrid
- Statuskarte

Innenflächen wie Jobkarten, Metriken und Checkbox-Zeilen nutzen hellere, leicht transparente Weißflächen:

```css
background: rgba(255,255,255,0.72);
border: 1px solid rgba(219,205,187,0.7);
```

Die Panels sollen nicht hart getrennt wirken. Die Rahmen sind hell, die Schatten weich, die Farben nah beieinander.

## Layoutstruktur

Die Seite besteht vertikal aus vier Bereichen:

1. Sticky-Bar
2. globale Backup-Ordner- und Laufaktionen
3. einklappbare Status- und Logkarte
4. zweispaltiger Arbeitsbereich aus Jobliste und Jobeditor

### Sticky-Bar

Die Sticky-Bar ist oben fixiert und bleibt beim Scrollen sichtbar.

```css
.sticky-bar {
  position: sticky;
  top: 10px;
  z-index: 40;
  padding: 14px 18px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto auto auto auto;
  gap: 12px;
  align-items: center;
  box-shadow: 0 10px 28px rgba(201, 94, 39, 0.12);
}
```

Inhalt:

- links: App-Name und kurze Caption
- rechts: primäre Auswahl-Aktion, sekundäre Aktionen, Abbrechen, Status-Pill

Der Titel:

```css
font-size: 20px;
line-height: 1;
font-weight: 700;
letter-spacing: -0.02em;
```

Die Caption:

```css
font-size: 13px;
color: var(--muted);
white-space: nowrap;
overflow: hidden;
text-overflow: ellipsis;
```

### Globale Aktionsfläche

Die zweite Fläche enthält den globalen Backup-Ordner und Aktionen für alle Jobs.

Grid:

```css
.top-controls {
  display: grid;
  grid-template-columns: 1.5fr repeat(4, auto);
  gap: 12px;
  align-items: end;
}
```

Der Pfadinput nimmt die meiste Breite ein. Buttons bleiben so breit wie ihr Inhalt.

### Statuskarte

Die Statuskarte ist ein einklappbares Panel. Im geschlossenen Zustand sieht man nur einen breiten Toggle-Header mit:

- Label `Fortschritt`
- Titel `Status und Log`
- Text `Ausklappen` oder `Einklappen`
- runder Chevron-Indikator

Der Header hat keine eigene Kartenoptik, sondern sitzt direkt im Panel:

```css
.status-toggle {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 18px;
  padding: 22px;
  background: transparent;
  text-align: left;
  border-radius: 0;
}
```

Der Chevron ist ein kleiner Kreis mit Akzent-Tönung:

```css
.status-chevron {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: rgba(201, 94, 39, 0.1);
  color: var(--accent-dark);
}
```

Im geöffneten Zustand rotiert der Chevron um 180 Grad.

### Hauptgrid

Desktop:

```css
.grid {
  display: grid;
  grid-template-columns: 420px minmax(0, 1fr);
  align-items: start;
}
```

Links steht die Jobliste mit fester Breite von 420 px. Rechts steht der Editor und nutzt die Restbreite.

Die rechte Spalte ist sticky:

```css
.pane-editor {
  position: sticky;
  top: 110px;
  max-height: calc(100vh - 110px - 24px);
  overflow-y: auto;
  scrollbar-gutter: stable;
}
```

Zwischen den Spalten liegt ein dezenter Trenner:

```css
border-left: 1px solid rgba(219, 205, 187, 0.7);
```

Unter 1180 px wird das Grid einspaltig. Der Editor ist dann nicht mehr sticky.

## Komponenten

### Buttons

Grundstil:

```css
button {
  border: 0;
  border-radius: 14px;
  padding: 12px 16px;
  font: inherit;
  font-weight: 700;
  cursor: pointer;
  transition: transform .12s ease, opacity .12s ease, background .12s ease;
}
```

Hover:

```css
transform: translateY(-1px);
```

Disabled:

```css
opacity: .45;
cursor: default;
transform: none;
```

Varianten:

```css
.primary {
  background: #c95e27;
  color: white;
}

.primary:hover {
  background: #9a4218;
}

.secondary {
  background: #ebe1d1;
  color: #182832;
}

.danger {
  background: #f2d8d3;
  color: #7c2323;
}
```

Primärbuttons werden für Start-/Speicheraktionen verwendet. Sekundärbuttons für ergänzende Operationen. Danger nur für Abbrechen, Löschen und destruktive Aktionen.

### Inputs, Selects und Textareas

Grundstil:

```css
input, select, textarea {
  width: 100%;
  border: 1px solid var(--line);
  background: rgba(255,255,255,0.88);
  color: var(--text);
  padding: 12px 14px;
  border-radius: 14px;
  font: inherit;
  outline: none;
}
```

Fokus:

```css
border-color: var(--accent);
box-shadow: 0 0 0 4px rgba(201, 94, 39, 0.12);
```

Formularfelder liegen in einem 2-Spalten-Grid:

```css
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.form-grid .full {
  grid-column: 1 / -1;
}
```

Unter 760 px wird das Formular einspaltig.

### Inline-Checkbox-Zeilen

Checkbox-Optionen werden als vollbreite Zeilen mit eigener heller Fläche dargestellt:

```css
.inline-check {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 14px;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: rgba(255,255,255,0.72);
}
```

Die eigentliche Checkbox bleibt klein und nativ.

### Jobkarten

Jobkarten sind kompakte, wiederholte Karten in der linken Spalte.

```css
.job-card {
  border: 1px solid transparent;
  border-radius: 18px;
  padding: 14px 16px;
  background: rgba(255,255,255,0.72);
  cursor: pointer;
  transition: border-color .15s ease, transform .15s ease, background .15s ease;
}
```

Hover:

```css
transform: translateY(-1px);
border-color: rgba(201, 94, 39, 0.3);
```

Aktive Karte:

```css
border-color: var(--accent);
background: rgba(255,255,255,0.94);
```

Aufbau:

- links: Checkbox
- Mitte: Jobname und Host
- rechts: Status-Badge
- darunter: Metazeile mit Projekt, Authentifizierung, Secret-Status

Kopfzeile:

```css
.job-card-head {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 12px;
  align-items: center;
}
```

Jobname:

```css
font-weight: 700;
letter-spacing: -0.01em;
```

Host:

```css
color: var(--muted);
font-size: 14px;
margin-top: 4px;
```

Metazeile:

```css
display: flex;
gap: 10px;
flex-wrap: wrap;
margin-top: 12px;
color: var(--muted);
font-size: 12px;
```

### Badges

Badges sind kleine Pillen mit 999 px Radius.

```css
.badge {
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  background: #e9dfcf;
}
```

Statusvarianten:

```css
.badge.success {
  background: rgba(47,125,88,.14);
  color: #2f7d58;
}

.badge.error {
  background: rgba(163,63,58,.12);
  color: #a33f3a;
}

.badge.running {
  background: rgba(154,106,26,.15);
  color: #9a6a1a;
}
```

### Status-Pill

Die Status-Pill in der Sticky-Bar ist ein horizontaler Indikator mit Punkt.

```css
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 999px;
  font-size: 14px;
  white-space: nowrap;
}
```

Punkt:

```css
.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}
```

Bereit:

```css
background: rgba(201, 94, 39, 0.1);
```

Punkt:

```css
background: #2f7d58;
box-shadow: 0 0 0 6px rgba(47, 125, 88, 0.12);
```

Laufend:

```css
background: rgba(154, 106, 26, 0.15);
```

Punkt:

```css
background: #9a6a1a;
box-shadow: 0 0 0 6px rgba(154, 106, 26, 0.12);
```

Offline/Fehler:

```css
background: rgba(163, 63, 58, 0.12);
color: #a33f3a;
```

Punkt:

```css
background: #a33f3a;
box-shadow: 0 0 0 6px rgba(163, 63, 58, 0.12);
```

### Statusmetriken

Im ausgeklappten Statusbereich stehen zwei Metrikkarten nebeneinander.

```css
.status-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.metric {
  padding: 16px;
  border-radius: 18px;
  background: rgba(255,255,255,0.72);
  border: 1px solid rgba(219,205,187,0.7);
}
```

Label:

```css
font-size: 12px;
text-transform: uppercase;
letter-spacing: 0.08em;
color: var(--muted);
margin-bottom: 8px;
```

Wert:

```css
font-size: 18px;
font-weight: 700;
letter-spacing: -0.02em;
```

Unter 1180 px werden die Metriken einspaltig.

### Progressbars

Progressbars sind flache, pillenförmige Balken.

```css
.bar {
  position: relative;
  width: 100%;
  height: 14px;
  border-radius: 999px;
  background: #eadfce;
  overflow: hidden;
}

.bar > span {
  display: block;
  height: 100%;
  width: 0%;
  border-radius: inherit;
  background: linear-gradient(90deg, #d47234, #c95e27);
  transition: width .18s ease;
}
```

Indeterminate-Zustand:

```css
.bar.indeterminate > span {
  width: 45%;
  background: linear-gradient(
    90deg,
    rgba(201,94,39,.2),
    rgba(201,94,39,.9),
    rgba(201,94,39,.2)
  );
  animation: slide 1.2s linear infinite;
}

@keyframes slide {
  from { transform: translateX(-120%); }
  to { transform: translateX(240%); }
}
```

Labels über den Balken sind 14 px und muted.

### Logs

Logs wirken wie ein heller Konsolenbereich, aber nicht technisch hart.

```css
.logs {
  min-height: 280px;
  max-height: 340px;
  overflow: auto;
  padding: 16px;
  border-radius: 18px;
  background: #fffaf3;
  border: 1px solid rgba(219,205,187,0.9);
  font-family: "SFMono-Regular", Menlo, monospace;
  font-size: 13px;
  line-height: 1.5;
}
```

Log-Farben:

```css
.log-line.info { color: #33454f; }
.log-line.success { color: var(--green); }
.log-line.error { color: var(--red); }
```

### Empty States

Leere Zustände sind helle, gestrichelte Boxen.

```css
.empty {
  padding: 18px;
  border-radius: 18px;
  border: 1px dashed var(--line);
  color: var(--muted);
  background: rgba(255,255,255,0.55);
}
```

### Flash Toast

Kurzmeldungen erscheinen unten rechts als dunkler Toast.

```css
.flash {
  position: fixed;
  right: 20px;
  bottom: 20px;
  max-width: min(460px, calc(100vw - 40px));
  padding: 14px 16px;
  border-radius: 16px;
  background: rgba(24,40,50,0.94);
  color: white;
  box-shadow: var(--shadow);
  opacity: 0;
  transform: translateY(10px);
  pointer-events: none;
  transition: opacity .18s ease, transform .18s ease;
}

.flash.show {
  opacity: 1;
  transform: translateY(0);
}
```

Fehler-Toasts nutzen:

```css
background: rgba(124,35,35,0.96);
```

## Abstände und Radien

Wichtige Maße:

- Shell-Gap: 18 px
- Panel-Padding groß: 22 px
- Hero/Top-Panel-Padding: 26 px 28 px
- Sticky-Bar-Padding: 14 px 18 px
- Formular-Gap: 14 px
- Button-Gap: 10-12 px
- Standard-Button-Padding: 12 px 16 px
- Input-Padding: 12 px 14 px
- Hauptpanel-Radius: 22 px
- Karten-/Metrik-Radius: 18 px
- Input-/Button-Radius: 14 px
- Pill-Radius: 999 px

Das Design lebt von konsistenten Zwischenräumen. Keine Elemente sollten direkt aneinanderkleben; selbst kompakte Jobkarten haben innen 14-16 px.

## Zustände

### Bereit

- Status-Pill mit grüner Dot-Markierung
- Text `Bereit`
- Aktionen aktiv

### Laufend

- Status-Pill amber
- Statuskarte klappt automatisch auf
- Start-, Speicher- und Löschaktionen sind deaktiviert
- Abbrechen ist aktiv
- Schrittprogress kann determinate oder indeterminate sein

### Abbruch angefordert

- Status bleibt laufend/amber, Text zeigt `Abbruch angefordert`
- Abbrechen-Button wird deaktiviert
- Schrittprogress zeigt indeterminate

### Fehler oder Offline

- Status-Pill rot
- Text rot oder Fehlerdetails im Statusbereich
- Toast kann dunkler Rotton sein

### Disabled

Alle deaktivierten Buttons und Controls verlieren deutlich Deckkraft:

```css
opacity: .45;
```

## Responsive Verhalten

### Bis 1180 px

Die Oberfläche wechselt von Desktop-Layout zu Tablet-/Narrow-Layout.

```css
@media (max-width: 1180px) {
  .sticky-bar {
    grid-template-columns: 1fr 1fr;
    align-items: start;
  }

  .sticky-brand {
    grid-column: 1 / -1;
  }

  .top-controls {
    grid-template-columns: 1fr 1fr;
  }

  .grid {
    grid-template-columns: 1fr;
  }

  .pane-editor {
    position: static;
    max-height: none;
    overflow-y: visible;
  }

  .pane + .pane {
    border-left: 0;
    border-top: 1px solid rgba(219, 205, 187, 0.7);
  }

  .status-grid {
    grid-template-columns: 1fr;
  }
}
```

### Bis 760 px

Die UI wird vollständig einspaltig.

```css
@media (max-width: 760px) {
  .shell {
    width: calc(100vw - 20px);
    margin: 10px auto;
  }

  .sticky-bar {
    top: 8px;
    grid-template-columns: 1fr;
    padding: 14px;
  }

  .hero,
  .pane {
    padding: 18px;
  }

  .status-toggle {
    padding: 18px;
    align-items: flex-start;
  }

  .status-content {
    padding: 0 18px 18px;
  }

  .top-controls,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .sticky-caption,
  .status-toggle-meta {
    white-space: normal;
  }
}
```

Auf Mobile dürfen Buttons untereinander stehen. Die Sticky-Bar bleibt sichtbar, aber wird nicht zu breit; Caption darf umbrechen.

## Zu übertragende HTML-Struktur

Die visuelle Struktur sollte in anderen Projekten ungefähr so bleiben:

```html
<div class="shell">
  <section class="sticky-bar">
    <div class="sticky-brand">
      <div class="sticky-title">Backup Manager</div>
      <div class="sticky-caption">...</div>
    </div>
    <button class="primary">...</button>
    <button class="secondary">...</button>
    <button class="danger">...</button>
    <div class="status-pill ready">
      <span class="dot"></span>
      <span>Bereit</span>
    </div>
  </section>

  <section class="hero">
    <div class="top-controls">...</div>
  </section>

  <section class="status-card collapsed">
    <button class="status-toggle">...</button>
    <div class="status-content">...</div>
  </section>

  <section class="grid">
    <div class="pane">
      <div class="section-title">...</div>
      <div class="jobs">...</div>
    </div>
    <div class="pane pane-editor">
      <form class="form-grid">...</form>
    </div>
  </section>
</div>
```

## Dos

- Warmen, hellen Hintergrund verwenden.
- Panels mit leichter Transparenz und weichem Schatten einsetzen.
- Hauptaktionen orange, sekundäre Aktionen beige, destruktive Aktionen rot einfärben.
- Status immer über Pill, Badge und Progress sichtbar machen.
- Labels klein, uppercase und muted halten.
- Formulare in ruhigen, klaren Grids anordnen.
- Den Editor auf Desktop sticky machen.
- Jobkarten kompakt halten und aktive Auswahl über Rahmen und hellere Fläche zeigen.

## Don'ts

- Keine dunkle Admin-Oberfläche daraus machen.
- Keine grellen Farben oder harten Schatten verwenden.
- Keine stark gesättigten Vollflächen für Panels nutzen.
- Keine Marketing-Hero-Sektion ergänzen.
- Keine dekorativen Illustrationen oder großen Icons einbauen.
- Keine verschachtelten Kartenlandschaften erzeugen.
- Keine runden Bubble-/Orb-Hintergründe sichtbar dominant machen.
- Keine Secrets oder technische Details als optische Hauptinformation hervorheben.

## Kurzform

Wenn das Design in einem anderen Projekt schnell rekonstruiert werden soll:

1. Nutze eine warme Creme-Seite mit subtilen radialen Hintergrundverläufen.
2. Lege die App in eine max. 1520 px breite Shell mit 18 px Gap.
3. Verwende halbtransparente, helle Panels mit 22 px Radius, 1 px weißlicher Kontur und weichem orangefarbenem Schatten.
4. Baue oben eine sticky Aktionsleiste mit Titel, Buttons und Status-Pill.
5. Setze darunter globale Controls, dann eine einklappbare Status-/Logkarte.
6. Nutze ein zweispaltiges Arbeitsgrid: 420 px Jobliste links, sticky Editor rechts.
7. Verwende Jobkarten mit 18 px Radius, hellweißer Fläche, Akzent-Rahmen im aktiven Zustand und kleinen Status-Badges.
8. Halte Buttons, Inputs und Checkbox-Zeilen bei 14 px Radius.
9. Verwende Orange für primäre Aktionen, Beige für sekundäre, Rot für Gefahr, Grün/Amber/Rot für Status.
10. Brich bei 1180 px auf einspaltiges Hauptlayout und bei 760 px auf komplett einspaltige Controls um.
