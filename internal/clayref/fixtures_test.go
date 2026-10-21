package clayref

import (
	"sort"
	"testing"
	"time"
)

// TestClayRefFixtures checks glay against upstream clay on hand written layouts.
func TestClayRefFixtures(t *testing.T) {
	bin := Build(t)
	s, err := NewStream(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	s.Timeout = time.Second
	fx := fixtures()
	names := make([]string, 0, len(fx))
	for name := range fx {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if !diff(t, bin, &s, fx[name]) {
				t.Fatal("reference harness timed out")
			}
		})
	}
}

// Fixture building helpers.

func grow() *Axis                { return &Axis{Type: "grow"} }
func growMax(max float32) *Axis  { return &Axis{Type: "grow", Max: max} }
func fixed(v float32) *Axis      { return &Axis{Type: "fixed", Min: v, Max: v} }
func pct(p float32) *Axis        { return &Axis{Type: "percent", Pct: p} }
func fit(min, max float32) *Axis { return &Axis{Type: "fit", Min: min, Max: max} }
func rgb(r, g, b float32) []float32 {
	return []float32{r, g, b, 255}
}

func open(sid string, d *Decl, children ...[]Op) []Op {
	ops := []Op{{Op: "open", SID: sid, Decl: d}}
	for _, c := range children {
		ops = append(ops, c...)
	}
	return append(ops, Op{Op: "close"})
}

func text(s string, cfg *TextCfg) []Op { return []Op{{Op: "text", Text: s, Cfg: cfg}} }

func frame(query []string, ops ...[]Op) Frame {
	f := Frame{Query: query}
	for _, o := range ops {
		f.Ops = append(f.Ops, o...)
	}
	return f
}

func box(w, h *Axis, bg []float32) *Decl {
	return &Decl{Layout: &Layout{W: w, H: h}, BG: bg}
}

func fixtures() map[string]*Request {
	white := &TextCfg{Color: rgb(255, 255, 255), Size: 16}
	base := func(frames ...Frame) *Request {
		return &Request{MaxElements: 128, Layout: Dims{W: 600, H: 400}, Measure: Measure{CharW: 8, LineH: 16}, Frames: frames}
	}
	fx := map[string]*Request{}

	// Layout of examples/png.
	menu := []Op{}
	for i := range 3 {
		menu = append(menu, Op{Op: "open", SIDI: &SIDI{Label: "MenuItem", Index: uint32(i)}, Decl: box(grow(), fixed(28), rgb(220, 80, float32(80*i)))}, Op{Op: "close"})
	}
	fx["png_example"] = base(frame([]string{"Root", "Header", "Sidebar", "MainPanel", "Footer"},
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Padding: []uint16{12, 12, 12, 12}, Gap: 8, Dir: "ttb"}, BG: rgb(30, 30, 30)},
			open("Header", &Decl{Layout: &Layout{W: grow(), H: fixed(48), Padding: []uint16{8, 8, 8, 8}, Gap: 8}, BG: rgb(50, 50, 70)},
				open("HeaderLeft", box(fixed(80), grow(), rgb(90, 200, 170))),
				open("HeaderRight", box(fixed(80), grow(), rgb(200, 150, 80))),
			),
			open("Content", &Decl{Layout: &Layout{W: grow(), H: grow(), Gap: 8}},
				open("Sidebar", &Decl{Layout: &Layout{W: fixed(120), H: grow(), Padding: []uint16{8, 8, 8, 8}, Gap: 6, Dir: "ttb"}, BG: rgb(45, 45, 55)}, menu),
				open("MainPanel", box(grow(), grow(), rgb(55, 55, 65))),
			),
			open("Footer", &Decl{
				Layout: &Layout{W: grow(), H: fixed(32), Padding: []uint16{4, 4, 4, 4}, Gap: 8}, BG: rgb(50, 50, 70),
				Border: &Border{Color: rgb(200, 200, 50), Width: []uint16{2, 2, 2, 2, 2}},
			},
				open("FooterLeft", box(grow(), grow(), rgb(80, 80, 120))),
				open("FooterRight", box(grow(), grow(), rgb(120, 80, 80))),
			),
		),
	))

	// Text wrapping modes, alignment, line height and letter spacing.
	para := "the quick brown fox jumps over the lazy dog\nand then\n\nsleeps  soundly "
	fx["text"] = base(frame(nil,
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Gap: 4, Dir: "ttb"}},
			open("Words", box(fixed(150), fit(0, 0), nil), text(para, white)),
			open("Newlines", box(fixed(150), fit(0, 0), nil), text(para, &TextCfg{Color: rgb(255, 0, 0), Wrap: "newlines", LineH: 20})),
			open("None", box(fixed(150), fit(0, 0), nil), text(para, &TextCfg{Wrap: "none", Spacing: 1, Font: 2})),
			open("Center", box(fixed(200), fit(0, 0), nil), text(para, &TextCfg{Align: "center", Size: 12})),
			open("Right", box(fixed(200), fit(0, 0), nil), text(para, &TextCfg{Align: "right", User: 5})),
			open("Long", box(fixed(40), fit(0, 0), nil), text("supercalifragilistic word", white)),
			open("Grow", box(grow(), fit(0, 0), nil), text("short", white), text("another text sibling that wraps", white)),
		),
	))

	// Child alignment on both axes and directions.
	align := []Op{}
	for _, dir := range []string{"", "ttb"} {
		for _, ax := range []string{"", "center", "right"} {
			for _, ay := range []string{"", "center", "bottom"} {
				align = append(align, open("", &Decl{Layout: &Layout{W: fixed(60), H: fixed(60), Padding: []uint16{2, 4, 6, 8}, Gap: 3, AX: ax, AY: ay, Dir: dir}, BG: rgb(10, 10, 10)},
					open("", box(fixed(10), fixed(15), rgb(255, 0, 0))),
					open("", box(fixed(20), fixed(5), rgb(0, 255, 0))),
				)...)
			}
		}
	}
	fx["alignment"] = base(frame(nil, open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Gap: 2}}, align)))

	// Sizing: grow distribution with min/max, fit, percent, fixed, and compression when content is too large.
	fx["sizing"] = base(frame([]string{"G1", "G2", "G3", "P", "C1", "C2", "C3"},
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Dir: "ttb", Gap: 4}},
			open("Grow", &Decl{Layout: &Layout{W: grow(), H: fixed(30), Gap: 5}},
				open("G1", box(grow(), grow(), rgb(255, 0, 0))),
				open("G2", box(growMax(50), grow(), rgb(0, 255, 0))),
				open("G3", box(&Axis{Type: "grow", Min: 200}, grow(), rgb(0, 0, 255))),
				open("P", box(pct(0.2), pct(0.5), rgb(255, 255, 0))),
			),
			open("Compress", &Decl{Layout: &Layout{W: fixed(200), H: fit(0, 0), Gap: 4}},
				open("C1", box(fit(0, 0), fixed(20), nil), text("hello world wraps", white)),
				open("C2", box(fit(30, 0), fixed(20), rgb(0, 255, 255)), open("", box(fixed(150), fixed(10), nil))),
				open("C3", box(fixed(100), fixed(20), rgb(255, 0, 255))),
			),
			open("Fit", &Decl{Layout: &Layout{W: fit(0, 100), H: fit(50, 0), Padding: []uint16{3, 3, 3, 3}, Dir: "ttb"}, BG: rgb(9, 9, 9)},
				open("", box(fixed(150), fixed(10), rgb(1, 2, 3))),
			),
		),
	))

	// Floating elements attached to parent, root and by ID with every attach point, z order and clipping.
	floats := []Op{}
	for i := range 9 {
		floats = append(floats, Op{Op: "open", SIDI: &SIDI{Label: "Float", Index: uint32(i)}, Decl: &Decl{
			Layout:   &Layout{W: fixed(10), H: fixed(10)},
			BG:       rgb(255, float32(20*i), 0),
			Floating: &Floating{AttachTo: "parent", Attach: []int{i, 8 - i}, Offset: []float32{1, 2}, Z: int16(i % 3)},
		}}, Op{Op: "close"})
	}
	fx["floating"] = base(frame([]string{"Anchor", "Clipped", "Tooltip", "Overlay"},
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Padding: []uint16{50, 50, 50, 50}}},
			open("Anchor", box(fixed(100), fixed(80), rgb(40, 40, 40)), floats),
			open("Clipped", &Decl{Layout: &Layout{W: fixed(100), H: fixed(50)}, Clip: &Clip{H: true, V: true}},
				open("Tooltip", &Decl{
					Layout:   &Layout{W: fit(0, 0), H: fit(0, 0), Padding: []uint16{2, 2, 2, 2}},
					BG:       rgb(255, 255, 200),
					Floating: &Floating{AttachTo: "id", ParentSID: "Anchor", Attach: []int{3, 5}, ClipTo: "parent", Expand: []float32{2, 3}, Capture: "passthrough", Z: 10},
				}, text("tooltip", white)),
			),
			open("Overlay", &Decl{
				Layout:   &Layout{W: pct(0.5), H: grow()},
				BG:       []float32{0, 0, 0, 128},
				Floating: &Floating{AttachTo: "root", Attach: []int{4, 4}, Z: -1},
			}),
		),
	))

	// Scroll/clip containers with child offsets, nesting and scissor commands.
	items := []Op{}
	for i := range 8 {
		items = append(items, Op{Op: "open", SIDI: &SIDI{Label: "Item", Index: uint32(i)}, Decl: box(grow(), fixed(30), rgb(float32(30*i), 100, 100))}, Op{Op: "close"})
	}
	fx["clip"] = base(frame([]string{"ScrollV", "ScrollH", "Inner"},
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Gap: 10}},
			open("ScrollV", &Decl{Layout: &Layout{W: fixed(150), H: fixed(100), Dir: "ttb", Gap: 2, Padding: []uint16{4, 4, 4, 4}}, Clip: &Clip{V: true, Offset: []float32{0, -45}}}, items),
			open("ScrollH", &Decl{Layout: &Layout{W: fixed(100), H: fixed(100), Gap: 2}, Clip: &Clip{H: true, Offset: []float32{-30, 0}}, BG: rgb(5, 5, 5)},
				open("Inner", &Decl{Layout: &Layout{W: fixed(80), H: grow()}, Clip: &Clip{H: true, V: true}}, text("clipped text that is long", white)),
				open("", box(fixed(80), grow(), rgb(200, 0, 0))),
			),
		),
	))

	// Borders including between children in both directions.
	fx["border"] = base(frame(nil,
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Gap: 10, Dir: "ttb"}},
			open("Row", &Decl{
				Layout: &Layout{W: grow(), H: fixed(40), Gap: 6, Padding: []uint16{3, 3, 3, 3}},
				Border: &Border{Color: rgb(255, 0, 0), Width: []uint16{1, 2, 3, 4, 3}}, Radius: []float32{5, 5, 5, 5},
			}, open("", box(grow(), grow(), nil)), open("", box(grow(), grow(), nil)), open("", box(grow(), grow(), nil))),
			open("Col", &Decl{
				Layout: &Layout{W: fixed(100), H: grow(), Gap: 5, Dir: "ttb"},
				Border: &Border{Color: rgb(0, 255, 0), Width: []uint16{0, 0, 0, 0, 2}},
			}, open("", box(grow(), grow(), nil)), open("", box(grow(), grow(), nil))),
			open("Invisible", &Decl{
				Layout: &Layout{W: grow(), H: fixed(10), Gap: 4},
				Border: &Border{Color: []float32{1, 1, 1, 0}, Width: []uint16{1, 1, 1, 1, 1}},
			}, open("", box(grow(), grow(), nil)), open("", box(grow(), grow(), nil))),
		),
	))

	// Aspect ratio elements, images and custom elements.
	fx["aspect_image"] = base(frame([]string{"Img", "Wide", "Custom"},
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Gap: 5}},
			open("Img", &Decl{Layout: &Layout{W: fixed(100)}, Aspect: 2, Image: 7, BG: []float32{255, 255, 255, 128}, Radius: []float32{1, 2, 3, 4}}),
			open("Wide", &Decl{Layout: &Layout{W: grow(), H: fixed(30)}, Aspect: 0.5}),
			open("Custom", &Decl{Layout: &Layout{W: fixed(40), H: fixed(40)}, Custom: 9, BG: rgb(1, 2, 3), User: 4}),
		),
	))

	// Elements coming and going across frames exercise hash map pruning and the text cache.
	frameA := frame([]string{"A", "B", "C"}, open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Dir: "ttb"}},
		open("A", box(grow(), fixed(20), rgb(1, 0, 0)), text("frame text", white)),
		open("B", box(grow(), fixed(20), rgb(0, 1, 0))),
	))
	frameB := frame([]string{"A", "B", "C"}, open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Dir: "ttb"}},
		open("C", box(grow(), fixed(20), rgb(0, 0, 1)), text("other text", white)),
		open("A", box(grow(), fixed(40), rgb(1, 0, 0))),
	))
	w, h := float32(300), float32(200)
	frameResized := frameB
	frameResized.W, frameResized.H = &w, &h
	fx["multiframe"] = base(frameA, frameB, frameA, frameA, frameA, frameResized)

	// Errors: duplicate IDs, percentage over 1, floating parent not found.
	fx["errors"] = base(frame([]string{"Dup"},
		open("Root", &Decl{Layout: &Layout{W: grow(), H: grow()}},
			open("Dup", box(fixed(10), fixed(10), rgb(1, 1, 1))),
			open("Dup", box(fixed(20), fixed(20), rgb(2, 2, 2))),
			open("Pct", box(pct(1.5), pct(2), rgb(3, 3, 3))),
			open("Orphan", &Decl{Layout: &Layout{W: fixed(5), H: fixed(5)}, Floating: &Floating{AttachTo: "id", ParentSID: "Nope"}}),
		),
	))

	// Exceeding the element capacity yields a single error text command.
	over := []Op{}
	for range 40 {
		over = append(over, open("", box(fixed(1), fixed(1), rgb(1, 1, 1)))...)
	}
	overflow := base(frame(nil, open("Root", box(grow(), grow(), nil), over)), frame(nil, open("Root", box(grow(), grow(), nil))))
	overflow.MaxElements = 16
	overflow.MaxWords = 32
	fx["overflow"] = overflow

	// Culling: elements fully outside the layout produce no commands, unless culling is disabled.
	offscreen := frame(nil, open("Root", &Decl{Layout: &Layout{W: grow(), H: grow(), Dir: "ttb"}},
		open("", box(grow(), fixed(410), rgb(1, 1, 1))),
		open("Below", box(grow(), fixed(50), rgb(2, 2, 2)), text("hidden", white)),
	))
	fx["culling"] = base(offscreen)
	noCull := base(offscreen)
	noCull.DisableCulling = true
	fx["culling_disabled"] = noCull
	return fx
}
