package memory

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

type Engine struct {
	DB       *db.SqliteDB
	Provider embedding.EmbeddingProvider
	Vault    *vault.Vault
}

func NewEngine(database *db.SqliteDB, provider embedding.EmbeddingProvider, v *vault.Vault) *Engine {
	return &Engine{
		DB:       database,
		Provider: provider,
		Vault:    v,
	}
}

// GenerateUUID v4 helper for standalone execution
func GenerateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (e *Engine) AddMemory(content, containerTag string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("content cannot be empty")
	}

	embeddings, err := e.Provider.GenerateEmbeddings([]string{content})
	if err != nil {
		return "", err
	}
	if len(embeddings) == 0 {
		return "", fmt.Errorf("failed to generate embeddings for content")
	}

	memID := GenerateUUID()
	var filePath string

	if e.Vault != nil {
		path, err := e.Vault.WriteMemory(memID, content, containerTag)
		if err != nil {
			return "", fmt.Errorf("failed to write memory to vault: %w", err)
		}
		filePath = path
	}

	mem := db.Memory{
		ID:           memID,
		Content:      content,
		Vector:       embeddings[0],
		ContainerTag: containerTag,
		CreatedAt:    time.Now(),
		FilePath:     filePath,
	}

	if err := e.DB.SaveMemory(mem); err != nil {
		return "", err
	}

	return memID, nil
}

func (e *Engine) QueryMemories(query, containerTag string, threshold float32, limit int) ([]db.SearchResult, error) {
	if query == "" {
		return []db.SearchResult{}, nil
	}

	embeddings, err := e.Provider.GenerateEmbeddings([]string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("failed to generate embeddings for query")
	}

	results, err := e.DB.SearchMemories(containerTag, embeddings[0], threshold, limit)
	if err != nil {
		return nil, err
	}

	// Hydrate content from physical vault files to ensure live disk edits are returned
	if e.Vault != nil {
		for i := range results {
			if results[i].Memory.FilePath != "" {
				fileContent, err := e.Vault.ReadMemory(results[i].Memory.FilePath)
				if err == nil {
					results[i].Memory.Content = fileContent
				}
			}
		}
	}

	return results, nil
}

// DeleteMemory removes a memory from both the SQLite index and the physical vault file.
func (e *Engine) DeleteMemory(id, containerTag string) error {
	if err := e.DB.DeleteMemory(id); err != nil {
		return err
	}
	if e.Vault != nil {
		_ = e.Vault.DeleteMemory(id)
	}
	return nil
}

// SyncVault performs a full bi-directional sync between the physical vault directory and the SQLite database index.
func (e *Engine) SyncVault() error {
	if e.Vault == nil {
		return fmt.Errorf("vault is not configured on engine")
	}

	// 1. List physical files on disk
	diskIds, err := e.Vault.ListPhysicalFiles()
	if err != nil {
		return fmt.Errorf("failed to list files during sync: %w", err)
	}

	diskIDsMap := make(map[string]bool)
	for _, id := range diskIds {
		diskIDsMap[id] = true
	}

	// 2. Retrieve all records from DB
	dbMemories, err := e.DB.GetAllMemories()
	if err != nil {
		return fmt.Errorf("failed to retrieve DB records during sync: %w", err)
	}

	dbMemoriesMap := make(map[string]db.Memory)
	for _, m := range dbMemories {
		dbMemoriesMap[m.ID] = m
	}

	// 3. Purge DB records that no longer exist physically on disk
	for _, m := range dbMemories {
		if m.FilePath != "" && !diskIDsMap[m.ID] {
			if err := e.DB.DeleteMemory(m.ID); err != nil {
				return fmt.Errorf("failed to purge orphaned memory %s: %w", m.ID, err)
			}
		}
	}

	// 4. Process new and modified files on disk
	for _, id := range diskIds {
		filePath := filepath.Join(e.Vault.Dir, id+".okf")
		info, err := os.Stat(filePath)
		if err != nil {
			continue // skip if file info error
		}

		dbMem, exists := dbMemoriesMap[id]
		isModified := exists && info.ModTime().After(dbMem.CreatedAt)

		if !exists || isModified {
			// Read and parse physical OKF content
			rawContent, err := e.Vault.ReadMemory(id)
			if err != nil {
				continue
			}

			doc, err := vault.ParseOKF(rawContent)
			if err != nil {
				continue
			}

			// Extract container tag or default to "default"
			containerTag := "default"
			if val, ok := doc.Frontmatter["container_tag"].(string); ok && val != "" {
				containerTag = val
			}

			// Generate embedding from the entire OKF document (including frontmatter metadata)
			// to allow semantic search over YAML fields!
			embeddings, err := e.Provider.GenerateEmbeddings([]string{rawContent})
			if err != nil {
				return fmt.Errorf("failed to generate embedding for synced file %s: %w", id, err)
			}

			if len(embeddings) > 0 {
				mem := db.Memory{
					ID:           id,
					Content:      doc.Body,
					Vector:       embeddings[0],
					ContainerTag: containerTag,
					CreatedAt:    info.ModTime(),
					FilePath:     filePath,
				}
				if err := e.DB.SaveMemory(mem); err != nil {
					return fmt.Errorf("failed to save synced memory %s: %w", id, err)
				}
			}
		}
	}

	return nil
}

// ContainerTagStat is a single row in the dashboard's tag filter list.
type ContainerTagStat struct {
	Name          string
	ContainerTag  string
	DocumentCount int
	MemoryCount   int
}

// ListContainerTags returns every container tag present in the store, with counts.
// The official dashboard uses this to populate its tag filter chips.
func (e *Engine) ListContainerTags() ([]ContainerTagStat, error) {
	stats, err := e.DB.ListContainerTags()
	if err != nil {
		return nil, err
	}

	out := make([]ContainerTagStat, 0, len(stats))
	for _, s := range stats {
		out = append(out, ContainerTagStat{
			Name:          s.Name,
			ContainerTag:  s.ContainerTag,
			DocumentCount: s.DocumentCount,
			MemoryCount:   s.MemoryCount,
		})
	}
	return out, nil
}

// DocumentSummary is one row in the dashboard's document table.
type DocumentSummary struct {
	ID            string             `json:"id"`
	Title         string             `json:"title"`
	Summary       string             `json:"summary"`
	Status        string             `json:"status"`
	CreatedAt     string             `json:"createdAt"`
	UpdatedAt     string             `json:"updatedAt"`
	Memories      []string           `json:"memories"`
	MemoryEntries []MemoryEntry      `json:"memoryEntries"`
}

// MemoryEntry is what the official dashboard's MemoryGraph expects inside each document.
type MemoryEntry struct {
	ID        string `json:"id"`
	Memory    string `json:"memory"`
	CreatedAt string `json:"createdAt"`
	IsStatic  bool   `json:"isStatic"`
}

// Page is a generic pagination envelope for the dashboard's list endpoint.
type Page[T any] struct {
	Items       []T
	TotalItems  int
	TotalPages  int
	CurrentPage int
}

// ListDocuments returns one page of memories rendered as documents, newest first.
// supermemory-native stores memories directly (no separate document layer), so each
// memory is presented as a single-document record. This is what the official dashboard's
// document table consumes via POST /v3/documents/documents.
func (e *Engine) ListDocuments(page, limit int) (Page[DocumentSummary], error) {
	if limit <= 0 {
		limit = 25
	}
	if page <= 0 {
		page = 1
	}

	total, err := e.DB.CountMemories()
	if err != nil {
		return Page[DocumentSummary]{}, err
	}

	offset := (page - 1) * limit
	rows, err := e.DB.ListDocumentSummaries(offset, limit)
	if err != nil {
		return Page[DocumentSummary]{}, err
	}

	items := []DocumentSummary{}
	for _, d := range rows {
		ts := d.CreatedAt.UTC().Format(time.RFC3339)
		items = append(items, DocumentSummary{
			ID:        d.ID,
			Title:     firstLine(d.Content, 80),
			Summary:   truncate(d.Content, 240),
			Status:    "done",
			CreatedAt: ts,
			UpdatedAt: ts,
			Memories:  []string{d.Content},
			MemoryEntries: []MemoryEntry{{
				ID:        d.ID,
				Memory:    d.Content,
				CreatedAt: ts,
				IsStatic:  false,
			}},
		})
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}
	return Page[DocumentSummary]{
		Items: items, TotalItems: total,
		TotalPages: totalPages, CurrentPage: page,
	}, nil
}

func firstLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 && i < max {
		return s[:i]
	}
	return truncate(s, max)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
