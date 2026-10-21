package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/vector"
)

// Namespace scopes documents, chunks, memories and buckets.
type Namespace struct {
	Name              string
	SupportingContext string
	Description       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NamespaceStats is a namespace plus the row counts exposed by GET /namespaces.
type NamespaceStats struct {
	Namespace
	DocumentCount int
	MemoryCount   int
}

// Document is one ingested v5 document.
type Document struct {
	Namespace         string
	ID                string
	Title             string
	Summary           string
	Content           string
	SupportingContext string
	Date              string
	TaskType          string
	Dreaming          string
	Status            string
	DreamingStatus    string
	Metadata          map[string]any
	Grp               map[string]any
	ResetMemories     bool
	FileName          string
	MIMEType          string
	FileType          string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Chunk is one embedded slice of a document.
type Chunk struct {
	ID         string
	Namespace  string
	DocumentID string
	Content    string
	Type       string
	Position   int
	Metadata   map[string]any
	Vector     []float32
	CreatedAt  time.Time
}

// MemoryRecord is one learned memory produced by the processor.
type MemoryRecord struct {
	ID          string
	Namespace   string
	DocumentID  string
	Content     string
	Metadata    map[string]any
	Vector      []float32
	IsForgotten bool
	IsInference bool
	IsStatic    bool
	IsLatest    bool
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MemoryHit couples a memory with its similarity to a query vector.
type MemoryHit struct {
	MemoryRecord
	Similarity float32
}

// Hit is a unified search result over memories and chunks.
type Hit struct {
	Kind        string
	ID          string
	Text        string
	DocumentID  string
	Metadata    map[string]any
	Similarity  float32
	IsLatest    bool
	IsInference bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ItemError is a per-ID failure in bulk operations.
type ItemError struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// Filter is the metadata filter accepted by list, search and profile.
type Filter struct {
	Field         string
	Operator      string
	Value         any
	CaseSensitive bool
}

// ListQuery carries the validated list parameters.
type ListQuery struct {
	Page             int
	Limit            int
	Sort             string
	Order            string
	Filter           *Filter
	IncludeForgotten bool
}

const (
	DocumentQueued     = "queued"
	DocumentExtracting = "extracting"
	DocumentChunking   = "chunking"
	DocumentEmbedding  = "embedding"
	DocumentIndexing   = "indexing"
	DocumentDone       = "done"
	DocumentFailed     = "failed"
)

// DefaultBuckets are the always-present profile buckets. They are constants,
// not rows, so a namespace deletion cannot remove them.
var DefaultBuckets = map[string]string{
	"preferences": "User preferences and working style",
	"tech_stack":  "Languages, frameworks and tools the user works with",
}

func v5Time(v any) time.Time {
	switch t := v.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return t
	case string:
		return parseV5Time(t)
	case []byte:
		return parseV5Time(string(t))
	default:
		return time.Time{}
	}
}

func parseV5Time(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func encodeMap(m map[string]any) string {
	if m == nil {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func decodeMap(raw string) map[string]any {
	m := map[string]any{}
	if raw == "" {
		return m
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}
	}
	return m
}

func (s *SqliteDB) EnsureNamespace(name string) (Namespace, error) {
	now := time.Now().UTC()
	if _, err := s.db.Exec(
		"INSERT OR IGNORE INTO namespaces (name, supporting_context, description, created_at, updated_at) VALUES (?, '', '', ?, ?)",
		name, now, now,
	); err != nil {
		return Namespace{}, err
	}
	ns, _, err := s.GetNamespace(name)
	return ns, err
}

func (s *SqliteDB) GetNamespace(name string) (Namespace, bool, error) {
	var ns Namespace
	var created, updated any
	err := s.db.QueryRow(
		"SELECT name, supporting_context, description, created_at, updated_at FROM namespaces WHERE name = ?", name,
	).Scan(&ns.Name, &ns.SupportingContext, &ns.Description, &created, &updated)
	if err == sql.ErrNoRows {
		return Namespace{}, false, nil
	}
	if err != nil {
		return Namespace{}, false, err
	}
	ns.CreatedAt = v5Time(created)
	ns.UpdatedAt = v5Time(updated)
	return ns, true, nil
}

func (s *SqliteDB) SetNamespaceContext(name, ctx string) (Namespace, bool, error) {
	res, err := s.db.Exec("UPDATE namespaces SET supporting_context = ?, updated_at = ? WHERE name = ?", ctx, time.Now().UTC(), name)
	if err != nil {
		return Namespace{}, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Namespace{}, false, nil
	}
	ns, ok, err := s.GetNamespace(name)
	return ns, ok, err
}

func (s *SqliteDB) TouchNamespace(name string) error {
	_, err := s.db.Exec("UPDATE namespaces SET updated_at = ? WHERE name = ?", time.Now().UTC(), name)
	return err
}

func (s *SqliteDB) ListNamespaces(page, limit int) ([]NamespaceStats, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM namespaces").Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := int64(page-1) * int64(limit)
	if offset < 0 || offset >= int64(total) {
		return []NamespaceStats{}, total, nil
	}
	rows, err := s.db.Query(`
		SELECT n.name, n.supporting_context, n.description, n.created_at, n.updated_at,
		       (SELECT COUNT(*) FROM documents d WHERE d.namespace = n.name),
		       (SELECT COUNT(*) FROM memories m WHERE m.container_tag = n.name)
		FROM namespaces n
		ORDER BY n.updated_at DESC, n.name ASC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]NamespaceStats, 0, limit)
	for rows.Next() {
		var st NamespaceStats
		var created, updated any
		if err := rows.Scan(&st.Name, &st.SupportingContext, &st.Description, &created, &updated, &st.DocumentCount, &st.MemoryCount); err != nil {
			return nil, 0, err
		}
		st.CreatedAt = v5Time(created)
		st.UpdatedAt = v5Time(updated)
		out = append(out, st)
	}
	return out, total, rows.Err()
}

// DeleteNamespace removes a namespace and its rows. With moveTo it first
// moves every row to the target namespace. Counts describe the source rows.
func (s *SqliteDB) DeleteNamespace(name, moveTo string) (docs, mems int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	if err := tx.QueryRow("SELECT COUNT(*) FROM documents WHERE namespace = ?", name).Scan(&docs); err != nil {
		return 0, 0, err
	}
	if err := tx.QueryRow("SELECT COUNT(*) FROM memories WHERE container_tag = ?", name).Scan(&mems); err != nil {
		return 0, 0, err
	}

	if moveTo != "" && moveTo != name {
		now := time.Now().UTC()
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO namespaces (name, supporting_context, description, created_at, updated_at) VALUES (?, '', '', ?, ?)",
			moveTo, now, now,
		); err != nil {
			return 0, 0, err
		}
		// A same-id document in the target is replaced below; drop its derived
		// rows first so the moved document does not inherit stale chunks or
		// memories.
		if _, err := tx.Exec(
			"DELETE FROM chunks WHERE namespace = ? AND document_id IN (SELECT id FROM documents WHERE namespace = ?)",
			moveTo, name,
		); err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec(
			"DELETE FROM memories WHERE container_tag = ? AND document_id IN (SELECT id FROM documents WHERE namespace = ?)",
			moveTo, name,
		); err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec("UPDATE OR REPLACE documents SET namespace = ? WHERE namespace = ?", moveTo, name); err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec("UPDATE chunks SET namespace = ? WHERE namespace = ?", moveTo, name); err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec("UPDATE memories SET container_tag = ? WHERE container_tag = ?", moveTo, name); err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec(
			"INSERT OR REPLACE INTO buckets (namespace, key, description) SELECT ?, key, description FROM buckets WHERE namespace = ?",
			moveTo, name,
		); err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec("DELETE FROM buckets WHERE namespace = ?", name); err != nil {
			return 0, 0, err
		}
	}

	for _, q := range []string{
		"DELETE FROM chunks WHERE namespace = ?",
		"DELETE FROM documents WHERE namespace = ?",
		"DELETE FROM memories WHERE container_tag = ?",
		"DELETE FROM buckets WHERE namespace = ?",
		"DELETE FROM namespaces WHERE name = ?",
	} {
		if _, err := tx.Exec(q, name); err != nil {
			return 0, 0, err
		}
	}
	return docs, mems, tx.Commit()
}

// UpsertDocument inserts or fully replaces a document row.
func (s *SqliteDB) UpsertDocument(d Document) error {
	_, err := s.db.Exec(`
		INSERT INTO documents (
			namespace, id, title, summary, content, metadata, grp, supporting_context,
			source_date, task_type, dreaming, status, dreaming_status, reset_memories,
			file_name, mime_type, file_type,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(namespace, id) DO UPDATE SET
			title = excluded.title,
			summary = excluded.summary,
			content = excluded.content,
			metadata = excluded.metadata,
			grp = excluded.grp,
			supporting_context = excluded.supporting_context,
			source_date = excluded.source_date,
			task_type = excluded.task_type,
			dreaming = excluded.dreaming,
			status = excluded.status,
			dreaming_status = excluded.dreaming_status,
			reset_memories = excluded.reset_memories,
			file_name = excluded.file_name,
			mime_type = excluded.mime_type,
			file_type = excluded.file_type,
			updated_at = excluded.updated_at`,
		d.Namespace, d.ID, d.Title, d.Summary, d.Content, encodeMap(d.Metadata), encodeMap(d.Grp),
		d.SupportingContext, d.Date, d.TaskType, d.Dreaming, d.Status, d.DreamingStatus,
		boolInt(d.ResetMemories), d.FileName, d.MIMEType, d.FileType, d.CreatedAt, d.UpdatedAt,
	)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDocument(sc rowScanner) (Document, error) {
	var d Document
	var metadata, grp string
	var reset int
	var created, updated any
	err := sc.Scan(
		&d.Namespace, &d.ID, &d.Title, &d.Summary, &d.Content, &metadata, &grp,
		&d.SupportingContext, &d.Date, &d.TaskType, &d.Dreaming, &d.Status,
		&d.DreamingStatus, &reset, &d.FileName, &d.MIMEType, &d.FileType, &created, &updated,
	)
	if err != nil {
		return Document{}, err
	}
	d.Metadata = decodeMap(metadata)
	d.Grp = decodeMap(grp)
	d.ResetMemories = reset != 0
	d.CreatedAt = v5Time(created)
	d.UpdatedAt = v5Time(updated)
	return d, nil
}

const documentColumns = `namespace, id, title, summary, content, metadata, grp, supporting_context,
	source_date, task_type, dreaming, status, dreaming_status, reset_memories,
	file_name, mime_type, file_type, created_at, updated_at`

func (s *SqliteDB) GetDocument(ns, id string) (Document, bool, error) {
	row := s.db.QueryRow("SELECT "+documentColumns+" FROM documents WHERE namespace = ? AND id = ?", ns, id)
	d, err := scanDocument(row)
	if err == sql.ErrNoRows {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, err
	}
	return d, true, nil
}

// UpdateDocumentFields applies a whitelisted partial update and returns the
// refreshed row.
func (s *SqliteDB) UpdateDocumentFields(ns, id string, fields map[string]any) (Document, bool, error) {
	allowed := map[string]bool{
		"title": true, "summary": true, "content": true, "metadata": true, "grp": true,
		"supporting_context": true, "source_date": true, "task_type": true, "dreaming": true,
		"status": true, "dreaming_status": true, "reset_memories": true, "updated_at": true,
		"file_name": true, "mime_type": true, "file_type": true,
	}
	var sets []string
	var args []any
	for k, v := range fields {
		if !allowed[k] {
			continue
		}
		sets = append(sets, k+" = ?")
		if b, ok := v.(bool); ok {
			v = boolInt(b)
		}
		if m, ok := v.(map[string]any); ok {
			v = encodeMap(m)
		}
		args = append(args, v)
	}
	if len(sets) == 0 {
		return Document{}, false, nil
	}
	args = append(args, ns, id)
	res, err := s.db.Exec("UPDATE documents SET "+strings.Join(sets, ", ")+" WHERE namespace = ? AND id = ?", args...)
	if err != nil {
		return Document{}, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Document{}, false, nil
	}
	return s.GetDocument(ns, id)
}

func (s *SqliteDB) ClaimQueuedDocument() (Document, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Document{}, false, err
	}
	defer tx.Rollback()

	row := tx.QueryRow("SELECT " + documentColumns + " FROM documents WHERE status = 'queued' ORDER BY created_at ASC, id ASC LIMIT 1")
	d, err := scanDocument(row)
	if err == sql.ErrNoRows {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, err
	}
	res, err := tx.Exec("UPDATE documents SET status = ? WHERE namespace = ? AND id = ? AND status = ?",
		DocumentExtracting, d.Namespace, d.ID, DocumentQueued)
	if err != nil {
		return Document{}, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Document{}, false, nil
	}
	d.Status = DocumentExtracting
	return d, true, tx.Commit()
}

func (s *SqliteDB) UpdateDocumentStatus(ns, id, status, dreamingStatus string) error {
	_, err := s.db.Exec("UPDATE documents SET status = ?, dreaming_status = ?, updated_at = ? WHERE namespace = ? AND id = ?",
		status, dreamingStatus, time.Now().UTC(), ns, id)
	return err
}

// UpdateProcessingStatus advances a document that is still being processed.
// It reports false when a newer write re-queued the document in the meantime;
// the worker must then abort without overwriting the queued state.
func (s *SqliteDB) UpdateProcessingStatus(ns, id, status, dreamingStatus string) (bool, error) {
	res, err := s.db.Exec(`UPDATE documents SET status = ?, dreaming_status = ?, updated_at = ?
		WHERE namespace = ? AND id = ? AND status <> ?`,
		status, dreamingStatus, time.Now().UTC(), ns, id, DocumentQueued)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ClearResetMemories consumes the reset flag unless the document was
// re-queued concurrently.
func (s *SqliteDB) ClearResetMemories(ns, id string) (bool, error) {
	res, err := s.db.Exec("UPDATE documents SET reset_memories = 0 WHERE namespace = ? AND id = ? AND status <> ?",
		ns, id, DocumentQueued)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *SqliteDB) ReplaceChunks(ns, docID string, chunks []Chunk) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM chunks WHERE namespace = ? AND document_id = ?", ns, docID); err != nil {
		return err
	}
	for _, c := range chunks {
		if _, err := tx.Exec(`
			INSERT INTO chunks (id, namespace, document_id, position, content, type, metadata, vector, norm, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, ns, docID, c.Position, c.Content, c.Type, encodeMap(c.Metadata),
			encodeVector(c.Vector), vectorNorm(c.Vector), c.CreatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SqliteDB) DeleteMemoriesByDocument(ns, docID string) error {
	_, err := s.db.Exec("DELETE FROM memories WHERE container_tag = ? AND document_id = ?", ns, docID)
	return err
}

func (s *SqliteDB) InsertMemories(records []MemoryRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, m := range records {
		if m.Version == 0 {
			m.Version = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO memories (
				id, content, vector, container_tag, created_at, file_path, norm, synced_at,
				document_id, metadata, is_forgotten, is_inference, is_static, is_latest, version, updated_at
			) VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Content, encodeVector(m.Vector), m.Namespace, m.CreatedAt, vectorNorm(m.Vector), m.CreatedAt,
			m.DocumentID, encodeMap(m.Metadata), boolInt(m.IsForgotten), boolInt(m.IsInference),
			boolInt(m.IsStatic), boolInt(m.IsLatest), m.Version, m.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SqliteDB) DeleteDocumentRecords(ns string, ids []string) (int, []ItemError, error) {
	count := 0
	errs := make([]ItemError, 0)
	for _, id := range ids {
		tx, err := s.db.Begin()
		if err != nil {
			return count, errs, err
		}
		var status string
		err = tx.QueryRow("SELECT status FROM documents WHERE namespace = ? AND id = ?", ns, id).Scan(&status)
		if err == sql.ErrNoRows {
			tx.Rollback()
			errs = append(errs, ItemError{ID: id, Error: "Document not found"})
			continue
		}
		if err != nil {
			tx.Rollback()
			return count, errs, err
		}
		switch status {
		case DocumentQueued, DocumentExtracting, DocumentChunking, DocumentEmbedding, DocumentIndexing:
			tx.Rollback()
			errs = append(errs, ItemError{ID: id, Error: "Document is still processing"})
			continue
		}
		for _, q := range []string{
			"DELETE FROM chunks WHERE namespace = ? AND document_id = ?",
			"DELETE FROM memories WHERE container_tag = ? AND document_id = ?",
			"DELETE FROM documents WHERE namespace = ? AND id = ?",
		} {
			if _, err := tx.Exec(q, ns, id); err != nil {
				tx.Rollback()
				return count, errs, err
			}
		}
		if err := tx.Commit(); err != nil {
			return count, errs, err
		}
		count++
	}
	return count, errs, nil
}

const memoryColumns = `id, container_tag, document_id, content, metadata, vector,
	is_forgotten, is_inference, is_static, is_latest, version, created_at,
	COALESCE(NULLIF(updated_at, ''), created_at)`

func scanMemory(sc rowScanner) (MemoryRecord, error) {
	var m MemoryRecord
	var metadata string
	var vectorRaw []byte
	var forgotten, inference, static, latest int
	var created, updated any
	err := sc.Scan(
		&m.ID, &m.Namespace, &m.DocumentID, &m.Content, &metadata, &vectorRaw,
		&forgotten, &inference, &static, &latest, &m.Version, &created, &updated,
	)
	if err != nil {
		return MemoryRecord{}, err
	}
	m.Metadata = decodeMap(metadata)
	m.Vector, _ = decodeVector(vectorRaw)
	m.IsForgotten = forgotten != 0
	m.IsInference = inference != 0
	m.IsStatic = static != 0
	m.IsLatest = latest != 0
	m.CreatedAt = v5Time(created)
	m.UpdatedAt = v5Time(updated)
	return m, nil
}

// MemoryIDsWithFiles returns IDs of memories in a namespace that have a
// physical vault file, so callers can remove the files before deleting rows.
func (s *SqliteDB) MemoryIDsWithFiles(ns string) ([]string, error) {
	rows, err := s.db.Query("SELECT id FROM memories WHERE container_tag = ? AND file_path <> ''", ns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SqliteDB) GetMemory(ns, id string) (MemoryRecord, bool, error) {
	row := s.db.QueryRow("SELECT "+memoryColumns+" FROM memories WHERE container_tag = ? AND id = ?", ns, id)
	m, err := scanMemory(row)
	if err == sql.ErrNoRows {
		return MemoryRecord{}, false, nil
	}
	if err != nil {
		return MemoryRecord{}, false, err
	}
	return m, true, nil
}

func (s *SqliteDB) GetMemoriesByDocument(ns, docID string, limit int) ([]MemoryRecord, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query("SELECT "+memoryColumns+" FROM memories WHERE container_tag = ? AND document_id = ? ORDER BY created_at ASC LIMIT ?", ns, docID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryRecord
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SqliteDB) GetChunks(ns, docID string) ([]Chunk, error) {
	rows, err := s.db.Query(`
		SELECT id, namespace, document_id, content, type, position, metadata, vector, created_at
		FROM chunks WHERE namespace = ? AND document_id = ? ORDER BY position ASC`, ns, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		var metadata string
		var vectorRaw []byte
		var created any
		if err := rows.Scan(&c.ID, &c.Namespace, &c.DocumentID, &c.Content, &c.Type, &c.Position, &metadata, &vectorRaw, &created); err != nil {
			return nil, err
		}
		c.Metadata = decodeMap(metadata)
		c.Vector, _ = decodeVector(vectorRaw)
		c.CreatedAt = v5Time(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SqliteDB) MemoryCounts(ns string, ids []string) (map[string]int, error) {
	counts := make(map[string]int, len(ids))
	for _, id := range ids {
		counts[id] = 0
	}
	if len(ids) == 0 {
		return counts, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, ns)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(
		"SELECT document_id, COUNT(*) FROM memories WHERE container_tag = ? AND is_forgotten = 0 AND document_id IN ("+placeholders+") GROUP BY document_id",
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		counts[id] = n
	}
	return counts, rows.Err()
}

func matchesFilter(meta map[string]any, f *Filter) bool {
	if f == nil {
		return true
	}
	raw, ok := meta[f.Field]
	if !ok {
		return false
	}
	got := fmt.Sprint(raw)
	want := fmt.Sprint(f.Value)
	equal := got == want
	if !f.CaseSensitive {
		equal = strings.EqualFold(got, want)
	}
	if f.Operator == "neq" {
		return !equal
	}
	return equal
}

// ListV5Content returns one page of documents, chunks or memories for a
// namespace. Only the slice for kind is populated.
func (s *SqliteDB) ListV5Content(ns, kind string, q ListQuery) (docs []Document, chunks []Chunk, mems []MemoryRecord, total int, err error) {
	docs = []Document{}
	chunks = []Chunk{}
	mems = []MemoryRecord{}

	switch kind {
	case "documents":
		rows, err := s.db.Query("SELECT "+documentColumns+" FROM documents WHERE namespace = ?", ns)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDocument(rows)
			if err != nil {
				return nil, nil, nil, 0, err
			}
			if !matchesFilter(d.Metadata, q.Filter) {
				continue
			}
			docs = append(docs, d)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, nil, 0, err
		}
		sort.SliceStable(docs, func(i, j int) bool {
			var a, b time.Time
			if q.Sort == "updatedAt" {
				a, b = docs[i].UpdatedAt, docs[j].UpdatedAt
			} else {
				a, b = docs[i].CreatedAt, docs[j].CreatedAt
			}
			if a.Equal(b) {
				return docs[i].ID < docs[j].ID
			}
			if q.Order == "asc" {
				return a.Before(b)
			}
			return a.After(b)
		})
		total = len(docs)
		docs = pageSlice(docs, q.Page, q.Limit)

	case "chunks":
		rows, err := s.db.Query(`
			SELECT id, namespace, document_id, content, type, position, metadata, vector, created_at
			FROM chunks WHERE namespace = ?`, ns)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var c Chunk
			var metadata string
			var vectorRaw []byte
			var created any
			if err := rows.Scan(&c.ID, &c.Namespace, &c.DocumentID, &c.Content, &c.Type, &c.Position, &metadata, &vectorRaw, &created); err != nil {
				return nil, nil, nil, 0, err
			}
			c.Metadata = decodeMap(metadata)
			c.Vector, _ = decodeVector(vectorRaw)
			c.CreatedAt = v5Time(created)
			if !matchesFilter(c.Metadata, q.Filter) {
				continue
			}
			chunks = append(chunks, c)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, nil, 0, err
		}
		sort.SliceStable(chunks, func(i, j int) bool {
			if q.Sort == "position" {
				if chunks[i].Position == chunks[j].Position {
					return chunks[i].ID < chunks[j].ID
				}
				if q.Order == "asc" {
					return chunks[i].Position < chunks[j].Position
				}
				return chunks[i].Position > chunks[j].Position
			}
			a, b := chunks[i].CreatedAt, chunks[j].CreatedAt
			if a.Equal(b) {
				return chunks[i].ID < chunks[j].ID
			}
			if q.Order == "asc" {
				return a.Before(b)
			}
			return a.After(b)
		})
		total = len(chunks)
		chunks = pageSlice(chunks, q.Page, q.Limit)

	case "memories":
		where := "container_tag = ?"
		if !q.IncludeForgotten {
			where += " AND is_forgotten = 0"
		}
		rows, err := s.db.Query("SELECT "+memoryColumns+" FROM memories WHERE "+where, ns)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanMemory(rows)
			if err != nil {
				return nil, nil, nil, 0, err
			}
			if !matchesFilter(m.Metadata, q.Filter) {
				continue
			}
			mems = append(mems, m)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, nil, 0, err
		}
		sort.SliceStable(mems, func(i, j int) bool {
			var a, b time.Time
			if q.Sort == "updatedAt" {
				a, b = mems[i].UpdatedAt, mems[j].UpdatedAt
			} else {
				a, b = mems[i].CreatedAt, mems[j].CreatedAt
			}
			if a.Equal(b) {
				return mems[i].ID < mems[j].ID
			}
			if q.Order == "asc" {
				return a.Before(b)
			}
			return a.After(b)
		})
		total = len(mems)
		mems = pageSlice(mems, q.Page, q.Limit)

	default:
		return nil, nil, nil, 0, fmt.Errorf("unknown list type %q", kind)
	}
	return docs, chunks, mems, total, nil
}

func pageSlice[T any](in []T, page, limit int) []T {
	if page < 1 {
		page = 1
	}
	start := int64(page-1) * int64(limit)
	if start < 0 || start >= int64(len(in)) {
		return []T{}
	}
	end := start + int64(limit)
	if end > int64(len(in)) {
		end = int64(len(in))
	}
	return in[int(start):int(end)]
}

func clampSimilarity(sim float32) float32 {
	if sim < 0 {
		return 0
	}
	return sim
}

func cosine(query []float32, stored []float32, storedNorm float64) (float32, bool) {
	if len(stored) != len(query) {
		return 0, false
	}
	normQuery := vectorNorm(query)
	if normQuery <= 0 || storedNorm <= 0 {
		return 0, false
	}
	dot, err := vector.DotProduct(query, stored)
	if err != nil {
		return 0, false
	}
	return clampSimilarity(float32(float64(dot) / (normQuery * storedNorm))), true
}

// SearchV5 scans chunks and/or memories and returns the best hits.
func (s *SqliteDB) SearchV5(ns string, vec []float32, mode string, threshold float32, limit int, includeForgotten bool, filter *Filter) ([]Hit, error) {
	if limit <= 0 {
		return []Hit{}, nil
	}
	hits := make([]Hit, 0)

	if mode == "hybrid" || mode == "memories" {
		where := "container_tag = ?"
		if !includeForgotten {
			where += " AND is_forgotten = 0"
		}
		rows, err := s.db.Query("SELECT "+memoryColumns+", norm FROM memories WHERE "+where, ns)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var m MemoryRecord
			var metadata string
			var vectorRaw []byte
			var forgotten, inference, static, latest int
			var created, updated any
			var norm float64
			if err := rows.Scan(&m.ID, &m.Namespace, &m.DocumentID, &m.Content, &metadata, &vectorRaw,
				&forgotten, &inference, &static, &latest, &m.Version, &created, &updated, &norm); err != nil {
				rows.Close()
				return nil, err
			}
			m.Metadata = decodeMap(metadata)
			if !matchesFilter(m.Metadata, filter) {
				continue
			}
			sim, ok := cosine(vec, mustDecode(vectorRaw), norm)
			if !ok || sim < threshold {
				continue
			}
			hits = append(hits, Hit{
				Kind: "memory", ID: m.ID, Text: m.Content, DocumentID: m.DocumentID,
				Metadata: m.Metadata, Similarity: sim,
				IsLatest: latest != 0 && forgotten == 0, IsInference: inference != 0,
				CreatedAt: v5Time(created), UpdatedAt: v5Time(updated),
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	if mode == "hybrid" || mode == "chunks" {
		rows, err := s.db.Query(`
			SELECT id, document_id, content, metadata, vector, norm, created_at
			FROM chunks WHERE namespace = ?`, ns)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, docID, content, metadata string
			var vectorRaw []byte
			var norm float64
			var created any
			if err := rows.Scan(&id, &docID, &content, &metadata, &vectorRaw, &norm, &created); err != nil {
				rows.Close()
				return nil, err
			}
			meta := decodeMap(metadata)
			if !matchesFilter(meta, filter) {
				continue
			}
			sim, ok := cosine(vec, mustDecode(vectorRaw), norm)
			if !ok || sim < threshold {
				continue
			}
			t := v5Time(created)
			hits = append(hits, Hit{
				Kind: "chunk", ID: id, Text: content, DocumentID: docID,
				Metadata: meta, Similarity: sim,
				IsLatest: true, CreatedAt: t, UpdatedAt: t,
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Similarity == hits[j].Similarity {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Similarity > hits[j].Similarity
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func mustDecode(raw []byte) []float32 {
	v, _ := decodeVector(raw)
	return v
}

func (s *SqliteDB) ForgetMemories(ns string, ids []string) ([]MemoryHit, []ItemError, error) {
	matches := make([]MemoryHit, 0)
	errs := make([]ItemError, 0)
	for _, id := range ids {
		m, ok, err := s.GetMemory(ns, id)
		if err != nil {
			return matches, errs, err
		}
		if !ok || m.IsForgotten {
			errs = append(errs, ItemError{ID: id, Error: "Memory not found"})
			continue
		}
		if _, err := s.db.Exec("UPDATE memories SET is_forgotten = 1, updated_at = ? WHERE container_tag = ? AND id = ?",
			time.Now().UTC(), ns, id); err != nil {
			return matches, errs, err
		}
		m.IsForgotten = true
		matches = append(matches, MemoryHit{MemoryRecord: m})
	}
	return matches, errs, nil
}

func (s *SqliteDB) SemanticForget(ns string, vec []float32, max int, dryRun bool) ([]MemoryHit, error) {
	if max <= 0 {
		max = 100
	}
	rows, err := s.db.Query("SELECT "+memoryColumns+", norm FROM memories WHERE container_tag = ? AND is_forgotten = 0", ns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hits := make([]MemoryHit, 0)
	for rows.Next() {
		var m MemoryRecord
		var metadata string
		var vectorRaw []byte
		var forgotten, inference, static, latest int
		var created, updated any
		var norm float64
		if err := rows.Scan(&m.ID, &m.Namespace, &m.DocumentID, &m.Content, &metadata, &vectorRaw,
			&forgotten, &inference, &static, &latest, &m.Version, &created, &updated, &norm); err != nil {
			return nil, err
		}
		sim, ok := cosine(vec, mustDecode(vectorRaw), norm)
		if !ok || sim < 0.3 {
			continue
		}
		m.Metadata = decodeMap(metadata)
		m.CreatedAt = v5Time(created)
		m.UpdatedAt = v5Time(updated)
		hits = append(hits, MemoryHit{MemoryRecord: m, Similarity: sim})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Similarity == hits[j].Similarity {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Similarity > hits[j].Similarity
	})
	if len(hits) > max {
		hits = hits[:max]
	}
	if dryRun {
		return hits, nil
	}
	for _, h := range hits {
		if _, err := s.db.Exec("UPDATE memories SET is_forgotten = 1, updated_at = ? WHERE container_tag = ? AND id = ?",
			time.Now().UTC(), ns, h.ID); err != nil {
			return hits, err
		}
	}
	return hits, nil
}

func (s *SqliteDB) ListBuckets(ns string) (map[string]string, error) {
	out := make(map[string]string, len(DefaultBuckets))
	for k, v := range DefaultBuckets {
		out[k] = v
	}
	rows, err := s.db.Query("SELECT key, description FROM buckets WHERE namespace = ?", ns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// PutBuckets stores bucket descriptions. The bool reports whether the
// namespace exists.
func (s *SqliteDB) PutBuckets(ns string, buckets map[string]string) (map[string]string, bool, error) {
	if _, ok, err := s.GetNamespace(ns); err != nil {
		return nil, false, err
	} else if !ok {
		return nil, false, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	for k, v := range buckets {
		if _, err := tx.Exec("INSERT OR REPLACE INTO buckets (namespace, key, description) VALUES (?, ?, ?)", ns, k, v); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	effective, err := s.ListBuckets(ns)
	return effective, true, err
}

// DeleteBuckets removes bucket rows. The bool reports whether the namespace
// exists.
func (s *SqliteDB) DeleteBuckets(ns string, keys []string) (map[string]string, bool, error) {
	if _, ok, err := s.GetNamespace(ns); err != nil {
		return nil, false, err
	} else if !ok {
		return nil, false, nil
	}
	for _, k := range keys {
		if _, err := s.db.Exec("DELETE FROM buckets WHERE namespace = ? AND key = ?", ns, k); err != nil {
			return nil, true, err
		}
	}
	effective, err := s.ListBuckets(ns)
	return effective, true, err
}
