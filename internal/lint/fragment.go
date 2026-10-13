package lint

import (
	"strings"
	"unicode/utf8"

	"github.com/vale-cli/vale/v3/internal/core"
	"github.com/vale-cli/vale/v3/internal/lint/code"
)

func findLine(s string, line int) string {
	lines := strings.Split(s, "\n")
	if line > len(lines) {
		return ""
	}
	return lines[line-1]
}

// delimiterWidth is the width, in runes, of the comment opener at the start of
// line as the language's delimiter pattern matches it. Without a pattern, or
// when it does not match at the start, it falls back to the padding with its
// trailing spaces removed.
func delimiterWidth(lang *code.Language, line string, padding int) int {
	if lang.Delims != nil {
		if loc := lang.Delims.FindStringIndex(line); loc != nil && loc[0] == 0 {
			return len([]rune(line[:loc[1]]))
		}
	}
	runes := []rune(line)
	n := min(padding, len(runes))
	for n > 0 && runes[n-1] == ' ' {
		n--
	}
	return n
}

func leadingSpaces(line string, offset int) int {
	spaces := 0
	for _, r := range line {
		if r == ' ' {
			spaces++
		} else {
			break
		}
	}
	return spaces - offset
}

func adjustAlerts(alerts []core.Alert, last int, comment code.Comment, lang *code.Language) []core.Alert {
	for i := range alerts {
		if i >= last {
			line := findLine(comment.Source, alerts[i].Line)

			padding, ok := anchoredPadding(comment, alerts[i].Line, line, lang)
			if !ok {
				padding = commentPadding(comment, alerts[i].Line, line, lang)
			}

			alerts[i].Line += comment.Line - 1
			alerts[i].Span = []int{
				alerts[i].Span[0] + comment.Offset + padding,
				alerts[i].Span[1] + comment.Offset + padding,
			}
		}
	}
	return alerts
}

// anchoredPadding places a line of the extracted comment by finding it in its
// source line. Extraction only removes a prefix -- the opener, the indentation,
// a JSDoc ` *` -- and the closer, so the extracted text, less trailing space,
// is a substring of the source line, and where it starts is the exact padding.
// Searching starts after the opener so the text cannot be found inside it.
//
// How a source line relates to the file depends on the comment. Every line of
// a merged run of line comments is that comment's own text: it starts with its
// delimiter, at the column comment.Offset places. Any other comment's first
// line is the same, but each later line is the whole file line, so the offset
// the caller adds comes back off.
func anchoredPadding(comment code.Comment, line int, source string, lang *code.Language) (int, bool) {
	text := strings.TrimRight(findLine(comment.Text, line), " \t")
	if text == "" {
		return 0, false
	}

	opener := line == 1 || comment.Merged
	from := 0
	if opener && lang.Delims != nil {
		if loc := lang.Delims.FindStringIndex(source); loc != nil && loc[0] == 0 {
			from = loc[1]
		}
	}
	i := strings.Index(source[from:], text)
	if i < 0 {
		return 0, false
	}

	padding := utf8.RuneCountInString(source[:from+i])
	if !opener {
		padding -= comment.Offset
	}
	return padding, true
}

// commentPadding returns how far to move an alert to get from a column in the
// extracted comment back to a column in the source.
//
// A dedented comment recorded exactly what it took off each line, so it is
// asked rather than measured. Measuring the source line only agrees with the
// dedent when everything removed was whitespace, which is why a language whose
// decoration is not whitespace -- JSDoc's ` *` -- reported columns that were
// short by the width of the decoration.
//
// The caller adds comment.Offset separately, and a comment that starts in from
// the margin has that same indentation counted in what the dedent removed, so
// it comes back off here. This is what leadingSpaces does for the measured
// path.
func commentPadding(comment code.Comment, line int, source string, lang *code.Language) int {
	if n, ok := comment.StripAt(line); ok {
		// Strip describes the dedent only. The opening delimiter -- `"""`,
		// `/**` -- was removed before that by Delims, and it is still on the
		// line the alert is measured against, so it is added here. On any line
		// but the first there is no delimiter and this contributes nothing.
		if p := lang.Padding(source); line == 1 && p > 0 {
			// The delimiter line: the marker the language's own delimiter
			// pattern removed, plus everything the strip took after it.
			// Measuring the marker with that pattern keeps `/**` exact in
			// a language that strips all of it (JavaScript) and in one that
			// strips `/*` and leaves the `* ` to the strip (TypeScript).
			return delimiterWidth(lang, source, p) + n
		}
		return lang.Padding(source) + max(n-comment.Offset, 0)
	}

	padding := lang.Padding(source)
	if strings.HasPrefix(source, " ") {
		padding += leadingSpaces(source, comment.Offset)
	}

	return padding
}

func (l *Linter) lintFragments(f *core.File) error {
	lang, err := code.GetLanguageFromExt(f.RealExt)
	if err != nil {
		return err
	}

	if found := updateQueries(f, l.Manager.Config); len(found) > 0 {
		lang.Queries = found
	}

	comments, err := code.GetComments([]byte(f.Content), lang)
	if err != nil {
		return err
	}

	wholeFile := f.Content
	converted := l.rstBatch(f, comments, strings.TrimPrefix(f.NormedExt, "."))

	last := 0
	for i, comment := range comments {
		// QDoc reads `/*! ... */` only; a `//` line comment or a plain
		// `/* ... */` block is code, not documentation.
		if f.NormedExt == ".qdoc" && !strings.HasPrefix(comment.Source, "/*!") {
			continue
		}

		f.SetMetaScope(comment.Scope)
		if l.skipsComment(comment.Scope) {
			continue
		}

		// The file's own mapping, unless the query named a markup of its own.
		format := comment.Format
		if format == "" {
			format = strings.TrimPrefix(f.NormedExt, ".")
		}
		err = l.lintComment(f, comment, format, converted[i])
		if err != nil {
			return err
		}
		f.Alerts = dropMasked(f.Alerts, last, comment)

		size := len(f.Alerts)
		if size != last {
			f.Alerts = adjustAlerts(f.Alerts, last, comment, lang)
		}
		last = size
	}

	// Each comment was linted in the file's place; put the file back, so the
	// `raw` scope that runs next reads the source and not the last comment.
	f.RestoreText(wholeFile)
	if err != nil {
		return err
	}
	return l.lintSummary(f)
}
