package core

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// A `[sic]` marks a mistake as the source's, not the writer's: a quotation
// keeps its original spelling, and the marker says that was deliberate. With
// `Sic` set, Vale takes the marker at its word and withdraws the alert it
// follows.
//
//	[*]
//	Sic = YES                          # spelling alerts only
//	Sic = spelling, repetition         # alerts from rules of these kinds
//	Sic = spelling, MyStyle.Agreement  # ...or from these styles and rules
//	Sic = NO                           # off (the default)
//
// `YES` covers spelling alone, because a misspelled word is what a `[sic]`
// usually marks. A list names what else it may cover, each entry a rule kind
// (the rule's `extends`), a style, or a single rule. The marker never
// withdraws an alert that is not on the list: it vouches for how the quoted
// words are written, not for the prose around them.
//
// The key belongs in the `[*]` section or a format section, like every
// per-format key, and a later matching section wins.
//
// An alert is withdrawn when its match is followed directly by the marker:
// `[sic]` or `(sic)`, in any case, with or without the emphatic `!` some
// traditions add (`[sic!]`), italic or not (`[*sic*]`, `*[sic]*`,
// `[<i>sic</i>]`, `[\textit{sic}]`), with escaped brackets (`\[sic\]`) as a
// Markdown or rustdoc source writes them. Between the match and the marker
// there may be closing quotes and punctuation, emphasis markers, and
// whitespace including one line break, after which the next line may open
// with a comment's or a blockquote's decoration (`//`, `#`, `*`, `--`, `;`,
// `>`). Nothing else may intervene: a marker after the next word marks that
// word.
//
// The scope of a `[sic]` is the match it follows, so it falls out of the rule:
// a spelling alert covers one word, and a rule that matches a phrase covers
// the phrase. Two mistakes in one quotation take two markers, as they do in
// print.
//
// Where a doc-comment convention gives brackets a meaning -- rustdoc and KDoc
// read `[sic]` as a link to a symbol named `sic` -- write `(sic)` or `\[sic\]`.

// sicMarker matches a `[sic]` marker at the start of the text that follows a
// match. See the comment above for what may come between the two.
var sicMarker = regexp.MustCompile(
	// Closing quotes and brackets, punctuation, and emphasis markers.
	`^[\p{Pe}\p{Pf}"'*_~.,;:!?…]*` +
		// Whitespace, and at most one line break, after which the next line
		// may open with a comment's or blockquote's decoration.
		`[ \t]*(?:\r?\n[ \t]*(?:>[ \t>]*|\*|//+|#+|--|;+)?[ \t]*)?` +
		// The marker: brackets or parentheses, escaped or not, around `sic`
		// written plain or emphasized.
		`[*_]{0,2}(?:\\?\[|\()` +
		`(?:[*_]{1,2}|<(?:i|em)>|\\(?:textit|emph)\{)?` +
		`(?i:sic)!?` +
		`(?:[*_]{1,2}|</(?:i|em)>|\})?` +
		`(?:\\?\]|\))`)

// ParseSic reads a `Sic` value: `YES` for spelling alone, `NO` or nothing for
// none, or a comma-separated list of rule kinds, styles and rules.
func ParseSic(value string) []string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "YES", "ON", "TRUE":
		return []string{"spelling"}
	case "NO", "OFF", "FALSE", "":
		return []string{}
	}

	var out []string
	for _, entry := range strings.Split(value, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			out = append(out, entry)
		}
	}
	return out
}

// SicApplies reports whether a `[sic]` may withdraw an alert from the rule
// named check, whose kind (its `extends`) is kind.
func (f *File) SicApplies(check, kind string) bool {
	for _, entry := range f.Sic {
		if entry == kind || entry == check || entry == StyleName(check) {
			return true
		}
	}
	return false
}

// sicFollows reports whether a `[sic]` marker follows the located alert's
// match.
//
// The alert's line and span are those it will be reported at: a 1-based line
// of the text being linted -- the file, or the comment a code file is read
// in -- and the first and last columns of the match, counted in runes from
// the start of that line. A match that crosses a line break counts the break
// as one rune, so the end is found by walking that many runes forward.
func (f *File) sicFollows(a Alert) bool {
	if a.Line < 1 || a.Line > len(f.Lines) || len(a.Span) < 2 || a.Span[0] < 1 || a.Span[1] < a.Span[0] {
		return false
	}

	// Runes from the start of the alert's line to just past its match.
	skip := a.Span[1]
	i := a.Line - 1
	for ; i < len(f.Lines); i++ {
		n := utf8.RuneCountInString(f.Lines[i])
		if skip < n {
			break
		}
		skip -= n
	}
	if i >= len(f.Lines) {
		return false
	}

	at := 0
	for ; skip > 0; skip-- {
		_, size := utf8.DecodeRuneInString(f.Lines[i][at:])
		at += size
	}

	// The marker may wrap onto the next line.
	after := f.Lines[i][at:]
	if i+1 < len(f.Lines) {
		after += f.Lines[i+1]
	}
	return sicMarker.MatchString(after)
}
