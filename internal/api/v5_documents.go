package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/memory"
)

const (
	v5MaxContentRunes = 1000000
	v5MaxContextRunes = 1500
)

var errV5ContentTooBig = errors.New("document content exceeds the maximum size")

type v5DocInput struct {
	Content    string
	HasContent bool
	ID         string
	Metadata   map[string]any
	HasMeta    bool
	Grp        map[string]any
	Date       string
	HasDate    bool
	TaskType   string
	HasTask    bool
	Dreaming   string
	HasDream   bool
	Context    string
	HasContext bool
}

func jsonTypeName(raw json.RawMessage) string {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return "null"
	}
	switch t[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	default:
		return "number"
	}
}

func v5FieldString(raw map[string]json.RawMessage, key string) (string, bool, bool) {
	r, ok := raw[key]
	if !ok {
		return "", false, true
	}
	var s string
	if err := json.Unmarshal(r, &s); err != nil {
		return "", true, false
	}
	return s, true, true
}

func readRunes(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}

func v5AttachTemporal(meta map[string]any, date string) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	if date == "" {
		return meta
	}
	meta["temporalContext"] = map[string]any{"documentDate": date, "eventDate": nil}
	return meta
}

// v5ParseDocInput validates document fields. When emit is false it reports
// failure without writing a response (batch items).
func v5ParseDocInput(w http.ResponseWriter, raw map[string]json.RawMessage, requireContent, emit bool) (v5DocInput, bool) {
	in := v5DocInput{}
	fail := func(code string, path []string, msg string, extra map[string]any) (v5DocInput, bool) {
		if emit {
			v5ValidationError(w, http.StatusBadRequest, raw, code, path, msg, extra)
		}
		return v5DocInput{}, false
	}

	content, present, ok := v5FieldString(raw, "content")
	if !ok {
		return fail("invalid_type", []string{"content"}, "Expected string, received "+jsonTypeName(raw["content"]), nil)
	}
	if !present {
		if requireContent {
			return fail("invalid_type", []string{"content"}, "Required", nil)
		}
	} else {
		in.Content, in.HasContent = content, true
		if content == "" {
			return fail("too_small", []string{"content"}, "String must contain at least 1 character(s)", map[string]any{"minimum": 1})
		}
		if len([]rune(content)) > v5MaxContentRunes {
			return fail("too_big", []string{"content"}, "String must contain at most 1000000 character(s)", map[string]any{"maximum": v5MaxContentRunes})
		}
	}

	if id, present, ok := v5FieldString(raw, "id"); !ok {
		return fail("invalid_type", []string{"id"}, "Expected string, received "+jsonTypeName(raw["id"]), nil)
	} else if present {
		if id == "" {
			return fail("too_small", []string{"id"}, "String must contain at least 1 character(s)", map[string]any{"minimum": 1})
		}
		if len(id) > 255 {
			return fail("too_big", []string{"id"}, "String must contain at most 255 character(s)", map[string]any{"maximum": 255})
		}
		in.ID = id
	}

	for key, dst := range map[string]*map[string]any{"metadata": &in.Metadata, "group": &in.Grp} {
		r, present := raw[key]
		if !present {
			continue
		}
		if jsonTypeName(r) != "object" {
			return fail("invalid_type", []string{key}, "Expected object, received "+jsonTypeName(r), nil)
		}
		var m map[string]any
		if err := json.Unmarshal(r, &m); err != nil {
			return fail("invalid_type", []string{key}, "Expected object, received "+jsonTypeName(r), nil)
		}
		*dst = m
		if key == "metadata" {
			in.HasMeta = true
		}
	}

	if date, present, ok := v5FieldString(raw, "date"); !ok {
		return fail("invalid_type", []string{"date"}, "Expected string, received "+jsonTypeName(raw["date"]), nil)
	} else if present {
		if _, valid := parseDateValue(date); !valid {
			return fail("invalid_value", []string{"date"}, "Invalid date", nil)
		}
		in.Date, in.HasDate = date, true
	}

	if tt, present, ok := v5FieldString(raw, "taskType"); !ok {
		return fail("invalid_type", []string{"taskType"}, "Expected string", nil)
	} else if present {
		if tt != "memory" && tt != "superrag" {
			return fail("invalid_value", []string{"taskType"}, "Invalid option", nil)
		}
		in.TaskType, in.HasTask = tt, true
	}

	if dr, present, ok := v5FieldString(raw, "dreaming"); !ok {
		return fail("invalid_type", []string{"dreaming"}, "Expected string", nil)
	} else if present {
		if dr != "dynamic" && dr != "instant" {
			return fail("invalid_value", []string{"dreaming"}, "Invalid option", nil)
		}
		in.Dreaming, in.HasDream = dr, true
	}

	if ctx, present, ok := v5FieldString(raw, "supportingContext"); !ok {
		return fail("invalid_type", []string{"supportingContext"}, "Expected string", nil)
	} else if present {
		if len([]rune(ctx)) > v5MaxContextRunes {
			return fail("too_big", []string{"supportingContext"}, "String must contain at most 1500 character(s)", map[string]any{"maximum": v5MaxContextRunes})
		}
		in.Context, in.HasContext = ctx, true
	}

	return in, true
}

func parseDateValue(value string) (time.Time, bool) {
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func v5ValidateNamespace(w http.ResponseWriter, ns string) bool {
	if namespacePattern.MatchString(ns) {
		return true
	}
	v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"namespace"}, "Invalid namespace", nil)
	return false
}

func (h *Handler) v5ApplyDocument(ns string, in v5DocInput, defaultTask, defaultDream string) (string, string, error) {
	now := time.Now().UTC()
	existing, found, err := h.Engine.DB.GetDocument(ns, in.ID)
	if err != nil {
		return "", "", err
	}

	taskType := defaultTask
	if taskType == "" {
		taskType = "memory"
	}
	dreaming := defaultDream
	if dreaming == "" {
		dreaming = "dynamic"
	}

	if found {
		if existing.Content == in.Content {
			return existing.ID, existing.Status, nil
		}
		joined := existing.Content + "\n" + in.Content
		if len([]rune(joined)) > v5MaxContentRunes {
			return "", "", errV5ContentTooBig
		}
		existing.Content = joined
		existing.Title = documentTitle(existing.Content)
		existing.Summary = readRunes(existing.Content, 200)
		existing.Status = db.DocumentQueued
		existing.DreamingStatus = db.DocumentQueued
		existing.ResetMemories = false
		existing.UpdatedAt = now
		if in.HasMeta {
			existing.Metadata = v5AttachTemporal(in.Metadata, in.Date)
		}
		if in.HasDate {
			existing.Date = in.Date
			existing.Metadata = v5AttachTemporal(existing.Metadata, in.Date)
		}
		if in.HasTask {
			existing.TaskType = in.TaskType
		} else if in.TaskType == "" && defaultTask != "" {
			existing.TaskType = defaultTask
		}
		if in.HasDream {
			existing.Dreaming = in.Dreaming
		}
		if in.HasContext {
			existing.SupportingContext = in.Context
		}
		if err := h.Engine.DB.UpsertDocument(existing); err != nil {
			return "", "", err
		}
		return existing.ID, existing.Status, nil
	}

	id := in.ID
	if id == "" {
		id = memory.GenerateUUID()
	}
	doc := db.Document{
		Namespace:         ns,
		ID:                id,
		Title:             documentTitle(in.Content),
		Summary:           readRunes(in.Content, 200),
		Content:           in.Content,
		SupportingContext: in.Context,
		Date:              in.Date,
		TaskType:          taskType,
		Dreaming:          dreaming,
		Status:            db.DocumentQueued,
		DreamingStatus:    db.DocumentQueued,
		Metadata:          v5AttachTemporal(in.Metadata, in.Date),
		Grp:               in.Grp,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := h.Engine.DB.UpsertDocument(doc); err != nil {
		return "", "", err
	}
	return id, doc.Status, nil
}

func (h *Handler) v5CreateDocument(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	raw, ok := v5Decode(w, r, nil, false, "content",
		"content", "id", "metadata", "group", "date", "taskType", "dreaming", "supportingContext")
	if !ok {
		return
	}
	in, ok := v5ParseDocInput(w, raw, true, true)
	if !ok {
		return
	}
	if _, err := h.Engine.DB.EnsureNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create namespace", err)
		return
	}
	id, status, err := h.v5ApplyDocument(ns, in, "", "")
	if errors.Is(err, errV5ContentTooBig) {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{"content"}, "String must contain at most 1000000 character(s)", map[string]any{"maximum": v5MaxContentRunes})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to store document", err)
		return
	}
	if err := h.Engine.DB.TouchNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update namespace", err)
		return
	}
	v5Write(w, http.StatusOK, map[string]any{"id": id, "status": status})
}

func (h *Handler) v5CreateDocumentBatch(w http.ResponseWriter, r *http.Request, ns string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	raw, ok := v5Decode(w, r, nil, false, "documents", "documents", "taskType", "dreaming")
	if !ok {
		return
	}
	docsRaw, present := raw["documents"]
	if !present {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"documents"}, "Required", nil)
		return
	}
	var items []json.RawMessage
	if err := json.Unmarshal(docsRaw, &items); err != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"documents"}, "Expected array", nil)
		return
	}
	if len(items) == 0 {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_small", []string{"documents"}, "Array must contain at least 1 element(s)", map[string]any{"minimum": 1})
		return
	}
	if len(items) > 600 {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{"documents"}, "Array must contain at most 600 element(s)", map[string]any{"maximum": 600})
		return
	}
	defaultTask, defaultDream := "", ""
	if v, p, valid := v5FieldString(raw, "taskType"); valid && p {
		if v != "memory" && v != "superrag" {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{"taskType"}, "Invalid option", nil)
			return
		}
		defaultTask = v
	}
	if v, p, valid := v5FieldString(raw, "dreaming"); valid && p {
		if v != "dynamic" && v != "instant" {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{"dreaming"}, "Invalid option", nil)
			return
		}
		defaultDream = v
	}

	if _, err := h.Engine.DB.EnsureNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create namespace", err)
		return
	}

	type result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	results := make([]result, 0, len(items))
	failed := 0
	for _, item := range items {
		fields, ok := v5DecodeItem(w, item, nil, "content", "id", "metadata", "group", "date", "taskType", "supportingContext")
		if !ok {
			return
		}
		in, ok := v5ParseDocInput(w, fields, true, false)
		if !ok {
			failed++
			continue
		}
		id, status, err := h.v5ApplyDocument(ns, in, defaultTask, defaultDream)
		if err != nil {
			failed++
			continue
		}
		results = append(results, result{ID: id, Status: status})
	}
	if err := h.Engine.DB.TouchNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update namespace", err)
		return
	}
	v5Write(w, http.StatusOK, map[string]any{"results": results, "count": len(results), "failed": failed})
}

func v5ParseInclude(values []string, allowed ...string) (map[string]bool, string, bool) {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	out := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !set[part] {
				return nil, part, false
			}
			out[part] = true
		}
	}
	return out, "", true
}

func (h *Handler) v5GetDocument(w http.ResponseWriter, r *http.Request, ns, id string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r, "include") {
		return
	}
	include, bad, ok := v5ParseInclude(r.URL.Query()["include"], "chunks", "memories")
	if !ok {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"include"}, "Invalid option: "+bad, nil)
		return
	}
	doc, found, err := h.Engine.DB.GetDocument(ns, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read document", err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Document not found", nil)
		return
	}
	var chunks []db.Chunk
	var mems []db.MemoryRecord
	if include["chunks"] {
		chunks, err = h.Engine.DB.GetChunks(ns, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to read chunks", err)
			return
		}
	}
	if include["memories"] {
		mems, err = h.Engine.DB.GetMemoriesByDocument(ns, id, 1000)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to read memories", err)
			return
		}
	}
	v5Write(w, http.StatusOK, v5DocFromRecord(doc, chunks, mems))
}

func (h *Handler) v5PatchDocument(w http.ResponseWriter, r *http.Request, ns, id string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	raw, ok := v5Decode(w, r, nil, false, "content",
		"content", "metadata", "group", "date", "taskType", "dreaming", "supportingContext")
	if !ok {
		return
	}
	if len(raw) == 0 {
		v5ValidationError(w, http.StatusBadRequest, raw, "custom", nil, "At least one document field must be provided", nil)
		return
	}
	_, hasContent := raw["content"]
	if _, hasGroup := raw["group"]; hasGroup && !hasContent {
		v5ValidationError(w, http.StatusBadRequest, raw, "custom", nil, "group only applies on re-ingestion; this update does not replace the document content", nil)
		return
	}
	in, ok := v5ParseDocInput(w, raw, hasContent, true)
	if !ok {
		return
	}
	existing, found, err := h.Engine.DB.GetDocument(ns, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read document", err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Document not found", nil)
		return
	}

	fields := map[string]any{"updated_at": time.Now().UTC()}
	if in.HasContent {
		fields["content"] = in.Content
		fields["title"] = documentTitle(in.Content)
		fields["summary"] = readRunes(in.Content, 200)
		fields["status"] = db.DocumentQueued
		fields["dreaming_status"] = db.DocumentQueued
		fields["reset_memories"] = 1
	}
	if in.HasMeta {
		fields["metadata"] = v5AttachTemporal(in.Metadata, in.Date)
	}
	if in.HasDate {
		fields["source_date"] = in.Date
		if !in.HasMeta {
			fields["metadata"] = v5AttachTemporal(existing.Metadata, in.Date)
		}
	}
	if in.HasTask {
		fields["task_type"] = in.TaskType
	}
	if in.HasDream {
		fields["dreaming"] = in.Dreaming
	}
	if in.HasContext {
		fields["supporting_context"] = in.Context
	}
	if len(in.Grp) > 0 {
		fields["grp"] = in.Grp
	}

	doc, updated, err := h.Engine.DB.UpdateDocumentFields(ns, id, fields)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update document", err)
		return
	}
	if !updated {
		writeErr(w, http.StatusNotFound, "Document not found", nil)
		return
	}
	_ = h.Engine.DB.TouchNamespace(ns)
	v5Write(w, http.StatusOK, map[string]any{"id": doc.ID, "status": doc.Status})
}

func (h *Handler) v5DeleteDocuments(w http.ResponseWriter, r *http.Request, ns string) {
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
	count, errs, err := h.Engine.DB.DeleteDocumentRecords(ns, ids)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete documents", err)
		return
	}
	_ = h.Engine.DB.TouchNamespace(ns)
	v5Write(w, http.StatusOK, map[string]any{"count": count, "errors": errs})
}

// v5ParseIDs validates a JSON array of 1..max non-empty string IDs.
func v5ParseIDs(w http.ResponseWriter, raw map[string]json.RawMessage, key string, max int) ([]string, bool) {
	value, present := raw[key]
	if !present {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{key}, "Required", nil)
		return nil, false
	}
	var ids []string
	if err := json.Unmarshal(value, &ids); err != nil {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{key}, "Expected array of strings", nil)
		return nil, false
	}
	if len(ids) == 0 {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_small", []string{key}, "Array must contain at least 1 element(s)", map[string]any{"minimum": 1})
		return nil, false
	}
	if len(ids) > max {
		v5ValidationError(w, http.StatusBadRequest, raw, "too_big", []string{key}, "Array must contain at most 100 element(s)", map[string]any{"maximum": max})
		return nil, false
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || len(id) > 255 {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_value", []string{key}, "Invalid id", nil)
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, true
}

func (h *Handler) v5GetMemory(w http.ResponseWriter, r *http.Request, ns, id string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r, "include", "relatedLimit") {
		return
	}
	include, bad, ok := v5ParseInclude(r.URL.Query()["include"], "related", "documents")
	if !ok {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"include"}, "Invalid option: "+bad, nil)
		return
	}
	relatedLimit := 10
	if v := r.URL.Query().Get("relatedLimit"); v != "" {
		page, limit, ok := v5PaginationParamsFor(w, v, 1, 100, 10, "relatedLimit")
		_ = page
		if !ok {
			return
		}
		relatedLimit = limit
	}
	mem, found, err := h.Engine.DB.GetMemory(ns, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read memory", err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Memory not found", nil)
		return
	}
	out := v5MemoryWithIncluded{v5Memory: v5MemoryFromRecord(mem)}
	if include["documents"] || include["related"] {
		var included v5Included
		if include["documents"] && mem.DocumentID != "" {
			if doc, ok, err := h.Engine.DB.GetDocument(ns, mem.DocumentID); err == nil && ok {
				included.Document = v5IncludedDoc(doc)
			}
		}
		if include["related"] && mem.DocumentID != "" {
			siblings, err := h.Engine.DB.GetMemoriesByDocument(ns, mem.DocumentID, 1000)
			if err == nil {
				included.Related = v5RelatedFrom(mem.ID, siblings, relatedLimit)
			}
		}
		out.Included = &included
	}
	v5Write(w, http.StatusOK, out)
}

// v5PaginationParamsFor parses one bounded integer query parameter.
func v5PaginationParamsFor(w http.ResponseWriter, value string, min, max, def int, field string) (int, int, bool) {
	n, err := strconv.Atoi(value)
	if err != nil {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{field}, "Expected number", nil)
		return 0, 0, false
	}
	if n < min {
		v5ValidationError(w, http.StatusBadRequest, nil, "too_small", []string{field}, "Number is too small", map[string]any{"minimum": min})
		return 0, 0, false
	}
	if n > max {
		v5ValidationError(w, http.StatusBadRequest, nil, "too_big", []string{field}, "Number is too big", map[string]any{"maximum": max})
		return 0, 0, false
	}
	return 1, n, true
}
