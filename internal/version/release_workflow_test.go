package version

import (
	"os"
	"path/filepath"
	"regexp"
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

// usesLine matches a workflow step's `uses:` reference to a remote action.
var usesLine = regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s*([^\s#]+)`)

// pinnedAction requires owner/repo@<40-hex commit SHA>; tags and branches can
// be moved to different code after review, a full commit SHA cannot.
var pinnedAction = regexp.MustCompile(`^[\w.-]+/[\w.-]+(?:/[\w./-]+)?@[0-9a-f]{40}$`)

func TestWorkflowActionsArePinnedToCommitSHAs(t *testing.T) {
	for _, name := range []string{"test.yml", "release.yml"} {
		workflow := readRepoFile(t, ".github", "workflows", name)
		matches := usesLine.FindAllStringSubmatch(workflow, -1)
		if len(matches) == 0 {
			t.Fatalf("%s: expected at least one uses: reference", name)
		}
		for _, match := range matches {
			ref := match[1]
			if strings.HasPrefix(ref, "./") {
				continue // local reusable workflow in this repository
			}
			if !pinnedAction.MatchString(ref) {
				t.Errorf("%s: action %q must be pinned to a full 40-character commit SHA", name, ref)
			}
		}
	}
}

func TestWorkflowCheckoutDoesNotPersistCredentials(t *testing.T) {
	for _, name := range []string{"test.yml", "release.yml"} {
		workflow := readRepoFile(t, ".github", "workflows", name)
		checkouts := strings.Count(workflow, "actions/checkout@")
		persisted := strings.Count(workflow, "persist-credentials: false")
		if checkouts == 0 || persisted < checkouts {
			t.Errorf("%s: every actions/checkout step must set persist-credentials: false (%d checkouts, %d opt-outs)", name, checkouts, persisted)
		}
	}
}

func TestCIScansForKnownVulnerabilities(t *testing.T) {
	ci := readRepoFile(t, ".github", "workflows", "test.yml")
	if !strings.Contains(ci, "golang.org/x/vuln/cmd/govulncheck@v") {
		t.Fatalf("expected CI to run a version-pinned govulncheck, got:\n%s", ci)
	}
	if strings.Contains(ci, "govulncheck@latest") {
		t.Fatalf("govulncheck must be pinned to a release, not @latest")
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
