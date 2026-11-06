package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/zachfire9/agent-harness/internal/agent"
)

const noStreamFlag = "--no-stream"

// parseNoStreamFlag removes --no-stream from args.
func parseNoStreamFlag(args []string) (bool, []string) {
	disabled := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == noStreamFlag {
			disabled = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return disabled, filtered
}

// replyPrinter shows streamed assistant text. Each model call's text goes on
// its own line, opened with prefix, so narration before a tool call ("Let me
// check.") is finished before an approval prompt or the next reply appears.
type replyPrinter struct {
	out     io.Writer
	prefix  string
	open    bool
	printed bool
}

func (p *replyPrinter) stream() agent.TextStream {
	return agent.TextStream{
		Text: func(text string) {
			if !p.open {
				fmt.Fprint(p.out, p.prefix)
				p.open = true
			}
			fmt.Fprint(p.out, text)
			p.printed = true
		},
		End: func() {
			if p.open {
				fmt.Fprintln(p.out)
				p.open = false
			}
		},
	}
}

// interruptibleTurn returns a context that Ctrl+C cancels for one chat turn.
// Once stop is called, Ctrl+C regains its default behavior (exit), so at the
// "You:" prompt it still quits chat.
func interruptibleTurn() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func (a App) turnContext() (context.Context, context.CancelFunc) {
	if a.newTurnContext != nil {
		return a.newTurnContext()
	}
	return interruptibleTurn()
}

// WithTurnContext replaces how chat creates each turn's context. Tests use it
// to simulate Ctrl+C without sending a real signal.
func (a App) WithTurnContext(newTurnContext func() (context.Context, context.CancelFunc)) App {
	a.newTurnContext = newTurnContext
	return a
}

// outputIsTerminal reports whether w is an interactive terminal; ask streams
// only then, so scripted output keeps containing just the final answer.
func (a App) outputIsTerminal(w io.Writer) bool {
	if a.streamOutput != nil {
		return *a.streamOutput
	}
	f, ok := w.(*os.File)
	return ok && isTerminal(f)
}

// WithTerminalOutput overrides output terminal detection for ask. Tests use it.
func (a App) WithTerminalOutput(terminal bool) App {
	a.streamOutput = &terminal
	return a
}

func turnWasCancelled(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() != nil && errors.Is(err, context.Canceled)
}
