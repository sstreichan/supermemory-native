package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

const (
	rrfK            = 60.0
	llmRerankLimit  = 10
	profileCacheTTL = 5 * time.Minute
)

// tokenize lowercases and splits on anything that is not a letter or digit.
func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
}

// lexicalScores ranks candidate texts with Okapi BM25.
func lexicalScores(query string, hits []db.Hit) map[string]float64 {
	scores := make(map[string]float64, len(hits))
	terms := uniqueTerms(tokenize(query))
	if len(terms) == 0 || len(hits) == 0 {
		return scores
	}
	docs := make([][]string, len(hits))
	df := make(map[string]int, len(terms))
	totalLen := 0
	for i, h := range hits {
		tokens := tokenize(h.Text)
		docs[i] = tokens
		totalLen += len(tokens)
		seen := make(map[string]bool, len(tokens))
		for _, tok := range tokens {
			if !seen[tok] {
				seen[tok] = true
				df[tok]++
			}
		}
	}
	avgLen := float64(totalLen) / float64(len(hits))
	if avgLen == 0 {
		return scores
	}
	const k1, b = 1.2, 0.75
	n := float64(len(hits))
	for i, h := range hits {
		tf := make(map[string]int, len(docs[i]))
		for _, tok := range docs[i] {
			tf[tok]++
		}
		dl := float64(len(docs[i]))
		var score float64
		for _, term := range terms {
			f := float64(tf[term])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (n-float64(df[term])+0.5)/(float64(df[term])+0.5))
			score += idf * (f * (k1 + 1)) / (f + k1*(1-b+b*dl/avgLen))
		}
		if score > 0 {
			scores[h.ID] = score
		}
	}
	return scores
}

func uniqueTerms(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// fuseRRF combines the vector ranking with a lexical ranking. The returned
// slice is deterministic: RRF score, then vector similarity, then id.
func fuseRRF(vectorHits []db.Hit, lexical []db.Hit, limit int) []db.Hit {
	if len(vectorHits) == 0 {
		return []db.Hit{}
	}
	type scored struct {
		hit   db.Hit
		score float64
	}
	byID := make(map[string]*scored, len(vectorHits))
	for i, h := range vectorHits {
		byID[h.ID] = &scored{hit: h, score: 1.0 / (rrfK + float64(i+1))}
	}
	for i, h := range lexical {
		if s, ok := byID[h.ID]; ok {
			s.score += 1.0 / (rrfK + float64(i+1))
		}
	}
	out := make([]scored, 0, len(byID))
	for _, s := range byID {
		out = append(out, *s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		if out[i].hit.Similarity != out[j].hit.Similarity {
			return out[i].hit.Similarity > out[j].hit.Similarity
		}
		return out[i].hit.ID < out[j].hit.ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	hits := make([]db.Hit, len(out))
	for i := range out {
		hits[i] = out[i].hit
	}
	return hits
}

// lexicalRanking returns candidates that actually match the query, best first.
func lexicalRanking(query string, hits []db.Hit) []db.Hit {
	scores := lexicalScores(query, hits)
	out := make([]db.Hit, 0, len(scores))
	for _, h := range hits {
		if scores[h.ID] > 0 {
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if scores[out[i].ID] != scores[out[j].ID] {
			return scores[out[i].ID] > scores[out[j].ID]
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (h *Handler) rewriteQuery(ctx context.Context, query string) string {
	if h.LLM == nil {
		return query
	}
	out, err := h.LLM.chat(ctx,
		"Rewrite the user's search query for retrieval. Keep names, numbers and meaning. Reply with the rewritten query only, on one line.",
		query)
	if err != nil {
		return query
	}
	out = strings.TrimSpace(strings.Trim(out, "\"'`"))
	if out == "" || len([]rune(out)) > 1000 {
		return query
	}
	return out
}

// rerankWithLLM asks the model for a listwise order of the top candidates.
// Any malformed response keeps the deterministic input order.
func (h *Handler) rerankWithLLM(ctx context.Context, query string, hits []db.Hit, limit int) []db.Hit {
	if h.LLM == nil {
		return hits
	}
	n := len(hits)
	if limit > 0 && n > limit {
		n = limit
	}
	if n > llmRerankLimit {
		n = llmRerankLimit
	}
	if n < 2 {
		return hits
	}
	head := hits[:n]
	var b strings.Builder
	for _, hit := range head {
		fmt.Fprintf(&b, "ID: %s\n%s\n\n", hit.ID, truncateRunes(hit.Text, 400))
	}
	out, err := h.LLM.chat(ctx,
		"Rank the passages by how well they answer the query. Reply with a JSON array of the IDs in best-first order, using every ID exactly once. No prose.",
		"Query: "+truncateRunes(query, 500)+"\n\n"+b.String())
	if err != nil {
		return hits
	}
	ids, err := parseStringArray(out)
	if err != nil || len(ids) != n {
		return hits
	}
	pos := make(map[string]int, n)
	for i, id := range ids {
		if _, dup := pos[id]; dup {
			return hits
		}
		pos[id] = i
	}
	for _, hit := range head {
		if _, ok := pos[hit.ID]; !ok {
			return hits
		}
	}
	reordered := make([]db.Hit, 0, len(hits))
	for _, id := range ids {
		for _, hit := range head {
			if hit.ID == id {
				reordered = append(reordered, hit)
				break
			}
		}
	}
	reordered = append(reordered, hits[n:]...)
	return reordered
}

// staticProfile synthesizes durable facts with the optional LLM. Results are
// cached per namespace and filter; without an LLM the profile stays empty.
func (h *Handler) staticProfile(ns string, filter *db.Filter, mems []db.MemoryRecord) []string {
	if h.LLM == nil || len(mems) == 0 {
		return []string{}
	}
	key := profileCacheKey(ns, filter)
	h.profileMu.Lock()
	if h.profileCache != nil {
		if entry, ok := h.profileCache[key]; ok && time.Since(entry.at) < profileCacheTTL {
			h.profileMu.Unlock()
			return entry.facts
		}
	}
	h.profileMu.Unlock()

	lines := make([]string, 0, 50)
	for _, m := range mems {
		if len(lines) == 50 {
			break
		}
		lines = append(lines, "- "+m.Content)
	}
	out, err := h.LLM.chat(context.Background(),
		"Extract durable, user-relevant facts from the memories. Reply with a JSON array of at most 10 short factual strings. No prose.",
		strings.Join(lines, "\n"))
	if err != nil {
		return []string{}
	}
	rawFacts, err := parseStringArray(out)
	if err != nil {
		return []string{}
	}
	facts := make([]string, 0, len(rawFacts))
	for _, f := range rawFacts {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		facts = append(facts, truncateRunes(f, 300))
		if len(facts) == 10 {
			break
		}
	}

	h.profileMu.Lock()
	if h.profileCache == nil {
		h.profileCache = map[string]profileCacheEntry{}
	}
	h.profileCache[key] = profileCacheEntry{facts: facts, at: time.Now()}
	h.profileMu.Unlock()
	return facts
}

type profileCacheEntry struct {
	facts []string
	at    time.Time
}

func profileCacheKey(ns string, filter *db.Filter) string {
	raw, _ := json.Marshal(filter)
	return ns + "|" + string(raw)
}

// parseStringArray accepts a bare JSON array, optionally wrapped in a markdown
// code fence or surrounded by prose.
func parseStringArray(raw string) ([]string, error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// v5RelatedRecords converts the structural parent/child/sibling sets into the
// public included.related shape.
func v5RelatedRecords(excludeID string, parents, children, siblings []db.MemoryRecord, limit int) *v5Related {
	out := &v5Related{
		Parents:  make([]v5RelatedMemory, 0),
		Children: make([]v5RelatedMemory, 0),
		Siblings: make([]v5RelatedMemory, 0),
	}
	add := func(dst *[]v5RelatedMemory, in []db.MemoryRecord) {
		for _, m := range in {
			if m.ID == excludeID || m.IsForgotten || len(*dst) == limit {
				continue
			}
			*dst = append(*dst, v5RelatedMemoryFrom(m))
		}
	}
	add(&out.Parents, parents)
	add(&out.Children, children)
	add(&out.Siblings, siblings)
	return out
}
