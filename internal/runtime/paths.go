package runtime

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
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
	JobsPath   string
}

// ValidateInstanceName rejects names that could escape an instance directory,
// conflict with systemd unit syntax, or require shell quoting.
func ValidateInstanceName(name string) error {
	if !safeInstanceName.MatchString(name) || len(name) > 64 || windowsReservedName(name) {
		return fmt.Errorf("invalid instance name %q", name)
	}
	return nil
}

func windowsReservedName(name string) bool {
	upper := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	return upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" || (len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9')
}

// LinuxPaths resolves the documented Linux per-user instance layout under dataRoot.
func LinuxPaths(dataRoot string, instance string) (Paths, error) {
	if err := ValidateInstanceName(instance); err != nil {
		return Paths{}, err
	}
	return PathsForHome(filepath.Join(dataRoot, "agent-harness", "instances", instance)), nil
}

// WindowsPaths uses LOCALAPPDATA, never roaming AppData or Linux/XDG defaults.
func WindowsPaths(localAppData, instance string) (Paths, error) {
	if localAppData == "" || !filepath.IsAbs(localAppData) {
		return Paths{}, fmt.Errorf("LOCALAPPDATA must be an absolute path (or provide --home)")
	}
	return LinuxPaths(localAppData, instance)
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
		JobsPath:   filepath.Join(state, "jobs.json"),
	}
}
