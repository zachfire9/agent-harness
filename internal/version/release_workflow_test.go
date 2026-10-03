package version

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubReleaseWorkflowPublishesVersionTags(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")

	for _, want := range []string{
		"on:",
		"push:",
		"tags:",
		"v*",
		"go test ./...",
		"scripts/build-release.sh",
		"gh release create",
		"dist/agent-harness_${VERSION}_linux_amd64.tar.gz",
		"dist/agent-harness_${VERSION}_linux_arm64.tar.gz",
		"dist/checksums.txt",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("expected release workflow to contain %q, got:\n%s", want, workflow)
		}
	}
}

func TestGitHubReleaseWorkflowDerivesBinaryVersionFromTag(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")

	for _, want := range []string{
		"GITHUB_REF_NAME#v",
		"agent-harness_${VERSION}_linux_amd64.tar.gz",
		"agent-harness_${VERSION}_linux_arm64.tar.gz",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("expected workflow to derive artifact version from tag without leading v using %q, got:\n%s", want, workflow)
		}
	}
}

func TestInstallDocsDescribeAutomatedReleasePublishing(t *testing.T) {
	docs := readRepoFile(t, "docs", "install-release.md")

	for _, want := range []string{
		"Publishing is automated by GitHub Actions",
		`git tag "v${VERSION}"`,
		`git push origin "v${VERSION}"`,
		"https://github.com/zachfire9/agent-harness/releases/download/v${VERSION}/agent-harness_${VERSION}_${ARCH}.tar.gz",
	} {
		if !strings.Contains(docs, want) {
			t.Fatalf("expected install docs to contain %q, got:\n%s", want, docs)
		}
	}
	if strings.Contains(docs, "gh release create") {
		t.Fatalf("install docs should no longer require manual gh release create publishing, got:\n%s", docs)
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read repo file %s: %v", path, err)
	}
	return string(content)
}
