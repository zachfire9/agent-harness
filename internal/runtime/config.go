package runtime

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultHeartbeatJobInterval = time.Minute

type RuntimeConfig struct {
	HeartbeatJobInterval time.Duration
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{HeartbeatJobInterval: defaultHeartbeatJobInterval}
}

func ReadRuntimeConfig(paths Paths) (RuntimeConfig, error) {
	cfg := DefaultRuntimeConfig()
	configPath := filepath.Join(paths.ConfigDir, "config.yaml")
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return RuntimeConfig{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) != "heartbeat_job_interval_seconds" {
			continue
		}
		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || seconds <= 0 {
			return RuntimeConfig{}, fmt.Errorf("invalid heartbeat_job_interval_seconds %q", strings.TrimSpace(value))
		}
		cfg.HeartbeatJobInterval = time.Duration(seconds) * time.Second
	}
	if err := scanner.Err(); err != nil {
		return RuntimeConfig{}, err
	}
	return cfg, nil
}
