package memory

import (
	"crypto/rand"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

type Engine struct {
	DB       *db.SqliteDB
	Provider embedding.EmbeddingProvider
	Vault    *vault.Vault

	// mu serializes state-mutating operations (add/delete/sync) so a running
	// sync can never purge or resurrect rows racing against API writes.
	mu sync.Mutex
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

	// The embedding call is a slow network round-trip that needs no shared
	// state: run it before taking the engine lock.
	embeddings, err := e.Provider.GenerateEmbeddings([]string{content})
	if err != nil {
		return "", err
	}
	if len(embeddings) == 0 {
		return "", fmt.Errorf("failed to generate embeddings for content")
	}

	memID := GenerateUUID()

	e.mu.Lock()
	defer e.mu.Unlock()

	var filePath string
	var syncedAt time.Time
	if e.Vault != nil {
		path, err := e.Vault.WriteMemory(memID, content, containerTag)
		if err != nil {
			return "", fmt.Errorf("failed to write memory to vault: %w", err)
		}
		filePath = path
		if fi, err := os.Stat(path); err == nil {
			syncedAt = fi.ModTime()
		}
	}

	mem := db.Memory{
		ID:           memID,
		Content:      content,
		Vector:       embeddings[0],
		ContainerTag: containerTag,
		CreatedAt:    time.Now(),
		FilePath:     filePath,
		SyncedAt:     syncedAt,
	}

	if err := e.DB.SaveMemory(mem); err != nil {
		// Roll back the physical file: an orphan left on disk would be
		// resurrected as a phantom memory by the next SyncVault run.
		if e.Vault != nil {
			_ = e.Vault.DeleteMemory(memID)
		}
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
	e.mu.Lock()
	defer e.mu.Unlock()

	// Remove the physical file first and never swallow its error: if the DB
	// delete then fails, the next sync purges the orphaned row — whereas the
	// reverse order would let sync resurrect a memory that was just deleted.
	if e.Vault != nil {
		if err := e.Vault.DeleteMemory(id); err != nil {
			return fmt.Errorf("failed to delete vault file: %w", err)
		}
	}
	if err := e.DB.DeleteMemory(id); err != nil {
		return fmt.Errorf("failed to delete memory index: %w", err)
	}
	return nil
}

// syncTask is one new or modified vault file awaiting re-indexing.
type syncTask struct {
	id           string
	filePath     string
	rawContent   string
	containerTag string
	body         string
	modTime      time.Time
}

// SyncVault performs a full bi-directional sync between the physical vault directory and the SQLite database index.
func (e *Engine) SyncVault() error {
	if e.Vault == nil {
		return fmt.Errorf("vault is not configured on engine")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Snapshot the DB first, then list the disk. A memory added between the
	//    two snapshots appears on disk *after* the DB read, so it can never be
	//    purged as an orphan — at worst it is re-imported, which is an
	//    idempotent upsert.
	refs, err := e.DB.GetMemoryRefs()
	if err != nil {
		return fmt.Errorf("failed to retrieve DB records during sync: %w", err)
	}

	diskIds, err := e.Vault.ListPhysicalFiles()
	if err != nil {
		return fmt.Errorf("failed to list files during sync: %w", err)
	}

	diskIDsMap := make(map[string]bool, len(diskIds))
	for _, id := range diskIds {
		diskIDsMap[id] = true
	}

	refsByID := make(map[string]db.MemoryRef, len(refs))
	for _, ref := range refs {
		refsByID[ref.ID] = ref
	}

	// 2. Purge DB records whose physical file is gone
	for _, ref := range refs {
		if ref.FilePath != "" && !diskIDsMap[ref.ID] {
			if err := e.DB.DeleteMemory(ref.ID); err != nil {
				return fmt.Errorf("failed to purge orphaned memory %s: %w", ref.ID, err)
			}
		}
	}

	// 3. Collect new and modified files on disk. Modification is detected
	//    against the file mtime recorded at the previous sync (SyncedAt), not
	//    against CreatedAt, so edits within the creation clock tick are not
	//    silently missed.
	var tasks []syncTask
	for _, id := range diskIds {
		filePath, err := e.Vault.PathForID(id)
		if err != nil {
			continue // unsafe file name: never resolves to a vault path
		}
		info, err := os.Stat(filePath)
		if err != nil {
			continue // skip if file info error
		}

		ref, exists := refsByID[id]
		if exists && !info.ModTime().After(ref.SyncedAt) {
			continue
		}

		rawContent, err := e.Vault.ReadMemory(id)
		if err != nil {
			continue
		}

		doc, err := vault.ParseOKF(rawContent)
		if err != nil {
			continue
		}

		containerTag := "default"
		if val, ok := doc.Frontmatter["container_tag"].(string); ok && val != "" {
			containerTag = val
		}

		tasks = append(tasks, syncTask{
			id:           id,
			filePath:     filePath,
			rawContent:   rawContent,
			containerTag: containerTag,
			body:         doc.Body,
			modTime:      info.ModTime(),
		})
	}

	if len(tasks) == 0 {
		return nil
	}

	// 4. Generate embeddings in a bounded worker pool: embedding calls are
	//    network round-trips and dominate sync time. Failures abort the sync
	//    (the next run retries), matching the previous fail-fast behavior.
	embeds := make([][]float32, len(tasks))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	workers := min(4, len(tasks))
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				emb, err := e.Provider.GenerateEmbeddings([]string{tasks[i].rawContent})
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("failed to generate embedding for synced file %s: %w", tasks[i].id, err)
					}
					errMu.Unlock()
					continue
				}
				if len(emb) > 0 {
					embeds[i] = emb[0]
				}
			}
		}()
	}
	for i := range tasks {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	// 5. Persist sequentially to keep SQLite transactions short and ordered.
	for i, t := range tasks {
		if len(embeds[i]) == 0 {
			continue
		}
		mem := db.Memory{
			ID:           t.id,
			Content:      t.body,
			Vector:       embeds[i],
			ContainerTag: t.containerTag,
			CreatedAt:    t.modTime,
			FilePath:     t.filePath,
			SyncedAt:     t.modTime,
		}
		if err := e.DB.SaveMemory(mem); err != nil {
			return fmt.Errorf("failed to save synced memory %s: %w", t.id, err)
		}
	}

	return nil
}

// CountMemories returns the total count of memories across the database.
func (e *Engine) CountMemories() (int, error) {
	if e.DB == nil {
		return 0, fmt.Errorf("database is not configured on engine")
	}
	return e.DB.CountMemories()
}

// ListContainerTags returns aggregated container tag statistics grouped by container tag.
func (e *Engine) ListContainerTags() ([]db.ContainerTagStats, error) {
	if e.DB == nil {
		return make([]db.ContainerTagStats, 0), fmt.Errorf("database is not configured on engine")
	}
	tags, err := e.DB.ListContainerTags()
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = make([]db.ContainerTagStats, 0)
	}
	return tags, nil
}

// ListMemoriesPage retrieves a paginated slice of memories with sorting, clamping,
// total item count, and live vault physical file hydration.
func (e *Engine) ListMemoriesPage(page, limit int, sortCol, order string) ([]db.Memory, int, error) {
	if e.DB == nil {
		return make([]db.Memory, 0), 0, fmt.Errorf("database is not configured on engine")
	}

	// 1. Clamp page and limit bounds
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := (page - 1) * limit

	// 2. Fetch total count across memories
	totalCount, err := e.DB.CountMemories()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count memories: %w", err)
	}

	if totalCount == 0 {
		return make([]db.Memory, 0), 0, nil
	}

	// 3. Retrieve paginated records from database
	memories, err := e.DB.ListMemoriesPage(limit, offset, sortCol, order)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list memories page: %w", err)
	}
	if memories == nil {
		memories = make([]db.Memory, 0)
	}

	// 4. Hydrate content from physical vault files so live disk edits are reflected
	if e.Vault != nil {
		for i := range memories {
			if memories[i].FilePath != "" {
				fileContent, err := e.Vault.ReadMemory(memories[i].FilePath)
				if err == nil {
					memories[i].Content = fileContent
				}
			}
		}
	}

	return memories, totalCount, nil
}
