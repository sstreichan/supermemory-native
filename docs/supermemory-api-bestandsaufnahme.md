# Bestandsaufnahme: offizielle Supermemory-API (Dokumentationsrecherche)

**Ergebniskategorie:** Dokumentationsrecherche **und** Live-Bestandsaufnahme (§14, „partielle Live-Bestandsaufnahme“).
**Stand:** 2026-10-20, 12:16–12:24 UTC (Dokumentation), 12:31–12:37 UTC (Lauf 1), 12:49–12:55 UTC (Lauf 2), 13:20–13:26 UTC (Lauf 3, alle offenen Testfälle), 13:33–13:36 UTC (Lauf 4, Typfehler/Batch/Timeout/Rerank). Insgesamt ca. 145 Requests.
**Auftragsbezug:** §2, §6 und §8 des Prüfauftrags. Ausgeschlossen und dokumentiert in §11: organisationsweite Listen, Datei-/URL-Ingestion, Connectors, Org-Settings, Feedback.

---

## 1. Kurzfassung

- Aktuelle stabile öffentliche API ist **v5** (`info.version: 5.0.0` im OpenAPI-Dokument). v3/v4 sind als Legacy gekennzeichnet, Abschaltung angekündigt für **2026-12-31**.
- v5-Anwendungsrouten sind **nicht** mit `/v5` präfigiert; der Namespace ist Pfadsegment: `https://api.supermemory.ai/ns/{namespace}/...`. `/v5/reference` ist ausschließlich die interaktive Referenz (Scalar-Shell, lädt `/v5/openapi`).
- Das v5-OpenAPI-Dokument beschreibt **33 Operationen auf 22 Pfaden**. Die Dokumentationsübersicht nennt nur 7 Routenfamilien und ist damit unvollständig (siehe §7).
- Wiederverwendung einer caller-definierten `id` bedeutet **Append/Diff**, nicht Ersetzen. Ersetzen ist ausschließlich `PATCH /ns/{namespace}/document/{id}` (Text/URL) bzw. `POST .../document/file/{id}` (Datei, vollständig) und `PATCH .../document/file/{id}` (Datei, partiell).
- Dokumentverarbeitung und Memory-Bildung sind getrennt: `system.status` (Extraktion/Indexierung) und `system.dreamingStatus` (Memory-Generierung). `status: "done"` belegt keine fertigen Memories.
- §2–§7 sind **dokumentiert** (Quelle je Zeile). §8–§10 sind **live beobachtet** (Statuscodes, Körper, Latenzen, Isolation, Bereinigung). §11 listet, was bewusst nicht geprüft wurde.

---

## 2. Quellen, Abruf und Versionsstand

### 2.1 Abgerufene Quellen

| Quelle | URL | HTTP | Inhalt | Abruf (UTC) |
| - | - | - | - | - |
| v5-Übersicht | `https://supermemory.ai/docs/v5/api-reference/overview` | 200 | Markdown (Mintlify alternate) | 12:16 |
| v5-Referenz | `https://api.supermemory.ai/v5/reference` | 200 | HTML-Shell, `"url": "/v5/openapi"` | 12:22 |
| Legacy-Übersicht | `https://supermemory.ai/docs/api-reference/overview` | 200 | Markdown | 12:16 |
| Migrationsleitfaden | `https://supermemory.ai/docs/migration/api-v5` | 200 | Markdown | 12:16 |
| Changelog | `https://supermemory.ai/changelog/` | 200 | Markdown, Seite 1 (Aug 21 – Okt 7) | 12:16 |
| Dokumentationsindex | `https://supermemory.ai/docs/llms.txt` | 200 | Text | 12:16 |
| v5-OpenAPI | `https://api.supermemory.ai/v5/openapi` | 200, `application/json` | 190.825 Bytes, SHA-256 `4062a3b085eadfb7…` | 12:16 |
| Root-OpenAPI | `https://api.supermemory.ai/openapi.json` | 200 | **byte-identisch** zu `/v5/openapi` | 12:19 |
| v3-OpenAPI | `https://api.supermemory.ai/v3/openapi` | 200 | 3.0.0, 42 Ops / 33 Pfade, SHA-256 `9eaed44cf5d8e437…` | 12:19 |
| v4-OpenAPI | `https://api.supermemory.ai/v4/openapi` | 200 | **byte-identisch** zu `/v3/openapi` | 12:19 |

Ergänzend gelesen: `docs/authentication`, `docs/concepts/how-it-works`, `docs/concepts/container-tags`, `docs/recall/search`, `docs/recall/memory-operations`, `docs/recall/user-profiles`, `docs/ingestion/document-operations`, `docs/overview/billing`, `docs/overview/security`, `docs/connectors/overview` sowie die Migrationsunterseiten `api-v5-document-writes`, `-document-updates`, `-document-reads`, `-recall`, `-profiles`, `-memory-forgetting`, `-settings`, `-organization`, `-filters`.

Keine Weiterleitungen: alle geprüften URLs antworten direkt mit `200`, `num_redirects=0` (`curl -L`, `--max-time 20`). Mintlify liefert die Markdown-Fassung zusätzlich über die `.md`-Variante, nicht per Redirect.

### 2.2 Verifikation des Ausgangsstands

| Behauptung (Planrevision) | Ergebnis | Beleg |
| - | - | - |
| v5 ist die aktuelle API | bestätigt | v5-Übersicht: „latest Supermemory API“; v5-OpenAPI `version: 5.0.0`; Changelog 2026-10-07 „Supermemory API v5 is here“ |
| allgemeine Übersicht ist v3/v4-Legacy | bestätigt | Legacy-Übersicht: „This is the legacy v3/v4 reference.“ |
| Abschaltung 31.12.2026 | bestätigt | Legacy-Übersicht: „deprecated and will be shut down on **December 31, 2026**“ |
| v5-Anwendungsrouten ohne `/v5`-Präfix | bestätigt | v5-Übersicht (Routenliste), Migrationsleitfaden: „v5 application routes are unversioned“; alle 22 OpenAPI-Pfade beginnen mit `/ns`, `/namespaces`, `/organization`, `/connectors`, `/feedback` |
| `/v5/reference` ist kein Routenpräfix | bestätigt | Referenz-Shell enthält `"url": "/v5/openapi"`; `/reference` enthält `"url": "/openapi.json"` |
| `/reference` zeigt auf die neueste Version | bestätigt (strukturell) | `/openapi.json` ist byte-identisch zu `/v5/openapi` |

### 2.3 Versionsebenen (getrennt erfasst)

| Ebene | Wert | Quelle |
| - | - | - |
| Produkt-/Referenzversion | v5 | v5-Übersicht, `/v5/reference` |
| OpenAPI-Dokument | 3.1.0, `info.version: 5.0.0` | `/v5/openapi` |
| Base-URL | `https://api.supermemory.ai` | `servers[0]` |
| Auth-Schema | `bearerAuth`, `type: http`, `scheme: bearer` | `components.securitySchemes` |
| TypeScript-SDK | `supermemory` ≥ 5.0.0 (v5-Surface) | Migrationsleitfaden, SDK-Tabelle |
| Python-SDK | `supermemory` ≥ 5.0.0 | ebd. |
| CLI | `supermemory` 5.x | ebd. |
| Selbst gehosteter Server | `supermemory local` ≥ v0.0.9 für v5 | ebd. |
| Legacy-Referenz | v3/v4, EOL 2026-12-31 | Legacy-Übersicht |

Abrufzeitpunkte der Schemas: v5 2026-10-20T12:16:20Z (`Date`-Header), v3/v4 2026-10-20T12:19Z. Ein dokumentierter Lebenszyklus nutzt mehrere Routenversionen (Legacy-Spec enthält `/v3/*` und `/v4/*`); daraus wird **kein** einheitlicher Versionsvertrag abgeleitet.

---

## 3. Werkzeug- und Zugriffsstatus

| Prüfpunkt | Status | Befund |
| - | - | - |
| Beliebige Methoden/Bodies | vorhanden | `curl`/HTTP-Client im Shell-Werkzeug |
| Status, Header, Body zugänglich | vorhanden | `-D`, `-w`, Body-Datei |
| Redirects kontrollierbar | vorhanden | `-L` bewusst nicht Standard; `num_redirects` messbar |
| Credentials sicher einfügbar | vorhanden | Key per `curl -K` aus Datei mit `chmod 600`; nicht in URL, Body oder Ausgabe |
| Messdaten für Latenz/Polling | vorhanden | `time_total`/`time_starttransfer` verfügbar |
| Zielorganisation/Scope | teilweise geklärt | Schreiben in zwei frei gewählten Namespaces erfolgreich; Scope nicht über `GET /namespaces` geprüft (§11) |

**Konsequenz:** Live-Teil ausgeführt (T01 bewusst ausgelassen). Budget, Datensatz und Bereinigung: §8–§10.

---

## 4. Endpoint-Inventar (v5, 33 Operationen / 22 Pfade)

Legende: `*` = Pflichtfeld. „Async“ = Antwort bestätigt Annahme, Verarbeitung läuft danach weiter. Auth = Bearer-Token; Einschränkungen siehe §5.4. Alle Pfade relativ zu `https://api.supermemory.ai`.

### 4.1 Ingest

| # | Methode | Pfad | Zweck | Body (Pflicht) | Antwort | Async | Statuscodes |
| - | - | - | - | - | - | - | - |
| I1 | POST | `/ns/{namespace}/document` | Dokument anlegen oder an bestehende `id` anhängen | `content*`; optional `id`, `supportingContext`, `metadata`, `group`, `date`, `taskType`, `dreaming` | `{id, status}` | ja | 200, 400, 401, 402, 403, 409, 500 |
| I2 | POST | `/ns/{namespace}/document/batch` | 1–600 Dokumente | `documents*` (1…600); top-level `taskType`, `dreaming` | `{results[], count, failed}` | ja | 200, 400, 401, 402, 403, 500 |
| I3 | POST | `/ns/{namespace}/document/file` | Datei-Upload (`multipart/form-data`) | `file*`; optional `supportingContext`, `metadata`, `group`, `date`, `taskType`, `dreaming`, `fileType`, `mimeType` | `{id, status}` | ja | 200, 400, 401, 402, 403, 409, 500 |
| I4 | POST | `/ns/{namespace}/document/file/{id}` | Datei-Dokument **vollständig** ersetzen | `file*` | `{id, status}` | ja | 200, 400, 401, 402, 403, 404, 409, 500 |
| I5 | PATCH | `/ns/{namespace}/document/file/{id}` | Datei-Dokument **partiell** aktualisieren (`file` optional) | mind. ein Feld | `{id, status}` | ja | 200, 400, 401, 402, 403, 404, 409, 500 |

Felddetails (I1, identisch pro Batch-Item):
`content` string; `id` 1–255 Zeichen; `supportingContext` ≤ 1500 Zeichen; `metadata` object (string | number | boolean | string[]); `group` object (gleiche Werttypen); `date` ISO-8601-String; `taskType` enum `memory|superrag`; `dreaming` enum `dynamic|instant` (Default `dynamic`). `additionalProperties: false`.

`status`-Enum (Acceptance): `unknown, queued, extracting, chunking, embedding, indexing, done, failed`.

Datei-Teile: `metadata` und `group` sind **JSON-kodierte Strings**, `supportingContext`/`date`/`taskType`/`dreaming`/`fileType`/`mimeType` einfache Strings; `fileType` enum `text|pdf|image|video|audio`.

### 4.2 Content Management

| # | Methode | Pfad | Zweck | Parameter | Antwort | Statuscodes |
| - | - | - | - | - | - | - |
| C1 | GET | `/ns/{namespace}/document/{id}` | Dokument lesen, optional Chunks/Memories | `include` (Query, Array, `chunks`/`memories`, `explode=false` → Komma) | `{id, title, type, summary, content, metadata, system, chunks?, memories?}` (`title`/`summary`/`content` sind `null`-fähig; `chunks`/`memories` nur bei Anforderung) | 200, 400, 401, 403, 404, 500 |
| C2 | PATCH | `/ns/{namespace}/document/{id}` | Inhalt ersetzen / Metadaten ändern | Body: `content`, `supportingContext` (≤1500), `metadata`, `group`, `date`, `taskType`, `dreaming`; kein Feld required, Body selbst required | `{id, status}` | 200, 400, 401, 402, 403, 404, 409, 500 |
| C3 | DELETE | `/ns/{namespace}/document` | 1–100 Dokumente löschen | Body: `ids*` (1…100, je 1–255) | `{count, errors[{id,error}]}` | 200, 400, 401, 403, 500 |
| C4 | POST | `/ns/{namespace}/list/{type}` | Dokumente/Chunks/Memories listen | Pfad `type` enum `documents\|chunks\|memories`; Query `page`(≥1, Default 1), `limit`(1–100, Default 10), `sort` enum `createdAt\|updatedAt\|position` (Default `createdAt`), `order` enum `asc\|desc` (Default `desc`); Body `filter`, `include` | `{documents[], chunks[], memories[], pagination{currentPage, limit, totalItems, totalPages}}` | 200, 400, 401, 403, 500 |
| C5 | GET | `/ns/{namespace}/memories/{id}` | Eine Memory lesen | `include` (`related`/`documents`, Komma), `relatedLimit` (1–100, Default 10) | Memory-Objekt + `included` | 200, 400, 401, 403, 404, 500 |
| C6 | POST | `/ns/{namespace}/list/documents/memory-counts` | Live-Memory-Zahl je Dokument für ≤100 IDs | Body: `ids*` | `{counts[{id, memoryCount}]}` | 200, 400, 401, 403, 500 |
| C7 | DELETE | `/ns/{namespace}/memories` | Memories exakt vergessen (Soft-Delete) | Body: `ids*` (1…500) | `{count, matches[{id,memory}], errors[]}` | 200, 400, 401, 403, 500 |
| C8 | DELETE | `/ns/{namespace}/memories/semantic` | Semantisch vergessen, mit Dry-Run | Body: `query*` (1–2000), `dryRun*` (boolean) | `{count, matches[], errors[]}` | 200, 400, 401, 403, 500 |

Semantische Löschung: max. 100 Treffer pro Request; `errors` ist laut Doku immer leer; `count == matches.length`. Dokumentlöschung: nur Dokumente mit `system.status` `done` oder `failed` werden gelöscht, sonst `errors[].error = "Document is still processing"`.

`list.include` (Body, Defaults): `{forgotten: false, memoryCount: true}`. `filter` gilt vor der Pagination.

### 4.3 Search

| # | Methode | Pfad | Body | Defaults/Limits | Antwort | Statuscodes |
| - | - | - | - | - | - | - |
| S1 | POST | `/ns/{namespace}/search` | `query*` (≥1 Zeichen); `filter`, `searchMode`, `limit`, `include`, `threshold`, `rerank`, `rewriteQuery` | `searchMode=hybrid` (`hybrid\|memories\|chunks`), `limit=10` (1–100), `threshold=0.3` (0–1), `rerank=none` (`none\|order\|aggregate`), `rewriteQuery=false`, `include` alle `false` | `{results[], searchTime}` | 200, 400, 401, 402, 403, 500 |

`results[]`: `id`, `memory?`, `chunk?`, `metadata`, `similarity` (0–1), `isLatest`, `isInference`, `system{createdAt?, updatedAt}`, `included?{document?, related?{parents,children,siblings}}`.

Keine Query-Parameter. Fehlerhafte Platzierung von Optionen → `400` (dokumentiert).

### 4.4 Profiles

| # | Methode | Pfad | Body | Antwort | Statuscodes |
| - | - | - | - | - | - |
| P1 | POST | `/ns/{namespace}/profile` | optional `filter`, `buckets` (≤50 Namen) | `{profile{static[], dynamic[], buckets{}}}` | 200, 400, 401, 403, 500 |
| P2 | GET | `/ns/{namespace}/profile/buckets` | — | `{buckets{name: description}}` | 200, 400, 401, 403 |
| P3 | PUT | `/ns/{namespace}/profile/buckets` | `buckets*` object; Key `^[a-z0-9][a-z0-9_-]*$`, 1–64; Value ≤2000 | `{buckets{}}` | 200, 400, 401, 403, 404, 409, 500 |
| P4 | DELETE | `/ns/{namespace}/profile/buckets` | `buckets*` array, 1–50 | `{buckets{}}` | 200, 400, 401, 403, 404, 409, 500 |

`static`/`dynamic`/`buckets` werden immer geliefert; Einträge sind `{id, memory}`. Synthetische Zusammenfassungen tragen IDs mit Präfix `aggregated_`. Org-eigene Buckets sind über PUT/DELETE nicht änderbar (`409`).

### 4.5 Namespaces

| # | Methode | Pfad | Parameter | Antwort | Statuscodes |
| - | - | - | - | - | - |
| N1 | GET | `/namespaces` | `page`(≥1, Default 1), `limit`(1–100, Default 10) | `{namespaces[{id, namespace, documentCount, memoryCount, description, system{createdAt, updatedAt}}], pagination}` | 200, 400, 401, 403, 500 |
| N2 | GET | `/ns` | identisch zu N1 (Alias) | identisch | 200, 400, 401, 403, 500 |
| N3 | GET | `/ns/{namespace}` | — | `{namespace, supportingContext, system{createdAt, updatedAt}}` | 200, 400, 401, 403, 404, 500 |
| N4 | PATCH | `/ns/{namespace}` | `supportingContext*` (string ≤1500 **oder** `null`) | wie N3 | 200, 400, 401, 403, 404, 500 |
| N5 | DELETE | `/ns/{namespace}` | Query `moveTo` | `200 {status:"deleted", namespace, deletedDocumentsCount, deletedMemoriesCount}` **oder** `202 {status:"queued", operationId, namespace, moveTo}` | 200, 202, 400, 401, 403, 404, 409, 500 |

Namespace-Muster: `^[a-zA-Z0-9_:-]+$`, max. 100 Zeichen. Erste Schreiboperation legt den Namespace implizit an. `sm_project_default` ist nicht dauerhaft löschbar. `moveTo` ist Query-Parameter; ein Body wird bei `DELETE /ns/{namespace}` **ignoriert** (dokumentiert) — dadurch kann ein irrtümlich im Body gesendetes `moveTo` zu einer dauerhaften Löschung führen.

### 4.6 Connectors

| # | Methode | Pfad | Parameter | Antwort | Statuscodes |
| - | - | - | - | - | - |
| K1 | GET | `/connectors` | `provider` (enum), `page`, `limit` | `{connectors[], pagination}` | 200, 400, 401, 403, 500 |
| K2 | POST | `/ns/{namespace}/connectors` | Body-`oneOf` je Provider: `provider*`; OAuth-Provider zusätzlich `redirectUrl`, optional `documentLimit`; Config-Provider (`web-crawler`, `s3`, `granola`) zusätzlich `config*` | 201 `{id, authorization{url, expiresAt}\|null}` | 201, 400, 401, 402, 403, 500, 503 |
| K3 | GET | `/ns/{namespace}/connectors` | `provider`, `page`, `limit` | `{connectors[], pagination}` | 200, 400, 401, 403, 500 |
| K4 | GET | `/ns/{namespace}/connectors/{id}` | `include` (`syncs`/`picker`), `returnUrl` | Connector-Objekt | 200, 400, 401, 403, 404, 500 |
| K5 | PATCH | `/ns/{namespace}/connectors/{id}` | `selection`, `documentLimit` (1–10000) | Connector-Objekt | 200, 400, 401, 403, 404, 500 |
| K6 | DELETE | `/ns/{namespace}/connectors/{id}` | `deleteDocuments` (bool, Default `true`) | `{id, deleted}` | 200, 400, 401, 403, 404, 500 |
| K7 | POST | `/ns/{namespace}/connectors/{id}/sync` | — | 202 `{id, status}` | 202, 400, 401, 402, 403, 404, 409, 500, 503 |

Provider-Enum: `notion, google-drive, onedrive, gmail, github, web-crawler, s3, granola`. `include=picker` erfordert Admin mit Schreibrecht. `sync` liefert `409`, solange ein Sync läuft.

### 4.7 Organization & Feedback

| # | Methode | Pfad | Body | Antwort | Statuscodes |
| - | - | - | - | - | - |
| O1 | GET | `/organization` | — | `{organizationalContext, namespaceCount}` | 200, 401, 403, 500 |
| O2 | PATCH | `/organization` | `organizationalContext*` (string ≥1 **oder** `null`) | wie O1 | 200, 400, 401, 403, 500 |
| F1 | POST | `/feedback` | `type*` (`bug\|feature_request\|docs\|other`), `title*` (1–120), `description*` (1–5000); optional `endpoint`, `requestId`, `expected`, `actual`, `client`, `contactEmail` | 202 `{id, remaining}` | 202, 400, 401, 403, 429, 502, 503 |

`/feedback` erlaubt anonyme Requests mit verwertbarer Client-IP; Rate-Limit dokumentiert (3/Tag pro Org bzw. IP, Pools 200/Tag authentifiziert, 20/Tag anonym) mit `Retry-After` bei `429`. `namespaceCount` ist informativ und nicht per PATCH änderbar.

---

## 5. Gemeinsame Verträge

### 5.1 Filter

Ein einziger optionaler `filter` (nicht `filters`) für Search, List und Profile:

| Operator | Werttyp | Zusatz |
| - | - | - |
| `eq` / `neq` | string \| number \| boolean | `caseSensitive` (Default `true`) nur bei string |
| `gt` / `gte` / `lt` / `lte` | number | — |
| `contains` / `notContains` | string | `caseSensitive` (Default `true`) |
| `arrayContains` / `arrayNotContains` | string | exaktes Array-Mitglied |
| `and` / `or` | `operands[]` (1–200) | max. 5 Verschachtelungsebenen |

Feldnamen: `^[a-zA-Z0-9_.-]+$`; Punkte sind Teil des Namens, kein Pfadzugriff. Unbekannte Keys oder falsche Werttypen → `400` (dokumentiert).

### 5.2 IDs, Idempotenz, Asynchronität

- **Dokument-ID**: caller-definiert (`id`, 1–255) oder serververgeben; Auflösung nur innerhalb des Namespace im Pfad.
- **Memory-ID**: serververgeben; Versionen via `isLatest`/`version`; Forgotten-Flag `isForgotten`.
- **Connector-ID**, **`operationId`** (Namespace-Move), **Chunk-ID**.
- **Kein dokumentierter Idempotency-Key.** Wiederholung mit gleicher `id` ist Append/Diff; Ersetzen erfordert PATCH/POST-File. Ein Timeout bei einem Schreibrequest hat damit ein unbekanntes Ergebnis.
- **Asynchron**: alle Ingest-Routen (Acceptance + `status`), Namespace-Move (`202 queued` + `operationId`), Connector-Sync (`202`). Synchron: Search, Profile, List, Get, Delete-Dokumente, Forget, Namespace-Delete ohne `moveTo`.

### 5.3 Fehlerformat

- `ErrorResponse`: `{error: string, details?: string}`.
- `ValidationErrorResponse`: `{success: false, error: [{message, path?}], data?}`.
- `400` ist `oneOf` aus beiden. Fehlende Header sind nicht rekonstruierbar; ein dokumentierter „request id“-Header existiert nicht.
- `402` = Credits/Quota erschöpft; `403` = Feature-/Plan-Gate oder fehlendes Namespace-Recht; `409` = Konflikt (Namespace-Merge, Dateipfad, laufender Sync, geschützter Bucket).

### 5.4 Auth und Berechtigungen

- Ausschließlich `Authorization: Bearer <key>`; Keys aus `console.supermemory.ai`.
- **Scoped Keys**: auf einen Namespace oder eine Liste begrenzt; erlaubte Routen: `/ns/{namespace}/document`, `/list`, `/search`, `/profile`, `/memories`, `/namespaces`, `/ns/{namespace}` sowie benannte Legacy-Routen. Andere Routen → `403`. Scoped Keys dürfen weder Billing lesen noch Org-Settings ändern noch Keys erzeugen. Namespace-Scoping ist **kein** Read-only.
- Namespace-Löschung/Move: Org-Admin/Owner mit Schreibrecht an Quelle (und Ziel). `PATCH /organization`: Admin.
- Namespace ist laut Doku eine Autorisierungsgrenze; Doku behauptet eigene Vektorindizes pro Namespace (Produktaussage, nicht live geprüft).

### 5.5 Tarif-/Kostenabhängigkeiten (dokumentiert)

| Vorgang | Meter |
| - | - |
| Ingest `taskType: memory` | `sm_tokens_text` / `sm_tokens_rich` (nur neue/geänderte Tokens bei gleicher `id`) |
| Ingest `taskType: superrag` | `sm_superrag_text` / `sm_superrag_rich` |
| `dreaming: instant` | +1 Operation pro Dokument |
| Search | `sm_search_queries` +1; `rewriteQuery`, `rerank` je +1 Operation bei Ausführung |
| Profile (v5) | kein Search-Query-Meter (aktuelle Implementierung) |
| Connector-Features | planabhängig (`403`) |

---

## 6. Ressourcen- und Verarbeitungsmodell (dokumentiert)

```
content (Text/URL/Datei)
  → Validierung → Annahme (id, status)
  → status: queued → extracting → chunking → embedding → indexing → done|failed
  → dreamingStatus: dreaming → done            (separate Phase)
  → outputs: chunks (Quelle), memories (Graph), profile (static/dynamic/buckets)
```

- `system.status` und `system.dreamingStatus` sind getrennt; `status: done` ≠ Memories fertig.
- `dreaming: dynamic` (Default) bündelt Dokumente; `instant` verarbeitet sofort und kostet eine Extra-Operation.
- `taskType: superrag` erzeugt keine Memories.
- `group` wirkt nur bei (Re-)Ingestion als Kontextauswahl; `metadata` wird für Filter verwendet.
- Profile werden kontinuierlich gepflegt, nicht pro Request berechnet (Produktaussage).

---

## 7. Differenzen und Auffälligkeiten zwischen Quellen

| # | Beobachtung | Quellen | Bewertung |
| - | - | - | - |
| 1 | v3- und v4-OpenAPI sind **byte-identisch** und deklarieren beide `info.version: 3.0.0`, obwohl die Datei `/v4/*`-Routen enthält | `/v3/openapi`, `/v4/openapi` | Versionsfeld der Legacy-Spec ist nicht aussagekräftig; nicht als v4-Version interpretieren |
| 2 | v5-Übersicht listet nur 7 Routenfamilien; die Spec enthält zusätzlich `/connectors`, `/feedback`, `/ns`, `/ns/{namespace}`, `/memories/{id}`, `/memories/semantic`, `/list/documents/memory-counts` und die Datei-Routen | v5-Übersicht vs. `/v5/openapi` | Übersicht ist unvollständig; Spec ist für das Inventar maßgeblich |
| 3 | `llms.txt` listet OpenAPI nur unter `/v4/openapi` und `/v5/openapi`; die Legacy-Übersicht nennt `/v3/openapi` | `llms.txt`, Legacy-Übersicht | Dokumentationsindex ist kein Vollständigkeitsbeleg |
| 4 | Für `GET /ns/{namespace}/memories/{id}` und `POST /ns/{namespace}/list/documents/memory-counts` existiert **keine** eigene Referenzseite im Index | `llms.txt` vs. Spec | Zwei Operationen sind nur über die Spec dokumentiert |
| 5 | PUT-Buckets: Doku „one to 50 entries“, Schema hat **kein** `minProperties`/`maxProperties` (nur Key-/Value-Grenzen) | `api-v5-profiles` vs. Spec | Obergrenze im Schema nicht abgebildet; Laufzeitverhalten unbekannt |
| 6 | `include.documents` in Search: „most recently updated source document“ (Recherche-Doku) vs. „most relevant source document“ (Migrationsleitfaden) | `recall/search` vs. `api-v5-recall` | Semantik unklar; nicht live geprüft |
| 7 | `status`-Enum enthält `unknown` in Spec/Doku, die Pipeline-Tabelle der How-it-works-Seite führt ihn nicht | `how-it-works` vs. Spec | Kein Widerspruch, aber unvollständige Beschreibung |
| 8 | Ingest-Routen: „sending a [query parameter] returns 400“ ist dokumentiert, im Schema aber nicht abbildbar | `api-v5-document-writes` | Nur Dokumentationsaussage |
| 9 | `PATCH /ns/{namespace}/document/{id}`: Doku fordert „any non-empty subset“, das Schema hat keinen `required`-Eintrag und `additionalProperties: false` | `document-operations` vs. Spec | Validierung ist nicht schema-erschöpfend beschrieben |
| 10 | Legacy-Spec enthält `/feedback` ohne Versionspräfix, obwohl sie als v3-Spec ausgeliefert wird | `/v3/openapi` | Hinweis auf gemeinsame Basis; keine Verhaltensaussage |
| 11 | Keine allgemeine Rate-Limit-Seite im Dokumentationsindex; Limits nur für Feedback und für Scoped Keys (`rateLimitMax` Default 500 / 60000 ms) dokumentiert | `llms.txt`, `authentication` | Rate-Limits der Kernrouten sind **nicht dokumentiert** |
| 12 | `include` bei `GET /document/{id}`: Schema `explode: false` (Komma-Liste), Migrationsleitfaden „comma-separated“, Beispiel in `document-operations` nutzt `?include=chunks&include=memories` (wiederholter Parameter) | Spec vs. `api-v5-document-reads` vs. `document-operations` | **live 2026-10-20:** beide Formen liefern Chunks und Memories; Widerspruch ohne Laufzeitfolge |
| 13 | `system.dreamingStatus` fehlt in den generierten Modellen von TS-SDK 5.0.1 und Python 5.0.0; typisiert erst ab TS 5.0.2 | `document-operations` (Note) vs. Spec | SDK- und HTTP-Vertrag weichen ab; Roh-HTTP für Monitoring nutzen oder SDK anheben |
| 14 | Filter-`value` ist im Schema `string`; live wird `value: 123` mit `200` und leerem Ergebnis akzeptiert statt mit `400` abgelehnt | Spec vs. Lauf 3 | Typ wird nicht erzwungen; Fehlkonfiguration bleibt still |

---

## 8. Live-Ausführung: Rahmen

| Punkt | Wert |
| - | - |
| Zeitraum | 2026-10-20 12:31–12:37 UTC (Lauf 1), 12:49–12:55 UTC (Lauf 2), 13:20–13:26 UTC (Lauf 3), 13:33–13:36 UTC (Lauf 4) |
| Host | `https://api.supermemory.ai` (kein Redirect, kein Proxy) |
| Zugang | ein API-Key über geschützte curl-Konfigurationsdatei; **kein** Key in URL, Body, Bericht oder Antwortausgabe |
| Umfang | ~95 Requests, 2 Namespaces, 10 Dokumente, 0 Datei-/URL-Ingestion, 0 Connectors |
| Budget | eingehalten (§7-Grenzen: 220 Requests, 10 Dokumente, 1,00 USD); Kosten im Bereich von 10 `instant`-Operations und ~25 Suchanfragen |
| Werkzeug | `curl` mit `-K` (Header aus Datei), `--max-time 30`, kein `-L` |
| Testfall-IDs | T01–T78 gemäß `docs/supermemory-api-testmatrix.md` |

Es wurden **keine** organisationsweiten Listen abgerufen (`GET /namespaces` wurde ausgelassen, §1-Grenze). Ein falscher Key wurde nur zum Nachweis von `401` verwendet.

## 9. Live-Beobachtungen (2026-10-20)

Quellenbasis aller Zeilen: **live beobachtet**. Statuscodes und Körper sind aus den Antworten übernommen; Anfrage-IDs sind mangels `request-id`-Header vom Prüfer vergeben.

### 9.1 Ingestion

| Operation | Status | Beobachtung |
| - | - | - |
| `POST /ns/{ns}/document` ohne `Content-Type: application/json` | 400 | `invalid_type` auf `content`; **kein** `415` |
| `POST /ns/{ns}/document` | 200 | `{id, status:"queued"}` |
| `POST …/document/batch` mit `dreaming` je Dokument | 400 | `unrecognized_keys: ["dreaming"]` – strikt je Dokument |
| `POST …/document/batch` mit `dreaming` auf oberster Ebene | 200 | `{results[], count:4, failed:0}` |
| `POST …/document` mit unbekanntem Feld `foo` | 400 | `unrecognized_keys: ["foo"]` – Body ist strikt |
| `POST …/document` mit `metadata: {suite: 123}` | 200 | Zahl akzeptiert, **Dokument wurde angelegt** (Bereinigung erfolgt) |
| `POST …/document` mit vorhandener `id` | 200 | Anhängen: kanonischer Inhalt wird zusammengeführt (`REPEAT-ALPHA` + `NEU-9921`) |

### 9.2 Lesen, Status, Verarbeitung

| Operation | Status | Beobachtung |
| - | - | - |
| `GET /ns/{ns}` (nicht existierend) | 404 | Namespace wird erst durch Schreiben implizit erzeugt |
| `POST …/list/documents` (nicht existierender Namespace) | 200 | leere Arrays, `pagination.totalItems: 0` – **kein** 404 |
| `POST …/list/documents` mit `limit` im Body | 400 | `unrecognized_keys: ["limit"]`; Pagination ist Query-Parameter |
| `POST …/list/documents?limit=101` | 400 | `too_big`, `maximum: 100` |
| `POST …/list/documents?limit=2&page=2&order=desc` | 200 | `totalPages: 5`; Default-Sortierung `createdAt desc` |
| `POST …/list/documents` mit `filter {field,operator:"eq",value}` | 200 | 2 Treffer; Operator-Enum live `eq`/`neq` |
| `POST …/list/documents` mit `filter {suite:{value}}` | 400 | `invalid_union` – Filterform aus v3 gilt nicht |
| `GET …/document/{id}` | 200 | `system.status`/`system.dreamingStatus` getrennt; `chunks`/`memories` **leer**, solange nicht per `include` angefordert |
| `GET …/document/{id}?include=chunks,memories` und `?include=chunks&include=memories` | 200 | beide Kodierungen liefern Daten (§7 Nr. 12 damit ohne Laufzeitfolge) |
| `GET …/document/{id}` auf fremden Namespace | 404 | Isolation zwischen A und B bestätigt |
| Verarbeitungsdauer | — | `queued` → `done` in 15–45 s; `dreamingStatus` erreicht `done` **nach** `status: done` |

### 9.3 Suche, Profil, Buckets

| Operation | Status | Beobachtung |
| - | - | - |
| `POST …/search` | 200 | `{results[], searchTime}`; `searchTime` in ms (146–147); Trefferfeld `similarity` (0–1), **nicht** `score` |
| `searchMode: "memories"` | 200 | Trefferobjekt mit `memory` |
| `searchMode: "chunks"` | 200 | Trefferobjekt mit `chunk` |
| `searchMode: "hybrid"` | 200 | gemischte Objektformen in einer `results`-Liste |
| `POST …/search` mit `query: ""` | 400 | `too_small`, `minimum: 1` |
| `POST …/search` mit `filter` | 200 | Filter greift; nur passende `metadata.suite` |
| Suche ohne `threshold` | 200 | liefert `limit` Treffer unabhängig von Ähnlichkeit (themenfremde Anfrage erreichte `similarity` 0.68) |
| `POST …/profile` mit `{}` | 200 | `{profile:{static:[], dynamic:[…]}}`; dynamische Einträge mit `[YYYY-MM-DD] `-Präfix |
| `GET …/profile` | 404 | `No route matches GET …` – **Dokumentation nennt GET** |
| `GET …/profile/buckets` | 200 | 7 Default-Buckets (`preferences`, `tech_stack`, …) mit Beschreibungstext |
| `PUT …/profile/buckets` `{buckets:{key:desc}}` | 200 | Merge; Default-Buckets bleiben erhalten |
| `POST …/profile/buckets` | 404 | `No route matches POST …` – **Dokumentation nennt POST** |
| `DELETE …/profile/buckets` `{buckets:[key]}` | 200 | löscht; `DELETE …/profile/buckets?key=…` → 400, `{buckets:{key:null}}` → 400 |

### 9.4 Ändern und Löschen

| Operation | Status | Beobachtung |
| - | - | - |
| `PATCH …/document/{id}` mit `content` | 200 | `{id, status:"queued"}`; kanonischer Inhalt **ersetzt**, Chunks neu erzeugt |
| `PATCH …/document/{id}` nur mit `metadata` | 200 | Metadata-Objekt ersetzt, `status` bleibt `done` (keine Neuverarbeitung) |
| `PATCH …/document/{id}` mit `{}` | 400 | `custom: "At least one document field must be provided"` |
| `PATCH …/document/{unbekannt}` | 404 | `{"error":"Document not found"}` |
| `DELETE …/document/{id}` | 404 | `No route matches DELETE …` – **Einzellöschung nicht vorhanden** |
| `DELETE …/document` `{ids:[…]}` | 200 | `{count, errors[]}`; Sammellöschung |
| `DELETE …/memories` `{ids:[…]}` | 200 | `{count, errors[], matches[]}` |
| `DELETE …/memories` erneut, gleiche `id` | 200 | `count: 0`, `errors:[{error:"Memory not found"}]` |
| `DELETE /ns/{ns}` | 200 | `{status:"deleted", namespace, deletedDocumentsCount, deletedMemoriesCount}` |

### 9.5 Verhaltensdetails (abgeleitet aus den Live-Antworten)

- **Anhängen vs. Ersetzen:** gleiche `id` per `POST` → Text zusammengeführt, Chunks zu **einem** Chunk mit alt+neu neu erzeugt, Memories akkumulieren (zwei Einträge, beide `isLatest: true`). `PATCH` → alter Text und alte Chunks verschwinden.
- **Forget ≠ Dokumentlöschung:** nach `DELETE /memories` war der Memory-Treffer in `searchMode=memories` weg, der Chunk im Modus `chunks` weiterhin auffindbar.
- **Dokumentlöschung entfernt Memories:** nach der Sammellöschung lieferte die Suche nach dem Dokumentmarker keinen Memory-Treffer mehr.
- **Memory-Erzeugung ist selektiv:** das Dokument `Merkwürfel zum Vergessen: VERGISS-MICH-9001.` erzeugte **0** Memories; die Forget-Prüfung wurde deshalb auf ein anderes Memory verlegt.
- **Sprache:** Memories und `summary` werden englisch formuliert, auch bei deutschem Eingabetext.
- **Memory-Metadaten:** `temporalContext {documentDate, eventDate}`, `temporalEventStartMs/EndMs`, `isStatic`, `isInference`, `isLatest`, `isForgotten`, `version`.

### 9.6 Transport und Fehlerformate

| Beobachtung | Detail |
| - | - |
| Fehlerkörper (Validierung) | `{data, error:[{code, path, message, …}], success:false}` |
| Fehlerkörper (Routing) | `{"error":"Not found","message":"No route matches …","documentation":"https://api.supermemory.ai/openapi.json"}` |
| Ungültiger Key | `401`, kein Body erfasst |
| Response-Header | **keine** `RateLimit-*`, **keine** `Retry-After`, **keine** `x-request-id`; vorhanden: `Server-Timing` (`auth.getSession+fetchOrg`, `auth.memberAccess`, `auth.pluginCheck`), `CF-RAY`, `Vary: Origin` |
| Latenzen | Lesen 0,26–1,5 s; Schreiben 0,7–2,0 s (gemessen mit `time_total`) |

### 9.7 Organisationsweite Namespace-Liste

| Operation | Status | Beobachtung |
| - | - | - |
| `GET /namespaces?limit=1` | 200 | `{namespaces[], pagination{currentPage,limit,totalItems,totalPages}}`; `totalItems: 34` |
| Elementfelder | — | `id`, `namespace`, `description`, `documentCount`, `memoryCount`, `system{createdAt,updatedAt}` – nur Metadaten, keine Inhalte |
| Sortierung | — | absteigend nach `system.updatedAt` |
| `limit=101` | 400 | `too_big`, `maximum: 100` (gleiche Grenze wie `list`) |
| `page=2&limit=1` | 200 | `currentPage: 2`, `totalItems: 34` |
| Scope des Keys | — | sichtbar sind **alle 34** Namespaces der Organisation, nicht nur die Testbereiche → der Key ist organisationsweit, nicht auf Testbereiche gescoped |
| Bereinigung, org-weit nachgeprüft | — | `smapi-20261020-t1:a`/`:b` sind in der Liste **nicht** enthalten |

Folge für die Aussagekraft: Die Isolationstests (§9.2) belegen die Trennung über das Namespace-Pfadsegment, **nicht** eine Begrenzung des Keys. Mit diesem Key wäre fremder Inhalt technisch lesbar; er wurde nicht abgerufen, und es wurden keine Namen oder Beschreibungen in diesen Bericht übernommen.

### 9.8 Zweiter Lauf: Optionen, Fehlerpfade, Suche (12:49–12:55 UTC)

Eigener Namespace `smapi-20261020-t2:a`, 4 Dokumente, danach gelöscht.

| Operation | Status | Beobachtung |
| - | - | - |
| `POST …/document` mit `date`, `group`, `supportingContext`, `taskType`, `dreaming` | 200 | alle Felder akzeptiert; `GET` zeigt `metadata.temporalContext.documentDate: "2025-01-15"` und Fenster `1736899200000`–`1736985600000` |
| `group` im Request | — | wird im `GET /document` **nicht** zurückgegeben; Ablage nicht beobachtbar |
| `taskType: "superrag"` | 200 | `status: done`, `dreamingStatus: done`, 1 Chunk, **0 Memories** → bestätigt „ohne Memory-Bildung“ |
| `dreaming: "dynamic"` (Default) | — | einzelnes Dokument nach >90 s weiter `dreaming`, 0 Memories |
| `dreaming: "instant"` | — | Memory nach ca. 35 s vorhanden |
| `POST …/list/documents/memory-counts` `{ids:[…]}` | 200 | `{counts:[{id, memoryCount}]}` |
| `GET …/memories/{id}` | 200 | Memory-Objekt wie in `list/memories` |
| `GET …/memories/{unbekannt}` | 404 | `{"error":"Memory not found"}` |
| `DELETE …/document` **während Verarbeitung** | 200 | `{count:0, errors:[{error:"Document is still processing"}]}`; **kein** 409, kein Retry-Hinweis |
| `DELETE …/document` nach Abschluss | 200 | `count:1`; Löschung ist damit zeitabhängig, nicht dauerhaft verweigert |
| `POST …/document/batch` mit doppelter `id` im selben Request | 200 | `count:2`, `failed:0`, **kein** 409; beide Einträge `queued` |
| `search` mit `threshold: 0.9` | 200 | 0 Treffer |
| `search` mit `threshold: 0` | 200 | 3 Treffer (`similarity` 0.75 / 0.70 / 0.52) → `threshold` filtert tatsächlich |
| `search` mit `include: {documents: true}` | 200 | Treffer erhält `included.document {id, title, type, summary, metadata, system}`; **kein** Top-Level-`documents` (Feldname Singular vs. Flag Plural) |
| `search` mit `rerank` | 200 | Enum live `none`/`order`/`aggregate`; `"bogus"` → 400 `invalid_value` |
| `search` mit `rewriteQuery: true` | 200 | akzeptiert; `similarity` derselben Anfrage änderte sich (0.702 → 0.759), Rangfolge nicht unabhängig geprüft |

**Einordnung:** `rerank` und `rewriteQuery` sind syntaktisch bestätigt, ihre Wirkung auf die Rangfolge nicht isoliert nachgewiesen. `group` ist ohne Ausgabe im `GET` nicht verifizierbar.

### 9.9 Dritter Lauf: geschlossene Testfälle (13:20–13:26 UTC)

Namespaces `smapi-20261020-t3:a` und `:b`, 5 Dokumente, danach gelöscht.

| Testfall | Status | Beobachtung |
| - | - | - |
| T23 Sortierung | 200 | `sort=createdAt&order=asc` liefert aufsteigende `system.createdAt` |
| T25 Chunks listen | 200 | `POST …/list/chunks` füllt nur `chunks[]`, `documentId` gesetzt |
| T32 `include.related` | 200 | Treffer erhält `included.related`; widersprüchliche Aussagen beide auffindbar |
| T37 Isolation B→A | 200 | Suche in B liefert ausschließlich B-Inhalt |
| T43 `PATCH {group}` | 400 | `"group only applies on re-ingestion; this update does not replace the document content"` – nicht ignoriert, sondern abgelehnt |
| T53 Profil mit Bucket-Filter | 200 | `profile.buckets` enthält nur den angeforderten Schlüssel; `static`/`dynamic` weiter vorhanden |
| T63 Forgotten-Sicht | 200 | vergessenes Memory erscheint nur mit `include:{forgotten:true}`; im Treffer `isLatest:false`, `isForgotten` **nicht** gesetzt |
| T64 Semantik-Dry-Run | 200 | `{count, errors[], matches[]}`; Dry-Run löscht **nicht** (Bestand unverändert) |
| T70 fehlende Auth | 401 | `{"error":"Unauthorized"}` – andere Form als Validierungsfehler |
| T72 leerer Body | 400 | `invalid_type` auf `content` |
| T76 Filter mit Zahl | **200** | `value: 123` wird akzeptiert und liefert ein leeres Ergebnis; die dokumentierte `400`-Erwartung tritt **nicht** ein |
| T78 Query-Parameter an `/search` | 400 | `unrecognized_keys: ["limit"]`; der Parameter wird als Body-Feld geparst und abgelehnt |

Zusatzbefund: `POST …/list/memories` blendet vergessene Memories **standardmäßig aus** (Bestand fiel nach dem Vergessen von 4 auf 3, obwohl `search` sie mit `include:{forgotten:true}` noch liefert).

### 9.10 Vierter Lauf: Typfehler, Batch-Grenze, Timeout, Rerank (13:33–13:36 UTC)

Namespace `smapi-20261020-t4:a`, 4 Dokumente, danach gelöscht.

| Prüfpunkt | Status | Beobachtung |
| - | - | - |
| T73 `content: 123` | 400 | `invalid_type`, `expected string, received number` |
| Batch mit 601 Einträgen (leere Objekte) | 400 | Fehler an `documents[0].content` – die Größengrenze wird **nicht zuerst** geprüft; es entstand kein Dokument |
| Schreib-Timeout (Client bricht nach 410 ms ab) | 000 | Verbindung abgebrochen, keine Antwort |
| Zustand danach | 200 | `GET …/document/audit-t4-to` → Dokument **existiert**; der Schreibvorgang war trotz Client-Timeout wirksam |
| Wiederholung derselben `id` mit identischem Inhalt | 200 | `status: "done"`, Inhalt unverändert – kein Duplikat bei identischem Text; ohne Idempotency-Key bleibt die Wirkung im Allgemeinen unklar |
| `rerank: "none"` | 200 | absteigend nach `similarity` (0.832 / 0.703 / 0.547 / 0.546 / 0.522) |
| `rerank: "order"` | 200 | Reihenfolge weicht ab: Positionen 3 und 4 getauscht (0.522 vor 0.546) → Reranking wirkt |
| `rerank: "aggregate"` | 200 | fügt einen synthetischen Treffer `aggregated_0_…` mit `similarity: 0.832` ein |

**Folge für Retries:** Ein Client-Timeout beweist nicht, dass kein Schreibvorgang stattfand. `GET` vor jedem Retry ist erforderlich; die `id`-basierte Append-Semantik verhindert nur bei identischem Text ein sichtbares Duplikat.

## 10. Bereinigungsstatus

| Ressource | Aktion | Ergebnis |
| - | - | - |
| Dokumente A (`smapi-20261020-t1:a`) | `DELETE …/document {ids:[…]}` (9 IDs) | `count: 9`, `errors: []` |
| Dokumente A, Einzellöschung `audit-a-r4-delete` | `DELETE …/document {ids:[…]}` | `count: 1`, `errors: []` |
| Dokument B | `DELETE …/document {ids:[…]}` | `count: 1`, `errors: []` |
| Memory (`suite: neg`) | `DELETE …/memories {ids:[…]}` | `count: 1` |
| Bucket `audit_suite`, `audit_b` | `DELETE …/profile/buckets {buckets:[…]}` | 200, aus der Liste entfernt |
| Namespace A/B | `DELETE /ns/{ns}` | `{status:"deleted", deletedDocumentsCount: 0, deletedMemoriesCount: 0}` |
| Namespace `smapi-20261020-t2:a` (Lauf 2, 4 Dokumente) | `DELETE …/document {ids:[…]}`, danach `DELETE /ns/{ns}` | `count: 4`, `errors: []`; Namespace `deleted` |
| Namespaces `t3:a`/`t3:b` (Lauf 3, 5 Dokumente) | `DELETE …/document {ids:[…]}`, danach `DELETE /ns/{ns}` | `count: 4` und `count: 1`, `errors: []`; beide `deleted` |
| Namespace `t4:a` (Lauf 4, 4 Dokumente) | `DELETE …/document {ids:[…]}`, danach `DELETE /ns/{ns}` | `count: 4`, `errors: []`; Namespace `deleted` |
| Nachprüfung | `GET /ns/{ns}` → **404**; `POST …/list/documents` → `totalItems: 0`; `GET /namespaces?limit=100` → 34, kein Name mit Präfix `smapi-` | alle Test-Namespaces entfernt |

Kein Testrest verbleibt. Nicht bereinigt werden konnten nur die Default-Buckets (nicht löschbar, waren nicht Teil des Tests).

## 11. Nicht geprüft / blockiert

| Bereich | Status | Grund |
| - | - | - |
| `GET /namespaces` (organisationsweite Liste) | **geprüft** (2026-10-20) | auf ausdrückliche Anweisung; 200, 34 Namespaces, nur Struktur/Anzahl protokolliert, keine Namen |
| Scope des Keys | **geklärt** | organisationsweit (34 Namespaces sichtbar); Namespace-Isolation belegt Routing, nicht Key-Scoping |
| Datei-/URL-Ingestion (`/document/file*`) | **nicht geprüft** | §1/§4 schließen Datei-Ingestion aus |
| Connectors (OAuth, Sync, Picker) | **nicht geprüft** | zusätzliche Freigabe und externe Konten nötig |
| `PATCH /organization`, Namespace-Move, Org-Settings | **nicht geprüft** | Admin-/destruktive Wirkung, nicht freigegeben |
| `/feedback` | **nicht geprüft** | schreibt in Herstellerprozess |
| Rangfolge-Wirkung von `rerank`/`rewriteQuery` | **nur syntaktisch** | akzeptiert und 200; keine unabhängige Relevanzbewertung |
| Rate-Limits der Kernrouten | **nicht dokumentiert** | keine `RateLimit-*`-Header, keine Doku-Seite; keine Lasttests (§1) |
| Idempotenz bei Timeouts | **teilweise** | Client-Abbruch nach 410 ms: Schreibvorgang wirksam; Retry mit gleicher `id` und identischem Text ohne Duplikat; kein Idempotency-Key vorhanden |
| Batch-Obergrenze 600 | **nicht abschließend** | 601 ungültige Einträge scheitern an der Item-Validierung, nicht an der Grenze; 601 gültige Dokumente bewusst nicht erzeugt (§1) |
| `402`/`429` | **nicht ausgelöst** | kein Abrechnungs- oder Lastfall erzeugt |
| Scoped-Key-Endpunkte (`/auth/scoped-key/*`) | **nicht geprüft** | Key-Typ unbekannt, Änderung am Key nicht freigegeben |
| Deep-Pagination | **teilweise** | `page=2` und `totalPages` geprüft, kein vollständiger Durchlauf |
| T73 `content` mit falschem Typ | **geprüft** | 400 `invalid_type` |

Ein nicht geprüfter Punkt ist **kein** Beleg dafür, dass die Funktion fehlt.

Testfall-IDs und Zuordnung Operation → Testfall: `docs/supermemory-api-testmatrix.md` (§4, §10).

---

## 12. Grenzen der Aussagekraft

- Der Bericht trennt **dokumentierten Vertrag** (§3–§7) von **beobachtetem Verhalten** (§8–§10); jede Live-Zeile ist einer ausgeführten Anfrage zuordenbar.
- Die Ausführung ist **eine** Momentaufnahme (2026-10-20, ein Konto, ein Key, ein Rechenzentrum) und kein Last- oder Dauertest.
- Ohne `request-id`-Header des Dienstes sind Anfrage-IDs prüferseitig; eine serverseitige Zuordnung ist nicht möglich.
- Rohprotokolle liegen als bereinigte Werkzeugausgabe vor (keine Header-Werte mit Geheimnissen, kein Key); Körper sind unverändert bis auf Kürzungen, die als solche markiert sind.
- Produktaussagen (eigene Vektorindizes pro Namespace, „hard isolation“, automatische Profilpflege) sind Herstellerangaben; beobachtet wurde nur die Isolation über Namespaces und die Existenz von `profile`-Ausgaben.
- Der Key wurde im Chat übergeben und ist damit als exponiert zu behandeln; er sollte nach Abschluss rotiert werden.
