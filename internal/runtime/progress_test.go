package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseProgressItemsNonEmptyLinesTrimsBlankLinesAndCreatesStableIDs(t *testing.T) {
	items, err := ParseProgressItems("non_empty_lines", "\n Perspicacious \n\nLaconic\n")
	if err != nil {
		t.Fatalf("parse items failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two items, got %#v", items)
	}
	if items[0].ID != "perspicacious" || items[0].Title != "Perspicacious" || len(items[0].Body) != 0 {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].ID != "laconic" || items[1].Title != "Laconic" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}

	again, err := ParseProgressItems("non_empty_lines", "Perspicacious\nLaconic\n")
	if err != nil {
		t.Fatalf("parse items again failed: %v", err)
	}
	if again[0].ID != items[0].ID || again[1].ID != items[1].ID {
		t.Fatalf("expected stable ids across parses: first=%#v again=%#v", items, again)
	}
}

func TestParseProgressItemsHeadingWithBulletsCreatesCards(t *testing.T) {
	input := `Perspicacious

- having keen mental perception
- example: Her perspicacious comment helped.

Laconic
- brief or terse in speech
`
	items, err := ParseProgressItems("heading_with_bullets", input)
	if err != nil {
		t.Fatalf("parse items failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two cards, got %#v", items)
	}
	if items[0].ID != "perspicacious" || items[0].Title != "Perspicacious" {
		t.Fatalf("unexpected first card metadata: %#v", items[0])
	}
	if !sameStrings(items[0].Body, []string{"having keen mental perception", "example: Her perspicacious comment helped."}) {
		t.Fatalf("unexpected first card body: %#v", items[0].Body)
	}
	if items[1].ID != "laconic" || !sameStrings(items[1].Body, []string{"brief or terse in speech"}) {
		t.Fatalf("unexpected second card: %#v", items[1])
	}
}

func TestRoundRobinProgressSelectsAdvancesPersistsAndWraps(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	items := []ProgressItem{{ID: "alpha", Title: "Alpha"}, {ID: "beta", Title: "Beta"}}
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	selection, err := SelectNextProgressItem(paths, ProgressSelectionRequest{
		Key:         "daily-vocab",
		SourceAlias: "vocabulary_doc_id",
		Parser:      "non_empty_lines",
		Items:       items,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("select first item failed: %v", err)
	}
	if selection.Item.ID != "alpha" || selection.State.Cursor.NextIndex != 0 {
		t.Fatalf("expected first selection without advancing, got %#v", selection)
	}

	if err := MarkProgressItemCompleted(paths, "daily-vocab", "alpha", now.Add(time.Minute)); err != nil {
		t.Fatalf("complete first item failed: %v", err)
	}

	selection, err = SelectNextProgressItem(paths, ProgressSelectionRequest{
		Key:         "daily-vocab",
		SourceAlias: "vocabulary_doc_id",
		Parser:      "non_empty_lines",
		Items:       items,
		Now:         now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("select second item failed: %v", err)
	}
	if selection.Item.ID != "beta" || selection.State.Cursor.NextIndex != 1 {
		t.Fatalf("expected second selection, got %#v", selection)
	}
	if err := MarkProgressItemCompleted(paths, "daily-vocab", "beta", now.Add(24*time.Hour+time.Minute)); err != nil {
		t.Fatalf("complete second item failed: %v", err)
	}

	selection, err = SelectNextProgressItem(paths, ProgressSelectionRequest{
		Key:         "daily-vocab",
		SourceAlias: "vocabulary_doc_id",
		Parser:      "non_empty_lines",
		Items:       items,
		Now:         now.Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("select wrapped item failed: %v", err)
	}
	if selection.Item.ID != "alpha" || selection.State.Cursor.Cycle != 1 || selection.State.Cursor.NextIndex != 0 {
		t.Fatalf("expected wrapped first item in cycle 1, got %#v", selection)
	}

	state, err := ReadProgressState(paths, "daily-vocab")
	if err != nil {
		t.Fatalf("read progress state failed: %v", err)
	}
	if state.Cursor.Cycle != 1 || state.Snapshot.Cycle != 1 || !sameStrings(state.Snapshot.ItemIDs, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected persisted progress state: %#v", state)
	}
}

func TestProgressSnapshotRefreshesOnlyAtCycleBoundary(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	initial := []ProgressItem{{ID: "alpha", Title: "Alpha"}, {ID: "beta", Title: "Beta"}}
	if _, err := SelectNextProgressItem(paths, ProgressSelectionRequest{Key: "daily-vocab", SourceAlias: "doc", Parser: "non_empty_lines", Items: initial, Now: now}); err != nil {
		t.Fatalf("select initial failed: %v", err)
	}
	if err := MarkProgressItemCompleted(paths, "daily-vocab", "alpha", now.Add(time.Minute)); err != nil {
		t.Fatalf("complete alpha failed: %v", err)
	}

	editedMidCycle := []ProgressItem{{ID: "new", Title: "New"}, {ID: "alpha", Title: "Alpha"}, {ID: "beta", Title: "Beta"}}
	selection, err := SelectNextProgressItem(paths, ProgressSelectionRequest{Key: "daily-vocab", SourceAlias: "doc", Parser: "non_empty_lines", Items: editedMidCycle, Now: now.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("select mid-cycle failed: %v", err)
	}
	if selection.Item.ID != "beta" || !sameStrings(selection.State.Snapshot.ItemIDs, []string{"alpha", "beta"}) {
		t.Fatalf("expected old snapshot mid-cycle, got %#v", selection)
	}
	if err := MarkProgressItemCompleted(paths, "daily-vocab", "beta", now.Add(24*time.Hour+time.Minute)); err != nil {
		t.Fatalf("complete beta failed: %v", err)
	}

	selection, err = SelectNextProgressItem(paths, ProgressSelectionRequest{Key: "daily-vocab", SourceAlias: "doc", Parser: "non_empty_lines", Items: editedMidCycle, Now: now.Add(48 * time.Hour)})
	if err != nil {
		t.Fatalf("select new cycle failed: %v", err)
	}
	if selection.Item.ID != "new" || !sameStrings(selection.State.Snapshot.ItemIDs, []string{"new", "alpha", "beta"}) {
		t.Fatalf("expected refreshed snapshot at cycle boundary, got %#v", selection)
	}
}

func TestProgressDoesNotAdvanceUntilCompletionAndRejectsWrongCompletionID(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	items := []ProgressItem{{ID: "alpha", Title: "Alpha"}, {ID: "beta", Title: "Beta"}}
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	selection, err := SelectNextProgressItem(paths, ProgressSelectionRequest{Key: "daily-vocab", SourceAlias: "doc", Parser: "non_empty_lines", Items: items, Now: now})
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	if selection.Item.ID != "alpha" {
		t.Fatalf("expected alpha, got %#v", selection.Item)
	}
	if err := MarkProgressItemCompleted(paths, "daily-vocab", "beta", now.Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "does not match pending item") {
		t.Fatalf("expected wrong completion id error, got %v", err)
	}
	selection, err = SelectNextProgressItem(paths, ProgressSelectionRequest{Key: "daily-vocab", SourceAlias: "doc", Parser: "non_empty_lines", Items: items, Now: now.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("select after failed completion failed: %v", err)
	}
	if selection.Item.ID != "alpha" || selection.State.Cursor.NextIndex != 0 {
		t.Fatalf("expected cursor not to advance after failed completion, got %#v", selection)
	}
}

func TestProgressStateDoesNotPersistFullSourceTextOrDeliveredBodies(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	sensitiveBody := "full definition that should not be persisted"
	items := []ProgressItem{{ID: "alpha", Title: "Alpha", Body: []string{sensitiveBody}}}
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if _, err := SelectNextProgressItem(paths, ProgressSelectionRequest{Key: "daily-vocab", SourceAlias: "doc", Parser: "heading_with_bullets", Items: items, Now: now}); err != nil {
		t.Fatalf("select failed: %v", err)
	}
	if err := MarkProgressItemCompleted(paths, "daily-vocab", "alpha", now.Add(time.Minute)); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(paths.StateDir, "progress", "daily-vocab.json"))
	if err != nil {
		t.Fatalf("read progress file failed: %v", err)
	}
	if strings.Contains(string(raw), sensitiveBody) || strings.Contains(string(raw), "delivered") {
		t.Fatalf("progress state should not persist source body or delivered content: %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("progress state should be valid json: %v", err)
	}
}
