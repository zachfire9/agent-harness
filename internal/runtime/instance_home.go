package runtime

import (
	"os"
	"path/filepath"
)

const starterConfig = `# agent-harness instance configuration
# Jobs are declared here and recorded under state/jobs.json.
jobs:
  - name: heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 60
`

// InitInstance creates the self-contained directory tree for an instance.
func InitInstance(paths Paths) error {
	for _, dir := range []string{paths.Home, paths.ConfigDir, paths.StateDir, paths.CacheDir, paths.WorkDir, paths.LogDir} {
		if err := ensurePrivateDir(dir); err != nil {
			return err
		}
	}
	if err := ensurePrivateDir(filepath.Join(paths.ConfigDir, "secrets")); err != nil {
		return err
	}
	configPath := filepath.Join(paths.ConfigDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		// Never rewrite an existing config; only tighten its mode.
		return os.Chmod(configPath, privateFileMode)
	} else if !os.IsNotExist(err) {
		return err
	}
	return writePrivateFile(configPath, []byte(starterConfig))
}
