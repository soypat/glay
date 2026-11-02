package glay

import (
	"math"
	"unsafe"
)

const (
	eps              floatn = 0.01
	maxfloat         floatn = math.MaxFloat32
	rootContainerStr        = "Clay__RootContainer"
	// maxScrollContainers is the capacity of the persistent scroll container array.
	maxScrollContainers = 100
)

const (
	defaultMaxElementCount = 8192
)

// defaultLayoutConfig is the layout config of text elements. Never modified.
var defaultLayoutConfig LayoutConfig

// Config configures a [Context] on [Context.Initialize].
type Config struct {
	// Layout are the initial layout dimensions, i.e. the screen size for a full screen layout.
	Layout Dimensions
	// MaxElementCount is the maximum number of elements (including text elements) in a layout.
	// Defaults to 8192.
	MaxElementCount int
	// MaxMeasureTextCacheWordCount is the maximum number of measured words cached.
	// Must be at least 32 and no more than 32*MaxElementCount. Defaults to max(32, 2*MaxElementCount).
	MaxMeasureTextCacheWordCount int
	// MeasureText measures the dimensions of text. Required to use text elements.
	// It must not retain text.
	MeasureText func(text string, config *TextElementConfig, userData any) Dimensions
	// MeasureTextUserData is passed to MeasureText.
	MeasureTextUserData any
	// ErrorHandler, if set, is called for every error clay would report.
	ErrorHandler ErrorHandler
	// ErrorHandlerUserData is passed to ErrorHandler.
	ErrorHandlerUserData any
}

// BooleanWarnings are sticky per-frame warnings, reset on [Context.BeginLayout].
type BooleanWarnings struct {
	MaxElementsExceeded           bool
	MaxRenderCommandsExceeded     bool
	MaxTextMeasureCacheExceeded   bool
	TextMeasurementFunctionNotSet bool
	HashMapCapacityExceeded       bool
}

// Context holds all layout state. All memory is allocated in [Context.Initialize];
// laying out frames does not allocate.
type Context struct {
	maxElementCount               intn
	maxMeasureTextCacheWordCount  intn
	errorHandler                  ErrorHandler
	errorHandlerUserData          any
	booleanWarnings               BooleanWarnings
	frameErr                      Error
	frameHasErr                   bool
	pointerInfo                   PointerData
	layoutDimensions              Dimensions
	disableCulling                bool
	externalScrollHandlingEnabled bool
	rootResizedLastFrame          bool
	generation                    uintn
	neededElements                intn
	measureText                   func(text string, config *TextElementConfig, userData any) Dimensions
	measureTextUserData           any
	queryScrollOffset             func(elementID uint32, userData any) Vector2
	queryScrollOffsetUserData     any
	// Layout elements / render commands.
	layoutElements              []layoutElement
	renderCommands              []RenderCommand
	openLayoutElementStack      []intn
	layoutElementChildren       []intn
	layoutElementChildrenBuffer []intn
	reusableElementIndexBuffer  []intn
	layoutElementClipElementIDs []intn
	// Misc data structures.
	layoutElementIDStrings             []string
	wrappedTextLines                   []WrappedTextLine
	layoutElementTreeNodeArray1        []layoutElementTreeNode
	layoutElementTreeRoots             []layoutElementTreeRoot
	layoutElementsHashMapInternal      []hashMapItem
	layoutElementsHashMap              []intn
	layoutElementsHashMapFreeList      []intn
	measureTextHashMapInternal         []measureTextCacheItem
	measureTextHashMapInternalFreeList []intn
	measureTextHashMap                 []intn
	measuredWords                      []measuredWord
	measuredWordsFreeList              []intn
	openClipElementStack               []intn
	pointerOverIDs                     []ElementID
	scrollContainerDatas               []scrollContainerDataInternal
	treeNodeVisited                    []bool
	// def holds the values clay returns pointers to on lookup misses and capacity overflows.
	// Clay sometimes writes through these pointers, so they are mutable per-context state.
	def defaults
}

type defaults struct {
	element  layoutElement
	hashItem hashMapItem
	measure  measureTextCacheItem
	scroll   scrollContainerDataInternal
	wrapped  WrappedTextLine
	treeNode layoutElementTreeNode
	treeRoot layoutElementTreeRoot
	word     measuredWord
	id       ElementID
}

func (d *defaults) reset() {
	*d = defaults{}
	d.hashItem.layoutElement = -1
	d.scroll.layoutElement = -1
	d.treeNode.layoutElement = -1
}

// WrappedTextLine is a line of wrapped text.
type WrappedTextLine struct {
	Dimensions Dimensions
	Line       string // Zero-copy subslice of the text element's string.
	Offset     intn   // Offset of Line within the text element's string.
	// length is clay's line length, which can be -1 when a line ends on a newline
	// preceded by a space. Such lines are empty but still generate render commands.
	length intn
}

func newWrappedLine(text string, dims Dimensions, off, n intn) WrappedTextLine {
	return WrappedTextLine{Dimensions: dims, Line: substr(text, off, n), Offset: off, length: n}
}

type textElementData struct {
	text                string
	preferredDimensions Dimensions
	wrappedLinesStart   intn // Index into Context.wrappedTextLines backing array.
	wrappedLinesLength  intn
}

type layoutElement struct {
	childrenOffset        intn // Index into Context.layoutElementChildren, set on close.
	childrenLength        uint16
	dimensions            Dimensions
	minDimensions         Dimensions
	config                ElementDeclaration // Unused for text elements.
	textConfig            TextElementConfig  // Text elements only.
	textElementData       textElementData    // Text elements only.
	id                    uintn
	floatingChildrenCount uint16
	isTextElement         bool
}

type scrollContainerDataInternal struct {
	layoutElement       intn // Index into Context.layoutElements, -1 if none.
	boundingBox         BoundingBox
	contentSize         Dimensions
	scrollOrigin        Vector2
	pointerOrigin       Vector2
	scrollMomentum      Vector2
	scrollPosition      Vector2
	previousDelta       Vector2
	momentumTime        floatn
	elementID           uintn
	openThisFrame       bool
	pointerScrollActive bool
}

type hashMapItem struct {
	boundingBox           BoundingBox
	elementID             ElementID
	layoutElement         intn // Index into Context.layoutElements, -1 if none.
	onHoverFunction       func(elementID ElementID, pointerInfo PointerData, userData any)
	hoverFunctionUserData any
	nextIndex             intn
	generation            uintn
	appearedThisFrame     bool
}

type measuredWord struct {
	startOffset intn
	length      intn
	width       floatn
	next        intn
}

type measureTextCacheItem struct {
	unwrappedDimensions     Dimensions
	measuredWordsStartIndex intn
	minWidth                floatn
	containsNewlines        bool
	// Hash map data.
	id         uintn
	nextIndex  intn
	generation uintn
}

type layoutElementTreeNode struct {
	layoutElement   intn // Index into Context.layoutElements.
	position        Vector2
	nextChildOffset Vector2
}

type layoutElementTreeRoot struct {
	layoutElementIndex intn
	parentID           uintn // Zero for the root layout tree.
	clipElementID      uintn // Zero if there is no clip element.
	zIndex             int16
	pointerOffset      Vector2 // Only used when scroll containers are managed externally.
}

// Initialize allocates all memory the context needs and prepares it for layout.
// If the context was previously initialized with sufficient capacity its memory is reused.
func (c *Context) Initialize(cfg Config) error {
	maxElements := cfg.MaxElementCount
	if maxElements == 0 {
		maxElements = defaultMaxElementCount
	}
	maxWords := cfg.defaultWords(maxElements)
	// Clay computes the text cache bucket count as maxWords/32 and indexes a
	// maxElements sized array with it.
	if maxElements < 2 || maxElements > math.MaxInt32 || maxWords < 32 || maxWords/32 > maxElements {
		return ErrInvalidConfig
	}
	c.maxElementCount = intn(maxElements)
	c.maxMeasureTextCacheWordCount = intn(maxWords)
	c.errorHandler = cfg.ErrorHandler
	c.errorHandlerUserData = cfg.ErrorHandlerUserData
	c.measureText = cfg.MeasureText
	c.measureTextUserData = cfg.MeasureTextUserData
	c.layoutDimensions = cfg.Layout
	c.booleanWarnings = BooleanWarnings{}
	c.frameHasErr = false
	c.pointerInfo = PointerData{}
	c.generation = 0
	c.rootResizedLastFrame = false
	c.def.reset()
	c.initializePersistentMemory()
	c.initializeEphemeralMemory()
	c.layoutElementsHashMap = c.layoutElementsHashMap[:cap(c.layoutElementsHashMap)]
	for i := range c.layoutElementsHashMap {
		c.layoutElementsHashMap[i] = -1
	}
	c.measureTextHashMap = c.measureTextHashMap[:cap(c.measureTextHashMap)]
	clear(c.measureTextHashMap)
	c.measureTextHashMapInternal = c.measureTextHashMapInternal[:1] // Reserve the 0 value to mean "no next element".
	c.measureTextHashMapInternal[0] = measureTextCacheItem{}
	return nil
}

func (cfg Config) defaultWords(maxElements int) int {
	if cfg.MaxMeasureTextCacheWordCount != 0 {
		return cfg.MaxMeasureTextCacheWordCount
	}
	return max(32, 2*maxElements)
}

// MemoryRequirements returns the number of bytes [Context.Initialize] allocates for cfg,
// the equivalent of Clay_MinMemorySize. It does not include memory referenced by
// strings and interfaces stored in the context.
func (cfg Config) MemoryRequirements() int {
	maxElements := cfg.MaxElementCount
	if maxElements == 0 {
		maxElements = defaultMaxElementCount
	}
	maxWords := cfg.defaultWords(maxElements)
	perElement := unsafe.Sizeof(hashMapItem{}) + unsafe.Sizeof(measureTextCacheItem{}) + unsafe.Sizeof(ElementID{}) +
		unsafe.Sizeof(layoutElement{}) + unsafe.Sizeof("") + unsafe.Sizeof(WrappedTextLine{}) +
		unsafe.Sizeof(layoutElementTreeNode{}) + unsafe.Sizeof(layoutElementTreeRoot{}) +
		unsafe.Sizeof(RenderCommand{}) + unsafe.Sizeof(false) +
		11*unsafe.Sizeof(intn(0)) // Hash map, free lists, stacks and index buffers.
	perWord := unsafe.Sizeof(measuredWord{}) + unsafe.Sizeof(intn(0))
	return maxElements*int(perElement) + maxWords*int(perWord) + maxScrollContainers*int(unsafe.Sizeof(scrollContainerDataInternal{}))
}

func (c *Context) initializePersistentMemory() {
	maxElements := c.maxElementCount
	maxWords := c.maxMeasureTextCacheWordCount
	alloc(&c.scrollContainerDatas, maxScrollContainers)
	alloc(&c.layoutElementsHashMapInternal, maxElements)
	alloc(&c.layoutElementsHashMap, maxElements)
	alloc(&c.layoutElementsHashMapFreeList, maxElements)
	alloc(&c.measureTextHashMapInternal, maxElements)
	alloc(&c.measureTextHashMapInternalFreeList, maxElements)
	alloc(&c.measuredWordsFreeList, maxWords)
	alloc(&c.measureTextHashMap, maxElements)
	alloc(&c.measuredWords, maxWords)
	alloc(&c.pointerOverIDs, maxElements)
}

func (c *Context) initializeEphemeralMemory() {
	maxElements := c.maxElementCount
	alloc(&c.layoutElementChildrenBuffer, maxElements)
	alloc(&c.layoutElements, maxElements)
	alloc(&c.layoutElementIDStrings, maxElements)
	alloc(&c.wrappedTextLines, maxElements)
	alloc(&c.layoutElementTreeNodeArray1, maxElements)
	alloc(&c.layoutElementTreeRoots, maxElements)
	alloc(&c.layoutElementChildren, maxElements)
	alloc(&c.openLayoutElementStack, maxElements)
	alloc(&c.renderCommands, maxElements)
	alloc(&c.treeNodeVisited, maxElements)
	c.treeNodeVisited = c.treeNodeVisited[:cap(c.treeNodeVisited)] // This array is accessed directly rather than behaving as a list.
	alloc(&c.openClipElementStack, maxElements)
	alloc(&c.reusableElementIndexBuffer, maxElements)
	alloc(&c.layoutElementClipElementIDs, maxElements)
}

// alloc sets dst to an empty slice with capacity exactly n, reusing dst's memory when possible.
// This is the equivalent of carving an array out of clay's arena.
func alloc[T any](dst *[]T, n intn) {
	if cap(*dst) >= int(n) {
		*dst = (*dst)[:0:n]
		return
	}
	*dst = make([]T, 0, n)
}

// SetMeasureTextFunction sets the text measurement function. See [Config.MeasureText].
func (c *Context) SetMeasureTextFunction(fn func(text string, config *TextElementConfig, userData any) Dimensions, userData any) {
	c.measureText = fn
	c.measureTextUserData = userData
}

// SetQueryScrollOffsetFunction sets the function used to query scroll offsets when external scroll handling is enabled.
func (c *Context) SetQueryScrollOffsetFunction(fn func(elementID uint32, userData any) Vector2, userData any) {
	c.queryScrollOffset = fn
	c.queryScrollOffsetUserData = userData
}

// SetLayoutDimensions updates the layout dimensions, i.e. in response to the window being resized.
func (c *Context) SetLayoutDimensions(dimensions Dimensions) {
	c.rootResizedLastFrame = !floatEqual(c.layoutDimensions.Width, dimensions.Width) || !floatEqual(c.layoutDimensions.Height, dimensions.Height)
	c.layoutDimensions = dimensions
}

// LayoutDimensions returns the current layout dimensions.
func (c *Context) LayoutDimensions() Dimensions { return c.layoutDimensions }

// SetCullingEnabled enables or disables culling of render commands for elements outside the layout dimensions.
func (c *Context) SetCullingEnabled(enabled bool) { c.disableCulling = !enabled }

// SetExternalScrollHandlingEnabled enables querying scroll offsets with the function set by [Context.SetQueryScrollOffsetFunction].
func (c *Context) SetExternalScrollHandlingEnabled(enabled bool) {
	c.externalScrollHandlingEnabled = enabled
}

// MaxElementCount returns the maximum number of elements supported by the context's configuration.
func (c *Context) MaxElementCount() int { return int(c.maxElementCount) }

// MaxMeasureTextCacheWordCount returns the maximum number of measured words the text cache can hold.
func (c *Context) MaxMeasureTextCacheWordCount() int { return int(c.maxMeasureTextCacheWordCount) }

// Warnings returns the sticky warnings of the current frame.
func (c *Context) Warnings() BooleanWarnings { return c.booleanWarnings }

// NeededElementCount returns the number of elements the last layout attempted to declare,
// including the root element. When the frame exceeded capacity, re-initialize with a
// MaxElementCount greater than this value.
func (c *Context) NeededElementCount() int { return int(c.neededElements) }

func (c *Context) report(err Error) {
	if !c.frameHasErr {
		c.frameErr = err
		c.frameHasErr = true
	}
	if c.errorHandler != nil {
		c.errorHandler(err, c.errorHandlerUserData)
	}
}

// BeginLayout starts a new layout. Elements are then declared with [Context.OpenElement] and friends.
func (c *Context) BeginLayout() {
	c.initializeEphemeralMemory()
	c.generation++
	c.neededElements = 0
	rootDimensions := c.layoutDimensions
	c.booleanWarnings = BooleanWarnings{}
	c.frameHasErr = false
	c.OpenElementWithID(ID(rootContainerStr))
	c.ConfigureOpenElement(&ElementDeclaration{
		Layout: LayoutConfig{Sizing: Sizing{Width: SizingAxisFixed(rootDimensions.Width), Height: SizingAxisFixed(rootDimensions.Height)}},
	})
	addv(c, &c.openLayoutElementStack, 0)
	add(c, &c.layoutElementTreeRoots, layoutElementTreeRoot{layoutElementIndex: 0}, &c.def.treeRoot)
}

// EndLayout computes the layout and returns the render commands, which are valid until the next call to BeginLayout.
// The returned error is the first error reported during the frame. Render commands are still
// valid unless the error is [ErrElementsCapacityExceeded], in which case they only hold an error message.
func (c *Context) EndLayout() ([]RenderCommand, error) {
	c.CloseElement()
	if len(c.openLayoutElementStack) > 1 {
		c.report(ErrUnbalancedOpenClose)
	}
	if c.booleanWarnings.MaxElementsExceeded {
		const message = "Clay Error: Layout elements exceeded Clay__maxElementCount"
		c.addRenderCommand(RenderCommand{
			BoundingBox: BoundingBox{Vector2: Vector2{X: c.layoutDimensions.Width/2 - 59*4, Y: c.layoutDimensions.Height / 2}},
			RenderData:  RenderData{Text: message, Color: Color{R: 255, G: 0, B: 0, A: 255}, FontSize: 16},
			CommandType: RenderCommandTypeText,
		})
	} else {
		c.calculateFinalLayout()
	}
	c.pruneHashMap()
	if c.booleanWarnings.MaxElementsExceeded {
		return c.renderCommands, ErrElementsCapacityExceeded
	} else if c.frameHasErr {
		return c.renderCommands, c.frameErr
	}
	return c.renderCommands, nil
}

func (c *Context) pruneHashMap() {
	for i := range c.layoutElementsHashMap {
		currentElementIndex := c.layoutElementsHashMap[i]
		previousElementIndex := intn(-1)
		for currentElementIndex != -1 {
			currentItem := get(c, c.layoutElementsHashMapInternal, currentElementIndex, &c.def.hashItem)
			nextIndex := currentItem.nextIndex
			if currentItem.generation <= c.generation { // Needs to be pruned.
				set(c, &c.layoutElementsHashMapInternal, currentElementIndex, hashMapItem{nextIndex: -1, layoutElement: -1})
				addv(c, &c.layoutElementsHashMapFreeList, currentElementIndex)
				if previousElementIndex == -1 {
					// Top of the bucket, rewrite the first bucket pointer.
					c.layoutElementsHashMap[i] = nextIndex
				} else {
					get(c, c.layoutElementsHashMapInternal, previousElementIndex, &c.def.hashItem).nextIndex = nextIndex
				}
				currentElementIndex = nextIndex
			} else {
				previousElementIndex = currentElementIndex
				currentElementIndex = nextIndex
			}
		}
	}
}

// Clay declares an element with the given ID, configured by decl, whose children are declared by children.
// A zero id declares an anonymous element. It is a convenience over [Context.OpenElementWithID],
// [Context.ConfigureOpenElement] and [Context.CloseElement], equivalent to the CLAY() macro.
// Closures may allocate; use the raw API on hot paths.
func (c *Context) Clay(id ElementID, decl *ElementDeclaration, children ...func(c *Context)) {
	if id.ID != 0 {
		c.OpenElementWithID(id)
	} else {
		c.OpenElement()
	}
	c.ConfigureOpenElement(decl)
	for _, child := range children {
		child(c)
	}
	c.CloseElement()
}

// Text declares a text element as a child of the open element. Equivalent to CLAY_TEXT.
func (c *Context) Text(text string, config TextElementConfig) {
	c.OpenTextElement(text, config)
}

func (c *Context) openLayoutElementIndex() intn {
	return getValue(c, c.openLayoutElementStack, intn(len(c.openLayoutElementStack))-1)
}

func (c *Context) openLayoutElement() *layoutElement {
	return get(c, c.layoutElements, c.openLayoutElementIndex(), &c.def.element)
}

func (c *Context) parentElement() *layoutElement {
	return get(c, c.layoutElements, getValue(c, c.openLayoutElementStack, intn(len(c.openLayoutElementStack))-2), &c.def.element)
}

// GetOpenElementID returns the ID of the currently open element.
func (c *Context) GetOpenElementID() uint32 { return c.openLayoutElement().id }

// IDLocal returns an element ID for label seeded with the open element's ID, equivalent to CLAY_ID_LOCAL.
func (c *Context) IDLocal(label string) ElementID { return hashString(label, c.GetOpenElementID()) }

// IDILocal returns an element ID for label and index seeded with the open element's ID, equivalent to CLAY_IDI_LOCAL.
func (c *Context) IDILocal(label string, index uint32) ElementID {
	return hashStringWithOffset(label, index, c.GetOpenElementID())
}

func (c *Context) child(el *layoutElement, i intn) intn {
	return getValue(c, c.layoutElementChildren[:cap(c.layoutElementChildren)], el.childrenOffset+i)
}

// OpenElement opens an anonymous element whose ID is derived from its parent. It must be
// configured with [Context.ConfigureOpenElement] and closed with [Context.CloseElement].
func (c *Context) OpenElement() {
	c.neededElements++
	if len(c.layoutElements) == cap(c.layoutElements)-1 || c.booleanWarnings.MaxElementsExceeded {
		c.booleanWarnings.MaxElementsExceeded = true
		return
	}
	openLayoutElement := add(c, &c.layoutElements, layoutElement{}, &c.def.element)
	idx := intn(len(c.layoutElements) - 1)
	addv(c, &c.openLayoutElementStack, idx)
	// Generate an ID.
	parentElement := c.parentElement()
	offset := uintn(parentElement.childrenLength) + uintn(parentElement.floatingChildrenCount)
	elementID := hashNumber(offset, parentElement.id)
	openLayoutElement.id = elementID.ID
	c.addHashMapItem(elementID, idx)
	addv(c, &c.layoutElementIDStrings, elementID.StringID)
	c.setClipElementID(idx)
}

// OpenElementWithID opens an element with the given ID. It must be configured with
// [Context.ConfigureOpenElement] and closed with [Context.CloseElement].
func (c *Context) OpenElementWithID(elementID ElementID) {
	c.neededElements++
	if len(c.layoutElements) == cap(c.layoutElements)-1 || c.booleanWarnings.MaxElementsExceeded {
		c.booleanWarnings.MaxElementsExceeded = true
		return
	}
	add(c, &c.layoutElements, layoutElement{id: elementID.ID}, &c.def.element)
	idx := intn(len(c.layoutElements) - 1)
	addv(c, &c.openLayoutElementStack, idx)
	c.addHashMapItem(elementID, idx)
	addv(c, &c.layoutElementIDStrings, elementID.StringID)
	c.setClipElementID(idx)
}

func (c *Context) setClipElementID(idx intn) {
	var clipID intn
	if len(c.openClipElementStack) > 0 {
		clipID = getValue(c, c.openClipElementStack, intn(len(c.openClipElementStack))-1)
	}
	set(c, &c.layoutElementClipElementIDs, idx, clipID)
}

// ConfigureOpenElement configures the open element. The declaration is copied.
func (c *Context) ConfigureOpenElement(declaration *ElementDeclaration) {
	openIdx := c.openLayoutElementIndex()
	openLayoutElement := c.openLayoutElement()
	openLayoutElement.config = *declaration
	if (declaration.Layout.Sizing.Width.Type == SizingPercent && declaration.Layout.Sizing.Width.Percent() > 1) ||
		(declaration.Layout.Sizing.Height.Type == SizingPercent && declaration.Layout.Sizing.Height.Percent() > 1) {
		c.report(ErrPercentageOver1)
	}

	if declaration.Floating.AttachTo != AttachToNone {
		floatingConfig := &openLayoutElement.config.Floating
		// Because of the auto generated root element the depth of the tree will always be at least 2 here.
		hierarchicalParent := c.parentElement()
		var clipElementID uintn
		switch declaration.Floating.AttachTo {
		case AttachToParent:
			// Attach to the element's direct hierarchical parent.
			floatingConfig.ParentID = hierarchicalParent.id
			if len(c.openClipElementStack) > 0 {
				clipElementID = uintn(getValue(c, c.openClipElementStack, intn(len(c.openClipElementStack))-1))
			}
		case AttachToElementWithID:
			parentItem := c.getHashMapItem(floatingConfig.ParentID)
			if parentItem == &c.def.hashItem {
				c.report(ErrFloatingContainerParentNotFound)
			} else {
				clipElementID = uintn(getValue(c, c.layoutElementClipElementIDs, parentItem.layoutElement))
			}
		case AttachToRoot:
			floatingConfig.ParentID = hashString(rootContainerStr, 0).ID
		}
		if declaration.Floating.ClipTo == ClipToNone {
			clipElementID = 0
		}
		currentElementIndex := c.openLayoutElementIndex()
		set(c, &c.layoutElementClipElementIDs, currentElementIndex, intn(clipElementID))
		addv(c, &c.openClipElementStack, intn(clipElementID))
		add(c, &c.layoutElementTreeRoots, layoutElementTreeRoot{
			layoutElementIndex: currentElementIndex,
			parentID:           floatingConfig.ParentID,
			clipElementID:      clipElementID,
			zIndex:             floatingConfig.ZIndex,
		}, &c.def.treeRoot)
	}

	if declaration.Clip.Horizontal || declaration.Clip.Vertical {
		addv(c, &c.openClipElementStack, intn(openLayoutElement.id))
		// Retrieve or create cached data to track scroll position across frames.
		var scrollOffset *scrollContainerDataInternal
		for i := range c.scrollContainerDatas {
			mapping := &c.scrollContainerDatas[i]
			if openLayoutElement.id == mapping.elementID {
				scrollOffset = mapping
				scrollOffset.layoutElement = openIdx
				scrollOffset.openThisFrame = true
			}
		}
		if scrollOffset == nil {
			scrollOffset = add(c, &c.scrollContainerDatas, scrollContainerDataInternal{
				layoutElement: openIdx,
				scrollOrigin:  Vector2{X: -1, Y: -1},
				elementID:     openLayoutElement.id,
				openThisFrame: true,
			}, &c.def.scroll)
		}
		if c.externalScrollHandlingEnabled && c.queryScrollOffset != nil {
			scrollOffset.scrollPosition = c.queryScrollOffset(scrollOffset.elementID, c.queryScrollOffsetUserData)
		}
	}
}

// CloseElement closes the open element.
func (c *Context) CloseElement() {
	if c.booleanWarnings.MaxElementsExceeded {
		return
	}
	openLayoutElement := c.openLayoutElement()
	layoutConfig := &openLayoutElement.config.Layout
	elementHasClipHorizontal := openLayoutElement.config.Clip.Horizontal
	elementHasClipVertical := openLayoutElement.config.Clip.Vertical
	if (elementHasClipHorizontal || elementHasClipVertical || openLayoutElement.config.Floating.AttachTo != AttachToNone) && len(c.openClipElementStack) > 0 {
		c.openClipElementStack = c.openClipElementStack[:len(c.openClipElementStack)-1]
	}

	leftRightPadding := floatn(int(layoutConfig.Padding.Left) + int(layoutConfig.Padding.Right))
	topBottomPadding := floatn(int(layoutConfig.Padding.Top) + int(layoutConfig.Padding.Bottom))

	// Attach children to the current open element.
	openLayoutElement.childrenOffset = intn(len(c.layoutElementChildren))
	nchildren := intn(openLayoutElement.childrenLength)
	bufLen := intn(len(c.layoutElementChildrenBuffer))
	childGap := floatn(cmax(int(nchildren)-1, 0) * int(layoutConfig.ChildGap))
	if layoutConfig.LayoutDirection == LeftToRight {
		openLayoutElement.dimensions.Width = leftRightPadding
		openLayoutElement.minDimensions.Width = leftRightPadding
		for i := intn(0); i < nchildren; i++ {
			childIndex := getValue(c, c.layoutElementChildrenBuffer, bufLen-nchildren+i)
			child := get(c, c.layoutElements, childIndex, &c.def.element)
			openLayoutElement.dimensions.Width += child.dimensions.Width
			openLayoutElement.dimensions.Height = cmax(openLayoutElement.dimensions.Height, child.dimensions.Height+topBottomPadding)
			// Minimum size of child elements doesn't matter to clip containers as they can shrink and hide their contents.
			if !elementHasClipHorizontal {
				openLayoutElement.minDimensions.Width += child.minDimensions.Width
			}
			if !elementHasClipVertical {
				openLayoutElement.minDimensions.Height = cmax(openLayoutElement.minDimensions.Height, child.minDimensions.Height+topBottomPadding)
			}
			addv(c, &c.layoutElementChildren, childIndex)
		}
		openLayoutElement.dimensions.Width += childGap
		if !elementHasClipHorizontal {
			openLayoutElement.minDimensions.Width += childGap
		}
	} else if layoutConfig.LayoutDirection == TopToBottom {
		openLayoutElement.dimensions.Height = topBottomPadding
		openLayoutElement.minDimensions.Height = topBottomPadding
		for i := intn(0); i < nchildren; i++ {
			childIndex := getValue(c, c.layoutElementChildrenBuffer, bufLen-nchildren+i)
			child := get(c, c.layoutElements, childIndex, &c.def.element)
			openLayoutElement.dimensions.Height += child.dimensions.Height
			openLayoutElement.dimensions.Width = cmax(openLayoutElement.dimensions.Width, child.dimensions.Width+leftRightPadding)
			// Minimum size of child elements doesn't matter to clip containers as they can shrink and hide their contents.
			if !elementHasClipVertical {
				openLayoutElement.minDimensions.Height += child.minDimensions.Height
			}
			if !elementHasClipHorizontal {
				openLayoutElement.minDimensions.Width = cmax(openLayoutElement.minDimensions.Width, child.minDimensions.Width+leftRightPadding)
			}
			addv(c, &c.layoutElementChildren, childIndex)
		}
		openLayoutElement.dimensions.Height += childGap
		if !elementHasClipVertical {
			openLayoutElement.minDimensions.Height += childGap
		}
	}
	c.layoutElementChildrenBuffer = c.layoutElementChildrenBuffer[:cmax(bufLen-nchildren, 0)]

	// Clamp element min and max width to the values configured in the layout.
	if layoutConfig.Sizing.Width.Type != SizingPercent {
		mm := &layoutConfig.Sizing.Width.MinMax
		if mm.Max <= 0 { // Set the max size if the user didn't specify, makes calculations easier.
			mm.Max = maxfloat
		}
		openLayoutElement.dimensions.Width = cmin(cmax(openLayoutElement.dimensions.Width, mm.Min), mm.Max)
		openLayoutElement.minDimensions.Width = cmin(cmax(openLayoutElement.minDimensions.Width, mm.Min), mm.Max)
	} else {
		openLayoutElement.dimensions.Width = 0
	}
	// Clamp element min and max height to the values configured in the layout.
	if layoutConfig.Sizing.Height.Type != SizingPercent {
		mm := &layoutConfig.Sizing.Height.MinMax
		if mm.Max <= 0 { // Set the max size if the user didn't specify, makes calculations easier.
			mm.Max = maxfloat
		}
		openLayoutElement.dimensions.Height = cmin(cmax(openLayoutElement.dimensions.Height, mm.Min), mm.Max)
		openLayoutElement.minDimensions.Height = cmin(cmax(openLayoutElement.minDimensions.Height, mm.Min), mm.Max)
	} else {
		openLayoutElement.dimensions.Height = 0
	}

	updateAspectRatioBox(openLayoutElement)

	elementIsFloating := openLayoutElement.config.Floating.AttachTo != AttachToNone

	// Close the currently open element.
	closingElementIndex := removeSwapback(c, &c.openLayoutElementStack, intn(len(c.openLayoutElementStack))-1)

	// Get the currently open parent.
	openLayoutElement = c.openLayoutElement()
	if len(c.openLayoutElementStack) > 1 {
		if elementIsFloating {
			openLayoutElement.floatingChildrenCount++
			return
		}
		openLayoutElement.childrenLength++
		addv(c, &c.layoutElementChildrenBuffer, closingElementIndex)
	}
}

func updateAspectRatioBox(el *layoutElement) {
	ar := el.config.AspectRatio.AspectRatio
	if ar != 0 {
		if el.dimensions.Width == 0 && el.dimensions.Height != 0 {
			el.dimensions.Width = el.dimensions.Height * ar
		} else if el.dimensions.Width != 0 && el.dimensions.Height == 0 {
			el.dimensions.Height = el.dimensions.Width * (1 / ar)
		}
	}
}

func elementSizing(el *layoutElement, xAxis bool) SizingAxis {
	if el.isTextElement {
		return SizingAxis{}
	} else if xAxis {
		return el.config.Layout.Sizing.Width
	}
	return el.config.Layout.Sizing.Height
}

func (d *Dimensions) axis(xAxis bool) *floatn {
	if xAxis {
		return &d.Width
	}
	return &d.Height
}

func (c *Context) sizeContainersAlongAxis(xAxis bool, textElementsOut, aspectRatioElementsOut *[]intn) {
	bfsBuffer := c.layoutElementChildrenBuffer[:0]
	resizableContainerBuffer := c.openLayoutElementStack[:0]
	for rootIndex := range c.layoutElementTreeRoots {
		bfsBuffer = bfsBuffer[:0]
		root := &c.layoutElementTreeRoots[rootIndex]
		rootElement := get(c, c.layoutElements, root.layoutElementIndex, &c.def.element)
		addv(c, &bfsBuffer, root.layoutElementIndex)

		// Size floating containers to their parents.
		if rootElement.config.Floating.AttachTo != AttachToNone {
			floatingElementConfig := &rootElement.config.Floating
			parentItem := c.getHashMapItem(floatingElementConfig.ParentID)
			if parentItem != &c.def.hashItem {
				parentLayoutElement := &c.layoutElements[:cap(c.layoutElements)][parentItem.layoutElement]
				switch rootElement.config.Layout.Sizing.Width.Type {
				case SizingGrow:
					rootElement.dimensions.Width = parentLayoutElement.dimensions.Width
				case SizingPercent:
					rootElement.dimensions.Width = parentLayoutElement.dimensions.Width * rootElement.config.Layout.Sizing.Width.Percent()
				}
				switch rootElement.config.Layout.Sizing.Height.Type {
				case SizingGrow:
					rootElement.dimensions.Height = parentLayoutElement.dimensions.Height
				case SizingPercent:
					rootElement.dimensions.Height = parentLayoutElement.dimensions.Height * rootElement.config.Layout.Sizing.Height.Percent()
				}
			}
		}

		if rootElement.config.Layout.Sizing.Width.Type != SizingPercent {
			mm := rootElement.config.Layout.Sizing.Width.MinMax
			rootElement.dimensions.Width = cmin(cmax(rootElement.dimensions.Width, mm.Min), mm.Max)
		}
		if rootElement.config.Layout.Sizing.Height.Type != SizingPercent {
			mm := rootElement.config.Layout.Sizing.Height.MinMax
			rootElement.dimensions.Height = cmin(cmax(rootElement.dimensions.Height, mm.Min), mm.Max)
		}

		for i := 0; i < len(bfsBuffer); i++ {
			parentIndex := bfsBuffer[i]
			parent := get(c, c.layoutElements, parentIndex, &c.def.element)
			parentLayoutConfig := &parent.config.Layout
			growContainerCount := 0
			parentSize := *parent.dimensions.axis(xAxis)
			var parentPadding floatn
			if xAxis {
				parentPadding = floatn(int(parentLayoutConfig.Padding.Left) + int(parentLayoutConfig.Padding.Right))
			} else {
				parentPadding = floatn(int(parentLayoutConfig.Padding.Top) + int(parentLayoutConfig.Padding.Bottom))
			}
			var innerContentSize floatn
			totalPaddingAndChildGaps := parentPadding
			sizingAlongAxis := (xAxis && parentLayoutConfig.LayoutDirection == LeftToRight) || (!xAxis && parentLayoutConfig.LayoutDirection == TopToBottom)
			resizableContainerBuffer = resizableContainerBuffer[:0]
			parentChildGap := floatn(parentLayoutConfig.ChildGap)
			isFirstChild := true

			for childOffset := intn(0); childOffset < intn(parent.childrenLength); childOffset++ {
				childElementIndex := c.child(parent, childOffset)
				childElement := get(c, c.layoutElements, childElementIndex, &c.def.element)
				childSizing := elementSizing(childElement, xAxis)
				childSize := *childElement.dimensions.axis(xAxis)

				if textElementsOut != nil && childElement.isTextElement {
					addv(c, textElementsOut, childElementIndex)
				} else if childElement.childrenLength > 0 {
					addv(c, &bfsBuffer, childElementIndex)
				}

				if !childElement.isTextElement && aspectRatioElementsOut != nil && childElement.config.AspectRatio.AspectRatio != 0 {
					addv(c, aspectRatioElementsOut, childElementIndex)
				}

				if childSizing.Type != SizingPercent && childSizing.Type != SizingFixed &&
					(!childElement.isTextElement || childElement.textConfig.WrapMode == TextWrapWords) {
					addv(c, &resizableContainerBuffer, childElementIndex)
				}

				if sizingAlongAxis {
					if childSizing.Type != SizingPercent {
						innerContentSize += childSize
					}
					if childSizing.Type == SizingGrow {
						growContainerCount++
					}
					if !isFirstChild {
						innerContentSize += parentChildGap // For children after index 0, the childAxisOffset is the gap from the previous child.
						totalPaddingAndChildGaps += parentChildGap
					}
				} else {
					innerContentSize = cmax(childSize, innerContentSize)
				}
				isFirstChild = false
			}

			// Expand percentage containers to size.
			for childOffset := intn(0); childOffset < intn(parent.childrenLength); childOffset++ {
				childElementIndex := c.child(parent, childOffset)
				childElement := get(c, c.layoutElements, childElementIndex, &c.def.element)
				childSizing := elementSizing(childElement, xAxis)
				childSize := childElement.dimensions.axis(xAxis)
				if childSizing.Type == SizingPercent {
					*childSize = (parentSize - totalPaddingAndChildGaps) * childSizing.Percent()
					if sizingAlongAxis {
						innerContentSize += *childSize
					}
					updateAspectRatioBox(childElement)
				}
			}

			if sizingAlongAxis {
				sizeToDistribute := parentSize - parentPadding - innerContentSize
				if sizeToDistribute < 0 {
					// The content is too large, compress the children as much as possible.
					// If the parent clips content in this axis direction, don't compress children, just leave them alone.
					if (xAxis && parent.config.Clip.Horizontal) || (!xAxis && parent.config.Clip.Vertical) {
						continue
					}
					// Scrolling containers preferentially compress before others.
					for sizeToDistribute < -eps && len(resizableContainerBuffer) > 0 {
						var largest, secondLargest floatn
						widthToAdd := sizeToDistribute
						for childIndex := range resizableContainerBuffer {
							child := get(c, c.layoutElements, resizableContainerBuffer[childIndex], &c.def.element)
							childSize := *child.dimensions.axis(xAxis)
							if floatEqual(childSize, largest) {
								continue
							}
							if childSize > largest {
								secondLargest = largest
								largest = childSize
							}
							if childSize < largest {
								secondLargest = cmax(secondLargest, childSize)
								widthToAdd = secondLargest - largest
							}
						}
						widthToAdd = cmax(widthToAdd, sizeToDistribute/floatn(len(resizableContainerBuffer)))

						// Clay loops forever when an iteration makes no progress, which happens with
						// negative child sizes. Breaking out is equivalent whenever clay terminates.
						progress := false
						for childIndex := 0; childIndex < len(resizableContainerBuffer); childIndex++ {
							child := get(c, c.layoutElements, resizableContainerBuffer[childIndex], &c.def.element)
							childSize := child.dimensions.axis(xAxis)
							minSize := *child.minDimensions.axis(xAxis)
							previousWidth := *childSize
							if floatEqual(*childSize, largest) {
								*childSize += widthToAdd
								if *childSize <= minSize {
									*childSize = minSize
									removeSwapback(c, &resizableContainerBuffer, intn(childIndex))
									childIndex--
									progress = true
								}
								sizeToDistribute -= *childSize - previousWidth
								progress = progress || *childSize != previousWidth
							}
						}
						if !progress {
							break
						}
					}
				} else if sizeToDistribute > 0 && growContainerCount > 0 {
					// The content is too small, allow SizingGrow containers to expand.
					for childIndex := 0; childIndex < len(resizableContainerBuffer); childIndex++ {
						child := get(c, c.layoutElements, resizableContainerBuffer[childIndex], &c.def.element)
						if elementSizing(child, xAxis).Type != SizingGrow {
							removeSwapback(c, &resizableContainerBuffer, intn(childIndex))
							childIndex--
						}
					}
					for sizeToDistribute > eps && len(resizableContainerBuffer) > 0 {
						smallest := maxfloat
						secondSmallest := maxfloat
						widthToAdd := sizeToDistribute
						for childIndex := range resizableContainerBuffer {
							child := get(c, c.layoutElements, resizableContainerBuffer[childIndex], &c.def.element)
							childSize := *child.dimensions.axis(xAxis)
							if floatEqual(childSize, smallest) {
								continue
							}
							if childSize < smallest {
								secondSmallest = smallest
								smallest = childSize
							}
							if childSize > smallest {
								secondSmallest = cmin(secondSmallest, childSize)
								widthToAdd = secondSmallest - smallest
							}
						}
						widthToAdd = cmin(widthToAdd, sizeToDistribute/floatn(len(resizableContainerBuffer)))

						progress := false // See compression loop above.
						for childIndex := 0; childIndex < len(resizableContainerBuffer); childIndex++ {
							child := get(c, c.layoutElements, resizableContainerBuffer[childIndex], &c.def.element)
							childSize := child.dimensions.axis(xAxis)
							maxSize := elementSizing(child, xAxis).MinMax.Max
							previousWidth := *childSize
							if floatEqual(*childSize, smallest) {
								*childSize += widthToAdd
								if *childSize >= maxSize {
									*childSize = maxSize
									removeSwapback(c, &resizableContainerBuffer, intn(childIndex))
									childIndex--
									progress = true
								}
								sizeToDistribute -= *childSize - previousWidth
								progress = progress || *childSize != previousWidth
							}
						}
						if !progress {
							break
						}
					}
				}
			} else {
				// Sizing along the non layout axis ("off axis").
				for childOffset := range resizableContainerBuffer {
					childElement := get(c, c.layoutElements, resizableContainerBuffer[childOffset], &c.def.element)
					childSizing := elementSizing(childElement, xAxis)
					minSize := *childElement.minDimensions.axis(xAxis)
					childSize := childElement.dimensions.axis(xAxis)
					maxSize := parentSize - parentPadding
					// If laying out the children of a scroll panel, grow containers expand to the size of the inner content, not the outer container.
					if (xAxis && parent.config.Clip.Horizontal) || (!xAxis && parent.config.Clip.Vertical) {
						maxSize = cmax(maxSize, innerContentSize)
					}
					if childSizing.Type == SizingGrow {
						*childSize = cmin(maxSize, childSizing.MinMax.Max)
					}
					*childSize = cmax(minSize, cmin(*childSize, maxSize))
				}
			}
		}
	}
}

func (c *Context) addRenderCommand(cmd RenderCommand) {
	if len(c.renderCommands) < cap(c.renderCommands)-1 {
		c.renderCommands = append(c.renderCommands, cmd)
	} else if !c.booleanWarnings.MaxRenderCommandsExceeded {
		c.booleanWarnings.MaxRenderCommandsExceeded = true
		c.report(ErrElementsCapacityExceeded)
	}
}

func (c *Context) elementIsOffscreen(bb *BoundingBox) bool {
	if c.disableCulling {
		return false
	}
	return bb.X > c.layoutDimensions.Width ||
		bb.Y > c.layoutDimensions.Height ||
		bb.X+bb.Width < 0 ||
		bb.Y+bb.Height < 0
}

func (c *Context) wrappedLine(td *textElementData, i intn) *WrappedTextLine {
	lines := c.wrappedTextLines[:cap(c.wrappedTextLines)]
	idx := td.wrappedLinesStart + i
	if idx < 0 || int(idx) >= len(lines) {
		// Clay reads past the end of its array here.
		return &c.def.wrapped
	}
	return &lines[idx]
}

func (c *Context) calculateFinalLayout() {
	// Calculate sizing along the X axis.
	textElements := c.openClipElementStack[:0]
	aspectRatioElements := c.reusableElementIndexBuffer[:0]
	c.sizeContainersAlongAxis(true, &textElements, &aspectRatioElements)

	// Wrap text.
	for textElementIndex := range textElements {
		element := get(c, c.layoutElements, textElements[textElementIndex], &c.def.element)
		textElementData := &element.textElementData
		textElementData.wrappedLinesStart = intn(len(c.wrappedTextLines))
		textElementData.wrappedLinesLength = 0
		containerElement := element
		textConfig := &containerElement.textConfig
		measureTextCacheItem := c.measureTextCached(textElementData.text, textConfig)
		var lineWidth floatn
		lineHeight := textElementData.preferredDimensions.Height
		if textConfig.LineHeight > 0 {
			lineHeight = floatn(textConfig.LineHeight)
		}
		var lineLengthChars, lineStartOffset intn
		if !measureTextCacheItem.containsNewlines && textElementData.preferredDimensions.Width <= containerElement.dimensions.Width {
			addv(c, &c.wrappedTextLines, newWrappedLine(textElementData.text, containerElement.dimensions, 0, intn(len(textElementData.text))))
			textElementData.wrappedLinesLength++
			continue
		}
		spaceWidth := c.measureTextRaw(" ", textConfig).Width
		wordIndex := measureTextCacheItem.measuredWordsStartIndex
		text := textElementData.text
		for wordIndex != -1 {
			if len(c.wrappedTextLines) > cap(c.wrappedTextLines)-1 {
				break
			}
			measuredWord := get(c, c.measuredWords, wordIndex, &c.def.word)
			if lineLengthChars == 0 && lineWidth+measuredWord.width > containerElement.dimensions.Width {
				// Only word on the line is too large, just render it anyway.
				addv(c, &c.wrappedTextLines, newWrappedLine(text, Dimensions{Width: measuredWord.width, Height: lineHeight}, measuredWord.startOffset, measuredWord.length))
				textElementData.wrappedLinesLength++
				wordIndex = measuredWord.next
				lineStartOffset = measuredWord.startOffset + measuredWord.length
			} else if measuredWord.length == 0 || lineWidth+measuredWord.width > containerElement.dimensions.Width {
				// measuredWord.length == 0 means a newline character.
				// Wrapped text lines list has overflowed, just render out the line.
				finalCharIsSpace := byteAt(text, cmax(lineStartOffset+lineLengthChars-1, 0)) == ' '
				w, n := lineWidth, lineLengthChars
				if finalCharIsSpace {
					w -= spaceWidth
					n--
				}
				addv(c, &c.wrappedTextLines, newWrappedLine(text, Dimensions{Width: w, Height: lineHeight}, lineStartOffset, n))
				textElementData.wrappedLinesLength++
				if lineLengthChars == 0 || measuredWord.length == 0 {
					wordIndex = measuredWord.next
				}
				lineWidth = 0
				lineLengthChars = 0
				lineStartOffset = measuredWord.startOffset
			} else {
				lineWidth += measuredWord.width + floatn(textConfig.LetterSpacing)
				lineLengthChars += measuredWord.length
				wordIndex = measuredWord.next
			}
		}
		if lineLengthChars > 0 {
			addv(c, &c.wrappedTextLines, newWrappedLine(text, Dimensions{Width: lineWidth - floatn(textConfig.LetterSpacing), Height: lineHeight}, lineStartOffset, lineLengthChars))
			textElementData.wrappedLinesLength++
		}
		containerElement.dimensions.Height = lineHeight * floatn(textElementData.wrappedLinesLength)
	}

	// Scale vertical heights according to aspect ratio.
	for i := range aspectRatioElements {
		aspectElement := get(c, c.layoutElements, aspectRatioElements[i], &c.def.element)
		aspectElement.dimensions.Height = (1 / aspectElement.config.AspectRatio.AspectRatio) * aspectElement.dimensions.Width
		aspectElement.config.Layout.Sizing.Height.MinMax.Max = aspectElement.dimensions.Height
	}

	// Propagate effect of text wrapping, aspect scaling etc. on height of parents.
	dfsBuffer := c.layoutElementTreeNodeArray1[:0]
	for i := range c.layoutElementTreeRoots {
		root := &c.layoutElementTreeRoots[i]
		c.setVisited(len(dfsBuffer), false)
		add(c, &dfsBuffer, layoutElementTreeNode{layoutElement: root.layoutElementIndex}, &c.def.treeNode)
	}
	for len(dfsBuffer) > 0 {
		currentElementTreeNode := &dfsBuffer[len(dfsBuffer)-1]
		currentElement := get(c, c.layoutElements, currentElementTreeNode.layoutElement, &c.def.element)
		if !c.visited(len(dfsBuffer) - 1) {
			c.setVisited(len(dfsBuffer)-1, true)
			// If the element has no children or is the container for a text element, don't bother inspecting it.
			if currentElement.isTextElement || currentElement.childrenLength == 0 {
				dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]
				continue
			}
			// Add the children to the DFS buffer (needs to be pushed in reverse so that stack traversal is in correct layout order).
			for i := intn(0); i < intn(currentElement.childrenLength); i++ {
				c.setVisited(len(dfsBuffer), false)
				add(c, &dfsBuffer, layoutElementTreeNode{layoutElement: c.child(currentElement, i)}, &c.def.treeNode)
			}
			continue
		}
		dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]

		// DFS node has been visited, this is on the way back up to the root.
		layoutConfig := &currentElement.config.Layout
		if layoutConfig.LayoutDirection == LeftToRight {
			// Resize any parent containers that have grown in height along their non layout axis.
			for j := intn(0); j < intn(currentElement.childrenLength); j++ {
				childElement := get(c, c.layoutElements, c.child(currentElement, j), &c.def.element)
				childHeightWithPadding := cmax(childElement.dimensions.Height+floatn(layoutConfig.Padding.Top)+floatn(layoutConfig.Padding.Bottom), currentElement.dimensions.Height)
				currentElement.dimensions.Height = cmin(cmax(childHeightWithPadding, layoutConfig.Sizing.Height.MinMax.Min), layoutConfig.Sizing.Height.MinMax.Max)
			}
		} else if layoutConfig.LayoutDirection == TopToBottom {
			// Resizing along the layout axis.
			contentHeight := floatn(int(layoutConfig.Padding.Top) + int(layoutConfig.Padding.Bottom))
			for j := intn(0); j < intn(currentElement.childrenLength); j++ {
				childElement := get(c, c.layoutElements, c.child(currentElement, j), &c.def.element)
				contentHeight += childElement.dimensions.Height
			}
			contentHeight += floatn(cmax(int(currentElement.childrenLength)-1, 0) * int(layoutConfig.ChildGap))
			currentElement.dimensions.Height = cmin(cmax(contentHeight, layoutConfig.Sizing.Height.MinMax.Min), layoutConfig.Sizing.Height.MinMax.Max)
		}
	}

	// Calculate sizing along the Y axis.
	c.sizeContainersAlongAxis(false, nil, nil)

	// Scale horizontal widths according to aspect ratio.
	for i := range aspectRatioElements {
		aspectElement := get(c, c.layoutElements, aspectRatioElements[i], &c.def.element)
		aspectElement.dimensions.Width = aspectElement.config.AspectRatio.AspectRatio * aspectElement.dimensions.Height
	}

	// Sort tree roots by z-index.
	roots := c.layoutElementTreeRoots
	for sortMax := len(roots) - 1; sortMax > 0; sortMax-- {
		for i := 0; i < sortMax; i++ {
			if roots[i+1].zIndex < roots[i].zIndex {
				roots[i], roots[i+1] = roots[i+1], roots[i]
			}
		}
	}

	// Calculate final positions and generate render commands.
	c.renderCommands = c.renderCommands[:0]
	for rootIndex := range c.layoutElementTreeRoots {
		dfsBuffer = dfsBuffer[:0]
		root := &c.layoutElementTreeRoots[rootIndex]
		rootElement := get(c, c.layoutElements, root.layoutElementIndex, &c.def.element)
		var rootPosition Vector2
		parentHashMapItem := c.getHashMapItem(root.parentID)
		// Position root floating containers.
		if rootElement.config.Floating.AttachTo != AttachToNone {
			config := &rootElement.config.Floating
			rootDimensions := rootElement.dimensions
			parentBoundingBox := parentHashMapItem.boundingBox
			// Set X position.
			var targetAttachPosition Vector2
			switch config.AttachPoints.Parent {
			case AttachPointLeftTop, AttachPointLeftCenter, AttachPointLeftBottom:
				targetAttachPosition.X = parentBoundingBox.X
			case AttachPointCenterTop, AttachPointCenterCenter, AttachPointCenterBottom:
				targetAttachPosition.X = parentBoundingBox.X + (parentBoundingBox.Width / 2)
			case AttachPointRightTop, AttachPointRightCenter, AttachPointRightBottom:
				targetAttachPosition.X = parentBoundingBox.X + parentBoundingBox.Width
			}
			switch config.AttachPoints.Element {
			case AttachPointCenterTop, AttachPointCenterCenter, AttachPointCenterBottom:
				targetAttachPosition.X -= rootDimensions.Width / 2
			case AttachPointRightTop, AttachPointRightCenter, AttachPointRightBottom:
				targetAttachPosition.X -= rootDimensions.Width
			}
			// Set Y position.
			switch config.AttachPoints.Parent {
			case AttachPointLeftTop, AttachPointRightTop, AttachPointCenterTop:
				targetAttachPosition.Y = parentBoundingBox.Y
			case AttachPointLeftCenter, AttachPointCenterCenter, AttachPointRightCenter:
				targetAttachPosition.Y = parentBoundingBox.Y + (parentBoundingBox.Height / 2)
			case AttachPointLeftBottom, AttachPointCenterBottom, AttachPointRightBottom:
				targetAttachPosition.Y = parentBoundingBox.Y + parentBoundingBox.Height
			}
			switch config.AttachPoints.Element {
			case AttachPointLeftCenter, AttachPointCenterCenter, AttachPointRightCenter:
				targetAttachPosition.Y -= rootDimensions.Height / 2
			case AttachPointLeftBottom, AttachPointCenterBottom, AttachPointRightBottom:
				targetAttachPosition.Y -= rootDimensions.Height
			}
			targetAttachPosition.X += config.Offset.X
			targetAttachPosition.Y += config.Offset.Y
			rootPosition = targetAttachPosition
		}
		if root.clipElementID != 0 {
			clipHashMapItem := c.getHashMapItem(root.clipElementID)
			if !c.elementIsOffscreen(&clipHashMapItem.boundingBox) {
				// Floating elements that are attached to scrolling contents won't be correctly positioned if external scroll handling is enabled, fix here.
				if c.externalScrollHandlingEnabled && clipHashMapItem.layoutElement >= 0 {
					clipCfg := c.layoutElements[:cap(c.layoutElements)][clipHashMapItem.layoutElement].config.Clip
					if clipCfg.Horizontal {
						rootPosition.X += clipCfg.ChildOffset.X
					}
					if clipCfg.Vertical {
						rootPosition.Y += clipCfg.ChildOffset.Y
					}
				}
				c.addRenderCommand(RenderCommand{
					BoundingBox: clipHashMapItem.boundingBox,
					ID:          hashNumber(rootElement.id, uintn(rootElement.childrenLength)+10).ID,
					ZIndex:      root.zIndex,
					CommandType: RenderCommandTypeScissorStart,
				})
			}
		}
		add(c, &dfsBuffer, layoutElementTreeNode{
			layoutElement:   root.layoutElementIndex,
			position:        rootPosition,
			nextChildOffset: Vector2{X: floatn(rootElement.config.Layout.Padding.Left), Y: floatn(rootElement.config.Layout.Padding.Top)},
		}, &c.def.treeNode)

		c.setVisited(0, false)
		for len(dfsBuffer) > 0 {
			currentElementTreeNode := &dfsBuffer[len(dfsBuffer)-1]
			currentElement := get(c, c.layoutElements, currentElementTreeNode.layoutElement, &c.def.element)
			layoutConfig := &currentElement.config.Layout
			if currentElement.isTextElement {
				layoutConfig = &defaultLayoutConfig
			}
			var scrollOffset Vector2

			// DFS is returning back upwards.
			if c.visited(len(dfsBuffer) - 1) {
				if currentElement.isTextElement {
					dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]
					continue
				}
				currentElementData := c.getHashMapItem(currentElement.id)
				if !c.elementIsOffscreen(&currentElementData.boundingBox) {
					closeClipElement := false
					if currentElement.config.Clip.Horizontal || currentElement.config.Clip.Vertical {
						closeClipElement = true
						for i := range c.scrollContainerDatas {
							if c.scrollContainerDatas[i].layoutElement == currentElementTreeNode.layoutElement {
								scrollOffset = currentElement.config.Clip.ChildOffset
								if c.externalScrollHandlingEnabled {
									scrollOffset = Vector2{}
								}
								break
							}
						}
					}
					if currentElement.config.Border.Width.hasAnyWidth() {
						currentElementBoundingBox := currentElementData.boundingBox
						borderConfig := &currentElement.config.Border
						c.addRenderCommand(RenderCommand{
							BoundingBox: currentElementBoundingBox,
							RenderData: RenderData{
								Color:        borderConfig.Color,
								CornerRadius: currentElement.config.CornerRadius,
								BorderWidth:  borderConfig.Width,
							},
							UserData:    currentElement.config.UserData,
							ID:          hashNumber(currentElement.id, uintn(currentElement.childrenLength)).ID,
							CommandType: RenderCommandTypeBorder,
						})
						if borderConfig.Width.BetweenChildren > 0 && borderConfig.Color.A > 0 {
							// Clay uses integer division here.
							halfGap := floatn(layoutConfig.ChildGap / 2)
							halfWidth := floatn(borderConfig.Width.BetweenChildren / 2)
							borderOffset := Vector2{X: floatn(layoutConfig.Padding.Left) - halfGap, Y: floatn(layoutConfig.Padding.Top) - halfGap}
							for i := intn(0); i < intn(currentElement.childrenLength); i++ {
								childElement := get(c, c.layoutElements, c.child(currentElement, i), &c.def.element)
								if layoutConfig.LayoutDirection == LeftToRight {
									if i > 0 {
										c.addRenderCommand(RenderCommand{
											BoundingBox: BoundingBox{
												Vector2:    Vector2{X: currentElementBoundingBox.X + borderOffset.X + scrollOffset.X - halfWidth, Y: currentElementBoundingBox.Y + scrollOffset.Y},
												Dimensions: Dimensions{Width: floatn(borderConfig.Width.BetweenChildren), Height: currentElement.dimensions.Height},
											},
											RenderData:  RenderData{Color: borderConfig.Color},
											UserData:    currentElement.config.UserData,
											ID:          hashNumber(currentElement.id, uintn(currentElement.childrenLength)+1+uintn(i)).ID,
											CommandType: RenderCommandTypeRectangle,
										})
									}
									borderOffset.X += childElement.dimensions.Width + floatn(layoutConfig.ChildGap)
								} else {
									if i > 0 {
										c.addRenderCommand(RenderCommand{
											BoundingBox: BoundingBox{
												Vector2:    Vector2{X: currentElementBoundingBox.X + scrollOffset.X, Y: currentElementBoundingBox.Y + borderOffset.Y + scrollOffset.Y - halfWidth},
												Dimensions: Dimensions{Width: currentElement.dimensions.Width, Height: floatn(borderConfig.Width.BetweenChildren)},
											},
											RenderData:  RenderData{Color: borderConfig.Color},
											UserData:    currentElement.config.UserData,
											ID:          hashNumber(currentElement.id, uintn(currentElement.childrenLength)+1+uintn(i)).ID,
											CommandType: RenderCommandTypeRectangle,
										})
									}
									borderOffset.Y += childElement.dimensions.Height + floatn(layoutConfig.ChildGap)
								}
							}
						}
					}
					// This exists because the scissor needs to end _after_ borders between elements.
					if closeClipElement {
						c.addRenderCommand(RenderCommand{
							ID:          hashNumber(currentElement.id, uintn(rootElement.childrenLength)+11).ID,
							CommandType: RenderCommandTypeScissorEnd,
						})
					}
				}
				dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]
				continue
			}

			// This will only be run a single time for each element in downwards DFS order.
			c.setVisited(len(dfsBuffer)-1, true)
			currentElementBoundingBox := BoundingBox{Vector2: currentElementTreeNode.position, Dimensions: currentElement.dimensions}
			var scrollContainerData *scrollContainerDataInternal
			if !currentElement.isTextElement {
				if currentElement.config.Floating.AttachTo != AttachToNone {
					expand := currentElement.config.Floating.Expand
					currentElementBoundingBox.X -= expand.Width
					currentElementBoundingBox.Width += expand.Width * 2
					currentElementBoundingBox.Y -= expand.Height
					currentElementBoundingBox.Height += expand.Height * 2
				}
				// Apply scroll offsets to container.
				if currentElement.config.Clip.Horizontal || currentElement.config.Clip.Vertical {
					for i := range c.scrollContainerDatas {
						mapping := &c.scrollContainerDatas[i]
						if mapping.layoutElement == currentElementTreeNode.layoutElement {
							scrollContainerData = mapping
							mapping.boundingBox = currentElementBoundingBox
							scrollOffset = currentElement.config.Clip.ChildOffset
							if c.externalScrollHandlingEnabled {
								scrollOffset = Vector2{}
							}
							break
						}
					}
				}
			}

			offscreen := c.elementIsOffscreen(&currentElementBoundingBox)

			// Generate render commands for current element.
			if !offscreen {
				if currentElement.isTextElement {
					c.emitTextCommands(currentElement, &currentElementBoundingBox, root.zIndex)
				} else {
					cfg := &currentElement.config
					if cfg.Image.ImageData != nil {
						c.addRenderCommand(RenderCommand{
							BoundingBox: currentElementBoundingBox,
							RenderData:  RenderData{Color: cfg.BackgroundColor, CornerRadius: cfg.CornerRadius, Data: cfg.Image.ImageData},
							UserData:    cfg.UserData,
							ID:          currentElement.id,
							ZIndex:      root.zIndex,
							CommandType: RenderCommandTypeImage,
						})
					}
					if cfg.Custom.CustomData != nil {
						c.addRenderCommand(RenderCommand{
							BoundingBox: currentElementBoundingBox,
							RenderData:  RenderData{Color: cfg.BackgroundColor, CornerRadius: cfg.CornerRadius, Data: cfg.Custom.CustomData},
							UserData:    cfg.UserData,
							ID:          currentElement.id,
							ZIndex:      root.zIndex,
							CommandType: RenderCommandTypeCustom,
						})
					}
					if cfg.Clip.Horizontal || cfg.Clip.Vertical {
						c.addRenderCommand(RenderCommand{
							BoundingBox: currentElementBoundingBox,
							RenderData:  RenderData{ClipHorizontal: cfg.Clip.Horizontal, ClipVertical: cfg.Clip.Vertical},
							UserData:    cfg.UserData,
							ID:          currentElement.id,
							ZIndex:      root.zIndex,
							CommandType: RenderCommandTypeScissorStart,
						})
					}
					if cfg.BackgroundColor.A > 0 {
						c.addRenderCommand(RenderCommand{
							BoundingBox: currentElementBoundingBox,
							RenderData:  RenderData{Color: cfg.BackgroundColor, CornerRadius: cfg.CornerRadius},
							UserData:    cfg.UserData,
							ID:          currentElement.id,
							ZIndex:      root.zIndex,
							CommandType: RenderCommandTypeRectangle,
						})
					}
				}
			}

			c.getHashMapItem(currentElement.id).boundingBox = currentElementBoundingBox

			if currentElement.isTextElement {
				continue
			}

			// Setup positions for child elements and add to DFS buffer.

			// On-axis alignment.
			var contentSizeCurrent Dimensions
			if layoutConfig.LayoutDirection == LeftToRight {
				for i := intn(0); i < intn(currentElement.childrenLength); i++ {
					childElement := get(c, c.layoutElements, c.child(currentElement, i), &c.def.element)
					contentSizeCurrent.Width += childElement.dimensions.Width
					contentSizeCurrent.Height = cmax(contentSizeCurrent.Height, childElement.dimensions.Height)
				}
				contentSizeCurrent.Width += floatn(cmax(int(currentElement.childrenLength)-1, 0) * int(layoutConfig.ChildGap))
				extraSpace := currentElement.dimensions.Width - floatn(int(layoutConfig.Padding.Left)+int(layoutConfig.Padding.Right)) - contentSizeCurrent.Width
				switch layoutConfig.ChildAlignment.X {
				case AlignXLeft:
					extraSpace = 0
				case AlignXCenter:
					extraSpace /= 2
				}
				extraSpace = cmax(0, extraSpace)
				currentElementTreeNode.nextChildOffset.X += extraSpace
			} else if layoutConfig.LayoutDirection == TopToBottom {
				for i := intn(0); i < intn(currentElement.childrenLength); i++ {
					childElement := get(c, c.layoutElements, c.child(currentElement, i), &c.def.element)
					contentSizeCurrent.Width = cmax(contentSizeCurrent.Width, childElement.dimensions.Width)
					contentSizeCurrent.Height += childElement.dimensions.Height
				}
				contentSizeCurrent.Height += floatn(cmax(int(currentElement.childrenLength)-1, 0) * int(layoutConfig.ChildGap))
				extraSpace := currentElement.dimensions.Height - floatn(int(layoutConfig.Padding.Top)+int(layoutConfig.Padding.Bottom)) - contentSizeCurrent.Height
				switch layoutConfig.ChildAlignment.Y {
				case AlignYTop:
					extraSpace = 0
				case AlignYCenter:
					extraSpace /= 2
				}
				extraSpace = cmax(0, extraSpace)
				currentElementTreeNode.nextChildOffset.Y += extraSpace
			}

			if scrollContainerData != nil {
				scrollContainerData.contentSize = Dimensions{
					Width:  contentSizeCurrent.Width + floatn(int(layoutConfig.Padding.Left)+int(layoutConfig.Padding.Right)),
					Height: contentSizeCurrent.Height + floatn(int(layoutConfig.Padding.Top)+int(layoutConfig.Padding.Bottom)),
				}
			}

			// Add children to the DFS buffer.
			nchildren := int(currentElement.childrenLength)
			base := len(dfsBuffer)
			if base+nchildren > cap(dfsBuffer) {
				// Clay writes past the end of its array here.
				c.report(ErrInternal)
				nchildren = cap(dfsBuffer) - base
			}
			dfsBuffer = dfsBuffer[:base+nchildren]
			for i := 0; i < nchildren; i++ {
				childIdx := c.child(currentElement, intn(i))
				childElement := get(c, c.layoutElements, childIdx, &c.def.element)
				// Alignment along non layout axis.
				if layoutConfig.LayoutDirection == LeftToRight {
					currentElementTreeNode.nextChildOffset.Y = floatn(currentElement.config.Layout.Padding.Top)
					whiteSpaceAroundChild := currentElement.dimensions.Height - floatn(int(layoutConfig.Padding.Top)+int(layoutConfig.Padding.Bottom)) - childElement.dimensions.Height
					switch layoutConfig.ChildAlignment.Y {
					case AlignYCenter:
						currentElementTreeNode.nextChildOffset.Y += whiteSpaceAroundChild / 2
					case AlignYBottom:
						currentElementTreeNode.nextChildOffset.Y += whiteSpaceAroundChild
					}
				} else {
					currentElementTreeNode.nextChildOffset.X = floatn(currentElement.config.Layout.Padding.Left)
					whiteSpaceAroundChild := currentElement.dimensions.Width - floatn(int(layoutConfig.Padding.Left)+int(layoutConfig.Padding.Right)) - childElement.dimensions.Width
					switch layoutConfig.ChildAlignment.X {
					case AlignXCenter:
						currentElementTreeNode.nextChildOffset.X += whiteSpaceAroundChild / 2
					case AlignXRight:
						currentElementTreeNode.nextChildOffset.X += whiteSpaceAroundChild
					}
				}
				childPosition := Vector2{
					X: currentElementBoundingBox.X + currentElementTreeNode.nextChildOffset.X + scrollOffset.X,
					Y: currentElementBoundingBox.Y + currentElementTreeNode.nextChildOffset.Y + scrollOffset.Y,
				}
				// DFS buffer elements need to be added in reverse because stack traversal happens backwards.
				newNodeIndex := len(dfsBuffer) - 1 - i
				dfsBuffer[newNodeIndex] = layoutElementTreeNode{
					layoutElement:   childIdx,
					position:        childPosition,
					nextChildOffset: childElement.childOffsetPadding(),
				}
				c.setVisited(newNodeIndex, false)
				// Update parent offsets.
				if layoutConfig.LayoutDirection == LeftToRight {
					currentElementTreeNode.nextChildOffset.X += childElement.dimensions.Width + floatn(layoutConfig.ChildGap)
				} else {
					currentElementTreeNode.nextChildOffset.Y += childElement.dimensions.Height + floatn(layoutConfig.ChildGap)
				}
			}
		}

		if root.clipElementID != 0 {
			clipHashMapItem := c.getHashMapItem(root.clipElementID)
			if !c.elementIsOffscreen(&clipHashMapItem.boundingBox) {
				c.addRenderCommand(RenderCommand{
					ID:          hashNumber(rootElement.id, uintn(rootElement.childrenLength)+11).ID,
					CommandType: RenderCommandTypeScissorEnd,
				})
			}
		}
	}
}

// childOffsetPadding returns the initial child offset of a DFS node, which is the element's
// left and top padding. For text elements clay reads this through a union, which yields the
// text config's font ID and letter spacing. Text elements have no children so it is unused.
func (el *layoutElement) childOffsetPadding() Vector2 {
	if el.isTextElement {
		return Vector2{X: floatn(el.textConfig.FontID), Y: floatn(el.textConfig.LetterSpacing)}
	}
	return Vector2{X: floatn(el.config.Layout.Padding.Left), Y: floatn(el.config.Layout.Padding.Top)}
}

func (c *Context) emitTextCommands(el *layoutElement, bb *BoundingBox, zIndex int16) {
	textElementConfig := &el.textConfig
	naturalLineHeight := el.textElementData.preferredDimensions.Height
	finalLineHeight := naturalLineHeight
	if textElementConfig.LineHeight > 0 {
		finalLineHeight = floatn(textElementConfig.LineHeight)
	}
	lineHeightOffset := (finalLineHeight - naturalLineHeight) / 2
	yPosition := lineHeightOffset
	for lineIndex := intn(0); lineIndex < el.textElementData.wrappedLinesLength; lineIndex++ {
		wrappedLine := c.wrappedLine(&el.textElementData, lineIndex)
		if wrappedLine.length == 0 {
			yPosition += finalLineHeight
			continue
		}
		offset := bb.Width - wrappedLine.Dimensions.Width
		if textElementConfig.TextAlignment == TextAlignLeft {
			offset = 0
		}
		if textElementConfig.TextAlignment == TextAlignCenter {
			offset /= 2
		}
		c.addRenderCommand(RenderCommand{
			BoundingBox: BoundingBox{
				Vector2:    Vector2{X: bb.X + offset, Y: bb.Y + yPosition},
				Dimensions: wrappedLine.Dimensions,
			},
			RenderData: RenderData{
				Text:          wrappedLine.Line,
				TextOffset:    wrappedLine.Offset,
				Color:         textElementConfig.TextColor,
				FontID:        textElementConfig.FontID,
				FontSize:      textElementConfig.FontSize,
				LetterSpacing: textElementConfig.LetterSpacing,
				LineHeight:    textElementConfig.LineHeight,
			},
			UserData:    textElementConfig.UserData,
			ID:          hashNumber(uintn(lineIndex), el.id).ID,
			ZIndex:      zIndex,
			CommandType: RenderCommandTypeText,
		})
		yPosition += finalLineHeight
		if !c.disableCulling && bb.Y+yPosition > c.layoutDimensions.Height {
			break
		}
	}
}

// GetElementData returns the bounding box of the element with the given ID computed by the last layout.
func (c *Context) GetElementData(id ElementID) (BoundingBox, bool) {
	item := c.getHashMapItem(id.ID)
	if item == &c.def.hashItem {
		return BoundingBox{}, false
	}
	return item.boundingBox, true
}

// GetTextLines returns the wrapped lines of the text element with the given ID computed by the last layout.
// The returned slice is valid until the next call to BeginLayout.
func (c *Context) GetTextLines(id ElementID) []WrappedTextLine {
	item := c.getHashMapItem(id.ID)
	if item == &c.def.hashItem || item.layoutElement < 0 || int(item.layoutElement) >= len(c.layoutElements) {
		return nil
	}
	el := &c.layoutElements[item.layoutElement]
	if !el.isTextElement {
		return nil
	}
	td := &el.textElementData
	start, end := int(td.wrappedLinesStart), int(td.wrappedLinesStart+td.wrappedLinesLength)
	return c.wrappedTextLines[min(start, len(c.wrappedTextLines)):min(end, len(c.wrappedTextLines))]
}

// GetScrollContainerData returns data on the scroll container with the given ID.
func (c *Context) GetScrollContainerData(id ElementID) ScrollContainerData {
	for i := range c.scrollContainerDatas {
		scd := &c.scrollContainerDatas[i]
		if scd.elementID == id.ID {
			if scd.layoutElement < 0 { // This can happen on the first frame before a scroll container is declared.
				return ScrollContainerData{}
			}
			return ScrollContainerData{
				ScrollPosition:            &scd.scrollPosition,
				ScrollContainerDimensions: scd.boundingBox.Dimensions,
				ContentDimensions:         scd.contentSize,
				Config:                    c.layoutElements[:cap(c.layoutElements)][scd.layoutElement].config.Clip,
				Found:                     true,
			}
		}
	}
	return ScrollContainerData{}
}

// SetPointerState sets the position and state of the pointer. It finds the elements under the
// pointer, calling their hover functions. Call it after EndLayout.
func (c *Context) SetPointerState(position Vector2, isPointerDown bool) {
	if c.booleanWarnings.MaxElementsExceeded {
		return
	}
	c.pointerInfo.Position = position
	c.pointerOverIDs = c.pointerOverIDs[:0]
	dfsBuffer := c.layoutElementChildrenBuffer[:0]
	for rootIndex := len(c.layoutElementTreeRoots) - 1; rootIndex >= 0; rootIndex-- {
		dfsBuffer = dfsBuffer[:0]
		root := &c.layoutElementTreeRoots[rootIndex]
		addv(c, &dfsBuffer, root.layoutElementIndex)
		c.setVisited(0, false)
		found := false
		for len(dfsBuffer) > 0 {
			if c.visited(len(dfsBuffer) - 1) {
				dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]
				continue
			}
			c.setVisited(len(dfsBuffer)-1, true)
			currentIdx := getValue(c, dfsBuffer, intn(len(dfsBuffer))-1)
			currentElement := get(c, c.layoutElements, currentIdx, &c.def.element)
			mapItem := c.getHashMapItem(currentElement.id)
			clipElementID := getValue(c, c.layoutElementClipElementIDs, currentIdx)
			clipItem := c.getHashMapItem(uintn(clipElementID))
			if mapItem.generation > c.generation {
				elementBox := mapItem.boundingBox
				elementBox.X -= root.pointerOffset.X
				elementBox.Y -= root.pointerOffset.Y
				if pointIsInsideRect(position, elementBox) && (clipElementID == 0 || pointIsInsideRect(position, clipItem.boundingBox) || c.externalScrollHandlingEnabled) {
					if mapItem.onHoverFunction != nil {
						mapItem.onHoverFunction(mapItem.elementID, c.pointerInfo, mapItem.hoverFunctionUserData)
					}
					add(c, &c.pointerOverIDs, mapItem.elementID, &c.def.id)
					found = true
				}
				for i := intn(currentElement.childrenLength) - 1; i >= 0; i-- {
					addv(c, &dfsBuffer, c.child(currentElement, i))
					c.setVisited(len(dfsBuffer)-1, false)
				}
			} else {
				dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]
			}
		}
		rootElement := get(c, c.layoutElements, root.layoutElementIndex, &c.def.element)
		if found && rootElement.config.Floating.AttachTo != AttachToNone && rootElement.config.Floating.PointerCaptureMode == PointerCaptureModeCapture {
			break
		}
	}

	if isPointerDown {
		if c.pointerInfo.State == PointerDataPressedThisFrame {
			c.pointerInfo.State = PointerDataPressed
		} else if c.pointerInfo.State != PointerDataPressed {
			c.pointerInfo.State = PointerDataPressedThisFrame
		}
	} else {
		if c.pointerInfo.State == PointerDataReleasedThisFrame {
			c.pointerInfo.State = PointerDataReleased
		} else if c.pointerInfo.State != PointerDataReleased {
			c.pointerInfo.State = PointerDataReleasedThisFrame
		}
	}
}

// PointerState returns the pointer state set by [Context.SetPointerState].
func (c *Context) PointerState() PointerData { return c.pointerInfo }

// PointerOver reports whether the pointer is over the element with the given ID.
func (c *Context) PointerOver(id ElementID) bool {
	for i := range c.pointerOverIDs {
		if c.pointerOverIDs[i].ID == id.ID {
			return true
		}
	}
	return false
}

// PointerOverIDs returns the IDs of the elements under the pointer. Valid until the next call to SetPointerState.
func (c *Context) PointerOverIDs() []ElementID { return c.pointerOverIDs }

// Hovered reports whether the pointer is over the open element. Call it during element declaration.
func (c *Context) Hovered() bool {
	if c.booleanWarnings.MaxElementsExceeded {
		return false
	}
	return c.PointerOver(ElementID{ID: c.openLayoutElement().id})
}

// OnHover sets a function called by SetPointerState when the pointer is over the open element.
func (c *Context) OnHover(fn func(elementID ElementID, pointerInfo PointerData, userData any), userData any) {
	if c.booleanWarnings.MaxElementsExceeded {
		return
	}
	item := c.getHashMapItem(c.openLayoutElement().id)
	item.onHoverFunction = fn
	item.hoverFunctionUserData = userData
}

// UpdateScrollContainers updates scroll positions from the scroll delta and pointer drag,
// progressing momentum scrolling. deltaTime is in seconds. Scroll containers not declared
// in the last layout are discarded.
func (c *Context) UpdateScrollContainers(enableDragScrolling bool, scrollDelta Vector2, deltaTime floatn) {
	isPointerActive := enableDragScrolling && (c.pointerInfo.State == PointerDataPressed || c.pointerInfo.State == PointerDataPressedThisFrame)
	// Don't apply scroll events to ancestors of the inner element.
	highestPriorityElementIndex := -1
	var highestPriorityScrollData *scrollContainerDataInternal
	for i := 0; i < len(c.scrollContainerDatas); i++ {
		scrollData := &c.scrollContainerDatas[i]
		if !scrollData.openThisFrame {
			removeSwapback(c, &c.scrollContainerDatas, intn(i))
			continue
		}
		scrollData.openThisFrame = false
		if c.getHashMapItem(scrollData.elementID) == &c.def.hashItem {
			// Element isn't rendered this frame but scroll offset has been retained.
			removeSwapback(c, &c.scrollContainerDatas, intn(i))
			continue
		}
		el := &c.layoutElements[:cap(c.layoutElements)][scrollData.layoutElement]

		// Touch / click is released.
		if !isPointerActive && scrollData.pointerScrollActive {
			xDiff := scrollData.scrollPosition.X - scrollData.scrollOrigin.X
			if xDiff < -10 || xDiff > 10 {
				scrollData.scrollMomentum.X = (scrollData.scrollPosition.X - scrollData.scrollOrigin.X) / (scrollData.momentumTime * 25)
			}
			yDiff := scrollData.scrollPosition.Y - scrollData.scrollOrigin.Y
			if yDiff < -10 || yDiff > 10 {
				scrollData.scrollMomentum.Y = (scrollData.scrollPosition.Y - scrollData.scrollOrigin.Y) / (scrollData.momentumTime * 25)
			}
			scrollData.pointerScrollActive = false
			scrollData.pointerOrigin = Vector2{}
			scrollData.scrollOrigin = Vector2{}
			scrollData.momentumTime = 0
		}

		// Apply existing momentum.
		scrollData.scrollPosition.X += scrollData.scrollMomentum.X
		scrollData.scrollMomentum.X *= 0.95
		scrollOccurred := scrollDelta.X != 0 || scrollDelta.Y != 0
		if (scrollData.scrollMomentum.X > -0.1 && scrollData.scrollMomentum.X < 0.1) || scrollOccurred {
			scrollData.scrollMomentum.X = 0
		}
		scrollData.scrollPosition.X = cmin(cmax(scrollData.scrollPosition.X, -cmax(scrollData.contentSize.Width-el.dimensions.Width, 0)), 0)

		scrollData.scrollPosition.Y += scrollData.scrollMomentum.Y
		scrollData.scrollMomentum.Y *= 0.95
		if (scrollData.scrollMomentum.Y > -0.1 && scrollData.scrollMomentum.Y < 0.1) || scrollOccurred {
			scrollData.scrollMomentum.Y = 0
		}
		scrollData.scrollPosition.Y = cmin(cmax(scrollData.scrollPosition.Y, -cmax(scrollData.contentSize.Height-el.dimensions.Height, 0)), 0)

		for j := range c.pointerOverIDs {
			if el.id == c.pointerOverIDs[j].ID {
				highestPriorityElementIndex = j
				highestPriorityScrollData = scrollData
			}
		}
	}

	if highestPriorityElementIndex > -1 && highestPriorityScrollData != nil {
		sd := highestPriorityScrollData
		scrollElement := &c.layoutElements[:cap(c.layoutElements)][sd.layoutElement]
		clipConfig := &scrollElement.config.Clip
		canScrollVertically := clipConfig.Vertical && sd.contentSize.Height > scrollElement.dimensions.Height
		canScrollHorizontally := clipConfig.Horizontal && sd.contentSize.Width > scrollElement.dimensions.Width
		// Handle wheel scroll.
		if canScrollVertically {
			sd.scrollPosition.Y = sd.scrollPosition.Y + scrollDelta.Y*10
		}
		if canScrollHorizontally {
			sd.scrollPosition.X = sd.scrollPosition.X + scrollDelta.X*10
		}
		// Handle click / touch scroll.
		if isPointerActive {
			sd.scrollMomentum = Vector2{}
			if !sd.pointerScrollActive {
				sd.pointerOrigin = c.pointerInfo.Position
				sd.scrollOrigin = sd.scrollPosition
				sd.pointerScrollActive = true
			} else {
				var scrollDeltaX, scrollDeltaY floatn
				if canScrollHorizontally {
					oldXScrollPosition := sd.scrollPosition.X
					sd.scrollPosition.X = sd.scrollOrigin.X + (c.pointerInfo.Position.X - sd.pointerOrigin.X)
					sd.scrollPosition.X = cmax(cmin(sd.scrollPosition.X, 0), -(sd.contentSize.Width - sd.boundingBox.Width))
					scrollDeltaX = sd.scrollPosition.X - oldXScrollPosition
				}
				if canScrollVertically {
					oldYScrollPosition := sd.scrollPosition.Y
					sd.scrollPosition.Y = sd.scrollOrigin.Y + (c.pointerInfo.Position.Y - sd.pointerOrigin.Y)
					sd.scrollPosition.Y = cmax(cmin(sd.scrollPosition.Y, 0), -(sd.contentSize.Height - sd.boundingBox.Height))
					scrollDeltaY = sd.scrollPosition.Y - oldYScrollPosition
				}
				if scrollDeltaX > -0.1 && scrollDeltaX < 0.1 && scrollDeltaY > -0.1 && scrollDeltaY < 0.1 && sd.momentumTime > 0.15 {
					sd.momentumTime = 0
					sd.pointerOrigin = c.pointerInfo.Position
					sd.scrollOrigin = sd.scrollPosition
				} else {
					sd.momentumTime += deltaTime
				}
			}
		}
		// Clamp any changes to scroll position to the maximum size of the contents.
		if canScrollVertically {
			sd.scrollPosition.Y = cmax(cmin(sd.scrollPosition.Y, 0), -(sd.contentSize.Height - scrollElement.dimensions.Height))
		}
		if canScrollHorizontally {
			sd.scrollPosition.X = cmax(cmin(sd.scrollPosition.X, 0), -(sd.contentSize.Width - scrollElement.dimensions.Width))
		}
	}
}

// GetScrollOffset returns the internally stored scroll offset of the open element.
// Generally used to set ClipElementConfig.ChildOffset of scroll containers.
func (c *Context) GetScrollOffset() Vector2 {
	if c.booleanWarnings.MaxElementsExceeded {
		return Vector2{}
	}
	id := c.openLayoutElement().id
	for i := range c.scrollContainerDatas {
		if c.scrollContainerDatas[i].elementID == id {
			return c.scrollContainerDatas[i].scrollPosition
		}
	}
	return Vector2{}
}

// setVisited and visited access treeNodeVisited, which clay indexes without bounds checks.
func (c *Context) setVisited(i int, v bool) {
	if i >= 0 && i < len(c.treeNodeVisited) {
		c.treeNodeVisited[i] = v
	}
}

func (c *Context) visited(i int) bool {
	return i >= 0 && i < len(c.treeNodeVisited) && c.treeNodeVisited[i]
}

func pointIsInsideRect(point Vector2, rect BoundingBox) bool {
	return point.X >= rect.X && point.X <= rect.X+rect.Width && point.Y >= rect.Y && point.Y <= rect.Y+rect.Height
}

// addHashMapItem adds the element at index layoutElement to the ID hash map.
func (c *Context) addHashMapItem(elementID ElementID, layoutElement intn) *hashMapItem {
	if len(c.layoutElementsHashMapInternal) == cap(c.layoutElementsHashMapInternal)-1 {
		if !c.booleanWarnings.HashMapCapacityExceeded {
			c.report(ErrHashMapCapacityExceeded)
			c.booleanWarnings.HashMapCapacityExceeded = true
		}
		return nil
	}
	item := hashMapItem{elementID: elementID, layoutElement: layoutElement, nextIndex: -1, generation: c.generation + 1, appearedThisFrame: true}
	hashBucket := elementID.ID % uintn(len(c.layoutElementsHashMap))
	hashItemPrevious := intn(-1)
	hashItemIndex := c.layoutElementsHashMap[hashBucket]
	for hashItemIndex != -1 { // Just replace collision, not a big deal - leave it up to the end user.
		hashItem := get(c, c.layoutElementsHashMapInternal, hashItemIndex, &c.def.hashItem)
		if hashItem.elementID.ID == elementID.ID { // Collision - resolve based on generation.
			item.nextIndex = hashItem.nextIndex
			if hashItem.generation <= c.generation { // First collision - assume this is the "same" element.
				hashItem.appearedThisFrame = hashItem.generation < c.generation
				hashItem.elementID = elementID // Make sure to copy this across. If the stringID reference has changed, we should update the hash item to use the new one.
				hashItem.generation = c.generation + 1
				hashItem.layoutElement = layoutElement
				hashItem.onHoverFunction = nil
				hashItem.hoverFunctionUserData = nil
			} else { // Multiple collisions this frame - two elements have the same ID.
				c.report(ErrDuplicateID)
			}
			return hashItem
		}
		hashItemPrevious = hashItemIndex
		hashItemIndex = hashItem.nextIndex
	}

	var indexToUse intn
	if len(c.layoutElementsHashMapFreeList) > 0 {
		indexToUse = getValue(c, c.layoutElementsHashMapFreeList, intn(len(c.layoutElementsHashMapFreeList))-1)
		c.layoutElementsHashMapFreeList = c.layoutElementsHashMapFreeList[:len(c.layoutElementsHashMapFreeList)-1]
	} else {
		indexToUse = intn(len(c.layoutElementsHashMapInternal))
	}
	hashItem := set(c, &c.layoutElementsHashMapInternal, indexToUse, item)
	if hashItemPrevious != -1 {
		get(c, c.layoutElementsHashMapInternal, hashItemPrevious, &c.def.hashItem).nextIndex = indexToUse
	} else {
		c.layoutElementsHashMap[hashBucket] = indexToUse
	}
	return hashItem
}

// getHashMapItem returns the hash map item for id, or the context's default item if not found.
func (c *Context) getHashMapItem(id uintn) *hashMapItem {
	hashBucket := id % uintn(len(c.layoutElementsHashMap))
	elementIndex := c.layoutElementsHashMap[hashBucket]
	for elementIndex != -1 {
		hashEntry := get(c, c.layoutElementsHashMapInternal, elementIndex, &c.def.hashItem)
		if hashEntry.elementID.ID == id {
			return hashEntry
		}
		elementIndex = hashEntry.nextIndex
	}
	return &c.def.hashItem
}

// Array helpers mirroring clay's bounds checked array functions: instead of panicking on
// out of range accesses they report an internal error and return a default value.

func get[T any](c *Context, s []T, i intn, def *T) *T {
	if i >= 0 && int(i) < len(s) {
		return &s[i]
	}
	c.report(ErrInternal)
	return def
}

func getValue[T any](c *Context, s []T, i intn) (v T) {
	if i >= 0 && int(i) < len(s) {
		return s[i]
	}
	c.report(ErrInternal)
	return v
}

func add[T any](c *Context, s *[]T, v T, def *T) *T {
	if len(*s) < cap(*s) {
		*s = append(*s, v)
		return &(*s)[len(*s)-1]
	}
	c.report(ErrInternal)
	return def
}

func addv[T any](c *Context, s *[]T, v T) {
	if len(*s) < cap(*s) {
		*s = append(*s, v)
		return
	}
	c.report(ErrInternal)
}

func set[T any](c *Context, s *[]T, i intn, v T) *T {
	if i >= 0 && int(i) < cap(*s) {
		if int(i) >= len(*s) {
			*s = (*s)[:i+1]
		}
		(*s)[i] = v
		return &(*s)[i]
	}
	c.report(ErrInternal)
	return nil
}

func removeSwapback[T any](c *Context, s *[]T, i intn) (removed T) {
	if i >= 0 && int(i) < len(*s) {
		last := len(*s) - 1
		removed = (*s)[i]
		(*s)[i] = (*s)[last]
		*s = (*s)[:last]
		return removed
	}
	c.report(ErrInternal)
	return removed
}

// cmax and cmin implement clay's CLAY__MAX/CLAY__MIN macros, which differ from
// the max/min builtins in their handling of NaN.
func cmax[T int | intn | floatn](x, y T) T {
	if x > y {
		return x
	}
	return y
}

func cmin[T int | intn | floatn](x, y T) T {
	if x < y {
		return x
	}
	return y
}

func floatEqual(left, right floatn) bool {
	subtracted := left - right
	return subtracted < eps && subtracted > -eps
}

// substr returns s[off:off+n] clamped to the bounds of s.
func substr(s string, off, n intn) string {
	start := cmin(cmax(int(off), 0), len(s))
	end := cmin(cmax(int(off+n), start), len(s))
	return s[start:end]
}

func byteAt(s string, i intn) byte {
	if i >= 0 && int(i) < len(s) {
		return s[i]
	}
	return 0
}

func hashNumber(offset, seed uintn) ElementID {
	hash := seed
	hash += offset + 48
	hash += hash << 10
	hash ^= hash >> 6
	hash += hash << 3
	hash ^= hash >> 11
	hash += hash << 15
	return ElementID{ID: hash + 1, Offset: offset, BaseID: seed} // Reserve the hash result of zero as "null id".
}

// schar converts a byte as clay does when hashing a char, which is signed on most platforms.
func schar(b byte) uintn { return uintn(int32(int8(b))) }

func hashString(key string, seed uintn) ElementID {
	hash := seed
	for i := 0; i < len(key); i++ {
		hash += schar(key[i])
		hash += hash << 10
		hash ^= hash >> 6
	}
	hash += hash << 3
	hash ^= hash >> 11
	hash += hash << 15
	return ElementID{ID: hash + 1, BaseID: hash + 1, StringID: key} // Reserve the hash result of zero as "null id".
}

func hashStringWithOffset(key string, offset, seed uintn) ElementID {
	base := seed
	for i := 0; i < len(key); i++ {
		base += schar(key[i])
		base += base << 10
		base ^= base >> 6
	}
	hash := base
	hash += offset
	hash += hash << 10
	hash ^= hash >> 6

	hash += hash << 3
	base += base << 3
	hash ^= hash >> 11
	base ^= base >> 11
	hash += hash << 15
	base += base << 15
	return ElementID{ID: hash + 1, Offset: offset, BaseID: base + 1, StringID: key} // Reserve the hash result of zero as "null id".
}
