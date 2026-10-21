package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

var namespacePattern = regexp.MustCompile(`^[a-zA-Z0-9_:-]{1,100}$`)

// serveV5 dispatches the v5 namespace-scoped surface. All paths here are
// authenticated by the caller (ServeHTTP).
func (h *Handler) serveV5(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		v5RouteNotFound(w, r)
		return
	}

	if parts[0] == "namespaces" {
		if len(parts) == 1 && r.Method == http.MethodGet {
			h.v5ListNamespaces(w, r)
			return
		}
		v5RouteNotFound(w, r)
		return
	}

	if parts[0] != "ns" {
		v5RouteNotFound(w, r)
		return
	}

	switch len(parts) {
	case 1:
		if r.Method == http.MethodGet {
			h.v5ListNamespaces(w, r)
			return
		}
	case 2:
		ns := parts[1]
		switch r.Method {
		case http.MethodGet:
			h.v5GetNamespace(w, r, ns)
			return
		case http.MethodPatch:
			h.v5PatchNamespace(w, r, ns)
			return
		case http.MethodDelete:
			h.v5DeleteNamespace(w, r, ns)
			return
		}
	case 3:
		ns, resource := parts[1], parts[2]
		switch resource {
		case "document":
			switch r.Method {
			case http.MethodPost:
				h.v5CreateDocument(w, r, ns)
				return
			case http.MethodDelete:
				h.v5DeleteDocuments(w, r, ns)
				return
			}
		case "memories":
			if r.Method == http.MethodDelete {
				h.v5ForgetMemories(w, r, ns)
				return
			}
		case "search":
			if r.Method == http.MethodPost {
				h.v5Search(w, r, ns)
				return
			}
		case "profile":
			if r.Method == http.MethodPost {
				h.v5Profile(w, r, ns)
				return
			}
		case "list":
			if r.Method == http.MethodPost {
				v5RouteNotFound(w, r)
				return
			}
		}
	case 4:
		ns, resource, sub := parts[1], parts[2], parts[3]
		switch resource {
		case "document":
			if sub == "file" {
				if r.Method == http.MethodPost {
					h.v5CreateDocumentFile(w, r, ns)
					return
				}
			} else if sub == "batch" {
				if r.Method == http.MethodPost {
					h.v5CreateDocumentBatch(w, r, ns)
					return
				}
			} else {
				switch r.Method {
				case http.MethodGet:
					h.v5GetDocument(w, r, ns, sub)
					return
				case http.MethodPatch:
					h.v5PatchDocument(w, r, ns, sub)
					return
				}
			}
		case "memories":
			if sub == "semantic" {
				if r.Method == http.MethodDelete {
					h.v5ForgetSemantic(w, r, ns)
					return
				}
			} else if r.Method == http.MethodGet {
				h.v5GetMemory(w, r, ns, sub)
				return
			}
		case "profile":
			if sub == "buckets" {
				switch r.Method {
				case http.MethodGet:
					h.v5GetBuckets(w, r, ns)
					return
				case http.MethodPut:
					h.v5PutBuckets(w, r, ns)
					return
				case http.MethodDelete:
					h.v5DeleteBuckets(w, r, ns)
					return
				}
			}
		case "list":
			if r.Method == http.MethodPost {
				h.v5ListContent(w, r, ns, sub)
				return
			}
		}
	case 5:
		if parts[2] == "document" && parts[3] == "file" {
			switch r.Method {
			case http.MethodPost:
				h.v5ReplaceDocumentFile(w, r, parts[1], parts[4])
				return
			case http.MethodPatch:
				h.v5PatchDocumentFile(w, r, parts[1], parts[4])
				return
			}
		}
		if parts[2] == "list" && parts[3] == "documents" && parts[4] == "memory-counts" {
			if r.Method == http.MethodPost {
				h.v5MemoryCounts(w, r, parts[1])
				return
			}
		}
	}
	v5RouteNotFound(w, r)
}

func v5RouteNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":         "Not found",
		"message":       "No route matches " + r.Method + " " + r.URL.Path,
		"documentation": "https://api.supermemory.ai/openapi.json",
	})
}

// v5ValidationError writes the v5 error envelope. path is omitted when nil.
func v5ValidationError(w http.ResponseWriter, status int, data any, code string, path []string, msg string, extra map[string]any) {
	item := map[string]any{"code": code, "message": msg}
	if path != nil {
		item["path"] = path
	}
	for k, v := range extra {
		item[k] = v
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":    data,
		"error":   []any{item},
		"success": false,
	})
}

func v5Write(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// v5Decode validates the content type, caps the body, enforces the key
// whitelist and decodes into dst. It returns the raw field map for presence
// checks. An empty body is only accepted when allowEmpty is true; in that
// case the content type is not checked either.
func v5Decode(w http.ResponseWriter, r *http.Request, dst any, allowEmpty bool, ctPath string, allowed ...string) (map[string]json.RawMessage, bool) {
	if allowEmpty && r.ContentLength == 0 {
		return map[string]json.RawMessage{}, true
	}
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{ctPath}, "Expected application/json content type", nil)
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{ctPath}, "Failed to read request body", nil)
		return nil, false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		if allowEmpty {
			return map[string]json.RawMessage{}, true
		}
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_json", []string{ctPath}, "unexpected end of JSON input", nil)
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{ctPath}, "Expected object", nil)
		return nil, false
	}
	if !v5CheckKeys(w, fields, allowed...) {
		return nil, false
	}
	if dst != nil {
		if err := json.Unmarshal(raw, dst); err != nil {
			v5ValidationError(w, http.StatusBadRequest, fields, "invalid_type", []string{ctPath}, "Expected a valid value", nil)
			return nil, false
		}
	}
	return fields, true
}

// v5DecodeItem applies the same key whitelist to a nested object.
func v5DecodeItem(w http.ResponseWriter, raw json.RawMessage, dst any, allowed ...string) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", nil, "Expected object", nil)
		return nil, false
	}
	if !v5CheckKeys(w, fields, allowed...) {
		return nil, false
	}
	if dst != nil {
		if err := json.Unmarshal(raw, dst); err != nil {
			v5ValidationError(w, http.StatusBadRequest, fields, "invalid_type", nil, "Expected a valid value", nil)
			return nil, false
		}
	}
	return fields, true
}

func v5CheckKeys(w http.ResponseWriter, fields map[string]json.RawMessage, allowed ...string) bool {
	set := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		set[k] = true
	}
	unknown := make([]string, 0)
	for k := range fields {
		if !set[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return true
	}
	sort.Strings(unknown)
	v5ValidationError(w, http.StatusBadRequest, fields, "unrecognized_keys", nil,
		"Unrecognized key(s) in object: "+strings.Join(unknown, ", "), map[string]any{"keys": unknown})
	return false
}

func v5RejectQueryKeys(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	set := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		set[k] = true
	}
	unknown := make([]string, 0)
	for k := range r.URL.Query() {
		if !set[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return true
	}
	sort.Strings(unknown)
	v5ValidationError(w, http.StatusBadRequest, nil, "unrecognized_keys", nil,
		"Unrecognized key(s) in object: "+strings.Join(unknown, ", "), map[string]any{"keys": unknown})
	return false
}

func v5PaginationParams(w http.ResponseWriter, r *http.Request) (page, limit int, ok bool) {
	page, limit = 1, 10
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"page"}, "Expected number", nil)
			return 0, 0, false
		}
		if n < 1 {
			v5ValidationError(w, http.StatusBadRequest, nil, "too_small", []string{"page"}, "Number must be greater than or equal to 1", map[string]any{"minimum": 1})
			return 0, 0, false
		}
		page = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"limit"}, "Expected number", nil)
			return 0, 0, false
		}
		if n < 1 {
			v5ValidationError(w, http.StatusBadRequest, nil, "too_small", []string{"limit"}, "Number must be greater than or equal to 1", map[string]any{"minimum": 1})
			return 0, 0, false
		}
		if n > 100 {
			v5ValidationError(w, http.StatusBadRequest, nil, "too_big", []string{"limit"}, "Number must be less than or equal to 100", map[string]any{"maximum": 100})
			return 0, 0, false
		}
		limit = n
	}
	return page, limit, true
}

func v5ListParams(w http.ResponseWriter, r *http.Request, kind string) (db.ListQuery, bool) {
	page, limit, ok := v5PaginationParams(w, r)
	if !ok {
		return db.ListQuery{}, false
	}
	q := db.ListQuery{Page: page, Limit: limit, Sort: "createdAt", Order: "desc"}
	if v := r.URL.Query().Get("sort"); v != "" {
		switch v {
		case "createdAt", "updatedAt", "position":
			q.Sort = v
		default:
			v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"sort"}, "Invalid option", nil)
			return db.ListQuery{}, false
		}
	}
	if q.Sort == "position" && kind != "chunks" {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"sort"}, "Invalid option", nil)
		return db.ListQuery{}, false
	}
	if v := r.URL.Query().Get("order"); v != "" {
		switch v {
		case "asc", "desc":
			q.Order = v
		default:
			v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"order"}, "Invalid option", nil)
			return db.ListQuery{}, false
		}
	}
	return q, true
}

type v5NamespaceBody struct {
	ID                string `json:"id"`
	Namespace         string `json:"namespace"`
	SupportingContext string `json:"supportingContext"`
	Description       string `json:"description"`
	DocumentCount     int    `json:"documentCount"`
	MemoryCount       int    `json:"memoryCount"`
	System            struct {
		CreatedAt time.Time `json:"createdAt"`
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"system"`
}

func namespaceBody(ns db.Namespace) v5NamespaceBody {
	var b v5NamespaceBody
	b.ID = ns.Name
	b.Namespace = ns.Name
	b.SupportingContext = ns.SupportingContext
	b.Description = ns.Description
	b.System.CreatedAt = ns.CreatedAt
	b.System.UpdatedAt = ns.UpdatedAt
	return b
}

func (h *Handler) v5ListNamespaces(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := v5PaginationParams(w, r)
	if !ok {
		return
	}
	list, total, err := h.Engine.DB.ListNamespaces(page, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list namespaces", err)
		return
	}
	out := make([]v5NamespaceBody, 0, len(list))
	for _, ns := range list {
		b := namespaceBody(ns.Namespace)
		b.DocumentCount = ns.DocumentCount
		b.MemoryCount = ns.MemoryCount
		out = append(out, b)
	}
	v5Write(w, http.StatusOK, map[string]any{
		"namespaces": out,
		"pagination": v5Pagination{CurrentPage: page, Limit: limit, TotalItems: total, TotalPages: v5Pages(total, limit)},
	})
}

func (h *Handler) v5GetNamespace(w http.ResponseWriter, r *http.Request, name string) {
	ns, ok, err := h.Engine.DB.GetNamespace(name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read namespace", err)
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "Namespace not found", nil)
		return
	}
	v5Write(w, http.StatusOK, namespaceBody(ns))
}

func (h *Handler) v5PatchNamespace(w http.ResponseWriter, r *http.Request, name string) {
	if !namespacePattern.MatchString(name) {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"namespace"}, "Invalid namespace", nil)
		return
	}
	raw, ok := v5Decode(w, r, nil, true, "supportingContext", "supportingContext")
	if !ok {
		return
	}
	value, present := raw["supportingContext"]
	if !present {
		v5ValidationError(w, http.StatusBadRequest, map[string]any{}, "invalid_type", []string{"supportingContext"}, "Required", nil)
		return
	}
	var ctx string
	if err := json.Unmarshal(value, &ctx); err != nil {
		v5ValidationError(w, http.StatusBadRequest, map[string]any{}, "invalid_type", []string{"supportingContext"}, "Expected string", nil)
		return
	}
	if len([]rune(ctx)) > 1500 {
		v5ValidationError(w, http.StatusBadRequest, map[string]any{}, "too_big", []string{"supportingContext"}, "String must contain at most 1500 character(s)", map[string]any{"maximum": 1500})
		return
	}
	ns, found, err := h.Engine.DB.SetNamespaceContext(name, ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update namespace", err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Namespace not found", nil)
		return
	}
	v5Write(w, http.StatusOK, namespaceBody(ns))
}

func (h *Handler) v5DeleteNamespace(w http.ResponseWriter, r *http.Request, name string) {
	if name == "sm_project_default" {
		writeErr(w, http.StatusConflict, "Namespace is protected", nil)
		return
	}
	if !namespacePattern.MatchString(name) {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"namespace"}, "Invalid namespace", nil)
		return
	}
	moveTo := r.URL.Query().Get("moveTo")
	if moveTo != "" && !namespacePattern.MatchString(moveTo) {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"moveTo"}, "Invalid namespace", nil)
		return
	}
	if moveTo == "sm_project_default" {
		writeErr(w, http.StatusConflict, "Namespace is protected", nil)
		return
	}
	if _, found, err := h.Engine.DB.GetNamespace(name); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read namespace", err)
		return
	} else if !found {
		writeErr(w, http.StatusNotFound, "Namespace not found", nil)
		return
	}
	if moveTo == "" || moveTo == name {
		// Remove vault files first: a surviving OKF file would be re-imported
		// by the next startup sync, resurrecting the deleted namespace.
		ids, err := h.Engine.DB.MemoryIDsWithFiles(name)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to read namespace files", err)
			return
		}
		for _, id := range ids {
			if err := h.Engine.Vault.DeleteMemory(id); err != nil {
				writeErr(w, http.StatusInternalServerError, "failed to delete namespace files", err)
				return
			}
		}
	}
	docs, mems, err := h.Engine.DB.DeleteNamespace(name, moveTo)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete namespace", err)
		return
	}
	v5Write(w, http.StatusOK, map[string]any{
		"status":                "deleted",
		"namespace":             name,
		"deletedDocumentsCount": docs,
		"deletedMemoriesCount":  mems,
	})
}
