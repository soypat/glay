package glay

//go:generate stringer -linecomment -output=stringers.go -type=LayoutDirection,LayoutAlignmentX,LayoutAlignmentY,SizingType,TextElementConfigWrapMode,TextAlignment,FloatingAttachPointType,PointerCaptureMode,FloatingAttachToElement,FloatingClipToElement,RenderCommandType,PointerDataInteractionState,Error

// Internal clay types to better match the source and also
// allow easily switching between a 32-bit implementation or 64-bit.
type (
	uintn  = uint32
	intn   = int32
	floatn = float32
)

// ID returns the element ID for label, equivalent to CLAY_ID/CLAY_SID.
func ID(label string) ElementID { return hashString(label, 0) }

// IDI returns the element ID for label and index, equivalent to CLAY_IDI/CLAY_SIDI.
// Use it instead of formatting index into the label to avoid allocating.
func IDI(label string, index uint32) ElementID { return hashStringWithOffset(label, index, 0) }

// IDFromUint returns an element ID with the given raw hash value. id must not be zero,
// which is reserved for "no ID". Useful when element identity is managed externally.
func IDFromUint(id uint32) ElementID { return ElementID{ID: id} }

// Dimensions are a width and height.
type Dimensions struct {
	Width, Height floatn
}

// Vector2 is a 2D point or offset.
type Vector2 struct {
	X, Y floatn
}

// Color is conventionally represented as 0-255 per channel, but interpretation is up to the renderer.
type Color struct {
	R, G, B, A floatn
}

// BoundingBox is an axis aligned rectangle with its position relative to the root of the layout.
type BoundingBox struct {
	Vector2
	Dimensions
}

// ElementID is a hashed element identifier, primarily created with [ID] and [IDI].
type ElementID struct {
	ID       uintn  // The resulting hash generated from the other fields.
	Offset   uintn  // A numerical offset applied after computing the hash from StringID.
	BaseID   uintn  // A base hash value to start from, for example the parent element ID for local IDs.
	StringID string // The string ID that was hashed.
}

// CornerRadius controls the corner rounding of rectangles, borders and images.
type CornerRadius struct {
	TopLeft, TopRight, BottomLeft, BottomRight floatn
}

// LayoutDirection controls the direction in which child elements are laid out.
type LayoutDirection uint8

const (
	LeftToRight LayoutDirection = iota // left to right
	TopToBottom                        // top to bottom
)

// LayoutAlignmentX controls the alignment of child elements along the x axis.
type LayoutAlignmentX uint8

const (
	AlignXLeft   LayoutAlignmentX = iota // align x left
	AlignXRight                          // align x right
	AlignXCenter                         // align x center
)

// LayoutAlignmentY controls the alignment of child elements along the y axis.
type LayoutAlignmentY uint8

const (
	AlignYTop    LayoutAlignmentY = iota // align y top
	AlignYBottom                         // align y bottom
	AlignYCenter                         // align y center
)

// SizingType controls how an element takes up space inside its parent container.
type SizingType uint8

const (
	SizingFit     SizingType = iota // sizing fit
	SizingGrow                      // sizing grow
	SizingPercent                   // sizing percent
	SizingFixed                     // sizing fixed
)

// ChildAlignment controls how child elements are aligned on each axis.
type ChildAlignment struct {
	X LayoutAlignmentX
	Y LayoutAlignmentY
}

// SizingMinMax holds the minimum and maximum size in pixels an element may take on an axis.
type SizingMinMax struct {
	Min, Max floatn
}

// SizingAxis controls the sizing of an element along one axis.
//
// Clay stores the percentage in a union with MinMax. glay mirrors this: for
// [SizingPercent] the percentage is stored in MinMax.Min. Use the constructors
// [SizingAxisPercent] et al. instead of setting fields directly.
type SizingAxis struct {
	MinMax SizingMinMax
	Type   SizingType
}

// Percent returns the percentage of a [SizingPercent] axis.
func (ax SizingAxis) Percent() floatn { return ax.MinMax.Min }

// SizingAxisFit wraps tightly to the size of the element's contents, clamped to min and max (0 means unbounded).
func SizingAxisFit(min, max floatn) SizingAxis {
	return SizingAxis{MinMax: SizingMinMax{Min: min, Max: max}, Type: SizingFit}
}

// SizingAxisGrow expands to fill available space in the parent, clamped to min and max (0 means unbounded).
func SizingAxisGrow(min, max floatn) SizingAxis {
	return SizingAxis{MinMax: SizingMinMax{Min: min, Max: max}, Type: SizingGrow}
}

// SizingAxisFixed sizes the axis to exactly size pixels.
func SizingAxisFixed(size floatn) SizingAxis {
	return SizingAxis{MinMax: SizingMinMax{Min: size, Max: size}, Type: SizingFixed}
}

// SizingAxisPercent sizes the axis to percent (0..1) of the parent's axis size minus padding and child gaps.
func SizingAxisPercent(percent floatn) SizingAxis {
	return SizingAxis{MinMax: SizingMinMax{Min: percent}, Type: SizingPercent}
}

// Sizing controls the sizing of an element along both axes.
type Sizing struct {
	Width  SizingAxis
	Height SizingAxis
}

// Padding is a gap in pixels between the bounding box of an element and where its children are placed.
type Padding struct {
	Left, Right, Top, Bottom uint16
}

// PaddingAll returns padding with all sides set to padding.
func PaddingAll(padding uint16) Padding {
	return Padding{Left: padding, Right: padding, Top: padding, Bottom: padding}
}

// LayoutConfig controls the size and position of an element and its children.
type LayoutConfig struct {
	Sizing          Sizing
	Padding         Padding
	ChildGap        uint16 // Gap between child elements along the layout axis.
	ChildAlignment  ChildAlignment
	LayoutDirection LayoutDirection
}

// TextElementConfigWrapMode controls how text is broken into lines when there is insufficient horizontal space.
type TextElementConfigWrapMode uint8

const (
	TextWrapWords    TextElementConfigWrapMode = iota // text wrap words
	TextWrapNewlines                                  // text wrap newlines
	TextWrapNone                                      // text wrap none
)

// TextAlignment controls how wrapped lines of text are horizontally aligned within their bounding box.
type TextAlignment uint8

const (
	TextAlignLeft   TextAlignment = iota // text align left
	TextAlignCenter                      // text align center
	TextAlignRight                       // text align right
)

// TextElementConfig controls text element measurement and rendering.
type TextElementConfig struct {
	UserData      any // Passed through to the resulting render commands.
	TextColor     Color
	FontID        uint16
	FontSize      uint16
	LetterSpacing uint16 // Extra horizontal space between characters.
	LineHeight    uint16 // Height of each wrapped line. Zero uses the measured height.
	WrapMode      TextElementConfigWrapMode
	TextAlignment TextAlignment
}

// AspectRatioElementConfig sets the target width/height ratio of an element.
type AspectRatioElementConfig struct {
	AspectRatio floatn
}

// ImageElementConfig marks an element as an image. ImageData is passed through to the renderer.
type ImageElementConfig struct {
	ImageData any
}

// FloatingAttachPointType selects a point on an element's bounding box.
type FloatingAttachPointType uint8

const (
	AttachPointLeftTop      FloatingAttachPointType = iota // attach left top
	AttachPointLeftCenter                                  // attach left center
	AttachPointLeftBottom                                  // attach left bottom
	AttachPointCenterTop                                   // attach center top
	AttachPointCenterCenter                                // attach center center
	AttachPointCenterBottom                                // attach center bottom
	AttachPointRightTop                                    // attach right top
	AttachPointRightCenter                                 // attach right center
	AttachPointRightBottom                                 // attach right bottom
)

// FloatingAttachPoints controls where a floating element attaches to its parent.
type FloatingAttachPoints struct {
	Element FloatingAttachPointType // Origin point on the floating element.
	Parent  FloatingAttachPointType // Point on the parent the floating element attaches to.
}

// PointerCaptureMode controls whether pointer events pass through a floating element.
type PointerCaptureMode uint8

const (
	PointerCaptureModeCapture     PointerCaptureMode = iota // pointer mode capture
	PointerCaptureModePassthrough                           // pointer mode passthrough
)

// FloatingAttachToElement controls which element a floating element is positioned relative to.
type FloatingAttachToElement uint8

const (
	AttachToNone          FloatingAttachToElement = iota // attach to none
	AttachToParent                                       // attach to parent
	AttachToElementWithID                                // attach to element with ID
	AttachToRoot                                         // attach to root
)

// FloatingClipToElement controls whether a floating element inherits clipping from the element it is attached to.
type FloatingClipToElement uint8

const (
	ClipToNone           FloatingClipToElement = iota // clip to none
	ClipToAttachedParent                              // clip to attached parent
)

// FloatingElementConfig makes an element float above others without affecting the layout of siblings or parents.
// Floating is enabled by setting AttachTo to something other than [AttachToNone].
type FloatingElementConfig struct {
	Offset             Vector2
	Expand             Dimensions // Expands the bounding box without affecting children.
	ParentID           uintn      // Element to attach to when AttachTo is AttachToElementWithID.
	ZIndex             int16
	AttachPoints       FloatingAttachPoints
	PointerCaptureMode PointerCaptureMode
	AttachTo           FloatingAttachToElement
	ClipTo             FloatingClipToElement
}

// CustomElementConfig creates custom render commands. CustomData is passed through to the renderer.
type CustomElementConfig struct {
	CustomData any
}

// ClipElementConfig clips overflowing children and offsets them, for scroll containers.
type ClipElementConfig struct {
	Horizontal  bool
	Vertical    bool
	ChildOffset Vector2
}

// BorderWidth holds per side border widths. BetweenChildren draws borders between children along the layout direction.
type BorderWidth struct {
	Left, Right, Top, Bottom, BetweenChildren uint16
}

func (bw BorderWidth) hasAnyWidth() bool {
	return bw.BetweenChildren > 0 || bw.Left > 0 || bw.Right > 0 || bw.Top > 0 || bw.Bottom > 0
}

// BorderElementConfig configures element borders.
type BorderElementConfig struct {
	Color Color
	Width BorderWidth
}

// ElementDeclaration configures an element. It mirrors Clay_ElementDeclaration.
type ElementDeclaration struct {
	Layout LayoutConfig
	// BackgroundColor generates a rectangle render command if no other config is specified,
	// otherwise it is passed as a property to image or custom render commands.
	BackgroundColor Color
	CornerRadius    CornerRadius
	AspectRatio     AspectRatioElementConfig
	Image           ImageElementConfig
	Floating        FloatingElementConfig
	Custom          CustomElementConfig
	Clip            ClipElementConfig
	Border          BorderElementConfig
	// UserData is passed through to render commands. Store pointers to avoid allocating.
	UserData any
}

// ScrollContainerData is the state of a scroll (clip) container.
type ScrollContainerData struct {
	// ScrollPosition points to the internal scroll position, which may be mutated.
	ScrollPosition            *Vector2
	ScrollContainerDimensions Dimensions
	ContentDimensions         Dimensions
	Config                    ClipElementConfig
	Found                     bool
}

// RenderCommandType determines how a render command should be handled.
type RenderCommandType uint8

const (
	RenderCommandTypeNone         RenderCommandType = iota // render:none
	RenderCommandTypeRectangle                             // render:rectangle
	RenderCommandTypeBorder                                // render:border
	RenderCommandTypeText                                  // render:text
	RenderCommandTypeImage                                 // render:image
	RenderCommandTypeScissorStart                          // render:scissor-start
	RenderCommandTypeScissorEnd                            // render:scissor-end
	RenderCommandTypeCustom                                // render:custom
)

// RenderData holds command specific data. Clay uses a union; glay flattens it and
// CommandType selects which fields are meaningful:
//   - Rectangle: Color, CornerRadius.
//   - Border: Color, CornerRadius, BorderWidth.
//   - Text: Text, TextOffset, Color, FontID, FontSize, LetterSpacing, LineHeight.
//   - Image: Color (tint), CornerRadius, Data (image data).
//   - Custom: Color, CornerRadius, Data (custom data).
//   - ScissorStart/ScissorEnd: ClipHorizontal, ClipVertical.
type RenderData struct {
	Color        Color
	CornerRadius CornerRadius
	BorderWidth  BorderWidth
	// Text is a zero-copy subslice of the string passed to OpenTextElement.
	Text string
	// TextOffset is the offset of Text within the source string (clay's baseChars).
	TextOffset     int32
	FontID         uint16
	FontSize       uint16
	LetterSpacing  uint16
	LineHeight     uint16
	ClipHorizontal bool
	ClipVertical   bool
	Data           any
}

// RenderCommand is a single draw instruction produced by [Context.EndLayout].
type RenderCommand struct {
	BoundingBox BoundingBox
	RenderData  RenderData
	UserData    any
	ID          uintn
	// ZIndex is the z order required for drawing this command correctly. Commands are already
	// sorted in ascending z order; this field is intended for batching renderers.
	ZIndex      int16
	CommandType RenderCommandType
}

// PointerDataInteractionState is the state of pointer interaction this frame.
type PointerDataInteractionState uint8

const (
	PointerDataPressedThisFrame  PointerDataInteractionState = iota // pointer pressed this frame
	PointerDataPressed                                              // pointer pressed
	PointerDataReleasedThisFrame                                    // pointer released this frame
	PointerDataReleased                                             // pointer released
)

// PointerData holds pointer position and interaction state.
type PointerData struct {
	Position Vector2
	State    PointerDataInteractionState
}

// Error is a layout error. Values mirror Clay_ErrorType.
type Error uint8

const (
	ErrTextMeasurementFunctionNotProvided Error = iota // text measurement function not provided
	ErrArenaCapacityExceeded                           // arena capacity exceeded
	ErrElementsCapacityExceeded                        // elements capacity exceeded; increase MaxElementCount
	ErrTextMeasurementCapacityExceeded                 // text measurement capacity exceeded; increase MaxMeasureTextCacheWordCount
	ErrDuplicateID                                     // element declared with an ID already used in this layout
	ErrFloatingContainerParentNotFound                 // floating element parent ID not found
	ErrPercentageOver1                                 // sizing percent over 1; expect range 0..1, i.e: 20% is 0.2
	ErrInternal                                        // internal error
	ErrUnbalancedOpenClose                             // unbalanced OpenElement/CloseElement calls
	ErrHashMapCapacityExceeded                         // element ID hash map capacity exceeded; increase MaxElementCount
	ErrInvalidConfig                                   // invalid configuration
)

func (e Error) Error() string { return e.String() }

// ErrorHandler is called for every error clay would report through its error handler.
type ErrorHandler func(err Error, userData any)
