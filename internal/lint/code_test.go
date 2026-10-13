package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vale-cli/vale/v3/internal/core"
)

// Both readers of a source file consult skipsComment, so a scope excluded from
// one is excluded from the other. Mapping a markup format onto a file switches
// which reader runs, and used to switch this off with it. See #858.
func TestSkipsComment(t *testing.T) {
	tests := []struct {
		name    string
		ignored []string
		scope   string
		want    bool
	}{
		{"nothing ignored", nil, "text.comment.block", false},
		{"the scope itself", []string{"text.comment.block"}, "text.comment.block", true},
		{"every comment", []string{"comment"}, "text.comment.block", true},
		{"a different scope", []string{"text.comment.line"}, "text.comment.block", false},
		{"alongside the defaults", []string{"code", "tt", "text.comment.block"},
			"text.comment.block", true},
	}

	cfg, err := core.NewConfig(&core.CLIFlags{})
	if err != nil {
		t.Fatal(err)
	}
	linter, err := NewLinter(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			linter.Manager.Config.IgnoredScopes = tt.ignored
			if got := linter.skipsComment(tt.scope); got != tt.want {
				t.Errorf("skipsComment(%q) = %v with %v, want %v",
					tt.scope, got, tt.ignored, tt.want)
			}
		})
	}
}

// Every comment form reports the column its text has in the source: single-line
// and multi-line JSDoc blocks, indented or not, their decorated inner lines,
// `/* */`, and runs of `//` comments that are merged into one. TypeScript's
// delimiter pattern strips `/*` from `/**` and JavaScript's strips all of it,
// so the same source is checked in both.
func TestCommentColumns(t *testing.T) {
	src := strings.Join([]string{
		"/** one ZEBRA here */",
		"    /** two ZEBRA here */",
		"/** three ZEBRA",
		" * four ZEBRA",
		" */",
		"    /** five ZEBRA",
		"     * six ZEBRA",
		"     * @param _x seven ZEBRA",
		"     */",
		"// eight ZEBRA",
		"    // nine ZEBRA",
		"/* ten ZEBRA */",
		"    /*",
		"     * eleven ZEBRA",
		"     */",
		"const a = 1; // twelve ZEBRA",
		"function f() {",
		"        // first ZEBRA line",
		"        // second ZEBRA line",
		"        // third ZEBRA",
		"}",
		"",
	}, "\n")

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "styles", "T"), 0o755); err != nil {
		t.Fatal(err)
	}
	rule := "extends: existence\nmessage: hit\nlevel: error\ntokens:\n  - ZEBRA\n"
	if err := os.WriteFile(filepath.Join(dir, "styles", "T", "Z.yml"), []byte(rule), 0o600); err != nil {
		t.Fatal(err)
	}
	ini := filepath.Join(dir, ".vale.ini")
	if err := os.WriteFile(ini, []byte("StylesPath = styles\n[*]\nBasedOnStyles = T\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(src, "\n")
	for _, ext := range []string{".ts", ".js", ".tsx"} {
		t.Run(ext, func(t *testing.T) {
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

			found := map[int]bool{}
			for _, f := range files {
				for _, a := range f.Alerts {
					want := strings.Index(lines[a.Line-1], "ZEBRA") + 1
					if a.Span[0] != want {
						t.Errorf("line %d %q: column %d, want %d", a.Line, lines[a.Line-1], a.Span[0], want)
					}
					found[a.Line] = true
				}
			}
			for i, l := range lines {
				if strings.Contains(l, "ZEBRA") && !found[i+1] {
					t.Errorf("line %d %q: no alert", i+1, l)
				}
			}
		})
	}
}
