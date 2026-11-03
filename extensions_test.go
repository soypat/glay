package glay

import (
	"strings"
	"testing"
)

// Tests of glay's additions to clay's API, which must not change clay's output.

// oddMeasure measures text with a line height that is not a whole number, so that line
// positions accumulate rounding errors.
func oddMeasure(text string, config *TextElementConfig, _ any) Dimensions {
	return Dimensions{Width: float32(len(text)) * 7.3, Height: 16.3}
}

func TestAppendTextLineBoxes(t *testing.T) {
	texts := []string{
		"one line",
		"several words wrapping around a narrow container many times over",
		"newlines\n\nand an empty line\nin between",
		"trailing space before newline \nnext",
	}
	configs := []TextElementConfig{
		{TextAlignment: TextAlignLeft},
		{TextAlignment: TextAlignCenter, LineHeight: 23},
		{TextAlignment: TextAlignRight},
		{TextAlignment: TextAlignCenter, LineHeight: 11},
	}
	for _, culling := range []bool{false, true} {
		var c Context
		err := c.Initialize(Config{Layout: Dimensions{Width: 300, Height: 120}, MaxElementCount: 64, MeasureText: oddMeasure})
		if err != nil {
			t.Fatal(err)
		}
		c.SetCullingEnabled(culling)
		var ids []ElementID
		c.BeginLayout()
		c.OpenElementWithID(ID("Column"))
		c.ConfigureOpenElement(&ElementDeclaration{Layout: LayoutConfig{
			LayoutDirection: TopToBottom,
			Sizing:          Sizing{Width: SizingAxisFixed(150), Height: SizingAxisFit(0, 0)},
			Padding:         PaddingAll(3),
			ChildGap:        5,
		}})
		for i, text := range texts {
			ids = append(ids, c.OpenTextElement(text, configs[i]))
		}
		c.CloseElement()
		cmds, err := c.EndLayout()
		if err != nil {
			t.Fatal(err)
		}
		boxes := make(map[uint32]BoundingBox)
		for _, cmd := range cmds {
			if cmd.CommandType == RenderCommandTypeText {
				boxes[cmd.ID] = cmd.BoundingBox
			}
		}
		matched, culled := 0, 0
		var buf [64]BoundingBox
		for _, id := range ids {
			lines := c.GetTextLines(id)
			got := c.AppendTextLineBoxes(buf[:0], id)
			if len(got) != len(lines) || len(lines) == 0 {
				t.Fatalf("culling=%v text %v: %d boxes for %d lines", culling, id.ID, len(got), len(lines))
			}
			for i := range lines {
				want, ok := boxes[hashNumber(uint32(i), id.ID).ID]
				switch {
				case !ok && (lines[i].length == 0 || culling):
					culled++ // Empty or culled lines have no render command.
				case !ok:
					t.Errorf("culling=%v text %v line %d %q: no render command", culling, id.ID, i, lines[i].Line)
				case got[i] != want:
					t.Errorf("culling=%v text %v line %d: box %+v, render command %+v", culling, id.ID, i, got[i], want)
				default:
					matched++
				}
			}
		}
		if matched < 10 || (culling && culled == 0) {
			t.Errorf("culling=%v: %d boxes matched render commands, %d lines without one", culling, matched, culled)
		}
	}
	var c Context
	if err := c.Initialize(Config{MeasureText: oddMeasure}); err != nil {
		t.Fatal(err)
	}
	c.BeginLayout()
	c.OpenElementWithID(ID("Box"))
	c.ConfigureOpenElement(&ElementDeclaration{})
	c.CloseElement()
	c.EndLayout()
	if got := c.AppendTextLineBoxes(nil, ID("Box")); got != nil {
		t.Errorf("boxes for an element that is not text: %+v", got)
	}
	if got := c.AppendTextLineBoxes(nil, ID("Missing")); got != nil {
		t.Errorf("boxes for a missing element: %+v", got)
	}
}

func TestAppendTextLineBoxesAllocs(t *testing.T) {
	c := newTestContext(t)
	fd := newFrameData()
	fd.layout(c)
	if _, err := c.EndLayout(); err != nil {
		t.Fatal(err)
	}
	var id ElementID
	c.BeginLayout()
	c.OpenElementWithID(ID("Root"))
	c.ConfigureOpenElement(&ElementDeclaration{Layout: LayoutConfig{Sizing: Sizing{Width: SizingAxisFixed(60)}}})
	id = c.OpenTextElement("hello world this text wraps around", fd.text)
	c.CloseElement()
	if _, err := c.EndLayout(); err != nil {
		t.Fatal(err)
	}
	var buf [16]BoundingBox
	n := 0
	allocs := testing.AllocsPerRun(100, func() { n = len(c.AppendTextLineBoxes(buf[:0], id)) })
	if allocs != 0 || n < 2 {
		t.Errorf("%v allocations for %d boxes", allocs, n)
	}
}

func TestSeparateCapacities(t *testing.T) {
	text := strings.Repeat("word ", 100)
	var id ElementID
	layout := func(c *Context) ([]RenderCommand, error) {
		c.BeginLayout()
		c.OpenElementWithID(ID("Narrow"))
		c.ConfigureOpenElement(&ElementDeclaration{Layout: LayoutConfig{Sizing: Sizing{Width: SizingAxisFixed(50)}}})
		id = c.OpenTextElement(text, TextElementConfig{})
		c.CloseElement()
		return c.EndLayout()
	}
	base := Config{Layout: Dimensions{Width: 100, Height: 100}, MaxElementCount: 16, MaxMeasureTextCacheWordCount: 256, MeasureText: monoMeasure}

	// Clay's sizing: lines and commands are capped at MaxElementCount.
	var c Context
	if err := c.Initialize(base); err != nil {
		t.Fatal(err)
	}
	c.SetCullingEnabled(false)
	_, err := layout(&c)
	w := c.Warnings()
	if err != ErrElementsCapacityExceeded || !w.MaxRenderCommandsExceeded || !w.MaxWrappedTextLinesExceeded {
		t.Fatalf("default capacities: err %v, warnings %+v", err, w)
	}
	if n := len(c.GetTextLines(id)); n != 16 {
		t.Fatalf("default capacities: want 16 lines kept, got %d", n)
	}

	// Separate capacities hold every line and command.
	sep := base
	sep.MaxRenderCommandCount = 128
	sep.MaxWrappedTextLineCount = 128
	if err := c.Initialize(sep); err != nil {
		t.Fatal(err)
	}
	c.SetCullingEnabled(false)
	cmds, err := layout(&c)
	if err != nil || c.Warnings() != (BooleanWarnings{}) {
		t.Fatalf("separate capacities: err %v, warnings %+v", err, c.Warnings())
	}
	if n := len(c.GetTextLines(id)); n != 100 || len(cmds) != 100 {
		t.Fatalf("separate capacities: %d lines and %d commands, want 100", n, len(cmds))
	}
	if c.MaxRenderCommandCount() != 128 || c.MaxWrappedTextLineCount() != 128 || c.MaxElementCount() != 16 {
		t.Fatal("capacity getters")
	}

	// Explicit capacities equal to MaxElementCount cost the same memory as the defaults.
	same := base
	same.MaxRenderCommandCount = 16
	same.MaxWrappedTextLineCount = 16
	if same.MemoryRequirements() != base.MemoryRequirements() || sep.MemoryRequirements() <= base.MemoryRequirements() {
		t.Fatalf("memory requirements: default %d, explicit %d, larger %d",
			base.MemoryRequirements(), same.MemoryRequirements(), sep.MemoryRequirements())
	}
	small := base
	small.MaxWrappedTextLineCount = 2
	if small.MemoryRequirements() >= base.MemoryRequirements() {
		t.Fatal("fewer lines should take less memory")
	}
}

func TestResetScrollContainers(t *testing.T) {
	var c Context
	if err := c.Initialize(Config{Layout: Dimensions{Width: 100, Height: 100}, MaxElementCount: 16}); err != nil {
		t.Fatal(err)
	}
	layout := func(ids ...string) {
		c.BeginLayout()
		for _, id := range ids {
			c.OpenElementWithID(ID(id))
			c.ConfigureOpenElement(&ElementDeclaration{Clip: ClipElementConfig{Vertical: true}})
			c.CloseElement()
		}
		if _, err := c.EndLayout(); err != nil {
			t.Fatal(err)
		}
	}
	layout("A", "B", "C")
	*c.GetScrollContainerData(ID("A")).ScrollPosition = Vector2{Y: -5}
	if len(c.scrollContainerDatas) != 3 {
		t.Fatalf("want 3 scroll containers, got %d", len(c.scrollContainerDatas))
	}
	c.ResetScrollContainers()
	if c.GetScrollContainerData(ID("A")).Found {
		t.Fatal("scroll container found after reset")
	}
	layout("A")
	sd := c.GetScrollContainerData(ID("A"))
	if len(c.scrollContainerDatas) != 1 || !sd.Found || *sd.ScrollPosition != (Vector2{}) {
		t.Fatalf("after reset: %d scroll containers, %+v", len(c.scrollContainerDatas), sd)
	}
	allocs := testing.AllocsPerRun(10, func() {
		c.ResetScrollContainers()
		layout("A", "B")
	})
	if allocs != 0 {
		t.Errorf("%v allocations", allocs)
	}
}
