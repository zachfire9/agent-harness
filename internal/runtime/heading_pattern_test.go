package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHeadingPatternGroupsPlainAndBulletLines(t *testing.T) {
	input := "Preamble\n- ignored\n\n 1 - Cedar \r\n Plain fact \n2024 was a good year\n\n- First bullet\n* Second bullet\n• Third bullet\n- \n2 - Maple\nAnother fact\n3 - Birch\n"
	items, err := ParseProgressItemsWithHeadingPattern("heading_pattern", input, `^[0-9]+\s*-\s*\S+`)
	if err != nil {
		t.Fatal(err)
	}
	want := []ProgressItem{
		{ID: "1-cedar", Title: "1 - Cedar", Body: []string{"Plain fact", "2024 was a good year", "First bullet", "Second bullet", "Third bullet"}},
		{ID: "2-maple", Title: "2 - Maple", Body: []string{"Another fact"}},
		{ID: "3-birch", Title: "3 - Birch"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %#v, want %#v", items, want)
	}
	again, err := ParseProgressItemsWithHeadingPattern("heading_pattern", input, `^[0-9]+\s*-\s*\S+`)
	if err != nil || !reflect.DeepEqual(items, again) {
		t.Fatalf("unstable items: %#v, %v", again, err)
	}
}

func TestHeadingPatternUsesConfiguredBoundary(t *testing.T) {
	items, err := ParseProgressItemsWithHeadingPattern("heading_pattern", "ignored\nCARD: A\n1 - not a heading\nCARD: B\nend", `^CARD:`)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || !reflect.DeepEqual(items[0].Body, []string{"1 - not a heading"}) || items[1].Title != "CARD: B" {
		t.Fatalf("unexpected items: %#v", items)
	}
	items, err = ParseProgressItemsWithHeadingPattern("heading_pattern", "no matching heading\n- fact\n", `^CARD:`)
	if err != nil || len(items) != 0 {
		t.Fatalf("preamble became items: %#v, %v", items, err)
	}
}

func TestHeadingPatternRejectsInvalidPatterns(t *testing.T) {
	for _, pattern := range []string{"", "  ", "[", "^", ".*", "a*", "(?:card)?"} {
		t.Run(pattern, func(t *testing.T) {
			if _, err := ParseProgressItemsWithHeadingPattern("heading_pattern", "CARD: A", pattern); err == nil || !strings.Contains(err.Error(), "heading_pattern") {
				t.Fatalf("expected heading_pattern error, got %v", err)
			}
		})
	}
	if _, err := ParseProgressItems("heading_pattern", "CARD: A"); err == nil || !strings.Contains(err.Error(), "heading_pattern") {
		t.Fatalf("missing pattern accepted: %v", err)
	}
}

func TestHeadingPatternConfigLoadsAndValidates(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		valid   bool
	}{
		{`^[0-9]+\s*-\s*\S+`, true}, {`^CARD:`, true}, {"", false}, {"   ", false}, {"[", false}, {".*", false}, {"^", false},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			paths, err := LinuxPaths(t.TempDir(), "default")
			if err != nil {
				t.Fatal(err)
			}
			if err := InitInstance(paths); err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf(`llms:
  default:
    provider: openai
    model: fake-model
  providers:
    - provider: openai
      api_key_env: FAKE_API_KEY
      models:
        - fake-model
jobs:
  - name: cards
    type: scheduled_notification
    enabled: true
    interval_seconds: 86400
    context:
      - id: card
        type: rotating_source_item
        source:
          type: local_file
          path: data/cards.txt
          parser: heading_pattern
          heading_pattern: '%s'
        selection:
          strategy: round_robin
          progress_key: cards
    render:
      type: llm_template
      prompt: "Card: {{ card }}"
      max_chars: 1000
`, tc.pattern)
			if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := ReadRuntimeConfig(paths)
			if !tc.valid {
				if err == nil || !strings.Contains(err.Error(), "heading_pattern") {
					t.Fatalf("expected config heading_pattern error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Jobs[0].Context[0].Source.HeadingPattern; got != tc.pattern {
				t.Fatalf("pattern lost: %q", got)
			}
		})
	}
}

func TestHeadingPatternContextRotationsAdvanceIndependentlyAndWrap(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	cfg := scheduledNotificationTestConfig()
	cfg.Context = []NotificationContextConfig{
		{ID: "card", Type: "rotating_source_item", Source: NotificationSourceConfig{Type: "google_doc", Alias: "cards", Parser: "heading_pattern", HeadingPattern: `^CARD:`}, Selection: NotificationSelectionConfig{Strategy: "round_robin", ProgressKey: "cards"}},
		{ID: "word", Type: "rotating_source_item", Source: NotificationSourceConfig{Type: "google_doc", Alias: "words", Parser: "non_empty_lines"}, Selection: NotificationSelectionConfig{Strategy: "round_robin", ProgressKey: "words"}},
	}
	reader := func(_ context.Context, _ Paths, _ JobConfig, alias string) (GoogleDocSource, error) {
		if alias == "cards" {
			return GoogleDocSource{Text: "ignore\nCARD: A\nPlain fact\n2024 fact\nCARD: B\n- Bullet fact"}, nil
		}
		return GoogleDocSource{Text: "Alpha\nBeta\nGamma"}, nil
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, want := range []string{"CARD: A\nPlain fact\n2024 fact", "CARD: B\nBullet fact", "CARD: A\nPlain fact\n2024 fact"} {
		values, pending, err := resolveNotificationContext(paths, now, cfg, reader)
		if err != nil {
			t.Fatal(err)
		}
		if values["card"] != want || values["word"] != "Alpha" {
			t.Fatalf("iteration %d: %#v", i, values)
		}
		state, err := ReadProgressState(paths, "cards")
		if err != nil {
			t.Fatal(err)
		}
		if state.Cursor.NextIndex != i%2 || state.Cursor.Cycle != i/2 {
			t.Fatalf("selection advanced: %#v", state.Cursor)
		}
		if err := MarkProgressItemCompleted(paths, pending[0].key, "wrong-id", now); err == nil {
			t.Fatal("wrong completion accepted")
		}
		if err := MarkProgressItemCompleted(paths, pending[0].key, pending[0].itemID, now); err != nil {
			t.Fatal(err)
		}
	}
	words, err := ReadProgressState(paths, "words")
	if err != nil {
		t.Fatal(err)
	}
	if words.Cursor.NextIndex != 0 || words.Cursor.Cycle != 0 {
		t.Fatalf("independent rotation advanced: %#v", words)
	}
}
