package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Vault struct {
	Dir string
}

// NewVault initializes a new Vault and ensures its directory exists.
// The directory is created owner-only: memory files can hold personal data.
func NewVault(dir string) (*Vault, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory %s: %w", absDir, err)
	}
	return &Vault{Dir: absDir}, nil
}

// ValidID reports whether id is safe to use as a vault file name component.
// It rejects empty ids, absolute paths, path separators and ".." so callers
// can never escape the vault directory (path traversal).
func ValidID(id string) bool {
	if id == "" || filepath.IsAbs(id) {
		return false
	}
	if strings.Contains(id, "..") || strings.ContainsAny(id, `/\`) {
		return false
	}
	return true
}

// PathForID resolves a memory id to its absolute path inside the vault and
// fails if the result would escape the vault directory.
func (v *Vault) PathForID(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("invalid memory id %q", id)
	}
	filePath := filepath.Join(v.Dir, id+".okf")
	rel, err := filepath.Rel(v.Dir, filePath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("resolved path for id %q escapes the vault", id)
	}
	return filePath, nil
}

// WriteMemory writes a memory to a physical OKF file in the vault.
func (v *Vault) WriteMemory(id, content, containerTag string) (string, error) {
	return v.WriteMemoryWithTime(id, content, containerTag, time.Now())
}

// WriteMemoryWithTime writes a memory to a physical OKF file in the vault with a specific timestamp.
func (v *Vault) WriteMemoryWithTime(id, content, containerTag string, t time.Time) (string, error) {
	doc, err := ParseOKF(content)
	if err != nil {
		return "", err
	}

	// Ensure standard OKF frontmatter properties are populated
	doc.SetDefault("type", "memory")
	doc.SetDefault("title", fmt.Sprintf("Memory %s", id))
	doc.SetDefault("container_tag", containerTag)
	doc.SetDefault("timestamp", t.Format(time.RFC3339))

	formatted, err := FormatOKF(doc)
	if err != nil {
		return "", err
	}

	filePath, err := v.PathForID(id)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filePath, []byte(formatted), 0600); err != nil {
		return "", fmt.Errorf("failed to write OKF file: %w", err)
	}

	return filePath, nil
}

// ReadMemory reads and parses an OKF file by its physical path or ID.
// Resolved paths are confined to the vault directory.
func (v *Vault) ReadMemory(id string) (string, error) {
	var filePath string
	if filepath.IsAbs(id) || strings.Contains(id, string(filepath.Separator)) {
		filePath = id
		rel, err := filepath.Rel(v.Dir, filePath)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("path %q is outside the vault", filePath)
		}
	} else {
		var err error
		filePath, err = v.PathForID(id)
		if err != nil {
			return "", err
		}
	}

	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(bytes), nil
}

// DeleteMemory deletes the physical file from the vault.
func (v *Vault) DeleteMemory(id string) error {
	filePath, err := v.PathForID(id)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete OKF file: %w", err)
	}
	return nil
}

// ListPhysicalFiles returns a list of all memory IDs present in the vault.
func (v *Vault) ListPhysicalFiles() ([]string, error) {
	entries, err := os.ReadDir(v.Dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault directory: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".okf") {
			id := strings.TrimSuffix(entry.Name(), ".okf")
			ids = append(ids, id)
		}
	}
	return ids, nil
}
