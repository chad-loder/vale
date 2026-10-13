package core

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vale-cli/vale/v3/internal/nlp"
)

// AddAlert must not panic on an alert with a negative Span -- spelling can
// produce one when the matched token isn't found verbatim in the block. See
// #808 (panic: slice bounds out of range [-1:]).
func TestAddAlertNegativeSpan(t *testing.T) {
	f := &File{
		ChkToCtx: map[string]string{},
		history:  map[string]int{},
		limits:   map[string]int{},
	}
	// count("word") > 1 and ctx < 1000 -> the disambiguation branch that
	// previously sliced ctx[0:Span[0]] with a negative index.
	blk := nlp.NewBlock("word and word", "word and word", "text.md")
	a := Alert{Check: "X", Match: "word", Span: []int{-1, 3}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AddAlert panicked on a negative span: %v", r)
		}
	}()
	f.AddAlert(a, blk, 1, 0, false)
}

// An anchored empty match is a location, not a measurement: it keeps its
// column rather than falling back to the start of its block.
func TestAddAlertEmptyMatchKeepsColumn(t *testing.T) {
	f := &File{
		ChkToCtx: map[string]string{},
		history:  map[string]int{},
		limits:   map[string]int{},
	}
	f.SetText("# Title\n\nAdd a thing\n")

	blk := nlp.NewLinedBlock(f.Content, "Add a thing", "text.md", 2)
	a := Alert{Check: "X", Match: "", Span: []int{14, 14}, HasByteOffsets: true}

	f.AddAlert(a, blk, 3, 0, false)

	if len(f.Alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(f.Alerts))
	}
	got := f.Alerts[0]
	if got.Line != 3 || got.Span[0] != 6 || got.Span[1] != 6 {
		t.Errorf("placed at %d:%v, want 3:[6 6]", got.Line, got.Span)
	}
}

// A measurement still lands at the start of its block.
func TestAddAlertMeasurementStartsBlock(t *testing.T) {
	f := &File{
		ChkToCtx: map[string]string{},
		history:  map[string]int{},
		limits:   map[string]int{},
	}
	f.SetText("# Title\n\nAdd a thing\n")

	blk := nlp.NewLinedBlock(f.Content, "Add a thing", "text.md", 2)
	a := Alert{Check: "X", Match: "", Span: []int{1, 1}}

	f.AddAlert(a, blk, 3, 0, false)

	if len(f.Alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(f.Alerts))
	}
	got := f.Alerts[0]
	if got.Line != 3 || got.Span[0] != 1 || got.Span[1] != 1 {
		t.Errorf("placed at %d:%v, want 3:[1 1]", got.Line, got.Span)
	}
}

// A match that inline markup splits in the source -- `§3.` read from `§3**.` --
// cannot be found by searching the document, so it is placed through the
// block's map from its text back to the source, in whichever unit the check
// reported: runes for a regex rule, bytes for spelling.
func TestAddAlertMapsMatchAcrossInlineMarkup(t *testing.T) {
	doc := "# Title\n\nchosen so they **do not break §3**.\n"
	text := "chosen so they do not break §3."

	src := strings.Index(doc, "chosen")
	bold := strings.Index(doc, "do not")
	dot := strings.Index(doc, "**.") + 2
	runs := []nlp.Run{
		{At: 0, Src: src, N: len("chosen so they ")},
		{At: len("chosen so they "), Src: bold, N: len("do not break §3")},
		{At: len("chosen so they do not break §3"), Src: dot, N: 1},
	}

	at := strings.Index(text, "§3.")
	runeAt := utf8.RuneCountInString(text[:at])
	wantCol := utf8.RuneCountInString(doc[strings.LastIndex(doc[:bold], "\n")+1:strings.Index(doc, "§3")]) + 1

	tests := []struct {
		name string
		span []int
	}{
		{"rune span", []int{runeAt, runeAt + 3}},
		{"byte span", []int{at, at + len("§3.")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{ChkToCtx: map[string]string{}, history: map[string]int{}, limits: map[string]int{}}
			f.SetText(doc)

			blk := nlp.NewLinedBlock(doc, text, "text.md", 2)
			blk.Runs = runs
			f.AddAlert(Alert{Check: "X", Match: "§3.", Span: tt.span}, blk, 3, 0, false)

			if len(f.Alerts) != 1 {
				t.Fatalf("got %d alerts, want 1", len(f.Alerts))
			}
			if got := f.Alerts[0]; got.Line != 3 || got.Span[0] != wantCol {
				t.Errorf("placed at %d:%d, want 3:%d", got.Line, got.Span[0], wantCol)
			}
		})
	}
}

// When extraction drops something other than emphasis or code markers -- a tag,
// say -- the words on either side can run together into a "word" the source
// never held. Such a match is not placed through the block's map.
func TestSourceSpanOfRejectsJoinedWords(t *testing.T) {
	doc := "x\nThe <source> is here\n"
	text := "Theis here"
	src := strings.Index(doc, "The")
	is := strings.Index(doc, "is here")
	blk := nlp.NewLinedBlock(doc, text, "text.md", 1)
	blk.Runs = []nlp.Run{
		{At: 0, Src: src, N: 3},
		{At: 3, Src: is, N: len("is here")},
	}

	if _, _, ok := sourceSpanOf(blk, Alert{Match: "Theis", Span: []int{0, 5}}); ok {
		t.Error("placed a match that the source does not hold")
	}
}
