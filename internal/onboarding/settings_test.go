package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsRoundTripAndNoSecret(t *testing.T) {
	p := filepath.Join(t.TempDir(), "space ü", "settings.json")
	s := Settings{BaseURL: "https://api.openai.com/v1", Model: "fake-model", Workspace: t.TempDir(), CredentialRef: "agent-harness/test"}
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil || got != s {
		t.Fatalf("round trip: %#v %v", got, err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "api_key") {
		t.Fatal("secret field persisted")
	}
}
func TestSettingsRejectUnsafeEndpointAndWorkspace(t *testing.T) {
	for _, base := range []string{"https://user:secret@example.com/v1", "https://example.com/v1?key=secret", "http://example.com/v1", "file:///tmp/x"} {
		if err := (Settings{BaseURL: base, Model: "fake", Workspace: t.TempDir()}).Validate(); err == nil {
			t.Fatalf("accepted %s", base)
		}
	}
	if err := (Settings{BaseURL: "https://example.com/v1", Model: "fake", Workspace: "relative"}).Validate(); err == nil {
		t.Fatal("relative workspace accepted")
	}
}
func TestReadMissingAndMalformed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	s, err := Read(p)
	if err != nil || s != (Settings{}) {
		t.Fatal(s, err)
	}
	os.WriteFile(p, []byte(`{"api_key":"fake-secret"}`), 0600)
	if _, err = Read(p); err == nil || strings.Contains(err.Error(), "fake-secret") {
		t.Fatal("must fail closed without echoing content")
	}
}
