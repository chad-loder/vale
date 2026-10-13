package lint

import (
	"testing"

	"github.com/vale-cli/vale/v3/internal/core"
	"github.com/vale-cli/vale/v3/internal/lint/code"
)

// A mask drops an alert whose match lies inside it, and keeps one that only
// starts there: a sentence that opens with a code span is still prose.
func TestDropMaskedKeepsMatchesThatLeaveTheMask(t *testing.T) {
	comment := code.Comment{Text: "`ab` runs on.", Masks: []code.Mask{{0, 4}}}
	alerts := []core.Alert{
		{Line: 1, Span: []int{2, 3}, Match: "ab"},
		{Line: 1, Span: []int{1, 13}, Match: "`ab` runs on."},
		{Line: 1, Span: []int{6, 9}, Match: "runs"},
	}
	got := dropMasked(alerts, 0, comment)
	if len(got) != 2 || got[0].Match != "`ab` runs on." || got[1].Match != "runs" {
		t.Errorf("dropMasked kept %v", got)
	}
}

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
