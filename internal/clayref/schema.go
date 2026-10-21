// Package clayref provides differential testing of glay against the upstream
// clay C implementation. A small C program built against clay.h
// (csrc/clayref.c) reads layout requests as JSON and writes the resulting render commands as JSON.
// The same requests are run through glay and the outputs compared.
//
// This package is test-only infrastructure and is not allocation conscious.
package clayref

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// Request is a sequence of layout frames run against a freshly initialized context.
// Frames within a request share persistent state (hash map, text measurement cache, scroll containers).
type Request struct {
	MaxElements    int     `json:"maxElements,omitempty"`
	MaxWords       int     `json:"maxWords,omitempty"`
	Layout         Dims    `json:"layout"`
	Measure        Measure `json:"measure"`
	DisableCulling bool    `json:"disableCulling,omitempty"`
	Frames         []Frame `json:"frames"`
}

// Dims are layout dimensions.
type Dims struct {
	W float32 `json:"w"`
	H float32 `json:"h"`
}

// Measure configures the deterministic monospace text measurer shared by both
// implementations. See [MeasureText].
type Measure struct {
	CharW float32 `json:"charW"`
	LineH float32 `json:"lineH"`
}

// MeasureText is the text measurement function both implementations use:
// width = len(text) * (CharW + letterSpacing), height = LineH.
func (m Measure) MeasureText(text string, letterSpacing uint16) (w, h float32) {
	return float32(len(text)) * (m.CharW + float32(letterSpacing)), m.LineH
}

// Frame is one BeginLayout/EndLayout pass. W and H, if set, change the layout dimensions before the frame.
type Frame struct {
	W     *float32 `json:"w,omitempty"`
	H     *float32 `json:"h,omitempty"`
	Ops   []Op     `json:"ops"`
	Query []string `json:"query,omitempty"` // Element string IDs to query after EndLayout.
}

// Op is a single element declaration operation: "open", "text" or "close".
// For "open" the element ID is taken from SID, SIDI or ID in that order; with none set the element is anonymous.
type Op struct {
	Op    string   `json:"op"`
	SID   string   `json:"sid,omitempty"`
	SIDI  *SIDI    `json:"sidi,omitempty"`
	ID    uint32   `json:"id,omitempty"`
	Local bool     `json:"local,omitempty"` // Seed ID hash with the open element's ID (CLAY_SID_LOCAL).
	Decl  *Decl    `json:"decl,omitempty"`
	Text  string   `json:"text,omitempty"`
	Cfg   *TextCfg `json:"cfg,omitempty"`
}

// SIDI is a string ID with an index, as in CLAY_SIDI. Encoded as [label, index].
type SIDI struct {
	Label string
	Index uint32
}

func (s SIDI) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{s.Label, s.Index})
}

func (s *SIDI) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("sidi: want [label, index], got %s", b)
	}
	if err := json.Unmarshal(raw[0], &s.Label); err != nil {
		return err
	}
	return json.Unmarshal(raw[1], &s.Index)
}

// Decl mirrors Clay_ElementDeclaration. Color and radius slices are [r,g,b,a] and [tl,tr,bl,br].
type Decl struct {
	Layout   *Layout   `json:"layout,omitempty"`
	BG       []float32 `json:"bg,omitempty"`
	Radius   []float32 `json:"radius,omitempty"`
	Aspect   float32   `json:"aspect,omitempty"`
	Image    uint64    `json:"image,omitempty"`
	Custom   uint64    `json:"custom,omitempty"`
	User     uint64    `json:"user,omitempty"`
	Floating *Floating `json:"floating,omitempty"`
	Clip     *Clip     `json:"clip,omitempty"`
	Border   *Border   `json:"border,omitempty"`
}

// Layout mirrors Clay_LayoutConfig. Padding is [left,right,top,bottom].
// AX is "left", "right" or "center". AY is "top", "bottom" or "center". Dir is "ltr" or "ttb".
type Layout struct {
	W       *Axis    `json:"w,omitempty"`
	H       *Axis    `json:"h,omitempty"`
	Padding []uint16 `json:"padding,omitempty"`
	Gap     uint16   `json:"gap,omitempty"`
	AX      string   `json:"ax,omitempty"`
	AY      string   `json:"ay,omitempty"`
	Dir     string   `json:"dir,omitempty"`
}

// Axis mirrors Clay_SizingAxis. Type is "fit", "grow", "percent" or "fixed".
type Axis struct {
	Type string  `json:"type,omitempty"`
	Min  float32 `json:"min,omitempty"`
	Max  float32 `json:"max,omitempty"`
	Pct  float32 `json:"pct,omitempty"`
}

// Floating mirrors Clay_FloatingElementConfig. Attach is [element, parent] attach point enum values.
// AttachTo is "none", "parent", "id" or "root". ClipTo is "none" or "parent". Capture is "capture" or "passthrough".
type Floating struct {
	Offset    []float32 `json:"offset,omitempty"`
	Expand    []float32 `json:"expand,omitempty"`
	ParentSID string    `json:"parentSid,omitempty"`
	ParentID  uint32    `json:"parentId,omitempty"`
	Z         int16     `json:"z,omitempty"`
	Attach    []int     `json:"attach,omitempty"`
	Capture   string    `json:"capture,omitempty"`
	AttachTo  string    `json:"attachTo,omitempty"`
	ClipTo    string    `json:"clipTo,omitempty"`
}

// Clip mirrors Clay_ClipElementConfig.
type Clip struct {
	H      bool      `json:"h,omitempty"`
	V      bool      `json:"v,omitempty"`
	Offset []float32 `json:"offset,omitempty"`
}

// Border mirrors Clay_BorderElementConfig. Width is [left,right,top,bottom,betweenChildren].
type Border struct {
	Color []float32 `json:"color,omitempty"`
	Width []uint16  `json:"width,omitempty"`
}

// TextCfg mirrors Clay_TextElementConfig. Wrap is "words", "newlines" or "none". Align is "left", "center" or "right".
type TextCfg struct {
	Color   []float32 `json:"color,omitempty"`
	Font    uint16    `json:"font,omitempty"`
	Size    uint16    `json:"size,omitempty"`
	Spacing uint16    `json:"spacing,omitempty"`
	LineH   uint16    `json:"lineH,omitempty"`
	Wrap    string    `json:"wrap,omitempty"`
	Align   string    `json:"align,omitempty"`
	User    uint64    `json:"user,omitempty"`
}

// Response is the result of running a [Request].
type Response struct {
	Fatal  string        `json:"fatal,omitempty"`
	Frames []FrameResult `json:"frames"`
}

// FrameResult holds the errors reported, render commands and query results of a single frame.
type FrameResult struct {
	Errors   []string      `json:"errors"`
	Commands []Command     `json:"commands"`
	Query    []QueryResult `json:"query"`
}

// Command is a render command. Only fields relevant to Type are set.
type Command struct {
	Type    string   `json:"type"`
	BB      [4]F32   `json:"bb"`
	ID      uint32   `json:"id"`
	Z       int16    `json:"z"`
	User    uint64   `json:"user"`
	Color   []F32    `json:"color,omitempty"`
	Radius  []F32    `json:"radius,omitempty"`
	Width   []uint16 `json:"width,omitempty"`
	Text    *string  `json:"text,omitempty"`
	Offset  int      `json:"offset,omitempty"`
	Font    uint16   `json:"font,omitempty"`
	Size    uint16   `json:"size,omitempty"`
	Spacing uint16   `json:"spacing,omitempty"`
	LineH   uint16   `json:"lineH,omitempty"`
	Clip    []bool   `json:"clip,omitempty"`
	Image   uint64   `json:"image,omitempty"`
	Custom  uint64   `json:"custom,omitempty"`
}

// QueryResult holds GetElementData and GetScrollContainerData results for a queried ID.
// ScrollDims is [containerW, containerH, contentW, contentH].
type QueryResult struct {
	ID          uint32 `json:"id"`
	Found       bool   `json:"found"`
	BB          [4]F32 `json:"bb"`
	ScrollFound bool   `json:"scrollFound"`
	ScrollDims  [4]F32 `json:"scrollDims"`
}

// F32 is a float32 whose JSON form may also be one of the strings "NaN", "+Inf" or "-Inf".
type F32 float32

func (f F32) MarshalJSON() ([]byte, error) {
	v := float64(f)
	switch {
	case math.IsNaN(v):
		return []byte(`"NaN"`), nil
	case math.IsInf(v, 1):
		return []byte(`"+Inf"`), nil
	case math.IsInf(v, -1):
		return []byte(`"-Inf"`), nil
	}
	return strconv.AppendFloat(nil, v, 'g', -1, 32), nil
}

func (f *F32) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case `"NaN"`:
		*f = F32(math.NaN())
		return nil
	case `"+Inf"`:
		*f = F32(math.Inf(1))
		return nil
	case `"-Inf"`:
		*f = F32(math.Inf(-1))
		return nil
	}
	v, err := strconv.ParseFloat(string(b), 32)
	*f = F32(v)
	return err
}
