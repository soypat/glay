package glay

import (
	"testing"
)

func monoMeasure(text string, config *TextElementConfig, _ any) Dimensions {
	return Dimensions{Width: float32(len(text)) * (8 + float32(config.LetterSpacing)), Height: 16}
}

type frameData struct {
	root, header, item, content, scroll, popup, image, text TextElementConfig
	decl                                                    [8]ElementDeclaration
	user                                                    int
}

func newFrameData() *frameData {
	fd := &frameData{}
	fd.decl[0] = ElementDeclaration{ // Root.
		BackgroundColor: Color{30, 30, 30, 255},
		Layout: LayoutConfig{
			LayoutDirection: TopToBottom,
			Sizing:          Sizing{Width: SizingAxisGrow(0, 0), Height: SizingAxisGrow(0, 0)},
			Padding:         PaddingAll(12),
			ChildGap:        8,
		},
	}
	fd.decl[1] = ElementDeclaration{ // Header with border between children.
		BackgroundColor: Color{50, 50, 70, 255},
		Layout:          LayoutConfig{Sizing: Sizing{Width: SizingAxisGrow(0, 0), Height: SizingAxisFixed(48)}, Padding: PaddingAll(8), ChildGap: 8},
		Border:          BorderElementConfig{Color: Color{200, 200, 50, 255}, Width: BorderWidth{2, 2, 2, 2, 2}},
		CornerRadius:    CornerRadius{4, 4, 4, 4},
		UserData:        &fd.user,
	}
	fd.decl[2] = ElementDeclaration{ // Header item.
		BackgroundColor: Color{90, 200, 170, 255},
		Layout:          LayoutConfig{Sizing: Sizing{Width: SizingAxisPercent(0.25), Height: SizingAxisGrow(0, 0)}},
	}
	fd.decl[3] = ElementDeclaration{ // Content.
		Layout: LayoutConfig{Sizing: Sizing{Width: SizingAxisGrow(0, 0), Height: SizingAxisGrow(0, 0)}, ChildGap: 8},
	}
	fd.decl[4] = ElementDeclaration{ // Scroll container.
		BackgroundColor: Color{45, 45, 55, 255},
		Layout:          LayoutConfig{LayoutDirection: TopToBottom, Sizing: Sizing{Width: SizingAxisFixed(120), Height: SizingAxisGrow(0, 0)}, Padding: PaddingAll(4)},
		Clip:            ClipElementConfig{Vertical: true, ChildOffset: Vector2{Y: -10}},
	}
	fd.decl[5] = ElementDeclaration{ // Floating popup.
		BackgroundColor: Color{200, 80, 80, 255},
		Layout:          LayoutConfig{Sizing: Sizing{Width: SizingAxisFixed(80), Height: SizingAxisFit(0, 0)}, Padding: PaddingAll(4)},
		Floating:        FloatingElementConfig{AttachTo: AttachToParent, ZIndex: 1, AttachPoints: FloatingAttachPoints{Element: AttachPointCenterTop, Parent: AttachPointCenterBottom}},
	}
	fd.decl[6] = ElementDeclaration{ // Image with aspect ratio.
		Layout:      LayoutConfig{Sizing: Sizing{Width: SizingAxisGrow(0, 0)}},
		AspectRatio: AspectRatioElementConfig{AspectRatio: 1.5},
		Image:       ImageElementConfig{ImageData: &fd.user},
	}
	fd.text = TextElementConfig{TextColor: Color{255, 255, 255, 255}, FontSize: 16, UserData: &fd.user}
	return fd
}

func (fd *frameData) layout(c *Context) {
	c.BeginLayout()
	c.OpenElementWithID(ID("Root"))
	c.ConfigureOpenElement(&fd.decl[0])
	{
		c.OpenElementWithID(ID("Header"))
		c.ConfigureOpenElement(&fd.decl[1])
		for i := range uint32(4) {
			c.OpenElementWithID(IDI("HeaderItem", i))
			c.ConfigureOpenElement(&fd.decl[2])
			c.CloseElement()
		}
		c.CloseElement()

		c.OpenElement()
		c.ConfigureOpenElement(&fd.decl[3])
		{
			c.OpenElementWithID(ID("Scroll"))
			c.ConfigureOpenElement(&fd.decl[4])
			for range 10 {
				c.OpenTextElement("hello world this text wraps around\nwith a newline", fd.text)
			}
			c.CloseElement()

			c.OpenElement()
			c.ConfigureOpenElement(&fd.decl[6])
			c.OpenElementWithID(c.IDLocal("Popup"))
			c.ConfigureOpenElement(&fd.decl[5])
			c.OpenTextElement("tooltip text", fd.text)
			c.CloseElement()
			c.CloseElement()
		}
		c.CloseElement()
	}
	c.CloseElement()
}

func newTestContext(t testing.TB) *Context {
	var c Context
	err := c.Initialize(Config{
		Layout:          Dimensions{Width: 600, Height: 400},
		MaxElementCount: 128,
		MeasureText:     monoMeasure,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &c
}

func TestFrameAllocs(t *testing.T) {
	c := newTestContext(t)
	fd := newFrameData()
	var ncmds int
	frame := func() {
		fd.layout(c)
		cmds, err := c.EndLayout()
		if err != nil {
			t.Fatal(err)
		}
		ncmds = len(cmds)
	}
	frame() // Warm up text measurement cache.
	allocs := testing.AllocsPerRun(100, frame)
	if allocs != 0 {
		t.Errorf("want 0 allocations per frame, got %v", allocs)
	}
	if ncmds < 20 {
		t.Errorf("expected a substantial number of render commands, got %d", ncmds)
	}
	// Pointer and scroll handling must also not allocate.
	allocs = testing.AllocsPerRun(100, func() {
		c.SetPointerState(Vector2{X: 50, Y: 100}, true)
		c.UpdateScrollContainers(true, Vector2{Y: -1}, 0.016)
	})
	if allocs != 0 {
		t.Errorf("want 0 allocations per pointer/scroll update, got %v", allocs)
	}
}

func TestReinitializeAllocs(t *testing.T) {
	c := newTestContext(t)
	allocs := testing.AllocsPerRun(10, func() {
		err := c.Initialize(Config{Layout: Dimensions{Width: 600, Height: 400}, MaxElementCount: 128, MeasureText: monoMeasure})
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("re-initializing with the same capacity should reuse memory, got %v allocations", allocs)
	}
}

func BenchmarkFrame(b *testing.B) {
	c := newTestContext(b)
	fd := newFrameData()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fd.layout(c)
		if _, err := c.EndLayout(); err != nil {
			b.Fatal(err)
		}
	}
}
