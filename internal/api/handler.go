package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/supermemory-native/supermemory-native/internal/api/assets"
	"github.com/supermemory-native/supermemory-native/internal/memory"
)

type Handler struct {
	Engine *memory.Engine
	Port   string
}

func NewHandler(eng *memory.Engine, port string) *Handler {
	return &Handler{Engine: eng, Port: port}
}

// knownRoutes lists every path this server exposes. It is used to tell a
// wrong-method request (405) apart from an unknown endpoint (404).
var knownRoutes = map[string]bool{
	"/":                       true,
	"/local-console.js":       true,
	"/v3/config":              true,
	"/v3/documents":           true,
	"/v3/documents/documents": true,
	"/v3/container-tags/list": true,
	"/v3/search":              true,
	"/v3/sync":                true,
	"/v4/search":              true,
	"/v4/profile":             true,
	"/v4/memories":            true,
}

type addRequest struct {
	Content      string `json:"content"`
	ContainerTag string `json:"containerTag"`
}

type queryRequest struct {
	Query        string `json:"q"`
	QueryAlias   string `json:"query"`
	ContainerTag string `json:"containerTag"`
}

// queryText returns the query, accepting both "q" and the "query" alias.
func (q queryRequest) queryText() string {
	if q.Query != "" {
		return q.Query
	}
	return q.QueryAlias
}

type deleteRequest struct {
	ContainerTag string `json:"containerTag"`
	ID           string `json:"id"`
	Content      string `json:"content"`
	Reason       string `json:"reason"`
}

type listDocumentsRequest struct {
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
	Sort  string `json:"sort"`
	Order string `json:"order"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Preflight CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, x-supermemory-api-key, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Method-aware routing: GET is only allowed on the static assets and the
	// container-tag list. Every other route keeps its 405 for GET requests.
	switch {
	case r.Method == "GET" && r.URL.Path == "/":
		h.serveAsset(w, "index.html", "text/html; charset=utf-8")
	case r.Method == "GET" && r.URL.Path == "/local-console.js":
		h.serveAsset(w, "local-console.js", "application/javascript; charset=utf-8")
	case r.Method == "GET" && r.URL.Path == "/v3/container-tags/list":
		h.handleContainerTagsList(w, r)
	case r.Method == "POST" && r.URL.Path == "/v3/config":
		h.handleConfig(w, r)
	case r.Method == "POST" && r.URL.Path == "/v3/documents":
		h.handleAddDocument(w, r)
	case r.Method == "POST" && r.URL.Path == "/v3/documents/documents":
		h.handleListDocuments(w, r)
	case r.Method == "POST" && (r.URL.Path == "/v3/search" || r.URL.Path == "/v4/search"):
		h.handleSearchQuery(w, r)
	case r.Method == "POST" && r.URL.Path == "/v4/profile":
		h.handleProfileQuery(w, r)
	case r.Method == "POST" && r.URL.Path == "/v3/sync":
		h.handleSync(w, r)
	case r.Method == "DELETE" && r.URL.Path == "/v4/memories":
		h.handleDeleteMemory(w, r)
	default:
		if knownRoutes[r.URL.Path] {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error": "method not allowed"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "endpoint not found"}`))
	}
}

// serveAsset writes an embedded dashboard file with an explicit content type,
// overriding the JSON default set at the top of ServeHTTP.
func (h *Handler) serveAsset(w http.ResponseWriter, name, contentType string) {
	data, err := assets.Content.ReadFile(name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "asset unavailable"}`))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(strings.TrimSpace(h.Port))
	if err != nil || port <= 0 || port > 65535 {
		port = 6767
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"port":    port,
	})
}

func (h *Handler) handleContainerTagsList(w http.ResponseWriter, r *http.Request) {
	tags, err := h.Engine.ListContainerTags()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tags)
}

func (h *Handler) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	var req listDocumentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 100
	}

	memories, total, err := h.Engine.ListMemoriesPage(page, limit, req.Sort, req.Order)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	documents := make([]map[string]interface{}, 0, len(memories))
	for _, m := range memories {
		memoryEntries := []map[string]interface{}{
			{
				"id":        m.ID,
				"memory":    m.Content,
				"createdAt": m.CreatedAt,
				"isStatic":  true,
			},
		}
		documents = append(documents, map[string]interface{}{
			"id":            m.ID,
			"title":         documentTitle(m.Content),
			"summary":       m.Content,
			"status":        "done",
			"createdAt":     m.CreatedAt,
			"updatedAt":     m.CreatedAt,
			"containerTag":  m.ContainerTag,
			"memories":      []interface{}{},
			"memoryEntries": memoryEntries,
		})
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"documents": documents,
		"pagination": map[string]interface{}{
			"totalItems":  total,
			"totalPages":  totalPages,
			"currentPage": page,
		},
	})
}

// documentTitle derives a short single-line title from a memory body.
func documentTitle(content string) string {
	line := content
	if idx := strings.IndexAny(line, "\r\n"); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	const maxRunes = 80
	runes := []rune(line)
	if len(runes) > maxRunes {
		line = string(runes[:maxRunes]) + "…"
	}
	if line == "" {
		return "(untitled)"
	}
	return line
}

func (h *Handler) handleAddDocument(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	id, err := h.Engine.AddMemory(req.Content, req.ContainerTag)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      id,
	})
}

func (h *Handler) handleProfileQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	// Semantic search with a low threshold to find relevant facts
	results, err := h.Engine.QueryMemories(req.queryText(), req.ContainerTag, 0.3, 10)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	// Format matching official Supermemory profile response
	dynamicFacts := make([]string, len(results))
	searchResults := make([]map[string]interface{}, len(results))

	for i, res := range results {
		dynamicFacts[i] = res.Memory.Content
		searchResults[i] = map[string]interface{}{
			"id":         res.Memory.ID,
			"memory":     res.Memory.Content,
			"similarity": res.Similarity,
			"updatedAt":  res.Memory.CreatedAt,
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"profile": map[string]interface{}{
			"static":  []string{},
			"dynamic": dynamicFacts,
		},
		"searchResults": map[string]interface{}{
			"results": searchResults,
			"total":   len(results),
		},
	})
}

func (h *Handler) handleSearchQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	results, err := h.Engine.QueryMemories(req.queryText(), req.ContainerTag, 0.4, 10)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	searchResults := make([]map[string]interface{}, len(results))
	for i, res := range results {
		searchResults[i] = map[string]interface{}{
			"id":         res.Memory.ID,
			"memory":     res.Memory.Content,
			"similarity": res.Similarity,
			"updatedAt":  res.Memory.CreatedAt,
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"results": searchResults,
		"total":   len(results),
	})
}

func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	if err := h.Engine.SyncVault(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Vault synchronization completed successfully",
	})
}

func (h *Handler) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	var req deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}
	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	// If no ID given, resolve by content within the container tag
	if req.ID == "" && req.Content != "" {
		results, err := h.Engine.QueryMemories(req.Content, req.ContainerTag, 0.99, 10)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
			return
		}
		for _, res := range results {
			if err := h.Engine.DeleteMemory(res.Memory.ID, req.ContainerTag); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
				return
			}
		}
	} else if req.ID != "" {
		if err := h.Engine.DeleteMemory(req.ID, req.ContainerTag); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}
