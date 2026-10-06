package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	ProgressParserNonEmptyLines      = "non_empty_lines"
	ProgressParserHeadingWithBullets = "heading_with_bullets"
	ProgressStrategyRoundRobin       = "round_robin"
)

type ProgressItem struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Body  []string `json:"body,omitempty"`
}

type ProgressCursor struct {
	NextIndex int `json:"next_index"`
	Cycle     int `json:"cycle"`
}

type ProgressSnapshot struct {
	Parser      string    `json:"parser"`
	SourceAlias string    `json:"source_alias"`
	ItemIDs     []string  `json:"item_ids"`
	Cycle       int       `json:"cycle"`
	CreatedAt   time.Time `json:"created_at"`
}

type ProgressHistory struct {
	PendingItemID     string    `json:"pending_item_id,omitempty"`
	PendingSelectedAt time.Time `json:"pending_selected_at,omitempty"`
	LastSelectedID    string    `json:"last_selected_id,omitempty"`
	LastSelectedAt    time.Time `json:"last_selected_at,omitempty"`
}

type ProgressState struct {
	Version  int              `json:"version"`
	Key      string           `json:"key"`
	Strategy string           `json:"strategy"`
	Cursor   ProgressCursor   `json:"cursor"`
	Snapshot ProgressSnapshot `json:"snapshot"`
	History  ProgressHistory  `json:"history"`
}

type ProgressSelectionRequest struct {
	Key         string
	SourceAlias string
	Parser      string
	Items       []ProgressItem
	Now         time.Time
}

type ProgressSelection struct {
	Item  ProgressItem
	State ProgressState
}

func ParseProgressItems(parser string, text string) ([]ProgressItem, error) {
	switch parser {
	case ProgressParserNonEmptyLines:
		return parseNonEmptyLineItems(text), nil
	case ProgressParserHeadingWithBullets:
		return parseHeadingWithBulletsItems(text), nil
	default:
		return nil, fmt.Errorf("unknown progress parser %q", parser)
	}
}

func parseNonEmptyLineItems(text string) []ProgressItem {
	var items []ProgressItem
	seen := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		title := strings.TrimSpace(line)
		if title == "" {
			continue
		}
		items = append(items, ProgressItem{ID: uniqueProgressID(title, nil, seen), Title: title})
	}
	return items
}

func parseHeadingWithBulletsItems(text string) []ProgressItem {
	var items []ProgressItem
	var current *ProgressItem
	seen := map[string]int{}
	flush := func() {
		if current == nil {
			return
		}
		current.ID = uniqueProgressID(current.Title, current.Body, seen)
		items = append(items, *current)
		current = nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if bullet, ok := trimBulletPrefix(line); ok {
			if current == nil {
				current = &ProgressItem{Title: bullet}
				continue
			}
			if bullet != "" {
				current.Body = append(current.Body, bullet)
			}
			continue
		}
		flush()
		current = &ProgressItem{Title: line}
	}
	flush()
	return items
}

func trimBulletPrefix(line string) (string, bool) {
	for _, prefix := range []string{"- ", "* ", "• "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
		}
	}
	return "", false
}

func uniqueProgressID(title string, body []string, seen map[string]int) string {
	base := slugifyProgressID(title)
	if base == "" {
		base = shortProgressHash(title + "\n" + strings.Join(body, "\n"))
	}
	seen[base]++
	if seen[base] == 1 {
		return base
	}
	return base + "-" + shortProgressHash(title+"\n"+strings.Join(body, "\n"))
}

func slugifyProgressID(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortProgressHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

func SelectNextProgressItem(paths Paths, req ProgressSelectionRequest) (ProgressSelection, error) {
	req.Key = strings.TrimSpace(req.Key)
	if req.Key == "" {
		return ProgressSelection{}, fmt.Errorf("progress key is required")
	}
	if req.Parser == "" {
		return ProgressSelection{}, fmt.Errorf("progress parser is required")
	}
	if len(req.Items) == 0 {
		return ProgressSelection{}, fmt.Errorf("progress items are required")
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	state, err := ReadProgressState(paths, req.Key)
	if err != nil {
		if !os.IsNotExist(err) {
			return ProgressSelection{}, err
		}
		state = ProgressState{Version: 1, Key: req.Key, Strategy: ProgressStrategyRoundRobin}
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Key == "" {
		state.Key = req.Key
	}
	if state.Strategy == "" {
		state.Strategy = ProgressStrategyRoundRobin
	}
	if state.Strategy != ProgressStrategyRoundRobin {
		return ProgressSelection{}, fmt.Errorf("unsupported progress strategy %q", state.Strategy)
	}
	if shouldRefreshProgressSnapshot(state, req) {
		state.Snapshot = ProgressSnapshot{
			Parser:      req.Parser,
			SourceAlias: req.SourceAlias,
			ItemIDs:     progressItemIDs(req.Items),
			Cycle:       state.Cursor.Cycle,
			CreatedAt:   req.Now,
		}
		state.Cursor.NextIndex = 0
	}
	if len(state.Snapshot.ItemIDs) == 0 {
		return ProgressSelection{}, fmt.Errorf("progress snapshot has no items")
	}
	if state.Cursor.NextIndex < 0 || state.Cursor.NextIndex >= len(state.Snapshot.ItemIDs) {
		return ProgressSelection{}, fmt.Errorf("progress cursor index %d is out of range for %d items", state.Cursor.NextIndex, len(state.Snapshot.ItemIDs))
	}
	itemID := state.Snapshot.ItemIDs[state.Cursor.NextIndex]
	item, ok := progressItemByID(req.Items, itemID)
	if !ok {
		return ProgressSelection{}, fmt.Errorf("progress item %q from active snapshot is missing from current source", itemID)
	}
	state.History.PendingItemID = itemID
	state.History.PendingSelectedAt = req.Now
	if err := WriteProgressState(paths, state); err != nil {
		return ProgressSelection{}, err
	}
	return ProgressSelection{Item: item, State: state}, nil
}

func shouldRefreshProgressSnapshot(state ProgressState, req ProgressSelectionRequest) bool {
	if len(state.Snapshot.ItemIDs) == 0 {
		return true
	}
	if state.Snapshot.Cycle != state.Cursor.Cycle {
		return true
	}
	if state.Snapshot.Parser != req.Parser || state.Snapshot.SourceAlias != req.SourceAlias {
		return true
	}
	return false
}

func MarkProgressItemCompleted(paths Paths, key string, itemID string, now time.Time) error {
	state, err := ReadProgressState(paths, key)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if state.History.PendingItemID == "" {
		return fmt.Errorf("progress key %q has no pending item", key)
	}
	if state.History.PendingItemID != itemID {
		return fmt.Errorf("completed item %q does not match pending item %q", itemID, state.History.PendingItemID)
	}
	state.History.LastSelectedID = itemID
	state.History.LastSelectedAt = now
	state.History.PendingItemID = ""
	state.History.PendingSelectedAt = time.Time{}
	state.Cursor.NextIndex++
	if state.Cursor.NextIndex >= len(state.Snapshot.ItemIDs) {
		state.Cursor.NextIndex = 0
		state.Cursor.Cycle++
	}
	return WriteProgressState(paths, state)
}

func progressItemIDs(items []ProgressItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func progressItemByID(items []ProgressItem, id string) (ProgressItem, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return ProgressItem{}, false
}

func ReadProgressState(paths Paths, key string) (ProgressState, error) {
	path, err := progressStatePath(paths, key)
	if err != nil {
		return ProgressState{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return ProgressState{}, err
	}
	defer file.Close()
	var state ProgressState
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return ProgressState{}, fmt.Errorf("read progress state %q: %w", key, err)
	}
	return state, nil
}

func WriteProgressState(paths Paths, state ProgressState) error {
	path, err := progressStatePath(paths, state.Key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func progressStatePath(paths Paths, key string) (string, error) {
	key = strings.TrimSpace(key)
	if err := ValidateInstanceName(key); err != nil {
		return "", fmt.Errorf("invalid progress key: %w", err)
	}
	return filepath.Join(paths.StateDir, "progress", key+".json"), nil
}
