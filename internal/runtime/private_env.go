package runtime

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var privateKeyName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_API_KEY$`)

// ParsePrivateEnvironment accepts literal, unquoted NAME_API_KEY=value records.
// It deliberately is not a shell/dotenv interpreter and cannot change PATH,
// PowerShell settings, proxy configuration, or executable search behavior.
func ParsePrivateEnvironment(raw []byte) (map[string]string, error) {
	if len(raw) > 1024*1024 || bytes.ContainsRune(raw, 0) {
		return nil, fmt.Errorf("invalid private environment file")
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || !privateKeyName.MatchString(key) || value == "" {
			return nil, fmt.Errorf("invalid private environment entry (expected NAME_API_KEY=value)")
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate private environment entry")
		}
		values[key] = value
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("cannot parse private environment file")
	}
	return values, nil
}

// LoadPrivateEnvironment is used at Windows daemon startup only. Values override
// the inherited environment. No credential or source line appears in errors.
func LoadPrivateEnvironment(paths Paths) error {
	path := filepath.Join(paths.ConfigDir, "secrets", "env")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("cannot read private environment file")
	}
	if err := checkPrivateEnvironmentFile(path, info); err != nil {
		return err
	}
	if info.Size() > 1024*1024 {
		return fmt.Errorf("private environment file too large")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read private environment file")
	}
	values, err := ParsePrivateEnvironment(raw)
	if err != nil {
		return err
	}
	for key, value := range values {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("cannot set private environment entry")
		}
	}
	return nil
}
