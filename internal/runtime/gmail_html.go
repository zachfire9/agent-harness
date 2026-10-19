package runtime

import (
	"html"
	"strings"
)

// renderGmailHTML is a non-recursive, linear renderer, not a general Markdown
// parser. Only h1-h3, flat bullets and paragraphs are recognized. All source
// text is escaped; no source-provided tag, URL or attribute is ever emitted.
func renderGmailHTML(text string) string {
	var out strings.Builder
	out.WriteString("<!doctype html><html><body>")
	block := ""
	closeBlock := func() {
		if block != "" {
			out.WriteString("</" + block + ">")
			block = ""
		}
	}
	for _, line := range strings.Split(normalizeGmailNewlines(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			closeBlock()
			continue
		}
		heading := 0
		for level := 1; level <= 3; level++ {
			if strings.HasPrefix(line, strings.Repeat("#", level)+" ") {
				heading = level
				break
			}
		}
		if heading != 0 {
			closeBlock()
			tag := "h" + string(rune('0'+heading))
			out.WriteString("<" + tag + ">" + html.EscapeString(strings.TrimSpace(line[heading+1:])) + "</" + tag + ">")
		} else if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			if block != "ul" {
				closeBlock()
				out.WriteString("<ul>")
				block = "ul"
			}
			out.WriteString("<li>" + html.EscapeString(strings.TrimSpace(line[2:])) + "</li>")
		} else {
			if block != "p" {
				closeBlock()
				out.WriteString("<p>")
				block = "p"
			} else {
				out.WriteString("<br>")
			}
			out.WriteString(html.EscapeString(line))
		}
	}
	closeBlock()
	out.WriteString("</body></html>")
	return out.String()
}

func normalizeGmailNewlines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}
