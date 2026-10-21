package memory

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

type testBagProvider struct{ fail bool }

func (p testBagProvider) Dimension() int { return 64 }

func (p testBagProvider) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if p.fail {
		return nil, errors.New("embedding failed")
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, 64)
		for _, token := range strings.Fields(strings.ToLower(text)) {
			var h uint32 = 2166136261
			for j := 0; j < len(token); j++ {
				h ^= uint32(token[j])
				h *= 16777619
			}
			vec[h%64]++
		}
		var sum float64
		for _, v := range vec {
			sum += float64(v) * float64(v)
		}
		if sum > 0 {
			norm := float32(math.Sqrt(sum))
			for j := range vec {
				vec[j] /= norm
			}
		}
		out[i] = vec
	}
	return out, nil
}

func newProcessorTestDB(t *testing.T) *db.SqliteDB {
	t.Helper()
	database, err := db.NewSqliteDB(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func seedDoc(t *testing.T, database *db.SqliteDB, ns, id, content, taskType string) {
	t.Helper()
	now := time.Now().UTC()
	if err := database.UpsertDocument(db.Document{
		Namespace: ns, ID: id, Title: "title", Summary: "summary", Content: content,
		TaskType: taskType, Dreaming: "dynamic", Status: db.DocumentQueued,
		DreamingStatus: db.DocumentQueued, Metadata: map[string]any{}, Grp: map[string]any{},
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}

func TestProcessorTransitions(t *testing.T) {
	database := newProcessorTestDB(t)
	p := NewProcessor(database, testBagProvider{})
	content := "My favorite color is teal. I live in Berlin. This final sentence is long enough."
	seedDoc(t, database, "ns", "d1", content, "memory")

	processed, err := p.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d", processed)
	}
	doc, _, _ := database.GetDocument("ns", "d1")
	if doc.Status != db.DocumentDone || doc.DreamingStatus != db.DocumentDone {
		t.Errorf("status = %q/%q", doc.Status, doc.DreamingStatus)
	}
	chunks, _ := database.GetChunks("ns", "d1")
	if len(chunks) == 0 {
		t.Errorf("expected chunks")
	}
	mems, _ := database.GetMemoriesByDocument("ns", "d1", 100)
	if len(mems) == 0 {
		t.Errorf("expected memories")
	}
	if len(mems) > 0 && mems[0].UpdatedAt.IsZero() {
		t.Errorf("memory updated_at was not persisted")
	}

	seedDoc(t, database, "ns", "d2", content, "superrag")
	if _, err := p.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("process superrag: %v", err)
	}
	superMems, _ := database.GetMemoriesByDocument("ns", "d2", 100)
	if len(superMems) != 0 {
		t.Errorf("superrag produced %d memories", len(superMems))
	}
	superDoc, _, _ := database.GetDocument("ns", "d2")
	if superDoc.DreamingStatus != db.DocumentDone {
		t.Errorf("dreaming status = %q", superDoc.DreamingStatus)
	}
}

func TestProcessorIdempotent(t *testing.T) {
	database := newProcessorTestDB(t)
	p := NewProcessor(database, testBagProvider{})
	content := "My favorite color is teal. I live in Berlin. This final sentence is long enough."
	seedDoc(t, database, "ns", "d1", content, "memory")

	if _, err := p.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("first process: %v", err)
	}
	chunksBefore, _ := database.GetChunks("ns", "d1")
	memsBefore, _ := database.GetMemoriesByDocument("ns", "d1", 100)

	if _, _, err := database.UpdateDocumentFields("ns", "d1", map[string]any{
		"status": db.DocumentQueued, "dreaming_status": db.DocumentQueued, "reset_memories": 0,
	}); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if _, err := p.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("second process: %v", err)
	}
	chunksAfter, _ := database.GetChunks("ns", "d1")
	memsAfter, _ := database.GetMemoriesByDocument("ns", "d1", 100)
	if len(chunksAfter) != len(chunksBefore) {
		t.Errorf("chunks changed: %d -> %d", len(chunksBefore), len(chunksAfter))
	}
	if len(memsAfter) != len(memsBefore) {
		t.Errorf("memories changed: %d -> %d", len(memsBefore), len(memsAfter))
	}
}

func TestProcessorFailure(t *testing.T) {
	database := newProcessorTestDB(t)
	p := NewProcessor(database, testBagProvider{fail: true})
	seedDoc(t, database, "ns", "d1", "This sentence is long enough to chunk.", "memory")

	processed, err := p.ProcessOnce(context.Background())
	if err == nil {
		t.Fatalf("expected an error")
	}
	if processed != 1 {
		t.Errorf("processed = %d", processed)
	}
	doc, _, _ := database.GetDocument("ns", "d1")
	if doc.Status != db.DocumentFailed || doc.DreamingStatus != db.DocumentFailed {
		t.Errorf("status = %q/%q", doc.Status, doc.DreamingStatus)
	}
}

// requeueProvider simulates a PATCH that re-queues the document while the
// worker is embedding the previously claimed content.
type requeueProvider struct {
	database *db.SqliteDB
	fired    bool
}

func (p *requeueProvider) Dimension() int { return 64 }

func (p *requeueProvider) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if !p.fired {
		p.fired = true
		if _, _, err := p.database.UpdateDocumentFields("ns", "d1", map[string]any{
			"content":         "New content that is long enough.",
			"status":          db.DocumentQueued,
			"dreaming_status": db.DocumentQueued,
			"reset_memories":  1,
			"updated_at":      time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
	}
	return testBagProvider{}.GenerateEmbeddings(texts)
}

func TestProcessorRequeuedDuringProcessing(t *testing.T) {
	database := newProcessorTestDB(t)
	provider := &requeueProvider{database: database}
	p := NewProcessor(database, provider)
	seedDoc(t, database, "ns", "d1", "My favorite color is teal.", "memory")

	if _, err := p.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("first process: %v", err)
	}
	doc, _, _ := database.GetDocument("ns", "d1")
	if doc.Status != db.DocumentQueued {
		t.Fatalf("status after racing write = %q, want queued", doc.Status)
	}
	if doc.Content != "New content that is long enough." {
		t.Errorf("content = %q", doc.Content)
	}
	if mems, _ := database.GetMemoriesByDocument("ns", "d1", 100); len(mems) != 0 {
		t.Errorf("stale memories were persisted: %d", len(mems))
	}

	if _, err := p.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("second process: %v", err)
	}
	doc, _, _ = database.GetDocument("ns", "d1")
	if doc.Status != db.DocumentDone {
		t.Fatalf("final status = %q", doc.Status)
	}
	chunks, _ := database.GetChunks("ns", "d1")
	if len(chunks) == 0 || !strings.Contains(chunks[0].Content, "New content") {
		t.Errorf("chunks = %+v", chunks)
	}
	mems, _ := database.GetMemoriesByDocument("ns", "d1", 100)
	if len(mems) != 0 {
		for _, m := range mems {
			if strings.Contains(m.Content, "teal") {
				t.Errorf("memory from replaced content: %q", m.Content)
			}
		}
	}
}
