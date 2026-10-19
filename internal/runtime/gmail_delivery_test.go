package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zachfire9/agent-harness/internal/llm"
)

// This is an isolated smoke test: real MIME and HTTP, fake model/token,
// temporary instance, and no Google or model-provider requests.
func TestGmailMultipartDeliveryProgressAndSafeState(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	token := `{"access_token":"fake-private-token","scope":"https://www.googleapis.com/auth/gmail.send","expiry":"2099-01-02T03:04:05Z"}`
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", token)
	writeRuntimeTestFile(t, paths.Home, "data/cards.txt", "Cedar\n- private-source-body\nMaple\n- next-source-body\n")
	cfg := scheduledNotificationTestConfig()
	cfg.Context = []NotificationContextConfig{
		{ID: "today_date", Type: "current_date", Timezone: "UTC", Format: "2006-01-02"},
		{ID: "card", Type: "rotating_source_item", Source: NotificationSourceConfig{Type: "local_file", Path: "data/cards.txt", Parser: ProgressParserHeadingWithBullets}, Selection: NotificationSelectionConfig{Strategy: ProgressStrategyRoundRobin, ProgressKey: "cards"}},
	}
	cfg.Render.Prompt = "private-prompt {{ today_date }} {{ card }}"
	body := "# Café 日報\n\nprivate-generated-body <script>bad()</script>\n\n- One\n- Two"
	model := &recordingLLMClient{response: body}
	type request struct {
		raw, path, method, auth, contentType string
		err                                  error
	}
	requests := make(chan request, 1)
	status := http.StatusServiceUnavailable
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		err := json.NewDecoder(r.Body).Decode(&payload)
		requests <- request{payload["raw"], r.URL.Path, r.Method, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), err}
		w.WriteHeader(status)
		fmt.Fprint(w, "private-response-body "+body+" "+payload["raw"]+" "+token)
	}))
	defer server.Close()
	runtimeConfig := RuntimeConfig{Jobs: []JobConfig{cfg}, Google: GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "gmail_send"}, Notifier: NotifierConfig{Type: "gmail", Gmail: GmailNotifierConfig{From: "agent@example.com", To: []string{"reader@example.com"}, BodyFormat: "multipart"}}}
	registry := map[string]Job{"scheduled_notification": scheduledNotificationJob{
		clientFactory: func(LLMConfig) (llm.ChatClient, error) { return model, nil },
		notifierFactory: func(p Paths, c JobConfig) Notifier {
			return gmailNotifier{paths: p, google: c.Google, config: c.Notifier.Gmail, endpoint: server.URL, client: server.Client()}
		},
	}}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	for attempt := 0; attempt < 2; attempt++ {
		state, runErr := runConfiguredJobNowWithRegistry(paths, runtimeConfig, cfg.Name, now.Add(time.Duration(attempt)*time.Minute), registry)
		if attempt == 0 {
			if runErr == nil || runErr.Error() != "gmail send failed with status 503" || state.Status != JobFailed {
				t.Fatalf("unsafe or missing failure: %#v %v", state, runErr)
			}
		} else if runErr != nil || state.Status != JobSucceeded {
			t.Fatalf("retry failed: %#v %v", state, runErr)
		}
		got := <-requests
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.path != "/gmail/v1/users/me/messages/send" || got.method != "POST" || got.auth != "Bearer fake-private-token" || got.contentType != "application/json" {
			t.Fatal("wrong HTTP request metadata")
		}
		_, parts := decodeGmailBody(t, got.raw)
		if parts[0] != body || !strings.Contains(parts[1], "&lt;script&gt;") {
			t.Fatalf("incorrect MIME bodies: %q", parts)
		}
		if len(model.requests) != attempt+1 {
			t.Fatal("expected one model request per job run")
		}
		if prompt := model.requests[attempt].Messages[1].Content; prompt != "private-prompt 2026-01-02 Cedar\nprivate-source-body" {
			t.Fatalf("wrong prompt %q", prompt)
		}
		progress, err := ReadProgressState(paths, "cards")
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			if progress.Cursor.NextIndex != 0 || progress.History.PendingItemID != "cedar" || progress.History.LastSelectedID != "" {
				t.Fatalf("failure advanced progress: %#v", progress)
			}
		} else if progress.Cursor.NextIndex != 1 || progress.History.PendingItemID != "" || progress.History.LastSelectedID != "cedar" {
			t.Fatalf("success did not advance once: %#v", progress)
		}
		for _, path := range []string{paths.JobsPath, filepath.Join(paths.StateDir, "progress", "cards.json")} {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"private-source-body", "private-prompt", "private-generated-body", "private-response-body", "fake-private-token", token, got.raw, "multipart/alternative", "<script>"} {
				if strings.Contains(string(raw), secret) {
					t.Fatalf("state leaked sensitive content in %s", path)
				}
			}
		}
		status = http.StatusOK
	}
}
