package glay

type (
	uintn    = uint32
	intn     = int32
	floatn   = float32
	strslice = []byte // Non-owning string slice
)

func ID(name string) ElementID {
	return hashString(name, 0, 0)
}

type Context struct {
	MaxElementCount               intn
	MaxMeasureTextCacheWordCount  intn
	WarningsEnabled               bool
	PointerInfo                   MousePointerData
	LayoutDimensions              Dimensions
	DynamicElementIndexBaseHash   ElementID
	DynamicElementIndex           uintn
	DebugModeEnabled              bool
	DisableCulling                bool
	ExternalScrollHandlingEnabled bool
	DebugSelectElementID          uintn
	Generation                    uintn
	MeasureTextUserData           any
	QueryScrollOffsetUserData     any
	// Layout elements / render commands
	LayoutElements              []LayoutElement
	renderCommands              []RenderCommand
	OpenLayoutElementStack      []intn
	LayoutElementChildren       []intn
	LayoutElementChildrenBuffer []intn
	TextElementData             []TextElementData
	ImageElementPointers        []intn
	ReusableElementIndexBuffer  []intn
	LayoutElementClipElementIDs []intn
	// Configs
	LayoutConfigs          []LayoutConfig
	ElementConfigs         []ElementConfig
	TextElementConfigs     []TextElementConfig
	ImageElementConfigs    []ImageElementConfig
	FloatingElementConfigs []FloatingElementConfig
	ScrollElementConfigs   []ScrollElementConfig
	CustomElementConfigs   []any
	BorderElementConfigs   []BorderElementConfig
	SharedElementConfigs   []SharedElementConfig
	// Misc Data Structures.
	LayoutElementIDStrings             []string
	WrappedTextLines                   []WrappedTextLine
	LayoutElementTreeNodes1            []layoutElementTreeNode
	LayoutElementTreeRoots             []layoutElementTreeRoot
	layoutElementHashMapInternal       []LayoutElementHashMapItem
	LayoutElementHashMap               []intn
	measureTextHashMapInternal         []measureTextCacheItem
	MeasureTextHashMapInternalFreelist []intn
	measureTextHashMap                 []intn
	measuredWords                      []measuredWord
	measuredWordsFreeList              []intn
	openClipElementStack               []intn
	PointerOverIDs                     []ElementID
	scrollContainerDatas               []scrollContainerDataInternal
	TreeNodeVisited                    []bool
	DynamicStringData                  []byte
	debugElementData                   []debugElementData
	GoHash                             map[uintn]*LayoutElementHashMapItem
}

type SharedElementConfig struct {
	BackgroundColor Color
	CornerRadius    CornerRadius
	UserData        any
}

type ElementConfigType uint8

const (
	ElementConfigTypeNone     ElementConfigType = iota // element config none
	ElementConfigTypeBorder                            // element config border
	ElementConfigTypeFloating                          // element config floating
	ElementConfigTypeScroll                            // element config scroll
	ElementConfigTypeImage                             // element config image
	ElementConfigTypeText                              // element config text
	ElementConfigTypeCustom                            // element config custom
	ElementConfigTypeShared                            // element config shared
)

func GetElementConfigType(a any) (Type ElementConfigType) {
	switch a.(type) {
	case *BorderElementConfig:
		Type = ElementConfigTypeBorder
	case *FloatingElementConfig:
		Type = ElementConfigTypeFloating
	case *ScrollElementConfig:
		Type = ElementConfigTypeScroll
	case *ImageElementConfig:
		Type = ElementConfigTypeImage
	case *TextElementConfig:
		Type = ElementConfigTypeText
	case *SharedElementConfig:
		Type = ElementConfigTypeShared
	default:
		panic("invalid element Config, make sure it is pointer type")
	}
	return Type
}

type ElementConfig struct {
	Type   ElementConfigType
	Config any
}

type WrappedTextLine struct {
	Dimensions Dimensions
	Line       string
}
type TextElementData struct {
	Text                string
	PreferredDimensions Dimensions
	ElementIndex        intn
	WrappedLines        []WrappedTextLine
}
type LayoutElement struct {
	ChildrenOrTextContent any // Is either []intn when children or TextContent.
	Dimensions            Dimensions
	MinDimensions         Dimensions
	LayoutConfig          *LayoutConfig
	ElementConfigs        []ElementConfig
	ID                    uintn
}

type layoutElementTreeNode struct {
	layoutElement   *LayoutElement
	position        Vector2
	NextChildOffset Vector2
}
type layoutElementTreeRoot struct {
	LayoutElementIndex intn
	ParentID           uintn
	ClipElementID      uintn
	Zindex             int16
	PointerOffset      Vector2
}
type LayoutElementHashMapItem struct {
	BoundingBox     BoundingBox
	ElementID       ElementID
	LayoutElement   *LayoutElement
	OnHover         func(_ ElementID, _ MousePointerData, userData any)
	OnHoverUserData any
	NextIndex       intn
	Generation      uintn
	IDAlias         uintn
}

func (hmi LayoutElementHashMapItem) isdefault() bool {
	return hmi.BoundingBox == BoundingBox{} && hmi.IDAlias == 0 && hmi.Generation == 0
}

type Dimensions struct {
	Width, Height floatn
}

type Color struct {
	R, G, B, A floatn
}

type BoundingBox struct {
	Vector2
	Dimensions
}

type Vector2 struct {
	X, Y floatn
}

type ElementID struct {
	ID       uintn
	Offset   uintn
	BaseID   uintn
	StringID string
}

type CornerRadius struct {
	TopLeft, TopRight, BottomLeft, BottomRight floatn
}

type LayoutDirection uint8

const (
	LeftToRight LayoutDirection = iota // left to right
	TopToBottom                        // top to bottom
)

type LayoutAlignmentX uint8

const (
	AlignXLeft   LayoutAlignmentX = iota // align x left
	AlignXRight                          // align x right
	AlignXCenter                         // align x center
)

type LayoutAlignmentY uint8

const (
	AlignYTop    LayoutAlignmentY = iota // align y top
	AlignYBottom                         // align y bottom
	AlignYCenter                         // align y center
)

type SizingType uint8

const (
	SizingFit     SizingType = iota // sizing fit
	SizingGrow                      // sizing grow
	SizingPercent                   // sizing percent
	SizingFixed                     // sizing fixed
)

type ChildAlignment struct {
	X LayoutAlignmentX
	Y LayoutAlignmentY
}

type SizingMinMax struct {
	Min, Max floatn
}

type SizingAxis struct {
	MinMax  SizingMinMax
	Percent floatn
	Type    SizingType
}

type Sizing struct {
	Width  SizingAxis
	Height SizingAxis
}

func NewSizingAxis(kind SizingType, a ...floatn) (ax SizingAxis) {
	ax.Type = kind
	need1Arg := kind == SizingPercent || kind == SizingFixed
	if len(a) == 0 || len(a) > 2 || need1Arg != (len(a) == 1) {
		panic("invalid number of arguments")
	} else if kind == SizingPercent {
		ax.Percent = a[0]
		return ax
	} else if kind == SizingFixed {
		ax.MinMax = SizingMinMax{Min: a[0], Max: a[0]}
		return ax
	}
	ax.MinMax.Min = a[0]
	ax.MinMax.Max = a[1]
	return ax
}

func (sz Sizing) SizingAxis(xaxis bool) SizingAxis {
	if xaxis {
		return sz.Width
	}
	return sz.Height
}

func (sz Sizing) Clamp(d Dimensions) Dimensions {
	d.Height = sz.ClampHeight(d.Height)
	d.Width = sz.ClampWidth(d.Width)
	return d
}
func (sz Sizing) ClampWidth(w floatn) floatn {
	return clamp(sz.Width.MinMax.Min, sz.Width.MinMax.Max, w)
}
func (sz Sizing) ClampHeight(h floatn) floatn {
	return clamp(sz.Height.MinMax.Min, sz.Height.MinMax.Max, h)
}

type Padding struct {
	Left, Right, Top, Bottom uint16
}

func PaddingAll(padding uint16) Padding {
	return Padding{
		Left: padding, Right: padding, Top: padding, Bottom: padding,
	}
}

func (pd Padding) Vertical() uint16   { return pd.Top + pd.Bottom }
func (pd Padding) Horizontal() uint16 { return pd.Left + pd.Right }

func (pd Padding) SizeAxis(xaxis bool) floatn {
	if xaxis {
		return floatn(pd.Horizontal())
	}
	return floatn(pd.Vertical())
}

type LayoutConfig struct {
	Sizing          Sizing
	Padding         Padding
	ChildGap        uint16
	ChildAlignment  ChildAlignment
	LayoutDirection LayoutDirection
}

type TextElementConfigWrapMode uint8

const (
	TextWrapWords    TextElementConfigWrapMode = iota // text wrap words
	TextWrapNewlines                                  // text wrap newlines
	TextWrapNone                                      // text wrap none
)

type TextAlignment uint8

const (
	TextAlignLeft   TextAlignment = iota // text align left
	TextAlignCenter                      // text align center
	TextAlignRight                       // text align right
)

type TextElementConfig struct {
	TextColor          Color
	FontID             uint16
	FontSize           uint16
	LetterSpacing      uint16
	LineHeight         uint16
	WrapMode           TextElementConfigWrapMode
	TextAlignment      TextAlignment
	HashStringContents bool
}

type ImageElementConfig struct {
	ImageData        any
	SourceDimensions Dimensions
}

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

type FloatingAttachPoints struct {
	Element FloatingAttachPointType
	Parent  FloatingAttachPointType
}

type MousePointerCaptureMode uint8

const (
	PointerCaptureModeCapture     MousePointerCaptureMode = iota // pointer mode capture
	PointerCaptureModePassthrough                                // pointer mode passthrough
)

type FloatingAttachToElement uint8

const (
	AttachToNone          FloatingAttachToElement = iota // attach to none
	AttachToParent                                       // attach to parent
	AttachToElementWithID                                // attach to element with ID
	AttachToRoot                                         // attach to root
)

type FloatingElementConfig struct {
	Offset             Vector2
	Expand             Dimensions
	ParentID           uintn
	Zindex             int16
	AttachPoints       FloatingAttachPoints
	PointerCaptureMode MousePointerCaptureMode
	AttachTo           FloatingAttachToElement
}

type ScrollElementConfig struct {
	Horizontal bool
	Vertical   bool
}

type BorderWidth struct {
	Left, Right, Top, Bottom, BetweenChildren uint16
}

type BorderElementConfig struct {
	Color Color
	Width BorderWidth
}

type TextRenderData struct {
	Contents      strslice
	TextColor     Color
	FontID        uint16
	FontSize      uint16
	LetterSpacing uint16
	LineHeight    uint16
}

type ImageRenderData struct {
	BackgroundColor  Color
	CornerRadius     CornerRadius
	SourceDimensions Dimensions
	ImageData        any
}

type ScrollRenderData struct {
	Horizontal, Vertical bool
}
type RectangleRenderData struct {
	BackgroundColor Color
	CornerRadius    CornerRadius
}
type BorderRenderData struct {
	Color        Color
	CornerRadius CornerRadius
	Width        BorderWidth
}
type CustomRenderData struct {
	BackgroundColor Color
	CornerRadius    CornerRadius
	CustomData      any
}
type RenderData any

type ScrollContainerData struct {
	ScrollPosition            *Vector2
	ScrollContainerDimensions Dimensions
	ContentDimensions         Dimensions
	Config                    ScrollElementConfig
	Found                     bool
}

type ElementData struct {
	BoundingBox BoundingBox
	Found       bool
}

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

type RenderCommand struct {
	BoundingBox BoundingBox
	RenderData  RenderData
	UserData    any
	ID          uintn
	Zindex      int16
	CommandType RenderCommandType
}

type MousePointerDataInteractionState uint8

const (
	PointerDataPressedThisFrame MousePointerDataInteractionState = iota // pointer pressed this frame
	PointerDataPressed                                                  // pointer pressed
	PointerReleasedThisFrame                                            // pointer released this frame
	PointerReleased                                                     // pointer released
)

type MousePointerData struct {
	Position Vector2
	State    MousePointerDataInteractionState
}

type ElementDeclaration struct {
	ID              ElementID
	Layout          LayoutConfig
	BackgroundColor Color
	CornerRadius    CornerRadius
	Image           ImageElementConfig
	Floating        FloatingElementConfig
	Scroll          ScrollElementConfig
	Border          BorderElementConfig
	UserData        any
}

type Error uint8

const (
	ErrTextMeasurementFunctionNotProvided Error = iota // a text measurement function wasn't provided using Clay_SetMeasureTextFunction(), or the provided function was null.
)

type debugElementData struct {
	collision, collapsed bool
}

type measuredWord struct {
	StartOffset intn
	Length      intn
	Width       floatn
	Next        intn
}

type measureTextCacheItem struct {
	unwrappedDimensions    Dimensions
	measureWordsStartIndex intn
	containsNewlines       bool
	// hash map data.
	ID         uintn
	nextIndex  intn
	generation uintn
}

type scrollContainerDataInternal struct {
	LayoutElement       *LayoutElement
	BoundingBox         BoundingBox
	ContentSize         Dimensions
	ScrollOrigin        Vector2
	PointerOrigin       Vector2
	ScrollMomentum      Vector2
	ScrollPosition      Vector2
	PreviousDelta       Vector2
	MomentumTime        floatn
	ElementID           uintn
	OpenThisFrame       bool
	PointerScrollActive bool
}
