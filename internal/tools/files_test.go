package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/tools"
)

func TestListFilesToolListsFilesInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "hello")
	writeTestFile(t, root, "notes/todo.txt", "remember")
	tool := tools.NewListFilesTool(root, 10_000)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatalf("execute list_files: %v", err)
	}

	if !strings.Contains(got, "README.md") || !strings.Contains(got, filepath.ToSlash("notes/todo.txt")) {
		t.Fatalf("expected listed workspace files, got %q", got)
	}
}

func TestListFilesToolSkipsGitDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".git/config", "remote config")
	writeTestFile(t, root, "README.md", "hello")
	tool := tools.NewListFilesTool(root, 10_000)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatalf("execute list_files: %v", err)
	}
	if strings.Contains(got, ".git") {
		t.Fatalf("expected .git directory to be skipped, got %q", got)
	}
	if !strings.Contains(got, "README.md") {
		t.Fatalf("expected normal file to be listed, got %q", got)
	}
}

func TestSearchFilesToolSkipsBinaryFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "agent harness")
	writeTestFile(t, root, "binary.dat", "agent\x00harness")
	tool := tools.NewSearchFilesTool(root, 10_000)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"agent","path":"."}`))
	if err != nil {
		t.Fatalf("execute search_files: %v", err)
	}
	if strings.Contains(got, "binary.dat") {
		t.Fatalf("expected binary file to be skipped, got %q", got)
	}
	if !strings.Contains(got, "README.md: agent harness") {
		t.Fatalf("expected text match, got %q", got)
	}
}

func TestFileToolsSkipSensitiveLocalFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".env", "OPENAI_API_KEY=secret")
	writeTestFile(t, root, "README.md", "safe agent notes")
	listTool := tools.NewListFilesTool(root, 10_000)
	readTool := tools.NewReadFileTool(root, 10_000)
	searchTool := tools.NewSearchFilesTool(root, 10_000)

	listed, err := listTool.Execute(context.Background(), json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatalf("execute list_files: %v", err)
	}
	if strings.Contains(listed, ".env") {
		t.Fatalf("expected .env to be skipped in listings, got %q", listed)
	}

	_, err = readTool.Execute(context.Background(), json.RawMessage(`{"path":".env"}`))
	if err == nil || !strings.Contains(err.Error(), "path is not allowed") {
		t.Fatalf("expected .env read to be blocked, got %v", err)
	}

	searched, err := searchTool.Execute(context.Background(), json.RawMessage(`{"query":"secret","path":"."}`))
	if err != nil {
		t.Fatalf("execute search_files: %v", err)
	}
	if strings.Contains(searched, "secret") || strings.Contains(searched, ".env") {
		t.Fatalf("expected .env to be skipped in search, got %q", searched)
	}
}

func TestReadFileToolReadsFileInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "docs/intro.txt", "hello from workspace")
	tool := tools.NewReadFileTool(root, 10_000)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"docs/intro.txt"}`))
	if err != nil {
		t.Fatalf("execute read_file: %v", err)
	}

	if got != "hello from workspace" {
		t.Fatalf("expected file contents, got %q", got)
	}
}

func TestFileToolsRejectParentTraversal(t *testing.T) {
	root := t.TempDir()
	tool := tools.NewReadFileTool(root, 10_000)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"../outside.txt"}`))
	if err == nil {
		t.Fatal("expected traversal error")
	}
	if !strings.Contains(err.Error(), "path escapes workspace") {
		t.Fatalf("expected workspace escape error, got %q", err.Error())
	}
}

func TestFileToolsRejectAbsolutePathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	tool := tools.NewReadFileTool(root, 10_000)

	_, err := tool.Execute(context.Background(), mustJSON(t, map[string]string{"path": outside}))
	if err == nil {
		t.Fatal("expected outside absolute path error")
	}
	if !strings.Contains(err.Error(), "path escapes workspace") {
		t.Fatalf("expected workspace escape error, got %q", err.Error())
	}
}

func TestSearchFilesToolFindsMatchingFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "agent harness")
	writeTestFile(t, root, "docs/notes.txt", "nothing here")
	writeTestFile(t, root, "internal/tool.txt", "workspace agent tool")
	tool := tools.NewSearchFilesTool(root, 10_000)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"agent","path":"."}`))
	if err != nil {
		t.Fatalf("execute search_files: %v", err)
	}

	if !strings.Contains(got, "README.md: agent harness") || !strings.Contains(got, "internal/tool.txt: workspace agent tool") {
		t.Fatalf("expected matching files and lines, got %q", got)
	}
	if strings.Contains(got, "docs/notes.txt") {
		t.Fatalf("did not expect non-matching file, got %q", got)
	}
}

func TestFileToolsCapOutputSize(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "large.txt", strings.Repeat("a", 64))
	tool := tools.NewReadFileTool(root, 16)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"large.txt"}`))
	if err != nil {
		t.Fatalf("execute read_file: %v", err)
	}

	if !strings.Contains(got, "[truncated after 16 bytes]") {
		t.Fatalf("expected truncation notice, got %q", got)
	}
	if len(got) > 80 {
		t.Fatalf("expected capped output plus short notice, got length %d", len(got))
	}
}

func TestReadFileToolMissingFileReturnsUsefulError(t *testing.T) {
	root := t.TempDir()
	tool := tools.NewReadFileTool(root, 10_000)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"missing.txt"}`))
	if err == nil {
		t.Fatal("expected missing file error")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("expected file not found error, got %q", err.Error())
	}
}

func TestWorkspaceFileToolSchemasDeclareArguments(t *testing.T) {
	root := t.TempDir()
	tests := []tools.Tool{
		tools.NewListFilesTool(root, 10_000),
		tools.NewReadFileTool(root, 10_000),
		tools.NewSearchFilesTool(root, 10_000),
	}

	for _, tool := range tests {
		t.Run(tool.Name(), func(t *testing.T) {
			if tool.Description() == "" {
				t.Fatal("expected description")
			}
			if tool.JSONSchema()["type"] != "object" {
				t.Fatalf("expected object schema, got %#v", tool.JSONSchema())
			}
		})
	}
}

func writeTestFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	bytes, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return bytes
}

func TestFileToolsBlockEnvVariantsAndCaseVariants(t *testing.T) {
	for _, name := range []string{".env.local", ".env.production", ".ENV", ".Env.Local", "deploy.PEM", "Server.Key"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, name, "canary-secret-value")
			writeTestFile(t, root, "README.md", "safe agent notes")

			listed, err := tools.NewListFilesTool(root, 10_000).Execute(context.Background(), json.RawMessage(`{"path":"."}`))
			if err != nil {
				t.Fatalf("execute list_files: %v", err)
			}
			if strings.Contains(strings.ToLower(listed), strings.ToLower(name)) {
				t.Fatalf("expected %s to be skipped in listings, got %q", name, listed)
			}

			_, err = tools.NewReadFileTool(root, 10_000).Execute(context.Background(), mustJSON(t, map[string]string{"path": name}))
			if err == nil || !strings.Contains(err.Error(), "path is not allowed") {
				t.Fatalf("expected %s read to be blocked, got %v", name, err)
			}

			searched, err := tools.NewSearchFilesTool(root, 10_000).Execute(context.Background(), json.RawMessage(`{"query":"canary-secret-value","path":"."}`))
			if err != nil {
				t.Fatalf("execute search_files: %v", err)
			}
			if strings.Contains(searched, "canary-secret-value") {
				t.Fatalf("expected %s to be skipped in search, got %q", name, searched)
			}
		})
	}
}

func TestReadFileToolBlocksTrailingDotAndSpaceSpellings(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".env", "canary-secret-value")
	readTool := tools.NewReadFileTool(root, 10_000)

	// Windows strips trailing dots and spaces, so these open .env there.
	for _, name := range []string{".env.", ".env ", ".env. ."} {
		_, err := readTool.Execute(context.Background(), mustJSON(t, map[string]string{"path": name}))
		if err == nil || !strings.Contains(err.Error(), "path is not allowed") {
			t.Fatalf("expected %q read to be blocked, got %v", name, err)
		}
	}
}

func TestFileToolsAllowEnvTemplates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".env.example", "OPENAI_API_KEY=your-openai-api-key-here")

	got, err := tools.NewReadFileTool(root, 10_000).Execute(context.Background(), json.RawMessage(`{"path":".env.example"}`))
	if err != nil {
		t.Fatalf("expected .env.example to be readable, got %v", err)
	}
	if !strings.Contains(got, "your-openai-api-key-here") {
		t.Fatalf("unexpected .env.example content %q", got)
	}
}

func TestSearchFilesToolDoesNotFollowSymlinkOutsideWorkspace(t *testing.T) {
	outside := t.TempDir()
	writeTestFile(t, outside, "token.json", `{"refresh_token":"canary-outside-secret"}`)
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "safe agent notes")
	symlinkOrSkip(t, filepath.Join(outside, "token.json"), filepath.Join(root, "innocent.txt"))

	searched, err := tools.NewSearchFilesTool(root, 10_000).Execute(context.Background(), json.RawMessage(`{"query":"canary-outside-secret","path":"."}`))
	if err != nil {
		t.Fatalf("execute search_files: %v", err)
	}
	if strings.Contains(searched, "canary-outside-secret") {
		t.Fatalf("search_files followed a symlink out of the workspace: %q", searched)
	}
}

func TestSearchFilesToolDoesNotFollowSymlinkToBlockedFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".env", "OPENAI_API_KEY=canary-blocked-secret")
	symlinkOrSkip(t, filepath.Join(root, ".env"), filepath.Join(root, "notes.txt"))

	searched, err := tools.NewSearchFilesTool(root, 10_000).Execute(context.Background(), json.RawMessage(`{"query":"canary-blocked-secret","path":"."}`))
	if err != nil {
		t.Fatalf("execute search_files: %v", err)
	}
	if strings.Contains(searched, "canary-blocked-secret") {
		t.Fatalf("search_files read a blocked file through a symlink: %q", searched)
	}
}

func TestSearchFilesToolFollowsSymlinkInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "docs/guide.md", "workspace-local-marker")
	symlinkOrSkip(t, filepath.Join(root, "docs", "guide.md"), filepath.Join(root, "guide-link.md"))

	searched, err := tools.NewSearchFilesTool(root, 10_000).Execute(context.Background(), json.RawMessage(`{"query":"workspace-local-marker","path":"."}`))
	if err != nil {
		t.Fatalf("execute search_files: %v", err)
	}
	if !strings.Contains(searched, "guide-link.md: workspace-local-marker") {
		t.Fatalf("expected in-workspace symlink to be searched, got %q", searched)
	}
}

// symlinkOrSkip creates a file symlink, skipping on Windows hosts where the
// user lacks the symlink privilege (Developer Mode off, not elevated).
func symlinkOrSkip(t *testing.T, target string, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink on this host: %v", err)
	}
}
