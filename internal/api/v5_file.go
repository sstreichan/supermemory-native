package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"

	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/memory"
)

const (
	v5MaxFileBytes      = 25 << 20
	v5MaxFetchBytes     = 10 << 20
	v5MultipartOverhead = 1 << 20
	v5MaxFetchRedirects = 5
)

// v5FileInput is one parsed multipart upload plus the validated document
// fields that accompany it.
type v5FileInput struct {
	Doc      v5DocInput
	HasFile  bool
	Content  string
	FileName string
	MIMEType string
	FileType string
}

var v5FileFormFields = map[string]bool{
	"file": true, "supportingContext": true, "metadata": true, "group": true,
	"date": true, "taskType": true, "dreaming": true, "fileType": true, "mimeType": true,
}

// v5ParseFileForm reads a multipart/form-data body, validates every field and
// extracts the uploaded file into plain text.
func (h *Handler) v5ParseFileForm(w http.ResponseWriter, r *http.Request, requireFile bool) (v5FileInput, map[string]json.RawMessage, bool) {
	empty := v5FileInput{}
	ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || ct != "multipart/form-data" {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"file"}, "Expected multipart/form-data", nil)
		return empty, nil, false
	}

	r.Body = http.MaxBytesReader(w, r.Body, v5MaxFileBytes+v5MultipartOverhead)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			v5ValidationError(w, http.StatusBadRequest, nil, "too_big", []string{"file"},
				"File must be at most 25 MiB", map[string]any{"maximum": v5MaxFileBytes})
			return empty, nil, false
		}
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"file"}, "Invalid multipart body", nil)
		return empty, nil, false
	}
	defer r.MultipartForm.RemoveAll()

	raw := map[string]json.RawMessage{}
	unknown := []string{}
	for key, values := range r.MultipartForm.Value {
		if !v5FileFormFields[key] {
			unknown = append(unknown, key)
			continue
		}
		if len(values) > 1 {
			v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{key}, "Expected a single value", nil)
			return empty, nil, false
		}
		if key == "metadata" || key == "group" {
			raw[key] = json.RawMessage(values[0])
			continue
		}
		quoted, _ := json.Marshal(values[0])
		raw[key] = quoted
	}
	for key := range r.MultipartForm.File {
		if key != "file" {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		v5ValidationError(w, http.StatusBadRequest, nil, "unrecognized_keys", nil,
			"Unrecognized key(s) in object: "+strings.Join(unknown, ", "), map[string]any{"keys": unknown})
		return empty, nil, false
	}

	for _, key := range []string{"metadata", "group"} {
		if value, ok := raw[key]; ok && jsonTypeName(value) != "object" {
			v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{key},
				"Expected object, received "+jsonTypeName(value), nil)
			return empty, nil, false
		}
	}

	doc, ok := v5ParseDocInput(w, raw, false, true)
	if !ok {
		return empty, nil, false
	}

	declaredType, _, valid := v5FieldString(raw, "fileType")
	if !valid {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"fileType"}, "Expected string", nil)
		return empty, nil, false
	}
	declaredMIME, _, valid := v5FieldString(raw, "mimeType")
	if !valid {
		v5ValidationError(w, http.StatusBadRequest, raw, "invalid_type", []string{"mimeType"}, "Expected string", nil)
		return empty, nil, false
	}

	out := v5FileInput{Doc: doc}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		if requireFile {
			v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"file"}, "Required", nil)
			return empty, nil, false
		}
		return out, raw, true
	}
	if len(files) > 1 {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"file"}, "Expected a single file", nil)
		return empty, nil, false
	}

	header := files[0]
	f, err := header.Open()
	if err != nil {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"file"}, "Failed to read uploaded file", nil)
		return empty, nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, v5MaxFileBytes+1))
	f.Close()
	if err != nil {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_type", []string{"file"}, "Failed to read uploaded file", nil)
		return empty, nil, false
	}
	if len(data) > v5MaxFileBytes {
		v5ValidationError(w, http.StatusBadRequest, nil, "too_big", []string{"file"},
			"File must be at most 25 MiB", map[string]any{"maximum": v5MaxFileBytes})
		return empty, nil, false
	}
	if len(data) == 0 {
		v5ValidationError(w, http.StatusBadRequest, nil, "too_small", []string{"file"},
			"File must not be empty", map[string]any{"minimum": 1})
		return empty, nil, false
	}

	name := strings.TrimSpace(strings.ReplaceAll(header.Filename, "\\", "/"))
	name = filepath.Base(name)
	if name == "" || name == "." || name == "/" {
		name = "upload"
	}

	kind, mimeType, failPath, failMsg := v5ResolveFileType(name, declaredType, declaredMIME, data)
	if failMsg != "" {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", failPath, failMsg, nil)
		return empty, nil, false
	}
	text, _, ok := v5ExtractText(kind, name, mimeType, data)
	if !ok {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"file"},
			"Unable to extract text from the uploaded file", nil)
		return empty, raw, false
	}
	if strings.TrimSpace(text) == "" {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"file"},
			"No text could be extracted from the uploaded file", nil)
		return empty, raw, false
	}
	if len([]rune(text)) > v5MaxContentRunes {
		v5ValidationError(w, http.StatusBadRequest, nil, "too_big", []string{"file"},
			"Extracted text must contain at most 1000000 character(s)", map[string]any{"maximum": v5MaxContentRunes})
		return empty, raw, false
	}

	out.HasFile = true
	out.Content = text
	out.FileName = name
	out.MIMEType = mimeType
	out.FileType = kind
	return out, raw, true
}

func v5StripMIMEParams(value string) string {
	if mt, _, err := mime.ParseMediaType(value); err == nil {
		return mt
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func v5IsTextMIME(mt string) bool {
	switch mt {
	case "application/json", "application/xml", "text/xml", "text/csv",
		"application/x-yaml", "application/yaml":
		return true
	}
	return strings.HasPrefix(mt, "text/")
}

var (
	v5ImageExtensions = map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
		".webp": true, ".tif": true, ".tiff": true, ".heic": true, ".avif": true, ".svg": true,
	}
	v5VideoExtensions = map[string]bool{
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true, ".m4v": true,
	}
	v5AudioExtensions = map[string]bool{
		".mp3": true, ".wav": true, ".m4a": true, ".aac": true, ".flac": true, ".ogg": true, ".opus": true,
	}
	v5TextExtensions = map[string]bool{
		".txt": true, ".text": true, ".md": true, ".markdown": true, ".html": true, ".htm": true,
		".json": true, ".csv": true, ".xml": true, ".yaml": true, ".yml": true, ".log": true,
		".tsv": true, ".rst": true, ".adoc": true, ".tex": true, ".srt": true, ".vtt": true,
	}
)

// v5ResolveFileType determines the extraction kind. When the caller named a
// type it must be text or pdf; otherwise the type is inferred from the
// declared MIME type, the filename extension and finally the content itself.
func v5ResolveFileType(name, declaredType, declaredMIME string, data []byte) (kind, mimeType string, failPath []string, failMsg string) {
	ext := strings.ToLower(filepath.Ext(name))
	declaredType = strings.ToLower(strings.TrimSpace(declaredType))
	declaredMIME = v5StripMIMEParams(declaredMIME)

	unsupported := func(category string) (string, string, []string, string) {
		return "", "", []string{"fileType"}, "Unsupported file type: " + category
	}

	switch declaredType {
	case "":
	case "text":
		kind = "text"
	case "pdf":
		kind = "pdf"
	case "image", "video", "audio":
		return unsupported(declaredType)
	default:
		return "", "", []string{"fileType"}, "Invalid option"
	}

	if kind == "" && declaredMIME != "" {
		switch {
		case strings.HasPrefix(declaredMIME, "image/"):
			return unsupported("image")
		case strings.HasPrefix(declaredMIME, "video/"):
			return unsupported("video")
		case strings.HasPrefix(declaredMIME, "audio/"):
			return unsupported("audio")
		case declaredMIME == "application/pdf":
			kind = "pdf"
		case v5IsTextMIME(declaredMIME):
			kind = "text"
		}
	}

	if kind == "" {
		switch {
		case ext == ".pdf":
			kind = "pdf"
		case v5TextExtensions[ext]:
			kind = "text"
		case v5ImageExtensions[ext]:
			return unsupported("image")
		case v5VideoExtensions[ext]:
			return unsupported("video")
		case v5AudioExtensions[ext]:
			return unsupported("audio")
		}
	}

	if kind == "" {
		detected := v5StripMIMEParams(http.DetectContentType(data))
		switch {
		case detected == "application/pdf":
			kind = "pdf"
		case v5IsTextMIME(detected):
			kind = "text"
		case strings.HasPrefix(detected, "image/"):
			return unsupported("image")
		case strings.HasPrefix(detected, "video/"):
			return unsupported("video")
		case strings.HasPrefix(detected, "audio/"):
			return unsupported("audio")
		case utf8.Valid(data) && !bytes.ContainsRune(data, 0):
			kind = "text"
		default:
			return "", "", []string{"file"}, "Unsupported file content"
		}
	}

	mimeType = declaredMIME
	if mimeType == "" {
		mimeType = v5StripMIMEParams(mime.TypeByExtension(ext))
	}
	if mimeType == "" {
		mimeType = v5StripMIMEParams(http.DetectContentType(data))
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		if kind == "pdf" {
			mimeType = "application/pdf"
		} else {
			mimeType = "text/plain"
		}
	}
	return kind, mimeType, nil, ""
}

func v5IsHTML(name, mimeType string) bool {
	if mimeType == "text/html" || mimeType == "application/xhtml+xml" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".html" || ext == ".htm"
}

// v5ExtractText turns the uploaded bytes into canonical document text. The
// second return value is the HTML <title> when one was found.
func v5ExtractText(kind, name, mimeType string, data []byte) (string, string, bool) {
	if kind == "pdf" {
		text, err := v5PDFText(data)
		if err != nil {
			return "", "", false
		}
		return v5NormalizeText(text), "", true
	}
	if !utf8.Valid(data) {
		return "", "", false
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	title := ""
	if v5IsHTML(name, mimeType) {
		text, title = v5HTMLToText(text)
	}
	return v5NormalizeText(text), title, true
}

func v5PDFText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("pdf: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		pageText, pageErr := page.GetPlainText(nil)
		if pageErr != nil {
			continue
		}
		buf.WriteString(pageText)
		buf.WriteString("\n")
	}
	return buf.String(), nil
}

var (
	v5HTMLBlockTags = regexp.MustCompile(`(?i)</?(?:p|div|br|li|tr|h[1-6]|section|article|header|footer|blockquote|pre|table|ul|ol|dd|dt)\b[^>]*>`)
	v5HTMLDropTags  = regexp.MustCompile(`(?is)<(script|style|noscript|template)\b[^>]*>.*?</\s*(?:script|style|noscript|template)\s*>`)
	v5HTMLTitleTag  = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
	v5HTMLAnyTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	v5Spaces        = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	v5BlankLines    = regexp.MustCompile(`\n{3,}`)
)

// v5HTMLToText strips markup without a DOM parser and returns the visible text
// plus the document title.
func v5HTMLToText(source string) (string, string) {
	title := ""
	if m := v5HTMLTitleTag.FindStringSubmatch(source); m != nil {
		title = strings.TrimSpace(html.UnescapeString(v5HTMLAnyTag.ReplaceAllString(m[1], "")))
	}
	text := v5HTMLDropTags.ReplaceAllString(source, "\n")
	text = v5HTMLBlockTags.ReplaceAllString(text, "\n")
	text = v5HTMLAnyTag.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return v5NormalizeText(text), title
}

func v5NormalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = v5Spaces.ReplaceAllString(text, " ")
	// Re-tighten the line edges the space pass left behind.
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	text = strings.Join(lines, "\n")
	text = v5BlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// v5CreateDocumentFile handles POST /ns/{ns}/document/file.
func (h *Handler) v5CreateDocumentFile(w http.ResponseWriter, r *http.Request, ns string) {
	h.v5WriteFileDocument(w, r, ns, "")
}

// v5ReplaceDocumentFile handles POST /ns/{ns}/document/file/{id}, a complete
// content replacement that requires the document to exist.
func (h *Handler) v5ReplaceDocumentFile(w http.ResponseWriter, r *http.Request, ns, id string) {
	h.v5WriteFileDocument(w, r, ns, id)
}

func (h *Handler) v5WriteFileDocument(w http.ResponseWriter, r *http.Request, ns, id string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	input, _, ok := h.v5ParseFileForm(w, r, true)
	if !ok {
		return
	}
	if _, err := h.Engine.DB.EnsureNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create namespace", err)
		return
	}

	now := time.Now().UTC()
	created := id == ""
	doc := db.Document{}
	if !created {
		existing, found, err := h.Engine.DB.GetDocument(ns, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to read document", err)
			return
		}
		if !found {
			writeErr(w, http.StatusNotFound, "Document not found", nil)
			return
		}
		doc = existing
	} else {
		id = memory.GenerateUUID()
		doc = db.Document{
			Namespace:      ns,
			ID:             id,
			TaskType:       "memory",
			Dreaming:       "dynamic",
			Metadata:       map[string]any{},
			Grp:            map[string]any{},
			CreatedAt:      now,
			Status:         db.DocumentQueued,
			DreamingStatus: db.DocumentQueued,
		}
	}

	doc.Content = input.Content
	doc.Title = input.FileName
	doc.Summary = readRunes(input.Content, 200)
	doc.FileName = input.FileName
	doc.MIMEType = input.MIMEType
	doc.FileType = input.FileType
	doc.Status = db.DocumentQueued
	doc.DreamingStatus = db.DocumentQueued
	doc.ResetMemories = !created
	doc.UpdatedAt = now

	in := input.Doc
	if in.HasMeta {
		doc.Metadata = v5AttachTemporal(in.Metadata, in.Date)
	} else {
		doc.Metadata = v5AttachTemporal(doc.Metadata, "")
	}
	if in.HasDate {
		doc.Date = in.Date
		doc.Metadata = v5AttachTemporal(doc.Metadata, in.Date)
	}
	if in.HasTask {
		doc.TaskType = in.TaskType
	}
	if in.HasDream {
		doc.Dreaming = in.Dreaming
	}
	if in.HasContext {
		doc.SupportingContext = in.Context
	}
	if in.Grp != nil {
		doc.Grp = in.Grp
	}

	if err := h.Engine.DB.UpsertDocument(doc); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to store document", err)
		return
	}
	if err := h.Engine.DB.TouchNamespace(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update namespace", err)
		return
	}
	v5Write(w, http.StatusOK, map[string]any{"id": id, "status": doc.Status})
}

// v5PatchDocumentFile handles PATCH /ns/{ns}/document/file/{id}. A new file
// replaces the content and requeues the document; field-only updates leave the
// processing status untouched.
func (h *Handler) v5PatchDocumentFile(w http.ResponseWriter, r *http.Request, ns, id string) {
	if !v5ValidateNamespace(w, ns) || !v5RejectQueryKeys(w, r) {
		return
	}
	input, raw, ok := h.v5ParseFileForm(w, r, false)
	if !ok {
		return
	}
	if !input.HasFile && len(raw) == 0 {
		v5ValidationError(w, http.StatusBadRequest, nil, "custom", nil, "At least one document field must be provided", nil)
		return
	}
	if !input.HasFile {
		hasDocumentField := false
		for _, key := range []string{"metadata", "group", "date", "taskType", "dreaming", "supportingContext"} {
			if _, ok := raw[key]; ok {
				hasDocumentField = true
				break
			}
		}
		if !hasDocumentField {
			v5ValidationError(w, http.StatusBadRequest, nil, "custom", nil, "At least one document field must be provided", nil)
			return
		}
	}
	if _, hasGroup := raw["group"]; hasGroup && !input.HasFile {
		v5ValidationError(w, http.StatusBadRequest, nil, "custom", nil,
			"group only applies on re-ingestion; this update does not replace the document content", nil)
		return
	}

	existing, found, err := h.Engine.DB.GetDocument(ns, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read document", err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Document not found", nil)
		return
	}

	in := input.Doc
	fields := map[string]any{"updated_at": time.Now().UTC()}
	if input.HasFile {
		fields["content"] = input.Content
		fields["title"] = input.FileName
		fields["summary"] = readRunes(input.Content, 200)
		fields["status"] = db.DocumentQueued
		fields["dreaming_status"] = db.DocumentQueued
		fields["reset_memories"] = 1
		fields["file_name"] = input.FileName
		fields["mime_type"] = input.MIMEType
		fields["file_type"] = input.FileType
	}
	if in.HasMeta {
		fields["metadata"] = v5AttachTemporal(in.Metadata, in.Date)
	}
	if in.HasDate {
		fields["source_date"] = in.Date
		if !in.HasMeta {
			fields["metadata"] = v5AttachTemporal(existing.Metadata, in.Date)
		}
	}
	if in.HasTask {
		fields["task_type"] = in.TaskType
	}
	if in.HasDream {
		fields["dreaming"] = in.Dreaming
	}
	if in.HasContext {
		fields["supporting_context"] = in.Context
	}
	if in.Grp != nil {
		fields["grp"] = in.Grp
	}

	doc, updated, err := h.Engine.DB.UpdateDocumentFields(ns, id, fields)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update document", err)
		return
	}
	if !updated {
		writeErr(w, http.StatusNotFound, "Document not found", nil)
		return
	}
	_ = h.Engine.DB.TouchNamespace(ns)
	v5Write(w, http.StatusOK, map[string]any{"id": doc.ID, "status": doc.Status})
}

// --- URL ingestion -------------------------------------------------------

// v5AllowPrivateFetch disables the SSRF guard. Tests set it because httptest
// servers listen on loopback; production keeps it false.
var v5AllowPrivateFetch bool

type v5FetchedPage struct {
	Text  string
	Title string
	URL   string
}

var errV5BlockedAddr = errors.New("blocked address")

// v5BlockedNets covers ranges that net.IP's predicates miss but which must
// never be reachable from a user-supplied URL: CGNAT, benchmarking, documentation,
// reserved space, NAT64 and other transition prefixes with embedded addresses.
var v5BlockedNets = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"255.255.255.255/32",
		"100::/64", "2001::/32", "2001:db8::/32", "fec0::/10",
		"64:ff9b::/96", "64:ff9b:1::/48",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("invalid blocked CIDR " + c)
		}
		nets = append(nets, n)
	}
	return nets
}()

func v5PublicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		for _, n := range v5BlockedNets {
			if n.Contains(v4) {
				return false
			}
		}
		return true
	}
	// 6to4 embeds an IPv4 address in bytes 2-6; it must pass the IPv4 rules.
	if len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x02 {
		return v5PublicIP(net.IP(ip[2:6]))
	}
	for _, n := range v5BlockedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

func v5ValidateFetchURL(u *url.URL) error {
	if u == nil {
		return errors.New("missing URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("unsupported URL scheme")
	}
	if u.Hostname() == "" {
		return errors.New("missing host")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !v5AllowPrivateFetch && !v5PublicIP(ip) {
		return errV5BlockedAddr
	}
	return nil
}

func v5NewFetchClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range ips {
				if v5AllowPrivateFetch || v5PublicIP(candidate.IP) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				}
			}
			return nil, errV5BlockedAddr
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= v5MaxFetchRedirects {
				return errors.New("too many redirects")
			}
			return v5ValidateFetchURL(req.URL)
		},
	}
}

var v5FetchClient = v5NewFetchClient()

func v5AsHTTPURL(s string) (*url.URL, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || strings.ContainsAny(trimmed, " \t\r\n") {
		return nil, false
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, false
	}
	return u, true
}

func v5FetchURLText(ctx context.Context, raw string) (v5FetchedPage, error) {
	u, ok := v5AsHTTPURL(raw)
	if !ok {
		return v5FetchedPage{}, errors.New("invalid URL")
	}
	if err := v5ValidateFetchURL(u); err != nil {
		return v5FetchedPage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return v5FetchedPage{}, err
	}
	req.Header.Set("User-Agent", "supermemory-native/1.0")
	req.Header.Set("Accept", "text/html, text/plain, text/markdown, application/json, application/xml, text/csv")

	resp, err := v5FetchClient.Do(req)
	if err != nil {
		return v5FetchedPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return v5FetchedPage{}, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	contentType := v5StripMIMEParams(resp.Header.Get("Content-Type"))
	switch contentType {
	case "text/html", "text/plain", "text/markdown", "application/json", "application/xml", "text/csv":
	default:
		return v5FetchedPage{}, fmt.Errorf("unsupported content type %q", contentType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, v5MaxFetchBytes+1))
	if err != nil {
		return v5FetchedPage{}, err
	}
	if len(data) > v5MaxFetchBytes {
		return v5FetchedPage{}, fmt.Errorf("response exceeds 10 MiB")
	}
	if !utf8.Valid(data) {
		return v5FetchedPage{}, errors.New("response is not valid UTF-8")
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	title := ""
	if contentType == "text/html" {
		text, title = v5HTMLToText(text)
	}
	text = v5NormalizeText(text)
	if text == "" {
		return v5FetchedPage{}, errors.New("no text could be extracted from the URL")
	}
	if len([]rune(text)) > v5MaxContentRunes {
		return v5FetchedPage{}, fmt.Errorf("extracted text exceeds the maximum size")
	}
	final := u.String()
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return v5FetchedPage{Text: text, Title: title, URL: final}, nil
}

// v5ResolveURLContent replaces an absolute http(s) content value with the
// text fetched from that URL. Any other content passes through untouched.
func (h *Handler) v5ResolveURLContent(ctx context.Context, w http.ResponseWriter, in *v5DocInput) bool {
	if !in.HasContent {
		return true
	}
	if _, ok := v5AsHTTPURL(in.Content); !ok {
		return true
	}
	page, err := v5FetchURLText(ctx, in.Content)
	if err != nil {
		v5ValidationError(w, http.StatusBadRequest, nil, "invalid_value", []string{"content"},
			"Failed to fetch URL content: "+err.Error(), nil)
		return false
	}
	in.Content = page.Text
	in.HasContent = true
	in.Title = page.Title
	if in.Title == "" {
		in.Title = page.URL
	}
	if in.Metadata == nil {
		in.Metadata = map[string]any{}
	}
	in.Metadata["sourceUrl"] = page.URL
	in.HasMeta = true
	return true
}
