package clayref

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Tolerance is the maximum absolute (or relative, for large magnitudes) difference allowed between floats.
const Tolerance = 1e-3

// Compare returns a description of the differences between want (reference) and got.
// It returns the empty string when both responses are equivalent.
// At most maxDiffs differences are reported.
func Compare(want, got *Response, maxDiffs int) string {
	d := differ{max: maxDiffs}
	if len(want.Frames) != len(got.Frames) {
		d.addf("frame count: want %d, got %d", len(want.Frames), len(got.Frames))
	}
	for i := range min(len(want.Frames), len(got.Frames)) {
		d.frame(i, &want.Frames[i], &got.Frames[i])
	}
	return d.String()
}

type differ struct {
	b   strings.Builder
	n   int
	max int
	// textOverread is set once clay overflows its text measurement cache or wrapped line
	// array (internal_error). Its measured words or wrapped lines
	// may then index past the end of a text element's string, for the rest of the request,
	// rendering bytes beyond it. glay clamps to the string, so only require glay's text
	// to be a prefix of clay's.
	textOverread bool
}

func (d *differ) addf(format string, args ...any) {
	d.n++
	if d.n <= d.max {
		fmt.Fprintf(&d.b, format+"\n", args...)
	}
}

func (d *differ) String() string {
	if d.n > d.max {
		fmt.Fprintf(&d.b, "... and %d more differences\n", d.n-d.max)
	}
	return d.b.String()
}

func (d *differ) frame(fi int, want, got *FrameResult) {
	we, ge := slices.Clone(want.Errors), slices.Clone(got.Errors)
	slices.Sort(we)
	slices.Sort(ge)
	if !slices.Equal(we, ge) {
		d.addf("frame %d errors: want %v, got %v", fi, we, ge)
	}
	if len(want.Commands) != len(got.Commands) {
		d.addf("frame %d command count: want %d, got %d", fi, len(want.Commands), len(got.Commands))
	}
	d.textOverread = d.textOverread || slices.Contains(we, "text_measurement_capacity_exceeded") ||
		slices.Contains(we, "elements_capacity_exceeded") || slices.Contains(we, "internal_error")
	for i := range min(len(want.Commands), len(got.Commands)) {
		d.command(fi, i, &want.Commands[i], &got.Commands[i], d.textOverread)
	}
	if len(want.Query) != len(got.Query) {
		d.addf("frame %d query count: want %d, got %d", fi, len(want.Query), len(got.Query))
	}
	for i := range min(len(want.Query), len(got.Query)) {
		w, g := &want.Query[i], &got.Query[i]
		if w.ID != g.ID || w.Found != g.Found || w.ScrollFound != g.ScrollFound ||
			!floatsEq(w.BB[:], g.BB[:]) || !floatsEq(w.ScrollDims[:], g.ScrollDims[:]) {
			d.addf("frame %d query %d:\n\twant %+v\n\tgot  %+v", fi, i, *w, *g)
		}
	}
}

func (d *differ) command(fi, ci int, w, g *Command, textOverread bool) {
	var fields []string
	if w.Type != g.Type {
		fields = append(fields, "type")
	}
	if w.ID != g.ID {
		fields = append(fields, "id")
	}
	if w.Z != g.Z {
		fields = append(fields, "z")
	}
	if w.User != g.User {
		fields = append(fields, "user")
	}
	if !floatsEq(w.BB[:], g.BB[:]) {
		fields = append(fields, "bb")
	}
	if !floatsEq(w.Color, g.Color) {
		fields = append(fields, "color")
	}
	if !floatsEq(w.Radius, g.Radius) {
		fields = append(fields, "radius")
	}
	if !slices.Equal(w.Width, g.Width) {
		fields = append(fields, "width")
	}
	if (w.Text == nil) != (g.Text == nil) {
		fields = append(fields, "text")
	} else if w.Text != nil && *w.Text != *g.Text && !(textOverread && strings.HasPrefix(*w.Text, *g.Text)) {
		fields = append(fields, "text")
	}
	if w.Offset != g.Offset {
		fields = append(fields, "offset")
	}
	if w.Font != g.Font || w.Size != g.Size || w.Spacing != g.Spacing || w.LineH != g.LineH {
		fields = append(fields, "font")
	}
	if !slices.Equal(w.Clip, g.Clip) {
		fields = append(fields, "clip")
	}
	if w.Image != g.Image {
		fields = append(fields, "image")
	}
	if w.Custom != g.Custom {
		fields = append(fields, "custom")
	}
	if len(fields) > 0 {
		d.addf("frame %d command %d differs in %v:\n\twant %s\n\tgot  %s", fi, ci, fields, w, g)
	}
}

func (c *Command) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s id=%d z=%d bb=%v", c.Type, c.ID, c.Z, c.BB)
	if c.User != 0 {
		fmt.Fprintf(&b, " user=%d", c.User)
	}
	if c.Color != nil {
		fmt.Fprintf(&b, " color=%v", c.Color)
	}
	if c.Radius != nil {
		fmt.Fprintf(&b, " radius=%v", c.Radius)
	}
	if c.Width != nil {
		fmt.Fprintf(&b, " width=%v", c.Width)
	}
	if c.Text != nil {
		fmt.Fprintf(&b, " text=%q offset=%d font=%d size=%d spacing=%d lineH=%d", *c.Text, c.Offset, c.Font, c.Size, c.Spacing, c.LineH)
	}
	if c.Clip != nil {
		fmt.Fprintf(&b, " clip=%v", c.Clip)
	}
	if c.Image != 0 {
		fmt.Fprintf(&b, " image=%d", c.Image)
	}
	if c.Custom != 0 {
		fmt.Fprintf(&b, " custom=%d", c.Custom)
	}
	return b.String()
}

func floatsEq(a, b []F32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !floatEq(float64(a[i]), float64(b[i])) {
			return false
		}
	}
	return true
}

func floatEq(a, b float64) bool {
	if a == b || (math.IsNaN(a) && math.IsNaN(b)) {
		return true
	}
	diff := math.Abs(a - b)
	return diff <= Tolerance || diff <= Tolerance*1e-3*max(math.Abs(a), math.Abs(b))
}
