package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestScheduledNotificationPreservesExistingGoogleDocCursors(t *testing.T) {
	for _, prefix := range []string{"", "google_doc:"} {
		t.Run("stored_prefix="+prefix, func(t *testing.T) {
			paths, err := LinuxPaths(t.TempDir(), "default")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
			cfg := scheduledNotificationTestConfig()
			cfg.Context = []NotificationContextConfig{
				{ID: "first", Type: "rotating_source_item", Source: NotificationSourceConfig{Type: "google_doc", Alias: "first-doc", Parser: ProgressParserNonEmptyLines}, Selection: NotificationSelectionConfig{Strategy: ProgressStrategyRoundRobin, ProgressKey: "first-progress"}},
				{ID: "second", Type: "rotating_source_item", Source: NotificationSourceConfig{Type: "google_doc", Alias: "second-doc", Parser: ProgressParserNonEmptyLines}, Selection: NotificationSelectionConfig{Strategy: ProgressStrategyRoundRobin, ProgressKey: "second-progress"}},
			}
			cfg.Render.Prompt = "{{ first }} / {{ second }}"
			texts := map[string]string{"first-doc": "A0\nA1\nA2", "second-doc": "B0\nB1\nB2\nB3"}
			reader := func(_ context.Context, _ Paths, _ JobConfig, alias string) (GoogleDocSource, error) {
				return GoogleDocSource{Alias: alias, Text: texts[alias]}, nil
			}
			initial := []ProgressCursor{{NextIndex: 2, Cycle: 4}, {NextIndex: 1, Cycle: 7}}
			for i, item := range cfg.Context {
				items, err := ParseProgressItems(item.Source.Parser, texts[item.Source.Alias])
				if err != nil {
					t.Fatal(err)
				}
				state := ProgressState{
					Version: 1, Key: item.Selection.ProgressKey, Strategy: ProgressStrategyRoundRobin,
					Cursor:   initial[i],
					Snapshot: ProgressSnapshot{Parser: item.Source.Parser, SourceAlias: prefix + item.Source.Alias, ItemIDs: progressItemIDs(items), Cycle: initial[i].Cycle, CreatedAt: now.Add(-time.Hour)},
				}
				if err := WriteProgressState(paths, state); err != nil {
					t.Fatal(err)
				}
			}

			steps := []struct {
				prompt  string
				fail    bool
				cursors []ProgressCursor
				ids     []string
			}{
				{"A2 / B1", true, initial, []string{"a2", "b1"}},
				{"A2 / B1", false, []ProgressCursor{{NextIndex: 0, Cycle: 5}, {NextIndex: 2, Cycle: 7}}, []string{"a2", "b1"}},
				{"A0 / B2", false, []ProgressCursor{{NextIndex: 1, Cycle: 5}, {NextIndex: 3, Cycle: 7}}, []string{"a0", "b2"}},
				{"A1 / B3", false, []ProgressCursor{{NextIndex: 2, Cycle: 5}, {NextIndex: 0, Cycle: 8}}, []string{"a1", "b3"}},
			}
			for n, step := range steps {
				client := &recordingLLMClient{response: "body"}
				notifier := &recordingNotifier{}
				if step.fail {
					notifier.err = errors.New("send failed")
				}
				state, err := runScheduledNotificationWithClients(paths, now.Add(time.Duration(n)*time.Minute), cfg, notifier, client, reader)
				if step.fail {
					if err == nil || len(notifier.messages) != 0 {
						t.Fatalf("step %d: expected failed delivery, got %v", n, err)
					}
				} else if err != nil || state.Status != JobSucceeded || len(notifier.messages) != 1 {
					t.Fatalf("step %d: expected successful delivery, got %#v, %v", n, state, err)
				}
				if len(client.requests) != 1 || client.requests[0].Messages[1].Content != step.prompt {
					t.Fatalf("step %d: expected prompt %q, got %#v", n, step.prompt, client.requests)
				}
				for i, item := range cfg.Context {
					progress, err := ReadProgressState(paths, item.Selection.ProgressKey)
					if err != nil {
						t.Fatal(err)
					}
					if progress.Cursor != step.cursors[i] {
						t.Fatalf("step %d, key %s: got cursor %#v, want %#v", n, progress.Key, progress.Cursor, step.cursors[i])
					}
					if step.fail {
						if progress.History.PendingItemID != step.ids[i] || progress.History.LastSelectedID != "" {
							t.Fatalf("failed delivery changed history: %#v", progress.History)
						}
					} else if progress.History.PendingItemID != "" || progress.History.LastSelectedID != step.ids[i] {
						t.Fatalf("successful delivery did not complete selected item: %#v", progress.History)
					}
					// Existing snapshots stay intact until their own cycle wraps.
					if n < 2 && (progress.Snapshot.SourceAlias != prefix+item.Source.Alias || !progress.Snapshot.CreatedAt.Equal(now.Add(-time.Hour))) {
						t.Fatalf("existing snapshot unexpectedly refreshed: %#v", progress.Snapshot)
					}
				}
			}
		})
	}
}

func TestProgressSourceCompatibilityDoesNotHideSourceChanges(t *testing.T) {
	for _, tc := range []struct {
		name, stored, requested, sourceType string
		refresh                             bool
	}{
		{"legacy Google alias", "cards", "google_doc:cards", "google_doc", false},
		{"typed Google alias", "google_doc:cards", "google_doc:cards", "google_doc", false},
		{"changed legacy alias", "other", "google_doc:cards", "google_doc", true},
		{"changed typed alias", "google_doc:other", "google_doc:cards", "google_doc", true},
		{"local identity unchanged", "local_file:cards", "local_file:cards", "local_file", false},
		{"local path changed", "local_file:other", "local_file:cards", "local_file", true},
		{"legacy Google to local", "cards", "local_file:cards", "local_file", true},
		{"typed Google to local", "google_doc:cards", "local_file:cards", "local_file", true},
		{"local to Google", "local_file:cards", "google_doc:cards", "google_doc", true},
		{"local identity is not a legacy alias", "local_file:cards", "google_doc:local_file:cards", "google_doc", true},
		{"typed Google identity is not a legacy alias", "google_doc:cards", "google_doc:google_doc:cards", "google_doc", true},
		{"untyped request retains exact comparison", "cards", "google_doc:cards", "", true},
		{"arbitrary prefix not stripped", "other:cards", "google_doc:cards", "google_doc", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths, err := LinuxPaths(t.TempDir(), "default")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
			items := []ProgressItem{{ID: "first", Title: "First"}, {ID: "second", Title: "Second"}}
			state := ProgressState{Version: 1, Key: "compat", Strategy: ProgressStrategyRoundRobin,
				Cursor:   ProgressCursor{NextIndex: 1, Cycle: 3},
				Snapshot: ProgressSnapshot{Parser: ProgressParserNonEmptyLines, SourceAlias: tc.stored, ItemIDs: progressItemIDs(items), Cycle: 3, CreatedAt: now.Add(-time.Hour)},
			}
			if err := WriteProgressState(paths, state); err != nil {
				t.Fatal(err)
			}
			req := ProgressSelectionRequest{Key: state.Key, SourceType: tc.sourceType, SourceAlias: tc.requested, Parser: state.Snapshot.Parser, Items: items, Now: now}
			selection, err := SelectNextProgressItem(paths, req)
			if err != nil {
				t.Fatal(err)
			}
			wantIndex := 1
			if tc.refresh {
				wantIndex = 0
			}
			if selection.State.Cursor.NextIndex != wantIndex || selection.Item.ID != items[wantIndex].ID {
				t.Fatalf("got selection %#v, want index %d", selection, wantIndex)
			}
			// Parser changes must still refresh even when the source is compatible.
			req.Parser = ProgressParserHeadingWithBullets
			if !shouldRefreshProgressSnapshot(selection.State, req) {
				t.Fatal("parser change did not refresh snapshot")
			}
		})
	}
}
