---
title: Externe Speicher
description: Ordner, virtuelle Galerien und ausgewählte Originalmedien sicher zu Nextcloud übertragen.
icon: lucide/cloud-upload
---

# Externe Speicher

Ab BearStack **1.12.0** unterstützt die Web-Foto-App optionale ausgehende Uploads. Nextcloud ist der erste Anbieter; mehrere Verbindungen, auch zum selben Server, sind möglich. Administratorrechte sind für Einrichtung, Vorschau und Warteschlange erforderlich. Android, Synchronisation und Downloads sind nicht Bestandteil dieser Funktion.

## Verbindungen einrichten

1. Unter **Einstellungen → Externe Speicher** eine Verbindung hinzufügen, benennen und aktivieren.
2. Die HTTPS-Adresse der Nextcloud eintragen und speichern. Zertifikate müssen vom BearStack-Server akzeptiert werden; die Prüfung lässt sich nicht abschalten.
3. **Konto verbinden** öffnet die Nextcloud-Anmeldung in einem eigenen Browserfenster. Dort anmelden und den Zugriff bestätigen. BearStack erhält ein separates App-Passwort über [Login Flow v2](https://docs.nextcloud.com/server/stable/developer_manual/client_apis/LoginFlow/index.html#login-flow-v2); Passwort und Polling-Token gelangen nicht in die BearStack-WebUI.
4. Über **Basis-Zielordner wählen** einen vorhandenen Ordner oder die oberste Ebene **Dateien** auswählen. Die Uploads legen darunter jeweils einen eigenen Zielordner an.

Der Status **Verbunden** ersetzt nach erfolgreicher Anmeldung die Aktion **Konto verbinden**. Konto, Basis-Zielordner und Upload-Bereitschaft stehen getrennt neben den Verbindungsdaten. Nach der Zielwahl heißt die Aktion **Zielordner ändern**. Die Ordnerauswahl bietet einen Navigationspfad und übernimmt den geöffneten Ordner über **Diesen Ordner verwenden** im festen Dialogfuß.

Jede Verbindung besitzt ihr eigenes Konto und Basisziel. **Verbindung trennen** entfernt Zugangsdaten nur lokal und deaktiviert die Verbindung. Ein App-Passwort bei Bedarf zusätzlich in Nextcloud selbst widerrufen; BearStack sendet auch beim Trennen keinen Löschbefehl.

## Ordner oder eine Auswahl hochladen

In echten Fotoordnern ab zwei Pfadbestandteilen, beispielsweise `2026/20260613-Geburtstag-Thomas-Paue`, befindet sich **Upload to Nextcloud** im Menü **…**. Suchergebnisse und konkrete Personengalerien bieten denselben Einstieg. Reine Personenübersichten enthalten keine konkrete Medienauswahl und bieten keinen Ordner-Upload.

Für einzelne Medien **Bearbeiten → Auswahlmodus** einschalten, die gewünschten Kacheln markieren und **Upload to Nextcloud (Auswahl)** verwenden. Das geht auch im Foto-Stammverzeichnis. Die Markierung gilt wie die übrigen Auswahlaktionen für die aktuelle Seite; die API akzeptiert höchstens 5.000 explizite Pfade je Auswahl. Ein ganzer virtueller Ordner wird dagegen über alle Seiten erfasst.

Im Modal genau eine Verbindung wählen und **Übertragung prüfen** ausführen. Bei nur einer verwendbaren Verbindung ist sie vorausgewählt. Die Vorschau zeigt Zielordner, Anzahl und Größe, vorhandene Dateien, Konflikte und neu zu übertragende Daten. Einzelheiten stehen unter **Dateien und Konflikte ansehen**. Erst **Upload starten** bestätigt den Hintergrundauftrag; das Modal kann danach geschlossen werden.

- Echte Ordner behalten ihren technischen Namen und sämtliche Unterordner. Exportiert werden Originalbilder, Videos und Audiodateien, keine Sidecars, Vorschaubilder oder Verwaltungsdateien.
- Bildgruppen liefern ausschließlich die in der Galerie sichtbaren Hauptbilder.
- Virtuelle Ordner erhalten Namen wie **Person – Thomas** oder **Suche – Urlaub**; eine explizite Auswahl heißt zunächst **Auswahl**. Diese Namen lassen sich im Modal ändern. Darunter bleiben die Pfade relativ zum Foto-Stamm erhalten.
- Suche, Person, Medientyp, GPS und Sichtbarkeit werden bei virtuellen Ordnern berücksichtigt. Ein echter Ordner-Upload umfasst rekursiv alle sichtbaren Medienarten. Versteckte private Medien bleiben ausgeschlossen, solange ihre Anzeige nicht ausdrücklich eingeschaltet ist.
- Gleicher Zielpfad und gleiche Größe bedeuten **vorhanden**, nicht nachgewiesene Inhaltsgleichheit. Abweichende Größen oder Datei-/Ordnerkollisionen ergeben einen Konflikt. Bestehende Inhalte werden weder überschrieben noch gelöscht.

## Warteschlange und Änderungen

Das Burgermenü enthält bei mindestens einer aktivierten Verbindung die **Upload-Warteschlange**. Sie zeigt je Auftrag eine Statusanzeige, Verbindung und Anbieter, den Zielordner sowie getrennte Kennzahlen für Gesamtumfang, offene Dateien, vorhandene Dateien und Konflikte. Der Fortschritt steht darunter; **Dateien und Fehler** öffnet die gegliederte Dateiliste. Die Warteschlange lässt sich nach Verbindung filtern. Ein Auftrag läuft zur Zeit mit höchstens zwei parallelen Dateien. Pausierte und auf Wiederholung wartende Aufträge blockieren andere Verbindungen nicht.

**Pause**, **Fortsetzen**, **Abbrechen** und **Wiederholen** ändern nur den lokalen Auftrag. Bereits hochgeladene Dateien bleiben erhalten. Vor einer Wiederholung prüft BearStack unklare Upload-Ergebnisse erneut am Ziel. Vorübergehende Fehler werden mit wachsender Wartezeit begrenzt wiederholt; volle Speicher und Anmeldefehler pausieren den Auftrag.

Die Vorschau fixiert ihre Dateiliste für 30 Minuten. Neue Treffer erfordern eine neue Vorschau. Veränderte oder verschwundene Quellen werden als Fehler gemeldet; vor jedem Upload werden die aktuellen Administratorrechte des Auftraggebers kontrolliert. Änderungen an Server, Konto oder Basisziel pausieren die betroffenen Aufträge und verlangen eine neue Vorschau. Deaktivieren pausiert die Verbindung; nach Aktivierung können unveränderte bestätigte Aufträge fortgesetzt werden. Andere Verbindungen bleiben unabhängig.

Neustarts nehmen bestätigte Aufträge wieder auf. Dateien ab 20 MiB verwenden [Chunked Upload v2](https://docs.nextcloud.com/server/stable/developer_manual/client_apis/WebDAV/chunking.html); bestätigte Teile werden mit einem nur vom Anbieter interpretierten Zustand gespeichert. Abgebrochene temporäre Uploads bleiben bis zur Bereinigung durch Nextcloud bestehen. BearStack sendet niemals `DELETE`; das protokollbedingte abschließende `MOVE` stammt ausschließlich aus dem eigenen temporären Upload und verwendet `Overwrite: F`.

## Betrieb und Erweiterungen

`internal/transfers` besitzt eine eigene SQLite-Datenbank unter `data_dir/transfers/transfers.db`. Zugangsdaten sind getrennt von der Konfiguration mit AES-GCM verschlüsselt; `credentials.key` muss zusammen mit der Datenbank gesichert werden. Siehe [Backup und Restore](installation.md#backup-und-restore). Abgeschlossene Auftragsdetails werden nach 30 Tagen ausschließlich lokal entfernt.

Die Seiten `/settings/storage-connections` und `/photos/uploads` sowie die API `/api/transfers/v1/` prüfen die Administratorrolle; schreibende Browseranfragen zusätzlich die Herkunft zum CSRF-Schutz. Galerieaufrufe lesen ausschließlich lokale Verbindungsdaten und warten auf keinen Cloud-Anbieter. Medienlisten und Uploads werden in begrenzten Stapeln beziehungsweise als Streams verarbeitet. Originale können auf einem schreibgeschützten Mount liegen.

Der Transferkern kennt weder DAV-URLs noch Nextcloud-HTTP-Statuscodes. Anbieter liefern Metadaten, Konfigurationsprüfung, Anmeldung, opake Basisziele, Bestandsabfrage, Zielstruktur und sicheres Neuanlegen. Der Vertrag enthält keine allgemeinen Lösch-, Überschreib- oder Verschiebeoperationen. Neue Adapter müssen die gemeinsame Suite unter `internal/transfers/contracttest` bestehen. Ein ausschließlich im Test vorhandener Anbieter mit opaken Ziel-IDs prüft diese Trennung. Die lokale Dokumentenablage bleibt eigenständig.
