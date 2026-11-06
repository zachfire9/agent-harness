//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd

package cli

import "os"

// isTerminal fails closed on platforms without a terminal check: commands
// then need --approve-commands instead of an interactive prompt.
func isTerminal(*os.File) bool { return false }
