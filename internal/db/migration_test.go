package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// A database created before file ingestion must keep its documents and gain
// the three file columns without losing data.
func TestDocumentsFileColumnsMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	_, err = raw.Exec(`
		CREATE TABLE documents (
			namespace TEXT NOT NULL,
			id TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			metadata TEXT NOT NULL DEFAULT '{}',
			grp TEXT NOT NULL DEFAULT '{}',
			supporting_context TEXT NOT NULL DEFAULT '',
			source_date TEXT NOT NULL DEFAULT '',
			task_type TEXT NOT NULL DEFAULT 'memory',
			dreaming TEXT NOT NULL DEFAULT 'dynamic',
			status TEXT NOT NULL DEFAULT 'queued',
			dreaming_status TEXT NOT NULL DEFAULT 'queued',
			reset_memories INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (namespace, id)
		);
		INSERT INTO documents (namespace, id, content, status, created_at, updated_at)
		VALUES ('legacy', 'doc1', 'old text', 'done', '2026-01-02 03:04:05', '2026-01-02 03:04:05');
	`)
	if err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	raw.Close()

	sdb, err := NewSqliteDB(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer sdb.Close()

	doc, found, err := sdb.GetDocument("legacy", "doc1")
	if err != nil || !found {
		t.Fatalf("get migrated doc: found=%v err=%v", found, err)
	}
	if doc.Content != "old text" || doc.Status != "done" {
		t.Fatalf("migrated doc lost data: %+v", doc)
	}
	if doc.FileName != "" || doc.MIMEType != "" || doc.FileType != "" {
		t.Fatalf("file columns should default empty: %+v", doc)
	}

	fresh := Document{
		Namespace: "legacy", ID: "doc2", Content: "new", Status: "queued",
		FileName: "a.txt", MIMEType: "text/plain", FileType: "text",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := sdb.UpsertDocument(fresh); err != nil {
		t.Fatalf("upsert with file columns: %v", err)
	}
	got, _, err := sdb.GetDocument("legacy", "doc2")
	if err != nil || got.FileName != "a.txt" || got.MIMEType != "text/plain" || got.FileType != "text" {
		t.Fatalf("file columns not persisted: %+v err=%v", got, err)
	}
}
