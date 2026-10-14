package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
	"github.com/supermemory-native/supermemory-native/internal/memory"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

type TestStats struct {
	Passed int
	Failed int
}

func (s *TestStats) Assert(cond bool, msg string) {
	if !cond {
		s.Failed++
		fmt.Printf("  [FAIL] %s\n", msg)
	} else {
		s.Passed++
		fmt.Printf("  [PASS] %s\n", msg)
	}
}

func main() {
	stats := &TestStats{}
	start := time.Now()
	fmt.Println("=================================================================")
	fmt.Println("🚀 STARTING EMPIRICAL CHALLENGER STRESS & FUZZ HARNESS (M1)")
	fmt.Println("=================================================================")

	testExtremeInputsAndBoundaries(stats)
	testMaliciousSQLInjection(stats)
	testLargeVolumeAndPagination(stats)
	testVaultHydrationAndFallbacks(stats)
	testConcurrencyStress(stats)

	elapsed := time.Since(start)
	fmt.Println("=================================================================")
	fmt.Printf("🏁 EMPIRICAL TEST SUITE COMPLETED in %v\n", elapsed)
	fmt.Printf("Total Passed: %d, Total Failed: %d\n", stats.Passed, stats.Failed)
	fmt.Println("=================================================================")

	if stats.Failed > 0 {
		os.Exit(1)
	}
}

func testExtremeInputsAndBoundaries(stats *TestStats) {
	fmt.Println("\n--- SUITE 1: Extreme Inputs & Boundaries ---")

	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to init in-memory sqlite: %v", err))
		return
	}
	defer sdb.Close()

	prov := embedding.NewMockProvider()
	eng := memory.NewEngine(sdb, prov, nil)

	// 1.1 Empty DB Tests
	count, err := sdb.CountMemories()
	stats.Assert(err == nil && count == 0, "DB CountMemories on empty DB returns 0, nil err")

	tags, err := sdb.ListContainerTags()
	stats.Assert(err == nil && tags != nil && len(tags) == 0, "DB ListContainerTags on empty DB returns non-nil empty slice")

	mems, err := sdb.ListMemoriesPage(10, 0, "created_at", "desc")
	stats.Assert(err == nil && mems != nil && len(mems) == 0, "DB ListMemoriesPage on empty DB returns non-nil empty slice")

	// 1.2 Extreme Limit & Offset at DB Layer
	// Negative limit
	mems, err = sdb.ListMemoriesPage(-1, 0, "created_at", "desc")
	stats.Assert(err == nil && mems != nil, "DB ListMemoriesPage limit=-1 handled cleanly")

	mems, err = sdb.ListMemoriesPage(-999999, 0, "created_at", "desc")
	stats.Assert(err == nil && mems != nil, "DB ListMemoriesPage limit=-999999 handled cleanly")

	// Limit = 0
	mems, err = sdb.ListMemoriesPage(0, 0, "created_at", "desc")
	stats.Assert(err == nil && mems != nil, "DB ListMemoriesPage limit=0 handled cleanly")

	// Negative offset
	mems, err = sdb.ListMemoriesPage(10, -1, "created_at", "desc")
	stats.Assert(err == nil && mems != nil, "DB ListMemoriesPage offset=-1 handled cleanly")

	mems, err = sdb.ListMemoriesPage(10, -999999, "created_at", "desc")
	stats.Assert(err == nil && mems != nil, "DB ListMemoriesPage offset=-999999 handled cleanly")

	// Large offset
	mems, err = sdb.ListMemoriesPage(10, 1000000000, "created_at", "desc")
	stats.Assert(err == nil && len(mems) == 0 && mems != nil, "DB ListMemoriesPage offset=1000000000 returns empty slice")

	// 1.3 Seed 5 memories to test populated bounds
	now := time.Now()
	for i := 1; i <= 5; i++ {
		_ = sdb.SaveMemory(db.Memory{
			ID:           fmt.Sprintf("seed_%d", i),
			Content:      fmt.Sprintf("Memory seed %d", i),
			Vector:       []float32{0.1, 0.2},
			ContainerTag: "test",
			CreatedAt:    now.Add(time.Duration(i) * time.Minute),
		})
	}

	count, _ = eng.CountMemories()
	stats.Assert(count == 5, "Engine CountMemories matches seeded count 5")

	// 1.4 Engine Level Extreme Page / Limit
	// Page = 0
	pMems, tot, err := eng.ListMemoriesPage(0, 10, "created_at", "desc")
	stats.Assert(err == nil && tot == 5 && len(pMems) == 5, "Engine ListMemoriesPage page=0 clamped to page=1")

	// Page = -10
	pMems, tot, err = eng.ListMemoriesPage(-10, 10, "created_at", "desc")
	stats.Assert(err == nil && tot == 5 && len(pMems) == 5, "Engine ListMemoriesPage page=-10 clamped to page=1")

	// Limit = 0
	pMems, tot, err = eng.ListMemoriesPage(1, 0, "created_at", "desc")
	stats.Assert(err == nil && tot == 5 && len(pMems) == 5, "Engine ListMemoriesPage limit=0 clamped to limit=100")

	// Limit = -100
	pMems, tot, err = eng.ListMemoriesPage(1, -100, "created_at", "desc")
	stats.Assert(err == nil && tot == 5 && len(pMems) == 5, "Engine ListMemoriesPage limit=-100 clamped to limit=100")

	// Limit > 1000 (e.g. 5000)
	pMems, tot, err = eng.ListMemoriesPage(1, 5000, "created_at", "desc")
	stats.Assert(err == nil && tot == 5 && len(pMems) == 5, "Engine ListMemoriesPage limit=5000 clamped to limit=1000")

	// Page = 1,000,000 (very large page number)
	pMems, tot, err = eng.ListMemoriesPage(1000000, 10, "created_at", "desc")
	stats.Assert(err == nil && tot == 5 && len(pMems) == 0 && pMems != nil, "Engine ListMemoriesPage page=1000000 returns 0 items, correct totalCount, non-nil slice")

	// Large page number near MaxInt32
	pMems, tot, err = eng.ListMemoriesPage(math.MaxInt32/1000, 10, "created_at", "desc")
	stats.Assert(err == nil && tot == 5 && len(pMems) == 0 && pMems != nil, "Engine ListMemoriesPage huge page index handled safely")

	// 1.5 Nil Engine DB Guards
	nilEng := memory.NewEngine(nil, prov, nil)
	_, err = nilEng.CountMemories()
	stats.Assert(err != nil, "Engine CountMemories on nil DB returns error, no panic")

	nTags, err := nilEng.ListContainerTags()
	stats.Assert(err != nil && nTags != nil, "Engine ListContainerTags on nil DB returns error and non-nil slice")

	nMems, _, err := nilEng.ListMemoriesPage(1, 10, "created_at", "desc")
	stats.Assert(err != nil && nMems != nil, "Engine ListMemoriesPage on nil DB returns error and non-nil slice")

	// 1.6 Closed DB propagation
	closedDB, _ := db.NewSqliteDB(":memory:")
	_ = closedDB.Close()
	_, err = closedDB.CountMemories()
	stats.Assert(err != nil, "DB CountMemories on closed DB returns error")
	_, err = closedDB.ListContainerTags()
	stats.Assert(err != nil, "DB ListContainerTags on closed DB returns error")
	_, err = closedDB.ListMemoriesPage(10, 0, "created_at", "desc")
	stats.Assert(err != nil, "DB ListMemoriesPage on closed DB returns error")
}

func testMaliciousSQLInjection(stats *TestStats) {
	fmt.Println("\n--- SUITE 2: Malicious SQL Strings & Whitelist Fuzzing ---")

	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to init in-memory sqlite: %v", err))
		return
	}
	defer sdb.Close()

	// Seed records to verify DB integrity remains intact
	now := time.Now()
	for i := 1; i <= 5; i++ {
		_ = sdb.SaveMemory(db.Memory{
			ID:           fmt.Sprintf("inj_%d", i),
			Content:      fmt.Sprintf("Original Content %d", i),
			Vector:       []float32{0.1, 0.2},
			ContainerTag: "secure",
			CreatedAt:    now.Add(time.Duration(i) * time.Minute),
		})
	}

	sortColPayloads := []string{
		"created_at; DROP TABLE memories; --",
		"' OR '1'='1",
		"\" OR \"1\"=\"1",
		"id; UPDATE memories SET content='HACKED'; --",
		"id UNION SELECT 1,2,3,4,5,6--",
		"(SELECT 1 FROM memories)",
		"1; ATTACH DATABASE 'evil.db' AS evil; --",
		"created_at/*comment*/",
		"random()",
		"CASE WHEN (1=1) THEN created_at ELSE id END",
		"created_at, content",
		"0x637265617465645f6174",
		"\x00created_at",
		"created_at\r\n",
		"created_at\n",
		"created_at; VACUUM; --",
		"1234",
		"!@#$%^&*()_+{}[]:;<>,.?/|\\~",
		";--",
		"SLEEP(5)",
		"BENCHMARK(1000000,MD5(1))",
	}

	for _, payload := range sortColPayloads {
		res, err := sdb.ListMemoriesPage(5, 0, payload, "desc")
		stats.Assert(err == nil, fmt.Sprintf("SQL injection sortCol [%s] safely handled with err==nil", payload))
		stats.Assert(len(res) == 5, fmt.Sprintf("SQL injection sortCol [%s] returned expected 5 rows", payload))

		// Check table was not dropped or corrupted
		cnt, cErr := sdb.CountMemories()
		stats.Assert(cErr == nil && cnt == 5, fmt.Sprintf("Table integrity verified after sortCol [%s]", payload))
	}

	orderPayloads := []string{
		"ASC; DROP TABLE memories; --",
		"DESC; DELETE FROM memories; --",
		"ASC, id DESC",
		"DESC--",
		"' OR '1'='1",
		"(CASE WHEN (1=1) THEN 'ASC' ELSE 'DESC' END)",
		"ASC/*comment*/",
		"\x00ASC",
		"INVALID_ORDER",
		";--",
		"SLEEP(5)",
		"BENCHMARK(1000000,MD5(1))",
	}

	for _, payload := range orderPayloads {
		res, err := sdb.ListMemoriesPage(5, 0, "created_at", payload)
		stats.Assert(err == nil, fmt.Sprintf("SQL injection order [%s] safely handled with err==nil", payload))
		stats.Assert(len(res) == 5, fmt.Sprintf("SQL injection order [%s] returned expected 5 rows", payload))

		cnt, cErr := sdb.CountMemories()
		stats.Assert(cErr == nil && cnt == 5, fmt.Sprintf("Table integrity verified after order [%s]", payload))
	}

	// Verify content was not modified to 'HACKED'
	mems, _ := sdb.ListMemoriesPage(5, 0, "id", "asc")
	for _, m := range mems {
		stats.Assert(!strings.Contains(m.Content, "HACKED"), fmt.Sprintf("Memory %s content uncorrupted: %s", m.ID, m.Content))
	}

	// Verify legitimate whitelist columns & aliases work correctly
	legitCols := []struct {
		input       string
		expectedCol string
	}{
		{"created_at", "created_at"},
		{"createdAt", "created_at"},
		{"  createdAt  ", "created_at"},
		{"id", "id"},
		{"ID", "id"},
		{"content", "content"},
		{"container_tag", "container_tag"},
		{"containertag", "container_tag"},
		{"file_path", "file_path"},
		{"filepath", "file_path"},
	}

	for _, tc := range legitCols {
		res, err := sdb.ListMemoriesPage(2, 0, tc.input, "asc")
		stats.Assert(err == nil && len(res) == 2, fmt.Sprintf("Legitimate column [%s] query succeeded", tc.input))
	}

	legitOrders := []string{"asc", "ASC", "  asc  ", "desc", "DESC", "  desc  "}
	for _, lo := range legitOrders {
		res, err := sdb.ListMemoriesPage(2, 0, "created_at", lo)
		stats.Assert(err == nil && len(res) == 2, fmt.Sprintf("Legitimate order [%s] query succeeded", lo))
	}
}

func testLargeVolumeAndPagination(stats *TestStats) {
	fmt.Println("\n--- SUITE 3: Large Volume Scale Test (1,250 Memories) ---")

	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to init in-memory sqlite: %v", err))
		return
	}
	defer sdb.Close()

	prov := embedding.NewMockProvider()
	eng := memory.NewEngine(sdb, prov, nil)

	// Dataset breakdown:
	// "work": 450
	// "personal": 350
	// "finance": 250
	// "": 100  (empty tag, normalizes to "default")
	// "   ": 50 (whitespace tag, normalizes to "default")
	// "archive": 50
	// Total: 1,250
	type tagDist struct {
		tag   string
		count int
	}
	distributions := []tagDist{
		{"work", 450},
		{"personal", 350},
		{"finance", 250},
		{"", 100},
		{"   ", 50},
		{"archive", 50},
	}

	totalExpected := 1250
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	counter := 0

	insertStart := time.Now()
	for _, dist := range distributions {
		for i := 0; i < dist.count; i++ {
			counter++
			m := db.Memory{
				ID:           fmt.Sprintf("mem_%05d", counter),
				Content:      fmt.Sprintf("Content for item %d in tag %q", counter, dist.tag),
				Vector:       []float32{float32(counter) * 0.001, 0.5},
				ContainerTag: dist.tag,
				CreatedAt:    baseTime.Add(time.Duration(counter) * time.Minute),
				FilePath:     "",
			}
			if err := sdb.SaveMemory(m); err != nil {
				stats.Assert(false, fmt.Sprintf("Failed to insert memory %d: %v", counter, err))
				return
			}
		}
	}
	insertDuration := time.Since(insertStart)
	fmt.Printf("  Inserted %d memories in %v\n", totalExpected, insertDuration)

	// 3.1 Verify CountMemories at Scale
	dbCount, err := sdb.CountMemories()
	stats.Assert(err == nil && dbCount == totalExpected, fmt.Sprintf("DB CountMemories returns exact count %d (got %d)", totalExpected, dbCount))

	engCount, err := eng.CountMemories()
	stats.Assert(err == nil && engCount == totalExpected, fmt.Sprintf("Engine CountMemories returns exact count %d (got %d)", totalExpected, engCount))

	// 3.2 Verify ListContainerTags Aggregation at Scale
	tags, err := eng.ListContainerTags()
	stats.Assert(err == nil, "ListContainerTags returned nil error")
	stats.Assert(len(tags) == 5, fmt.Sprintf("Expected 5 grouped tags, got %d", len(tags)))

	expectedTagCounts := map[string]int{
		"archive":  50,
		"default":  150, // 100 empty + 50 whitespace
		"finance":  250,
		"personal": 350,
		"work":     450,
	}

	tagSum := 0
	for _, t := range tags {
		expected, exists := expectedTagCounts[t.ContainerTag]
		stats.Assert(exists, fmt.Sprintf("Tag %q is recognized in expected set", t.ContainerTag))
		stats.Assert(t.MemoryCount == expected, fmt.Sprintf("Tag %q MemoryCount expected %d, got %d", t.ContainerTag, expected, t.MemoryCount))
		stats.Assert(t.DocumentCount == expected, fmt.Sprintf("Tag %q DocumentCount expected %d, got %d", t.ContainerTag, expected, t.DocumentCount))
		stats.Assert(t.Name == t.ContainerTag, fmt.Sprintf("Tag %q Name matches ContainerTag", t.ContainerTag))
		stats.Assert(t.Emoji == "📁", fmt.Sprintf("Tag %q Emoji is 📁", t.ContainerTag))
		tagSum += t.MemoryCount
	}
	stats.Assert(tagSum == totalExpected, fmt.Sprintf("Sum of tag counts (%d) equals total records (%d)", tagSum, totalExpected))

	// Tags must be sorted alphabetically by tag ASC
	alphabetical := true
	for i := 0; i < len(tags)-1; i++ {
		if tags[i].ContainerTag > tags[i+1].ContainerTag {
			alphabetical = false
		}
	}
	stats.Assert(alphabetical, "ListContainerTags results are sorted alphabetically by tag ASC")

	// 3.3 Exhaustive Pagination Walk across 1,250 records (Limit = 100)
	// Expected: 13 pages (1..12 have 100 items, 13 has 50 items)
	seenIDs := make(map[string]bool)
	limit := 100
	totalFetched := 0

	for page := 1; page <= 13; page++ {
		pMems, tot, err := eng.ListMemoriesPage(page, limit, "created_at", "asc")
		stats.Assert(err == nil, fmt.Sprintf("Page %d query returned err==nil", page))
		stats.Assert(tot == totalExpected, fmt.Sprintf("Page %d totalCount metadata is %d", page, totalExpected))

		expectedLen := 100
		if page == 13 {
			expectedLen = 50
		}
		stats.Assert(len(pMems) == expectedLen, fmt.Sprintf("Page %d returned exactly %d items (got %d)", page, expectedLen, len(pMems)))

		for _, m := range pMems {
			if seenIDs[m.ID] {
				stats.Assert(false, fmt.Sprintf("Duplicate ID detected across pages: %s", m.ID))
			}
			seenIDs[m.ID] = true
			totalFetched++
		}
	}

	stats.Assert(totalFetched == totalExpected, fmt.Sprintf("Total items fetched across all pages is %d (got %d)", totalExpected, totalFetched))
	stats.Assert(len(seenIDs) == totalExpected, fmt.Sprintf("Total unique IDs is %d (got %d)", totalExpected, len(seenIDs)))

	// Check boundary beyond last page
	page14, tot14, err := eng.ListMemoriesPage(14, limit, "created_at", "asc")
	stats.Assert(err == nil && tot14 == totalExpected && len(page14) == 0 && page14 != nil, "Page 14 (first empty page) returns 0 items, correct total, non-nil slice")

	page100, tot100, err := eng.ListMemoriesPage(100, limit, "created_at", "asc")
	stats.Assert(err == nil && tot100 == totalExpected && len(page100) == 0 && page100 != nil, "Page 100 returns 0 items, correct total, non-nil slice")

	pageMillion, totMil, err := eng.ListMemoriesPage(1000000, limit, "created_at", "asc")
	stats.Assert(err == nil && totMil == totalExpected && len(pageMillion) == 0 && pageMillion != nil, "Page 1000000 returns 0 items, correct total, non-nil slice")

	// 3.4 Pagination with Custom Limits
	// Limit = 500 -> 3 pages: 500, 500, 250, then page 4 empty
	p1_500, _, _ := eng.ListMemoriesPage(1, 500, "id", "asc")
	stats.Assert(len(p1_500) == 500, "Limit 500 page 1 has 500 items")
	p2_500, _, _ := eng.ListMemoriesPage(2, 500, "id", "asc")
	stats.Assert(len(p2_500) == 500, "Limit 500 page 2 has 500 items")
	p3_500, _, _ := eng.ListMemoriesPage(3, 500, "id", "asc")
	stats.Assert(len(p3_500) == 250, "Limit 500 page 3 has 250 items")
	p4_500, _, _ := eng.ListMemoriesPage(4, 500, "id", "asc")
	stats.Assert(len(p4_500) == 0, "Limit 500 page 4 has 0 items")

	// Limit = 1000 -> 2 pages: 1000, 250, then page 3 empty
	p1_1000, _, _ := eng.ListMemoriesPage(1, 1000, "id", "asc")
	stats.Assert(len(p1_1000) == 1000, "Limit 1000 page 1 has 1000 items")
	p2_1000, _, _ := eng.ListMemoriesPage(2, 1000, "id", "asc")
	stats.Assert(len(p2_1000) == 250, "Limit 1000 page 2 has 250 items")
	p3_1000, _, _ := eng.ListMemoriesPage(3, 1000, "id", "asc")
	stats.Assert(len(p3_1000) == 0, "Limit 1000 page 3 has 0 items")

	// 3.5 Sorting Monotonicity at Scale
	// Test ASC vs DESC on created_at
	ascItems, _, _ := eng.ListMemoriesPage(1, 1000, "created_at", "asc")
	isAsc := true
	for i := 0; i < len(ascItems)-1; i++ {
		if ascItems[i].CreatedAt.After(ascItems[i+1].CreatedAt) {
			isAsc = false
		}
	}
	stats.Assert(isAsc, "created_at ASC returns strictly non-decreasing timestamps")

	descItems, _, _ := eng.ListMemoriesPage(1, 1000, "created_at", "desc")
	isDesc := true
	for i := 0; i < len(descItems)-1; i++ {
		if descItems[i].CreatedAt.Before(descItems[i+1].CreatedAt) {
			isDesc = false
		}
	}
	stats.Assert(isDesc, "created_at DESC returns strictly non-increasing timestamps")

	// Verify first item of ASC is last item of DESC
	stats.Assert(ascItems[0].ID == "mem_00001", fmt.Sprintf("First item in ASC is mem_00001 (got %s)", ascItems[0].ID))
	stats.Assert(descItems[0].ID == "mem_01250", fmt.Sprintf("First item in DESC is mem_01250 (got %s)", descItems[0].ID))
}

func testVaultHydrationAndFallbacks(stats *TestStats) {
	fmt.Println("\n--- SUITE 4: Vault Disk Hydration & Fallback Resilience ---")

	tempVaultDir, err := os.MkdirTemp("", "challenger_vault_*")
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to create temp vault dir: %v", err))
		return
	}
	defer os.RemoveAll(tempVaultDir)

	v, err := vault.NewVault(tempVaultDir)
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to init vault: %v", err))
		return
	}

	sdb, err := db.NewSqliteDB(":memory:")
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to init in-memory sqlite: %v", err))
		return
	}
	defer sdb.Close()

	prov := embedding.NewMockProvider()
	eng := memory.NewEngine(sdb, prov, v)

	// Create 100 memories, with physical files on disk
	now := time.Now()
	for i := 1; i <= 100; i++ {
		id := fmt.Sprintf("hydra_%03d", i)
		fileName := id + ".okf"
		filePath := filepath.Join(tempVaultDir, fileName)

		// Disk content differs from DB content
		initialDisk := fmt.Sprintf("---\ntype: memory\ncontainer_tag: sync\n---\nDisk Content Initial %d", i)
		_ = os.WriteFile(filePath, []byte(initialDisk), 0644)

		_ = sdb.SaveMemory(db.Memory{
			ID:           id,
			Content:      fmt.Sprintf("DB Content Stale %d", i),
			Vector:       []float32{0.1, 0.2},
			ContainerTag: "sync",
			CreatedAt:    now.Add(time.Duration(i) * time.Minute),
			FilePath:     filePath,
		})
	}

	// 4.1 Verify that ListMemoriesPage hydrates disk content
	page1, _, err := eng.ListMemoriesPage(1, 100, "id", "asc")
	stats.Assert(err == nil && len(page1) == 100, "Queried 100 memories with vault files")

	allHydrated := true
	for i, m := range page1 {
		expectedDisk := fmt.Sprintf("Disk Content Initial %d", i+1)
		if !strings.Contains(m.Content, expectedDisk) {
			allHydrated = false
			fmt.Printf("    Memory %s not hydrated: %q\n", m.ID, m.Content)
		}
	}
	stats.Assert(allHydrated, "All 100 memories hydrated from physical disk .okf files")

	// 4.2 Live disk modification: update first 10 files on disk
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("hydra_%03d", i)
		filePath := filepath.Join(tempVaultDir, id+".okf")
		newDisk := fmt.Sprintf("---\ntype: memory\ncontainer_tag: sync\n---\nLIVE MODIFIED ON DISK %d", i)
		_ = os.WriteFile(filePath, []byte(newDisk), 0644)
	}

	pageRefreshed, _, err := eng.ListMemoriesPage(1, 10, "id", "asc")
	stats.Assert(err == nil && len(pageRefreshed) == 10, "Fetched refreshed page 1 (items 1-10)")

	liveHydrated := true
	for i, m := range pageRefreshed {
		expectedLive := fmt.Sprintf("LIVE MODIFIED ON DISK %d", i+1)
		if !strings.Contains(m.Content, expectedLive) {
			liveHydrated = false
		}
	}
	stats.Assert(liveHydrated, "Live filesystem edits immediately reflected in ListMemoriesPage")

	// 4.3 Resilience to deleted / missing physical files: delete files 1..5
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("hydra_%03d", i)
		filePath := filepath.Join(tempVaultDir, id+".okf")
		_ = os.Remove(filePath)
	}

	pageFallback, _, err := eng.ListMemoriesPage(1, 10, "id", "asc")
	stats.Assert(err == nil && len(pageFallback) == 10, "Fetched page with missing disk files without error")

	// Files 1..5 must fall back to DB content; files 6..10 must retain live modified disk content
	fallbackWorked := true
	for i, m := range pageFallback {
		if i < 5 {
			expectedDB := fmt.Sprintf("DB Content Stale %d", i+1)
			if !strings.Contains(m.Content, expectedDB) {
				fallbackWorked = false
				fmt.Printf("    Expected DB fallback for %s, got: %q\n", m.ID, m.Content)
			}
		} else {
			expectedLive := fmt.Sprintf("LIVE MODIFIED ON DISK %d", i+1)
			if !strings.Contains(m.Content, expectedLive) {
				fallbackWorked = false
				fmt.Printf("    Expected live disk for %s, got: %q\n", m.ID, m.Content)
			}
		}
	}
	stats.Assert(fallbackWorked, "Missing disk files gracefully fall back to DB content while remaining files hydrate")
}

func testConcurrencyStress(stats *TestStats) {
	fmt.Println("\n--- SUITE 5: Concurrent Read / Query Stress ---")

	tempDir, err := os.MkdirTemp("", "challenger_conc_*")
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to create temp dir: %v", err))
		return
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test.db")
	sdb, err := db.NewSqliteDB(dbFile)
	if err != nil {
		stats.Assert(false, fmt.Sprintf("Failed to init file sqlite: %v", err))
		return
	}
	defer sdb.Close()

	prov := embedding.NewMockProvider()
	eng := memory.NewEngine(sdb, prov, nil)

	// Seed 200 items
	now := time.Now()
	for i := 1; i <= 200; i++ {
		_ = sdb.SaveMemory(db.Memory{
			ID:           fmt.Sprintf("conc_%d", i),
			Content:      fmt.Sprintf("Concurrent Content %d", i),
			Vector:       []float32{0.1, 0.2},
			ContainerTag: fmt.Sprintf("tag_%d", i%5),
			CreatedAt:    now.Add(time.Duration(i) * time.Minute),
		})
	}

	// Spin 20 concurrent goroutines executing various queries
	done := make(chan bool, 20)
	errChan := make(chan error, 100)

	for g := 0; g < 20; g++ {
		go func(gid int) {
			defer func() { done <- true }()
			for iter := 0; iter < 10; iter++ {
				_, err := eng.CountMemories()
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d CountMemories error: %w", gid, err)
					return
				}

				_, err = eng.ListContainerTags()
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d ListContainerTags error: %w", gid, err)
					return
				}

				page := (iter % 4) + 1
				mems, tot, err := eng.ListMemoriesPage(page, 20, "created_at", "desc")
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d ListMemoriesPage error: %w", gid, err)
					return
				}
				if tot != 200 || len(mems) != 20 {
					errChan <- fmt.Errorf("goroutine %d unexpected pagination: len=%d, tot=%d", gid, len(mems), tot)
					return
				}
			}
		}(g)
	}

	for g := 0; g < 20; g++ {
		<-done
	}
	close(errChan)
	var errs []error
	for e := range errChan {
		errs = append(errs, e)
	}

	for _, e := range errs {
		fmt.Printf("    Concurrent error: %v\n", e)
	}

	stats.Assert(len(errs) == 0, fmt.Sprintf("20 concurrent goroutines completed 200 total query iterations with 0 errors (got %d errors)", len(errs)))
}
