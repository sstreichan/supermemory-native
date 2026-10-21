## Prüfauftrag: Ist-Zustand der offiziellen Supermemory-API dokumentieren

### 1. Ziel und Abgrenzung

Erstelle eine genaue, nachvollziehbare und soweit möglich reproduzierbare Bestandsaufnahme der offiziellen, gehosteten Supermemory-API zum tatsächlichen Untersuchungszeitpunkt.

Nutze offizielle Quellen und nach Freigabe tatsächlich ausgeführte HTTP-Anfragen. Dokumentiere verfügbare Funktionen, Eingaben, Ausgaben und die von außen beobachtbare Verarbeitung.

Das Ergebnis dient als unabhängige Referenz für einen möglichen späteren Vergleich. Ein solcher Vergleich gehört nicht zu diesem Auftrag.

Nicht Bestandteil sind:

- Untersuchung oder Bewertung selbst gehosteter Implementierungen;
- Ableitung des offiziellen Verhaltens aus lokalen Routern oder Vertragstests;
- Aufrufe selbst gehosteter Instanzen;
- Kompatibilitätsbewertungen, Migrationen oder Implementierungsänderungen;
- Änderungen an Repositories, Tests oder Konfigurationen;
- Erstellung von Programmcode, Skripten oder automatisierten Testprogrammen;
- Commits, Issues oder Pull Requests.

Erlaubte Arbeitsergebnisse sind der Markdown-Bericht, bereinigte Anfrageprotokolle und zugehörige Dokumentationsanhänge. Diese Artefakte sind keine Implementierungsänderungen.

Beginne mit einer Dokumentations- und Machbarkeitsprüfung. Führe den Live-Teil erst aus, wenn Werkzeuge, sichere Zugangsdaten, Testbereich, Budget und erforderliche Freigaben geklärt sind.

Wenn eine notwendige Angabe fehlt, stelle jeweils eine konkrete Rückfrage und warte auf die Antwort.

### 2. Aktuellen Versionsstand und Quellen feststellen

Prüfe zu Beginn mindestens folgende offiziellen Quellen:

- [Aktuelle v5-Übersicht](https://supermemory.ai/docs/v5/api-reference/overview)
- [Versionierte v5-Referenz](https://api.supermemory.ai/v5/reference)
- [Legacy-Übersicht](https://supermemory.ai/docs/api-reference/overview)
- [Offizieller Versionswechsel-Leitfaden](https://supermemory.ai/docs/migration/api-v5)
- [Changelog](https://supermemory.ai/changelog/)
- [Dokumentationsindex](https://supermemory.ai/docs/llms.txt)

Bekannter Ausgangsstand dieser Planrevision vom 20. Oktober 2026:

- Die offizielle Dokumentation führt v5 als aktuelle API.
- Die bisherige allgemeine API-Übersicht ist als v3/v4-Legacy-Referenz gekennzeichnet.
- Für v3/v4 wird eine Abschaltung zum 31. Dezember 2026 angekündigt.
- Die dokumentierten v5-Anwendungsrouten sind nicht mit `/v5` präfigiert. Der Referenzpfad `/v5/reference` ist kein Präfix für Anwendungsanfragen.

Verifiziere diese Angaben bei Ausführung erneut. Übernimm sie nicht ungeprüft als dann aktuellen Zustand.

Nutze standardmäßig die zum Untersuchungszeitpunkt aktuelle stabile öffentliche API als primären Live-Prüfumfang. Untersuche Legacy-Versionen nur bei ausdrücklich festgelegtem Bedarf und mit eigenem Budget.

Trenne dabei:

- Produkt- beziehungsweise Referenzversion;
- HTTP-Methode und tatsächlichen Routenpfad;
- Version eines gegebenenfalls verwendeten SDKs;
- Version und Abrufzeitpunkt eines Schemas.

Ein dokumentierter Legacy-Lebenszyklus darf mehrere Routenversionen nutzen. Erfasse die jeweilige Version pro Operation, ohne daraus einen einheitlichen Versionsvertrag abzuleiten.

Nutze den Versionswechsel-Leitfaden nur zur Klärung von Versionen und Semantik. Führe keine Migrationsbefehle aus und untersuche dafür keine lokale Implementierung.

Halte Untersuchungsbeginn und -ende, Zeitzone, Quellenabrufzeiten, Weiterleitungen und relevante Deprecation-Hinweise fest.

Ein Dokumentationsindex ist ein Hilfsmittel zur Suche, kein Beweis für Vollständigkeit oder Aktualität.

### 3. Werkzeuge und sichere Authentifizierung prüfen

Prüfe vor authentifizierten Anfragen, ob ein verfügbares Werkzeug:

- beliebige dokumentierte HTTP-Methoden und Request-Bodies unterstützt;
- HTTP-Status, relevante Header und Antwortkörper zugänglich macht;
- automatische Weiterleitungen und Wiederholungen kontrollierbar macht;
- Zugangsdaten sicher einfügt, ohne sie dem Modell oder Protokollen offenzulegen;
- ausreichende Messdaten für Laufzeiten und Polling liefert.

Verwende einen bereits sicher bereitgestellten API-Key nur, wenn Zielorganisation und Verwendungszweck geklärt sind. Behaupte nicht, auf einen Schlüssel aus einem früheren Gespräch zugreifen zu können.

Falls Zugangsdaten fehlen, fordere ihre Bereitstellung über einen unterstützten Secret-Mechanismus an. Eine Umgebungsvariable ist nur dann geeignet, wenn das ausführende Werkzeug tatsächlich sicher darauf zugreifen kann.

Für Zugangsdaten gilt:

- Niemals im Chat, Bericht, Request-Body, URL oder Protokoll ausgeben.
- Authentifizierungsheader vollständig maskieren.
- Auch Cookies, signierte URLs und andere vertrauliche Antwortwerte bereinigen.
- Keine Schlüssel oder vertraulichen Responses an Suchdienste oder öffentliche Dokumentationswerkzeuge übergeben.
- Authentifizierung ausschließlich an den verifizierten offiziellen API-Host senden.
- Automatische Weiterleitungen bei authentifizierten Requests standardmäßig deaktivieren.

Ein Supermemory-MCP-Connector ist kein automatischer Beleg für rohe REST-Antworten. Verwende ihn nur ergänzend, wenn Zielinstanz, Konto, Scope und mögliche Transformationen geklärt sind. Kennzeichne Connector-Beobachtungen separat.

Fehlt ein geeigneter sicherer HTTP-Zugang, liefere eine Dokumentationsrecherche mit blockiertem Live-Teil. Erfinde keine Ausführung und ersetze Rohprotokolle nicht durch SDK-Beispiele oder Connector-Zusammenfassungen.

### 4. Testbereich, Freigaben und Budget verbindlich festlegen

Verwende ausschließlich synthetische Daten in einem dedizierten Testkonto oder nachweislich zulässigen Testbereich.

Lege eine einmalige Durchlaufkennung sowie zwei getrennte Testbereiche A und B fest. Wähle deren Namen gemäß den dokumentierten Regeln.

Eine zufällig gewählte Kennung allein beweist weder Besitz noch technische Isolation. Kläre, ob der verwendete Schlüssel organisationsweit oder enger berechtigt ist und ob der Testbereich ausdrücklich oder implizit angelegt wird.

Nicht erlaubt sind:

- unbeschränkte organisationsweite Such-, Listen- oder Profilabfragen;
- Zugriff auf bestehende produktive Inhalte;
- Änderungen an Organisationskonfiguration, Abrechnung oder produktiven Connectors;
- globale Resets, breit gefasste Löschungen oder Namespace-Merges;
- externe Datenimporte, URL-Abrufe oder Datei-Uploads im Basistest.

Inventarisiere solche Funktionen anhand der Dokumentation. Live-Tests dafür benötigen einen zusätzlichen ausdrücklich freigegebenen Prüfauftrag.

Vereinbare vor dem Live-Teil:

- maximales Anfragevolumen einschließlich Polling, Fehlerfällen und Retries;
- maximale abrechenbare Nutzung beziehungsweise Kosten;
- maximal anzulegende Ressourcen;
- Anfrage-Timeouts und Polling-Zeitlimits;
- eine innerhalb des Gesamtbudgets reservierte Bereinigungskapazität;
- konkrete Abbruchbedingungen.

Ein Request-Limit ist kein verlässlicher Ersatz für ein Kostenlimit. Berücksichtige auch asynchrone Verarbeitung und mögliche Zusatzkosten einzelner Verarbeitungsmodi.

Hole vor jeder freigabepflichtigen Aktion die erforderliche Bestätigung ein. Zeige dafür den aufgelösten Zielhost, die Methode, den Pfad, Parameter, vollständige synthetische Nutzdaten und den erwartbaren Umfang der Wirkung.

Ein allgemeiner Prüfauftrag oder ein genehmigtes Budget ersetzt keine werkzeugseitig erforderliche Einzelbestätigung. Ändern sich Ziel oder Inhalt, ist eine neue Freigabe erforderlich.

### 5. Quellenbasis und Prüfstatus getrennt führen

Verwende zwei unabhängige Kennzeichnungen.

Quellenbasis:

- Dokumentiert: durch eine konkrete offizielle Quelle gestützt.
- Live beobachtet: durch eine tatsächlich ausgeführte Anfrage belegt.
- Abgeleitet: Interpretation mit benannter Grundlage und Grenzen.

Prüfstatus:

- Geplant;
- Ausgeführt;
- Nicht geprüft;
- Blockiert;
- Abgebrochen.

Eine Aussage kann dokumentiert und live beobachtet sein. Ein ausgeführter Test kann von der dokumentierten Erwartung abweichen.

Erfasse pro Test zusätzlich das Ergebnis:

- entspricht der dokumentierten Erwartung;
- weicht davon ab;
- nicht eindeutig beurteilbar.

Behandle fehlende Berechtigungen, Zeitüberschreitungen und ungeeignete Werkzeuge nicht als Beweis dafür, dass eine Funktion nicht existiert.

### 6. Endpunkt- und Featureinventar erstellen

Leite alle Pfade, Methoden, Felder und Datentypen aus offiziellen Quellen ab.

Erfasse pro Operation:

- Referenzversion und tatsächlichen HTTP-Pfad;
- Zweck und Ressourcentyp;
- Authentifizierung und dokumentierten Berechtigungsumfang;
- Position aller Parameter: Pfad, Query, Header oder Body;
- Pflichtfelder, optionale Felder und Datentypen;
- dokumentierte Standardwerte, Validierungen und Limits;
- Response-Formate und Medientypen;
- Statuscodes und Fehlerstruktur;
- synchrone beziehungsweise asynchrone Verarbeitung;
- Ressourcen- und Jobkennungen;
- bekannte Tarifabhängigkeiten und Seiteneffekte;
- Quelle, Quellenbasis und Prüfstatus;
- zugehörige Testfall-IDs.

Erfasse insbesondere Ingestion, Dokumentverwaltung, Chunks, Memories, Suche, Profile, Namespaces beziehungsweise Legacy-Tags sowie weitere dokumentierte Funktionen.

Führe Organisationsverwaltung, Connectors und andere nicht sicher testbare Bereiche im Inventar, ohne daraus eine Live-Testpflicht abzuleiten.

Dokumentiere Differenzen zwischen Referenzseiten, Schemas und SDKs. Behandle keine einzelne Quelle automatisch als fehlerfrei.

### 7. Testdesign und synthetischen Datensatz vorbereiten

Erstelle vor dem ersten Live-Aufruf eine Testfallmatrix mit:

- stabiler Testfall-ID;
- Zweck;
- Voraussetzungen;
- exakter Anfrage;
- dokumentierter Erwartung;
- beobachtbaren Erfolgskriterien;
- maximalen Folgeanfragen;
- Freigabebedarf;
- Bereinigungsweg.

Nutze einen kleinen Datensatz mit:

- einer dauerhaften Präferenz;
- einer zeitlich begrenzten Information mit konkretem Datum;
- einer ausdrücklich widersprechenden späteren Information;
- unterschiedlichen Kontrollmarkern für A und B;
- einer thematisch unabhängigen Negativkontrollfrage.

Verwende getrennte Ressourcen für:

- Hinzufügen widersprechender Information;
- erneutes Hinzufügen mit derselben benutzerdefinierten ID;
- Ersetzen des ursprünglichen Inhalts;
- Vergessen einer Memory;
- Löschen eines Dokuments.

Vermeide, dass ein destruktiver Test die Ausgangslage eines anderen Tests zerstört.

Formuliere Suchtests als exakte Wortlautabfrage, Paraphrase und Negativkontrolle. Eine Negativkontrolle muss nicht zwingend null Treffer liefern. Bewerte die tatsächlich zurückgegebenen Inhalte und dokumentierten Suchoptionen.

Erwarte keine wortwörtlich identischen generierten Memories oder Profile und keine deterministische Rangfolge, sofern die Dokumentation dies nicht garantiert.

### 8. Kontrollierten Lebenszyklus durchführen

Führe nur dokumentierte, freigegebene und budgetierte Schritte aus.

1. Verifiziere Zielhost und zulässigen Scope ohne Auslesen produktiver Inhalte.
2. Lege Testbereich A und Kontrollbereich B auf dokumentiertem Weg an oder nutze deren dokumentierte implizite Anlage.
3. Erfasse eine leere Ausgangslage nur innerhalb dieser Testbereiche.
4. Lege den synthetischen Ausgangsinhalt in A an.
5. Protokolliere Annahmeantwort und zurückgegebene Kennungen.
6. Beobachte Dokumentverarbeitung und Memory-Verarbeitung getrennt.
7. Rufe Dokument, Chunks und Memories soweit unterstützt separat beziehungsweise über dokumentierte Includes ab.
8. Führe Suchtests mit protokollierten Optionen durch.
9. Rufe das Profil auf dokumentiertem Weg ab.
10. Führe die getrennten Korrektur-, ID-Wiederholungs- und Ersetzungstests aus.
11. Wiederhole die relevanten Abruf-, Such- und Profilbeobachtungen.
12. Prüfe die Isolation zwischen A und B.
13. Prüfe Vergessen und Dokumentlöschung an dafür vorgesehenen Ressourcen.
14. Bereinige verbleibende Ressourcen nach exakter Zielfreigabe.
15. Kontrolliere die beobachtbare Bereinigungswirkung innerhalb des reservierten Budgets.

Für v5 besonders beachten:

- `system.status` und `system.dreamingStatus` getrennt erfassen.
- Dokumentstatus `done` nicht automatisch als fertige Memory- oder Profilbildung interpretieren.
- `taskType` und `dreaming` einschließlich dokumentierter Standardwerte erfassen.
- Einen beschleunigten Verarbeitungsmodus nur mit freigegebenem Kostenumfang verwenden.
- Profile nicht mit Legacy-Suchparametern aufrufen; Suchverhalten separat prüfen.
- Erneutes Hinzufügen mit derselben ID nicht mit Ersetzen per `PATCH` gleichsetzen.

Bleibe bei einer anderen geprüften Version bei deren eigenem Vertrag. Übertrage diese Felder und Regeln nicht ungeprüft.

### 9. Polling, Wiederholungen und Abbruch behandeln

Lege Polling-Intervalle, maximale Versuche und Gesamtwartezeit vorab fest. Beachte dokumentierte Rate-Limits und gegebenenfalls `Retry-After`.

Protokolliere jede tatsächlich gesendete Folgeanfrage mit eigener Anfrage-ID.

Unterscheide:

- Annahme des Schreibvorgangs;
- Abschluss der Dokumentverarbeitung;
- Abschluss der Memory-Verarbeitung;
- Sichtbarkeit in Abruf, Suche und Profil.

Ein Polling-Timeout belegt nur, dass das gewünschte Ergebnis innerhalb des Beobachtungsfensters nicht festgestellt wurde.

Wiederhole schreibende Requests nach Verbindungsabbruch oder Timeout nicht blind. Ihr Ergebnis kann unbekannt sein. Nutze zuerst dokumentierte Identitäts-, Status- oder Idempotenzmechanismen. Falls eine Wiederholung nötig ist, prüfe Wirkung, Budget und Freigabe erneut.

Brich neue Tests ab bei:

- unerwarteter Offenlegung produktiver Daten;
- unklarem Zielhost oder Scope;
- überschrittenem Budget;
- unklarer Schreibwirkung;
- wiederholten Zugriffsfehlern oder Dienststörungen;
- nicht sicher auflösbaren Bereinigungszielen.

Nutze danach nur noch sichere, freigegebene Bereinigungsschritte innerhalb der verbleibenden Grenzen.

### 10. Isolation und Fehlerverhalten prüfen

Prüfe Isolation ausschließlich zwischen den eigenen Testbereichen A und B:

- Kontrollinhalte dürfen nicht unerwartet im jeweils anderen Bereich erscheinen.
- Direkte Abrufe fremder Testbereichs-IDs müssen nach dokumentierter Scope-Semantik bewertet werden.
- Prüfe, soweit unterstützt, auch Includes, verwandte Memories und Profile.
- Setze Namespace-Zuordnung nicht automatisch mit einer vollständigen Autorisierungsgrenze gleich.

Leite aus erfolgreichen Tests keine allgemeine Sicherheitsgarantie ab.

Priorisiere wenige risikoarme Fehlerfälle:

- fehlende Authentifizierung;
- synthetisch ungültige Authentifizierung;
- fehlendes Pflichtfeld;
- falscher Datentyp;
- unbekannte eigene Testkennung;
- dokumentierte Pagination und Filter;
- leere oder unbekannte Felder, soweit sicher.

Verwende keine möglicherweise fremde Kennung und keine zufällig beschafften Zugangsdaten.

Ein unbekanntes Feld in einem schreibenden Request kann ignoriert werden und trotzdem eine Ressource erzeugen. Plane es als potenziell erfolgreichen Schreibvorgang.

Teste vermeintlich nicht unterstützte Methoden nur, wenn eine unerwartete Ausführung innerhalb des freigegebenen Testbereichs ungefährlich wäre. Sonst dokumentiere den Fall als nicht geprüft.

Keine absichtliche Überlastung, Kontingentausschöpfung oder großen Grenzpayloads.

### 11. Jede ausgeführte Anfrage nachvollziehbar protokollieren

Unterscheide stabile Testfall-IDs von eindeutigen Anfrage-IDs. Ein Testfall kann mehrere Anfragen enthalten.

Erfasse pro Anfrage:

- Testfall-ID, Anfrage-ID und Versuchszähler;
- Zweck und Auslöser;
- Zielhost, Referenzversion und tatsächlichen Pfad;
- Start- und Endzeitpunkt mit Zeitzone;
- Methode und Query-Parameter;
- relevante bereinigte Request-Header;
- vollständigen synthetischen Body oder ausdrücklich „kein Body“;
- HTTP-Status oder Transportfehler;
- relevante bereinigte Response-Header;
- Antwortkörper;
- gemessene Dauer und verfügbare Messgenauigkeit;
- Ressourcen- und Jobkennungen;
- dokumentierte Erwartung;
- tatsächliche Beobachtung;
- Maskierungen, Kürzungen und Werkzeugbeschränkungen.

Bewahre bereinigte Rohprotokolle und interpretierende Zusammenfassungen getrennt auf.

Kennzeichne, ob der Antwortkörper unverändert, formatiert, geparst oder rekonstruiert vorliegt. Behaupte keine bytegenaue Rohantwort, wenn das Werkzeug nur verarbeitetes JSON liefert.

Nicht-JSON-Antworten, leere Bodies und Transportfehler sind eigenständige Ergebnisse. Fehlende Header dürfen nicht erfunden werden.

Wenn ein Werkzeug unerwartet produktive Inhalte zurückgibt, übernimm sie nicht in den Bericht. Stoppe den betroffenen Test und dokumentiere nur den bereinigten Vorfall.

### 12. Datenverarbeitung und Löschwirkung erklären

Beschreibe nur den belegten Datenfluss:

Eingang → Validierung → Annahme → Dokumentverarbeitung → gegebenenfalls Memory-Verarbeitung → Abruf, Suche und Profil → Aktualisierung → Vergessen oder Löschen.

Trenne:

- Originalinhalt;
- Dokument;
- Chunk;
- Memory und deren Versionen;
- Profil;
- Namespace beziehungsweise Legacy-Tag;
- Dokument-, Memory-, Job- und benutzerdefinierte IDs.

Erkläre dokumentierte Zustände, beobachtete Sichtbarkeit und relevante Metadaten.

Unterscheide Vergessen, Dokumentlöschung, Namespace-Löschung und behauptete dauerhafte Entfernung. Ein nicht mehr sichtbarer Suchtreffer beweist weder physische Löschung noch Entfernung aus Backups.

Prüfe bei Sammeloperationen den Antwortkörper auf Teilfehler. Bewerte einen HTTP-Erfolg nicht automatisch als Erfolg für jede angegebene Ressource.

Dokumentiere nur den tatsächlich geprüften Endpunkt und Beobachtungszeitraum. Erfinde keine internen Modelle, Datenbanken, Prompts oder Algorithmen.

### 13. Ressourcenbestand und Bereinigung absichern

Führe ab dem ersten Schreibversuch ein Ressourcenregister mit:

- Durchlaufkennung;
- Namespace beziehungsweise Testbereich;
- Ressourcentyp;
- benutzerdefinierter und zurückgegebener ID;
- erzeugender Anfrage-ID;
- Zustand;
- geplanter Bereinigungsoperation;
- Bereinigungsfreigabe;
- tatsächlichem Ergebnis;
- Nachprüfung.

Erfasse auch Ressourcen mit unklarem Anlageergebnis nach Timeouts.

Lösche nur Ressourcen, deren Zugehörigkeit zum aktuellen Testlauf belegt ist. Ein Namenspräfix allein reicht nicht als Nachweis.

Semantische Löschungen nur nach dokumentierter Vorschau, soweit vorhanden. Bevorzuge anschließend die Freigabe und Löschung exakt aufgelöster IDs statt einer erneut ausgeführten semantischen Auswahl.

Eine vollständige Testbereichslöschung ist nur zulässig, wenn dessen ausschließliche Zugehörigkeit zum Testlauf nachgewiesen und genau dieses Ziel bestätigt wurde.

Wenn die Bereinigung blockiert bleibt, dokumentiere Restressourcen, Grund und nötige Folgeaktion. Behaupte keine vollständige Bereinigung.

### 14. Lieferumfang

Liefere eine zusammenhängende Markdown-Bestandsaufnahme mit:

1. Kurzfassung und eindeutiger Ergebniskategorie.
2. Untersuchungszeitraum, Umfang und Ausschlüssen.
3. Quellen-, Versions-, Werkzeug- und Zugriffsübersicht.
4. Endpunkt- und Featureinventar.
5. Testfallmatrix mit Prioritäten und Ergebnissen.
6. Vollständigen bereinigten Protokollen der ausgeführten Anfragen.
7. Erklärung von Ressourcenmodell, Verarbeitung und Sichtbarkeit.
8. Beobachtungen zu Suche, Profilen, Fehlern und Isolation.
9. Widersprüchen zwischen offiziellen Quellen und Live-Verhalten.
10. Nicht geprüften, blockierten und abgebrochenen Fällen.
11. Ressourcenregister und Bereinigungsstatus.
12. Reproduktionshinweisen und Grenzen der Aussagekraft.

Verwende als Ergebniskategorie:

- Dokumentationsrecherche: keine nachgewiesenen authentifizierten Live-Referenzfälle;
- Partielle Live-Bestandsaufnahme: nur ein Teil des freigegebenen Kernumfangs ausgeführt;
- Live-Bestandsaufnahme des abgegrenzten Umfangs: alle vereinbarten Kernfälle ausgeführt und belegt.

Verlinke offizielle Quellen unmittelbar bei dokumentierten Aussagen. Verweise bei Live-Beobachtungen auf Anfrage-IDs.

Markiere variable Werte wie Durchlaufkennung, IDs, Zeitstempel, Laufzeiten, Scores und generierte Texte. Reproduzierbarkeit bedeutet einen nachvollziehbaren Ablauf, nicht garantierte identische Antworten eines veränderlichen Dienstes.

### 15. Abnahmekriterien

Der Auftrag ist abgenommen, wenn:

- Versionswahl und Quellenstand nachvollziehbar begründet sind;
- sämtliche Live-Aussagen zu ausgeführten Anfragen zurückverfolgt werden können;
- Quellenbasis, Prüfstatus und Testergebnis getrennt bleiben;
- Dokument- und Memory-Verarbeitung nicht verwechselt werden;
- unterschiedliche Aktualisierungswege separat geprüft oder ausdrücklich ausgelassen sind;
- Kosten-, Anfrage-, Zeit- und Bereinigungsgrenzen eingehalten wurden;
- keine Geheimnisse oder produktiven Inhalte offengelegt wurden;
- keine nicht freigegebenen Zustandsänderungen erfolgt sind;
- Teilfehler und unbekannte Schreibausgänge sichtbar bleiben;
- nicht geprüfte Funktionen nicht als verifiziert erscheinen;
- jede bekannte Testressource einen dokumentierten Bereinigungsstatus besitzt;
- keine weitergehenden Sicherheits-, Vollständigkeits- oder Löschgarantien behauptet werden, als die Belege tragen;
- das Ergebnis unabhängig von selbst gehosteten Implementierungen bleibt.

Fehlende Zugänge oder verweigerte Freigaben rechtfertigen ein begrenztes Ergebnis, aber keine erfundenen Live-Nachweise.
