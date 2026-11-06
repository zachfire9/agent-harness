package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zachfire9/agent-harness/internal/tools"
)

const approveCommandsFlag = "--approve-commands"

// parseApproveCommandsFlag removes --approve-commands from args. The flag is
// only ever read from the person's command line, never from model output.
func parseApproveCommandsFlag(args []string) (bool, []string) {
	enabled := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == approveCommandsFlag {
			enabled = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return enabled, filtered
}

// lineInput is the single reader for a session's stdin. chat reads prompts and
// approval answers through the same scanner so neither steals buffered input.
type lineInput struct {
	scanner *bufio.Scanner
}

func newLineInput(r io.Reader) *lineInput {
	return &lineInput{scanner: bufio.NewScanner(r)}
}

// ReadLine returns the next line, or ok=false at end of input or on error.
func (in *lineInput) ReadLine() (line string, ok bool, err error) {
	if !in.scanner.Scan() {
		return "", false, in.scanner.Err()
	}
	return in.scanner.Text(), true, nil
}

// commandApprover picks how run_command is gated for ask and chat:
//   - --approve-commands: allowlisted commands run without prompting;
//   - an interactive terminal: the person is asked before every run;
//   - otherwise (pipes, files, scripts, services): every run is declined.
func commandApprover(autoApprove bool, interactive bool, in *lineInput, prompts io.Writer) tools.CommandApprover {
	switch {
	case autoApprove:
		return tools.CommandApproverFunc(func(context.Context, string) (bool, string) { return true, "" })
	case interactive:
		return terminalApprover{in: in, out: prompts}
	default:
		return tools.CommandApproverFunc(func(context.Context, string) (bool, string) {
			return false, "commands need a person's approval and no interactive terminal is attached; the user can rerun with " + approveCommandsFlag + " to allow allowlisted commands without prompting"
		})
	}
}

// terminalApprover asks on the terminal. Anything but y/yes is a refusal, and
// end of input or a read error is treated as a refusal (fail closed).
type terminalApprover struct {
	in  *lineInput
	out io.Writer
}

func (p terminalApprover) ApproveCommand(_ context.Context, command string) (bool, string) {
	// command is an exact allowlist entry, so it is safe to display verbatim.
	fmt.Fprintf(p.out, "\nThe agent wants to run: %s\nAllow? [y/N]: ", command)
	answer, ok, _ := p.in.ReadLine()
	if !ok {
		fmt.Fprintln(p.out)
		return false, "the user did not answer the approval prompt; the command was not run"
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, ""
	default:
		return false, "the user declined to run this command"
	}
}

// inputIsTerminal reports whether r is an interactive terminal, unless a test
// has overridden detection with WithInteractiveInput.
func (a App) inputIsTerminal(r io.Reader) bool {
	if a.interactive != nil {
		return *a.interactive
	}
	f, ok := r.(*os.File)
	return ok && isTerminal(f)
}

// WithInteractiveInput overrides terminal detection for the app's input. Tests
// use it to exercise the approval prompt with scripted input.
func (a App) WithInteractiveInput(interactive bool) App {
	a.interactive = &interactive
	return a
}
