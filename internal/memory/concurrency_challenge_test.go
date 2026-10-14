package memory

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

// TestChallengeConcurrentListMemoriesAndDiskEdits tests simultaneous ListMemoriesPage calls
// while editing vault files on disk (without concurrent DB writes).
func TestChallengeConcurrentListMemoriesAndDiskEdits(t *testing.T) {
	tempDBDir, err := os.MkdirTemp("", "challenge_db_*")
	if err != nil {
		t.Fatalf("failed to create temp db dir: %v", err)
	}
	defer os.RemoveAll(tempDBDir)

	dbPath := filepath.Join(tempDBDir, "test.db")
	sdb, err := db.NewSqliteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	tempVaultDir, err := os.MkdirTemp("", "challenge_vault_*")
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

	// Seed 25 memories
	initialIDs := make([]string, 25)
	for i := 0; i < 25; i++ {
		tag := fmt.Sprintf("project_%d", i%5)
		content := fmt.Sprintf("Initial seed memory content %d for tag %s", i, tag)
		id, err := eng.AddMemory(content, tag)
		if err != nil {
			t.Fatalf("failed to seed memory %d: %v", i, err)
		}
		initialIDs[i] = id
	}

	var wg sync.WaitGroup
	stopSignal := make(chan struct{})

	var readSuccessCount int64
	var readErrorCount int64
	var diskWriteCount int64

	// Goroutines Group A: Concurrent ListMemoriesPage readers (10 workers)
	for w := 0; w < 10; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			for {
				select {
				case <-stopSignal:
					return
				default:
					page := r.Intn(5) + 1
					limit := r.Intn(15) + 5
					sortCols := []string{"created_at", "id", "content", "container_tag", "file_path", "createdAt"}
					sortCol := sortCols[r.Intn(len(sortCols))]
					orders := []string{"ASC", "DESC", "asc", "desc"}
					order := orders[r.Intn(len(orders))]

					mems, total, err := eng.ListMemoriesPage(page, limit, sortCol, order)
					if err != nil {
						atomic.AddInt64(&readErrorCount, 1)
						t.Errorf("ListMemoriesPage error during concurrent read: %v", err)
					} else {
						atomic.AddInt64(&readSuccessCount, 1)
						if mems == nil {
							t.Errorf("ListMemoriesPage returned nil slice")
						}
						if total != 25 {
							t.Errorf("unexpected total count: got %d, expected 25", total)
						}
						for _, m := range mems {
							if m.ID == "" {
								t.Errorf("read memory with empty ID")
							}
						}
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(w)
	}

	// Goroutines Group B: Concurrent disk file mutators (5 workers)
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID*100)))
			for {
				select {
				case <-stopSignal:
					return
				default:
					targetID := initialIDs[r.Intn(len(initialIDs))]
					filePath := filepath.Join(tempVaultDir, targetID+".okf")
					newBody := fmt.Sprintf("Live disk mutated content by worker %d at %s [iter %d]",
						workerID, time.Now().Format(time.RFC3339Nano), r.Intn(10000))
					newDiskContent := fmt.Sprintf("---\ntype: memory\ntitle: Mutated Memory %s\ncontainer_tag: concurrent_tag\n---\n%s\n",
						targetID, newBody)

					// Write to disk simulating user editing in Obsidian or external editor
					err := os.WriteFile(filePath, []byte(newDiskContent), 0644)
					if err == nil {
						atomic.AddInt64(&diskWriteCount, 1)
					}
					time.Sleep(2 * time.Millisecond)
				}
			}
		}(w)
	}

	// Let the stress test run for 2 seconds
	time.Sleep(2000 * time.Millisecond)
	close(stopSignal)
	wg.Wait()

	t.Logf("Stress test summary: Reads=%d, ReadErrors=%d, DiskWrites=%d",
		atomic.LoadInt64(&readSuccessCount),
		atomic.LoadInt64(&readErrorCount),
		atomic.LoadInt64(&diskWriteCount),
	)

	if atomic.LoadInt64(&readErrorCount) > 0 {
		t.Fatalf("encountered %d read errors during concurrent operations", atomic.LoadInt64(&readErrorCount))
	}
	if atomic.LoadInt64(&readSuccessCount) == 0 {
		t.Fatalf("no reads executed during test")
	}
}

// TestChallengeConcurrentDatabaseWritesDuringListMemories tests simultaneous ListMemoriesPage calls
// while new memories are actively being written to the database.
func TestChallengeConcurrentDatabaseWritesDuringListMemories(t *testing.T) {
	tempDBDir, err := os.MkdirTemp("", "challenge_db_writes_*")
	if err != nil {
		t.Fatalf("failed to create temp db dir: %v", err)
	}
	defer os.RemoveAll(tempDBDir)

	dbPath := filepath.Join(tempDBDir, "test.db")
	sdb, err := db.NewSqliteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	tempVaultDir, err := os.MkdirTemp("", "challenge_vault_writes_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	eng := NewEngine(sdb, embedding.NewMockProvider(), v)

	// Seed 10 memories
	for i := 0; i < 10; i++ {
		_, _ = eng.AddMemory(fmt.Sprintf("seed %d", i), "tag")
	}

	var wg sync.WaitGroup
	stopSignal := make(chan struct{})

	var readSuccessCount int64
	var readBusyErrorCount int64
	var writeCount int64

	// 5 Readers
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopSignal:
					return
				default:
					_, _, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
					if err != nil {
						if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
							atomic.AddInt64(&readBusyErrorCount, 1)
						} else {
							t.Errorf("unexpected error: %v", err)
						}
					} else {
						atomic.AddInt64(&readSuccessCount, 1)
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(w)
	}

	// 2 Writers adding memories
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			for {
				select {
				case <-stopSignal:
					return
				default:
					_, err := eng.AddMemory(fmt.Sprintf("content %d", r.Intn(10000)), "tag")
					if err == nil {
						atomic.AddInt64(&writeCount, 1)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
		}(w)
	}

	time.Sleep(1500 * time.Millisecond)
	close(stopSignal)
	wg.Wait()

	t.Logf("DB Write concurrency results: Reads=%d, ReadBusyErrors=%d, Writes=%d",
		atomic.LoadInt64(&readSuccessCount),
		atomic.LoadInt64(&readBusyErrorCount),
		atomic.LoadInt64(&writeCount),
	)
}

// TestChallengeHydrationCorruptedFiles tests handling of malformed, binary, 0-byte,
// and missing files during vault hydration.
func TestChallengeHydrationCorruptedFiles(t *testing.T) {
	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	tempVaultDir, err := os.MkdirTemp("", "challenge_corrupt_vault_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	eng := NewEngine(sdb, embedding.NewMockProvider(), v)

	// Scenario A: Malformed YAML frontmatter / garbage content
	corruptContent := "Corrupt original DB content"
	idA, err := eng.AddMemory(corruptContent, "tag_a")
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}
	corruptFilePath := filepath.Join(tempVaultDir, idA+".okf")
	// Overwrite with severely broken syntax: unclosed delimiters, invalid characters
	garbageData := "--- malformed ::: yaml {{}} [[ unclosed \x00\xff\xfe binary garbage"
	if err := os.WriteFile(corruptFilePath, []byte(garbageData), 0644); err != nil {
		t.Fatalf("failed to write garbage data: %v", err)
	}

	// Scenario B: Zero-byte file
	zeroContent := "Zero-byte original DB content"
	idB, err := eng.AddMemory(zeroContent, "tag_b")
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}
	zeroFilePath := filepath.Join(tempVaultDir, idB+".okf")
	if err := os.WriteFile(zeroFilePath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write zero byte file: %v", err)
	}

	// Scenario C: Large content (1 MB)
	largeContent := "Large original DB content"
	idC, err := eng.AddMemory(largeContent, "tag_c")
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}
	largeFilePath := filepath.Join(tempVaultDir, idC+".okf")
	largeData := strings.Repeat("A", 1024*1024) // 1MB
	if err := os.WriteFile(largeFilePath, []byte(largeData), 0644); err != nil {
		t.Fatalf("failed to write large file: %v", err)
	}

	// Scenario D: Directory path instead of file
	dirMem := db.Memory{
		ID:           "id_dir_mem",
		Content:      "DB content with directory FilePath",
		Vector:       []float32{0.1, 0.2},
		ContainerTag: "tag_d",
		CreatedAt:    time.Now(),
		FilePath:     tempVaultDir, // is a directory!
	}
	if err := sdb.SaveMemory(dirMem); err != nil {
		t.Fatalf("failed to save dir memory: %v", err)
	}

	// Query ListMemoriesPage to verify resilience
	mems, total, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil {
		t.Fatalf("ListMemoriesPage failed on corrupted files: %v", err)
	}
	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}

	memMap := make(map[string]db.Memory)
	for _, m := range mems {
		memMap[m.ID] = m
	}

	// Verify Scenario A: Garbage data read without panic
	if m, ok := memMap[idA]; !ok {
		t.Errorf("memory idA not found")
	} else {
		if !strings.Contains(m.Content, "malformed") {
			t.Errorf("expected garbage content returned verbatim without panic, got: %q", m.Content)
		}
	}

	// Verify Scenario B: 0-byte file
	if m, ok := memMap[idB]; !ok {
		t.Errorf("memory idB not found")
	} else {
		t.Logf("0-byte file returned content: %q (length %d)", m.Content, len(m.Content))
	}

	// Verify Scenario C: Large file read without error
	if m, ok := memMap[idC]; !ok {
		t.Errorf("memory idC not found")
	} else {
		if len(m.Content) != 1024*1024 {
			t.Errorf("expected 1MB content, got length %d", len(m.Content))
		}
	}

	// Verify Scenario D: Directory path gracefully falls back to DB content
	if m, ok := memMap["id_dir_mem"]; !ok {
		t.Errorf("memory id_dir_mem not found")
	} else {
		if m.Content != "DB content with directory FilePath" {
			t.Errorf("expected graceful fallback to DB content when FilePath is directory, got: %q", m.Content)
		}
	}
}

// TestChallengeHydrationLockedFile tests that when a physical OKF file is locked
// exclusively on Windows, ListMemoriesPage gracefully falls back to the database content.
func TestChallengeHydrationLockedFile(t *testing.T) {
	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	tempVaultDir, err := os.MkdirTemp("", "challenge_locked_vault_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	eng := NewEngine(sdb, embedding.NewMockProvider(), v)

	origContent := "Original SQLite content that must be returned on lock fallback"
	id, err := eng.AddMemory(origContent, "user_locked")
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}

	filePath := filepath.Join(tempVaultDir, id+".okf")

	// Update physical file with mutated content
	diskContent := "---\ntype: memory\n---\nDisk content before locking\n"
	if err := os.WriteFile(filePath, []byte(diskContent), 0644); err != nil {
		t.Fatalf("failed to write disk content: %v", err)
	}

	// Lock the file exclusively on Windows using CreateFile with shareMode=0
	pathPtr, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		t.Fatalf("UTF16PtrFromString failed: %v", err)
	}

	// dwShareMode = 0 prevents any other process/thread from reading or writing
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // EXCLUSIVE LOCK - NO SHARING
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("failed to exclusively lock file %s: %v", filePath, err)
	}

	// Ensure we release the lock at the end
	defer syscall.CloseHandle(handle)

	// Now try to read via os.ReadFile directly to confirm it returns ERROR_SHARING_VIOLATION
	_, readErr := os.ReadFile(filePath)
	if readErr == nil {
		t.Fatalf("expected os.ReadFile to fail on locked file, but it succeeded")
	}
	t.Logf("Confirmed locked file read error: %v", readErr)

	// Call ListMemoriesPage while file is locked
	mems, total, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil {
		t.Fatalf("ListMemoriesPage failed instead of falling back gracefully: %v", err)
	}
	if total != 1 || len(mems) != 1 {
		t.Fatalf("expected 1 memory, got total=%d len=%d", total, len(mems))
	}

	// The memory content MUST gracefully fall back to the SQLite DB content!
	if mems[0].Content != origContent {
		t.Errorf("expected fallback to DB content %q, got: %q", origContent, mems[0].Content)
	} else {
		t.Logf("Graceful fallback verified! Returned DB content: %q", mems[0].Content)
	}

	// Now unlock the file
	if err := syscall.CloseHandle(handle); err != nil {
		t.Errorf("failed to close handle: %v", err)
	}

	// Now that file is unlocked, ListMemoriesPage should hydrate the disk content again
	memsUnlocked, _, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
	if err != nil {
		t.Fatalf("ListMemoriesPage failed after unlock: %v", err)
	}
	if !strings.Contains(memsUnlocked[0].Content, "Disk content before locking") {
		t.Errorf("expected hydrated content after unlock, got: %q", memsUnlocked[0].Content)
	} else {
		t.Logf("Hydration resumed after unlock! Content: %q", memsUnlocked[0].Content)
	}
}

// TestChallengeConcurrentFileReplacementAtomic tests rapid atomic file replacement
// (temp file rename over existing file) while concurrent ListMemoriesPage calls are running.
func TestChallengeConcurrentFileReplacementAtomic(t *testing.T) {
	tempDBDir, err := os.MkdirTemp("", "challenge_atomic_db_*")
	if err != nil {
		t.Fatalf("failed to create temp db dir: %v", err)
	}
	defer os.RemoveAll(tempDBDir)

	dbPath := filepath.Join(tempDBDir, "atomic.db")
	sdb, err := db.NewSqliteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer sdb.Close()

	tempVaultDir, err := os.MkdirTemp("", "challenge_atomic_vault_*")
	if err != nil {
		t.Fatalf("failed to create temp vault dir: %v", err)
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	eng := NewEngine(sdb, embedding.NewMockProvider(), v)

	// Create 10 memories
	ids := make([]string, 10)
	for i := 0; i < 10; i++ {
		id, err := eng.AddMemory(fmt.Sprintf("Original content %d", i), "atomic_tag")
		if err != nil {
			t.Fatalf("failed to add memory: %v", err)
		}
		ids[i] = id
	}

	var wg sync.WaitGroup
	stopSignal := make(chan struct{})

	// 8 Readers
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stopSignal:
					return
				default:
					mems, total, err := eng.ListMemoriesPage(1, 10, "created_at", "desc")
					if err != nil {
						t.Errorf("reader %d error: %v", id, err)
					}
					if total != 10 || len(mems) != 10 {
						t.Errorf("reader %d unexpected count: total=%d len=%d", id, total, len(mems))
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(w)
	}

	// 4 Writers performing atomic overwrites (write temp file + rename)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID*77)))
			for {
				select {
				case <-stopSignal:
					return
				default:
					targetID := ids[r.Intn(len(ids))]
					targetPath := filepath.Join(tempVaultDir, targetID+".okf")
					tempPath := filepath.Join(tempVaultDir, fmt.Sprintf("%s.tmp.%d", targetID, r.Intn(100000)))

					content := fmt.Sprintf("---\ntype: memory\n---\nAtomically replaced %d at %s\n",
						workerID, time.Now().Format(time.RFC3339Nano))

					if err := os.WriteFile(tempPath, []byte(content), 0644); err == nil {
						// On Windows, os.Rename over an existing file may fail if file is open,
						// or succeed if not locked. We test how both handle it.
						_ = os.Rename(tempPath, targetPath)
						_ = os.Remove(tempPath) // clean up if rename failed
					}
					time.Sleep(2 * time.Millisecond)
				}
			}
		}(w)
	}

	time.Sleep(2000 * time.Millisecond)
	close(stopSignal)
	wg.Wait()
}
