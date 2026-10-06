package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	GoogleScopeProfileGmailSend    = "gmail_send"
	GoogleScopeProfileDocsReadonly = "docs_readonly"

	GoogleScopeGmailSend    = "https://www.googleapis.com/auth/gmail.send"
	GoogleScopeDocsReadonly = "https://www.googleapis.com/auth/documents.readonly"
	GoogleScopeDocuments    = "https://www.googleapis.com/auth/documents"

	defaultGoogleOAuthTokenEndpoint = "https://oauth2.googleapis.com/token"
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

type googleOAuthClientCredentialsFile struct {
	Installed googleOAuthClientCredentials `json:"installed"`
	Web       googleOAuthClientCredentials `json:"web"`
}

type googleOAuthClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type googleOAuthRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// EnsureGoogleAccessToken returns a usable access token, refreshing and rewriting the
// configured token file when the saved token is expired or close to expiry. It never
// returns token values in error messages.
func EnsureGoogleAccessToken(ctx context.Context, paths Paths, cfg GoogleConfig, client *http.Client, tokenEndpoint string, refreshSkew time.Duration) (googleAccessTokenFile, error) {
	if err := cfg.Validate(); err != nil {
		return googleAccessTokenFile{}, err
	}
	cfg = cfg.WithDefaults()
	tokenPath := filepath.Join(paths.Home, filepath.FromSlash(cfg.TokenPath))
	token, err := readGoogleAccessTokenFile(tokenPath)
	if err != nil {
		return googleAccessTokenFile{}, err
	}
	if !googleTokenNeedsRefresh(token, refreshSkew) {
		return token, nil
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return googleAccessTokenFile{}, fmt.Errorf("google token cannot be refreshed")
	}
	creds, err := readGoogleOAuthClientCredentials(filepath.Join(paths.Home, filepath.FromSlash(cfg.ClientCredentialsPath)))
	if err != nil {
		return googleAccessTokenFile{}, err
	}
	refreshed, err := refreshGoogleAccessToken(ctx, client, tokenEndpoint, creds, token.RefreshToken)
	if err != nil {
		return googleAccessTokenFile{}, err
	}
	if refreshed.AccessToken == "" {
		return googleAccessTokenFile{}, fmt.Errorf("google token refresh returned no access token")
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	if refreshed.Scope == "" {
		refreshed.Scope = token.Scope
	}
	if refreshed.ExpiresIn <= 0 {
		refreshed.ExpiresIn = 3600
	}
	updated := googleAccessTokenFile{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
		Scope:        refreshed.Scope,
		Expiry:       time.Now().UTC().Add(time.Duration(refreshed.ExpiresIn) * time.Second).Format(time.RFC3339),
	}
	if err := writeGoogleAccessTokenFile(tokenPath, updated); err != nil {
		return googleAccessTokenFile{}, err
	}
	return updated, nil
}

func readGoogleAccessTokenFile(path string) (googleAccessTokenFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return googleAccessTokenFile{}, fmt.Errorf("google token is missing")
		}
		return googleAccessTokenFile{}, fmt.Errorf("read google token metadata: %w", err)
	}
	var token googleAccessTokenFile
	if err := json.Unmarshal(raw, &token); err != nil {
		return googleAccessTokenFile{}, fmt.Errorf("read google token metadata: %w", err)
	}
	return token, nil
}

func writeGoogleAccessTokenFile(path string, token googleAccessTokenFile) error {
	raw, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func googleTokenNeedsRefresh(token googleAccessTokenFile, refreshSkew time.Duration) bool {
	if strings.TrimSpace(token.Expiry) == "" {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, token.Expiry)
	if err != nil {
		return false
	}
	return !time.Now().UTC().Add(refreshSkew).Before(expiry.UTC())
}

func readGoogleOAuthClientCredentials(path string) (googleOAuthClientCredentials, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return googleOAuthClientCredentials{}, fmt.Errorf("google client credentials are missing")
		}
		return googleOAuthClientCredentials{}, fmt.Errorf("read google client credentials metadata: %w", err)
	}
	var file googleOAuthClientCredentialsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return googleOAuthClientCredentials{}, fmt.Errorf("read google client credentials metadata: %w", err)
	}
	creds := file.Installed
	if creds.ClientID == "" && creds.ClientSecret == "" {
		creds = file.Web
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return googleOAuthClientCredentials{}, fmt.Errorf("google client credentials are incomplete")
	}
	return creds, nil
}

func refreshGoogleAccessToken(ctx context.Context, client *http.Client, tokenEndpoint string, creds googleOAuthClientCredentials, refreshToken string) (googleOAuthRefreshResponse, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(tokenEndpoint) == "" {
		tokenEndpoint = defaultGoogleOAuthTokenEndpoint
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", creds.ClientID)
	form.Set("client_secret", creds.ClientSecret)
	form.Set("refresh_token", refreshToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return googleOAuthRefreshResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return googleOAuthRefreshResponse{}, fmt.Errorf("google token refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleOAuthRefreshResponse{}, fmt.Errorf("google token refresh failed with status %d", resp.StatusCode)
	}
	var refreshed googleOAuthRefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshed); err != nil {
		return googleOAuthRefreshResponse{}, fmt.Errorf("read google token refresh metadata: %w", err)
	}
	return refreshed, nil
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
