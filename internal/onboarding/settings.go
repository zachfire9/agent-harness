// Package onboarding manages interactive preferences, never daemon configuration.
package onboarding

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Settings struct {
	BaseURL       string `json:"base_url"`
	Model         string `json:"model"`
	Workspace     string `json:"workspace"`
	CredentialRef string `json:"credential_ref,omitempty"`
	RunLogs       bool   `json:"run_logs"`
}

func (s Settings) Validate() error {
	u, e := url.Parse(s.BaseURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return errors.New("endpoint must be HTTPS (HTTP allowed only for loopback), without credentials, query or fragment")
	}
	if strings.TrimSpace(s.Model) == "" || strings.ContainsAny(s.Model, "\r\n\x00") {
		return errors.New("model is required and must be a single line")
	}
	if !filepath.IsAbs(s.Workspace) || strings.ContainsAny(s.Workspace, "\r\n\x00") {
		return errors.New("workspace must be an absolute directory")
	}
	return nil
}
func Read(path string) (Settings, error) {
	var s Settings
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, errors.New("cannot read onboarding settings")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&s) != nil {
		return Settings{}, errors.New("invalid onboarding settings; rerun setup or remove settings")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Settings{}, errors.New("invalid onboarding settings")
	}
	if e = s.Validate(); e != nil {
		return Settings{}, e
	}
	return s, nil
}
func Save(path string, s Settings) error {
	if e := s.Validate(); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return errors.New("cannot create settings directory")
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	f, e := os.CreateTemp(filepath.Dir(path), "settings-*.tmp")
	if e != nil {
		return errors.New("cannot create settings file")
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return errors.New("cannot write settings")
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return errors.New("cannot replace settings")
	}
	return nil
}
func DefaultPath() (string, error) {
	d, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "agent-harness", "interactive", "settings.json"), nil
}

type CredentialStore interface {
	Get(string) (string, error)
	Put(string, string) error
	Delete(string) error
}

const CredentialTarget = "agent-harness/interactive/default"
