package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

func TestEngine(t *testing.T) {
	// Initialize in-memory DB
	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	// Initialize temp Vault
	tempVaultDir, err := os.MkdirTemp("", "supermemory_engine_vault_test_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	// Initialize mock provider
	prov := embedding.NewMockProvider()

	// Initialize engine with Vault
	eng := NewEngine(sdb, prov, v)

	// 1. Add memory
	content := "Alice prefers Node.js for quick MCP bridges."
	id1, err := eng.AddMemory(content, "user_alice")
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}
	if id1 == "" {
		t.Error("expected non-empty memory ID")
	}

	id2, err := eng.AddMemory("Rust has zero-cost abstractions.", "user_alice")
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}
	if id2 == "" {
		t.Error("expected non-empty memory ID")
	}

	// Verify that physical OKF files were created in the vault directory
	expectedFilePath1 := filepath.Join(tempVaultDir, id1+".okf")
	if _, err := os.Stat(expectedFilePath1); os.IsNotExist(err) {
		t.Errorf("expected physical OKF file to exist at %s, but it does not", expectedFilePath1)
	}

	// Boundary check: empty content
	_, err = eng.AddMemory("", "user_alice")
	if err == nil {
		t.Error("expected error for empty content, got nil")
	}

	// 2. Query memories and verify hydration from physical file
	results, err := eng.QueryMemories("What language does Alice prefer?", "user_alice", 0.1, 10)
	if err != nil {
		t.Fatalf("failed to query memories: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected at least 1 query result, got 0")
	}

	// Verify the result is loaded and contains the physical file's formatted OKF representation
	foundAlice := false
	for _, res := range results {
		if res.Memory.ID == id1 {
			foundAlice = true
			if !strings.Contains(res.Memory.Content, "type: memory") {
				t.Errorf("expected queried memory content to be hydrated OKF, got: %q", res.Memory.Content)
			}
			if !strings.Contains(res.Memory.Content, content) {
				t.Errorf("expected queried memory body to contain %q, got %q", content, res.Memory.Content)
			}
		}
	}
	if !foundAlice {
		t.Error("expected to find memory 1 in search results")
	}

	// 3. VERIFY PHYSICAL FILE IS SOURCE OF TRUTH: edit the file directly on disk and query again!
	editedContent := `---
type: memory
title: Edited Memory
container_tag: user_alice
custom_field: hello
---
Alice prefers Go for high-concurrency pipelines.`

	err = os.WriteFile(expectedFilePath1, []byte(editedContent), 0644)
	if err != nil {
		t.Fatalf("failed to edit file directly on disk: %v", err)
	}

	// Query again and check if it returned the edited content from disk!
	results2, err := eng.QueryMemories("What language does Alice prefer?", "user_alice", 0.1, 10)
	if err != nil {
		t.Fatalf("failed to query memories: %v", err)
	}

	foundEdited := false
	for _, res := range results2 {
		if res.Memory.ID == id1 {
			foundEdited = true
			if !strings.Contains(res.Memory.Content, "Alice prefers Go for high-concurrency pipelines.") {
				t.Errorf("expected to get edited content from disk, but got: %q", res.Memory.Content)
			}
			if !strings.Contains(res.Memory.Content, "custom_field: hello") {
				t.Errorf("expected to get custom YAML field, but got: %q", res.Memory.Content)
			}
		}
	}
	if !foundEdited {
		t.Error("expected to find edited memory in results")
	}

	// Boundary check: empty query
	emptyResults, err := eng.QueryMemories("", "user_alice", 0.1, 10)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if len(emptyResults) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(emptyResults))
	}

	// 4. Provider failure modes (for 100% code coverage)
	prov.Fail = true
	_, err = eng.AddMemory("broken", "user_alice")
	if err == nil {
		t.Error("expected error during provider failure on add, got nil")
	}

	_, err = eng.QueryMemories("broken", "user_alice", 0.1, 10)
	if err == nil {
		t.Error("expected error during provider failure on query, got nil")
	}
}

func TestEngineSyncAndMigration(t *testing.T) {
	// Initialize in-memory DB
	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	// Initialize temp Vault
	tempVaultDir, err := os.MkdirTemp("", "supermemory_sync_vault_test_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	prov := embedding.NewMockProvider()
	eng := NewEngine(sdb, prov, v)

	// A. TEST AUTOMATIC MIGRATION
	// Create a legacy unmigrated record in DB (with file_path = "")
	legacyID := "legacy-123"
	legacyContent := "This is a legacy database memory from older schema version."
	legacyMemory := db.Memory{
		ID:           legacyID,
		Content:      legacyContent,
		Vector:       []float32{0.5, 0.5, 0.5},
		ContainerTag: "user_legacy",
		CreatedAt:    time.Now().Add(-24 * time.Hour), // 1 day ago
		FilePath:     "",                              // empty signals legacy unmigrated
	}

	if err := sdb.SaveMemory(legacyMemory); err != nil {
		t.Fatalf("failed to save legacy memory: %v", err)
	}

	// Run automatic migration on main daemon startup simulation
	unmigrated, err := sdb.GetUnmigratedMemories()
	if err != nil {
		t.Fatalf("failed to get unmigrated: %v", err)
	}
	if len(unmigrated) != 1 {
		t.Errorf("expected 1 unmigrated memory, got %d", len(unmigrated))
	}

	// Migrate unmigrated records
	for _, m := range unmigrated {
		filePath, err := v.WriteMemoryWithTime(m.ID, m.Content, m.ContainerTag, m.CreatedAt)
		if err != nil {
			t.Fatalf("failed to write memory to vault: %v", err)
		}
		if err := sdb.UpdateFilePath(m.ID, filePath); err != nil {
			t.Fatalf("failed to update file path: %v", err)
		}
	}

	// Assert SQLite file_path is updated
	list, err := sdb.ListMemories("user_legacy", 10)
	if err != nil {
		t.Fatalf("failed to list memories: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 memory in user_legacy, got %d", len(list))
	}
	expectedMigratedPath := filepath.Join(tempVaultDir, legacyID+".okf")
	if list[0].FilePath != expectedMigratedPath {
		t.Errorf("expected migrated path %q, got %q", expectedMigratedPath, list[0].FilePath)
	}

	// Assert physical file exists and contains frontmatter
	bytes, err := os.ReadFile(expectedMigratedPath)
	if err != nil {
		t.Fatalf("failed to read migrated file: %v", err)
	}
	migratedContent := string(bytes)
	if !strings.Contains(migratedContent, "container_tag: user_legacy") || !strings.Contains(migratedContent, legacyContent) {
		t.Errorf("migrated file content looks wrong: %q", migratedContent)
	}

	// B. TEST BI-DIRECTIONAL SYNC
	// 1. Simulate new physical file created on disk (e.g. from Obsidian / Git pull)
	newDiskID := "obsidian-note-999"
	newDiskContent := `---
type: obsidian
container_tag: user_obsidian
tags:
  - sync
  - live
---
This is a physical note added directly into the Obsidian vault directory.`

	newDiskPath := filepath.Join(tempVaultDir, newDiskID+".okf")
	if err := os.WriteFile(newDiskPath, []byte(newDiskContent), 0644); err != nil {
		t.Fatalf("failed to write obsidian note: %v", err)
	}

	// 2. Simulate physical file deletion from disk
	err = v.DeleteMemory(legacyID)
	if err != nil {
		t.Fatalf("failed to delete legacy note: %v", err)
	}

	// 3. Trigger SyncVault()
	if err := eng.SyncVault(); err != nil {
		t.Fatalf("SyncVault failed: %v", err)
	}

	// 4. Assert Deleted File index was purged from DB
	legacyList, err := sdb.ListMemories("user_legacy", 10)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(legacyList) != 0 {
		t.Errorf("expected legacy memory index to be purged, but found: %v", legacyList)
	}

	// 5. Assert New Note index was parsed and added to DB
	obsidianList, err := sdb.ListMemories("user_obsidian", 10)
	if err != nil {
		t.Fatalf("failed to list obsidian container: %v", err)
	}
	if len(obsidianList) != 1 {
		t.Fatalf("expected 1 record in user_obsidian container, got %d", len(obsidianList))
	}
	if obsidianList[0].ID != newDiskID {
		t.Errorf("expected ID %q, got %q", newDiskID, obsidianList[0].ID)
	}
	if obsidianList[0].FilePath != newDiskPath {
		t.Errorf("expected FilePath %q, got %q", newDiskPath, obsidianList[0].FilePath)
	}
	if !strings.Contains(obsidianList[0].Content, "This is a physical note added") {
		t.Errorf("expected note body parsed correctly, got %q", obsidianList[0].Content)
	}
}

func TestEngineVaultHydration(t *testing.T) {
	// Initialize in-memory DB
	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	// Initialize temp Vault directory
	tempVaultDir, err := os.MkdirTemp("", "supermemory_engine_hydration_test_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	prov := embedding.NewMockProvider()
	eng := NewEngine(sdb, prov, v)

	// 1. Add Memory through engine (creates DB record and physical OKF file)
	origContent := "Initial database content before live modification."
	id1, err := eng.AddMemory(origContent, "user_alice")
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}

	expectedFilePath := filepath.Join(tempVaultDir, id1+".okf")
	if _, err := os.Stat(expectedFilePath); os.IsNotExist(err) {
		t.Fatalf("expected physical file at %s", expectedFilePath)
	}

	// 2. Modify the physical OKF file directly on disk
	diskContent := `---
type: memory
title: Mutated On Disk
container_tag: user_alice
---
Live content directly updated in Obsidian vault.`

	if err := os.WriteFile(expectedFilePath, []byte(diskContent), 0644); err != nil {
		t.Fatalf("failed to write live update to disk: %v", err)
	}

	// 3. Query paginated memories through Engine and verify vault content hydration
	mems, total, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil {
		t.Fatalf("failed to list memories page: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total count 1, got %d", total)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory returned, got %d", len(mems))
	}

	// Content must reflect live disk content rather than stale DB content
	if !strings.Contains(mems[0].Content, "Live content directly updated in Obsidian vault.") {
		t.Errorf("expected memory content to be hydrated from disk, got: %q", mems[0].Content)
	}
	if mems[0].Content == origContent {
		t.Errorf("memory content was NOT hydrated; still has stale DB content: %q", mems[0].Content)
	}

	// 4. Memory with empty FilePath remains unmutated (legacy DB fallback)
	legacyMem := db.Memory{
		ID:           "legacy_no_file",
		Content:      "Pure database content without file backing",
		Vector:       []float32{0.1, 0.2},
		ContainerTag: "legacy",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		FilePath:     "", // empty FilePath
	}
	if err := sdb.SaveMemory(legacyMem); err != nil {
		t.Fatalf("failed to save legacy memory: %v", err)
	}

	memsPage, total, err := eng.ListMemoriesPage(1, 10, "created_at", "asc")
	if err != nil || total != 2 {
		t.Fatalf("expected total 2, got %d (err: %v)", total, err)
	}

	foundLegacy := false
	for _, m := range memsPage {
		if m.ID == "legacy_no_file" {
			foundLegacy = true
			if m.Content != "Pure database content without file backing" {
				t.Errorf("expected legacy content to remain intact, got: %q", m.Content)
			}
		}
	}
	if !foundLegacy {
		t.Error("legacy memory not found in page")
	}

	// 5. Missing disk file does not cause error (graceful fallback)
	missingFileMem := db.Memory{
		ID:           "missing_file_mem",
		Content:      "Fallback DB content when file is gone",
		Vector:       []float32{0.2, 0.3},
		ContainerTag: "ghost",
		CreatedAt:    time.Now().Add(1 * time.Hour),
		FilePath:     filepath.Join(tempVaultDir, "nonexistent.okf"),
	}
	if err := sdb.SaveMemory(missingFileMem); err != nil {
		t.Fatalf("failed to save missing file memory: %v", err)
	}

	pageWithGhost, total, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil {
		t.Fatalf("expected graceful fallback on missing file, got error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	foundGhost := false
	for _, m := range pageWithGhost {
		if m.ID == "missing_file_mem" {
			foundGhost = true
			if m.Content != "Fallback DB content when file is gone" {
				t.Errorf("expected fallback content when file missing, got: %q", m.Content)
			}
		}
	}
	if !foundGhost {
		t.Error("missing file memory record not returned")
	}

	// 6. Engine with nil Vault does not panic and returns DB content cleanly
	nilVaultEng := NewEngine(sdb, prov, nil)
	nilVaultPage, nilTotal, err := nilVaultEng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil || nilTotal != 3 || len(nilVaultPage) != 3 {
		t.Fatalf("nil vault engine failed: total=%d, len=%d, err=%v", nilTotal, len(nilVaultPage), err)
	}

	// 7. Engine pagination and count metadata validation
	p1, tot, err := eng.ListMemoriesPage(1, 2, "created_at", "asc")
	if err != nil || tot != 3 || len(p1) != 2 {
		t.Errorf("page 1 pagination error: total=%d, len=%d, err=%v", tot, len(p1), err)
	}
	p2, tot2, err := eng.ListMemoriesPage(2, 2, "created_at", "asc")
	if err != nil || tot2 != 3 || len(p2) != 1 {
		t.Errorf("page 2 pagination error: total=%d, len=%d, err=%v", tot2, len(p2), err)
	}
}

func TestEngineQueriesAndHydration(t *testing.T) {
	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	tempVaultDir, err := os.MkdirTemp("", "supermemory_engine_m1_test_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	prov := embedding.NewMockProvider()
	eng := NewEngine(sdb, prov, v)

	// 1. Empty database verification
	count, err := eng.CountMemories()
	if err != nil || count != 0 {
		t.Fatalf("expected count 0, got %d, err %v", count, err)
	}

	tags, err := eng.ListContainerTags()
	if err != nil || len(tags) != 0 {
		t.Fatalf("expected 0 tags, got %d, err %v", len(tags), err)
	}

	p0, total, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil || total != 0 || len(p0) != 0 {
		t.Fatalf("expected 0 memories, got %d (total %d), err %v", len(p0), total, err)
	}

	// 2. Add memories across multiple tags
	id1, _ := eng.AddMemory("Fact 1 for alpha", "alpha")
	_, _ = eng.AddMemory("Fact 2 for alpha", "alpha")
	_, _ = eng.AddMemory("Fact 3 for beta", "beta")

	// 3. CountMemories verification
	count, err = eng.CountMemories()
	if err != nil || count != 3 {
		t.Fatalf("expected count 3, got %d, err %v", count, err)
	}

	// 4. ListContainerTags verification
	tags, err = eng.ListContainerTags()
	if err != nil || len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d, err %v", len(tags), err)
	}
	if tags[0].ContainerTag != "alpha" || tags[0].MemoryCount != 2 {
		t.Errorf("unexpected tag 0: %+v", tags[0])
	}
	if tags[1].ContainerTag != "beta" || tags[1].MemoryCount != 1 {
		t.Errorf("unexpected tag 1: %+v", tags[1])
	}

	// 5. ListMemoriesPage pagination verification
	page1, total, err := eng.ListMemoriesPage(1, 2, "created_at", "desc")
	if err != nil || total != 3 || len(page1) != 2 {
		t.Fatalf("page 1 failed: len=%d, total=%d, err=%v", len(page1), total, err)
	}

	page2, total, err := eng.ListMemoriesPage(2, 2, "created_at", "desc")
	if err != nil || total != 3 || len(page2) != 1 {
		t.Fatalf("page 2 failed: len=%d, total=%d, err=%v", len(page2), total, err)
	}

	// Out of bounds page
	page3, total, err := eng.ListMemoriesPage(3, 2, "created_at", "desc")
	if err != nil || total != 3 || len(page3) != 0 {
		t.Fatalf("page 3 failed: len=%d, total=%d, err=%v", len(page3), total, err)
	}

	// Boundary clamp test: page=0, limit=0
	clampedPage, total, err := eng.ListMemoriesPage(0, 0, "", "")
	if err != nil || total != 3 || len(clampedPage) != 3 {
		t.Fatalf("clamped page failed: len=%d, total=%d, err=%v", len(clampedPage), total, err)
	}

	// Boundary clamp test: limit > 1000
	hugeLimitPage, total, err := eng.ListMemoriesPage(1, 5000, "", "")
	if err != nil || total != 3 || len(hugeLimitPage) != 3 {
		t.Fatalf("huge limit page failed: len=%d, total=%d, err=%v", len(hugeLimitPage), total, err)
	}

	// 6. Live Vault File Hydration verification
	filePath1 := filepath.Join(tempVaultDir, id1+".okf")
	editedDiskContent := "---\ntype: memory\ncontainer_tag: alpha\n---\nEdited live directly on filesystem"
	if err := os.WriteFile(filePath1, []byte(editedDiskContent), 0644); err != nil {
		t.Fatalf("failed to write edited file: %v", err)
	}

	hydratedPage, _, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil {
		t.Fatalf("failed to query page: %v", err)
	}

	foundEdited := false
	for _, m := range hydratedPage {
		if m.ID == id1 {
			foundEdited = true
			if !strings.Contains(m.Content, "Edited live directly on filesystem") {
				t.Errorf("expected hydrated content from disk, got: %q", m.Content)
			}
		}
	}
	if !foundEdited {
		t.Errorf("expected memory %s in page results", id1)
	}

	// 7. Nil DB guards
	nilDBEng := NewEngine(nil, prov, v)
	if _, err := nilDBEng.CountMemories(); err == nil {
		t.Error("expected error from CountMemories with nil DB")
	}
	if _, err := nilDBEng.ListContainerTags(); err == nil {
		t.Error("expected error from ListContainerTags with nil DB")
	}
	if _, _, err := nilDBEng.ListMemoriesPage(1, 10, "", ""); err == nil {
		t.Error("expected error from ListMemoriesPage with nil DB")
	}
}

