package onboarding

import (
	"strings"
	"testing"
)

func TestWizardRerunPreservesLoggingPreference(t *testing.T) {
	w, u, _ := wizardFixture(t)
	s := Settings{BaseURL: "https://example.com/v1", Model: "fake", Workspace: w.DefaultWorkspace, RunLogs: true}
	if err := Save(w.Path, s); err != nil {
		t.Fatal(err)
	}
	u.answers = answers("session", "yes")
	u.answers[4] = "" // Accept the existing preference, not a reset to first-run defaults.
	r, err := w.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !r.Settings.RunLogs {
		t.Fatal("rerun lost existing diagnostic logging preference")
	}
}

func TestWizardCancelAtHiddenPromptDoesNotStoreCredential(t *testing.T) {
	w, u, store := wizardFixture(t)
	u.answers = answers("store", "yes")
	u.secret = "cancel"
	_, err := w.Run()
	if err == nil || !strings.Contains(err.Error(), "cancel") || store.writes != 0 {
		t.Fatal("hidden prompt cancellation must not store a credential")
	}
}
