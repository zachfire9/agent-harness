package version

import (
	"strings"
	"testing"
)

func TestInfoUsesDevelopmentDefaults(t *testing.T) {
	info := Info()

	if info.Version != "dev" {
		t.Fatalf("expected default version dev, got %q", info.Version)
	}
	if info.Commit != "unknown" {
		t.Fatalf("expected default commit unknown, got %q", info.Commit)
	}
	if info.BuildDate != "unknown" {
		t.Fatalf("expected default build date unknown, got %q", info.BuildDate)
	}
	if info.Dirty != "unknown" {
		t.Fatalf("expected default dirty state unknown, got %q", info.Dirty)
	}
}

func TestFormatHumanIncludesMetadata(t *testing.T) {
	info := Metadata{
		Version:   "0.1.0-dev",
		Commit:    "abc1234",
		BuildDate: "2026-10-01T14:00:00Z",
		Dirty:     "false",
	}

	got := info.FormatHuman()

	for _, want := range []string{
		"version: 0.1.0-dev",
		"commit: abc1234",
		"build_date: 2026-10-01T14:00:00Z",
		"dirty: false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected human version output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestSanitizeVersionForArtifactName(t *testing.T) {
	tests := map[string]string{
		"0.1.0-dev":       "0.1.0-dev",
		"release/0.1.0":   "release_0.1.0",
		"feature build 🚀": "feature_build__",
		"":                "dev",
	}

	for input, want := range tests {
		if got := SanitizeForArtifact(input); got != want {
			t.Fatalf("SanitizeForArtifact(%q) = %q, want %q", input, got, want)
		}
	}
}
