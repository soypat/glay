package clayref

import (
	"fmt"

	"github.com/soypat/glay"
)

// handle is an integer stand-in for clay's void* user data, image data and custom data.
type handle uint64

func toAny(v uint64) any {
	if v == 0 {
		return nil
	}
	return handle(v)
}

func fromAny(v any) uint64 {
	h, _ := v.(handle)
	return uint64(h)
}

var errNames = map[glay.Error]string{
	glay.ErrTextMeasurementFunctionNotProvided: "text_measurement_function_not_provided",
	glay.ErrArenaCapacityExceeded:              "arena_capacity_exceeded",
	glay.ErrElementsCapacityExceeded:           "elements_capacity_exceeded",
	glay.ErrTextMeasurementCapacityExceeded:    "text_measurement_capacity_exceeded",
	glay.ErrDuplicateID:                        "duplicate_id",
	glay.ErrFloatingContainerParentNotFound:    "floating_container_parent_not_found",
	glay.ErrPercentageOver1:                    "percentage_over_1",
	glay.ErrInternal:                           "internal_error",
	glay.ErrUnbalancedOpenClose:                "unbalanced_open_close",
	glay.ErrHashMapCapacityExceeded:            "hash_map_capacity_exceeded",
}

var cmdNames = map[glay.RenderCommandType]string{
	glay.RenderCommandTypeNone:         "none",
	glay.RenderCommandTypeRectangle:    "rectangle",
	glay.RenderCommandTypeBorder:       "border",
	glay.RenderCommandTypeText:         "text",
	glay.RenderCommandTypeImage:        "image",
	glay.RenderCommandTypeScissorStart: "scissor_start",
	glay.RenderCommandTypeScissorEnd:   "scissor_end",
	glay.RenderCommandTypeCustom:       "custom",
}

// RunGlay runs req through glay and returns the response in the same form the C harness produces.
func RunGlay(req *Request) (*Response, error) {
	maxElements := req.MaxElements
	if maxElements == 0 {
		maxElements = 8192
	}
	var frameErrs []string
	var ctx glay.Context
	err := ctx.Initialize(glay.Config{
		Layout:                       glay.Dimensions{Width: req.Layout.W, Height: req.Layout.H},
		MaxElementCount:              maxElements,
		MaxMeasureTextCacheWordCount: req.MaxWords,
		MeasureText: func(text string, cfg *glay.TextElementConfig, _ any) glay.Dimensions {
			w, h := req.Measure.MeasureText(text, cfg.LetterSpacing)
			return glay.Dimensions{Width: w, Height: h}
		},
		ErrorHandler: func(err glay.Error, _ any) {
			frameErrs = append(frameErrs, errNames[err])
		},
	})
	if err != nil {
		return nil, err
	}
	ctx.SetCullingEnabled(!req.DisableCulling)
	resp := &Response{}
	for fi := range req.Frames {
		f := &req.Frames[fi]
		if f.W != nil || f.H != nil {
			dims := ctx.LayoutDimensions()
			if f.W != nil {
				dims.Width = *f.W
			}
			if f.H != nil {
				dims.Height = *f.H
			}
			ctx.SetLayoutDimensions(dims)
		}
		frameErrs = frameErrs[:0]
		ctx.BeginLayout()
		depth := 0
		for i := range f.Ops {
			op := &f.Ops[i]
			switch op.Op {
			case "open":
				if id, ok := openID(&ctx, op); ok {
					ctx.OpenElementWithID(id)
				} else {
					ctx.OpenElement()
				}
				decl := toDecl(op.Decl)
				ctx.ConfigureOpenElement(&decl)
				depth++
			case "text":
				ctx.OpenTextElement(op.Text, toTextCfg(op.Cfg))
			case "close":
				if depth > 0 { // Closing the root element is not a valid use of the API, skip it.
					ctx.CloseElement()
					depth--
				}
			default:
				return nil, fmt.Errorf("frame %d op %d: unknown op %q", fi, i, op.Op)
			}
		}
		cmds, _ := ctx.EndLayout()
		fr := FrameResult{Errors: append([]string{}, frameErrs...), Commands: []Command{}, Query: []QueryResult{}}
		for i := range cmds {
			fr.Commands = append(fr.Commands, fromCommand(&cmds[i]))
		}
		for _, q := range f.Query {
			id := glay.ID(q)
			bb, found := ctx.GetElementData(id)
			sd := ctx.GetScrollContainerData(id)
			fr.Query = append(fr.Query, QueryResult{
				ID:          id.ID,
				Found:       found,
				BB:          bbox(bb),
				ScrollFound: sd.Found,
				ScrollDims: [4]F32{
					F32(sd.ScrollContainerDimensions.Width), F32(sd.ScrollContainerDimensions.Height),
					F32(sd.ContentDimensions.Width), F32(sd.ContentDimensions.Height),
				},
			})
		}
		resp.Frames = append(resp.Frames, fr)
	}
	return resp, nil
}

func openID(ctx *glay.Context, op *Op) (glay.ElementID, bool) {
	var seed uint32
	if op.Local {
		seed = ctx.GetOpenElementID()
	}
	switch {
	case op.SID != "":
		if op.Local {
			return ctx.IDLocal(op.SID), true
		}
		return glay.ID(op.SID), true
	case op.SIDI != nil:
		if seed != 0 {
			return ctx.IDILocal(op.SIDI.Label, op.SIDI.Index), true
		}
		return glay.IDI(op.SIDI.Label, op.SIDI.Index), true
	case op.ID != 0:
		return glay.IDFromUint(op.ID), true
	}
	return glay.ElementID{}, false
}

func color(c []float32) glay.Color {
	var v [4]float32
	copy(v[:], c)
	return glay.Color{R: v[0], G: v[1], B: v[2], A: v[3]}
}

func vec2(f []float32) [2]float32 {
	var v [2]float32
	copy(v[:], f)
	return v
}

func toAxis(a *Axis) glay.SizingAxis {
	if a == nil {
		return glay.SizingAxis{}
	}
	switch a.Type {
	case "grow":
		return glay.SizingAxis{MinMax: glay.SizingMinMax{Min: a.Min, Max: a.Max}, Type: glay.SizingGrow}
	case "percent":
		return glay.SizingAxisPercent(a.Pct)
	case "fixed":
		return glay.SizingAxis{MinMax: glay.SizingMinMax{Min: a.Min, Max: a.Max}, Type: glay.SizingFixed}
	}
	return glay.SizingAxis{MinMax: glay.SizingMinMax{Min: a.Min, Max: a.Max}, Type: glay.SizingFit}
}

func toDecl(d *Decl) (decl glay.ElementDeclaration) {
	if d == nil {
		return decl
	}
	if l := d.Layout; l != nil {
		var pad [4]uint16
		copy(pad[:], l.Padding)
		decl.Layout = glay.LayoutConfig{
			Sizing:   glay.Sizing{Width: toAxis(l.W), Height: toAxis(l.H)},
			Padding:  glay.Padding{Left: pad[0], Right: pad[1], Top: pad[2], Bottom: pad[3]},
			ChildGap: l.Gap,
		}
		switch l.AX {
		case "right":
			decl.Layout.ChildAlignment.X = glay.AlignXRight
		case "center":
			decl.Layout.ChildAlignment.X = glay.AlignXCenter
		}
		switch l.AY {
		case "bottom":
			decl.Layout.ChildAlignment.Y = glay.AlignYBottom
		case "center":
			decl.Layout.ChildAlignment.Y = glay.AlignYCenter
		}
		if l.Dir == "ttb" {
			decl.Layout.LayoutDirection = glay.TopToBottom
		}
	}
	decl.BackgroundColor = color(d.BG)
	var r [4]float32
	copy(r[:], d.Radius)
	decl.CornerRadius = glay.CornerRadius{TopLeft: r[0], TopRight: r[1], BottomLeft: r[2], BottomRight: r[3]}
	decl.AspectRatio.AspectRatio = d.Aspect
	decl.Image.ImageData = toAny(d.Image)
	decl.Custom.CustomData = toAny(d.Custom)
	decl.UserData = toAny(d.User)
	if fl := d.Floating; fl != nil {
		off, exp := vec2(fl.Offset), vec2(fl.Expand)
		var at [2]int
		copy(at[:], fl.Attach)
		f := &decl.Floating
		f.Offset = glay.Vector2{X: off[0], Y: off[1]}
		f.Expand = glay.Dimensions{Width: exp[0], Height: exp[1]}
		f.ParentID = fl.ParentID
		if fl.ParentSID != "" {
			f.ParentID = glay.ID(fl.ParentSID).ID
		}
		f.ZIndex = fl.Z
		f.AttachPoints = glay.FloatingAttachPoints{Element: glay.FloatingAttachPointType(at[0]), Parent: glay.FloatingAttachPointType(at[1])}
		if fl.Capture == "passthrough" {
			f.PointerCaptureMode = glay.PointerCaptureModePassthrough
		}
		switch fl.AttachTo {
		case "parent":
			f.AttachTo = glay.AttachToParent
		case "id":
			f.AttachTo = glay.AttachToElementWithID
		case "root":
			f.AttachTo = glay.AttachToRoot
		}
		if fl.ClipTo == "parent" {
			f.ClipTo = glay.ClipToAttachedParent
		}
	}
	if cl := d.Clip; cl != nil {
		off := vec2(cl.Offset)
		decl.Clip = glay.ClipElementConfig{Horizontal: cl.H, Vertical: cl.V, ChildOffset: glay.Vector2{X: off[0], Y: off[1]}}
	}
	if b := d.Border; b != nil {
		var w [5]uint16
		copy(w[:], b.Width)
		decl.Border = glay.BorderElementConfig{
			Color: color(b.Color),
			Width: glay.BorderWidth{Left: w[0], Right: w[1], Top: w[2], Bottom: w[3], BetweenChildren: w[4]},
		}
	}
	return decl
}

func toTextCfg(t *TextCfg) (cfg glay.TextElementConfig) {
	if t == nil {
		return cfg
	}
	cfg = glay.TextElementConfig{
		UserData:      toAny(t.User),
		TextColor:     color(t.Color),
		FontID:        t.Font,
		FontSize:      t.Size,
		LetterSpacing: t.Spacing,
		LineHeight:    t.LineH,
	}
	switch t.Wrap {
	case "newlines":
		cfg.WrapMode = glay.TextWrapNewlines
	case "none":
		cfg.WrapMode = glay.TextWrapNone
	}
	switch t.Align {
	case "center":
		cfg.TextAlignment = glay.TextAlignCenter
	case "right":
		cfg.TextAlignment = glay.TextAlignRight
	}
	return cfg
}

func bbox(bb glay.BoundingBox) [4]F32 {
	return [4]F32{F32(bb.X), F32(bb.Y), F32(bb.Width), F32(bb.Height)}
}

func colorF(c glay.Color) []F32 { return []F32{F32(c.R), F32(c.G), F32(c.B), F32(c.A)} }

func radiusF(r glay.CornerRadius) []F32 {
	return []F32{F32(r.TopLeft), F32(r.TopRight), F32(r.BottomLeft), F32(r.BottomRight)}
}

func fromCommand(rc *glay.RenderCommand) Command {
	d := &rc.RenderData
	c := Command{
		Type: cmdNames[rc.CommandType],
		BB:   bbox(rc.BoundingBox),
		ID:   rc.ID,
		Z:    rc.ZIndex,
		User: fromAny(rc.UserData),
	}
	switch rc.CommandType {
	case glay.RenderCommandTypeRectangle:
		c.Color, c.Radius = colorF(d.Color), radiusF(d.CornerRadius)
	case glay.RenderCommandTypeBorder:
		bw := d.BorderWidth
		c.Color, c.Radius = colorF(d.Color), radiusF(d.CornerRadius)
		c.Width = []uint16{bw.Left, bw.Right, bw.Top, bw.Bottom, bw.BetweenChildren}
	case glay.RenderCommandTypeText:
		text := d.Text
		c.Text = &text
		c.Offset = int(d.TextOffset)
		c.Color = colorF(d.Color)
		c.Font, c.Size, c.Spacing, c.LineH = d.FontID, d.FontSize, d.LetterSpacing, d.LineHeight
	case glay.RenderCommandTypeImage:
		c.Color, c.Radius = colorF(d.Color), radiusF(d.CornerRadius)
		c.Image = fromAny(d.Data)
	case glay.RenderCommandTypeScissorStart, glay.RenderCommandTypeScissorEnd:
		c.Clip = []bool{d.ClipHorizontal, d.ClipVertical}
	case glay.RenderCommandTypeCustom:
		c.Color, c.Radius = colorF(d.Color), radiusF(d.CornerRadius)
		c.Custom = fromAny(d.Data)
	}
	return c
}
