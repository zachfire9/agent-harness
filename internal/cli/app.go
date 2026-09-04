package cli

import (
	"fmt"
	"io"
	"strings"
)

const defaultMessage = "agent-harness: staged learning CLI ready"

// Run executes the agent-harness command and returns a process-style exit code.
func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) <= 1 {
		fmt.Fprintln(stdout, defaultMessage)
		return 0
	}

	switch args[1] {
	case "ask":
		return runAsk(args[2:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[1])
		return 1
	}
}

func runAsk(promptArgs []string, stdout io.Writer, stderr io.Writer) int {
	prompt := strings.TrimSpace(strings.Join(promptArgs, " "))
	if prompt == "" {
		fmt.Fprintln(stderr, "ask requires a prompt")
		return 1
	}

	fmt.Fprintf(stdout, "Prompt: %s\n", prompt)
	return 0
}
