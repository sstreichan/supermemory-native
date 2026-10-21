package api

import (
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

func v5System(d db.Document) v5DocSystem {
	return v5DocSystem{
		Status:         d.Status,
		DreamingStatus: d.DreamingStatus,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

func v5DocFromRecord(d db.Document, chunks []db.Chunk, mems []db.MemoryRecord) v5Doc {
	out := v5Doc{
		ID:       d.ID,
		Type:     "text",
		Title:    d.Title,
		Summary:  d.Summary,
		Content:  d.Content,
		Metadata: d.Metadata,
		System:   v5System(d),
		Chunks:   make([]v5Chunk, 0, len(chunks)),
		Memories: make([]v5Memory, 0, len(mems)),
	}
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	for _, c := range chunks {
		out.Chunks = append(out.Chunks, v5ChunkFromRecord(c))
	}
	for _, m := range mems {
		out.Memories = append(out.Memories, v5MemoryFromRecord(m))
	}
	return out
}

func v5DocListItemFromRecord(d db.Document) v5DocListItem {
	meta := d.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	return v5DocListItem{
		ID:       d.ID,
		Type:     "text",
		Title:    d.Title,
		Summary:  d.Summary,
		Metadata: meta,
		System:   v5System(d),
	}
}

func v5ChunkFromRecord(c db.Chunk) v5Chunk {
	meta := c.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	out := v5Chunk{
		ID:         c.ID,
		DocumentID: c.DocumentID,
		Position:   c.Position,
		Content:    c.Content,
		Type:       c.Type,
		Metadata:   meta,
	}
	out.System.CreatedAt = c.CreatedAt
	return out
}

func v5MemoryFromRecord(m db.MemoryRecord) v5Memory {
	meta := m.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	out := v5Memory{
		ID:          m.ID,
		Memory:      m.Content,
		Metadata:    meta,
		IsStatic:    m.IsStatic,
		IsInference: m.IsInference,
		IsLatest:    m.IsLatest,
		IsForgotten: m.IsForgotten,
		Version:     m.Version,
	}
	out.System.CreatedAt = m.CreatedAt
	out.System.UpdatedAt = m.UpdatedAt
	return out
}

func v5IncludedDoc(d db.Document) *v5IncludedDocument {
	meta := d.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	return &v5IncludedDocument{
		ID:       d.ID,
		Type:     "text",
		Title:    d.Title,
		Summary:  d.Summary,
		Metadata: meta,
		System:   v5System(d),
	}
}

func v5RelatedMemoryFrom(m db.MemoryRecord) v5RelatedMemory {
	meta := m.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	out := v5RelatedMemory{
		ID:       m.ID,
		Memory:   m.Content,
		Relation: "derives",
		Metadata: meta,
		Version:  m.Version,
	}
	out.System.CreatedAt = m.CreatedAt
	out.System.UpdatedAt = m.UpdatedAt
	return out
}

func v5RelatedFrom(excludeID string, siblings []db.MemoryRecord, limit int) *v5Related {
	out := &v5Related{
		Parents:  make([]v5RelatedMemory, 0),
		Children: make([]v5RelatedMemory, 0),
		Siblings: make([]v5RelatedMemory, 0),
	}
	for _, m := range siblings {
		if m.ID == excludeID || m.IsForgotten {
			continue
		}
		out.Siblings = append(out.Siblings, v5RelatedMemoryFrom(m))
		if len(out.Siblings) == limit {
			break
		}
	}
	return out
}

// v5HitToWire converts a DB hit into the public search result shape.
func v5HitToWire(h db.Hit) v5SearchHit {
	meta := h.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	out := v5SearchHit{
		ID:          h.ID,
		Metadata:    meta,
		Similarity:  float64(h.Similarity),
		IsLatest:    h.IsLatest,
		IsInference: h.IsInference,
	}
	out.System.CreatedAt = h.CreatedAt
	out.System.UpdatedAt = h.UpdatedAt
	if h.Kind == "chunk" {
		text := h.Text
		out.Chunk = &text
	} else {
		text := h.Text
		out.Memory = &text
	}
	return out
}

func v5Now() time.Time { return time.Now().UTC() }
