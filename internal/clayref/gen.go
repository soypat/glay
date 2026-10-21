package clayref

import "strings"

// Gen deterministically builds a valid [Request] from arbitrary fuzz input.
// Once data is exhausted all choices resolve to their first option, so generation always terminates.
// Only features glay supports are generated.
func Gen(data []byte) *Request {
	g := gen{data: data}
	req := &Request{
		MaxElements: pick(&g, 256, 64, 32, 16, 1024),
		Layout:      Dims{W: pick[float32](&g, 600, 320, 100, 1024), H: pick[float32](&g, 400, 240, 100, 768)},
		Measure:     Measure{CharW: pick[float32](&g, 8, 4, 6.5), LineH: pick[float32](&g, 16, 8, 20)},
	}
	// Clay divides by MaxWords/32 to get the text cache bucket count, so MaxWords must be at least 32.
	req.MaxWords = max(32, req.MaxElements*pick(&g, 2, 1, 4))
	req.DisableCulling = g.chance(10)
	nframes := 1 + g.intn(3)
	for range nframes {
		var f Frame
		if g.chance(15) {
			w, h := pick[float32](&g, 300, 640, 50), pick[float32](&g, 200, 480, 50)
			f.W, f.H = &w, &h
		}
		g.ids = g.ids[:0]
		g.budget = req.MaxElements + req.MaxElements/4 // Sometimes exceed capacity.
		// Unbalanced open/close is not generated: upstream clay dereferences a nil children
		// pointer in that case after reporting the error.
		g.element(&f, 0)
		f.Query = append(f.Query, g.ids...)
		f.Query = append(f.Query, "Missing")
		req.Frames = append(req.Frames, f)
	}
	return req
}

type gen struct {
	data   []byte
	i      int
	budget int
	ids    []string
}

func (g *gen) byte() byte {
	if g.i >= len(g.data) {
		return 0
	}
	b := g.data[g.i]
	g.i++
	return b
}

func (g *gen) intn(n int) int {
	if n <= 1 {
		return 0
	}
	return int(g.byte()) % n
}

// chance returns true with roughly pct percent probability.
func (g *gen) chance(pct int) bool { return g.intn(100) >= 100-pct }

func pick[T any](g *gen, opts ...T) T { return opts[g.intn(len(opts))] }

var idPool = []string{"A", "B", "C", "D", "E", "F", "Root", "Panel"}

func (g *gen) element(f *Frame, depth int) {
	g.budget--
	op := Op{Op: "open", Decl: g.decl()}
	switch g.intn(5) {
	case 0, 1: // Anonymous.
	case 2:
		op.SID = pick(g, idPool...)
		g.ids = append(g.ids, op.SID)
	case 3:
		op.SIDI = &SIDI{Label: pick(g, idPool...), Index: uint32(g.intn(4))}
	case 4:
		op.SID = pick(g, idPool...)
		op.Local = true
	}
	f.Ops = append(f.Ops, op)
	nchild := 0
	if depth < 8 && g.budget > 0 {
		nchild = g.intn(5 - min(depth/2, 3))
	}
	for range nchild {
		if g.budget <= 0 {
			break
		}
		if g.chance(30) {
			g.budget--
			op := Op{Op: "text", Text: g.text(), Cfg: g.textcfg()}
			if strings.TrimSpace(op.Text) == "" {
				// Upstream clay subtracts letter spacing from the measured width, making whitespace-only
				// text negative width, which livelocks its compression loop.
				op.Cfg.Spacing = 0
			}
			f.Ops = append(f.Ops, op)
		} else {
			g.element(f, depth+1)
		}
	}
	f.Ops = append(f.Ops, Op{Op: "close"})
}

func (g *gen) axis() *Axis {
	switch g.intn(5) {
	case 0:
		return nil // Fit, zero min/max.
	case 1:
		return &Axis{Type: "fit", Min: pick[float32](g, 0, 10, 50), Max: pick[float32](g, 0, 100, 30)}
	case 2:
		return &Axis{Type: "grow", Min: pick[float32](g, 0, 10, 50), Max: pick[float32](g, 0, 200, 40)}
	case 3:
		return &Axis{Type: "percent", Pct: pick[float32](g, 0.5, 0.25, 1, 0.333, 1.5)}
	default:
		// Fixed sizes stay larger than the maximum generated padding: upstream clay livelocks
		// in its compression loop when an element's content box goes negative.
		v := pick[float32](g, 20, 50, 100, 33.3)
		return &Axis{Type: "fixed", Min: v, Max: v}
	}
}

func (g *gen) color() []float32 {
	if g.chance(40) {
		return nil
	}
	return []float32{float32(g.byte()), float32(g.byte()), float32(g.byte()), pick[float32](g, 255, 128, 0)}
}

func (g *gen) decl() *Decl {
	d := &Decl{
		Layout: &Layout{
			W: g.axis(), H: g.axis(),
			Gap: pick[uint16](g, 0, 4, 8),
			AX:  pick(g, "", "right", "center"),
			AY:  pick(g, "", "bottom", "center"),
			Dir: pick(g, "", "ttb"),
		},
		BG: g.color(),
	}
	if g.chance(50) {
		p := pick[uint16](g, 0, 2, 4)
		d.Layout.Padding = []uint16{p, pick[uint16](g, p, 0, 3), pick[uint16](g, p, 1), pick[uint16](g, p, 5)}
	}
	if g.chance(20) {
		r := pick[float32](g, 4, 0, 10)
		d.Radius = []float32{r, r, pick[float32](g, r, 2), r}
	}
	if g.chance(8) {
		d.Aspect = pick[float32](g, 1, 0.5, 2, 1.777)
	}
	if g.chance(8) {
		d.Image = uint64(1 + g.intn(3))
	}
	if g.chance(5) {
		d.Custom = uint64(1 + g.intn(3))
	}
	if g.chance(20) {
		d.User = uint64(1 + g.intn(9))
	}
	if g.chance(10) {
		fl := &Floating{
			Offset:   []float32{pick[float32](g, 0, 5, -10), pick[float32](g, 0, 5, -10)},
			Z:        pick[int16](g, 0, 1, -1, 5),
			Attach:   []int{g.intn(9), g.intn(9)},
			Capture:  pick(g, "", "passthrough"),
			AttachTo: pick(g, "parent", "root", "id"),
			ClipTo:   pick(g, "", "parent"),
		}
		if g.chance(20) {
			fl.Expand = []float32{pick[float32](g, 0, 2), pick[float32](g, 0, 3)}
		}
		if fl.AttachTo == "id" {
			fl.ParentSID = pick(g, idPool...)
		}
		d.Floating = fl
	}
	if g.chance(15) {
		d.Clip = &Clip{H: g.chance(50), V: g.chance(70), Offset: []float32{pick[float32](g, 0, -5, -20), pick[float32](g, 0, -10, -40)}}
	}
	if g.chance(15) {
		d.Border = &Border{
			Color: []float32{200, 200, 50, pick[float32](g, 255, 0)},
			Width: []uint16{pick[uint16](g, 1, 0, 2), pick[uint16](g, 1, 0), pick[uint16](g, 1, 0, 3), pick[uint16](g, 1, 0), pick[uint16](g, 0, 1, 2)},
		}
	}
	return d
}

var words = []string{"a", "hello", "wrap", "longerword", "x", "go", "UI"}

func (g *gen) text() string {
	n := g.intn(6)
	b := make([]byte, 0, 32)
	if g.chance(5) {
		b = append(b, ' ')
	}
	for i := range n {
		if i > 0 {
			b = append(b, pick(g, " ", " ", " ", "\n", "  ")...)
		}
		b = append(b, pick(g, words...)...)
	}
	if g.chance(5) {
		b = append(b, pick(g, " ", "\n")...)
	}
	return string(b)
}

func (g *gen) textcfg() *TextCfg {
	return &TextCfg{
		Color:   g.color(),
		Font:    uint16(g.intn(3)),
		Size:    pick[uint16](g, 16, 0, 12),
		Spacing: pick[uint16](g, 0, 1, 2),
		LineH:   pick[uint16](g, 0, 20, 10),
		Wrap:    pick(g, "", "newlines", "none"),
		Align:   pick(g, "", "center", "right"),
		User:    uint64(pick(g, 0, 0, 3)),
	}
}
