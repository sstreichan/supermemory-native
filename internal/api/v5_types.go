package api

import "time"

type v5DocSystem struct {
	Status         string    `json:"status"`
	DreamingStatus string    `json:"dreamingStatus"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type v5Chunk struct {
	ID         string         `json:"id"`
	DocumentID string         `json:"documentId,omitempty"`
	Position   int            `json:"position"`
	Content    string         `json:"content"`
	Type       string         `json:"type"`
	Metadata   map[string]any `json:"metadata"`
	System     struct {
		CreatedAt time.Time `json:"createdAt"`
	} `json:"system"`
}

type v5Memory struct {
	ID          string         `json:"id"`
	Memory      string         `json:"memory"`
	Metadata    map[string]any `json:"metadata"`
	IsStatic    bool           `json:"isStatic"`
	IsInference bool           `json:"isInference"`
	IsLatest    bool           `json:"isLatest"`
	IsForgotten bool           `json:"isForgotten"`
	Version     int            `json:"version"`
	System      struct {
		CreatedAt time.Time `json:"createdAt"`
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"system"`
}

type v5Doc struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Summary  string         `json:"summary"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
	System   v5DocSystem    `json:"system"`
	Chunks   []v5Chunk      `json:"chunks"`
	Memories []v5Memory     `json:"memories"`
}

type v5DocListItem struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Summary  string         `json:"summary"`
	Metadata map[string]any `json:"metadata"`
	URL      *string        `json:"url"`
	System   v5DocSystem    `json:"system"`
}

type v5SearchHit struct {
	ID          string         `json:"id"`
	Memory      *string        `json:"memory,omitempty"`
	Chunk       *string        `json:"chunk,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	Similarity  float64        `json:"similarity"`
	IsLatest    bool           `json:"isLatest"`
	IsInference bool           `json:"isInference"`
	System      struct {
		CreatedAt time.Time `json:"createdAt"`
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"system"`
	Included *v5Included `json:"included,omitempty"`
}

type v5Included struct {
	Document *v5IncludedDocument `json:"document,omitempty"`
	Related  *v5Related          `json:"related,omitempty"`
}

type v5IncludedDocument struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Summary  string         `json:"summary"`
	Metadata map[string]any `json:"metadata"`
	System   v5DocSystem    `json:"system"`
}

type v5Related struct {
	Parents  []v5RelatedMemory `json:"parents"`
	Children []v5RelatedMemory `json:"children"`
	Siblings []v5RelatedMemory `json:"siblings"`
}

type v5RelatedMemory struct {
	ID       string         `json:"id"`
	Memory   string         `json:"memory"`
	Relation string         `json:"relation"`
	Metadata map[string]any `json:"metadata"`
	Version  int            `json:"version"`
	System   struct {
		CreatedAt time.Time `json:"createdAt"`
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"system"`
}

type v5MemoryWithIncluded struct {
	v5Memory
	Included *v5Included `json:"included,omitempty"`
}

type v5MemoryMatch struct {
	ID     string `json:"id"`
	Memory string `json:"memory"`
}

type v5Pagination struct {
	CurrentPage int `json:"currentPage"`
	Limit       int `json:"limit"`
	TotalItems  int `json:"totalItems"`
	TotalPages  int `json:"totalPages"`
}

func v5Pages(total, limit int) int {
	if total == 0 {
		return 0
	}
	return (total + limit - 1) / limit
}
