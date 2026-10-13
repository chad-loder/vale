package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vale-cli/vale/v3/internal/core"
)

// sicStyles writes a style `T` with a rule of each kind a `[sic]` is scoped
// by: spelling, a one-word existence rule, a phrase existence rule, and a
// repetition rule.
func sicStyles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rules := map[string]string{
		"Spell.yml":  "extends: spelling\nmessage: \"'%s'\"\nlevel: error\n",
		"Zebra.yml":  "extends: existence\nmessage: \"'%s'\"\nlevel: error\ntokens:\n  - ZEBRA\n",
		"Phrase.yml": "extends: existence\nmessage: \"'%s'\"\nlevel: error\ntokens:\n  - could of\n",
		"Repeat.yml": "extends: repetition\nmessage: \"'%s'\"\nlevel: error\nalpha: true\ntokens:\n  - '[^\\s]+'\n",
		"Long.yml":   "extends: occurrence\nmessage: \"long\"\nlevel: error\nscope: sentence\nmax: 12\ntoken: '\\b(\\w+)\\b'\n",
	}
	if err := os.MkdirAll(filepath.Join(dir, "styles", "T"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range rules {
		if err := os.WriteFile(filepath.Join(dir, "styles", "T", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// lintSic lints src as a file with extension ext under the configuration
// body, and returns each alert as `line:col:Check`, sorted.
func lintSic(t *testing.T, dir, body, ext, src string) []string {
	t.Helper()
	ini := filepath.Join(dir, ".vale.ini")
	if err := os.WriteFile(ini, []byte("StylesPath = styles\n"+body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.ReadPipeline(&core.CLIFlags{Path: ini, IgnoreGlobal: true, InExt: ext}, false)
	if err != nil {
		t.Fatal(err)
	}
	linter, err := NewLinter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	files, err := linter.LintString(src)
	if err != nil {
		t.Fatal(err)
	}

	var out []string
	for _, f := range files {
		for _, a := range f.Alerts {
			out = append(out, fmt.Sprintf("%d:%d:%s", a.Line, a.Span[0], a.Check))
		}
	}
	sort.Strings(out)
	return out
}

func checkSic(t *testing.T, got []string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("alerts\n got %q\nwant %q", got, want)
	}
}

const sicProse = "The note said inteference [sic] twice.\n" +
	"\n" +
	"It said ZEBRA [sic] once.\n" +
	"\n" +
	"It said you could of [sic] known.\n"

// Without the key, a marker changes nothing.
func TestSicIsOffByDefault(t *testing.T) {
	dir := sicStyles(t)
	got := lintSic(t, dir, "[*]\nBasedOnStyles = T\nT.Long = NO\n", ".md", sicProse)
	checkSic(t, got, "1:15:T.Spell", "3:9:T.Zebra", "5:13:T.Phrase")
}

func TestSicYesCoversSpellingOnly(t *testing.T) {
	dir := sicStyles(t)
	for _, body := range []string{
		"[*]\nBasedOnStyles = T\nT.Long = NO\nSic = YES\n",
		"[*]\nBasedOnStyles = T\nT.Long = NO\n[*.md]\nSic = YES\n",
	} {
		got := lintSic(t, dir, body, ".md", sicProse)
		checkSic(t, got, "3:9:T.Zebra", "5:13:T.Phrase")
	}
}

func TestSicNoIsOff(t *testing.T) {
	dir := sicStyles(t)
	got := lintSic(t, dir, "[*]\nBasedOnStyles = T\nT.Long = NO\nSic = NO\n", ".md", sicProse)
	checkSic(t, got, "1:15:T.Spell", "3:9:T.Zebra", "5:13:T.Phrase")
}

// A list scopes the marker by rule kind, by style, or by single rule.
func TestSicScopes(t *testing.T) {
	dir := sicStyles(t)
	tests := []struct {
		sic  string
		want []string
	}{
		// By kind: both existence rules, the phrase included, and not spelling.
		{"existence", []string{"1:15:T.Spell"}},
		{"spelling, existence", nil},
		// By rule: that rule alone.
		{"T.Zebra", []string{"1:15:T.Spell", "5:13:T.Phrase"}},
		{"T.Phrase", []string{"1:15:T.Spell", "3:9:T.Zebra"}},
		// By style: every rule in it.
		{"T", nil},
		// A name that matches nothing covers nothing.
		{"Other, T.Spel", []string{"1:15:T.Spell", "3:9:T.Zebra", "5:13:T.Phrase"}},
	}
	for _, tt := range tests {
		t.Run(tt.sic, func(t *testing.T) {
			got := lintSic(t, dir, "[*]\nBasedOnStyles = T\nT.Long = NO\nSic = "+tt.sic+"\n", ".md", sicProse)
			checkSic(t, got, tt.want...)
		})
	}
}

// The marker vouches for the words it follows, not for the sentence: a rule
// of a kind that is not listed still reports, on the same sentence.
func TestSicLeavesUnlistedKindsAlone(t *testing.T) {
	dir := sicStyles(t)
	src := "One two three four five six seven eight nine ten eleven twelve thirteen inteference [sic].\n"
	got := lintSic(t, dir, "[*]\nBasedOnStyles = T\nSic = YES\n", ".md", src)
	checkSic(t, got, "1:1:T.Long")
}

func TestSicRepetition(t *testing.T) {
	dir := sicStyles(t)
	src := "It said the the [sic] thing.\n\nIt said the the thing.\n"
	body := "[*]\nBasedOnStyles = T\nT.Long = NO\nT.Spell = NO\nSic = repetition\n"
	checkSic(t, lintSic(t, dir, body, ".md", src), "3:9:T.Repeat")
}

// A later section wins for the files it matches, and only for those.
func TestSicPerSection(t *testing.T) {
	dir := sicStyles(t)
	body := "[*]\nBasedOnStyles = T\nT.Long = NO\nSic = YES\n[*.txt]\nSic = NO\n[*.html]\nSic = existence\n"
	checkSic(t, lintSic(t, dir, body, ".md", sicProse), "3:9:T.Zebra", "5:13:T.Phrase")
	checkSic(t, lintSic(t, dir, body, ".txt", sicProse), "1:15:T.Spell", "3:9:T.Zebra", "5:13:T.Phrase")
	html := "<p>The note said inteference [sic] twice.</p>\n<p>It said ZEBRA [sic] once.</p>\n"
	checkSic(t, lintSic(t, dir, body, ".html", html), "1:18:T.Spell")
}

// Like every per-format key, `Sic` is not a core option.
func TestSicIsNotACoreOption(t *testing.T) {
	dir := t.TempDir()
	ini := filepath.Join(dir, ".vale.ini")
	if err := os.WriteFile(ini, []byte("Sic = YES\n[*]\nBasedOnStyles = Vale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := core.ReadPipeline(&core.CLIFlags{Path: ini, IgnoreGlobal: true}, false)
	if err == nil || !strings.Contains(err.Error(), "'Sic' is a syntax-specific option") {
		t.Errorf("err = %v, want the syntax-specific option error", err)
	}
}

// A withdrawn alert still consumes its occurrence: the same word unmarked,
// later in the paragraph, is reported where it is and not where the first
// one was.
func TestSicKeepsLaterOccurrencesInPlace(t *testing.T) {
	dir := sicStyles(t)
	src := "It said inteference [sic] and then inteference again, and ZEBRA [sic] then ZEBRA.\n"
	body := "[*]\nBasedOnStyles = T\nT.Long = NO\nSic = spelling, T.Zebra\n"
	checkSic(t, lintSic(t, dir, body, ".md", src), "1:36:T.Spell", "1:76:T.Zebra")
}

// A marker in a code span is code: it withdraws nothing.
func TestSicInCodeIsNotAMarker(t *testing.T) {
	dir := sicStyles(t)
	src := "Code `inteference [sic]` is not prose, and inteferencez `[sic]` is code.\n"
	body := "[*]\nBasedOnStyles = T\nT.Long = NO\nSic = YES\n"
	checkSic(t, lintSic(t, dir, body, ".md", src), "1:44:T.Spell")
}

// Only the match the marker follows is withdrawn.
func TestSicMarksOnlyTheMatchItFollows(t *testing.T) {
	dir := sicStyles(t)
	src := "It said inteference word [sic], and inteferencez [sic].\n"
	body := "[*]\nBasedOnStyles = T\nT.Long = NO\nSic = YES\n"
	checkSic(t, lintSic(t, dir, body, ".md", src), "1:9:T.Spell")
}

// The marker in each format's own way of writing it, and in each comment
// syntax, wrapped onto the next line where a comment can wrap.
func TestSicFormats(t *testing.T) {
	dir := sicStyles(t)
	body := "[*]\nBasedOnStyles = T\nT.Long = NO\nT.Repeat = NO\nSic = YES\n"
	tests := []struct {
		ext, src string
		want     []string
	}{
		{".md", "Quoted \"inteference,\" [*sic*] and inteferencez *[sic]*.\n", nil},
		{".md", "Escaped inteference \\[sic\\] here.\n", nil},
		{".md", "Wrapped inteference\n[sic] here.\n", nil},
		{".md", "> Quoted inteference\n> [sic] here.\n", nil},
		{".md", "A list item:\n\n- with inteference (sic) here.\n", nil},
		{".txt", "Plain inteference [sic] here and inteferencez there.\n", []string{"1:34:T.Spell"}},
		{".html", "<p>Quoted inteference [<i>sic</i>] and inteferencez there.</p>\n", []string{"1:40:T.Spell"}},
		{".ts", "// Quoted inteference [sic] and inteferencez there.\nx();\n", []string{"1:33:T.Spell"}},
		{".ts", "// Wrapped inteference\n// [sic] here.\nx();\n", nil},
		{".ts", "/**\n * Doc inteference\n * [sic] here, inteferencez there.\n */\nx();\n", []string{"3:16:T.Spell"}},
		{".py", "# Quoted inteference (sic) and inteferencez there.\nx = 1\n", []string{"1:32:T.Spell"}},
		{".go", "package p\n\n// F quotes inteference (sic) here.\nfunc F() {}\n", nil},
		{".rs", "/// Quotes inteference \\[sic\\] here.\nfn f() {}\n", nil},
	}
	off := strings.Replace(body, "Sic = YES", "Sic = NO", 1)
	for _, tt := range tests {
		t.Run(tt.ext+" "+strings.SplitN(tt.src, "\n", 2)[0], func(t *testing.T) {
			got := lintSic(t, dir, body, tt.ext, tt.src)
			checkSic(t, got, tt.want...)

			// The marker is what withdrew the rest: without it they report.
			if all := lintSic(t, dir, off, tt.ext, tt.src); len(all) <= len(got) {
				t.Errorf("with Sic = NO: %q; the marked words were never flagged", all)
			}
		})
	}
}
