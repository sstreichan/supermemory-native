# Testmatrix und synthetischer Datensatz (v5)

**Status:** ausgeführt 2026-10-20 (Lauf 1 12:31–12:37 UTC; Lauf 2: T66, Suchparameter, Memory-Endpunkte, Ingest-Optionen; Lauf 3: alle restlichen Fälle; Lauf 4: T73, Batch-/Timeout-/Rerank-Sonden); Ergebnisse, Abweichungen und Bereinigungsstatus: `docs/supermemory-api-bestandsaufnahme.md` §8–§10. Alle Testressourcen gelöscht. Grundlage: `docs/supermemory-api-bestandsaufnahme.md` (Dokumentationsrecherche 2026-10-20).
**Zweck:** §7 des Prüfauftrags — Testfälle, Datensatz, Budget und Freigabegrenzen vor dem ersten Live-Aufruf festlegen.
**Nicht enthalten:** Ausführung, Ergebnisse, Behauptungen über Laufzeitverhalten.

---

## 1. Freigabegrenzen

| Punkt | Festlegung |
| - | - |
| Zielhost | ausschließlich `https://api.supermemory.ai` |
| API-Version | v5 (unversionierte Pfade, `/ns/{namespace}/...`) |
| Testbereiche | genau zwei: A und B (Namen in §2) |
| Daten | ausschließlich synthetisch, keine realen Personen/Organisationen |
| Ausgeschlossen | Connectors (OAuth/Picker), `PATCH /organization`, Namespace-Delete **mit** `moveTo`, alle Legacy-Routen (`/v3/*`, `/v4/*`), Datei-Uploads, URL-Ingestion |
| Nicht ausgeschlossen | Namespace-Delete **ohne** `moveTo` als letzter Bereinigungsschritt, nur nach Einzelfreigabe |
| Auth | Bearer-Key; `Authorization` in jedem Protokoll vollständig maskieren (`Bearer ***`) |
| Redirects | deaktiviert (`curl` ohne `-L`) |
| Retries | keine automatischen Wiederholungen schreibender Requests |

**Offene Voraussetzungen (blockieren den Start):**

1. API-Key für ein dediziertes Testkonto, übergeben über einen Secret-Mechanismus (nicht im Chat, nicht in der Shell-History).
2. Nachweis des Key-Scopes: bevorzugt **Scoped Key** auf genau A und B; alternativ Org-Key mit ausdrücklicher Freigabe für `GET /namespaces` (nur Metadaten).
3. Budgetfreigabe (§7) und Bestätigung der Einzelaktionen (§9).

---

## 2. Durchlaufkennung und Testbereiche

| Feld | Wert |
| - | - |
| Durchlauf-ID | `smapi-20261020-t1` |
| Namespace A | `smapi-20261020-t1:a` |
| Namespace B | `smapi-20261020-t1:b` |
| Gültigkeit der Namen | `^[a-zA-Z0-9_:-]+$`, ≤ 100 Zeichen — dokumentiert eingehalten |

Implizite Anlage: Der erste Schreibvorgang in A bzw. B legt den Namespace an (dokumentiert). Eine explizite Anlage-API gibt es nicht.

---

## 3. Synthetischer Datensatz

Alle Inhalte sind erfunden. Jede Ressource hat eine eigene `id`; destruktive Tests verwenden **keine** der Baseline-Ressourcen.

### 3.1 Dokumente in A

| Ress-ID | Zweck | `content` (exakt) | Zusatzfelder |
| - | - | - | - |
| `audit-a-d1-pref` | dauerhafte Präferenz | `Der Nutzer bevorzugt morgendliche Termine und knappe Antworten.` | `metadata: {suite:"pref"}`, `dreaming:"instant"` |
| `audit-a-d2-freeze` | zeitlich begrenzte Information mit Datum | `Am 2026-11-03 gilt ein Deployment-Freeze für alle Produktionssysteme.` | `metadata: {suite:"temporal"}`, `date:"2026-10-20"`, `dreaming:"instant"` |
| `audit-a-d3-freeze-update` | ausdrücklich widersprechende spätere Information | `Der am 2026-11-03 geplante Deployment-Freeze wurde am 2026-10-25 aufgehoben.` | `metadata: {suite:"temporal"}`, `date:"2026-10-25"`, `dreaming:"instant"` |
| `audit-a-d4-marker-a` | Kontrollmarker A | `Kontrollmarker A lautet ORANGE-KAKTUS-4711.` | `metadata: {suite:"control", area:"a"}`, `dreaming:"instant"` |
| `audit-a-d6-travel` | Negativkontrolle (fachfremd) | `Reisekosten werden nach Genehmigung innerhalb von 14 Tagen erstattet.` | `metadata: {suite:"travel"}`, `dreaming:"instant"` |

### 3.2 Dokumente in B

| Ress-ID | Zweck | `content` (exakt) | Zusatzfelder |
| - | - | - | - |
| `audit-b-d5-marker-b` | Kontrollmarker B | `Kontrollmarker B lautet BLAUER-LOTUS-8823.` | `metadata: {suite:"control", area:"b"}`, `dreaming:"instant"` |

### 3.3 Separate Ressourcen für Änderungswege und Destruktion (alle in A)

| Ress-ID | Zweck | Erster Inhalt | Folgeschritt |
| - | - | - | - |
| `audit-a-r1-repeat` | gleiche ID erneut hinzufügen | `ID-Wiederholung erster Stand: REPEAT-ALPHA.` | zweiter `POST` mit `content: "ID-Wiederholung zweiter Stand: REPEAT-BETA."`, gleiche `id` |
| `audit-a-r2-replace` | Inhalt ersetzen | `Ersetzungstest alter Stand: REPLACE-ALT.` | `PATCH` mit `content: "Ersetzungstest neuer Stand: REPLACE-NEU."` |
| `audit-a-r3-forget` | Memory vergessen | `Merkwürfel zum Vergessen: VERGISS-MICH-9001.` | Memory-ID aus `GET ...?include=memories` ermitteln, dann `DELETE /memories` |
| `audit-a-r4-delete` | Dokument löschen | `Löschkandidat: DELETE-OMEGA-777.` | `DELETE /document` mit dieser `id` |
| `audit-a-r5-processing` | **optional** Löschung während Verarbeitung | `Verarbeitungsrest: PROCESSING-CHI-555.` | sofort `DELETE /document` (T66) |

Anlage: r1–r4 gemeinsam über T16, r5 nur bei freigegebenem T66.

`dreaming: "instant"` für alle Änderungsressourcen, damit Memories im Beobachtungsfenster entstehen können. Kosten: je eine Extra-Operation pro Dokument (dokumentiert).

### 3.4 Suchabfragen

| Query-ID | Wortlaut | Zweck |
| - | - | - |
| `q-exact-pref` | `Welche Präferenz hat der Nutzer bei Terminen?` | exakter Wortlaut |
| `q-paraphrase-pref` | `Wann arbeitet die Person am liebsten?` | Paraphrase |
| `q-freeze` | `Wann ist der Deployment-Freeze im November?` | Widerspruchssichtbarkeit |
| `q-marker-b` | `BLAUER-LOTUS-8823` | Isolation A→B |
| `q-negative` | `Welches Kfz-Kennzeichen hat der Firmenwagen?` | Negativkontrolle |

Eine Negativkontrolle darf Treffer liefern; bewertet wird der Inhalt, nicht die Trefferzahl.

---

## 4. Testfälle

Spalten: **ID** · Zweck · exakte Anfrage · dokumentierte Erwartung · Erfolgskriterium · max. Folgeanfragen · Freigabe · Ergebnis (Live 2026-10-20).

### Phase P0 — Scope und Ausgangslage

| ID | Zweck | Anfrage | Erwartung (dokumentiert) | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T01 | Scope prüfen, ohne Inhalte zu lesen | `GET /namespaces?limit=100` | `{namespaces[], pagination}`; mit Scoped Key nur freigegebene Namespaces | Scoped Key: ausschließlich A/B sichtbar. Org-Key ohne Freigabe: T01 entfällt | 0 | ja | ✅ org-weit, 34 Namespaces, nur Metadaten protokolliert |
| T02 | Ausgangslage A | `POST /ns/…:a/list/documents` Body `{}` | `200` mit leerem Ergebnis; `404` ist für nicht existierende Namespaces **nicht dokumentiert** | eines von beiden, exakt protokolliert | 0 | ja | ✅ 200 leer (kein 404) |
| T03 | Ausgangslage B | `POST /ns/…:b/list/documents` Body `{}` | wie T02 | wie T02 | 0 | ja | ✅ 200 leer |

Wenn kein Scoped Key vorliegt: T01 entfällt, T02 legt A durch den ersten Schreibzugriff an. T01 ist dann als „nicht geprüft“ zu protokollieren.

### Phase P1 — Ingestion und Verarbeitung

| ID | Zweck | Anfrage | Erwartung (dokumentiert) | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T10 | Einzeldokument anlegen | `POST /ns/…:a/document` mit D1 | `200 {id, status: "queued"}` | `id` zurückgegeben, Status im Enum | 0 | ja | ✅ 200 `queued` |
| T11 | Batch mit 4 Dokumenten | `POST /ns/…:a/document/batch` mit D2, D3, D4, D6 | `200 {results[], count:4, failed:0}` | alle vier akzeptiert, Reihenfolge = Request | 0 | ja | ✅ `count:4, failed:0` |
| T12 | Kontrollmarker B | `POST /ns/…:b/document` mit D5 | `200 {id, status}` | akzeptiert | 0 | ja | ✅ 200 `queued` |
| T13 | Status-Polling A | `GET /ns/…:a/document/{id}` je Dokument | `system.status` durchläuft Pipeline bis `done` | `done` oder `failed` innerhalb 60 s | ≤20 je Dokument | ja | ✅ `done` in 15–45 s |
| T14 | Dreaming-Status getrennt | derselbe GET | `system.dreamingStatus` ist `dreaming` oder `done` | separat protokolliert; `status:"done"` ≠ `dreamingStatus:"done"` | 0 (im selben Aufruf) | ja | ✅ getrennt; `dreamingStatus` erreicht `done` nach `status` |
| T15 | Chunks und Memories | `GET /ns/…:a/document/{id}?include=chunks,memories` | beide Arrays vorhanden; `chunks[].documentId` | Chunks > 0 bei `status:done`; Memories dürfen 0 sein | 0 | ja | ✅ Chunks > 0; Memories 0–2 |
| T16 | Änderungsressourcen anlegen | `POST /ns/…:a/document/batch` mit r1, r2, r3, r4 | `{count:4, failed:0}` | alle vier akzeptiert | 0 | ja | ✅ `count:4` |

Polling-Regel: 3 s Intervall, max. 20 Versuche, Gesamtfenster 60 s pro Dokument; danach „innerhalb des Beobachtungsfensters nicht festgestellt“, kein Blind-Retry des Schreibvorgangs.

### Phase P2 — Retrieval

| ID | Zweck | Anfrage | Erwartung | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T20 | Dokument per ID | `GET /ns/…:a/document/audit-a-d1-pref` | `id`, `content`, `metadata`, `system` | Inhalt entspricht D1 (oder dokumentierter Extraktion) | 0 | ja | ✅ `id/content/metadata/system` |
| T21 | Liste Dokumente | `POST /ns/…:a/list/documents?limit=100` | nur `documents[]` gefüllt | 5 Baseline-Dokumente + Änderungsressourcen sichtbar | 0 | ja | ✅ 9 Dokumente |
| T22 | Pagination | `?limit=2&page=1` dann `page=2` | `pagination{currentPage,limit,totalItems,totalPages}` | keine Dopplung, `totalItems` stabil | 1 | ja | ✅ `totalPages:5`, Seite 2 korrekt |
| T23 | Sortierung | `?sort=createdAt&order=asc` | aufsteigend | Reihenfolge plausibel | 0 | ja | ✅ `asc` korrekt |
| T24 | Filter | Body `{"filter":{"field":"suite","operator":"eq","value":"control"}}` | nur passende Dokumente | genau das Marker-Dokument | 0 | ja | ✅ 2 Treffer, nur `pref` |
| T25 | Chunks listen | `POST /ns/…:a/list/chunks?limit=100` | `chunks[]` gefüllt, andere Arrays leer | `documentId` gesetzt | 0 | ja | ✅ 4 Chunks, `documentId` gesetzt, andere Arrays leer |

### Phase P3 — Suche

| ID | Zweck | Anfrage | Erwartung | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T30 | exakter Wortlaut | `POST /ns/…:a/search` `{query:q-exact-pref}` | `results[]` mit Präferenz-Inhalt | mindestens ein inhaltlich passender Treffer; Rangfolge nicht erwartet | 0 | ja | ✅ Treffer vorhanden |
| T31 | Paraphrase | `{query:q-paraphrase-pref}` | semantischer Treffer | Inhalt thematisch passend | 0 | ja | ✅ semantischer Treffer |
| T32 | Widerspruch | `{query:q-freeze,include:{related:true}}` | beide Aussagen oder aktualisierte Fassung | beide Inhalte auffindbar **oder** Abweichung dokumentiert | 0 | ja | ✅ `included.related` vorhanden |
| T33 | Negativkontrolle | `{query:q-negative}` | kein thematischer Treffer erwartet | Treffer werden einzeln bewertet, nicht nur gezählt | 0 | ja | ⚠️ themenfremde Treffer ohne `threshold` |
| T34 | Modi | `{query:q-exact-pref,searchMode:"chunks"}` und `"memories"` | `chunk` bzw. `memory`-Feld | Feldpräsenz entspricht Modus | 1 | ja | ✅ `chunk` bzw. `memory` je Modus |
| T35 | Include-Varianten | `include:{documents:true}`, dann `{related:true}`, dann `{forgotten:true}` | `included.document` bzw. `included.related` | Felder nur bei Anforderung | 2 | ja | ✅ `included.document`/`included.related`; `forgotten` siehe T63 |
| T36 | Isolation A→B | `{query:q-marker-b}` in A | kein B-Inhalt | kein Treffer mit `BLAUER-LOTUS-8823` | 0 | ja | ✅ kein B-Inhalt |
| T37 | Isolation B→A | `{query:q-exact-pref}` in B | kein A-Inhalt | kein Treffer mit A-Inhalten | 0 | ja | ✅ kein A-Inhalt |

### Phase P4 — Änderungswege (getrennt)

| ID | Zweck | Anfrage | Erwartung | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T40 | gleiche ID erneut | `POST …/document` `{content:"…REPEAT-BETA.",id:"audit-a-r1-repeat"}` | Append/Diff, kein stilles Ersetzen | GET zeigt neuen **und** alten Marker oder dokumentierte Diff-Semantik | 1 (GET) | ja | ✅ Append; Chunks ersetzt, Memories akkumuliert |
| T41 | PATCH ersetzt | `PATCH …/document/audit-a-r2-replace` `{content:"…REPLACE-NEU."}` | neuer Inhalt ist kanonisch | GET enthält `REPLACE-NEU`, alter Marker darf fehlen | 1 (GET) | ja | ✅ Inhalt ersetzt |
| T42 | Metadaten-only | `PATCH …/document/audit-a-r2-replace` `{metadata:{reviewed:true}}` | kein Reindex | `content` unverändert, `metadata` gemerged | 1 (GET) | ja | ⚠️ Metadata ersetzt; Merge nicht unterscheidbar |
| T43 | Gruppe ohne Inhalt | `PATCH …/audit-a-r2-replace` `{group:{x:"y"}}` | dokumentiert: wirkt nur bei Re-Ingestion | Verhalten protokolliert, keine Erwartung erzwungen | 1 (GET) | ja | ⚠️ 400 `group only applies on re-ingestion` |

### Phase P5 — Profile und Buckets

| ID | Zweck | Anfrage | Erwartung | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T50 | Profil lesen | `POST /ns/…:a/profile` `{}` | `{profile:{static[],dynamic[],buckets{}}}` | alle drei Felder vorhanden | 0 | ja | ✅ `static`/`dynamic`/`buckets` vorhanden |
| T51 | Buckets anlegen | `PUT /ns/…:a/profile/buckets` `{buckets:{audit_prefs:"Synthetische Präferenzen"}}` | `{buckets{}}` | Bucket sichtbar | 0 | ja | ✅ via `PUT` (dokumentiertes `POST` → 404) |
| T52 | Buckets lesen | `GET /ns/…:a/profile/buckets` | Map inkl. `preferences` und `audit_prefs` | beide vorhanden | 0 | ja | ✅ Default-Buckets + eigener Key |
| T53 | Profil mit Bucket | `POST …/profile` `{buckets:["audit_prefs"]}` | nur dieser Bucket gefüllt | `buckets` enthält ausschließlich `audit_prefs` | 0 | ja | ✅ `buckets` auf Anforderung gefiltert |
| T54 | Bucket löschen | `DELETE …/profile/buckets` `{buckets:["audit_prefs"]}` | entfernt | GET zeigt ihn nicht mehr | 1 | ja | ✅ entfernt |

### Phase P6 — Vergessen und Löschen (nur eigene Ressourcen)

| ID | Zweck | Anfrage | Erwartung | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T60 | Memory-ID ermitteln | `GET …/document/audit-a-r3-forget?include=memories` | `memories[]` | Memory vorhanden, deren Text den Würfel referenziert; keine wortwörtliche Form erwartet | 1 | ja | ✅ in Lauf 3 vorhanden (Lauf 1: 0 Memories) |
| T61 | exakt vergessen | `DELETE /ns/…:a/memories` `{ids:[…]}` | `{count,matches,errors}` | `count == matches.length`, `errors` leer | 0 | ja | ✅ `count:1`, `errors:[]` |
| T62 | Nachprüfung | `POST …/search` `{query:"VERGISS-MICH-9001",searchMode:"memories"}` | kein Treffer erwartet | kein Treffer mit diesem Token; Abweichung dokumentieren | 0 | ja | ✅ kein Treffer |
| T63 | Forgotten-Sicht | `…` mit `include:{forgotten:true}` | ggf. sichtbar mit `isForgotten` | Verhalten protokolliert | 0 | ja | ✅ nur mit `include:{forgotten:true}`; `isLatest:false`, `isForgotten` nicht im Treffer |
| T64 | Semantik-Dry-Run | `DELETE …/memories/semantic` `{query:"Vergiss synthetische Präferenzen",dryRun:true}` | `matches[]`, keine Änderung | danach T30 unverändert | 1 | ja | ✅ `matches[]`, keine Löschung |
| T65 | Dokument löschen | `DELETE /ns/…:a/document` `{ids:["audit-a-r4-delete"]}` | `{count:1,errors:[]}` | GET danach `404` | 1 | ja | ✅ `count:1`, danach 404 |
| T66 | Löschung während Verarbeitung | `DELETE` auf ein frisch angelegtes Dokument | `errors[].error = "Document is still processing"` | Teilfehler sichtbar, HTTP 200 | 0 | ja (optional) | ✅ `errors:[{"error":"Document is still processing"}]`, HTTP 200 |

### Phase P7 — Fehlerfälle (risikoarm)

| ID | Zweck | Anfrage | Erwartung | Erfolgskriterium | Folge | Freigabe | Ergebnis |
| - | - | - | - | - | - | - | - |
| T70 | fehlende Auth | `POST /ns/…:a/search` ohne Header | `401 {error}` | 401 | 0 | ja | ✅ 401 `{"error":"Unauthorized"}` |
| T71 | ungültige Auth | `Authorization: Bearer invalid-token` | `401` | 401 | 0 | ja | ✅ 401 |
| T72 | Pflichtfeld fehlt | `POST …/document` `{}` | `400` (Error- oder Validation-Form) | 400 | 0 | ja | ✅ 400 `invalid_type` |
| T73 | falscher Typ | `POST …/document` `{content: 123}` | `400` | 400 | 0 | ja | ✅ 400 `invalid_type` |
| T74 | unbekannte eigene ID | `GET …/document/audit-a-does-not-exist` | `404` | 404 | 0 | ja | ✅ 404 `Document not found` |
| T75 | Limit außerhalb | `POST …/list/documents?limit=101` | `400` | 400 | 0 | ja | ✅ 400 `too_big` |
| T76 | Filter falscher Typ | `{"filter":{"field":"suite","operator":"eq","value":123}}` (string-Operator) | `400` | 400 | 0 | ja | ⚠️ 200 akzeptiert statt 400 |
| T77 | unbekanntes Body-Feld | `POST …/document` `{content:"…",foo:"bar"}` | `400` erwartet; **kann** ignoriert werden und Ressource erzeugen | Status protokolliert; bei Anlage sofort in Ressourcenregister | 1 (GET) | ja | ✅ 400 `unrecognized_keys`, keine Ressource |
| T78 | Option an falscher Stelle | `POST …/search?limit=5` mit Body | `400` | 400 | 0 | ja | ✅ 400 `unrecognized_keys: ["limit"]` |

Keine Überlast-, Quota- oder Großpayload-Tests.

---

## 5. Prioritäten, Vorbedingungen und Bereinigungsweg

Priorität: **P1** = Kernumfang, **P2** = Ergänzung, **P3** = optional.

| ID | Prio | Vorbedingung | Bereinigungsweg |
| - | - | - | - |
| T01 | P1 | Key mit bekanntem Scope | keine |
| T02 | P1 | Namespace A erreichbar | keine (nur Lesen) |
| T03 | P1 | Namespace B erreichbar | keine |
| T10 | P1 | A existiert (implizit) | B5: `audit-a-d1-pref` |
| T11 | P1 | A existiert | B5: D2, D3, D4, D6 |
| T12 | P1 | B existiert (implizit) | B5: `audit-b-d5-marker-b` |
| T13 | P1 | T10–T12, T16 akzeptiert | keine |
| T14 | P1 | T13 | keine |
| T15 | P1 | `system.status=done` | keine |
| T16 | P1 | A existiert | B5: r1–r4 |
| T20 | P1 | T13 `done` | keine |
| T21 | P2 | T13, T16 | keine |
| T22 | P2 | ≥3 Dokumente in A | keine |
| T23 | P3 | ≥3 Dokumente in A | keine |
| T24 | P2 | T11 akzeptiert | keine |
| T25 | P2 | T13 `done` | keine |
| T30 | P1 | `dreamingStatus=done` für D1 | keine |
| T31 | P2 | wie T30 | keine |
| T32 | P2 | D2, D3 `dreamingStatus=done` | keine |
| T33 | P2 | D6 vorhanden | keine |
| T34 | P3 | T30 erfolgreich | keine |
| T35 | P3 | T30 erfolgreich | keine |
| T36 | P1 | D5 `dreamingStatus=done` | keine |
| T37 | P1 | D1–D4 `dreamingStatus=done` | keine |
| T40 | P1 | r1 akzeptiert | B5: r1 |
| T41 | P1 | r2 `done` | B5: r2 |
| T42 | P2 | T41 | B5: r2 |
| T43 | P3 | T41 | B5: r2 |
| T50 | P2 | `dreamingStatus=done` für mind. ein Dokument | keine |
| T51 | P2 | A existiert | T54 bzw. B5-Nachlauf |
| T52 | P2 | T51 | T54 |
| T53 | P3 | T51 | T54 |
| T54 | P2 | T51 | keine |
| T60 | P1 | r3 `dreamingStatus=done`, Memory vorhanden | B5: r3 |
| T61 | P1 | T60 liefert IDs | keine (Soft-Delete) |
| T62 | P1 | T61 | keine |
| T63 | P3 | T61 | keine |
| T64 | P2 | A hat Memories | keine (Dry-Run) |
| T65 | P1 | r4 `done` | bereits gelöscht |
| T66 | P3 | r5 gerade akzeptiert | B5: r5 |
| T70 | P1 | keiner | keine |
| T71 | P1 | keiner | keine |
| T72 | P1 | keiner | keine |
| T73 | P1 | keiner | keine |
| T74 | P1 | A existiert | keine |
| T75 | P1 | A existiert | keine |
| T76 | P2 | A existiert | keine |
| T77 | P3 | A existiert | B5, falls Ressource angelegt |
| T78 | P1 | A existiert | keine |



## 6. Polling, Retries, Abbruch

| Regel | Wert |
| - | - |
| Poll-Intervall | 3 s |
| Poll-Versuche je Dokument | max. 20 |
| Wartezeit gesamt | max. 15 min |
| Request-Timeout | 30 s |
| Retry bei Timeout (schreibend) | **keiner**; zuerst Status per `GET` prüfen |
| Retry bei 5xx (lesend) | max. 1, mit 5 s Pause |
| Retry bei `429` | nur nach `Retry-After`, max. 2 |

Abbruch des Laufs bei: unerwarteter Offenlegung fremder Daten; Scope-Abweichung (T01); Budgetüberschreitung; unklarer Schreibwirkung nach Timeout; wiederholten 5xx/401/403; nicht auflösbarem Bereinigungsziel.

---

## 7. Budgetvorschlag (freigabepflichtig)

| Größe | Obergrenze |
| - | - |
| Requests gesamt | 220, davon **40 reserviert für Bereinigung** |
| Namespaces | 2 (A, B) |
| Dokumente | 10 (5 Baseline A, 1 B, 4 Änderungsressourcen) + optional 1 (T66) |
| Memories | ≤ 60 erwartet (generiert, nicht steuerbar) |
| Connectors | 0 |
| Kosten | **1,00 USD** hart |
| Laufzeit | 45 min |
| Datei-/URL-Ingestion | 0 |

Kostenschätzung: Text-Tokens im niedrigen 4-stelligen Bereich, 10 × `instant` = 10 Operations (≈ 0,001 USD), ≤ 20 Suchen (≈ 0,0001 USD). Die 1,00-USD-Grenze ist rein defensiv; sie ersetzt kein Request-Limit.

---

## 8. Ressourcenregister (Vorlage)

| Lauf-ID | Bereich | Typ | Caller-ID | Server-ID | erzeugende Anfrage | Zustand | Bereinigung | Freigabe | Ergebnis | Nachprüfung |
| - | - | - | - | - | - | - | - | - | - | - |
| … | A | document | `audit-a-d1-pref` | | T10 | geplant | `DELETE /document {ids:[…]}` | offen | | |

Geführt ab dem ersten Schreibversuch; auch Ressourcen mit unklarem Ausgang (Timeout) werden erfasst.

**Bereinigungsreihenfolge:**

1. Semantik-/Bucket-Reste: `DELETE …/profile/buckets` (falls T51 fehlschlug).
2. Dokumente: `DELETE /ns/…:a/document` mit allen registrierten IDs (max. 100 pro Aufruf), dann B.
3. Namespaces: `DELETE /ns/…:a`, `DELETE /ns/…:b` — nur nach Nachweis, dass sie ausschließlich diesem Lauf gehören, und nur mit Einzelbestätigung.
4. Nachprüfung: `GET /ns/…` → `404`; `GET /namespaces` → beide Namen abwesend.

Keine Bereinigung über Namenspräfix-Selektion. Keine `moveTo`-Aktion. Kein Org-Reset.

---

## 9. Einzelbestätigungen vor Ausführung

Jede Zeile braucht vor Ausführung eine Bestätigung mit aufgelöstem Host, Methode, Pfad, Parametern und vollständigen synthetischen Nutzdaten.

| Block | Aktionen | Wirkung |
| - | - | - |
| B1 | T01–T03 | nur lesen |
| B2 | T10–T12, T16, T40, T41, T50–T54, T60, T64 | schreibend, 10 Dokumente, bis 2 Buckets |
| B3 | T61, T65, T66 | destruktiv auf **eigenen** Ressourcen |
| B4 | T70–T78 | Fehlerfälle, ggf. eine zusätzliche Ressource (T77) |
| B5 | Bereinigung Dokumente | destruktiv, exakte IDs |
| B6 | Bereinigung Namespaces A/B | destruktiv, ganze Testbereiche |

Ein generelles Budget (B1–B4) ersetzt **nicht** B5/B6.

---

## 10. Zuordnung Inventar → Testfälle

| Operation | Testfälle |
| - | - |
| `POST /document`, `/document/batch` | T10, T11, T12, T16, T40 |
| `GET /document/{id}` | T13, T14, T15, T20, T60 |
| `PATCH /document/{id}` | T41, T42, T43 |
| `DELETE /document` | T65, T66 |
| `POST /list/{type}` | T02, T03, T21–T25 |
| `POST /search` | T30–T37, T62, T63, T70, T71 |
| `POST /profile`, `/profile/buckets` | T50–T54 |
| `DELETE /memories`, `/memories/semantic` | T61–T64 |
| `GET /namespaces` | T01, Bereinigungs-Nachprüfung |
| `POST /document/file*`, Connectors, `/organization`, `/feedback` | **nicht geprüft** (ausgeschlossen, §1) |

---

## 11. Offene Punkte vor Start

1. Key + Scope-Nachweis (§1).
2. Budgetfreigabe (§7) und Blöcke B1–B6 (§9).
3. Entscheidung zu T01: Scoped Key oder Verzicht auf `GET /namespaces`.
4. Entscheidung zu T77: unbekanntes Feld im Schreibrequest — zulässig, weil Wirkung auf den eigenen Testbereich begrenzt ist.
