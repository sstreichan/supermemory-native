# Plan: Eigenständiger lokaler Dashboard-Bau (mit Bun Build-Workflow)

## Hintergrund & Motivation

Die als Referenz vorliegenden Dashboard-Dateien (`docs/supermemory/index.html` und `docs/supermemory/local-console.js`) sind die vom offiziellen Supermemory-Server ausgelieferten Assets und stammen aus einer separaten, nicht öffentlichen Codebase. Deren Verwendung ohne ausdrückliche Lizenz ist rechtlich unsicher.

**Ziel:** Ersetze diese Assets durch eine selbstständig entwickelte, minimale Dashboard-Implementierung, die ausschließlich die lokalen API-Endpunkte des `supermemory-native`-Daemons verwendet.

**Abgrenzung:** Die Referenzdateien bleiben unverändert in `docs/supermemory/` liegen und werden **nicht** ausgeliefert (kein `go:embed` auf dieses Verzeichnis). Ob sie zusätzlich aus dem Repository entfernt werden, ist ein separater Lizenzentscheid und nicht Teil dieses Plans.

---

## Phasen

### Phase 1: API-Analyse & Backend-Voraussetzungen

Folgende Endpunkte werden für das Dashboard benötigt. **Wichtig:** Routen mit dem Status *Zu implementieren* existieren aktuell noch nicht in `internal/api/handler.go` und müssen dort ergänzt werden. Die GET-Routen dürfen dabei **nicht pauschal alle Pfade freigeben** (siehe Phase 6), damit bestehende 405-Antworten (z. B. `GET /v3/documents`) erhalten bleiben.

| Endpoint | Methode | Request Body | Response | Status im Backend |
|---|---|---|---|---|
| `/` | GET | — | HTML (`index.html`) | *Zu implementieren* |
| `/local-console.js` | GET | — | JS (`local-console.js` Bundle) | *Zu implementieren* |
| `/v3/config` | POST | `{}` | `{"success": true, "port": 6767}` | *Zu implementieren* (liefert die Daten für die Statusanzeige im Header) |
| `/v3/documents` | POST | `{"content":"...","containerTag":"..."}` | `{"success":true,"id":"..."}` | Bereits vorhanden |
| `/v3/documents/documents` | POST | `{"page":1,"limit":100,"sort":"createdAt","order":"desc"}` | `{documents:[{id,title,summary,status,createdAt,updatedAt,memories:[],memoryEntries:[{id,memory,createdAt,isStatic}]}],pagination:{totalItems,totalPages,currentPage}}` | *Zu implementieren* (Aggregiert aus SQLite `memories`) |
| `/v3/container-tags/list` | GET | — | `[{name,containerTag,documentCount,memoryCount,emoji}]` | *Zu implementieren* (GROUP BY `container_tag`) |
| `/v4/search` | POST | `{"q":"...","containerTag":"..."}` | `{results:[{id,memory,similarity,updatedAt}],total}` | Vorhanden (Achtung: Feld heißt `q`, optional `query` als Alias unterstützen) |
| `/v4/profile` | POST | `{"q":"...","containerTag":"..."}` | `{profile:{static:[],dynamic:[]},searchResults:{results:[...],total}}` | Vorhanden (Achtung: Feld heißt `q`, optional `query` als Alias unterstützen) |
| `/v4/memories` | DELETE | `{"containerTag":"...","id":"..."}` ODER `{"content":"...","containerTag":"..."}` | `{"success":true}` | Bereits vorhanden |
| `/v3/sync` | POST | `{}` | `{"success":true,"message":"Vault synchronization completed successfully"}` | Bereits vorhanden |

> **Hinweis:** Das offizielle Referenz-Bundle fragt tatsächlich nur `/v3/container-tags/list` und `/v3/documents/documents` ab. Alle übrigen Zeilen sind für den erweiterten Funktionsumfang des neuen Dashboards vorgesehen, nicht für die 1:1-Kompatibilität mit dem Original.

---

### Phase 2: Dashboard-Architektur (Bun Build)

- **Vanilla JavaScript (ES6+)** als Basis-Sprache (kein React/Vue Overhead, keine externen CDN-Abhängigkeiten)
- **Bun als Build-Tool** für Bundling + Minification (`bun build --target browser --minify --format iife`). **Wichtig:** Ohne explizites `--format iife` gibt `bun build` für `--target browser` standardmäßig ein ES-Modul (ESM) aus; sobald das Quellmodul einen Export enthält, endet das Bundle auf `export{...}` (mit Bun 1.3.5 verifiziert), und ein klassisches `<script src="/local-console.js" defer>` scheitert mit `SyntaxError: Unexpected token 'export'`. Alternative wäre das Laden als Modul (`<script type="module" ...>`); dieser Plan nutzt bewusst IIFE + klassisches Script.
- **Asset-Pipeline:**
  1. Quellcode wird gepflegt in `internal/api/assets/src/` (`index.html`, `local-console.js`); CSS/Design liegt inline in `index.html`, da die Pipeline bewusst keinen separaten CSS-Schritt hat
  2. Bun bündelt und minifiziert `src/local-console.js` nach `internal/api/assets/local-console.js`; `src/index.html` wird nur unverändert nach `internal/api/assets/index.html` kopiert (nicht minifiziert)
  3. Die gebauten Artefakte (`index.html`, `local-console.js`) werden im Git-Repository eingecheckt, damit Go-Builds (`go build`, Dockerfile, GitHub Actions Release-Workflow) ohne Bun-Laufzeitumgebung lauffähig bleiben
  4. Einbettung in das Go-Binary via `go:embed`
- **Keine externen Abhängigkeiten** — vollständige Offline-Fähigkeit (kein Google Fonts, keine externen CDNs). Die Referenz-HTML lädt heute Google Fonts und ein `/local-assets/memory-field.webp`; beides entfällt, da keine `/local-assets/*`-Route vorgesehen ist. Das neue `index.html` darf daher außer `/local-console.js` keine externen Ressourcen referenzieren.
- **Sicheres DOM-Handling:** Verzicht auf unsichere inline `onclick`-Attribute mit String-Interpolation; Nutzung von Event-Delegation (`addEventListener`) und konsequentem Escaping (`textContent` statt ungefiltertem `innerHTML`) zum Schutz vor XSS
- **Ziel-Größe:** < 40 KB für das kompilierte JS-Bundle (weiches Ziel); das Go-Test-Gate ist auf **50 KB** festgelegt (siehe Phase 8)
- **Hinweis zum Tree-Shaking (mit Bun 1.3.5 verifiziert):** Bun optimiert Code des Einstiegspunkts aggressiv weg, wenn er keinen Top-Level-Seiteneffekt hat. Ein Einstiegspunkt, dessen Logik nur aus ungenutzten Exporten oder nicht aufgerufenen Funktionen besteht, schrumpft auf einen leeren IIFE-Loader/Wrapper (gemessen: ~12 Bytes ohne Exports, ~0,5 KB bei Export-Interop, ~100 Bytes für eine leere Datei) — also **nicht** auf 0 Bytes. Das Modul muss daher einen Top-Level-Seiteneffekt besitzen (z. B. `DOMContentLoaded`-Handler). Weil der Wrapper auch bei wegoptimierter Logik nie leer ist, genügt eine reine "nicht leer"-Prüfung als Gate nicht (siehe Phase 8, Test 3).

### Dateistruktur

```text
internal/api/assets/
├── src/
│   ├── index.html           ← Entwicklerversion (HTML/CSS)
│   └── local-console.js     ← Entwickler-JS (ES6+, modular)
├── index.html               ← Gebauter / kopierter Output für go:embed
├── local-console.js         ← Gebauter minifizierter Output für go:embed
├── assets.go                ← Neu zu erstellen: nur go:embed-Deklaration (Auslieferung erfolgt in handler.go)
├── build.sh                 ← Bun-Build-Skript (Linux/macOS)
├── build.ps1                ← Bun-Build-Skript (Windows PowerShell)
└── package.json             ← Bun/NPM Build-Scripts ("build": "bun build ...")
```

---

### Phase 3: index.html (`src/index.html`)

Semantisches, modernes HTML5 & CSS:
- Header mit Statusanzeige (Daemon-Port, Status – gespeist aus `POST /v3/config`)
- Statistik-Karten (Gesamtzahl Dokumente, Erinnerungen, Container-Tags)
- Suchleiste mit Relevanz-Scores und Live-Ergebnisanzeige
- Dokumenten- & Erinnerungs-Tabelle mit Paginierung
- Container-Tags-Sidebar inklusive Zählern
- Leichtgewichtiger SVG-Memory-Graph für Beziehungen zwischen Dokumenten und Fakten
- Schnelleingabe-Formular für neue Erinnerungen
- Lade-Indikatoren und Fehler-Banner
- Tab-/Hash-Routing `#overview` (Default) und `#memory` wie in der Referenz (nötig, damit die Vorher/Nachher-Aufnahmen deckungsgleich sind)
- Einbindung des Bundles via `<script src="/local-console.js" defer></script>` (nur mit dem IIFE-Build aus Phase 2/5 gültig)

### Design-Spezifikation (Referenz-Treue)

Ziel: Das neue Dashboard soll **möglichst wie die Referenz aussehen**. Übernommen wird das komplette visuelle System samt Seiten-Shell (Hero, Tabs, Kartenraster, Mono-Labels, Marquee/Baseboard, Fenster-Chrome der Memory-Karte). **Nicht** übernommen wird der Marketing-Inhalt der Referenz (SDK-/curl-Snippets, Plugin-Akkordeons, Demo-API-Keys) — dessen Plätze füllen die funktionalen Blöcke aus Phase 3.

Referenz-Baseline: `docs/dashboard-design/before/*.png` (Aufnahme siehe „Design-Verifikation").

**1. Design-Tokens** — wörtlich als `:root` in `src/index.html` übernehmen:

```css
:root{
  /* Flächen & Linien */
  --line:#dfdfdf; --line-soft:#ececec; --chip:#c0c0c0;
  --paper:#ffffff; --warmwhite:#fffbf5; --tint:rgba(5,98,239,.07);
  /* Text & Marke */
  --ink:#111113; --body:#6f6f6f; --warm:#79716b; --sub:#9194a1;
  --blue:#0562ef; --success:#16a34a;
  /* Codeblock */
  --code-bg:#0b0c10; --code-text:#e8e8ee; --code-muted:#83869a;
  /* Raster */
  --inset:clamp(20px,3vw,48px); --overlap:96px;
  /* Schatten */
  --shadow-card:0 0 0 1px rgba(9,17,32,.05),0 1px 2px rgba(9,17,32,.05),0 12px 32px -12px rgba(5,34,91,.16);
  --shadow-card-deep:0 0 0 1px rgba(9,17,32,.05),0 2px 4px rgba(9,17,32,.06),0 24px 56px -20px rgba(5,34,91,.28);
  --shadow-window:0 0 0 1px rgba(9,17,32,.07),0 2px 6px rgba(9,17,32,.10),0 40px 80px -28px rgba(4,24,96,.50);
}
body{background:#fafafa;color:var(--ink);font-size:15px;line-height:1.55;-webkit-font-smoothing:antialiased;min-height:100vh}
::selection{background:rgba(5,98,239,.18)}
```

**2. Typografie** — die Referenz lädt Geist, Geist Mono und Space Grotesk von Google Fonts; das ist offline nicht zulässig (Phase 2). Deshalb werden System-Stacks auf die bestehenden Variablennamen gemappt:

```css
--font-sans:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;
--font-display:var(--font-sans);
--font-mono:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,"Liberation Mono",monospace;
```

Für die Optik sind die **Metriken** ausschlaggebend — diese unverändert übernehmen:
- `h1`: display, 700, `clamp(42px,6.2vw,78px)`, `line-height:.98`, `letter-spacing:-.035em`, `max-width:16ch`, weiß, `text-shadow:0 2px 24px rgba(4,24,96,.35)`; hervorgehobenes Wort (`.gradient`) in `#dbe9ff`.
- `.subtitle`: 16.5px, `rgba(255,255,255,.88)`, `max-width:56ch`.
- Labels (`summary-label`, `card-label`, `section-eyebrow`, `mem-stat-label`, Panel-`h3`): mono 10–11px, uppercase, `letter-spacing:.12em`; Eyebrows in `--blue`, neutrale Labels in `--sub`/`--warm`.
- Fließtext: 13–14px in `--body`.
- Statistik-Zahlen: display 700, 44px, `letter-spacing:-.03em`, `font-variant-numeric:tabular-nums`.
- `.section-title`: display 600, 24px, `letter-spacing:-.015em`.
- Optionaler Upgrade-Pfad für exakte Glyphen: Geist/Geist Mono/Space Grotesk sind OFL-lizenziert — als WOFF2 vendorn und per data-URI-`@font-face` inline in `index.html` einbetten (kostet keine JS-Bundle-Größe, benötigt aber einen zusätzlichen Inline-Schritt in Phase 5 und eine Attributionszeile). Default bleibt der System-Stack.

**3. Seiten-Shell (Reihenfolge und Maße)**
1. `.wrap`: `max-width:1240px; margin:0 auto; border-left/right:1px solid var(--line); min-height:100vh; display:flex; flex-direction:column`.
2. `.hero`: `background-color:#6d9ee8`, `padding:30px var(--inset) calc(var(--overlap) + 56px)`, `border-bottom:1px solid var(--line)`; Overlay via `::before` mit `linear-gradient(180deg,rgba(4,24,96,.34) 0%,rgba(4,24,96,.12) 42%,rgba(4,24,96,.02) 75%)`. Die Referenz legt zusätzlich `/local-assets/memory-field.webp` darüber — entfällt (keine `/local-assets/*`-Route); die flache `#6d9ee8`-Fläche plus Overlay ist der Ersatzzustand.
3. `header`: flex, `gap:12px`, `margin-bottom:54px`, weiß; Logo-Box 27×27 px (eigenes neutrales Inline-SVG, siehe 7.); Brand „supermemory · local" display 600/17px, `letter-spacing:-.01em`; `.status-pill` rechts (`margin-left:auto`).
4. `.tabs`: zwei Chips, `display:inline-flex; gap:8px; margin:0 0 44px`.
5. Zwei Hero-Copy-Blöcke (`#hero-overview`, `#hero-memory`) mit `h1` + `.subtitle`, per `hidden` umgeschaltet.
6. `.content`: `padding:0 var(--inset); flex:1`.
7. Overview-Panel: `.resource-band` → `.summary-row` → `.section-grid` → `.links` → `footer`.
8. `.marquee` + `.baseboard` als Wrap-Abschluss.
9. Memory-Panel: `#memory-root` (`margin-top:calc(-1 * var(--overlap))`) → `.mem-graph-card` → `.mem-statbar` → `.mem-panels`.

**4. Komponenten (zu übernehmende Werte)**

| Komponente | Werte |
|---|---|
| `.status-pill` | Höhe 26px, Padding 0 11px, `border:1px solid rgba(255,255,255,.45)`, Radius 3px, mono 11px, uppercase, `ls:.1em`, `background:rgba(255,255,255,.12)`, `backdrop-filter:blur(6px)`; `.status-dot` 6px, `#4ade80`, Puls 2.4s |
| `.tab-btn` | Höhe 30px, Padding 0 16px, Radius 3px, mono 11px, uppercase, `ls:.1em`; `.active`: Fläche `--paper`, Text `--blue`, 600, plus zwei 5px-Diamant-Notches (`rotate(45deg)`) oben-links/unten-rechts |
| `.resource-band` | `margin:calc(-1 * var(--overlap)) 0 14px` (Karten brechen 96px über die Hero-Kante), ab 760px Grid `1.35fr 1fr 1fr`, `gap:14px` |
| `.resource-link` | `min-height:118px`, Padding 18px, Radius 10px, `--shadow-window`, flex column/space-between; `.primary` = `--blue`-Fläche mit weißer Schrift |
| `.resource-label` | display 600, 18px, `letter-spacing:-.01em`; `.resource-desc` 13px |
| `.summary-row` / `.summary-card` | Grid `repeat(auto-fit,minmax(220px,1fr))`, `gap:1px` auf `--line`-Grund (=Haarlinien), Radius 10px, `overflow:hidden`, `--shadow-card`, `margin-bottom:72px`; Karte Padding 14px 16px; Label mono 10px uppercase `--sub`; Wert mono 12.5px |
| `.card-block` | `--paper`, Radius 10px, Padding 16px, `--shadow-card` |
| `.copy` / `.copy-btn` | `--code-bg`/`--code-text`, mono 12.5px, Radius 8px, Padding `14px 16px` (rechts 88px für den Button); Button top/right 10px, Radius 5px, mono 10px |
| `.plugin` (`<details>`) | Radius 10px, `--shadow-card`, offen `--shadow-card-deep`; `summary` Padding 14px 16px; `.plugin-logo` 38px, Radius 9px; Chevron rotiert auf `90deg` in `--blue` |
| `.mem-graph-card` | `--paper`, Radius 10px, `--shadow-window`; Titelleiste (`::before`) 38px `#f5f5f4`; macOS-Punkte (`::after`) `#ff5f57 / #febc2e / #28c840` (18px Abstand) |
| `.mem-statbar` | Grid `1fr`, ab 760px `1fr 1fr 1fr auto`; Zellen mit 1px `--line-soft`-Trennern; `.mem-stat-value` display 700, 44px, tabular-nums; `.mem-stat::after` Punkt-Raster 34×10px |
| `.mem-panels` | Grid `1fr`, ab 920px `1fr 1fr 1fr` |
| `.mem-row` / `.mem-dot` | Flex, `gap:10px`, Padding 9px 0, `border-top:1px solid var(--line-soft)`; `.mem-dot` 7px, `rotate(45deg)`, `--blue` (`.static` `#8b5cf6`) |
| `.mem-tag-chip` | mono 11.5px, `--tint`, `border:1px solid rgba(5,98,239,.25)`, Radius 3px, Padding 3px 9px |
| `.mem-loading` | zentriert, 12px-Ring (2px `--line`, Oberkante `--blue`), 700ms linear |
| `.baseboard` | Höhe 56px, `border-top:1px solid var(--line)`, `--warmwhite`, Punkt-Raster `radial-gradient(circle,#d9d2c7 1px,transparent 1.2px)`, `14px 14px` |
| `.marquee` | `border-top:1px solid var(--line)`, `--warmwhite`, Padding 8px 0; Text mono 10.5px, uppercase, `ls:.14em`, `--warm`; Lauf `46s linear infinite` |

**5. Raster & Responsive**
- Basis: einspaltig; `--inset:clamp(20px,3vw,48px)` für alle horizontalen Abstände.
- `≥760px`: `.resource-band` → `1.35fr 1fr 1fr`; `.mem-statbar` → `1fr 1fr 1fr auto`.
- `≥920px`: `.section-grid` → `1fr 1fr`; `.mem-panels` → `1fr 1fr 1fr`.
- `.wrap` bleibt 1240px breit und mittig, darunter mit den 1px-Seitenlinien.

**6. Interaktion & Zustände**
- Transitions: 140ms ease (Karten/Buttons/Pillen), 160ms (Chevron), 240ms Tab-Panel („tab-in": `opacity 0→1`, `translateY(8px)→0`).
- Hover: `.resource-link` `translateY(-2px)`; `.tab-btn` `rgba(255,255,255,.22)`; `.mem-refresh` Text `--blue` auf `--tint`.
- Animationen: Status-Puls 2.4s, Marquee 46s, Spinner 700ms.
- Tab-/Hash-Routing: `#overview` (Default) und `#memory`; die Memory-Ansicht ist der Ort für den SVG-Graph.
- Semantik: Tabs als `<button type="button">` mit `aria-label` am Tab-Streifen und an der Graph-Fläche.

**7. Bewusst abweichend**
- Keine Google Fonts, kein `/local-assets/memory-field.webp` und **keine externen Requests** (Phase 2).
- Keine Übernahme der Marken-/Produktgrafiken (supermemory-Logo, Plugin-Logos): stattdessen ein eigenes neutrales Inline-SVG in gleicher Box (27×27 px) und Strichstärke.
- Keine Demo-API-Keys/Hardcodings aus den Referenz-Snippets.

---

### Phase 4: local-console.js (`src/local-console.js`)

Modulare Struktur:
- **Hilfsfunktionen:** `fetchJson(url, options)`, `timeAgo(date)`, `truncate(text, length)` — kein `escapeHtml` nötig, da ausschließlich `textContent` statt `innerHTML` verwendet wird
- **State-Verwaltung:** `documents`, `tags`, `searchResults`, `activeFilter`, `pagination`
- **Rendering & DOM:** Sichere Render-Funktionen (`renderStats`, `renderDocuments`, `renderTags`, `renderSearchResults`, `renderGraph`) mittels `textContent` und Event-Delegation
- **API-Interaktion:**
  - `loadConfig()` (`POST /v3/config`)
  - `loadDocuments(page)` (`POST /v3/documents/documents`)
  - `loadContainerTags()` (`GET /v3/container-tags/list`)
  - `performSearch(query, containerTag)` (`POST /v4/search` mit Fallback auf `/v4/profile`; beide erwarten den Container-Tag im Request-Body)
  - `addMemory(content, containerTag)` (`POST /v3/documents`)
  - `deleteMemory(id, containerTag)` (`DELETE /v4/memories`)
  - `triggerSync()` (`POST /v3/sync`)
- **Initialisierung:** Selbstausführendes Modul / DOMContentLoaded-Handler

---

### Phase 5: Build-Skripte (`build.sh`, `build.ps1`, `package.json`)

Cross-Plattform Bun-Build-Pipeline:
1. `src/local-console.js` → `bun build ./src/local-console.js --minify --target browser --format iife --outfile ./local-console.js`
2. `src/index.html` → Kopieren nach `./index.html`
3. Ausgabe der Dateigrößen und Validierung
4. `package.json` im Asset-Ordner bereitstellen (ausschließlich Bun, damit der Workflow ohne Node.js-Laufzeit funktioniert):

   ```json
   {
     "name": "supermemory-dashboard",
     "scripts": {
       "build": "bun run build:js && bun run build:html",
       "build:js": "bun build ./src/local-console.js --minify --target browser --format iife --outfile ./local-console.js",
       "build:html": "bun -e \"require('fs').copyFileSync('src/index.html', 'index.html')\""
     }
   }
   ```

`build.sh` und `build.ps1` rufen jeweils `bun run build` auf und prüfen anschließend die Bundle-Größe.

---

### Phase 6: Go Backend-Integration (`assets.go` & `handler.go`)

1. **`internal/api/assets/assets.go` neu anlegen:**
   ```go
   package assets

   import "embed"

   //go:embed index.html local-console.js
   var Content embed.FS
   ```
2. **`internal/db/sqlite.go` (und `internal/memory/engine.go`) erweitern:** Die neuen Endpunkte brauchen Datenzugriffe, die noch **nicht** existieren. `GET /v3/container-tags/list` benötigt ein `GROUP BY container_tag` inklusive Zähler, `POST /v3/documents/documents` eine tag-übergreifende, paginierte Liste inklusive Gesamtzahl (`COUNT(*)`). Für solche Listen sind nur `ListMemories(containerTag, limit)` (taggebunden) und `GetAllMemories()` vorhanden — beide ohne Offset/Total. Also z. B. `ListContainerTags()`, `CountMemories()` und `ListMemoriesPage(limit, offset)` in `internal/db/sqlite.go` ergänzen und über `Engine` bereitstellen. `ListMemoriesPage` liefert die in der Anfrage angeforderte Sortierung; für den Referenz-Client (`sort=createdAt&order=desc`) ist `ORDER BY created_at DESC` der Default.
3. **Port an den Handler durchreichen:** `POST /v3/config` soll den tatsächlichen Port liefern. Dieser wird aktuell ausschließlich in `cmd/supermemory-native/main.go` ermittelt (`PORT`, Default `6767`) und nie an den Handler übergeben (`api.NewHandler(engine)`). **Gewählter Weg:** `Handler` um ein `Port`-Feld erweitern, `NewHandler(eng, port)` und `main.go` entsprechend anpassen. Da `os.Getenv("PORT")` einen String liefert, ist der Wert in der Response als Zahl zu serialisieren, damit sie dem Schema `{"success": true, "port": 6767}` aus Phase 1 entspricht. **Achtung:** `NewHandler` wird auch im Test-Setup (`setupTestHandler` in `internal/api/handler_test.go`) als `NewHandler(eng)` aufgerufen — dieser Aufruf muss beim Signaturwechsel mit angepasst werden, sonst kompiliert das `api`-Testpaket nicht mehr. **Alternative (nur falls die Signatur unverändert bleiben soll):** Port im Handler per `os.Getenv("PORT")` mit Default `6767` lesen; dann entfällt die Signaturänderung, und der Default muss mit dem in `main.go` übereinstimmen.
4. **`internal/api/handler.go` erweitern:**
   - **Methoden-Routing statt globaler Freigabe:** Statt `GET` pauschal zu erlauben, wird die erlaubte Methode pro Route geprüft. `GET` ist nur für `/`, `/local-console.js` und `/v3/container-tags/list` zulässig; alle übrigen Pfade behalten bei `GET` die Antwort `405 Method Not Allowed` (damit bleibt `TestHandlerOPTIONSAndMethodNotAllowed` grün).
   - Routing für:
     - `GET /` → Liefert `index.html` (`Content-Type: text/html; charset=utf-8`)
     - `GET /local-console.js` → Liefert `local-console.js` (`Content-Type: application/javascript; charset=utf-8`)
     - `GET /v3/container-tags/list` → Ermittelt Tags und Zähler aus der SQLite-Datenbank
     - `POST /v3/documents/documents` → Liefert paginierte Dokumente/Memories
     - `POST /v3/config` → Liefert `{"success": true, "port": <PORT>}`
   - Die statischen Routen setzen den `Content-Type`-Header **nach** dem globalen JSON-Default, da `ServeHTTP` aktuell zu Beginn `Content-Type: application/json` setzt.
   - In `queryRequest` neben `q` optional auch `query` unterstützen für maximale Client-Kompatibilität.
   - `handler.go` importiert dafür `github.com/supermemory-native/supermemory-native/internal/api/assets` (kein Import-Zyklus, da `assets` nur `embed` nutzt).

---

### Phase 7: Build & Deployment

1. **Lokale Entwicklung (Cross-Plattform):**
   - Windows PowerShell:
     ```powershell
     cd internal\api\assets; .\build.ps1; cd ..\..\..
     go build .\cmd\supermemory-native
     ```
   - Linux/macOS:
     ```bash
     cd internal/api/assets && ./build.sh && cd ../../.. && go build ./cmd/supermemory-native
     ```
2. **Docker & CI/CD Integration:**
   - Da die kompilierten Assets (`index.html`, `local-console.js`) versioniert im Repository liegen, bleibt das bestehende `Dockerfile` (`golang:1.25-alpine`) sowie die GitHub Actions (`.github/workflows/release.yml`) schlank und benötigt keine zusätzliche Node/Bun-Installation in den Go-Containern.
   - **Bereits erledigt:** `.github/workflows/release.yml` setzt `go-version: '1.25'` und passt damit zu `go.mod` (`go 1.25.0`) sowie zum `Dockerfile` (`golang:1.25-alpine`). Eine Anhebung ist nicht mehr nötig; anderenfalls würde die Go-Toolchain bei jedem Build automatisch eine passende Version nachladen (oder ohne Netzwerk fehlschlagen).

---

### Phase 8: Test-Plan (Go-Tests)

Erweiterung der Test-Suite in `internal/api/`:
1. `TestDashboardServed`: Verifiziert, dass `GET /` den HTTP-Status 200 liefert, `text/html` zurückgibt und `/local-console.js` referenziert.
2. `TestDashboardJSAPIReferences`: Verifiziert, dass `GET /local-console.js` Status 200 und `application/javascript` liefert und die Endpunkte (`/v3/documents/documents`, `/v4/search` etc.) enthält. Auf exakte Pfade prüfen, da `/v3/documents` auch ein Teilstring von `/v3/documents/documents` ist.
3. `TestLocalConsoleBundleSize`: Verifiziert, dass das Bundle unter dem harten Limit von 50 KB bleibt (Zielwert aus Phase 2: < 40 KB) **und** die erwartete Logik enthält. Eine reine "nicht leer"-Prüfung genügt nicht: Ein auf den IIFE-Wrapper wegoptimiertes Bundle ist noch ~100–500 Bytes groß und würde ein Leer-/Größen-Gate scheinbar bestehen (siehe Tree-Shaking-Hinweis in Phase 2). Daher zusätzlich prüfen, dass mindestens einer der erwarteten API-Pfade (`/v3/documents/documents`, `/v4/search`) im Bundle vorkommt, oder eine Untergrenze deutlich oberhalb des Wrappers setzen.
4. `TestContainerTagsList`: Verifiziert, dass `GET /v3/container-tags/list` korrekte Zähler und JSON-Format liefert.
5. `TestDocumentsPagination`: Verifiziert, dass `POST /v3/documents/documents` Paginierung korrekt implementiert.
6. `TestConfigEndpoint`: Verifiziert, dass `POST /v3/config` `{"success": true}` mit gültigem Port liefert.
7. `TestGetOnApiRoutesStill405`: Verifiziert, dass `GET /v3/documents` weiterhin `405` (nicht `404`/`200`) liefert.

### Design-Verifikation (Vorher/Nachher-Screenshots)

Ziel: die Referenztreue belegen — vorher die Baseline der Referenz, nachher dieselben drei Ansichten aus dem neuen Daemon.

**Artefakte**
- `docs/dashboard-design/before/` — Referenz (Baseline, bereits aufgenommen): `overview-desktop.png` (1440×2600), `memory-desktop.png` (1440×1600), `overview-mobile.png` (390×1500).
- `docs/dashboard-design/after/` — dieselben drei Ansichten aus dem neuen Dashboard.

**Werkzeuge (im Repo)**
- `docs/dashboard-design/reference_server.py` — serviert `docs/supermemory/index.html` unter `/`, `local-console.js` unter `/local-console.js` und beantwortet die beiden vom Referenz-Console aufgerufenen Routen (`POST /v3/documents/documents`, `GET /v3/container-tags/list`) mit leeren Antworten, damit die Memory-Karte ihren Leerzustand rendert statt zu fehlern.
- `docs/dashboard-design/capture.sh` — Chrome-Headless-Aufnahme bei festen Viewports (`--headless=new --window-size=… --virtual-time-budget=15000`); `CHROME`-Pfad per Umgebungsvariable überschreibbar.

**Ablauf**
```bash
# 1) Baseline (neu) erzeugen
bash docs/dashboard-design/capture.sh before

# 2) Neues Dashboard bauen und starten
cd internal/api/assets && ./build.sh && cd ../../..
go build -o supermemory-native ./cmd/supermemory-native
./supermemory-native &          # PORT=6767; optional ein paar Memories anlegen

# 3) Nachher-Aufnahmen
docs/dashboard-design/capture.sh after           # → docs/dashboard-design/after/*.png
```

**Abhakliste (Vorher vs. Nachher)**
- Hero: Fläche `#6d9ee8` + Overlay-Verlauf, Padding und die 96px-Überlappung der ersten Karte.
- Header: Logo-Box 27px, Brand-Typo, `.status-pill`-Geometrie/Farbe inkl. Puls.
- Tabs: Chip-Geometrie und aktiver Zustand inkl. Diamant-Notches.
- Karten: Radius 10px, Schatten-Tripel (`--shadow-card/-deep/-window`), 1px-Haarlinien.
- Typo: mono 10–11px/uppercase/`.12em`-Labels, 44px-tabular-Statistiken, `h1`-Clamp.
- Raster: Umbruch bei 760px und 920px, `.wrap`-Breite 1240px.
- Netzwerk: nur `/`, `/local-console.js` und die API-Aufrufe — **keine externen Hosts**.
- Optionaler Pixel-Diff: `magick compare -metric AE before/x.png after/x.png diff.png`. Wegen abweichendem Inhalt und System-Fonts nur Zusatzsignal — maßgeblich ist die Sichtprüfung.

**Dokumentation:** Ergebnis (Datum, Commit, Rest-Abweichungen) im Commit-/PR-Text festhalten; die PNGs sind die Referenz für spätere Änderungen.

---

### Phase 9: Nächste Schritte

1. **Datenzugriff erweitern:** `internal/db/sqlite.go` um paginierte, tag-übergreifende Listen-/Zähl-Abfragen und `GROUP BY container_tag` ergänzen und in `internal/memory/engine.go` kapseln.
2. **Asset-Verzeichnis aufsetzen:** `internal/api/assets/src/` mit `index.html` und `local-console.js` anlegen.
3. **Build-Skripte erstellen:** `build.sh`, `build.ps1` und `package.json` in `internal/api/assets/` einrichten.
4. **Assets kompilieren:** `bun run build` ausführen und `assets.go` mit `//go:embed` erstellen; das Bundle gegen das Gate aus Phase 8, Test 3 prüfen (Größe **und** enthaltener API-Pfad, nicht nur "nicht leer").
5. **Backend anbinden:** `handler.go` um methodenabhängiges GET-Routing, die neuen Endpunkte (`/`, `/local-console.js`, `/v3/container-tags/list`, `/v3/documents/documents`, `/v3/config`) und den Port erweitern (siehe Phase 6.3) und das Dashboard im Browser testen.
6. **Design verifizieren:** Dashboard bauen/starten und `docs/dashboard-design/capture.sh after` ausführen; `after/*.png` gegen `before/*.png` und die Abhakliste prüfen, Abweichungen in `src/index.html` beheben und erneut aufnehmen.
7. **Go-Tests schreiben & ausführen:** Unit- und Integrationstests hinzufügen und verifizieren (`go test ./...`, `go vet ./...`).
8. **CI prüfen:** `go-version` in `.github/workflows/release.yml` ist bereits `1.25` (passend zu `go.mod` und `Dockerfile`) — keine Änderung mehr erforderlich.
9. **Git Commit:** Änderungen inklusive der Screenshot-Artefakte (`docs/dashboard-design/`) committen.

> **Reihenfolge:** Die Schritte 2–4 laufen vor Schritt 5, weil `handler.go` mit dem neuen Routing `internal/api/assets` importiert und der Go-Build sonst bis nach dem Asset-Build nicht kompiliert.
