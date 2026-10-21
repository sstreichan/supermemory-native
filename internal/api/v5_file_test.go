package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// doV5File sends a multipart/form-data request. An empty fileName omits the
// file part entirely.
func doV5File(t *testing.T, h *Handler, method, path string, fields map[string]string, fileName string, data []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := mw.WriteField(k, fields[k]); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	if fileName != "" {
		fw, err := mw.CreateFormFile("file", fileName)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	out := map[string]any{}
	if rr.Body.Len() > 0 {
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode response: %v (body %q)", err, rr.Body.String())
		}
	}
	return rr.Code, out
}

func v5ErrorItem(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	errs, ok := body["error"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("missing error array: %v", body)
	}
	item, ok := errs[0].(map[string]any)
	if !ok {
		t.Fatalf("malformed error item: %v", errs[0])
	}
	return item
}

func v5ErrorPath(t *testing.T, item map[string]any) []string {
	t.Helper()
	raw, ok := item["path"].([]any)
	if !ok {
		t.Fatalf("missing error path: %v", item)
	}
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		s, _ := part.(string)
		out = append(out, s)
	}
	return out
}

func wantErrorCode(t *testing.T, body map[string]any, code string, path ...string) {
	t.Helper()
	item := v5ErrorItem(t, body)
	if item["code"] != code {
		t.Fatalf("error code = %v, want %q (%v)", item["code"], code, item)
	}
	if len(path) > 0 {
		got := v5ErrorPath(t, item)
		if strings.Join(got, ".") != strings.Join(path, ".") {
			t.Fatalf("error path = %v, want %v", got, path)
		}
	}
}

func getDocJSON(t *testing.T, h *Handler, ns, id, query string) (int, map[string]any) {
	t.Helper()
	return doV5(t, h, http.MethodGet, "/ns/"+ns+"/document/"+id+query, nil)
}

// minimalPDF builds a valid one-page PDF with correct xref offsets.
func minimalPDF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	var offsets []int
	writeObj := func(body string) {
		offsets = append(offsets, buf.Len())
		buf.WriteString(body)
	}
	buf.WriteString("%PDF-1.4\n")
	writeObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	writeObj("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	writeObj("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n")
	writeObj("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	content := "BT /F1 24 Tf 72 720 Td (Zeppelin-4417 sample PDF text) Tj ET"
	writeObj(fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))
	xref := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return buf.Bytes()
}

func TestV5FileTextUpload(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "file:text"

	fields := map[string]string{
		"metadata":          `{"suite":"pref","n":1}`,
		"group":             `{"collection":"notes"}`,
		"date":              "2025-01-15",
		"supportingContext": "uploaded via file",
	}
	code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", fields, "notes.txt",
		[]byte("My favorite color is teal.\nSecond line about the zeppelin project."))
	if code != http.StatusOK {
		t.Fatalf("POST file = %d %v", code, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", body)
	}
	if body["status"] != "queued" {
		t.Fatalf("status = %v, want queued", body["status"])
	}

	rec, found, err := h.Engine.DB.GetDocument(ns, id)
	if err != nil || !found {
		t.Fatalf("document not persisted: found=%v err=%v", found, err)
	}
	if rec.FileName != "notes.txt" || rec.FileType != "text" {
		t.Fatalf("file metadata = %q/%q", rec.FileName, rec.FileType)
	}
	if rec.MIMEType != "text/plain" {
		t.Fatalf("mime = %q", rec.MIMEType)
	}
	if rec.SupportingContext != "uploaded via file" {
		t.Fatalf("supportingContext = %q", rec.SupportingContext)
	}
	if rec.Grp["collection"] != "notes" {
		t.Fatalf("group = %v", rec.Grp)
	}
	if rec.Metadata["suite"] != "pref" {
		t.Fatalf("metadata = %v", rec.Metadata)
	}
	temporal, _ := rec.Metadata["temporalContext"].(map[string]any)
	if temporal["documentDate"] != "2025-01-15" {
		t.Fatalf("temporalContext = %v", rec.Metadata["temporalContext"])
	}

	code, doc := getDocJSON(t, h, ns, id, "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d %v", code, doc)
	}
	if doc["title"] != "notes.txt" {
		t.Fatalf("title = %v, want uploaded filename", doc["title"])
	}
	system, _ := doc["system"].(map[string]any)
	if system["status"] != "queued" {
		t.Fatalf("status before processing = %v", system["status"])
	}

	processAll(t, h)

	code, doc = getDocJSON(t, h, ns, id, "?include=chunks,memories")
	if code != http.StatusOK {
		t.Fatalf("GET after process = %d %v", code, doc)
	}
	system, _ = doc["system"].(map[string]any)
	if system["status"] != "done" {
		t.Fatalf("status after processing = %v", system["status"])
	}
	chunks, _ := doc["chunks"].([]any)
	if len(chunks) == 0 {
		t.Fatalf("no chunks after processing: %v", doc)
	}
	if !strings.Contains(doc["content"].(string), "teal") {
		t.Fatalf("content = %v", doc["content"])
	}
	if doc["summary"] != "My favorite color is teal.\nSecond line about the zeppelin project." {
		t.Fatalf("summary = %v", doc["summary"])
	}

	code, nsBody := doV5(t, h, http.MethodGet, "/ns/"+ns, nil)
	if code != http.StatusOK {
		t.Fatalf("GET namespace = %d %v", code, nsBody)
	}
	if nsBody["namespace"] != ns {
		t.Fatalf("namespace = %v", nsBody["namespace"])
	}
	nsRec, found, err := h.Engine.DB.GetNamespace(ns)
	if err != nil || !found {
		t.Fatalf("namespace not persisted: found=%v err=%v", found, err)
	}
	if nsRec.UpdatedAt.Before(nsRec.CreatedAt) {
		t.Fatalf("namespace updatedAt %v predates createdAt %v", nsRec.UpdatedAt, nsRec.CreatedAt)
	}
}

func TestV5FilePDFUpload(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "file:pdf"
	pdfData := minimalPDF(t)

	t.Run("explicit fileType", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file",
			map[string]string{"fileType": "pdf", "mimeType": "application/pdf"}, "report.pdf", pdfData)
		if code != http.StatusOK {
			t.Fatalf("POST pdf = %d %v", code, body)
		}
		code, doc := getDocJSON(t, h, ns, body["id"].(string), "")
		if code != http.StatusOK {
			t.Fatalf("GET = %d %v", code, doc)
		}
		content, _ := doc["content"].(string)
		if !strings.Contains(content, "Zeppelin-4417") {
			t.Fatalf("extracted content = %q", content)
		}
		if doc["title"] != "report.pdf" {
			t.Fatalf("title = %v", doc["title"])
		}
	})

	t.Run("inferred from extension", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", nil, "inferred.pdf", pdfData)
		if code != http.StatusOK {
			t.Fatalf("POST pdf = %d %v", code, body)
		}
		code, doc := getDocJSON(t, h, ns, body["id"].(string), "")
		if code != http.StatusOK {
			t.Fatalf("GET = %d %v", code, doc)
		}
		if !strings.Contains(doc["content"].(string), "Zeppelin-4417") {
			t.Fatalf("extracted content = %q", doc["content"])
		}
	})
}

func TestV5FileReplacePost(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "file:replace"

	code, created := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", nil, "first.txt",
		[]byte("OLD-MARKER-111 first upload body"))
	if code != http.StatusOK {
		t.Fatalf("create = %d %v", code, created)
	}
	id := created["id"].(string)
	processAll(t, h)

	code, replaced := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file/"+id, nil, "second.txt",
		[]byte("NEW-MARKER-222 complete replacement body"))
	if code != http.StatusOK {
		t.Fatalf("replace = %d %v", code, replaced)
	}
	if replaced["status"] != "queued" {
		t.Fatalf("replace status = %v", replaced["status"])
	}

	code, doc := getDocJSON(t, h, ns, id, "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d %v", code, doc)
	}
	content := doc["content"].(string)
	if strings.Contains(content, "OLD-MARKER-111") || !strings.Contains(content, "NEW-MARKER-222") {
		t.Fatalf("content was not replaced: %q", content)
	}
	if doc["title"] != "second.txt" {
		t.Fatalf("title = %v", doc["title"])
	}

	processAll(t, h)
	code, doc = getDocJSON(t, h, ns, id, "?include=chunks")
	chunks := doc["chunks"].([]any)
	if len(chunks) == 0 {
		t.Fatalf("no chunks after replacement: %v", doc)
	}
	for _, chunk := range chunks {
		text := chunk.(map[string]any)["content"].(string)
		if strings.Contains(text, "OLD-MARKER-111") {
			t.Fatalf("stale chunk survived replacement: %q", text)
		}
	}

	code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file/does-not-exist", nil, "x.txt", []byte("body"))
	if code != http.StatusNotFound || body["error"] != "Document not found" {
		t.Fatalf("replace unknown = %d %v", code, body)
	}
}

func TestV5FilePatch(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "file:patch"

	code, created := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file",
		map[string]string{"metadata": `{"suite":"initial"}`}, "v1.txt", []byte("PATCH-OLD-900 original text"))
	if code != http.StatusOK {
		t.Fatalf("create = %d %v", code, created)
	}
	id := created["id"].(string)
	processAll(t, h)

	before, _ := h.Engine.DB.GetChunks(ns, id)
	if len(before) == 0 {
		t.Fatal("expected chunks after processing")
	}

	// Metadata-only patch keeps the processing status and chunks.
	code, body := doV5File(t, h, http.MethodPatch, "/ns/"+ns+"/document/file/"+id,
		map[string]string{"metadata": `{"suite":"updated"}`}, "", nil)
	if code != http.StatusOK {
		t.Fatalf("patch metadata = %d %v", code, body)
	}
	if body["status"] != "done" {
		t.Fatalf("status after metadata patch = %v, want done", body["status"])
	}
	rec, _, _ := h.Engine.DB.GetDocument(ns, id)
	if rec.Metadata["suite"] != "updated" {
		t.Fatalf("metadata = %v", rec.Metadata)
	}
	if rec.Content != "PATCH-OLD-900 original text" {
		t.Fatalf("content changed on metadata patch: %q", rec.Content)
	}
	afterMeta, _ := h.Engine.DB.GetChunks(ns, id)
	if len(afterMeta) != len(before) {
		t.Fatalf("chunks changed on metadata patch: %d -> %d", len(before), len(afterMeta))
	}

	// Patch with a new file replaces the content and requeues.
	code, body = doV5File(t, h, http.MethodPatch, "/ns/"+ns+"/document/file/"+id,
		map[string]string{"supportingContext": "patched context"}, "v2.txt", []byte("PATCH-NEW-901 replacement text"))
	if code != http.StatusOK {
		t.Fatalf("patch file = %d %v", code, body)
	}
	if body["status"] != "queued" {
		t.Fatalf("status after file patch = %v, want queued", body["status"])
	}
	rec, _, _ = h.Engine.DB.GetDocument(ns, id)
	if !strings.Contains(rec.Content, "PATCH-NEW-901") || strings.Contains(rec.Content, "PATCH-OLD-900") {
		t.Fatalf("content after file patch = %q", rec.Content)
	}
	if rec.FileName != "v2.txt" || rec.SupportingContext != "patched context" {
		t.Fatalf("file/context after patch = %q/%q", rec.FileName, rec.SupportingContext)
	}

	processAll(t, h)
	chunks, _ := h.Engine.DB.GetChunks(ns, id)
	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "PATCH-OLD-900") {
			t.Fatalf("stale chunk survived: %q", chunk.Content)
		}
	}

	// Empty patch and group without a file are rejected.
	code, body = doV5File(t, h, http.MethodPatch, "/ns/"+ns+"/document/file/"+id, nil, "", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("empty patch = %d %v", code, body)
	}
	wantErrorCode(t, body, "custom")

	code, body = doV5File(t, h, http.MethodPatch, "/ns/"+ns+"/document/file/"+id,
		map[string]string{"group": `{"a":"b"}`}, "", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("group-only patch = %d %v", code, body)
	}
	wantErrorCode(t, body, "custom")

	code, body = doV5File(t, h, http.MethodPatch, "/ns/"+ns+"/document/file/nope",
		map[string]string{"metadata": `{"a":1}`}, "", nil)
	if code != http.StatusNotFound || body["error"] != "Document not found" {
		t.Fatalf("patch unknown = %d %v", code, body)
	}
}

func TestV5FileValidation(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "file:validation"

	t.Run("missing file", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", map[string]string{"metadata": `{}`}, "", nil)
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_type", "file")
	})

	t.Run("unsupported image type", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file",
			map[string]string{"fileType": "image"}, "photo.png", []byte("\x89PNG\r\n\x1a\n0000"))
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_value", "fileType")
		if msg, _ := v5ErrorItem(t, body)["message"].(string); !strings.Contains(msg, "image") {
			t.Fatalf("message does not name the type: %q", msg)
		}
	})

	t.Run("image inferred from extension", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", nil, "photo.jpg", []byte{0xff, 0xd8, 0xff, 0xe0, 0x00})
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_value", "fileType")
	})

	t.Run("unknown binary", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", nil, "blob.bin",
			[]byte{0x00, 0xff, 0xfe, 0x80, 0x01, 0x02, 0x03, 0x04})
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_value", "file")
	})

	t.Run("unknown form field", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file",
			map[string]string{"bogus": "1"}, "x.txt", []byte("hello world, this is text"))
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "unrecognized_keys")
		keys, _ := v5ErrorItem(t, body)["keys"].([]any)
		if len(keys) != 1 || keys[0] != "bogus" {
			t.Fatalf("keys = %v", keys)
		}
	})

	t.Run("invalid metadata json", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file",
			map[string]string{"metadata": `[1,2]`}, "x.txt", []byte("hello world, this is text"))
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_type", "metadata")
	})

	t.Run("invalid date", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file",
			map[string]string{"date": "15.01.2025"}, "x.txt", []byte("hello world, this is text"))
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_value", "date")
	})

	t.Run("json body is not multipart", func(t *testing.T) {
		code, body := doV5(t, h, http.MethodPost, "/ns/"+ns+"/document/file", map[string]any{"content": "x"})
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_type", "file")
	})

	t.Run("over 25 MiB", func(t *testing.T) {
		big := bytes.Repeat([]byte("a"), v5MaxFileBytes+1)
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", nil, "big.txt", big)
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "too_big", "file")
	})

	t.Run("unreadable text does not create a document", func(t *testing.T) {
		code, body := doV5File(t, h, http.MethodPost, "/ns/"+ns+"/document/file", nil, "bad.txt",
			[]byte{0xff, 0xfe, 0x00, 0x81})
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_value", "file")
		code, listed := doV5(t, h, http.MethodPost, "/ns/"+ns+"/list/documents", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("list: %d %v", code, listed)
		}
		if total := listed["pagination"].(map[string]any)["totalItems"].(float64); total != 0 {
			t.Fatalf("failed upload must not create a document, total=%v", total)
		}
	})
}

func TestV5URLIngestion(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "url:ingest"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<html><head><title>Fetched Page</title><style>p{}</style></head>`+
			`<body><p>Alpha marker 7781 on the fetched page.</p><script>var noise="bad-marker";</script></body></html>`)
	}))
	defer srv.Close()

	old := v5AllowPrivateFetch
	v5AllowPrivateFetch = true
	defer func() { v5AllowPrivateFetch = old }()

	code, body := doV5(t, h, http.MethodPost, "/ns/"+ns+"/document", map[string]any{"content": srv.URL, "id": "u1"})
	if code != http.StatusOK {
		t.Fatalf("POST url = %d %v", code, body)
	}
	code, doc := getDocJSON(t, h, ns, "u1", "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d %v", code, doc)
	}
	content := doc["content"].(string)
	if !strings.Contains(content, "Alpha marker 7781") {
		t.Fatalf("content = %q", content)
	}
	if strings.Contains(content, "bad-marker") || strings.Contains(content, "<html") {
		t.Fatalf("html was not stripped: %q", content)
	}
	if doc["title"] != "Fetched Page" {
		t.Fatalf("title = %v, want HTML title", doc["title"])
	}
	meta, _ := doc["metadata"].(map[string]any)
	if meta["sourceUrl"] != srv.URL {
		t.Fatalf("metadata.sourceUrl = %v", meta["sourceUrl"])
	}

	// PATCH with a URL replaces the content as well.
	code, _ = doV5(t, h, http.MethodPatch, "/ns/"+ns+"/document/u1", map[string]any{"content": srv.URL})
	if code != http.StatusOK {
		t.Fatalf("PATCH url = %d", code)
	}
	rec, _, _ := h.Engine.DB.GetDocument(ns, "u1")
	if !strings.Contains(rec.Content, "Alpha marker 7781") || rec.Status != "queued" {
		t.Fatalf("patched doc = %q status %q", rec.Content, rec.Status)
	}
}

func TestV5PublicIP(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": false, "10.1.2.3": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "198.18.0.5": false,
		"192.0.2.7": false, "240.0.0.1": false, "0.0.0.0": false,
		"::1": false, "fc00::1": false, "fe80::1": false, "fec0::1": false,
		"2001:db8::1": false, "2001::1": false, "64:ff9b::7f00:1": false,
		"2002:7f00:0001::1": false, "2002:5db8:d822::1": true,
		"93.184.216.34": true, "2606:4700:4700::1111": true,
	}
	for raw, want := range cases {
		if got := v5PublicIP(net.ParseIP(raw)); got != want {
			t.Errorf("v5PublicIP(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestV5URLFetchGuards(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	old := v5AllowPrivateFetch
	v5AllowPrivateFetch = false
	defer func() { v5AllowPrivateFetch = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "loopback body")
	}))
	defer srv.Close()

	code, body := doV5(t, h, http.MethodPost, "/ns/url:guard/document", map[string]any{"content": srv.URL})
	if code != http.StatusBadRequest {
		t.Fatalf("loopback fetch = %d %v", code, body)
	}
	wantErrorCode(t, body, "invalid_value", "content")

	// Redirects are validated before they are followed.
	client := v5NewFetchClient()
	redirectReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/next", nil)
	if err := client.CheckRedirect(redirectReq, nil); err == nil {
		t.Fatal("redirect to a loopback address was not blocked")
	}
	fileReq, _ := http.NewRequest(http.MethodGet, "file:///etc/passwd", nil)
	if err := client.CheckRedirect(fileReq, nil); err == nil {
		t.Fatal("non-http redirect scheme was not blocked")
	}

	t.Run("non-text content type", func(t *testing.T) {
		v5AllowPrivateFetch = true
		defer func() { v5AllowPrivateFetch = false }()
		binarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0x00, 0x01, 0x02})
		}))
		defer binarySrv.Close()
		code, body := doV5(t, h, http.MethodPost, "/ns/url:guard/document", map[string]any{"content": binarySrv.URL})
		if code != http.StatusBadRequest {
			t.Fatalf("binary fetch = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_value", "content")
	})

	t.Run("oversized response", func(t *testing.T) {
		v5AllowPrivateFetch = true
		defer func() { v5AllowPrivateFetch = false }()
		bigSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			chunk := bytes.Repeat([]byte("a"), 1<<20)
			for range 11 {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		}))
		defer bigSrv.Close()
		code, body := doV5(t, h, http.MethodPost, "/ns/url:guard/document", map[string]any{"content": bigSrv.URL})
		if code != http.StatusBadRequest {
			t.Fatalf("oversized fetch = %d %v", code, body)
		}
		wantErrorCode(t, body, "invalid_value", "content")
	})
}
