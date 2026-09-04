package cli

import (
	"fmt"
	"io"
)

const defaultMessage = "agent-harness: staged learning CLI ready"

// Run executes the agent-harness command and returns a process-style exit code.
func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stdout, defaultMessage)
	return 0
}
