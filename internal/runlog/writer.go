package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
)

// Event is one durable machine-readable run/session log record.
type Event struct {
	RunID          string                `json:"run_id,omitempty"`
	Time           string                `json:"time,omitempty"`
	Type           string                `json:"type"`
	Prompt         string                `json:"prompt,omitempty"`
	Error          string                `json:"error,omitempty"`
	Messages       []llm.Message         `json:"messages,omitempty"`
	TraceEvents    []agent.TraceEvent    `json:"trace_events,omitempty"`
	ContextReports []agent.ContextReport `json:"context_reports,omitempty"`
	Truncation     *agent.Truncation     `json:"truncation,omitempty"`
	Step           int                   `json:"step,omitempty"`
}

// Writer appends JSONL events for one agent run. A disabled Writer is a no-op.
type Writer struct {
	enabled bool
	runID   string
	path    string
	file    *os.File
	secrets []string
}

// NewWriter creates a run log writer under dir when enabled. Disabled writers do not create files.
func NewWriter(dir string, enabled bool, secrets []string) (*Writer, error) {
	if !enabled {
		return &Writer{enabled: false}, nil
	}
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(".agent-harness", "runs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create run log dir: %w", err)
	}

	now := time.Now().UTC()
	runID := now.Format("20060102T150405.000000000Z")
	path := filepath.Join(dir, runID+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create run log: %w", err)
	}

	return &Writer{enabled: true, runID: runID, path: path, file: file, secrets: compactSecrets(secrets)}, nil
}

// Path returns the created JSONL path, or empty for a disabled writer.
func (w *Writer) Path() string {
	if w == nil {
		return ""
	}
	return w.path
}

// Write appends one event to the run log as redacted JSONL.
func (w *Writer) Write(event Event) error {
	if w == nil || !w.enabled {
		return nil
	}
	event.RunID = w.runID
	event.Time = time.Now().UTC().Format(time.RFC3339Nano)

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal run log event: %w", err)
	}
	redacted := redactString(string(payload), w.secrets)
	if _, err := w.file.WriteString(redacted + "\n"); err != nil {
		return fmt.Errorf("write run log event: %w", err)
	}
	return nil
}

// Close closes the underlying file for enabled writers.
func (w *Writer) Close() error {
	if w == nil || !w.enabled || w.file == nil {
		return nil
	}
	return w.file.Close()
}

func compactSecrets(secrets []string) []string {
	compacted := make([]string, 0, len(secrets))
	seen := map[string]bool{}
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" || seen[secret] {
			continue
		}
		seen[secret] = true
		compacted = append(compacted, secret)
	}
	return compacted
}

func redactString(value string, secrets []string) string {
	for _, secret := range secrets {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}
