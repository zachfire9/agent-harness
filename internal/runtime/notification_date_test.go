package runtime

import (
	"strings"
	"testing"
	"time"
)

func TestCurrentDateValueFixedTime(t *testing.T) {
	now := time.Date(2026, 1, 2, 1, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		config NotificationContextConfig
		want   string
	}{
		{"default UTC", NotificationContextConfig{}, "Friday January 2, 2026"},
		{"timezone boundary", NotificationContextConfig{Timezone: "America/New_York", Format: "2006-01-02"}, "2026-01-01"},
		{"custom layout", NotificationContextConfig{Format: "Monday, January 2, 2006"}, "Friday, January 2, 2026"},
		{"no language-specific expansion", NotificationContextConfig{Format: "{weekday_es} {day_es} {month_es} 2006"}, "{weekday_es} {day_es} {month_es} 2026"},
		{"literal newline", NotificationContextConfig{Format: `2006-01-02\nMonday`}, "2026-01-02\nFriday"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := currentDateValue(tc.config, now)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestCurrentDateValueRejectsInvalidTimezone(t *testing.T) {
	if _, err := currentDateValue(NotificationContextConfig{Timezone: "Invalid/Zone"}, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}

func TestScheduledNotificationPassesAuthoritativeDateAndConfiguredTranslationInstructions(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	cfg := scheduledNotificationTestConfig()
	cfg.Context = []NotificationContextConfig{{ID: "today_date", Type: "current_date", Timezone: "America/New_York", Format: "Monday, January 2, 2006"}}
	cfg.Render.Prompt = "The authoritative current date is {{ today_date }}. Use this date; do not infer today's date. Write the date in Spanish and Japanese. Spell out day numbers using natural conventions."
	now := time.Date(2026, 1, 2, 1, 30, 0, 0, time.UTC)
	client := &recordingLLMClient{response: "Example model-formatted date"}
	notifier := &recordingNotifier{}
	state, err := runScheduledNotificationWithClients(paths, now, cfg, notifier, client, nil)
	if err != nil || state.Status != JobSucceeded {
		t.Fatalf("run failed: %#v, %v", state, err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("want one model call, got %d", len(client.requests))
	}
	prompt := client.requests[0].Messages[1].Content
	want := strings.ReplaceAll(cfg.Render.Prompt, "{{ today_date }}", "Thursday, January 1, 2026")
	if prompt != want {
		t.Fatalf("got prompt %q, want %q", prompt, want)
	}
	if len(notifier.messages) != 1 || notifier.messages[0].Body != client.response {
		t.Fatalf("expected model output delivered unchanged, got %#v", notifier.messages)
	}
}
