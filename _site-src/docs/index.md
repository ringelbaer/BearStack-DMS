---
title: BearStack
description: Lokale Dokumenten- und Fotoverwaltung für kleine Server.
template: landing.html
hide:
  - navigation
  - toc
  - path
---

<section class="bs-hero">
  <div class="bs-hero__shade"></div>
  <div class="bs-hero__layout">
    <div class="bs-hero__inner">
      <p class="bs-kicker">Schlankes Archiv. Starke Funktion.</p>
      <h1>BearStack</h1>
      <p class="bs-lead">
        BearStack organisiert Dokumente und Fotos und ergänzt Ansicht, OCR, Suche und viele weitere Funktionen um deine vorhandenen
        Dateien herum. Metadaten landen lokal in SQLite, die Dateien selbst bleiben unverändert im Dateisystem.
      </p>
      <div class="bs-actions">
        <a class="md-button md-button--primary" href="installation.html">Loslegen</a>
        <a class="md-button" href="dokumente.html">Dokumente</a>
        <a class="md-button" href="fotos.html">Fotos</a>
      </div>
    </div>
  </div>
</section>

<section class="bs-band bs-band--intro" aria-label="Kurzüberblick">
  <div class="bs-wrap bs-intro">
    <div>
      <p class="bs-kicker">Warum BearStack</p>
      <h2>Ein Archiv, schnell und schlank und trotzdem mit umfassenden Funktionen.</h2>
    </div>
    <p>
      BearStack ist für Menschen gebaut, die ihre Unterlagen und Medien nicht in einer Blackbox verlieren wollen. Die App ermöglicht Ordnung über Indexe, Tags und Volltext, ohne die abgelegten Dateien umzuschreiben. 
    </p>
  </div>
</section>

<section class="bs-band" aria-label="Kernfunktionen">
  <div class="bs-wrap">
    <div class="bs-section-head">
      <p class="bs-kicker">Kernfunktionen</p>
      <h2>Alles Wichtige für ein privates Archiv.</h2>
    </div>
    <div class="bs-feature-grid">
      <article class="bs-feature">
        <span class="bs-feature__icon">01</span>
        <h3>Unveränderte Dateien</h3>
        <p>BearStack ergänzt Metadaten, Tags und Suche, ohne deine Dateien in ein proprietäres Format zu verwandeln.</p>
      </article>
      <article class="bs-feature">
        <span class="bs-feature__icon">02</span>
        <h3>Volltext finden</h3>
        <p>Text-Extraktion, OCR-Jobs, Suchfavoriten und eine Tag-basierte, virtuelle Ordnerstruktur machen Dokumente intuitiv auffindbar.</p>
      </article>
      <article class="bs-feature">
        <span class="bs-feature__icon">03</span>
        <h3>Fotoarchiv on the go</h3>
        <p>Das optionale Fotomodul liefert deine Fotosammlung in rasender Geschwindigkeit auf jedes Gerät.</p>
      </article>
      <article class="bs-feature">
        <span class="bs-feature__icon">04</span>
        <h3>Klein betreiben</h3>
        <p>Go-Binary, umfassendes Caching und Hintergrund-Thumbnail-Generierung passen gut zu Raspberry Pi und NAS.</p>
      </article>
    </div>
  </div>
</section>

<section class="bs-band bs-band--quiet" aria-label="Arbeitsfluss">
  <div class="bs-wrap bs-flow">
    <div class="bs-section-head">
      <p class="bs-kicker">Arbeitsfluss</p>
      <h2>Dokumente finden statt Zeit verlieren.</h2>
    </div>
    <ol class="bs-steps">
      <li><strong>Importieren</strong><span>Dateien über WebUI, Mail oder API importieren.</span></li>
      <li><strong>Verarbeiten</strong><span>Indexing, Vorschauerstellung und OCR laufen im Hintergrund und belassen die Originaldatei unangetastet.</span></li>
      <li><strong>Ordnen</strong><span>Datum, Tags und benutzerdefinierte Felder bringen Struktur ohne starre Ablage. List- und Batch-Editing sparen Zeit und Nerven.</span></li>
      <li><strong>Finden</strong><span>Suche, Filter, einstellbare Suchfavoriten und agile Ordnerstruktur führen schnell zur richtigen Datei zurück.</span></li>
    </ol>
  </div>
</section>

<section class="bs-band" aria-label="Schnellstart">
  <div class="bs-wrap bs-command">
    <div>
      <p class="bs-kicker">Schnellstart</p>
      <h2>Eine Binary, ein lokales <span class="bs-nowrap">Datenverzeichnis</span>.</h2>
      <p>Für Entwicklung und Tests reicht ein lokaler Start mit Basic Auth.</p>
    </div>
    <pre><code>BEARSTACK_AUTH_USER=admin \
BEARSTACK_AUTH_PASSWORD=change-me \
go run ./cmd/bearstack</code></pre>
  </div>
</section>

<section class="bs-band bs-band--cta" aria-label="Nächster Schritt">
  <div class="bs-wrap bs-cta">
    <div>
      <p class="bs-kicker">Bereit zum Einrichten</p>
      <h2>Starte lokal und zieh BearStack danach auf Pi, NAS oder Server um.</h2>
    </div>
    <a class="md-button md-button--primary" href="installation.html">Installation lesen</a>
  </div>
</section>
