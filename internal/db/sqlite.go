package db

import (
	"database/sql"
	"encoding/json"
	"sort"
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
}

type SearchResult struct {
	Memory     Memory  `json:"memory"`
	Similarity float32 `json:"similarity"`
}

type SqliteDB struct {
	db *sql.DB
}

func NewSqliteDB(dbPath string) (*SqliteDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Create tables if they don't exist
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		vector TEXT NOT NULL,
		container_tag TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		file_path TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_memories_container ON memories(container_tag);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	// Check if file_path column exists in case the table was created under an older schema version
	rows, err := db.Query("PRAGMA table_info(memories)")
	if err != nil {
		db.Close()
		return nil, err
	}
	
	hasFilePath := false
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
		if name == "file_path" {
			hasFilePath = true
		}
	}
	rows.Close()

	if !hasFilePath {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN file_path TEXT NOT NULL DEFAULT ''"); err != nil {
			db.Close()
			return nil, err
		}
	}

	return &SqliteDB{db: db}, nil
}

func (s *SqliteDB) Close() error {
	return s.db.Close()
}

func (s *SqliteDB) SaveMemory(m Memory) error {
	vectorJSON, err := json.Marshal(m.Vector)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO memories (id, content, vector, container_tag, created_at, file_path)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		content = excluded.content,
		vector = excluded.vector,
		container_tag = excluded.container_tag,
		created_at = excluded.created_at,
		file_path = excluded.file_path;
	`
	_, err = s.db.Exec(query, m.ID, m.Content, string(vectorJSON), m.ContainerTag, m.CreatedAt, m.FilePath)
	return err
}

func (s *SqliteDB) DeleteMemory(id string) error {
	_, err := s.db.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

func (s *SqliteDB) ListMemories(containerTag string, limit int) ([]Memory, error) {
	rows, err := s.db.Query("SELECT id, content, vector, container_tag, created_at, file_path FROM memories WHERE container_tag = ? ORDER BY created_at DESC LIMIT ?", containerTag, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var vectorStr string
		if err := rows.Scan(&m.ID, &m.Content, &vectorStr, &m.ContainerTag, &m.CreatedAt, &m.FilePath); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(vectorStr), &m.Vector); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

func (s *SqliteDB) SearchMemories(containerTag string, queryVec []float32, threshold float32, limit int) ([]SearchResult, error) {
	rows, err := s.db.Query("SELECT id, content, vector, container_tag, created_at, file_path FROM memories WHERE container_tag = ?", containerTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var m Memory
		var vectorStr string
		if err := rows.Scan(&m.ID, &m.Content, &vectorStr, &m.ContainerTag, &m.CreatedAt, &m.FilePath); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(vectorStr), &m.Vector); err != nil {
			return nil, err
		}

		// Calculate similarity using our ultra-fast pure Go vector package
		sim, err := vector.CosineSimilarity(queryVec, m.Vector)
		if err != nil {
			return nil, err
		}

		if sim >= threshold {
			results = append(results, SearchResult{
				Memory:     m,
				Similarity: sim,
			})
		}
	}

	// Sort results in descending order of similarity
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	// Slice by limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetUnmigratedMemories retrieves all memories that have an empty file_path.
func (s *SqliteDB) GetUnmigratedMemories() ([]Memory, error) {
	rows, err := s.db.Query("SELECT id, content, container_tag, created_at FROM memories WHERE file_path = ''")
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
	return memories, nil
}

// ContainerTagSummary is one row returned by ListContainerTags.
type ContainerTagSummary struct {
	Name          string
	ContainerTag  string
	DocumentCount int
	MemoryCount   int
}

// MemorySummary is a lightweight view of one memory row for listing.
type MemorySummary struct {
	ID        string
	Content   string
	CreatedAt time.Time
}

// ListContainerTags returns every container tag present in the store, with counts.
func (s *SqliteDB) ListContainerTags() ([]ContainerTagSummary, error) {
	rows, err := s.db.Query(
		`SELECT container_tag, COUNT(*), COUNT(*) FROM memories
		 GROUP BY container_tag ORDER BY container_tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ContainerTagSummary{}
	for rows.Next() {
		var s ContainerTagSummary
		if err := rows.Scan(&s.Name, &s.DocumentCount, &s.MemoryCount); err != nil {
			return nil, err
		}
		s.ContainerTag = s.Name
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountMemories returns the total number of memories in the store.
func (s *SqliteDB) CountMemories() (int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// ListDocumentSummaries returns one page of memories as document summaries, newest first.
func (s *SqliteDB) ListDocumentSummaries(offset, limit int) ([]MemorySummary, error) {
	rows, err := s.db.Query(
		`SELECT id, content, created_at FROM memories
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MemorySummary, 0)
	for rows.Next() {
		var m MemorySummary
		if err := rows.Scan(&m.ID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *SqliteDB) UpdateFilePath(id, filePath string) error {
	_, err := s.db.Exec("UPDATE memories SET file_path = ? WHERE id = ?", filePath, id)
	return err
}

// GetAllMemories retrieves all memories from the database.
func (s *SqliteDB) GetAllMemories() ([]Memory, error) {
	rows, err := s.db.Query("SELECT id, content, vector, container_tag, created_at, file_path FROM memories")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var vectorStr string
		if err := rows.Scan(&m.ID, &m.Content, &vectorStr, &m.ContainerTag, &m.CreatedAt, &m.FilePath); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(vectorStr), &m.Vector); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}
