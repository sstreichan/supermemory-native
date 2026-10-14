package db

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSqliteDB(t *testing.T) {
	// Use an in-memory SQLite database for testing
	dbFile := ":memory:"
	sdb, err := NewSqliteDB(dbFile)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer sdb.Close()

	// 1. Save memory
	m1 := Memory{
		ID:           "mem1",
		Content:      "Alice prefers Node.js for quick MCP bridges.",
		Vector:       []float32{1.0, 0.0, 0.0},
		ContainerTag: "user_alice",
		CreatedAt:    time.Now(),
		FilePath:     "/vault/mem1.okf",
	}
	if err := sdb.SaveMemory(m1); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}

	m2 := Memory{
		ID:           "mem2",
		Content:      "Rust provides ultra-fast zero-overhead vector calculations.",
		Vector:       []float32{0.0, 1.0, 0.0},
		ContainerTag: "user_alice",
		CreatedAt:    time.Now(),
		FilePath:     "/vault/mem2.okf",
	}
	if err := sdb.SaveMemory(m2); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}

	// 2. List memories
	list, err := sdb.ListMemories("user_alice", 10)
	if err != nil {
		t.Fatalf("failed to list memories: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 memories, got %d", len(list))
	}
	
	// Assert file paths exist
	foundMem1 := false
	for _, m := range list {
		if m.ID == "mem1" {
			foundMem1 = true
			if m.FilePath != "/vault/mem1.okf" {
				t.Errorf("expected file path '/vault/mem1.okf', got %q", m.FilePath)
			}
		}
	}
	if !foundMem1 {
		t.Error("expected to find mem1 in listed memories")
	}

	// 3. Search memories (Semantic Search)
	queryVec := []float32{1.0, 0.1, 0.0}
	results, err := sdb.SearchMemories("user_alice", queryVec, 0.5, 10)
	if err != nil {
		t.Fatalf("failed to search memories: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Memory.ID != "mem1" {
		t.Errorf("expected mem1, got %s", results[0].Memory.ID)
	}
	if results[0].Memory.FilePath != "/vault/mem1.okf" {
		t.Errorf("expected search result to carry correct FilePath, got %q", results[0].Memory.FilePath)
	}

	// 4. Delete memory
	if err := sdb.DeleteMemory("mem1"); err != nil {
		t.Fatalf("failed to delete memory: %v", err)
	}
	list, err = sdb.ListMemories("user_alice", 10)
	if err != nil {
		t.Fatalf("failed to list memories: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 memory after deletion, got %d", len(list))
	}
}

func TestNewSqliteDBError(t *testing.T) {
	// Attempt to open a database at an invalid location to trigger error
	_, err := NewSqliteDB("/nonexistent_dir/db.sqlite")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

func TestCountMemories(t *testing.T) {
	sdb, err := NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	// 1. Empty DB returns 0
	count, err := sdb.CountMemories()
	if err != nil {
		t.Fatalf("expected nil error on empty DB, got: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 for empty DB, got %d", count)
	}

	// 2. Incremental inserts
	m1 := Memory{
		ID:           "mem_count_1",
		Content:      "Memory One",
		Vector:       []float32{1.0, 0.0},
		ContainerTag: "tag_a",
		CreatedAt:    time.Now(),
	}
	if err := sdb.SaveMemory(m1); err != nil {
		t.Fatalf("failed to save m1: %v", err)
	}

	count, err = sdb.CountMemories()
	if err != nil || count != 1 {
		t.Errorf("expected count 1 after 1 insert, got %d (err: %v)", count, err)
	}

	m2 := Memory{
		ID:           "mem_count_2",
		Content:      "Memory Two",
		Vector:       []float32{0.0, 1.0},
		ContainerTag: "tag_b",
		CreatedAt:    time.Now(),
	}
	m3 := Memory{
		ID:           "mem_count_3",
		Content:      "Memory Three",
		Vector:       []float32{0.5, 0.5},
		ContainerTag: "tag_a",
		CreatedAt:    time.Now(),
	}
	_ = sdb.SaveMemory(m2)
	_ = sdb.SaveMemory(m3)

	count, err = sdb.CountMemories()
	if err != nil || count != 3 {
		t.Errorf("expected count 3 after 3 inserts, got %d (err: %v)", count, err)
	}

	// 3. Decrement after delete
	if err := sdb.DeleteMemory("mem_count_2"); err != nil {
		t.Fatalf("failed to delete mem_count_2: %v", err)
	}
	count, err = sdb.CountMemories()
	if err != nil || count != 2 {
		t.Errorf("expected count 2 after delete, got %d (err: %v)", count, err)
	}

	// 4. Closed DB error handling
	_ = sdb.Close()
	_, err = sdb.CountMemories()
	if err == nil {
		t.Error("expected error when calling CountMemories on closed DB, got nil")
	}
}

func TestListMemoriesPage(t *testing.T) {
	sdb, err := NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	// 1. Empty DB bounds
	emptyList, err := sdb.ListMemoriesPage(10, 0, "created_at", "desc")
	if err != nil {
		t.Fatalf("expected nil error on empty DB, got: %v", err)
	}
	if len(emptyList) != 0 {
		t.Errorf("expected 0 items from empty DB, got %d", len(emptyList))
	}

	// Seed 5 memories with deterministic timestamps
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		m := Memory{
			ID:           fmt.Sprintf("id_%d", i),
			Content:      fmt.Sprintf("Content %d", i),
			Vector:       []float32{float32(i) * 0.1, 0.5},
			ContainerTag: "work",
			CreatedAt:    baseTime.Add(time.Duration(i) * time.Hour),
		}
		if err := sdb.SaveMemory(m); err != nil {
			t.Fatalf("failed to save memory %d: %v", i, err)
		}
	}

	// 2. Pagination Limit & Offset (ASC)
	page1, err := sdb.ListMemoriesPage(2, 0, "created_at", "asc")
	if err != nil || len(page1) != 2 {
		t.Fatalf("expected 2 items for page 1, got %d (err: %v)", len(page1), err)
	}
	if page1[0].ID != "id_1" || page1[1].ID != "id_2" {
		t.Errorf("unexpected page 1 items: %s, %s", page1[0].ID, page1[1].ID)
	}

	page2, err := sdb.ListMemoriesPage(2, 2, "created_at", "asc")
	if err != nil || len(page2) != 2 {
		t.Fatalf("expected 2 items for page 2, got %d (err: %v)", len(page2), err)
	}
	if page2[0].ID != "id_3" || page2[1].ID != "id_4" {
		t.Errorf("unexpected page 2 items: %s, %s", page2[0].ID, page2[1].ID)
	}

	page3, err := sdb.ListMemoriesPage(2, 4, "created_at", "asc")
	if err != nil || len(page3) != 1 {
		t.Fatalf("expected 1 item for page 3, got %d (err: %v)", len(page3), err)
	}
	if page3[0].ID != "id_5" {
		t.Errorf("unexpected page 3 item: %s", page3[0].ID)
	}

	// 3. Out-of-bounds offset returns empty slice without error
	outOfBounds, err := sdb.ListMemoriesPage(2, 10, "created_at", "asc")
	if err != nil {
		t.Fatalf("expected nil error on out-of-bounds offset, got: %v", err)
	}
	if len(outOfBounds) != 0 {
		t.Errorf("expected 0 items for out-of-bounds offset, got %d", len(outOfBounds))
	}

	// 4. Sort Order (DESC)
	descPage, err := sdb.ListMemoriesPage(5, 0, "created_at", "desc")
	if err != nil || len(descPage) != 5 {
		t.Fatalf("expected 5 items for DESC, got %d (err: %v)", len(descPage), err)
	}
	if descPage[0].ID != "id_5" || descPage[4].ID != "id_1" {
		t.Errorf("expected first item id_5 and last id_1 in DESC, got %s and %s", descPage[0].ID, descPage[4].ID)
	}

	// 5. Column Whitelist Fallback & SQL Injection Safety
	// Column alias mapping (createdAt -> created_at)
	aliasPage, err := sdb.ListMemoriesPage(2, 0, "createdAt", "desc")
	if err != nil || len(aliasPage) != 2 || aliasPage[0].ID != "id_5" {
		t.Errorf("failed camelCase column alias mapping: %v", err)
	}

	// Column alias mapping (id)
	idPage, err := sdb.ListMemoriesPage(2, 0, "id", "asc")
	if err != nil || len(idPage) != 2 || idPage[0].ID != "id_1" {
		t.Errorf("failed id sort column: %v", err)
	}

	// Attempted SQL injection in column name must safely fall back to default created_at
	injectedCol, err := sdb.ListMemoriesPage(2, 0, "created_at; DROP TABLE memories; --", "desc")
	if err != nil {
		t.Fatalf("expected whitelist fallback to succeed without error, got: %v", err)
	}
	if len(injectedCol) != 2 {
		t.Errorf("expected 2 items from fallback, got %d", len(injectedCol))
	}

	// Verify memories table was NOT dropped
	countAfterInjection, err := sdb.CountMemories()
	if err != nil || countAfterInjection != 5 {
		t.Fatalf("table memories corrupted or dropped by injection: %v, count=%d", err, countAfterInjection)
	}

	// Invalid order direction falls back to DESC
	invalidOrderPage, err := sdb.ListMemoriesPage(2, 0, "created_at", "INVALID_ORDER_INJECTION; --")
	if err != nil || len(invalidOrderPage) != 2 || invalidOrderPage[0].ID != "id_5" {
		t.Errorf("failed order fallback to DESC: %v", err)
	}
}

func TestListContainerTags(t *testing.T) {
	sdb, err := NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	// 1. Empty DB returns empty slice
	emptyTags, err := sdb.ListContainerTags()
	if err != nil {
		t.Fatalf("expected nil error on empty DB, got: %v", err)
	}
	if len(emptyTags) != 0 {
		t.Errorf("expected 0 container tags on empty DB, got %d", len(emptyTags))
	}

	// 2. Multiple tags and multiple memories under same tag
	dataset := []struct {
		id  string
		tag string
	}{
		{"m1", "work"},
		{"m2", "work"},
		{"m3", "work"},
		{"m4", "personal"},
		{"m5", "personal"},
		{"m6", "research"},
		{"m7", ""}, // empty container_tag to test fallback
	}

	for _, item := range dataset {
		m := Memory{
			ID:           item.id,
			Content:      "sample content",
			Vector:       []float32{1.0, 0.0},
			ContainerTag: item.tag,
			CreatedAt:    time.Now(),
		}
		if err := sdb.SaveMemory(m); err != nil {
			t.Fatalf("failed to save memory %s: %v", item.id, err)
		}
	}

	tags, err := sdb.ListContainerTags()
	if err != nil {
		t.Fatalf("failed to list container tags: %v", err)
	}

	// Expect 4 groups: work (3), personal (2), research (1), default (1)
	if len(tags) != 4 {
		t.Fatalf("expected 4 container tag groups, got %d: %+v", len(tags), tags)
	}

	tagMap := make(map[string]ContainerTagStats)
	for _, stat := range tags {
		tagMap[stat.ContainerTag] = stat
	}

	// Verify "work" group
	if w, exists := tagMap["work"]; !exists {
		t.Error("missing 'work' tag")
	} else {
		if w.DocumentCount != 3 || w.MemoryCount != 3 {
			t.Errorf("expected work count 3, got doc=%d, mem=%d", w.DocumentCount, w.MemoryCount)
		}
		if w.Name != "work" {
			t.Errorf("expected Name 'work', got %q", w.Name)
		}
		if w.Emoji == "" {
			t.Error("expected non-empty Emoji")
		}
	}

	// Verify "personal" group
	if p, exists := tagMap["personal"]; !exists {
		t.Error("missing 'personal' tag")
	} else if p.DocumentCount != 2 || p.MemoryCount != 2 {
		t.Errorf("expected personal count 2, got doc=%d, mem=%d", p.DocumentCount, p.MemoryCount)
	}

	// Verify "research" group
	if r, exists := tagMap["research"]; !exists {
		t.Error("missing 'research' tag")
	} else if r.DocumentCount != 1 || r.MemoryCount != 1 {
		t.Errorf("expected research count 1, got doc=%d, mem=%d", r.DocumentCount, r.MemoryCount)
	}

	// 3. Verify empty tag fallback to "default"
	if def, exists := tagMap["default"]; !exists {
		t.Error("expected empty tag to normalize/group under 'default'")
	} else if def.DocumentCount != 1 || def.MemoryCount != 1 {
		t.Errorf("expected default count 1, got doc=%d, mem=%d", def.DocumentCount, def.MemoryCount)
	}
}

func TestMain(m *testing.M) {
	// We make sure the test runner cleans up any generated databases
	code := m.Run()
	os.Remove("test.db")
	os.Exit(code)
}

