package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/api/assets"
	"github.com/supermemory-native/supermemory-native/internal/memory"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

// maxBodyBytes caps request bodies: legitimate JSON payloads are tiny,
// anything larger is abuse or an attack.
const maxBodyBytes = 1 << 20 // 1 MiB

type Handler struct {
	Engine    *memory.Engine
	Processor *memory.Processor
	port      int
	apiKey    string
}

// NewHandler builds the API handler. The port is parsed once here instead of
// on every config request; the API key is read from SUPERMEMORY_API_KEY and
// enforced for non-loopback clients (see authorized).
func NewHandler(eng *memory.Engine, port string) *Handler {
	p, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || p <= 0 || p > 65535 {
		p = 6767
	}
	return &Handler{
		Engine:    eng,
		Processor: memory.NewProcessor(eng.DB, eng.Provider),
		port:      p,
		apiKey:    os.Getenv("SUPERMEMORY_API_KEY"),
	}
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

// Typed response envelopes. Field names and shapes must keep matching what
// the embedded dashboard and external clients parse.
type successResponse struct {
	Success bool `json:"success"`
}

type successIDResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
}

type configResponse struct {
	Success bool `json:"success"`
	Port    int  `json:"port"`
}

type syncResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type memoryEntry struct {
	ID        string    `json:"id"`
	Memory    string    `json:"memory"`
	CreatedAt time.Time `json:"createdAt"`
	IsStatic  bool      `json:"isStatic"`
}

type documentRow struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Summary       string        `json:"summary"`
	Status        string        `json:"status"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	ContainerTag  string        `json:"containerTag"`
	Memories      []string      `json:"memories"`
	MemoryEntries []memoryEntry `json:"memoryEntries"`
}

type pagination struct {
	TotalItems  int `json:"totalItems"`
	TotalPages  int `json:"totalPages"`
	CurrentPage int `json:"currentPage"`
}

type listDocumentsResponse struct {
	Documents  []documentRow `json:"documents"`
	Pagination pagination    `json:"pagination"`
}

type searchResultRow struct {
	ID         string    `json:"id"`
	Memory     string    `json:"memory"`
	Similarity float32   `json:"similarity"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type searchResponse struct {
	Results []searchResultRow `json:"results"`
	Total   int               `json:"total"`
}

type profileBody struct {
	Static  []string `json:"static"`
	Dynamic []string `json:"dynamic"`
}

type profileResponse struct {
	Profile       profileBody    `json:"profile"`
	SearchResults searchResponse `json:"searchResults"`
}

// writeErr logs the underlying error server-side and sends a generic JSON
// error to the client. Internal error strings never leak into responses.
func writeErr(w http.ResponseWriter, status int, publicMsg string, err error) {
	if err != nil {
		log.Printf("api: %s: %v", publicMsg, err)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": publicMsg})
}

// prepareBody applies the shared request-body guards: it rejects non-JSON
// content types and caps the body size. Browsers cannot send
// Content-Type: application/json cross-origin without a preflight, and
// preflights fail now that no CORS headers are emitted — so this blocks
// cross-site request forgery.
func prepareBody(w http.ResponseWriter, r *http.Request) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "content-type must be application/json", nil)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	return true
}

func bodyDecodeErr(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeErr(w, http.StatusRequestEntityTooLarge, "request body too large", nil)
		return
	}
	writeErr(w, http.StatusBadRequest, "failed to decode body", err)
}

// readJSON decodes the request body into dst, enforcing prepareBody's guards.
// On failure the error response is written and false is returned.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !prepareBody(w, r) {
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		bodyDecodeErr(w, err)
		return false
	}
	return true
}

// readJSONOptional is readJSON for endpoints whose payload is irrelevant
// (config, sync): an empty body is accepted, but size and content-type
// guards still apply.
func readJSONOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !prepareBody(w, r) {
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		bodyDecodeErr(w, err)
		return false
	}
	return true
}

// authorized reports whether the request may proceed. When an API key is
// configured, non-loopback clients must present it (Authorization: Bearer or
// x-supermemory-api-key). Loopback clients are exempt so the bundled local
// console keeps working; the loopback threat (websites in the user's browser)
// is already neutralized by the missing CORS headers plus content-type
// enforcement.
func (h *Handler) authorized(r *http.Request) bool {
	if h.apiKey == "" {
		return true
	}
	if isLoopbackAddr(r.RemoteAddr) {
		return true
	}

	provided := r.Header.Get("x-supermemory-api-key")
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		provided = strings.TrimPrefix(auth, "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(h.apiKey)) == 1
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// assetETags caches SHA-256 ETags for the embedded dashboard assets so
// repeat loads can be answered with 304 Not Modified.
var assetETags = map[string]string{}

func init() {
	for _, name := range []string{"index.html", "local-console.js"} {
		data, err := assets.Content.ReadFile(name)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		assetETags[name] = `"` + hex.EncodeToString(sum[:]) + `"`
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// No CORS headers on purpose: the console is same-origin, and cross-origin
	// preflights must fail so foreign pages cannot call this API.
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// The dashboard shell is public: a remote browser cannot know the API key
	// before it has the page that asks for it, and these two embedded assets
	// contain no memory data. Every API route below stays behind authorized().
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		h.serveAsset(w, r, "index.html", "text/html; charset=utf-8")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/local-console.js" {
		h.serveAsset(w, r, "local-console.js", "application/javascript; charset=utf-8")
		return
	}

	// v5 namespace-scoped routes run before the legacy auth path so their
	// error envelope matches the hosted API.
	if p := r.URL.Path; p == "/namespaces" || p == "/ns" || strings.HasPrefix(p, "/ns/") {
		if !h.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="supermemory"`)
			writeErr(w, http.StatusUnauthorized, "Unauthorized", nil)
			return
		}
		h.serveV5(w, r)
		return
	}

	if !h.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="supermemory"`)
		writeErr(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	// Method-aware routing: GET is only allowed on the container-tag list.
	// Every other route keeps its 405 for GET requests.
	switch {
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
// overriding the JSON default set at the top of ServeHTTP. Assets carry an
// ETag plus Cache-Control so repeat loads revalidate cheaply.
func (h *Handler) serveAsset(w http.ResponseWriter, r *http.Request, name, contentType string) {
	data, err := assets.Content.ReadFile(name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "asset unavailable", err)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if etag := assetETags[name]; etag != "" {
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	var req struct{}
	if !readJSONOptional(w, r, &req) {
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(configResponse{Success: true, Port: h.port})
}

func (h *Handler) handleContainerTagsList(w http.ResponseWriter, r *http.Request) {
	tags, err := h.Engine.ListContainerTags()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list container tags", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tags)
}

func (h *Handler) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	var req listDocumentsRequest
	if !readJSON(w, r, &req) {
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
	// One page hydrates up to `limit` vault files; keep the bound far below
	// the engine's 1000-row ceiling so a single request cannot trigger a
	// thousand file reads.
	if limit > 250 {
		limit = 250
	}

	memories, total, err := h.Engine.ListMemoriesPage(page, limit, req.Sort, req.Order)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list documents", err)
		return
	}

	documents := make([]documentRow, 0, len(memories))
	for _, m := range memories {
		documents = append(documents, documentRow{
			ID:           m.ID,
			Title:        documentTitle(m.Content),
			Summary:      m.Content,
			Status:       "done",
			CreatedAt:    m.CreatedAt,
			UpdatedAt:    m.CreatedAt,
			ContainerTag: m.ContainerTag,
			Memories:     []string{},
			MemoryEntries: []memoryEntry{
				{
					ID:        m.ID,
					Memory:    m.Content,
					CreatedAt: m.CreatedAt,
					IsStatic:  true,
				},
			},
		})
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(listDocumentsResponse{
		Documents: documents,
		Pagination: pagination{
			TotalItems:  total,
			TotalPages:  totalPages,
			CurrentPage: page,
		},
	})
}

// documentTitle derives a short single-line title from a memory body.
// OKF-aware extraction order:
//  1. the `title:` field inside a leading `---` frontmatter block
//     (surrounding quotes stripped);
//  2. otherwise the first non-empty line outside the frontmatter that is not
//     a `---` delimiter nor a frontmatter key line, with a leading markdown
//     heading `#` marker stripped;
//  3. otherwise the literal first line (unchanged plain-text behaviour),
//     falling back to "(untitled)".
func documentTitle(content string) string {
	if title, ok := okfTitle(content); ok {
		return clipTitle(title)
	}
	// Final fallback: original first-line behaviour for plain text.
	line := content
	if idx := strings.IndexAny(line, "\r\n"); idx >= 0 {
		line = line[:idx]
	}
	return clipTitle(strings.TrimSpace(line))
}

// okfTitle extracts the OKF-derived title candidate from a document body.
// It returns ok=false when content is empty or contains only frontmatter
// delimiters and key lines, in which case documentTitle falls back to the
// first-line rule.
func okfTitle(content string) (string, bool) {
	inFrontmatter := false
	sawFrontmatter := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		if line == "---" {
			if !sawFrontmatter {
				// Opening delimiter of a leading frontmatter block.
				sawFrontmatter = true
				inFrontmatter = true
			} else if inFrontmatter {
				// Closing delimiter; the body follows.
				inFrontmatter = false
			}
			continue
		}
		if inFrontmatter {
			key, value, found := strings.Cut(line, ":")
			if !found || strings.TrimSpace(key) != "title" {
				continue // other frontmatter key line
			}
			if title := unquoteTitleValue(value); title != "" {
				return title, true
			}
			continue // empty `title:` — try the body instead
		}
		// First meaningful body line; strip a markdown heading marker.
		line = strings.TrimLeft(line, "#")
		return strings.TrimSpace(line), true
	}
	return "", false
}

// unquoteTitleValue trims a frontmatter value and strips one pair of
// surrounding double or single quotes.
func unquoteTitleValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if q := value[0]; (q == '"' || q == '\'') && value[len(value)-1] == q {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// clipTitle bounds a title to 80 runes and substitutes "(untitled)" for
// empty results.
func clipTitle(line string) string {
	if line == "" {
		return "(untitled)"
	}
	const maxRunes = 80
	runes := []rune(line)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return line
}

func (h *Handler) handleAddDocument(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	if !readJSON(w, r, &req) {
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		writeErr(w, http.StatusBadRequest, "content cannot be empty", nil)
		return
	}
	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	id, err := h.Engine.AddMemory(req.Content, req.ContainerTag)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to add document", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(successIDResponse{Success: true, ID: id})
}

func (h *Handler) handleProfileQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if !readJSON(w, r, &req) {
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	// Semantic search with a low threshold to find relevant facts
	results, err := h.Engine.QueryMemories(req.queryText(), req.ContainerTag, 0.3, 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to run profile query", err)
		return
	}

	dynamicFacts := make([]string, len(results))
	searchResults := make([]searchResultRow, len(results))
	for i, res := range results {
		dynamicFacts[i] = res.Memory.Content
		searchResults[i] = searchResultRow{
			ID:         res.Memory.ID,
			Memory:     res.Memory.Content,
			Similarity: res.Similarity,
			UpdatedAt:  res.Memory.CreatedAt,
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(profileResponse{
		Profile: profileBody{
			Static:  []string{},
			Dynamic: dynamicFacts,
		},
		SearchResults: searchResponse{
			Results: searchResults,
			Total:   len(results),
		},
	})
}

func (h *Handler) handleSearchQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if !readJSON(w, r, &req) {
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	results, err := h.Engine.QueryMemories(req.queryText(), req.ContainerTag, 0.4, 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to run search", err)
		return
	}

	searchResults := make([]searchResultRow, len(results))
	for i, res := range results {
		searchResults[i] = searchResultRow{
			ID:         res.Memory.ID,
			Memory:     res.Memory.Content,
			Similarity: res.Similarity,
			UpdatedAt:  res.Memory.CreatedAt,
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(searchResponse{
		Results: searchResults,
		Total:   len(results),
	})
}

func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	var req struct{}
	if !readJSONOptional(w, r, &req) {
		return
	}

	if err := h.Engine.SyncVault(); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to synchronize vault", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(syncResponse{
		Success: true,
		Message: "Vault synchronization completed successfully",
	})
}

func (h *Handler) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	var req deleteRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	if req.ID == "" && req.Content == "" {
		writeErr(w, http.StatusBadRequest, "id or content is required", nil)
		return
	}

	if req.ID != "" {
		if !vault.ValidID(req.ID) {
			writeErr(w, http.StatusBadRequest, "invalid memory id", nil)
			return
		}
		if err := h.Engine.DeleteMemory(req.ID, req.ContainerTag); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to delete memory", err)
			return
		}
	} else {
		// Resolve by content: semantic search only proposes candidates, and
		// the candidate must match the requested content exactly (raw or as
		// OKF body) so a near-duplicate is never deleted.
		results, err := h.Engine.QueryMemories(req.Content, req.ContainerTag, 0.99, 10)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to resolve content for deletion", err)
			return
		}
		for _, res := range results {
			body := res.Memory.Content
			if doc, err := vault.ParseOKF(res.Memory.Content); err == nil {
				body = doc.Body
			}
			if res.Memory.Content != req.Content && body != req.Content {
				continue
			}
			if err := h.Engine.DeleteMemory(res.Memory.ID, req.ContainerTag); err != nil {
				writeErr(w, http.StatusInternalServerError, "failed to delete memory", err)
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(successResponse{Success: true})
}
