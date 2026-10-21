package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

var filterFieldPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func v5ParseFilter(w http.ResponseWriter, raw map[string]json.RawMessage) (*db.Filter, bool) {
	value, present := raw["filter"]
	if !present {
		return nil, true
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_union", []string{"filter"}, "Invalid filter", nil)
		return nil, false
	}
	fieldRaw, ok := fields["field"]
	var field string
	if !ok || json.Unmarshal(fieldRaw, &field) != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_union", []string{"filter", "field"}, "Invalid input", nil)
		return nil, false
	}
	if !filterFieldPattern.MatchString(field) {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{"filter", "field"}, "Invalid field name", nil)
		return nil, false
	}
	opRaw, ok := fields["operator"]
	var op string
	if !ok || json.Unmarshal(opRaw, &op) != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_union", []string{"filter", "operator"}, "Invalid input", nil)
		return nil, false
	}
	if op != "eq" && op != "neq" {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{"filter", "operator"}, "Invalid option", nil)
		return nil, false
	}
	valRaw, ok := fields["value"]
	if !ok {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"filter", "value"}, "Required", nil)
		return nil, false
	}
	var val any
	if err := json.Unmarshal(valRaw, &val); err != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"filter", "value"}, "Invalid value", nil)
		return nil, false
	}
	f := &db.Filter{Field: field, Operator: op, Value: val}
	if csRaw, ok := fields["caseSensitive"]; ok {
		_ = json.Unmarshal(csRaw, &f.CaseSensitive)
	}
	return f, true
}

func (h *Handler) v5ListContent(w http.ResponseWriter, r *http.Request, ns, kind string) {
	if !v5ValidateNamespace(w, ns) {
		return
	}
	switch kind {
	case "documents", "chunks", "memories":
	default:
		v5RouteNotFound(w, r)
		return
	}
	q, ok := v5ListParams(w, r, kind)
	if !ok {
		return
	}
	raw, ok := v5Decode(w, r, nil, true, "filter", "filter", "include")
	if !ok {
		return
	}
	filter, ok := v5ParseFilter(w, raw)
	if !ok {
		return
	}
	q.Filter = filter
	if incRaw, present := raw["include"]; present {
		var inc map[string]json.RawMessage
		if err := json.Unmarshal(incRaw, &inc); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"include"}, "Expected object", nil)
			return
		}
		if fRaw, ok := inc["forgotten"]; ok {
			_ = json.Unmarshal(fRaw, &q.IncludeForgotten)
		}
	}

	docs, chunks, mems, total, err := h.Engine.DB.ListV5Content(ns, kind, q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list content", err)
		return
	}

	docOut := make([]v5DocListItem, 0, len(docs))
	for _, d := range docs {
		docOut = append(docOut, v5DocListItemFromRecord(d))
	}
	chunkOut := make([]v5Chunk, 0, len(chunks))
	for _, c := range chunks {
		chunkOut = append(chunkOut, v5ChunkFromRecord(c))
	}
	memOut := make([]v5Memory, 0, len(mems))
	for _, m := range mems {
		memOut = append(memOut, v5MemoryFromRecord(m))
	}

	v5Write(w, http.StatusOK, map[string]any{
		"documents":  docOut,
		"chunks":     chunkOut,
		"memories":   memOut,
		"pagination": v5Pagination{CurrentPage: q.Page, Limit: q.Limit, TotalItems: total, TotalPages: v5Pages(total, q.Limit)},
	})
}

func (h *Handler) v5MemoryCounts(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	raw, ok := v5Decode(w, r, nil, false, "ids", "ids")
	if !ok {
		return
	}
	ids, ok := v5ParseIDs(w, raw, "ids", 100)
	if !ok {
		return
	}
	counts, err := h.Engine.DB.MemoryCounts(ns, ids)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to count memories", err)
		return
	}
	type count struct {
		ID          string `json:"id"`
		MemoryCount int    `json:"memoryCount"`
	}
	out := make([]count, 0, len(ids))
	for _, id := range ids {
		out = append(out, count{ID: id, MemoryCount: counts[id]})
	}
	v5Write(w, http.StatusOK, map[string]any{"counts": out})
}
