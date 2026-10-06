package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultGoogleDocsAPIEndpoint = "https://docs.googleapis.com"

type GoogleDocSource struct {
	Alias      string
	DocumentID string
	Title      string
	Text       string
}

type googleDocsDocument struct {
	Title string `json:"title"`
	Body  struct {
		Content []googleDocsStructuralElement `json:"content"`
	} `json:"body"`
}

type googleDocsStructuralElement struct {
	Paragraph *googleDocsParagraph `json:"paragraph"`
}

type googleDocsParagraph struct {
	Elements []googleDocsParagraphElement `json:"elements"`
}

type googleDocsParagraphElement struct {
	TextRun *googleDocsTextRun `json:"textRun"`
}

type googleDocsTextRun struct {
	Content string `json:"content"`
}

func ReadGoogleDocSource(ctx context.Context, paths Paths, cfg RuntimeConfig, alias string) (GoogleDocSource, error) {
	return readGoogleDocSourceWithClient(ctx, paths, cfg, alias, http.DefaultClient, defaultGoogleDocsAPIEndpoint, defaultGoogleOAuthTokenEndpoint)
}

func readGoogleDocSourceWithClient(ctx context.Context, paths Paths, cfg RuntimeConfig, alias string, client *http.Client, docsEndpoint string, oauthEndpoint string) (GoogleDocSource, error) {
	alias = strings.TrimSpace(alias)
	documentID := strings.TrimSpace(cfg.Sources.GoogleDocs[alias])
	if documentID == "" {
		return GoogleDocSource{}, fmt.Errorf("unknown google docs source %q", alias)
	}
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(docsEndpoint) == "" {
		docsEndpoint = defaultGoogleDocsAPIEndpoint
	}
	token, err := EnsureGoogleAccessToken(ctx, paths, cfg.Google, client, oauthEndpoint, time.Minute)
	if err != nil {
		return GoogleDocSource{}, err
	}
	if !googleTokenHasAnyScope(token.Scope, GoogleScopeDocsReadonly, GoogleScopeDocuments) {
		return GoogleDocSource{}, fmt.Errorf("google token is missing required documents.readonly or documents scope")
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return GoogleDocSource{}, fmt.Errorf("google token is missing access token")
	}

	endpoint := strings.TrimRight(docsEndpoint, "/") + "/v1/documents/" + url.PathEscape(documentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GoogleDocSource{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err := client.Do(req)
	if err != nil {
		return GoogleDocSource{}, fmt.Errorf("google docs read request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GoogleDocSource{}, fmt.Errorf("google docs read failed with status %d", resp.StatusCode)
	}
	var document googleDocsDocument
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return GoogleDocSource{}, fmt.Errorf("read google docs document metadata: %w", err)
	}
	return GoogleDocSource{
		Alias:      alias,
		DocumentID: documentID,
		Title:      document.Title,
		Text:       extractGoogleDocText(document),
	}, nil
}

func googleTokenHasAnyScope(scopeList string, wants ...string) bool {
	wanted := map[string]bool{}
	for _, want := range wants {
		wanted[want] = true
	}
	for _, scope := range strings.Fields(scopeList) {
		if wanted[scope] {
			return true
		}
	}
	return false
}

func extractGoogleDocText(document googleDocsDocument) string {
	var b strings.Builder
	for _, block := range document.Body.Content {
		if block.Paragraph == nil {
			continue
		}
		for _, element := range block.Paragraph.Elements {
			if element.TextRun == nil {
				continue
			}
			b.WriteString(element.TextRun.Content)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
