package api

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) v5ForgetMemories(w http.ResponseWriter, r *http.Request, ns string) {
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
	if _, found, err := h.Engine.DB.GetNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read namespace", err)
		return
	} else if !found {
		writeErr(w, http.StatusNotFound, "Namespace not found", nil)
		return
	}
	matches, errs, err := h.Engine.DB.ForgetMemories(ns, ids)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to forget memories", err)
		return
	}
	out := make([]v5MemoryMatch, 0, len(matches))
	for _, m := range matches {
		out = append(out, v5MemoryMatch{ID: m.ID, Memory: m.Content})
	}
	_ = h.Engine.DB.TouchNamespace(ns)
	v5Write(w, http.StatusOK, map[string]any{"count": len(out), "matches": out, "errors": errs})
}

func (h *Handler) v5ForgetSemantic(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	raw, ok := v5Decode(w, r, nil, false, "query", "query", "dryRun")
	if !ok {
		return
	}
	query, present, valid := v5FieldString(raw, "query")
	if !valid || !present {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"query"}, "Required", nil)
		return
	}
	if query == "" {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_small", []string{"query"}, "String must contain at least 1 character(s)", map[string]any{"minimum": 1})
		return
	}
	if len([]rune(query)) > v5MaxContentRunes {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{"query"}, "String must contain at most 1000000 character(s)", map[string]any{"maximum": v5MaxContentRunes})
		return
	}
	dryRaw, present := raw["dryRun"]
	if !present {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"dryRun"}, "Required", nil)
		return
	}
	var dryRun bool
	if err := json.Unmarshal(dryRaw, &dryRun); err != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"dryRun"}, "Expected boolean", nil)
		return
	}
	if _, found, err := h.Engine.DB.GetNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read namespace", err)
		return
	} else if !found {
		writeErr(w, http.StatusNotFound, "Namespace not found", nil)
		return
	}

	vecs, err := h.Processor.Provider.GenerateEmbeddings([]string{query})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to embed query", err)
		return
	}
	if len(vecs) == 0 {
		writeErr(w, http.StatusInternalServerError, "failed to embed query", nil)
		return
	}
	hits, err := h.Engine.DB.SemanticForget(ns, vecs[0], 100, dryRun)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to forget memories", err)
		return
	}
	out := make([]v5MemoryMatch, 0, len(hits))
	for _, m := range hits {
		out = append(out, v5MemoryMatch{ID: m.ID, Memory: m.Content})
	}
	count := 0
	if !dryRun {
		count = len(out)
		_ = h.Engine.DB.TouchNamespace(ns)
	}
	v5Write(w, http.StatusOK, map[string]any{"count": count, "matches": out, "errors": []any{}})
}
