package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"net/textproto"
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
	if err := validateNotifierConfig(NotifierConfig{Type: "gmail", Gmail: n.config}, n.google); err != nil {
		return err
	}
	if err := validateGmailHeaderValues(message.Job); err != nil {
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

func validateGmailHeaderValues(values ...string) error {
	for _, value := range values {
		for _, char := range value {
			if char < 32 || char == 127 || char == '\u0085' || char == '\u2028' || char == '\u2029' {
				return fmt.Errorf("gmail notifier header contains forbidden control characters")
			}
		}
	}
	return nil
}

func encodeGmailRawMessage(config GmailNotifierConfig, message Message) string {
	subject := "agent-harness notification"
	if strings.TrimSpace(config.SubjectPrefix) != "" {
		subject = strings.TrimSpace(config.SubjectPrefix) + " " + message.Job
	}
	subject = mime.QEncoding.Encode("UTF-8", subject)
	headers := []string{
		"From: " + (&mail.Address{Address: config.From}).String(),
		"To: " + strings.Join(config.To, ", "),
		"Subject: " + subject,
		"Content-Type: text/plain; charset=UTF-8",
	}
	if config.BodyFormat == "multipart" {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		// The existing job byte cap can split a UTF-8 sequence. Repair malformed
		// text for both rich alternatives without changing legacy plain bytes.
		textBody := strings.ToValidUTF8(message.Body, "\uFFFD")
		for i, text := range []string{textBody, renderGmailHTML(textBody)} {
			mediaType := []string{"text/plain", "text/html"}[i]
			part, _ := writer.CreatePart(textproto.MIMEHeader{
				"Content-Type":              {mediaType + "; charset=UTF-8"},
				"Content-Transfer-Encoding": {"quoted-printable"},
			})
			// These writers only target an in-memory bytes.Buffer: writes cannot fail.
			encoded := quotedprintable.NewWriter(part)
			_, _ = encoded.Write([]byte(normalizeGmailNewlines(text)))
			_ = encoded.Close()
		}
		_ = writer.Close()
		headers = append(headers[:3], "MIME-Version: 1.0", "Content-Type: "+mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": writer.Boundary()}))
		raw := strings.Join(headers, "\r\n") + "\r\n\r\n" + body.String()
		return base64.RawURLEncoding.EncodeToString([]byte(raw))
	}
	raw := strings.Join(headers, "\r\n") + "\r\n\r\n" + message.Body + "\r\n"
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
