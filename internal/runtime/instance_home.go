package runtime

import (
	"os"
	"path/filepath"
)

const starterConfig = `# agent-harness instance configuration
# Built-in heartbeat job interval. Later steps can add more job types here.
heartbeat_job_interval_seconds: 60
`

// InitInstance creates the self-contained directory tree for an instance.
func InitInstance(paths Paths) error {
	for _, dir := range []string{paths.Home, paths.ConfigDir, paths.StateDir, paths.CacheDir, paths.WorkDir, paths.LogDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	configPath := filepath.Join(paths.ConfigDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(configPath, []byte(starterConfig), 0o644)
}
