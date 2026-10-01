package runtime

import (
	"fmt"
	"path/filepath"
	"regexp"
)

var safeInstanceName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

// Paths contains all writable directories/files for a single agent-harness instance.
type Paths struct {
	Home       string
	ConfigDir  string
	StateDir   string
	CacheDir   string
	WorkDir    string
	LogDir     string
	StatusPath string
}

// ValidateInstanceName rejects names that could escape an instance directory,
// conflict with systemd unit syntax, or require shell quoting.
func ValidateInstanceName(name string) error {
	if !safeInstanceName.MatchString(name) {
		return fmt.Errorf("invalid instance name %q", name)
	}
	return nil
}

// LinuxPaths resolves the documented Linux per-user instance layout under dataRoot.
func LinuxPaths(dataRoot string, instance string) (Paths, error) {
	if err := ValidateInstanceName(instance); err != nil {
		return Paths{}, err
	}
	return PathsForHome(filepath.Join(dataRoot, "agent-harness", "instances", instance)), nil
}

// PathsForHome resolves the standard directories under an explicit instance home.
func PathsForHome(home string) Paths {
	state := filepath.Join(home, "state")
	return Paths{
		Home:       home,
		ConfigDir:  filepath.Join(home, "config"),
		StateDir:   state,
		CacheDir:   filepath.Join(home, "cache"),
		WorkDir:    filepath.Join(home, "work"),
		LogDir:     filepath.Join(home, "logs"),
		StatusPath: filepath.Join(state, "status.json"),
	}
}
