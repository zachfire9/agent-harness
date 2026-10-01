package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State string

const (
	StateUnknown  State = "unknown"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopped  State = "stopped"
	StateStale    State = "stale"
	StateError    State = "error"
)

// Status is the app-owned health/status document stored in an instance home.
type Status struct {
	Instance        string    `json:"instance"`
	Status          State     `json:"status"`
	StartedAt       time.Time `json:"started_at"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	PID             int       `json:"pid"`
	Version         string    `json:"version"`
	Error           string    `json:"error,omitempty"`
}

func WriteStatus(path string, status Status) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func ReadStatus(path string) (Status, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func ClassifyStatus(status Status, now time.Time, heartbeatTTL time.Duration) State {
	if status.Status == "" {
		return StateUnknown
	}
	if status.Status != StateRunning && status.Status != StateStarting {
		return status.Status
	}
	if status.LastHeartbeatAt.IsZero() {
		return StateUnknown
	}
	if now.Sub(status.LastHeartbeatAt) > heartbeatTTL {
		return StateStale
	}
	return status.Status
}
