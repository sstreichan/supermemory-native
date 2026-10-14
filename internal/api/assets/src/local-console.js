/* local-console.js — self-contained dashboard logic for the local daemon.
 *
 * Vanilla ES6+. All DOM writes go through textContent / createElement so no
 * user-supplied content is ever parsed as HTML. Every request targets the local
 * daemon only; there are no external origins.
 */

var state = {
  config: { port: 6767 },
  documents: [],
  tags: [],
  searchResults: [],
  activeFilter: "",
  pagination: { totalItems: 0, totalPages: 1, currentPage: 1 },
  page: 1,
  limit: 20,
  memoryLoaded: false,
  busy: false,
};

var JSON_HEADERS = { "Content-Type": "application/json" };

/* ── helpers ─────────────────────────────────────────────────────────── */

function byId(id) {
  return document.getElementById(id);
}

/* h(tag, className, text) — creates an element with safe text content. */
function h(tag, className, text) {
  var el = document.createElement(tag);
  if (className) el.className = className;
  if (text !== undefined && text !== null) el.textContent = String(text);
  return el;
}

var SVG_NS = "http://www.w3.org/2000/svg";

function svgEl(tag, attrs) {
  var el = document.createElementNS(SVG_NS, tag);
  for (var key in attrs) {
    if (Object.prototype.hasOwnProperty.call(attrs, key)) {
      el.setAttribute(key, String(attrs[key]));
    }
  }
  return el;
}

function clear(el) {
  while (el && el.firstChild) el.removeChild(el.firstChild);
}

function truncate(text, length) {
  var value = String(text === undefined || text === null ? "" : text);
  if (value.length <= length) return value;
  return value.slice(0, length - 1).trimEnd() + "…";
}

function timeAgo(input) {
  if (!input) return "—";
  var then = new Date(input).getTime();
  if (isNaN(then)) return "—";
  var diff = Math.max(0, Date.now() - then);
  var seconds = Math.floor(diff / 1000);
  if (seconds < 60) return seconds + "s ago";
  var minutes = Math.floor(seconds / 60);
  if (minutes < 60) return minutes + "m ago";
  var hours = Math.floor(minutes / 60);
  if (hours < 24) return hours + "h ago";
  var days = Math.floor(hours / 24);
  if (days < 30) return days + "d ago";
  var months = Math.floor(days / 30);
  if (months < 12) return months + "mo ago";
  return Math.floor(months / 12) + "y ago";
}

function fetchJson(url, options) {
  return fetch(url, options).then(function (res) {
    return res.text().then(function (text) {
      var data = null;
      if (text) {
        try {
          data = JSON.parse(text);
        } catch (err) {
          data = null;
        }
      }
      if (!res.ok) {
        var message = (data && (data.error || data.message)) || "HTTP " + res.status;
        throw new Error(message);
      }
      return data;
    });
  });
}

function showError(message) {
  var banner = byId("error-banner");
  if (!banner) return;
  banner.textContent = String(message);
  banner.hidden = false;
}

function hideError() {
  var banner = byId("error-banner");
  if (banner) banner.hidden = true;
}

/* ── API calls ───────────────────────────────────────────────────────── */

function loadConfig() {
  return fetchJson("/v3/config", { method: "POST", headers: JSON_HEADERS, body: "{}" }).then(function (data) {
    if (data && typeof data.port === "number") state.config.port = data.port;
  });
}

function loadContainerTags() {
  return fetchJson("/v3/container-tags/list", { method: "GET" }).then(function (data) {
    state.tags = Array.isArray(data) ? data : [];
  });
}

function loadDocuments(page) {
  state.page = page && page > 0 ? page : 1;
  var body = JSON.stringify({
    page: state.page,
    limit: state.limit,
    sort: "createdAt",
    order: "desc",
  });
  return fetchJson("/v3/documents/documents", { method: "POST", headers: JSON_HEADERS, body: body }).then(function (data) {
    data = data || {};
    state.documents = Array.isArray(data.documents) ? data.documents : [];
    state.pagination = data.pagination || { totalItems: 0, totalPages: 1, currentPage: state.page };
  });
}

function performSearch(query, containerTag) {
  if (!query) return Promise.resolve({ results: [], total: 0 });
  var body = JSON.stringify({ q: query, query: query, containerTag: containerTag || "default" });
  return fetchJson("/v4/search", { method: "POST", headers: JSON_HEADERS, body: body }).then(
    function (data) {
      data = data || {};
      return { results: Array.isArray(data.results) ? data.results : [], total: data.total || 0 };
    },
    function () {
      return fetchJson("/v4/profile", { method: "POST", headers: JSON_HEADERS, body: body }).then(function (data) {
        var search = (data && data.searchResults) || {};
        return {
          results: Array.isArray(search.results) ? search.results : [],
          total: search.total || 0,
        };
      });
    }
  );
}

function addMemory(content, containerTag) {
  var body = JSON.stringify({ content: content, containerTag: containerTag || "default" });
  return fetchJson("/v3/documents", { method: "POST", headers: JSON_HEADERS, body: body });
}

function deleteMemory(id, containerTag) {
  var body = JSON.stringify({ id: id, containerTag: containerTag || "default" });
  return fetchJson("/v4/memories", { method: "DELETE", headers: JSON_HEADERS, body: body });
}

function triggerSync() {
  return fetchJson("/v3/sync", { method: "POST", headers: JSON_HEADERS, body: "{}" });
}

/* ── derived values ──────────────────────────────────────────────────── */

function totalMemories() {
  var sum = 0;
  for (var i = 0; i < state.tags.length; i++) {
    sum += Number(state.tags[i].memoryCount) || 0;
  }
  if (sum > 0) return sum;
  return Number(state.pagination.totalItems) || 0;
}

function docTitle(doc) {
  var source = doc && (doc.title || doc.summary || "");
  var firstLine = String(source).split("\n")[0];
  return truncate(firstLine, 72) || "(untitled)";
}

function docEntries(doc) {
  return doc && Array.isArray(doc.memoryEntries) ? doc.memoryEntries : [];
}

/* ── rendering ───────────────────────────────────────────────────────── */

function renderStatus() {
  var el = byId("status-text");
  if (!el) return;
  el.textContent = "running · " + state.config.port;
}

function renderSummary() {
  var endpoint = byId("summary-endpoint");
  var documents = byId("summary-documents");
  var memories = byId("summary-memories");
  var tags = byId("summary-tags");
  if (endpoint) endpoint.textContent = "http://localhost:" + state.config.port;
  if (documents) documents.textContent = String(state.pagination.totalItems || 0);
  if (memories) memories.textContent = String(totalMemories());
  if (tags) tags.textContent = String(state.tags.length);
}

function renderStats() {
  var docs = byId("mem-stat-docs");
  var mems = byId("mem-stat-mems");
  var tags = byId("mem-stat-tags");
  var sub = byId("graph-sub");
  var total = Number(state.pagination.totalItems) || 0;
  var memories = totalMemories();
  if (docs) docs.textContent = String(total);
  if (mems) mems.textContent = String(memories);
  if (tags) tags.textContent = String(state.tags.length);
  if (sub) sub.textContent = total + " documents · " + memories + " memories · newest first";
}

function renderTags() {
  var list = byId("tag-list");
  if (!list) return;
  clear(list);
  if (state.tags.length === 0) {
    list.appendChild(h("p", "empty-note", "No container tags yet."));
    return;
  }
  for (var i = 0; i < state.tags.length; i++) {
    var tag = state.tags[i] || {};
    var name = tag.containerTag || tag.name || "default";
    var item = h("div", "tag-item");
    var chip = h("button", "mem-tag-chip", (tag.emoji ? tag.emoji + " " : "") + name);
    chip.type = "button";
    chip.setAttribute("data-filter", name);
    if (state.activeFilter === name) chip.className = "mem-tag-chip active";
    chip.setAttribute("aria-pressed", state.activeFilter === name ? "true" : "false");
    item.appendChild(chip);
    item.appendChild(h("span", "mem-row-time", (Number(tag.memoryCount) || 0) + " memories"));
    list.appendChild(item);
  }
}

function renderDocuments() {
  var list = byId("doc-list");
  if (!list) return;
  clear(list);

  var docs = state.documents;
  if (state.activeFilter) {
    docs = docs.filter(function (doc) {
      return (doc.containerTag || "default") === state.activeFilter;
    });
  }

  if (docs.length === 0) {
    var note = state.activeFilter
      ? "No documents tagged “" + state.activeFilter + "” on this page."
      : "Nothing stored yet — capture a memory above.";
    list.appendChild(h("p", "empty-note", note));
    renderPagination();
    return;
  }

  for (var i = 0; i < docs.length; i++) {
    var doc = docs[i] || {};
    var row = h("div", "doc-row");
    row.appendChild(h("span", "mem-dot static"));
    var main = h("div", "doc-main");
    main.appendChild(h("p", "doc-title", docTitle(doc)));
    main.appendChild(h("p", "doc-body", truncate(doc.summary || "", 220)));
    var meta = h("div", "doc-meta");
    meta.appendChild(h("span", null, doc.containerTag || "default"));
    meta.appendChild(h("span", null, timeAgo(doc.createdAt)));
    meta.appendChild(h("span", null, (docEntries(doc).length || 0) + " memories"));
    main.appendChild(meta);
    row.appendChild(main);
    var forget = h("button", "link-btn", "forget");
    forget.type = "button";
    forget.setAttribute("data-forget", doc.id || "");
    forget.setAttribute("data-tag", doc.containerTag || "default");
    row.appendChild(forget);
    list.appendChild(row);
  }

  renderPagination();
}

function renderPagination() {
  var el = byId("doc-pagination");
  if (!el) return;
  clear(el);

  var pag = state.pagination || {};
  var page = Number(pag.currentPage) || 1;
  var totalPages = Math.max(1, Number(pag.totalPages) || 1);
  var totalItems = Number(pag.totalItems) || 0;

  var prev = h("button", "link-btn", "← prev");
  prev.type = "button";
  prev.disabled = page <= 1;
  prev.setAttribute("data-page", String(page - 1));
  el.appendChild(prev);

  el.appendChild(h("span", null, "page " + page + " / " + totalPages + " · " + totalItems + " documents"));

  var next = h("button", "link-btn", "next →");
  next.type = "button";
  next.disabled = page >= totalPages;
  next.setAttribute("data-page", String(page + 1));
  el.appendChild(next);
}

function renderSearchResults(results) {
  var list = byId("search-results");
  if (!list) return;
  clear(list);

  if (!state.searchResults.length) {
    list.appendChild(h("p", "empty-note", "No search yet."));
    return;
  }
  if (results.length === 0) {
    list.appendChild(h("p", "empty-note", "No matches for that query."));
    return;
  }

  for (var i = 0; i < results.length; i++) {
    var item = results[i] || {};
    var row = h("div", "doc-row");
    row.appendChild(h("span", "mem-dot"));
    var main = h("div", "doc-main");
    main.appendChild(h("p", "doc-body", item.memory || ""));
    var meta = h("div", "doc-meta");
    var similarity = typeof item.similarity === "number" ? item.similarity : 0;
    meta.appendChild(h("span", "score", "similarity " + similarity.toFixed(3)));
    meta.appendChild(h("span", null, timeAgo(item.updatedAt)));
    main.appendChild(meta);
    row.appendChild(main);
    list.appendChild(row);
  }
}

function renderGraph() {
  var canvas = byId("graph-canvas");
  if (!canvas) return;
  clear(canvas);

  var docs = state.documents || [];
  if (docs.length === 0) {
    var empty = h("div", "mem-graph-empty");
    empty.appendChild(h("div", "mem-graph-empty-title", "nothing here yet"));
    empty.appendChild(h("div", "mem-graph-empty-sub", "add your first memory and watch the graph grow"));
    canvas.appendChild(empty);
    return;
  }

  var W = 1000;
  var H = 460;
  var svg = svgEl("svg", { viewBox: "0 0 " + W + " " + H, preserveAspectRatio: "xMidYMid meet" });
  var cx = W / 2;
  var cy = H / 2;
  var nodes = docs.slice(0, 8);
  var n = nodes.length;
  var rx = 340;
  var ry = 140;

  for (var i = 0; i < n; i++) {
    var angle = -Math.PI / 2 + (i * 2 * Math.PI) / n;
    var dx = cx + rx * Math.cos(angle);
    var dy = cy + ry * Math.sin(angle);
    var mx = cx + (rx + 100) * Math.cos(angle);
    var my = cy + (ry + 64) * Math.sin(angle);

    svg.appendChild(
      svgEl("line", { x1: dx, y1: dy, x2: mx, y2: my, stroke: "#a3acc2", "stroke-width": 1 })
    );

    var doc = nodes[i] || {};
    svg.appendChild(svgEl("rect", { x: dx - 70, y: dy - 18, width: 140, height: 36, rx: 8, fill: "#e6efff", stroke: "#0562ef" }));
    var docLabel = svgEl("text", {
      x: dx, y: dy, "text-anchor": "middle", "dominant-baseline": "middle",
      "font-family": "monospace", "font-size": 10, fill: "#111113",
    });
    docLabel.textContent = truncate(docTitle(doc), 18);
    svg.appendChild(docLabel);

    var memory = docEntries(doc)[0] || {};
    svg.appendChild(svgEl("rect", { x: mx - 58, y: my - 15, width: 116, height: 30, rx: 6, fill: "#f1f1f3", stroke: "#d2d4dc" }));
    var memLabel = svgEl("text", {
      x: mx, y: my, "text-anchor": "middle", "dominant-baseline": "middle",
      "font-family": "monospace", "font-size": 9, fill: "#6f6f6f",
    });
    memLabel.textContent = truncate(String(memory.memory || "").replace(/\s+/g, " "), 16);
    svg.appendChild(memLabel);
  }

  canvas.appendChild(svg);
}

function renderMemoryPanels() {
  var docsPanel = byId("panel-docs");
  var memsPanel = byId("panel-mems");
  var tagsPanel = byId("panel-tags");
  var docs = state.documents || [];

  if (docsPanel) {
    clear(docsPanel);
    if (docs.length === 0) {
      docsPanel.appendChild(h("li", "mem-empty", "No documents yet — add one and it lands here."));
    } else {
      for (var i = 0; i < Math.min(docs.length, 6); i++) {
        var doc = docs[i] || {};
        var row = h("li", "mem-row");
        row.appendChild(h("span", "mem-dot static"));
        var body = h("div", "mem-row-body");
        body.appendChild(h("div", "mem-row-memory", docTitle(doc)));
        body.appendChild(h("div", "mem-row-time", timeAgo(doc.createdAt)));
        row.appendChild(body);
        docsPanel.appendChild(row);
      }
    }
  }

  if (memsPanel) {
    clear(memsPanel);
    var entries = [];
    for (var j = 0; j < docs.length && entries.length < 8; j++) {
      var docEntriesList = docEntries(docs[j]);
      for (var k = 0; k < docEntriesList.length && entries.length < 8; k++) {
        entries.push(docEntriesList[k]);
      }
    }
    if (entries.length === 0) {
      memsPanel.appendChild(h("li", "mem-empty", "No memories extracted yet — ingest something first."));
    } else {
      for (var e = 0; e < entries.length; e++) {
        var entry = entries[e] || {};
        var entryRow = h("li", "mem-row");
        entryRow.appendChild(h("span", "mem-dot"));
        var entryBody = h("div", "mem-row-body");
        entryBody.appendChild(h("div", "mem-row-memory", truncate(String(entry.memory || "").replace(/\s+/g, " "), 90)));
        entryBody.appendChild(h("div", "mem-row-time", timeAgo(entry.createdAt)));
        entryRow.appendChild(entryBody);
        memsPanel.appendChild(entryRow);
      }
    }
  }

  if (tagsPanel) {
    clear(tagsPanel);
    if (state.tags.length === 0) {
      tagsPanel.appendChild(h("li", "mem-empty", "No container tags yet — pass containerTag when adding."));
    } else {
      for (var t = 0; t < state.tags.length; t++) {
        var tag = state.tags[t] || {};
        var tagRow = h("li", "mem-row");
        tagRow.appendChild(h("span", "mem-dot"));
        var tagBody = h("div", "mem-row-body");
        tagBody.appendChild(h("span", "mem-tag-chip", (tag.emoji ? tag.emoji + " " : "") + (tag.containerTag || tag.name || "default")));
        tagBody.appendChild(h("div", "mem-row-time", (Number(tag.memoryCount) || 0) + " memories"));
        tagRow.appendChild(tagBody);
        tagsPanel.appendChild(tagRow);
      }
    }
  }
}

function renderMarquee() {
  var inner = byId("marquee-inner");
  if (!inner) return;
  clear(inner);

  var items = [
    "local unit",
    "port " + state.config.port,
    (Number(state.pagination.totalItems) || 0) + " documents stored",
    totalMemories() + " memories indexed",
    state.tags.length + " container tags",
    "zero cloud",
    "everything stays on this machine",
  ];

  for (var copy = 0; copy < 2; copy++) {
    var span = h("span");
    for (var i = 0; i < items.length; i++) {
      // One blue diamond before every item, matching the reference marquee.
      span.appendChild(h("b", null, "◆"));
      span.appendChild(document.createTextNode("\u00a0\u00a0" + items[i] + "\u00a0\u00a0\u00a0\u00a0"));
    }
    inner.appendChild(span);
  }
}

function renderAll() {
  renderStatus();
  renderSummary();
  renderStats();
  renderTags();
  renderDocuments();
  renderGraph();
  renderMemoryPanels();
  renderMarquee();
}

/* ── refresh ─────────────────────────────────────────────────────────── */

function refresh() {
  return loadConfig()
    .then(loadContainerTags)
    .then(function () {
      return loadDocuments(state.page);
    })
    .then(function () {
      hideError();
      state.memoryLoaded = true;
      renderAll();
    })
    .catch(function (err) {
      showError(err && err.message ? err.message : String(err));
    });
}

function setBusy(busy) {
  state.busy = busy;
  var ids = ["search-btn", "add-btn"];
  for (var i = 0; i < ids.length; i++) {
    var btn = byId(ids[i]);
    if (btn) btn.disabled = busy;
  }
}

/* ── tabs ────────────────────────────────────────────────────────────── */

function activate(name, updateHash) {
  var buttons = document.querySelectorAll(".tab-btn");
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].classList.toggle("active", buttons[i].getAttribute("data-tab") === name);
  }
  var panels = document.querySelectorAll(".tab-panel");
  for (var j = 0; j < panels.length; j++) {
    panels[j].hidden = panels[j].id !== "tab-" + name;
  }
  var copies = document.querySelectorAll(".hero-copy");
  for (var k = 0; k < copies.length; k++) {
    copies[k].hidden = copies[k].id !== "hero-" + name;
  }
  if (name === "memory" && !state.memoryLoaded) {
    refresh();
  }
  if (updateHash) {
    history.replaceState(null, "", name === "memory" ? "#memory" : location.pathname);
  }
}

/* ── events ──────────────────────────────────────────────────────────── */

function init() {
  var buttons = document.querySelectorAll(".tab-btn");
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].addEventListener("click", function (event) {
      activate(event.currentTarget.getAttribute("data-tab"), true);
    });
  }

  window.addEventListener("hashchange", function () {
    activate(location.hash === "#memory" ? "memory" : "overview", false);
  });

  var band = document.querySelector(".resource-band");
  if (band) {
    band.addEventListener("click", function (event) {
      var target = event.target.closest("[data-action]");
      if (!target) return;
      var action = target.getAttribute("data-action");
      if (action === "focus-add") {
        var content = byId("add-content");
        if (content) {
          content.focus();
          content.scrollIntoView({ behavior: "smooth", block: "center" });
        }
      } else if (action === "sync") {
        setBusy(true);
        showError("");
        hideError();
        var status = byId("add-status");
        triggerSync()
          .then(function (data) {
            if (status) status.textContent = (data && data.message) || "Vault synced.";
            return refresh();
          })
          .catch(function (err) {
            showError(err && err.message ? err.message : String(err));
          })
          .then(function () {
            setBusy(false);
          });
      }
    });
  }

  var searchForm = byId("search-form");
  if (searchForm) {
    searchForm.addEventListener("submit", function (event) {
      event.preventDefault();
      var input = byId("search-input");
      var query = input ? input.value.trim() : "";
      state.searchResults = query ? [1] : [];
      setBusy(true);
      performSearch(query, state.activeFilter || "default")
        .then(function (result) {
          state.searchResults = result.results.length ? result.results : query ? [1] : [];
          renderSearchResults(result.results);
        })
        .catch(function (err) {
          showError(err && err.message ? err.message : String(err));
        })
        .then(function () {
          setBusy(false);
        });
    });
  }

  var addForm = byId("add-form");
  if (addForm) {
    addForm.addEventListener("submit", function (event) {
      event.preventDefault();
      var content = byId("add-content");
      var tag = byId("add-tag");
      var status = byId("add-status");
      var value = content ? content.value.trim() : "";
      if (!value) return;
      setBusy(true);
      addMemory(value, tag && tag.value.trim() ? tag.value.trim() : "default")
        .then(function () {
          if (content) content.value = "";
          if (status) status.textContent = "Stored.";
          return refresh();
        })
        .catch(function (err) {
          showError(err && err.message ? err.message : String(err));
        })
        .then(function () {
          setBusy(false);
        });
    });
  }

  var refreshBtn = byId("mem-refresh");
  if (refreshBtn) {
    refreshBtn.addEventListener("click", function () {
      refresh();
    });
  }

  document.addEventListener("click", function (event) {
    var copyBtn = event.target.closest(".copy-btn");
    if (copyBtn) {
      var target = byId(copyBtn.getAttribute("data-copy"));
      var pre = target ? target.querySelector("pre") : null;
      var text = pre ? pre.textContent : "";
      if (navigator.clipboard && text) {
        navigator.clipboard.writeText(text.trim()).then(function () {
          var previous = copyBtn.textContent;
          copyBtn.textContent = "copied";
          copyBtn.classList.add("copied");
          setTimeout(function () {
            copyBtn.textContent = previous;
            copyBtn.classList.remove("copied");
          }, 1400);
        });
      }
      return;
    }

    var forget = event.target.closest("[data-forget]");
    if (forget) {
      var id = forget.getAttribute("data-forget");
      if (!id) return;
      forget.disabled = true;
      deleteMemory(id, forget.getAttribute("data-tag"))
        .then(function () {
          return refresh();
        })
        .catch(function (err) {
          showError(err && err.message ? err.message : String(err));
          forget.disabled = false;
        });
      return;
    }

    var pageBtn = event.target.closest("[data-page]");
    if (pageBtn && !pageBtn.disabled) {
      loadDocuments(Number(pageBtn.getAttribute("data-page")))
        .then(function () {
          renderDocuments();
          renderGraph();
          renderMemoryPanels();
          renderMarquee();
        })
        .catch(function (err) {
          showError(err && err.message ? err.message : String(err));
        });
      return;
    }

    var filter = event.target.closest("[data-filter]");
    if (filter) {
      var name = filter.getAttribute("data-filter");
      state.activeFilter = state.activeFilter === name ? "" : name;
      renderTags();
      renderDocuments();
    }
  });

  var list = byId("tag-list");
  if (list) list.setAttribute("aria-label", "Container tags");

  activate(location.hash === "#memory" ? "memory" : "overview", false);
  refresh();
}

/* Top-level side effect: Bun keeps the module when the entry point runs code. */
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
