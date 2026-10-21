package db

import (
	"sort"
)

// RetrievalCandidates returns every chunk and/or memory in a namespace with
// its cosine similarity to vec. Unlike SearchV5 it applies no threshold and no
// limit so callers can fuse vector and lexical rankings before truncating.
func (s *SqliteDB) RetrievalCandidates(ns string, vec []float32, mode string, includeForgotten bool, filter *Filter) ([]Hit, error) {
	hits := make([]Hit, 0)

	if mode == "hybrid" || mode == "memories" {
		where := "container_tag = ?"
		if !includeForgotten {
			where += " AND is_forgotten = 0"
		}
		rows, err := s.db.Query("SELECT "+memoryColumns+", norm FROM memories WHERE "+where, ns)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var m MemoryRecord
			var metadata string
			var vectorRaw []byte
			var forgotten, inference, static, latest int
			var created, updated any
			var norm float64
			if err := rows.Scan(&m.ID, &m.Namespace, &m.DocumentID, &m.Content, &metadata, &vectorRaw,
				&forgotten, &inference, &static, &latest, &m.Version, &created, &updated, &norm); err != nil {
				rows.Close()
				return nil, err
			}
			m.Metadata = decodeMap(metadata)
			if !matchesFilter(m.Metadata, filter) {
				continue
			}
			sim, ok := cosine(vec, mustDecode(vectorRaw), norm)
			if !ok {
				continue
			}
			hits = append(hits, Hit{
				Kind: "memory", ID: m.ID, Text: m.Content, DocumentID: m.DocumentID,
				Metadata: m.Metadata, Similarity: clampSimilarity(sim),
				IsLatest: latest != 0 && forgotten == 0, IsInference: inference != 0,
				CreatedAt: v5Time(created), UpdatedAt: v5Time(updated),
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	if mode == "hybrid" || mode == "chunks" {
		rows, err := s.db.Query(`
			SELECT id, document_id, content, metadata, vector, norm, created_at
			FROM chunks WHERE namespace = ?`, ns)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, docID, content, metadata string
			var vectorRaw []byte
			var norm float64
			var created any
			if err := rows.Scan(&id, &docID, &content, &metadata, &vectorRaw, &norm, &created); err != nil {
				rows.Close()
				return nil, err
			}
			meta := decodeMap(metadata)
			if !matchesFilter(meta, filter) {
				continue
			}
			sim, ok := cosine(vec, mustDecode(vectorRaw), norm)
			if !ok {
				continue
			}
			t := v5Time(created)
			hits = append(hits, Hit{
				Kind: "chunk", ID: id, Text: content, DocumentID: docID,
				Metadata: meta, Similarity: clampSimilarity(sim),
				IsLatest: true, CreatedAt: t, UpdatedAt: t,
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Similarity == hits[j].Similarity {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Similarity > hits[j].Similarity
	})
	return hits, nil
}

// RelatedMemories splits memories in the namespace into older and newer
// neighbours of memoryID by embedding similarity, plus the memories derived
// from the same document. It is a structural relation, not dreaming output.
func (s *SqliteDB) RelatedMemories(ns, memoryID string, limit int) (parents, children, siblings []MemoryRecord, err error) {
	parents = make([]MemoryRecord, 0)
	children = make([]MemoryRecord, 0)
	siblings = make([]MemoryRecord, 0)
	if limit <= 0 {
		return parents, children, siblings, nil
	}
	self, ok, err := s.GetMemory(ns, memoryID)
	if err != nil {
		return parents, children, siblings, err
	}
	if !ok || self.IsForgotten {
		return parents, children, siblings, nil
	}

	rows, err := s.db.Query("SELECT "+memoryColumns+", norm FROM memories WHERE container_tag = ? AND id <> ? AND is_forgotten = 0", ns, memoryID)
	if err != nil {
		return parents, children, siblings, err
	}
	type neighbour struct {
		rec MemoryRecord
		sim float32
	}
	older := make([]neighbour, 0)
	newer := make([]neighbour, 0)
	for rows.Next() {
		var m MemoryRecord
		var metadata string
		var vectorRaw []byte
		var forgotten, inference, static, latest int
		var created, updated any
		var norm float64
		if err := rows.Scan(&m.ID, &m.Namespace, &m.DocumentID, &m.Content, &metadata, &vectorRaw,
			&forgotten, &inference, &static, &latest, &m.Version, &created, &updated, &norm); err != nil {
			rows.Close()
			return parents, children, siblings, err
		}
		m.Metadata = decodeMap(metadata)
		m.IsForgotten = forgotten != 0
		m.IsInference = inference != 0
		m.IsStatic = static != 0
		m.IsLatest = latest != 0
		m.CreatedAt = v5Time(created)
		m.UpdatedAt = v5Time(updated)

		if m.DocumentID != "" && m.DocumentID == self.DocumentID && len(siblings) < limit {
			siblings = append(siblings, m)
		}
		if m.ID == self.ID || len(self.Vector) == 0 || m.IsForgotten {
			continue
		}
		sim, ok := cosine(self.Vector, mustDecode(vectorRaw), norm)
		if !ok || sim < 0.35 {
			continue
		}
		n := neighbour{rec: m, sim: sim}
		switch {
		case m.CreatedAt.Before(self.CreatedAt):
			older = append(older, n)
		case m.CreatedAt.After(self.CreatedAt):
			newer = append(newer, n)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return parents, children, siblings, err
	}
	rows.Close()

	bySimilarity := func(in []neighbour) []neighbour {
		sort.SliceStable(in, func(i, j int) bool {
			if in[i].sim == in[j].sim {
				if in[i].rec.CreatedAt.Equal(in[j].rec.CreatedAt) {
					return in[i].rec.ID < in[j].rec.ID
				}
				return in[i].rec.CreatedAt.After(in[j].rec.CreatedAt)
			}
			return in[i].sim > in[j].sim
		})
		return in
	}
	for _, n := range bySimilarity(older) {
		if len(parents) == limit {
			break
		}
		parents = append(parents, n.rec)
	}
	for _, n := range bySimilarity(newer) {
		if len(children) == limit {
			break
		}
		children = append(children, n.rec)
	}
	return parents, children, siblings, nil
}
