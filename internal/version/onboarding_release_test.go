package version

import (
	"strings"
	"testing"
)

func TestWindowsOnboardingPackagingAndGate(t *testing.T) {
	ci := readRepoFile(t, ".github", "workflows", "test.yml")
	if !strings.Contains(ci, "scripts/test-windows-onboarding.ps1") {
		t.Fatal("native onboarding must gate releases through reusable CI")
	}
	build := readRepoFile(t, "scripts", "build-release.sh")
	for _, want := range []string{"windows-first-conversation.md", "FIRST-CONVERSATION.md"} {
		if !strings.Contains(build, want) {
			t.Fatalf("ZIP missing beginner guide: %s", want)
		}
	}
	smoke := readRepoFile(t, "scripts", "test-windows-onboarding.ps1")
	for _, want := range []string{"Compress-Archive", "Expand-Archive", "HARNESS_ONBOARDING_SMOKE_BINARY", "TestNativeOnboardingSmoke", "finally"} {
		if !strings.Contains(smoke, want) {
			t.Fatalf("native smoke missing %s", want)
		}
	}
}
