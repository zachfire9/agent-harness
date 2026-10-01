package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	GoogleScopeProfileGmailSend    = "gmail_send"
	GoogleScopeProfileDocsReadonly = "docs_readonly"

	GoogleScopeGmailSend    = "https://www.googleapis.com/auth/gmail.send"
	GoogleScopeDocsReadonly = "https://www.googleapis.com/auth/documents.readonly"
)

// GoogleConfig contains paths and safe metadata for Google OAuth integration.
// Secret values are intentionally stored in files under the instance home, not inline config.
type GoogleConfig struct {
	ClientCredentialsPath string
	TokenPath             string
	AccountHint           string
	ScopeProfile          string
}

func (cfg GoogleConfig) configured() bool {
	return strings.TrimSpace(cfg.ClientCredentialsPath) != "" || strings.TrimSpace(cfg.TokenPath) != "" || strings.TrimSpace(cfg.AccountHint) != "" || strings.TrimSpace(cfg.ScopeProfile) != ""
}

func (cfg GoogleConfig) WithDefaults() GoogleConfig {
	if cfg.ClientCredentialsPath == "" {
		cfg.ClientCredentialsPath = "config/secrets/google-client.json"
	}
	if cfg.TokenPath == "" {
		cfg.TokenPath = "config/secrets/google-token.json"
	}
	return cfg
}

func (cfg GoogleConfig) Validate() error {
	if !cfg.configured() {
		return nil
	}
	cfg = cfg.WithDefaults()
	if err := validateInstanceRelativePath(cfg.ClientCredentialsPath); err != nil {
		return fmt.Errorf("invalid google client_credentials_path: %w", err)
	}
	if err := validateInstanceRelativePath(cfg.TokenPath); err != nil {
		return fmt.Errorf("invalid google token_path: %w", err)
	}
	if _, err := cfg.ScopeProfileScopes(); err != nil {
		return err
	}
	return nil
}

func (cfg GoogleConfig) ScopeProfileScopes() ([]string, error) {
	switch cfg.ScopeProfile {
	case GoogleScopeProfileGmailSend:
		return []string{GoogleScopeGmailSend}, nil
	case GoogleScopeProfileDocsReadonly:
		return []string{GoogleScopeDocsReadonly}, nil
	case "":
		if cfg.configured() {
			return nil, fmt.Errorf("google scope_profile is required")
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown google scope_profile %q", cfg.ScopeProfile)
	}
}

func (cfg GoogleConfig) Scopes() []string {
	scopes, err := cfg.ScopeProfileScopes()
	if err != nil {
		return nil
	}
	return scopes
}

type GoogleAuthStatusInfo struct {
	Connected    bool
	AccountHint  string
	ScopeProfile string
	Scopes       []string
	TokenExpiry  string
	TokenPath    string
}

func (s GoogleAuthStatusInfo) SafeString() string {
	var b strings.Builder
	fmt.Fprintln(&b, "google_auth:")
	fmt.Fprintf(&b, "  connected: %t\n", s.Connected)
	if s.AccountHint != "" {
		fmt.Fprintf(&b, "  account_hint: %s\n", s.AccountHint)
	}
	if s.ScopeProfile != "" {
		fmt.Fprintf(&b, "  scope_profile: %s\n", s.ScopeProfile)
	}
	for _, scope := range s.Scopes {
		fmt.Fprintf(&b, "  scope: %s\n", scope)
	}
	if s.TokenExpiry != "" {
		fmt.Fprintf(&b, "  token_expiry: %s\n", s.TokenExpiry)
	}
	if s.TokenPath != "" {
		fmt.Fprintf(&b, "  token_path: %s\n", s.TokenPath)
	}
	return b.String()
}

type googleTokenFile struct {
	Expiry string `json:"expiry"`
	Scope  string `json:"scope"`
}

func GoogleAuthStatus(paths Paths, cfg GoogleConfig) (GoogleAuthStatusInfo, error) {
	if !cfg.configured() {
		return GoogleAuthStatusInfo{}, fmt.Errorf("google config is not configured")
	}
	if err := cfg.Validate(); err != nil {
		return GoogleAuthStatusInfo{}, err
	}
	cfg = cfg.WithDefaults()
	scopes := cfg.Scopes()
	status := GoogleAuthStatusInfo{
		Connected:    false,
		AccountHint:  cfg.AccountHint,
		ScopeProfile: cfg.ScopeProfile,
		Scopes:       scopes,
		TokenPath:    cfg.TokenPath,
	}
	tokenPath := filepath.Join(paths.Home, filepath.FromSlash(cfg.TokenPath))
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return status, nil
		}
		return GoogleAuthStatusInfo{}, err
	}
	var token googleTokenFile
	if err := json.Unmarshal(raw, &token); err != nil {
		return GoogleAuthStatusInfo{}, fmt.Errorf("read google token metadata: %w", err)
	}
	status.Connected = true
	status.TokenExpiry = token.Expiry
	return status, nil
}

func RevokeGoogleToken(paths Paths, cfg GoogleConfig) error {
	if !cfg.configured() {
		return fmt.Errorf("google config is not configured")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg = cfg.WithDefaults()
	tokenPath := filepath.Join(paths.Home, filepath.FromSlash(cfg.TokenPath))
	if err := os.Remove(tokenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func GoogleAuthStartInstructions(paths Paths, cfg GoogleConfig) (string, error) {
	if !cfg.configured() {
		return "", fmt.Errorf("google config is not configured")
	}
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	cfg = cfg.WithDefaults()
	var b strings.Builder
	fmt.Fprintln(&b, "google auth start")
	if cfg.AccountHint != "" {
		fmt.Fprintf(&b, "account_hint: %s\n", cfg.AccountHint)
	}
	fmt.Fprintf(&b, "scope_profile: %s\n", cfg.ScopeProfile)
	for _, scope := range cfg.Scopes() {
		fmt.Fprintf(&b, "scope: %s\n", scope)
	}
	fmt.Fprintf(&b, "client_credentials_path: %s\n", cfg.ClientCredentialsPath)
	fmt.Fprintf(&b, "token_path: %s\n", cfg.TokenPath)
	fmt.Fprintf(&b, "secrets_dir: %s\n", filepath.Join(paths.Home, "config", "secrets"))
	fmt.Fprintln(&b, "next_step: complete OAuth flow with the configured client credentials; do not paste tokens into logs or status output")
	return b.String(), nil
}
