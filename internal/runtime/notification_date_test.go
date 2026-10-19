package runtime

import (
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
		{"localized opt in", NotificationContextConfig{Format: "{weekday_es} {day_es} {month_es} 2006"}, "viernes dos enero 2026"},
		{"literal newline", NotificationContextConfig{Format: `2006-01-02\n{weekday_it}`}, "2026-01-02\nvenerdì"},
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
	if _, err := currentDateValue(NotificationContextConfig{Timezone: "Invalid/Zone"}, time.Now()); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}
