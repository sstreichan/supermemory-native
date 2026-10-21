package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

func (h *Handler) v5Search(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	start := time.Now()
	raw, ok := v5Decode(w, r, nil, false, "query",
		"query", "filter", "searchMode", "limit", "include", "threshold", "rerank", "rewriteQuery")
	if !ok {
		return
	}

	query, present, valid := v5FieldString(raw, "query")
	if !valid {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"query"}, "Expected string", nil)
		return
	}
	if !present {
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

	limit := 10
	if limRaw, present := raw["limit"]; present {
		var n int
		if err := json.Unmarshal(limRaw, &n); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"limit"}, "Expected number", nil)
			return
		}
		if n < 1 {
			v5ValidationError(w, http.StatusBadRequest, raw, "too_small", []string{"limit"}, "Number must be greater than or equal to 1", map[string]any{"minimum": 1})
			return
		}
		if n > 100 {
			v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{"limit"}, "Number must be less than or equal to 100", map[string]any{"maximum": 100})
			return
		}
		limit = n
	}

	threshold := float32(0.3)
	if tRaw, present := raw["threshold"]; present {
		var f float64
		if err := json.Unmarshal(tRaw, &f); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"threshold"}, "Expected number", nil)
			return
		}
		threshold = float32(f)
	}

	mode := "hybrid"
	if modeRaw, present := raw["searchMode"]; present {
		if err := json.Unmarshal(modeRaw, &mode); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"searchMode"}, "Expected string", nil)
			return
		}
		switch mode {
		case "hybrid", "memories", "chunks":
		default:
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{"searchMode"}, "Invalid option", nil)
			return
		}
	}

	rerank := "none"
	if rrRaw, present := raw["rerank"]; present {
		if err := json.Unmarshal(rrRaw, &rerank); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"rerank"}, "Expected string", nil)
			return
		}
		switch rerank {
		case "none", "order", "aggregate":
		default:
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{"rerank"}, "Invalid option", nil)
			return
		}
	}

	rewriteQuery := false
	if rwRaw, present := raw["rewriteQuery"]; present {
		if err := json.Unmarshal(rwRaw, &rewriteQuery); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"rewriteQuery"}, "Expected boolean", nil)
			return
		}
	}

	includeDocs, includeRelated, includeForgotten := false, false, false
	if incRaw, present := raw["include"]; present {
		var inc map[string]json.RawMessage
		if err := json.Unmarshal(incRaw, &inc); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"include"}, "Expected object", nil)
			return
		}
		if v, ok := inc["documents"]; ok {
			_ = json.Unmarshal(v, &includeDocs)
		}
		if v, ok := inc["related"]; ok {
			_ = json.Unmarshal(v, &includeRelated)
		}
		if v, ok := inc["forgotten"]; ok {
			_ = json.Unmarshal(v, &includeForgotten)
		}
	}

	filter, ok := v5ParseFilter(w, raw)
	if !ok {
		return
	}

	effectiveQuery := query
	if rewriteQuery {
		effectiveQuery = h.rewriteQuery(r.Context(), query)
	}

	vecs, err := h.Processor.Provider.GenerateEmbeddings([]string{effectiveQuery})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to embed query", err)
		return
	}
	if len(vecs) == 0 {
		writeErr(w, http.StatusInternalServerError, "failed to embed query", nil)
		return
	}

	candidates, err := h.Engine.DB.RetrievalCandidates(ns, vecs[0], mode, includeForgotten, filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to search", err)
		return
	}
	vectorHits := make([]db.Hit, 0, len(candidates))
	for _, hit := range candidates {
		if hit.Similarity >= threshold {
			vectorHits = append(vectorHits, hit)
		}
	}

	hits := vectorHits
	if rerank == "order" || rerank == "aggregate" {
		hits = fuseRRF(vectorHits, lexicalRanking(effectiveQuery, vectorHits), 0)
		if h.LLM != nil {
			hits = h.rerankWithLLM(r.Context(), effectiveQuery, hits)
		}
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}

	results := make([]v5SearchHit, 0, len(hits)+1)
	if rerank == "aggregate" && len(hits) > 0 {
		top := hits[0]
		agg := v5HitToWire(top)
		agg.ID = fmt.Sprintf("aggregated_0_%d", time.Now().UnixNano())
		results = append(results, agg)
	}
	for _, hit := range hits {
		wire := v5HitToWire(hit)
		included := &v5Included{}
		hasIncluded := false
		if includeDocs && hit.DocumentID != "" {
			if doc, ok, err := h.Engine.DB.GetDocument(ns, hit.DocumentID); err == nil && ok {
				included.Document = v5IncludedDoc(doc)
				hasIncluded = true
			}
		}
		if includeRelated && hit.DocumentID != "" {
			var siblings []db.MemoryRecord
			var parents, children []db.MemoryRecord
			if hit.Kind == "memory" {
				parents, children, siblings, err = h.Engine.DB.RelatedMemories(ns, hit.ID, 10)
			} else {
				siblings, err = h.Engine.DB.GetMemoriesByDocument(ns, hit.DocumentID, 1000)
			}
			if err == nil {
				included.Related = v5RelatedRecords(hit.ID, parents, children, siblings, 10)
				hasIncluded = true
			}
		}
		if hasIncluded {
			wire.Included = included
		}
		results = append(results, wire)
	}

	v5Write(w, http.StatusOK, map[string]any{
		"results":    results,
		"searchTime": time.Since(start).Milliseconds(),
	})
}
