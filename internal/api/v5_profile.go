package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

var bucketKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (h *Handler) v5Profile(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) {
		return
	}
	raw, ok := v5Decode(w, r, nil, true, "filter", "filter", "buckets")
	if !ok {
		return
	}
	filter, ok := v5ParseFilter(w, raw)
	if !ok {
		return
	}
	requested := []string{}
	if bRaw, present := raw["buckets"]; present {
		if jsonTypeName(bRaw) != "array" {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Expected array, received "+jsonTypeName(bRaw), nil)
			return
		}
		if err := json.Unmarshal(bRaw, &requested); err != nil {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Expected array of strings", nil)
			return
		}
		if len(requested) > 50 {
			v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{"buckets"}, "Array must contain at most 50 element(s)", map[string]any{"maximum": 50})
			return
		}
	}

	q := db.ListQuery{Page: 1, Limit: 100, Sort: "updatedAt", Order: "desc", Filter: filter}
	_, _, mems, _, err := h.Engine.DB.ListV5Content(ns, "memories", q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to build profile", err)
		return
	}
	type dynamicEntry struct {
		ID     string `json:"id"`
		Memory string `json:"memory"`
	}
	dynamic := make([]dynamicEntry, 0, len(mems))
	for _, m := range mems {
		prefix := ""
		if !m.CreatedAt.IsZero() {
			prefix = "[" + m.CreatedAt.UTC().Format("2006-01-02") + "] "
		}
		dynamic = append(dynamic, dynamicEntry{ID: m.ID, Memory: prefix + m.Content})
	}

	effective, err := h.Engine.DB.ListBuckets(ns)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list buckets", err)
		return
	}
	buckets := map[string]string{}
	if _, present := raw["buckets"]; present {
		for _, key := range requested {
			if v, ok := effective[key]; ok {
				buckets[key] = v
			}
		}
	} else {
		buckets = effective
	}

	static := h.staticProfile(ns, filter, mems)
	v5Write(w, http.StatusOK, map[string]any{
		"profile": map[string]any{
			"static":  static,
			"dynamic": dynamic,
			"buckets": buckets,
		},
	})
}

func (h *Handler) v5GetBuckets(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	buckets, err := h.Engine.DB.ListBuckets(ns)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list buckets", err)
		return
	}
	v5Write(w, http.StatusOK, map[string]any{"buckets": buckets})
}

func v5ProtectedBucket(key string) bool {
	_, ok := db.DefaultBuckets[key]
	return ok
}

func (h *Handler) v5PutBuckets(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	raw, ok := v5Decode(w, r, nil, false, "buckets", "buckets")
	if !ok {
		return
	}
	value, present := raw["buckets"]
	if !present {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Required", nil)
		return
	}
	if jsonTypeName(value) != "object" {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Expected object, received "+jsonTypeName(value), nil)
		return
	}
	var buckets map[string]string
	if err := json.Unmarshal(value, &buckets); err != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Expected object of strings", nil)
		return
	}
	for key, val := range buckets {
		if !bucketKeyPattern.MatchString(key) || len(key) > 64 {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{"buckets", key}, "Invalid bucket key", nil)
			return
		}
		if len([]rune(val)) > 2000 {
			v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{"buckets", key}, "String must contain at most 2000 character(s)", map[string]any{"maximum": 2000})
			return
		}
		if v5ProtectedBucket(key) {
			writeErr(w, http.StatusConflict, "Bucket is protected", nil)
			return
		}
	}
	effective, found, err := h.Engine.DB.PutBuckets(ns, buckets)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to store buckets", err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Namespace not found", nil)
		return
	}
	_ = h.Engine.DB.TouchNamespace(ns)
	v5Write(w, http.StatusOK, map[string]any{"buckets": effective})
}

func (h *Handler) v5DeleteBuckets(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	raw, ok := v5Decode(w, r, nil, false, "buckets", "buckets")
	if !ok {
		return
	}
	value, present := raw["buckets"]
	if !present {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Required", nil)
		return
	}
	if jsonTypeName(value) != "array" {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Expected array, received "+jsonTypeName(value), nil)
		return
	}
	var keys []string
	if err := json.Unmarshal(value, &keys); err != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"buckets"}, "Expected array of strings", nil)
		return
	}
	if len(keys) == 0 {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_small", []string{"buckets"}, "Array must contain at least 1 element(s)", map[string]any{"minimum": 1})
		return
	}
	if len(keys) > 50 {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{"buckets"}, "Array must contain at most 50 element(s)", map[string]any{"maximum": 50})
		return
	}
	for _, key := range keys {
		if v5ProtectedBucket(key) {
			writeErr(w, http.StatusConflict, "Bucket is protected", nil)
			return
		}
	}
	effective, found, err := h.Engine.DB.DeleteBuckets(ns, keys)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete buckets", err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Namespace not found", nil)
		return
	}
	_ = h.Engine.DB.TouchNamespace(ns)
	v5Write(w, http.StatusOK, map[string]any{"buckets": effective})
}
