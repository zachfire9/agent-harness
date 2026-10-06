package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const defaultGmailAPIEndpoint = "https://gmail.googleapis.com"

type gmailNotifier struct {
	paths         Paths
	google        GoogleConfig
	config        GmailNotifierConfig
	endpoint      string
	oauthEndpoint string
	client        *http.Client
}

type googleAccessTokenFile struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Expiry       string `json:"expiry"`
	Scope        string `json:"scope"`
}

func (n gmailNotifier) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client := n.client
	if client == nil {
		client = http.DefaultClient
	}
	token, err := EnsureGoogleAccessToken(ctx, n.paths, n.google, client, n.oauthEndpoint, time.Minute)
	if err != nil {
		return err
	}
	if !tokenHasScope(token.Scope, GoogleScopeGmailSend) {
		return fmt.Errorf("google token is missing required gmail.send scope")
	}
	if token.AccessToken == "" {
		return fmt.Errorf("google token is missing access token")
	}

	payload := map[string]string{"raw": encodeGmailRawMessage(n.config, message)}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(n.endpoint, "/")
	if endpoint == "" {
		endpoint = defaultGmailAPIEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/gmail/v1/users/me/messages/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gmail send request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gmail send failed with status %d", resp.StatusCode)
	}
	return nil
}

func tokenHasScope(scopeText string, required string) bool {
	for _, scope := range strings.Fields(scopeText) {
		if scope == required {
			return true
		}
	}
	return false
}

func encodeGmailRawMessage(config GmailNotifierConfig, message Message) string {
	subject := "agent-harness notification"
	if strings.TrimSpace(config.SubjectPrefix) != "" {
		subject = strings.TrimSpace(config.SubjectPrefix) + " " + message.Job
	}
	headers := []string{
		"From: " + (&mail.Address{Address: config.From}).String(),
		"To: " + strings.Join(config.To, ", "),
		"Subject: " + subject,
		"Content-Type: text/plain; charset=UTF-8",
	}
	raw := strings.Join(headers, "\r\n") + "\r\n\r\n" + message.Body + "\r\n"
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
