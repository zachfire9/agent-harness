package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultFileToolMaxOutputBytes = 16_000

type fileToolArgs struct {
	Path string `json:"path"`
}

type searchFilesArgs struct {
	Path  string `json:"path"`
	Query string `json:"query"`
}

type workspaceTool struct {
	root           string
	maxOutputBytes int
}

// ListFilesTool lists files under a configured workspace root.
type ListFilesTool struct{ workspaceTool }

// ReadFileTool reads files under a configured workspace root.
type ReadFileTool struct{ workspaceTool }

// SearchFilesTool searches text files under a configured workspace root.
type SearchFilesTool struct{ workspaceTool }

// NewListFilesTool creates a workspace-safe list_files tool.
func NewListFilesTool(root string, maxOutputBytes int) ListFilesTool {
	return ListFilesTool{workspaceTool: newWorkspaceTool(root, maxOutputBytes)}
}

// NewReadFileTool creates a workspace-safe read_file tool.
func NewReadFileTool(root string, maxOutputBytes int) ReadFileTool {
	return ReadFileTool{workspaceTool: newWorkspaceTool(root, maxOutputBytes)}
}

// NewSearchFilesTool creates a workspace-safe search_files tool.
func NewSearchFilesTool(root string, maxOutputBytes int) SearchFilesTool {
	return SearchFilesTool{workspaceTool: newWorkspaceTool(root, maxOutputBytes)}
}

func newWorkspaceTool(root string, maxOutputBytes int) workspaceTool {
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultFileToolMaxOutputBytes
	}
	return workspaceTool{root: root, maxOutputBytes: maxOutputBytes}
}

func (ListFilesTool) Name() string { return "list_files" }

func (ListFilesTool) Description() string {
	return "List files under the configured workspace root"
}

func (ListFilesTool) JSONSchema() map[string]any {
	return pathOnlySchema("Directory path inside the workspace")
}

func (t ListFilesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed fileToolArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid list_files arguments: %w", err)
	}

	root, target, err := t.resolve(parsed.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("directory not found: %s", cleanDisplayPath(parsed.Path))
		}
		return "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", cleanDisplayPath(parsed.Path))
	}

	var lines []string
	if err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == target {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if isBlockedWorkspacePath(name) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
		return ctx.Err()
	}); err != nil {
		return "", fmt.Errorf("list files: %w", err)
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return "(no files found)", nil
	}
	return capOutput(strings.Join(lines, "\n"), t.maxOutputBytes), nil
}

func (ReadFileTool) Name() string { return "read_file" }

func (ReadFileTool) Description() string {
	return "Read a text file under the configured workspace root"
}

func (ReadFileTool) JSONSchema() map[string]any {
	return pathOnlySchema("File path inside the workspace")
}

func (t ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed fileToolArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}

	_, target, err := t.resolve(parsed.Path)
	if err != nil {
		return "", err
	}
	if err := t.ensureAllowed(target, parsed.Path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("file not found: %s", cleanDisplayPath(parsed.Path))
		}
		return "", fmt.Errorf("read file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return capOutput(string(data), t.maxOutputBytes), nil
}

func (SearchFilesTool) Name() string { return "search_files" }

func (SearchFilesTool) Description() string {
	return "Search text files under the configured workspace root"
}

func (SearchFilesTool) JSONSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"query"},
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Text to search for inside workspace files",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path inside the workspace; defaults to .",
			},
		},
	}
}

func (t SearchFilesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed searchFilesArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid search_files arguments: %w", err)
	}
	parsed.Query = strings.TrimSpace(parsed.Query)
	if parsed.Query == "" {
		return "", errors.New("query is required")
	}

	root, target, err := t.resolve(parsed.Path)
	if err != nil {
		return "", err
	}

	var matches []string
	if err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if isBlockedWorkspacePath(filepath.ToSlash(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if isBlockedWorkspacePath(filepath.ToSlash(rel)) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		if strings.ContainsRune(content, '\x00') {
			return nil
		}
		if !strings.Contains(content, parsed.Query) {
			return ctx.Err()
		}
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, parsed.Query) {
				matches = append(matches, fmt.Sprintf("%s: %s", filepath.ToSlash(rel), strings.TrimSpace(line)))
			}
		}
		return ctx.Err()
	}); err != nil {
		return "", fmt.Errorf("search files: %w", err)
	}
	if len(matches) == 0 {
		return "(no matches found)", nil
	}
	sort.Strings(matches)
	return capOutput(strings.Join(matches, "\n"), t.maxOutputBytes), nil
}

func (t workspaceTool) resolve(requestedPath string) (string, string, error) {
	if strings.TrimSpace(t.root) == "" {
		return "", "", errors.New("workspace root is required")
	}
	root, err := filepath.Abs(t.root)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace root: %w", err)
	}

	requestedPath = strings.TrimSpace(requestedPath)
	if requestedPath == "" {
		requestedPath = "."
	}
	if !filepath.IsAbs(requestedPath) {
		requestedPath = filepath.Join(root, requestedPath)
	}
	target, err := filepath.Abs(requestedPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}
	target = filepath.Clean(target)
	if err := ensureInsideWorkspace(root, target, requestedPath); err != nil {
		return "", "", err
	}

	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err == nil {
		resolvedTarget = filepath.Clean(resolvedTarget)
		if err := ensureInsideWorkspace(root, resolvedTarget, requestedPath); err != nil {
			return "", "", err
		}
		target = resolvedTarget
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}

	return root, target, nil
}

func (t workspaceTool) ensureAllowed(target string, displayPath string) error {
	root, err := filepath.Abs(t.root)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if isBlockedWorkspacePath(filepath.ToSlash(rel)) {
		return fmt.Errorf("path is not allowed: %s", cleanDisplayPath(displayPath))
	}
	return nil
}

func isBlockedWorkspacePath(relativePath string) bool {
	relativePath = filepath.ToSlash(filepath.Clean(relativePath))
	if relativePath == "." {
		return false
	}
	first, _, _ := strings.Cut(relativePath, "/")
	switch first {
	case ".git", ".agent-harness":
		return true
	}
	base := filepath.Base(relativePath)
	return base == ".env" || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key")
}

func ensureInsideWorkspace(root string, target string, displayPath string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes workspace: %s", cleanDisplayPath(displayPath))
	}
	return nil
}

func pathOnlySchema(pathDescription string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": pathDescription,
			},
		},
	}
}

func capOutput(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + fmt.Sprintf("\n[truncated after %d bytes]", maxBytes)
}

func cleanDisplayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "."
	}
	return filepath.ToSlash(filepath.Clean(path))
}
