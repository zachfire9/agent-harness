package onboarding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalIntegration is confined to the interactive settings directory. It never
// installs globally, changes machine PATH, or removes a user's executable/files.
type LocalIntegration struct {
	SettingsPath, Source string
	Run                  func(string) (string, error)
}
type ownership struct {
	Path         bool   `json:"path_owned"`
	ShortcutHash string `json:"shortcut_hash,omitempty"`
}

func (p LocalIntegration) binDir() string { return filepath.Join(filepath.Dir(p.SettingsPath), "bin") }
func (p LocalIntegration) exe() string    { return filepath.Join(p.binDir(), "agent-harness.exe") }
func (p LocalIntegration) shortcut() string {
	return filepath.Join(filepath.Dir(p.SettingsPath), "Chat with Agent Harness.lnk")
}
func (p LocalIntegration) manifest() string {
	return filepath.Join(filepath.Dir(p.SettingsPath), "integration.json")
}
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func hashFile(path string) string {
	b, e := os.ReadFile(path)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (p LocalIntegration) owner() (ownership, error) {
	var o ownership
	b, e := os.ReadFile(p.manifest())
	if os.IsNotExist(e) {
		return o, nil
	}
	if e != nil || json.Unmarshal(b, &o) != nil {
		return o, errors.New("cannot read integration ownership; no changes made")
	}
	return o, nil
}
func (p LocalIntegration) run(s string) (string, error) {
	if p.Run == nil {
		return "", errors.New("Windows integration unavailable")
	}
	v, e := p.Run("$ErrorActionPreference='Stop'\n" + s)
	if e != nil {
		return "", errors.New("Windows user integration failed")
	}
	return strings.TrimSpace(v), nil
}
func containsPath(path, entry string) bool {
	for _, s := range strings.Split(path, ";") {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(s), `\/`), strings.TrimRight(entry, `\/`)) {
			return true
		}
	}
	return false
}
func (p LocalIntegration) Review(o IntegrationOptions, workspace string) (string, error) {
	for _, dir := range []string{filepath.Dir(p.SettingsPath), p.binDir()} {
		if info, e := os.Lstat(dir); e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return "", errors.New("integration directory is not an ordinary directory")
		}
	}
	if o.Path {
		raw, e := p.run("[Environment]::GetEnvironmentVariable('Path','User')")
		if e != nil {
			return "", e
		}
		for _, dir := range strings.Split(raw, ";") {
			if dir == "" || strings.EqualFold(filepath.Clean(dir), filepath.Clean(p.binDir())) {
				continue
			}
			if _, e := os.Stat(filepath.Join(dir, "agent-harness.exe")); e == nil {
				return "", errors.New("another agent-harness.exe exists on user PATH; resolve installation conflict explicitly before enabling PATH")
			}
		}
	}

	if !filepath.IsAbs(p.Source) || !filepath.IsAbs(p.SettingsPath) || !filepath.IsAbs(workspace) || strings.ContainsAny(p.Source+p.SettingsPath+workspace, "\r\n\x00") {
		return "", errors.New("integration requires absolute single-line paths")
	}
	own, e := p.owner()
	if e != nil {
		return "", e
	}
	if o.Copy {
		if info, e := os.Lstat(p.exe()); e == nil {
			if !info.Mode().IsRegular() || hashFile(p.exe()) != hashFile(p.Source) {
				return "", errors.New("stable executable conflicts with an existing installation; move it explicitly before retrying")
			}
		} else if !os.IsNotExist(e) {
			return "", errors.New("cannot inspect stable executable")
		}
	}
	if (o.Path || o.Shortcut) && !o.Copy {
		if _, e = os.Stat(p.exe()); e != nil {
			return "", errors.New("select stable executable copy before PATH/shortcut integration")
		}
	}
	if o.Shortcut {
		if _, e = os.Lstat(p.shortcut()); e == nil && (own.ShortcutHash == "" || hashFile(p.shortcut()) != own.ShortcutHash) {
			return "", errors.New("shortcut already exists and is not an unchanged owned shortcut")
		}
	}
	return fmt.Sprintf("Copy %t: %s -> %s (different existing files are never overwritten)\nuser PATH %t: %s (existing entries skipped; machine PATH untouched)\nShortcut %t: %s; working directory %s; arguments chat --workspace saved --settings <absolute settings path>; no keys in arguments.", o.Copy, p.Source, p.exe(), o.Path, p.binDir(), o.Shortcut, p.shortcut(), workspace), nil
}
func (p LocalIntegration) Apply(o IntegrationOptions, workspace string) error {
	if _, e := p.Review(o, workspace); e != nil {
		return e
	}
	own, e := p.owner()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(p.binDir(), 0700); e != nil {
		return e
	}
	persist := func() error { b, _ := json.Marshal(own); return os.WriteFile(p.manifest(), b, 0600) }
	if o.Copy && hashFile(p.exe()) == "" {
		b, e := os.ReadFile(p.Source)
		if e != nil {
			return errors.New("cannot read executable")
		}
		f, e := os.OpenFile(p.exe(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if e != nil {
			return errors.New("stable executable appeared during setup; retry review")
		}
		_, e = f.Write(b)
		ce := f.Close()
		if e != nil || ce != nil {
			return errors.New("executable copy failed")
		}
	}
	if o.Path {
		raw, e := p.run("[Environment]::GetEnvironmentVariable('Path','User')")
		if e != nil {
			return e
		}
		if !containsPath(raw, p.binDir()) {
			if _, e = p.run("$p=[string][Environment]::GetEnvironmentVariable('Path','User'); [Environment]::SetEnvironmentVariable('Path',($p.TrimEnd(';')+';'+" + psQuote(p.binDir()) + "),'User')"); e != nil {
				return e
			}
			own.Path = true
			if e = persist(); e != nil {
				return e
			}
		}
	}
	if o.Shortcut { // The executable is a console app: Windows opens a terminal for it.
		args := "chat --workspace saved --settings " + windowsQuote(p.SettingsPath)
		script := "$s=([Activator]::CreateInstance([type]::GetTypeFromProgID('WScript.Shell'))).CreateShortcut(" + psQuote(p.shortcut()) + "); $s.TargetPath=" + psQuote(p.exe()) + "; $s.Arguments=" + psQuote(args) + "; $s.WorkingDirectory=" + psQuote(workspace) + "; $s.Save()"
		if _, e = p.run(script); e != nil {
			return e
		}
		own.ShortcutHash = hashFile(p.shortcut())
		if own.ShortcutHash == "" {
			return errors.New("shortcut was not created")
		}
		if e = persist(); e != nil {
			return e
		}
	}
	return persist()
}
func (p LocalIntegration) Remove() error {
	own, e := p.owner()
	if e != nil {
		return e
	}
	if own.Path {
		script := "$p=[string][Environment]::GetEnvironmentVariable('Path','User'); $parts=@($p.Split(';') | Where-Object { $_.TrimEnd('\\','/') -ine " + psQuote(strings.TrimRight(p.binDir(), `\/`)) + " }); [Environment]::SetEnvironmentVariable('Path',($parts -join ';'),'User')"
		if _, e = p.run(script); e != nil {
			return e
		}
		own.Path = false
	}
	if own.ShortcutHash != "" {
		if h := hashFile(p.shortcut()); h != "" && h != own.ShortcutHash {
			return errors.New("shortcut changed outside setup; retained for manual review")
		}
		if e = os.Remove(p.shortcut()); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	if e = os.Remove(p.manifest()); e != nil && !os.IsNotExist(e) {
		return e
	}
	return nil
}

// windowsQuote follows CommandLineToArgvW escaping, without invoking a shell.
func windowsQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, c := range s {
		if c == '\\' {
			slashes++
			continue
		}
		if c == '"' {
			b.WriteString(strings.Repeat("\\", slashes*2+1))
			b.WriteRune(c)
		} else {
			b.WriteString(strings.Repeat("\\", slashes))
			b.WriteRune(c)
		}
		slashes = 0
	}
	b.WriteString(strings.Repeat("\\", slashes*2))
	b.WriteByte('"')
	return b.String()
}
