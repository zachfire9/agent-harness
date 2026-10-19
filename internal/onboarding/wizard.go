package onboarding

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type UI interface {
	Ask(string, string) (string, error)
	Secret() (string, error)
	Notice(string)
}
type IntegrationOptions struct{ Copy, Path, Shortcut bool }
type Integration interface {
	Review(IntegrationOptions, string) (string, error)
	Apply(IntegrationOptions, string) error
	Remove() error
}
type Wizard struct {
	Path, DefaultWorkspace string
	UI                     UI
	Store                  CredentialStore
	Integration            Integration
	Check                  func(Settings, string) error
}
type Result struct {
	Saved, StartChat bool
	SessionKey       string
	Settings         Settings
}

func Target(path string) string {
	p, _ := filepath.Abs(path)
	h := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(p))))
	return fmt.Sprintf("agent-harness/interactive/%x", h[:16])
}
func (w Wizard) ask(label, def string) (string, error) {
	v, e := w.UI.Ask(label, def)
	if e != nil || strings.EqualFold(v, "cancel") {
		return "", errors.New("setup cancelled (EOF or cancel); no further changes made")
	}
	return strings.TrimSpace(v), nil
}
func (w Wizard) yes(label string) (bool, error) {
	return w.yesDefault(label, false)
}
func (w Wizard) yesDefault(label string, current bool) (bool, error) {
	def := "no"
	if current {
		def = "yes"
	}
	for {
		v, e := w.ask(label+" [yes/no]", def)
		if e != nil {
			return false, e
		}
		switch strings.ToLower(v) {
		case "yes", "y":
			return true, nil
		case "no", "n":
			return false, nil
		}
		w.UI.Notice("Enter yes or no, or cancel.")
	}
}
func (w Wizard) Run() (Result, error) {
	var r Result
	s, e := Read(w.Path)
	if e != nil {
		return r, e
	}
	if s.BaseURL == "" {
		s.BaseURL = "https://api.openai.com/v1"
		s.Model = "gpt-4.1-mini"
		s.Workspace = w.DefaultWorkspace
	}
	w.UI.Notice("API use is billed separately from chat subscriptions. Prompts and tool/file contents are sent to your provider. Type cancel at any prompt. No daemon configuration is changed.")
	for {
		s.BaseURL, e = w.ask("OpenAI-compatible endpoint", s.BaseURL)
		if e != nil {
			return r, e
		}
		s.Model, e = w.ask("Model", s.Model)
		if e != nil {
			return r, e
		}
		s.Workspace, e = w.ask("Workspace (only this folder is exposed to file tools; created after review)", s.Workspace)
		if e != nil {
			return r, e
		}
		if e = s.Validate(); e == nil {
			break
		}
		w.UI.Notice(e.Error())
	}
	modeDefault := "session"
	if s.CredentialRef != "" {
		modeDefault = "keep"
	}
	var mode, key string
	for {
		mode, e = w.ask("Credential: store (Windows Credential Manager), session (this setup chat only), keep, or remove", modeDefault)
		if e != nil {
			return r, e
		}
		if mode == "store" || mode == "session" || mode == "remove" || (mode == "keep" && s.CredentialRef == Target(w.Path)) {
			break
		}
		w.UI.Notice("Choose store/session/remove, or keep an existing credential.")
	}
	if mode == "store" || mode == "session" {
		key, e = w.UI.Secret()
		if e != nil {
			return r, errors.New("hidden key input unavailable; no credential saved")
		}
		if strings.EqualFold(strings.TrimSpace(key), "cancel") {
			return r, errors.New("setup cancelled; no credential saved")
		}
		if strings.TrimSpace(key) == "" {
			return r, errors.New("API key cannot be empty")
		}
	}
	s.RunLogs, e = w.yesDefault("Enable diagnostic run logs (may contain conversation/tool data; not resumed memory)?", s.RunLogs)
	if e != nil {
		return r, e
	}
	var opts IntegrationOptions
	opts.Copy, e = w.yes("Copy executable to stable per-user location (existing different file is a conflict)?")
	if e != nil {
		return r, e
	}
	opts.Path, e = w.yes("Add stable executable directory to user PATH (new terminals only)?")
	if e != nil {
		return r, e
	}
	opts.Shortcut, e = w.yes("Create Chat with Agent Harness terminal shortcut?")
	if e != nil {
		return r, e
	}
	review := "No executable/PATH/shortcut changes."
	if opts.Copy || opts.Path || opts.Shortcut {
		if w.Integration == nil {
			return r, errors.New("Windows integration unavailable")
		}
		review, e = w.Integration.Review(opts, s.Workspace)
		if e != nil {
			return r, e
		}
	}
	w.UI.Notice(fmt.Sprintf("Review: settings %s\nEndpoint %s; model %s\nWorkspace %s\nCredential action %s; diagnostic logs %t\n%s\nWorkspace files are never removed. Existing saved credentials are replaced only for store/remove/session. Provider-side revocation is separate.", w.Path, s.BaseURL, s.Model, s.Workspace, mode, s.RunLogs, review))
	yes, e := w.yes("Apply these changes?")
	if e != nil {
		return r, e
	}
	if !yes {
		return r, errors.New("setup cancelled; no changes made")
	}
	if info, err := os.Stat(s.Workspace); err == nil && !info.IsDir() {
		return r, errors.New("workspace is not a directory")
	}
	if e = os.MkdirAll(s.Workspace, 0700); e != nil {
		return r, errors.New("cannot create workspace")
	}
	switch mode {
	case "store":
		if w.Store == nil || w.Store.Put(Target(w.Path), key) != nil {
			return r, errors.New("Credential Manager write failed; nothing stored as plaintext")
		}
		s.CredentialRef = Target(w.Path)
	case "session", "remove":
		if s.CredentialRef != "" {
			if s.CredentialRef != Target(w.Path) || w.Store == nil || w.Store.Delete(s.CredentialRef) != nil {
				return r, errors.New("credential removal failed")
			}
		}
		s.CredentialRef = ""
	}
	if e = Save(w.Path, s); e != nil {
		return r, e
	}
	if opts.Copy || opts.Path || opts.Shortcut {
		if e = w.Integration.Apply(opts, s.Workspace); e != nil {
			return r, errors.New("settings saved, optional integration failed; rerun setup to retry")
		}
	}
	r.Saved = true
	r.Settings = s
	if mode == "session" {
		r.SessionKey = key
	}
	yes, e = w.yes("Test connection? Sends a small request and may incur API charges (offline validation has already passed).")
	if e != nil {
		return r, e
	}
	if yes {
		if key == "" && s.CredentialRef != "" && w.Store != nil {
			key, e = w.Store.Get(s.CredentialRef)
		}
		if e != nil || key == "" || w.Check == nil {
			return r, errors.New("connection check needs an available key")
		}
		if e = w.Check(s, key); e != nil {
			return r, e
		}
		w.UI.Notice("Connection check passed (not a guarantee of future provider availability).")
	}
	r.StartChat, e = w.yes("Start a new chat now? History is in memory only; model confirm:true is not human approval.")
	return r, e
}
func (w Wizard) Remove() error {
	s, e := Read(w.Path)
	if e != nil {
		return e
	}
	w.UI.Notice("Remove interactive settings, this profile's Credential Manager entry and owned optional integrations. Workspace files and copied executable are retained. Provider-side revocation is separate.")
	yes, e := w.yes("Remove?")
	if e != nil {
		return e
	}
	if !yes {
		return errors.New("removal cancelled")
	}
	if s.CredentialRef != "" {
		if s.CredentialRef != Target(w.Path) || w.Store == nil || w.Store.Delete(s.CredentialRef) != nil {
			return errors.New("credential removal failed")
		}
	}
	if w.Integration != nil {
		if e = w.Integration.Remove(); e != nil {
			return e
		}
	}
	if e = os.Remove(w.Path); e != nil && !os.IsNotExist(e) {
		return errors.New("cannot remove settings")
	}
	return nil
}
