package onboarding

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeUI struct {
	answers []string
	secret  string
	output  bytes.Buffer
}

func (u *fakeUI) Ask(label, def string) (string, error) {
	u.output.WriteString(label + " " + def + "\n")
	if len(u.answers) == 0 {
		return "", errors.New("EOF")
	}
	v := u.answers[0]
	u.answers = u.answers[1:]
	if v == "" {
		v = def
	}
	return v, nil
}
func (u *fakeUI) Secret() (string, error) { return u.secret, nil }
func (u *fakeUI) Notice(s string)         { u.output.WriteString(s + "\n") }

type fakeStore struct {
	key    string
	fail   bool
	writes int
}

func (s *fakeStore) Get(string) (string, error) {
	if s.fail {
		return "", errors.New("fake-secret")
	}
	return s.key, nil
}
func (s *fakeStore) Put(_ string, v string) error {
	if s.fail {
		return errors.New("fake-secret")
	}
	s.key = v
	s.writes++
	return nil
}
func (s *fakeStore) Delete(string) error {
	if s.fail {
		return errors.New("fake-secret")
	}
	s.key = ""
	return nil
}
func wizardFixture(t *testing.T) (Wizard, *fakeUI, *fakeStore) {
	t.Helper()
	d := t.TempDir()
	u := &fakeUI{secret: "fake-hidden-key"}
	s := &fakeStore{}
	return Wizard{Path: filepath.Join(d, "settings.json"), DefaultWorkspace: filepath.Join(d, "workspace ü"), UI: u, Store: s}, u, s
}
func answers(mode, review string) []string {
	return []string{"", "", "", mode, "n", "n", "n", "n", review, "n", "n"}
}
func TestWizardDefaultsStoreAndRerun(t *testing.T) {
	w, u, s := wizardFixture(t)
	u.answers = answers("store", "yes")
	r, e := w.Run()
	if e != nil || !r.Saved || s.key != "fake-hidden-key" {
		t.Fatal(r, e)
	}
	got, e := Read(w.Path)
	if e != nil || got.RunLogs || got.CredentialRef != Target(w.Path) {
		t.Fatal(got, e)
	}
	if _, e = os.Stat(got.Workspace); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(u.output.String(), s.key) {
		t.Fatal("leaked key")
	}
	u.answers = answers("keep", "yes")
	_, e = w.Run()
	if e != nil || s.writes != 1 {
		t.Fatal(e, s.writes)
	}
}
func TestWizardCancelAndEOFAreSideEffectFree(t *testing.T) {
	for _, a := range [][]string{nil, answers("store", "no"), {"cancel"}} {
		w, u, s := wizardFixture(t)
		u.answers = a
		_, e := w.Run()
		if e == nil {
			t.Fatal("expected cancellation")
		}
		if _, e = os.Stat(w.Path); !os.IsNotExist(e) || s.writes != 0 {
			t.Fatal("side effects before review")
		}
	}
}
func TestWizardSessionOnlyAndRemove(t *testing.T) {
	w, u, s := wizardFixture(t)
	u.answers = answers("session", "yes")
	r, e := w.Run()
	if e != nil || r.SessionKey != "fake-hidden-key" || s.writes != 0 {
		t.Fatal(r, e)
	}
	got, _ := Read(w.Path)
	if got.CredentialRef != "" {
		t.Fatal(got)
	}
	u.answers = []string{"yes"}
	if e = w.Remove(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(got.Workspace); e != nil {
		t.Fatal("workspace removed")
	}
}
func TestWizardStoreFailureNeverPlaintext(t *testing.T) {
	w, u, s := wizardFixture(t)
	s.fail = true
	u.answers = answers("store", "yes")
	_, e := w.Run()
	if e == nil || strings.Contains(e.Error(), "fake-secret") {
		t.Fatal(e)
	}
	if _, e = os.Stat(w.Path); !os.IsNotExist(e) {
		t.Fatal("saved despite credential failure")
	}
}
func TestConsoleRejectsNonTTY(t *testing.T) {
	if _, e := NewConsole(strings.NewReader("yes\n"), &bytes.Buffer{}); e == nil {
		t.Fatal("non TTY accepted")
	}
}
