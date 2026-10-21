package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/supermemory-native/supermemory-native/internal/vector"
)

type Memory struct {
	ID           string    `json:"id"`
	Content      string    `json:"content"`
	Vector       []float32 `json:"vector"`
	ContainerTag string    `json:"container_tag"`
	CreatedAt    time.Time `json:"created_at"`
	FilePath     string    `json:"file_path"`
	SyncedAt     time.Time `json:"-"`
	Norm         float64   `json:"-"`
}

// MemoryRef is the lightweight projection used by vault sync: it carries only
// the fields needed to reconcile DB rows against physical files.
type MemoryRef struct {
	ID       string
	FilePath string
	SyncedAt time.Time
}

type SearchResult struct {
	Memory     Memory  `json:"memory"`
	Similarity float32 `json:"similarity"`
}

// ContainerTagStats represents aggregated statistics for a container tag.
type ContainerTagStats struct {
	Name          string `json:"name"`
	ContainerTag  string `json:"containerTag"`
	DocumentCount int    `json:"documentCount"`
	MemoryCount   int    `json:"memoryCount"`
	Emoji         string `json:"emoji"`
}

type SqliteDB struct {
	db *sql.DB
}

func NewSqliteDB(dbPath string) (*SqliteDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Durability + concurrency: WAL lets readers run during writes, and a busy
	// handler retries instead of surfacing SQLITE_BUSY to concurrent writers.
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL;").Scan(&mode); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set WAL journal mode: %w", err)
	}
	var busyTimeout int
	if err := db.QueryRow("PRAGMA busy_timeout=5000;").Scan(&busyTimeout); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Create tables if they don't exist
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		vector BLOB NOT NULL,
		container_tag TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		file_path TEXT NOT NULL DEFAULT '',
		norm REAL NOT NULL DEFAULT 0,
		synced_at DATETIME NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_memories_container ON memories(container_tag);
	CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at);

	CREATE TABLE IF NOT EXISTS namespaces (
		name TEXT PRIMARY KEY,
		supporting_context TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS documents (
		namespace TEXT NOT NULL,
		id TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		grp TEXT NOT NULL DEFAULT '{}',
		supporting_context TEXT NOT NULL DEFAULT '',
		source_date TEXT NOT NULL DEFAULT '',
		task_type TEXT NOT NULL DEFAULT 'memory',
		dreaming TEXT NOT NULL DEFAULT 'dynamic',
		status TEXT NOT NULL DEFAULT 'queued',
		dreaming_status TEXT NOT NULL DEFAULT 'queued',
		reset_memories INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (namespace, id)
	);
	CREATE INDEX IF NOT EXISTS idx_documents_ns_created ON documents(namespace, created_at);
	CREATE TABLE IF NOT EXISTS chunks (
		id TEXT PRIMARY KEY,
		namespace TEXT NOT NULL,
		document_id TEXT NOT NULL,
		position INTEGER NOT NULL,
		content TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'text',
		metadata TEXT NOT NULL DEFAULT '{}',
		vector BLOB NOT NULL,
		norm REAL NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_chunks_ns_doc ON chunks(namespace, document_id);
	CREATE TABLE IF NOT EXISTS buckets (
		namespace TEXT NOT NULL,
		key TEXT NOT NULL,
		description TEXT NOT NULL,
		PRIMARY KEY (namespace, key)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	// Detect columns introduced by newer schema versions on an existing table.
	rows, err := db.Query("PRAGMA table_info(memories)")
	if err != nil {
		db.Close()
		return nil, err
	}

	hasFilePath := false
	hasNorm := false
	hasSyncedAt := false
	newColumns := map[string]string{
		"document_id":  "TEXT NOT NULL DEFAULT ''",
		"metadata":     "TEXT NOT NULL DEFAULT '{}'",
		"is_forgotten": "INTEGER NOT NULL DEFAULT 0",
		"is_inference": "INTEGER NOT NULL DEFAULT 0",
		"is_static":    "INTEGER NOT NULL DEFAULT 0",
		"is_latest":    "INTEGER NOT NULL DEFAULT 1",
		"version":      "INTEGER NOT NULL DEFAULT 1",
		"updated_at":   "DATETIME NOT NULL DEFAULT ''",
	}
	for rows.Next() {
		var cid int
		var name, typeStr string
		var notnull, pk int
		var dfltVal sql.NullString
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err != nil {
			rows.Close()
			db.Close()
			return nil, err
		}
		switch name {
		case "file_path":
			hasFilePath = true
		case "norm":
			hasNorm = true
		case "synced_at":
			hasSyncedAt = true
		default:
			delete(newColumns, name)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		db.Close()
		return nil, err
	}

	if !hasFilePath {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN file_path TEXT NOT NULL DEFAULT ''"); err != nil {
			db.Close()
			return nil, err
		}
	}
	if !hasNorm {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN norm REAL NOT NULL DEFAULT 0"); err != nil {
			db.Close()
			return nil, err
		}
	}
	addedSyncedAt := false
	if !hasSyncedAt {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN synced_at DATETIME NOT NULL DEFAULT ''"); err != nil {
			db.Close()
			return nil, err
		}
		addedSyncedAt = true
	}

	// Rows that predate the synced_at column fall back to created_at, which
	// preserves the legacy modification-detection behavior exactly.
	if addedSyncedAt {
		if _, err := db.Exec("UPDATE memories SET synced_at = created_at WHERE synced_at = '' OR synced_at IS NULL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to backfill synced_at: %w", err)
		}
	}

	// v5 columns. Applied in sorted order so schema evolution is deterministic.
	pending := make([]string, 0, len(newColumns))
	for name := range newColumns {
		pending = append(pending, name)
	}
	sort.Strings(pending)
	for _, name := range pending {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN " + name + " " + newColumns[name]); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, ok := newColumns["updated_at"]; ok {
		if _, err := db.Exec("UPDATE memories SET updated_at = created_at WHERE updated_at = '' OR updated_at IS NULL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to backfill updated_at: %w", err)
		}
	}

	// One-time conversion of legacy JSON text vectors to BLOB storage. Rows
	// that fail to parse are left untouched; decodeVector rejects them safely.
	if err := migrateTextVectors(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SqliteDB{db: db}, nil
}

// migrateTextVectors converts every legacy JSON-encoded vector row to the
// binary BLOB representation and stores the precomputed L2 norm alongside it.
func migrateTextVectors(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT id, vector FROM memories WHERE typeof(vector) = 'text'")
	if err != nil {
		return err
	}

	type conversion struct {
		id     string
		vector []float32
	}
	var conversions []conversion
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var vec []float32
		if err := json.Unmarshal([]byte(raw), &vec); err != nil {
			continue // corrupt row: leave for decodeVector to reject at query time
		}
		conversions = append(conversions, conversion{id: id, vector: vec})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, c := range conversions {
		if _, err := tx.Exec(
			"UPDATE memories SET vector = ?, norm = ? WHERE id = ?",
			encodeVector(c.vector), vectorNorm(c.vector), c.id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SqliteDB) Close() error {
	return s.db.Close()
}

func (s *SqliteDB) SaveMemory(m Memory) error {
	vectorBlob := encodeVector(m.Vector)
	norm := vectorNorm(m.Vector)
	syncedAt := m.SyncedAt
	if syncedAt.IsZero() {
		// Rows written outside the sync path (legacy callers) treat their
		// creation time as the last known file state.
		syncedAt = m.CreatedAt
	}

	query := `
	INSERT INTO memories (id, content, vector, container_tag, created_at, file_path, norm, synced_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		content = excluded.content,
		vector = excluded.vector,
		container_tag = excluded.container_tag,
		created_at = excluded.created_at,
		file_path = excluded.file_path,
		norm = excluded.norm,
		synced_at = excluded.synced_at;
	`
	_, err := s.db.Exec(query, m.ID, m.Content, vectorBlob, m.ContainerTag, m.CreatedAt, m.FilePath, norm, syncedAt)
	return err
}

func (s *SqliteDB) DeleteMemory(id string) error {
	_, err := s.db.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

func (s *SqliteDB) ListMemories(containerTag string, limit int) ([]Memory, error) {
	rows, err := s.db.Query("SELECT id, content, container_tag, created_at, file_path FROM memories WHERE container_tag = ? ORDER BY created_at DESC LIMIT ?", containerTag, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.ContainerTag, &m.CreatedAt, &m.FilePath); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memories, nil
}

func (s *SqliteDB) SearchMemories(containerTag string, queryVec []float32, threshold float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		return []SearchResult{}, nil
	}
	rows, err := s.db.Query("SELECT id, content, vector, container_tag, created_at, file_path, norm FROM memories WHERE container_tag = ?", containerTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// The query norm is constant for the whole scan: compute it once instead
	// of once per row.
	normQuery := vectorNorm(queryVec)

	var results []SearchResult
	for rows.Next() {
		var m Memory
		var vectorRaw []byte
		var norm float64
		if err := rows.Scan(&m.ID, &m.Content, &vectorRaw, &m.ContainerTag, &m.CreatedAt, &m.FilePath, &norm); err != nil {
			return nil, err
		}

		vec, err := decodeVector(vectorRaw)
		if err != nil || len(vec) != len(queryVec) {
			// Corrupt or dimension-mismatched rows are skipped instead of
			// aborting the entire search.
			continue
		}
		if norm <= 0 {
			norm = vectorNorm(vec)
		}

		var sim float32
		if normQuery > 0 && norm > 0 {
			dot, err := vector.DotProduct(queryVec, vec)
			if err != nil {
				continue
			}
			sim = float32(float64(dot) / (normQuery * norm))
		}
		if sim < threshold {
			continue
		}

		insertTopK(&results, SearchResult{Memory: m, Similarity: sim}, limit)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// insertTopK inserts sr into results keeping the slice sorted by descending
// similarity and capped at limit entries, avoiding a full sort of every match.
func insertTopK(results *[]SearchResult, sr SearchResult, limit int) {
	res := *results
	pos := sort.Search(len(res), func(i int) bool { return res[i].Similarity < sr.Similarity })

	if len(res) < limit {
		res = append(res, SearchResult{})
		copy(res[pos+1:], res[pos:])
		res[pos] = sr
		*results = res
		return
	}
	if pos >= limit {
		return
	}
	copy(res[pos+1:], res[pos:len(res)-1])
	res[pos] = sr
}

// GetUnmigratedMemories retrieves all memories that have an empty file_path.
func (s *SqliteDB) GetUnmigratedMemories() ([]Memory, error) {
	rows, err := s.db.Query("SELECT id, content, container_tag, created_at FROM memories WHERE file_path = '' AND document_id = ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.ContainerTag, &m.CreatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memories, nil
}

// UpdateFilePath updates the file path pointer for a specific memory record.
func (s *SqliteDB) UpdateFilePath(id, filePath string) error {
	_, err := s.db.Exec("UPDATE memories SET file_path = ? WHERE id = ?", filePath, id)
	return err
}

// GetMemoryRefs retrieves the lightweight sync projection (id, file path,
// last-synced timestamp) for every memory without loading content or vectors.
func (s *SqliteDB) GetMemoryRefs() ([]MemoryRef, error) {
	rows, err := s.db.Query("SELECT id, file_path, synced_at FROM memories")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []MemoryRef
	for rows.Next() {
		var ref MemoryRef
		if err := rows.Scan(&ref.ID, &ref.FilePath, &ref.SyncedAt); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

// CountMemories returns the total count of memories in the database.
func (s *SqliteDB) CountMemories() (int, error) {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ListMemoriesPage retrieves a paginated slice of memories sorted by sortCol and order.
// sortCol and order are validated against a strict whitelist to prevent SQL injection.
// Defaults to created_at DESC if sortCol or order are invalid or empty.
func (s *SqliteDB) ListMemoriesPage(limit, offset int, sortCol, order string) ([]Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var col string
	switch strings.ToLower(strings.TrimSpace(sortCol)) {
	case "created_at", "createdat":
		col = "created_at"
	case "id":
		col = "id"
	case "content":
		col = "content"
	case "container_tag", "containertag":
		col = "container_tag"
	case "file_path", "filepath":
		col = "file_path"
	default:
		col = "created_at"
	}

	var dir string
	switch strings.ToUpper(strings.TrimSpace(order)) {
	case "ASC":
		dir = "ASC"
	case "DESC":
		dir = "DESC"
	default:
		dir = "DESC"
	}

	// The dashboard list view never reads embeddings: skip the vector column
	// entirely instead of decoding every row's vectors.
	query := fmt.Sprintf(
		"SELECT id, content, container_tag, created_at, file_path FROM memories ORDER BY %s %s LIMIT ? OFFSET ?",
		col, dir,
	)

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memories := make([]Memory, 0)
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.ContainerTag, &m.CreatedAt, &m.FilePath); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return memories, nil
}

// ListContainerTags returns aggregated container tag statistics grouped by container_tag.
// Defaults empty tag to "default", emoji to "📁", and returns non-nil empty slice.
func (s *SqliteDB) ListContainerTags() ([]ContainerTagStats, error) {
	query := `SELECT CASE WHEN TRIM(container_tag) = '' THEN 'default' ELSE container_tag END AS tag, COUNT(*) 
	          FROM memories 
	          GROUP BY tag 
	          ORDER BY tag ASC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make([]ContainerTagStats, 0)
	for rows.Next() {
		var tag string
		var count int
		if err := rows.Scan(&tag, &count); err != nil {
			return nil, err
		}
		if tag == "" {
			tag = "default"
		}
		stats = append(stats, ContainerTagStats{
			Name:          tag,
			ContainerTag:  tag,
			DocumentCount: count,
			MemoryCount:   count,
			Emoji:         "📁",
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}
