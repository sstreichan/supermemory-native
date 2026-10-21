package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
)

// Processor turns queued v5 documents into chunks and memories.
type Processor struct {
	DB       *db.SqliteDB
	Provider embedding.EmbeddingProvider
}

func NewProcessor(database *db.SqliteDB, provider embedding.EmbeddingProvider) *Processor {
	return &Processor{DB: database, Provider: provider}
}

// ProcessOnce claims and processes at most one queued document. It returns 1
// when a document was picked up (successfully or not) and 0 when the queue is
// empty.
func (p *Processor) ProcessOnce(ctx context.Context) (int, error) {
	doc, ok, err := p.DB.ClaimQueuedDocument()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}

	fail := func(cause error) (int, error) {
		ok, err := p.DB.UpdateProcessingStatus(doc.Namespace, doc.ID, db.DocumentFailed, db.DocumentFailed)
		if err != nil {
			return 1, err
		}
		if !ok {
			// A newer write re-queued the document; leave it queued.
			return 1, nil
		}
		return 1, cause
	}
	advance := func(status, dreamingStatus string) (bool, error) {
		return p.DB.UpdateProcessingStatus(doc.Namespace, doc.ID, status, dreamingStatus)
	}

	if ok, err := advance(db.DocumentChunking, "queued"); err != nil {
		return fail(err)
	} else if !ok {
		return 1, nil
	}
	texts := chunkText(doc.Content)

	embeddings, err := p.Provider.GenerateEmbeddings(texts)
	if err != nil {
		return fail(err)
	}
	if len(embeddings) != len(texts) {
		return fail(fmt.Errorf("embedding count mismatch: got %d for %d chunks", len(embeddings), len(texts)))
	}
	if ok, err := advance(db.DocumentEmbedding, "queued"); err != nil {
		return fail(err)
	} else if !ok {
		return 1, nil
	}

	now := time.Now().UTC()
	chunks := make([]db.Chunk, 0, len(texts))
	for i, text := range texts {
		sum := sha256.Sum256([]byte(doc.Namespace + "\x00" + doc.ID + "\x00" + fmt.Sprint(i)))
		chunks = append(chunks, db.Chunk{
			ID:         hex.EncodeToString(sum[:])[:24],
			Namespace:  doc.Namespace,
			DocumentID: doc.ID,
			Position:   i,
			Content:    text,
			Type:       "text",
			Metadata:   map[string]any{},
			Vector:     embeddings[i],
			CreatedAt:  now,
		})
	}
	if ok, err := advance(db.DocumentIndexing, "queued"); err != nil {
		return fail(err)
	} else if !ok {
		return 1, nil
	}
	if err := p.DB.ReplaceChunks(doc.Namespace, doc.ID, chunks); err != nil {
		return fail(err)
	}

	if doc.ResetMemories {
		if err := p.DB.DeleteMemoriesByDocument(doc.Namespace, doc.ID); err != nil {
			return fail(err)
		}
		if ok, err := p.DB.ClearResetMemories(doc.Namespace, doc.ID); err != nil {
			return fail(err)
		} else if !ok {
			return 1, nil
		}
	}

	if doc.TaskType != "superrag" {
		if ok, err := advance(db.DocumentIndexing, "dreaming"); err != nil {
			return fail(err)
		} else if !ok {
			return 1, nil
		}
		existing, err := p.DB.GetMemoriesByDocument(doc.Namespace, doc.ID, 1000)
		if err != nil {
			return fail(err)
		}
		seen := make(map[string]bool, len(existing))
		for _, m := range existing {
			seen[m.Content] = true
		}
		candidates := extractMemories(doc.Content)
		fresh := make([]string, 0, len(candidates))
		for _, text := range candidates {
			if seen[text] {
				continue
			}
			seen[text] = true
			fresh = append(fresh, text)
		}
		if len(fresh) > 0 {
			vecs, err := p.Provider.GenerateEmbeddings(fresh)
			if err != nil {
				return fail(err)
			}
			if len(vecs) != len(fresh) {
				return fail(fmt.Errorf("embedding count mismatch: got %d for %d memories", len(vecs), len(fresh)))
			}
			records := make([]db.MemoryRecord, 0, len(fresh))
			for i, text := range fresh {
				records = append(records, db.MemoryRecord{
					ID:         GenerateUUID(),
					Namespace:  doc.Namespace,
					DocumentID: doc.ID,
					Content:    text,
					Metadata:   memoryMetadata(doc),
					Vector:     vecs[i],
					IsLatest:   true,
					Version:    1,
					CreatedAt:  now,
					UpdatedAt:  now,
				})
			}
			if err := p.DB.InsertMemories(records); err != nil {
				return fail(err)
			}
		}
	}

	if ok, err := advance(db.DocumentDone, db.DocumentDone); err != nil {
		return fail(err)
	} else if !ok {
		return 1, nil
	}
	_ = p.DB.TouchNamespace(doc.Namespace)
	return 1, nil
}

// memoryMetadata copies the document metadata and attaches the temporal
// context derived from the document date.
func memoryMetadata(doc db.Document) map[string]any {
	out := make(map[string]any, len(doc.Metadata)+1)
	for k, v := range doc.Metadata {
		out[k] = v
	}
	if doc.Date == "" {
		return out
	}
	out["temporalContext"] = map[string]any{
		"documentDate": doc.Date,
		"eventDate":    nil,
	}
	if t, ok := parseDocumentDate(doc.Date); ok {
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		out["temporalEventStartMs"] = start.UnixMilli()
		out["temporalEventEndMs"] = start.Add(24 * time.Hour).UnixMilli()
	}
	return out
}

func parseDocumentDate(value string) (time.Time, bool) {
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// splitUnits splits content at line breaks and sentence terminators.
func splitUnits(content string) []string {
	var out []string
	var b strings.Builder
	for i := 0; i < len(content); i++ {
		c := content[i]
		b.WriteByte(c)
		if c == '\n' {
			out = append(out, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		if (c == '.' || c == '!' || c == '?') && (i+1 == len(content) || content[i+1] == ' ') {
			out = append(out, strings.TrimSpace(b.String()))
			b.Reset()
			if i+1 < len(content) && content[i+1] == ' ' {
				i++
			}
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// chunkText groups sentences into chunks of at most 1000 runes.
func chunkText(content string) []string {
	units := splitUnits(content)
	chunks := make([]string, 0, 8)
	var current []rune
	flush := func() {
		if len(current) > 0 {
			chunks = append(chunks, strings.TrimSpace(string(current)))
			current = nil
		}
	}
	for _, unit := range units {
		runes := []rune(unit)
		if len(runes) < 15 {
			continue
		}
		if len(runes) > 1000 {
			flush()
			for len(runes) > 1000 {
				chunks = append(chunks, string(runes[:1000]))
				runes = runes[1000:]
			}
			if len(runes) > 0 {
				chunks = append(chunks, string(runes))
			}
			continue
		}
		if len(current)+len(runes)+1 > 1000 {
			flush()
		}
		if len(current) > 0 {
			current = append(current, ' ')
		}
		current = append(current, runes...)
	}
	flush()

	if len(chunks) == 0 {
		fallback := strings.TrimSpace(content)
		if len([]rune(fallback)) > 1000 {
			fallback = string([]rune(fallback)[:1000])
		}
		chunks = append(chunks, fallback)
	}
	if len(chunks) > 50 {
		chunks = chunks[:50]
	}
	return chunks
}

// extractMemories returns up to 20 substantial sentences as memories.
func extractMemories(content string) []string {
	out := make([]string, 0, 20)
	seen := map[string]bool{}
	for _, unit := range splitUnits(content) {
		if len([]rune(unit)) < 15 || seen[unit] {
			continue
		}
		seen[unit] = true
		out = append(out, unit)
		if len(out) == 20 {
			break
		}
	}
	return out
}
