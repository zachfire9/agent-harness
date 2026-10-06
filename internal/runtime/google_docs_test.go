package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRuntimeConfigParsesGoogleDocsSources(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	writeRuntimeTestFile(t, paths.Home, "config/config.yaml", `google:
  token_path: "config/secrets/google-token.json"
  scope_profile: "docs_readonly"
sources:
  google_docs:
    vocabulary_doc_id: "doc-vocabulary-123"
    daily_prompt: "doc-daily-456"
`)

	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := cfg.Sources.GoogleDocs["vocabulary_doc_id"]; got != "doc-vocabulary-123" {
		t.Fatalf("expected vocabulary doc id, got %q", got)
	}
	if got := cfg.Sources.GoogleDocs["daily_prompt"]; got != "doc-daily-456" {
		t.Fatalf("expected daily prompt doc id, got %q", got)
	}
}

func TestReadRuntimeConfigRejectsEmptyGoogleDocsSourceID(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	writeRuntimeTestFile(t, paths.Home, "config/config.yaml", `sources:
  google_docs:
    vocabulary_doc_id: ""
`)

	_, err := ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), `google docs source "vocabulary_doc_id" missing document id`) {
		t.Fatalf("expected missing google docs document id error, got %v", err)
	}
}

func TestReadGoogleDocSourceUsesConfiguredDocIDAndReadonlyScope(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", `{"access_token":"ya29.docs-token","scope":"https://www.googleapis.com/auth/documents.readonly","expiry":"2099-01-02T03:04:05Z"}`)

	var gotPath string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprintln(w, `{"title":"Vocabulary","body":{"content":[{"paragraph":{"elements":[{"textRun":{"content":"First line\n"}},{"textRun":{"content":"Second line"}}]}}]}}`)
	}))
	defer server.Close()

	cfg := RuntimeConfig{
		Google:  GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "docs_readonly"},
		Sources: SourceConfig{GoogleDocs: map[string]string{"vocabulary_doc_id": "doc-vocabulary-123"}},
	}
	doc, err := readGoogleDocSourceWithClient(context.Background(), paths, cfg, "vocabulary_doc_id", http.DefaultClient, server.URL, defaultGoogleOAuthTokenEndpoint)
	if err != nil {
		t.Fatalf("read google doc source: %v", err)
	}
	if gotPath != "/v1/documents/doc-vocabulary-123" || gotAuth != "Bearer ya29.docs-token" {
		t.Fatalf("unexpected docs request path/auth: path=%q auth=%q", gotPath, gotAuth)
	}
	if doc.Alias != "vocabulary_doc_id" || doc.DocumentID != "doc-vocabulary-123" || doc.Title != "Vocabulary" {
		t.Fatalf("unexpected document metadata: %#v", doc)
	}
	if doc.Text != "First line\nSecond line" {
		t.Fatalf("unexpected document text: %q", doc.Text)
	}
}

func TestReadGoogleDocSourceRejectsUnknownAliasAndWrongScope(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", `{"access_token":"ya29.docs-token","scope":"https://www.googleapis.com/auth/gmail.send","expiry":"2099-01-02T03:04:05Z"}`)
	cfg := RuntimeConfig{
		Google:  GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "docs_readonly"},
		Sources: SourceConfig{GoogleDocs: map[string]string{"vocabulary_doc_id": "doc-vocabulary-123"}},
	}

	_, err := readGoogleDocSourceWithClient(context.Background(), paths, cfg, "missing_alias", http.DefaultClient, "https://docs.example", defaultGoogleOAuthTokenEndpoint)
	if err == nil || !strings.Contains(err.Error(), `unknown google docs source "missing_alias"`) {
		t.Fatalf("expected unknown source error, got %v", err)
	}

	_, err = readGoogleDocSourceWithClient(context.Background(), paths, cfg, "vocabulary_doc_id", http.DefaultClient, "https://docs.example", defaultGoogleOAuthTokenEndpoint)
	if err == nil || !strings.Contains(err.Error(), "google token is missing required documents.readonly scope") {
		t.Fatalf("expected missing docs scope error, got %v", err)
	}
}

func TestExtractGoogleDocTextDoesNotIncludeSecretsFromErrors(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", `{"access_token":"ya29.secret-doc-token","scope":"https://www.googleapis.com/auth/documents.readonly","expiry":"2099-01-02T03:04:05Z"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"denied"}`, http.StatusForbidden)
	}))
	defer server.Close()

	cfg := RuntimeConfig{
		Google:  GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "docs_readonly"},
		Sources: SourceConfig{GoogleDocs: map[string]string{"vocabulary_doc_id": "doc-vocabulary-123"}},
	}
	_, err := readGoogleDocSourceWithClient(context.Background(), paths, cfg, "vocabulary_doc_id", http.DefaultClient, server.URL, defaultGoogleOAuthTokenEndpoint)
	if err == nil || !strings.Contains(err.Error(), "google docs read failed with status 403") {
		t.Fatalf("expected sanitized docs read error, got %v", err)
	}
	if strings.Contains(err.Error(), "ya29.secret-doc-token") || strings.Contains(err.Error(), "access_token") {
		t.Fatalf("docs read error leaked token material: %v", err)
	}
}

func TestGoogleDocsSourceCanRefreshExpiredReadonlyToken(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", `{"access_token":"ya29.expired-token","refresh_token":"refresh-token","scope":"https://www.googleapis.com/auth/documents.readonly","expiry":"2000-01-02T03:04:05Z"}`)
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-client.json", `{"installed":{"client_id":"client-id","client_secret":"client-secret"}}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			fmt.Fprintln(w, `{"access_token":"ya29.fresh-doc-token","expires_in":3600,"scope":"https://www.googleapis.com/auth/documents.readonly"}`)
		case "/v1/documents/doc-vocabulary-123":
			if got := r.Header.Get("Authorization"); got != "Bearer ya29.fresh-doc-token" {
				t.Fatalf("expected refreshed token, got %q", got)
			}
			fmt.Fprintln(w, `{"title":"Vocabulary","body":{"content":[{"paragraph":{"elements":[{"textRun":{"content":"Fresh text"}}]}}]}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := RuntimeConfig{
		Google:  GoogleConfig{ClientCredentialsPath: "config/secrets/google-client.json", TokenPath: "config/secrets/google-token.json", ScopeProfile: "docs_readonly"},
		Sources: SourceConfig{GoogleDocs: map[string]string{"vocabulary_doc_id": "doc-vocabulary-123"}},
	}
	doc, err := readGoogleDocSourceWithClient(context.Background(), paths, cfg, "vocabulary_doc_id", http.DefaultClient, server.URL, server.URL+"/token")
	if err != nil {
		t.Fatalf("read google doc source after refresh: %v", err)
	}
	if doc.Text != "Fresh text" {
		t.Fatalf("unexpected text: %q", doc.Text)
	}
}
