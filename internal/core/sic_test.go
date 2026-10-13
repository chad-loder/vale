package core

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSic(t *testing.T) {
	tests := []struct {
		value string
		want  []string
	}{
		{"YES", []string{"spelling"}},
		{"yes", []string{"spelling"}},
		{" on ", []string{"spelling"}},
		{"true", []string{"spelling"}},
		{"NO", []string{}},
		{"off", []string{}},
		{"false", []string{}},
		{"", []string{}},
		{"spelling", []string{"spelling"}},
		{"spelling, repetition", []string{"spelling", "repetition"}},
		{" spelling ,, MyStyle.Agreement , Other ", []string{"spelling", "MyStyle.Agreement", "Other"}},
	}
	for _, tt := range tests {
		got := ParseSic(tt.value)
		if got == nil {
			t.Errorf("ParseSic(%q) = nil; an explicit value must not read as unset", tt.value)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseSic(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestSicApplies(t *testing.T) {
	f := &File{Sic: []string{"spelling", "Style.Rule", "Whole"}}
	tests := []struct {
		check, kind string
		want        bool
	}{
		{"Vale.Spelling", "spelling", true},      // by kind
		{"Style.Rule", "existence", true},        // by rule
		{"Whole.Anything", "substitution", true}, // by style
		{"Style.Other", "existence", false},      // same style, rule not listed
		{"Wholesale.Rule", "existence", false},   // a style name is not a prefix
		{"Vale.Repetition", "repetition", false},
	}
	for _, tt := range tests {
		if got := f.SicApplies(tt.check, tt.kind); got != tt.want {
			t.Errorf("SicApplies(%q, %q) = %v, want %v", tt.check, tt.kind, got, tt.want)
		}
	}

	off := &File{}
	if off.SicApplies("Vale.Spelling", "spelling") {
		t.Error("a file with no Sic setting withdrew a spelling alert")
	}
}

// Each case is the text that follows a match.
func TestSicMarker(t *testing.T) {
	marked := []string{
		"[sic]",
		" [sic]",
		"(sic)",
		" (sic) and on",
		" [SIC]",
		" [Sic]",
		`" [sic]`,
		`," [sic]`,
		`." (sic)`,
		"” [sic]",
		"’ [sic]",
		"» [sic]",
		") [sic]",
		"… [sic]",
		"?! [sic]",
		"* [sic]",
		"** [sic]",
		"_ [sic]",
		" [*sic*]",
		" [_sic_]",
		" [**sic**]",
		" *[sic]*",
		" _(sic)_",
		` \[sic\]`,
		` \[*sic*\]`,
		" [<i>sic</i>]",
		" [<em>sic</em>]",
		` [\textit{sic}]`,
		` [\emph{sic}]`,
		"\t[sic]",
		"\n[sic]",
		"\r\n[sic]",
		"  \n   [sic] continues",
		"\n// [sic]",
		"\n/// [sic]",
		"\n * [sic]",
		"\n# [sic]",
		"\n## [sic]",
		"\n-- (sic)",
		"\n; [sic]",
		"\n> [sic]",
		"\n> > [sic]",
		" [sic!]",
		" (SIC!)",
		// A Markdown link written over the marker still opens with it; what
		// follows the closing bracket is not read.
		" [sic](https://example.com)",
	}
	for _, s := range marked {
		if !sicMarker.MatchString(s) {
			t.Errorf("%q: no marker found", s)
		}
	}

	unmarked := []string{
		"",
		" sic",
		" word [sic]",
		" - [sic]",
		" `[sic]`",
		" [sic",
		" sic]",
		" [sick]",
		" [basic]",
		" [ sic ]",
		" [sic.]",
		" [sic!!]",
		" [sic?]",
		" {sic}",
		"\n\n[sic]",
		"\nword [sic]",
		"\n// word [sic]",
	}
	for _, s := range unmarked {
		if sicMarker.MatchString(s) {
			t.Errorf("%q: marker found", s)
		}
	}
}

func TestSicFollows(t *testing.T) {
	lines := []string{
		"The README said \"inteference\" [sic] here.\n", // 1
		"Ünïcödé before teh [sic] it.\n",                // 2
		"A word at the end teh\n",                       // 3
		"[sic] opens this line.\n",                      // 4
		"A match that runs over a\n",                    // 5
		"break could of [sic] here.\n",                  // 6
		"No marker for teh here.\n",                     // 7
		"Marker one word late: teh word [sic].\n",       // 8
		"Last line teh",                                 // 9
	}
	f := &File{Lines: lines}

	// col returns the 1-based rune column of the first occurrence of s on
	// line n.
	col := func(n int, s string) int {
		l := lines[n-1]
		return len([]rune(l[:strings.Index(l, s)])) + 1
	}
	at := func(n int, s string) Alert {
		c := col(n, s)
		return Alert{Line: n, Span: []int{c, c + len([]rune(s)) - 1}, Match: s}
	}

	tests := []struct {
		name string
		a    Alert
		want bool
	}{
		{"quoted word", at(1, "inteference"), true},
		{"multibyte runes before the match", at(2, "teh"), true},
		{"marker on the next line", at(3, "teh"), true},
		{"no marker", at(7, "teh"), false},
		{"marker after the next word", at(8, "teh"), false},
		{"last line, no next line", at(9, "teh"), false},
		{"the marker's own line is not a match", at(4, "opens"), false},
	}

	// A match across a line break: from "a" on line 5 to "could of" on 6.
	start := col(5, "a\n")
	multi := Alert{Line: 5, Span: []int{start, start + len([]rune("a\nbreak could of")) - 1}}
	tests = append(tests, struct {
		name string
		a    Alert
		want bool
	}{"match across a line break", multi, true})

	for _, tt := range tests {
		if got := f.sicFollows(tt.a); got != tt.want {
			t.Errorf("%s: sicFollows(line %d, span %v) = %v, want %v", tt.name, tt.a.Line, tt.a.Span, got, tt.want)
		}
	}
}

func TestSicFollowsRejectsOutOfRangeAlerts(t *testing.T) {
	f := &File{Lines: []string{"teh [sic]\n"}}
	for _, a := range []Alert{
		{Line: 0, Span: []int{1, 3}},
		{Line: 2, Span: []int{1, 3}},
		{Line: 1, Span: []int{0, 3}},
		{Line: 1, Span: []int{3, 2}},
		{Line: 1, Span: []int{1}},
		{Line: 1, Span: []int{1, 500}},
	} {
		if f.sicFollows(a) {
			t.Errorf("sicFollows(line %d, span %v) = true for a position outside the text", a.Line, a.Span)
		}
	}
}
