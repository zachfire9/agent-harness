package cli

import (
	"fmt"
	"io"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
)

func writeContextWarnings(stderr io.Writer, reports []agent.ContextReport) {
	for _, report := range reports {
		for _, truncation := range report.Truncations {
			messageType := string(truncation.Role)
			if truncation.Index == -1 && truncation.Role == llm.RoleSystem {
				messageType = "running summary"
			}
			fmt.Fprintf(stderr, "context warning: truncated %s message index=%d kept=%d omitted=%d original=%d\n", messageType, truncation.Index, truncation.KeptChars, truncation.OmittedChars, truncation.OriginalChars)
		}
	}
}
