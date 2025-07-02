package glay

import (
	"errors"
	"log/slog"
	"math"
)

const (
	eps                     = 0.01
	maxfloat         floatn = math.MaxFloat32
	rootContainerStr        = "Clay__RootContainer"
)

var (
	defaultSharedElementConfig = SharedElementConfig{}
)

type Config struct {
	Layout Dimensions
}

const (
	defaultMaxElementCount              = 8192
	defaultMaxMeasureTextWordCacheCount = 16384
)

func (context *Context) Initialize(cfg Config) error {
	if context.MaxElementCount == 0 {
		context.MaxElementCount = defaultMaxElementCount
	}
	if context.MaxMeasureTextCacheWordCount == 0 {
		context.MaxMeasureTextCacheWordCount = defaultMaxMeasureTextWordCacheCount
	}
	if context.GoHash == nil {
		context.GoHash = make(map[uintn]*LayoutElementHashMapItem)
	} else {
		clear(context.GoHash)
	}
	context.LayoutDimensions = cfg.Layout
	var arena _Arena
	context.initializePersistentMemory(&arena)
	context.initializeEphemeralMemory(&arena)
	// arrmemset(context.LayoutElementHashMap[:cap(context.LayoutElementHashMap)], -1)
	arrmemset(context.measureTextHashMap[:cap(context.measureTextHashMap)], 0)
	context.measureTextHashMap = context.measureTextHashMap[:1] // Reserve the 0 value to mean "no next element"
	return nil
}

func (context *Context) Clay(decl ElementDeclaration, declChildren ...func() error) (err error) {
	err = context.OpenElement()
	if err != nil {
		return err
	}
	err = context.ConfigureOpenElement(decl)
	if err != nil {
		return err
	}
	for _, decl := range declChildren {
		err = decl()
		if err != nil {
			return err
		}
	}
	return context.CloseElement()
}

func (context *Context) initializePersistentMemory(arena *_Arena) {
	maxElemCount := context.MaxElementCount
	maxMeasureTextCacheWordCount := context.MaxMeasureTextCacheWordCount
	alloc(arena, &context.scrollContainerDatas, 10)
	// alloc(arena, &context.layoutElementHashMapInternal, maxElemCount)
	// alloc(arena, &context.LayoutElementHashMap, maxElemCount)
	alloc(arena, &context.measureTextHashMapInternal, maxElemCount)
	alloc(arena, &context.measuredWordsFreeList, maxMeasureTextCacheWordCount)
	alloc(arena, &context.measureTextHashMap, maxElemCount)
	alloc(arena, &context.measuredWords, maxMeasureTextCacheWordCount)
	alloc(arena, &context.PointerOverIDs, maxElemCount)
	alloc(arena, &context.debugElementData, maxElemCount)
	alloc(arena, &context.renderCommands, maxElemCount)
}

func (context *Context) initializeEphemeralMemory(arena *_Arena) {
	maxElementCount := context.MaxElementCount
	alloc(arena, &context.LayoutElementChildrenBuffer, maxElementCount)
	alloc(arena, &context.LayoutElements, maxElementCount)
	// alloc(arena, &context.Warnings, 100)

	alloc(arena, &context.LayoutConfigs, maxElementCount)
	alloc(arena, &context.ElementConfigs, maxElementCount)
	alloc(arena, &context.TextElementConfigs, maxElementCount)
	alloc(arena, &context.ImageElementConfigs, maxElementCount)
	alloc(arena, &context.FloatingElementConfigs, maxElementCount)
	alloc(arena, &context.ScrollElementConfigs, maxElementCount)
	alloc(arena, &context.CustomElementConfigs, maxElementCount)
	alloc(arena, &context.BorderElementConfigs, maxElementCount)
	alloc(arena, &context.SharedElementConfigs, maxElementCount)

	alloc(arena, &context.LayoutElementIDStrings, maxElementCount)
	alloc(arena, &context.WrappedTextLines, maxElementCount)
	alloc(arena, &context.LayoutElementTreeNodes1, maxElementCount)
	alloc(arena, &context.LayoutElementTreeRoots, maxElementCount)
	alloc(arena, &context.TreeNodeVisited, maxElementCount)
	context.TreeNodeVisited = context.TreeNodeVisited[:cap(context.TreeNodeVisited)] // This array is accessed directly.

	alloc(arena, &context.LayoutElementChildren, maxElementCount)
	alloc(arena, &context.OpenLayoutElementStack, maxElementCount)
	alloc(arena, &context.openClipElementStack, maxElementCount)
	alloc(arena, &context.ReusableElementIndexBuffer, maxElementCount)
	alloc(arena, &context.LayoutElementClipElementIDs, maxElementCount)
	alloc(arena, &context.DynamicStringData, maxElementCount)
}

type _Arena struct{}

func alloc[T any](_ *_Arena, dst *[]T, n intn) {
	if cap(*dst) >= int(n) {
		*dst = (*dst)[:0] // Enoguh capacity, reslice.
	}
	*dst = make([]T, n)[:0] // need a new slice.
}

func (context *Context) storeLayoutConfig(layout LayoutConfig) *LayoutConfig {
	return &layout
}

func (context *Context) BeginLayout() error {
	var arena _Arena
	context.initializeEphemeralMemory(&arena)
	context.Generation++
	context.DynamicElementIndex = 0
	// Setup root container that covers entire window.
	rootDimensions := context.LayoutDimensions
	if len(context.LayoutElements) != 0 {
		return errors.New("expect no elements on call to BeginLayout")
	}
	context.OpenElement()
	context.ConfigureOpenElement(ElementDeclaration{
		ID:     ID(rootContainerStr),
		Layout: LayoutConfig{Sizing: Sizing{Width: NewSizingAxis(SizingFixed, rootDimensions.Width), Height: NewSizingAxis(SizingFixed, rootDimensions.Height)}},
	})
	context.OpenLayoutElementStack = arradd(context.OpenLayoutElementStack, 0)
	context.LayoutElementTreeRoots = append(context.LayoutElementTreeRoots, layoutElementTreeRoot{LayoutElementIndex: 0})
	return nil
}

func (context *Context) EndLayout() ([]RenderCommand, error) {
	err := context.CloseElement()
	if err != nil {
		return nil, err
	}
	elementsExceededBeforeDebugView := context.warnMaxElementsExceeded()
	if context.DebugModeEnabled && !elementsExceededBeforeDebugView {
		context.WarningsEnabled = false
		context.renderDebugView()
		context.WarningsEnabled = true
	}
	if context.warnMaxElementsExceeded() {
		if !elementsExceededBeforeDebugView {
			err = errors.New("max elements exceeded after adding debug view")
		} else {
			err = errors.New("max elements exceeded")
		}
	} else {
		err = context.calculateFinalLayout()
	}
	return context.renderCommands, err
}

func (context *Context) renderDebugView() {

}

func (context *Context) warnMaxElementsExceeded() bool {
	return false // TODO
}

// OpenElement creates a LayoutElement and opens it, making it the currently open layout element.
func (context *Context) OpenElement() error {
	if arrfree(context.LayoutElements) == 0 {
		return errors.New("max elements exceeded")
	}
	elemIdx := arrlen(context.LayoutElements)
	context.LayoutElements = arradd(context.LayoutElements, LayoutElement{})
	context.OpenLayoutElementStack = arradd(context.OpenLayoutElementStack, elemIdx)
	if len(context.openClipElementStack) > 0 {
		context.LayoutElementClipElementIDs[elemIdx] = context.openClipElementStack[len(context.openClipElementStack)-1]
	} else {
		arr := context.LayoutElementClipElementIDs[:1]
		arr[0] = 0
	}
	return nil
}

func (context *Context) ConfigureOpenElement(decl ElementDeclaration) error {
	openLayoutElement := context.openLayoutElement()
	openLayoutElement.LayoutConfig = context.storeLayoutConfig(decl.Layout)
	if (decl.Layout.Sizing.Width.Type == SizingPercent && decl.Layout.Sizing.Width.Percent > 1) ||
		(decl.Layout.Sizing.Height.Type == SizingPercent && decl.Layout.Sizing.Height.Percent > 1) {
		return errors.New("invalid sizing percent. Expect in range 0..1, i.e: 20% is 0.2")
	}

	openLayoutElementID := decl.ID
	_ = openLayoutElementID

	// FIXED: Instead of taking a slice from context.ElementConfigs, create a new slice
	// openLayoutElement.ElementConfigs = context.ElementConfigs[len(context.ElementConfigs):]
	openLayoutElement.ElementConfigs = make([]ElementConfig, 0, 4) // Pre-allocate space for a few configs

	var sharedConfig SharedElementConfig
	if decl.BackgroundColor.A > 0 {
		sharedConfig.BackgroundColor = decl.BackgroundColor

	}
	if decl.CornerRadius != (CornerRadius{}) {
		sharedConfig.CornerRadius = decl.CornerRadius
	}
	if decl.UserData != nil {
		sharedConfig.UserData = decl.UserData
	}
	if sharedConfig != (SharedElementConfig{}) {

		context.SharedElementConfigs = arradd(context.SharedElementConfigs, sharedConfig)

		openLayoutElement.attachConfig(&context.SharedElementConfigs[len(context.SharedElementConfigs)-1])

	}

	if decl.Image.ImageData != nil {
		context.ImageElementConfigs = arradd(context.ImageElementConfigs, decl.Image)
		openLayoutElement.attachConfig(&context.ImageElementConfigs[len(context.ImageElementConfigs)-1])
	}
	if decl.Floating.AttachTo != AttachToNone && len(context.OpenLayoutElementStack) >= 2 {
		floatingConfig := decl.Floating
		hierarchicalParent := &context.LayoutElements[context.OpenLayoutElementStack[len(context.OpenLayoutElementStack)-2]]
		var clipElementID uintn
		switch floatingConfig.AttachTo {
		case AttachToParent:
			floatingConfig.ParentID = hierarchicalParent.ID
			if len(context.openClipElementStack) > 0 {
				clipElementID = uintn(context.openClipElementStack[len(context.openClipElementStack)-1])
			}

		case AttachToElementWithID:
			parentItem := context.HashMapItem(floatingConfig.ParentID)
			if parentItem != nil {
				return errors.New("a parent item was declared with a parentID but no element with that ID was found")
			}
			// clipElementID = uintn(context.LayoutElementClipElementIDs[parentItem.LayoutElement])
		case AttachToRoot:
			floatingConfig.ParentID = hashString(rootContainerStr, 0, 0).ID
		}
		if openLayoutElement.ID == 0 {
			openLayoutElementID = hashString("Clay__FloatingContainer", uintn(arrlen(context.LayoutElementTreeRoots)), 0)
		}
		context.LayoutElementTreeRoots = arradd(context.LayoutElementTreeRoots, layoutElementTreeRoot{
			LayoutElementIndex: context.OpenLayoutElementStack[len(context.OpenLayoutElementStack)-1],
			ParentID:           floatingConfig.ParentID,
			ClipElementID:      clipElementID,
			Zindex:             floatingConfig.Zindex,
		})
		context.FloatingElementConfigs = arradd(context.FloatingElementConfigs, floatingConfig)
		openLayoutElement.attachConfig(context.FloatingElementConfigs[len(context.FloatingElementConfigs)-1])
	}
	if openLayoutElementID.ID != 0 {
		context.attachID(openLayoutElementID)
	} else if openLayoutElement.ID == 0 {
		openLayoutElementID = context.generateIDForAnonElement(openLayoutElement)
	}

	if decl.Scroll.Horizontal || decl.Scroll.Vertical {
		context.ScrollElementConfigs = arradd(context.ScrollElementConfigs, decl.Scroll)
		openLayoutElement.attachConfig(arrlast(context.ScrollElementConfigs))
		var scrollOffset *scrollContainerDataInternal
		for i := intn(0); i < arrlen(context.scrollContainerDatas); i++ {
			mapping := &context.scrollContainerDatas[i]
			if openLayoutElement.ID == mapping.ElementID {
				scrollOffset = mapping
				scrollOffset.LayoutElement = openLayoutElement
				scrollOffset.OpenThisFrame = true
				break
			}
		}
		if scrollOffset == nil {
			context.scrollContainerDatas = arradd(context.scrollContainerDatas, scrollContainerDataInternal{
				LayoutElement: openLayoutElement,
				ScrollOrigin:  Vector2{-1, -1},
				ElementID:     openLayoutElement.ID,
				OpenThisFrame: true,
			})
			scrollOffset = arrlast(context.scrollContainerDatas)
		}
		if context.ExternalScrollHandlingEnabled {
			scrollOffset.ScrollPosition = context.queryScrollOffset(scrollOffset.ElementID)
		}
	}
	if decl.Border.Width != (BorderWidth{}) {
		context.BorderElementConfigs = arradd(context.BorderElementConfigs, decl.Border)
		openLayoutElement.attachConfig(arrlast(context.BorderElementConfigs))
	}
	return nil
}

func (context *Context) queryScrollOffset(elementID uintn) Vector2 {
	panic("user scrolling not implemented")
	return Vector2{-1, -1}
}

func (context *Context) generateIDForAnonElement(openLayoutElement *LayoutElement) ElementID {
	parentElement := context.openParentLayoutElement()

	elementID := hashNumber(uintn(len(parentElement.Children())), parentElement.ID)
	openLayoutElement.ID = elementID.ID
	context.AddHashMapItem(elementID, openLayoutElement, 0)
	context.LayoutElementIDStrings = arradd(context.LayoutElementIDStrings, elementID.StringID)
	return elementID
}

func (context *Context) CloseElement() error {
	elementHasScrollHorizontal := false
	elementHasScrollVertical := false

	openLayoutElement := context.openLayoutElement()
	layoutConfig := openLayoutElement.LayoutConfig
	config, _ := openLayoutElement.GetConfig(ElementConfigTypeScroll).(*ScrollElementConfig)
	if config != nil {
		elementHasScrollHorizontal = config.Horizontal
		elementHasScrollVertical = config.Vertical
		context.openClipElementStack, _ = arrpop(context.openClipElementStack)
	}

	// Attach uninitialized children to the current open element.
	// They will be initialized during loop.
	openLayoutElement.ChildrenOrTextContent = context.LayoutElementChildren[len(context.LayoutElementChildren) : len(context.LayoutElementChildren)+len(openLayoutElement.Children())]
	children := openLayoutElement.Children()
	childGap := floatn(max(len(children)-1, 0)) * floatn(layoutConfig.ChildGap)
	if layoutConfig.LayoutDirection == LeftToRight {
		openLayoutElement.Dimensions.Width = floatn(layoutConfig.Padding.Horizontal())
		for i := intn(0); i < arrlen(children); i++ {
			childIndex := context.LayoutElementChildrenBuffer[len(context.LayoutElementChildrenBuffer)-len(children)+int(i)]
			child := &context.LayoutElements[childIndex]
			openLayoutElement.Dimensions.Width += child.Dimensions.Width
			openLayoutElement.Dimensions.Height = max(openLayoutElement.Dimensions.Height, child.Dimensions.Height+floatn(layoutConfig.Padding.Vertical()))
			// Minimum size of child elements doesn't matter to scroll containers as they can shrink and hide their contents
			if !elementHasScrollHorizontal {
				openLayoutElement.MinDimensions.Width += child.MinDimensions.Width
			}
			if !elementHasScrollVertical {
				openLayoutElement.MinDimensions.Height = max(openLayoutElement.MinDimensions.Height, child.MinDimensions.Height+floatn(layoutConfig.Padding.Vertical()))
			}
			context.LayoutElementChildren = arradd(context.LayoutElementChildren, childIndex)
		}
		openLayoutElement.Dimensions.Width += childGap
		openLayoutElement.MinDimensions.Width += childGap
	} else if layoutConfig.LayoutDirection == TopToBottom {

		openLayoutElement.Dimensions.Height = floatn(layoutConfig.Padding.Vertical())
		for i := intn(0); i < arrlen(children); i++ {
			childIndex := context.LayoutElementChildrenBuffer[len(context.LayoutElementChildrenBuffer)-len(children)+int(i)]
			child := &context.LayoutElements[childIndex]
			openLayoutElement.Dimensions.Height += child.Dimensions.Height
			openLayoutElement.Dimensions.Width = max(openLayoutElement.Dimensions.Width, child.Dimensions.Width+floatn(layoutConfig.Padding.Horizontal()))
			// Minimum size of child elements doesn't matter to scroll containers as they can shrink and hide their contents
			if !elementHasScrollVertical {
				openLayoutElement.MinDimensions.Height += child.MinDimensions.Height
			}
			if !elementHasScrollHorizontal {
				openLayoutElement.MinDimensions.Width = max(openLayoutElement.MinDimensions.Width, child.MinDimensions.Width+floatn(layoutConfig.Padding.Horizontal()))
			}
			context.LayoutElementChildren = arradd(context.LayoutElementChildren, childIndex)
		}
		openLayoutElement.Dimensions.Height += childGap
		openLayoutElement.MinDimensions.Height += childGap
	}
	context.LayoutElementChildrenBuffer = context.LayoutElementChildrenBuffer[:len(context.LayoutElementChildrenBuffer)-len(children)]

	// Clamp element min/max width to layout values.
	if layoutConfig.Sizing.Width.Type != SizingPercent {
		if layoutConfig.Sizing.Width.MinMax.Max <= 0 {
			// Set the max size if the user didn't specify, makes calculations easier
			layoutConfig.Sizing.Width.MinMax.Max = maxfloat
		}
		openLayoutElement.Dimensions.Width = layoutConfig.Sizing.ClampWidth(openLayoutElement.Dimensions.Width)
		openLayoutElement.MinDimensions.Width = layoutConfig.Sizing.ClampWidth(openLayoutElement.MinDimensions.Width)
	} else {
		openLayoutElement.Dimensions.Width = 0
	}
	if layoutConfig.Sizing.Height.Type != SizingPercent {
		if layoutConfig.Sizing.Height.MinMax.Max <= 0 {
			layoutConfig.Sizing.Height.MinMax.Max = maxfloat
		}
		openLayoutElement.Dimensions.Height = layoutConfig.Sizing.ClampHeight(openLayoutElement.Dimensions.Height)
		openLayoutElement.MinDimensions.Height = layoutConfig.Sizing.ClampHeight(openLayoutElement.MinDimensions.Height)
	} else {
		openLayoutElement.Dimensions.Height = 0
	}

	openLayoutElement.UpdateAspectRatioBox()

	elementIsFloating := openLayoutElement.GetConfig(ElementConfigTypeFloating) != nil

	// Close the currently open element.
	var closingElementIndex intn
	context.OpenLayoutElementStack, closingElementIndex = arrpop(context.OpenLayoutElementStack)
	openLayoutElement = context.openLayoutElement()
	if !elementIsFloating && len(context.OpenLayoutElementStack) > 1 {
		children := openLayoutElement.Children()
		if children == nil {
			openLayoutElement.ChildrenOrTextContent = append(children, -1) // Extend with bogus data and allocate buffer.
		} else {
			openLayoutElement.ChildrenOrTextContent = arrextend(children, 1) // Normally extend buffer.
		}

		context.LayoutElementChildrenBuffer = arradd(context.LayoutElementChildrenBuffer, closingElementIndex)
	}
	return nil
}
func (context *Context) openLayoutElement() *LayoutElement {
	return &context.LayoutElements[context.OpenLayoutElementStack[len(context.OpenLayoutElementStack)-1]]
}
func (context *Context) openParentLayoutElement() *LayoutElement {
	return &context.LayoutElements[context.OpenLayoutElementStack[len(context.OpenLayoutElementStack)-2]]
}
func (context *Context) calculateFinalLayout() error {
	context.sizeContainersAlongAxis(true)

	// Wrap text.
	for textElementIndex := intn(0); textElementIndex < arrlen(context.TextElementData); textElementIndex++ {
		textElementData := &context.TextElementData[textElementIndex]
		textElementData.WrappedLines = context.WrappedTextLines[len(context.WrappedTextLines):] // Forget to initialize capacity here?
		containerElement := &context.LayoutElements[textElementData.ElementIndex]
		// Guaranteed to be text.
		textConfig := containerElement.GetConfig(ElementConfigTypeText).(*TextElementConfig)
		mtci := context.measureTextCached(textElementData.Text, textConfig)
		if !mtci.containsNewlines && textElementData.PreferredDimensions.Width <= containerElement.Dimensions.Width {
			context.WrappedTextLines = arradd(context.WrappedTextLines, WrappedTextLine{Dimensions: containerElement.Dimensions, Line: textElementData.Text})
			textElementData.WrappedLines = textElementData.WrappedLines[:len(textElementData.WrappedLines)+1]
			continue
		}
		var lineWidth, lineHeight floatn = 0, textElementData.PreferredDimensions.Height
		if textConfig.LineHeight > 0 {
			lineHeight = floatn(textConfig.LineHeight)
		}
		var lineLengthChars, lineStartOffset intn
		spaceWidth := context.measureSpaceWidth(textConfig)
		wordIndex := mtci.measureWordsStartIndex
		for wordIndex != -1 {
			if arrlen(context.WrappedTextLines) > arrcap(context.WrappedTextLines)-1 {
				break
			}
			measuredWord := &context.measuredWords[wordIndex]
			if lineLengthChars == 0 && lineWidth+measuredWord.Width > containerElement.Dimensions.Width {
				// Only word on the line is too large, render it anyway.
				context.WrappedTextLines = arradd(context.WrappedTextLines, WrappedTextLine{Dimensions: Dimensions{measuredWord.Width, lineHeight}, Line: textElementData.Text[measuredWord.StartOffset : measuredWord.StartOffset+measuredWord.Length]})
				textElementData.WrappedLines = arrextend(textElementData.WrappedLines, 1)
				wordIndex = measuredWord.Next
				lineStartOffset = measuredWord.StartOffset + measuredWord.Length
			} else if measuredWord.Length == 0 || lineWidth+measuredWord.Width > containerElement.Dimensions.Width {
				// Wrapped text lines list has overflowed, just render out the line.
				finalCharIsSpace := textElementData.Text[lineStartOffset+lineLengthChars-1] == ' '
				width := lineWidth
				line := textElementData.Text[lineStartOffset : lineStartOffset+lineLengthChars]
				if finalCharIsSpace {
					width -= spaceWidth
					line = line[:len(line)-1]
				}
				context.WrappedTextLines = arradd(context.WrappedTextLines, WrappedTextLine{
					Dimensions: Dimensions{Width: width, Height: lineHeight},
					Line:       line,
				})
				textElementData.WrappedLines = arrextend(textElementData.WrappedLines, 1)
				if lineLengthChars == 0 || measuredWord.Length == 0 {
					wordIndex = measuredWord.Next
				}
				lineWidth = 0
				lineLengthChars = 0
				lineStartOffset = measuredWord.StartOffset
			} else {
				lineWidth += measuredWord.Width
				lineLengthChars += measuredWord.Length
				wordIndex = measuredWord.Next
			}
		}
		if lineLengthChars > 0 {
			context.WrappedTextLines = arradd(context.WrappedTextLines, WrappedTextLine{
				Dimensions: Dimensions{Width: lineWidth, Height: lineHeight},
				Line:       textElementData.Text[lineStartOffset : lineStartOffset+lineLengthChars],
			})
			textElementData.WrappedLines = arrextend(textElementData.WrappedLines, 1)
		}
		containerElement.Dimensions.Height = lineHeight * floatn(arrlen(textElementData.WrappedLines))
	}

	// Scale vertical image heights according to aspect ratio.
	for i := intn(0); i < arrlen(context.ImageElementPointers); i++ {
		imageElement := &context.LayoutElements[context.ImageElementPointers[i]]
		config := imageElement.GetConfig(ElementConfigTypeImage).(*ImageElementConfig)
		imageElement.Dimensions.Height = config.SourceDimensions.Height / max(config.SourceDimensions.Width, 1) * imageElement.Dimensions.Width
	}

	// Propagate effect of text wrapping, image aspect scaling, etc. on height of parents.
	dfsBuffer := context.LayoutElementTreeNodes1[:0]
	for i := intn(0); i < arrlen(context.LayoutElementTreeRoots); i++ {
		root := &context.LayoutElementTreeRoots[i]
		context.TreeNodeVisited[len(dfsBuffer)] = false
		dfsBuffer = arradd(dfsBuffer, layoutElementTreeNode{layoutElement: &context.LayoutElements[root.LayoutElementIndex]})
	}
	for len(dfsBuffer) > 0 {
		currentElementTreeNode := &dfsBuffer[len(dfsBuffer)-1]
		currentElement := currentElementTreeNode.layoutElement
		children := currentElement.Children()
		if !context.TreeNodeVisited[len(dfsBuffer)-1] {
			context.TreeNodeVisited[len(dfsBuffer)-1] = true
			if currentElement.GetConfig(ElementConfigTypeText) != nil || len(children) == 0 {
				// If the element has no children or is the container for a text element, don't bother inspecting it
				dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]
				continue
			}
			for i := intn(0); i < arrlen(children); i++ {
				context.TreeNodeVisited[len(dfsBuffer)] = false
				dfsBuffer = arradd(dfsBuffer, layoutElementTreeNode{layoutElement: &context.LayoutElements[children[i]]})
			}
			continue
		}
		dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]

		// DFS node has been visited, this is on the way back up to the root.
		layoutConfig := currentElement.LayoutConfig
		if layoutConfig.LayoutDirection == LeftToRight {
			// Resize any parent containers that have grown in height along their non layout axis
			for j := intn(0); j < arrlen(children); j++ {
				childElement := &context.LayoutElements[children[j]]
				childHeightWithPadding := max(childElement.Dimensions.Height+floatn(layoutConfig.Padding.Vertical()), currentElement.Dimensions.Height)
				currentElement.Dimensions.Height = layoutConfig.Sizing.ClampHeight(childHeightWithPadding)
			}
		} else if layoutConfig.LayoutDirection == TopToBottom {
			// Resizing along the layout axis.
			contentHeight := floatn(layoutConfig.Padding.Vertical())
			for j := intn(0); j < arrlen(children); j++ {
				childElement := &context.LayoutElements[children[j]]
				contentHeight += childElement.Dimensions.Height
			}
			contentHeight += max(floatn(len(children)-1), 0) * floatn(layoutConfig.ChildGap)
			currentElement.Dimensions.Height = layoutConfig.Sizing.ClampHeight(contentHeight)
		}
	}

	// Calculate sizing along Y axis.
	context.sizeContainersAlongAxis(false)

	// Sort tree roots by z-index.
	sortMax := arrlen(context.LayoutElementTreeRoots) - 1
	for sortMax > 0 {
		for i := intn(0); i < sortMax; i++ {
			current := context.LayoutElementTreeRoots[i]
			next := context.LayoutElementTreeRoots[i+1]
			if next.Zindex < current.Zindex {
				context.LayoutElementTreeRoots[i] = next
				context.LayoutElementTreeRoots[i+1] = current
			}
		}
		sortMax--
	}

	// Calculate final positions and generate render commands.
	context.renderCommands = context.renderCommands[:0]
	dfsBuffer = dfsBuffer[:0]
	for rootIndex := intn(0); rootIndex < arrlen(context.LayoutElementTreeRoots); rootIndex++ {
		dfsBuffer = dfsBuffer[:0]
		root := &context.LayoutElementTreeRoots[rootIndex]
		rootElement := &context.LayoutElements[root.LayoutElementIndex]
		rootPosition := Vector2{}
		parentHashMapItem := context.HashMapItem(root.ParentID)
		// Position root floating containers.
		rootFloatConfig, _ := rootElement.GetConfig(ElementConfigTypeFloating).(*FloatingElementConfig)
		if rootFloatConfig != nil && parentHashMapItem != nil {
			rootDimensions := rootElement.Dimensions
			parentBoundingBox := parentHashMapItem.BoundingBox
			targetAttachPosition := Vector2{}
			if rootFloatConfig.AttachPoints.Parent.AttachLeft() {
				targetAttachPosition.X = parentBoundingBox.X
			} else if rootFloatConfig.AttachPoints.Parent.AttachHorizontalCenter() {
				targetAttachPosition.X = parentBoundingBox.X + parentBoundingBox.Width/2
			} else if rootFloatConfig.AttachPoints.Parent.AttachRight() {
				targetAttachPosition.X = parentBoundingBox.X + parentBoundingBox.Width
			} else {
				panic("invalid attach point")
			}

			if rootFloatConfig.AttachPoints.Element.AttachLeft() {
			} else if rootFloatConfig.AttachPoints.Element.AttachHorizontalCenter() {
				targetAttachPosition.X -= rootDimensions.Width / 2
			} else if rootFloatConfig.AttachPoints.Element.AttachRight() {
				targetAttachPosition.X -= rootDimensions.Width
			} else {
				panic("invalid attach point")
			}

			if rootFloatConfig.AttachPoints.Parent.AttachTop() {
				targetAttachPosition.Y = parentBoundingBox.Y
			} else if rootFloatConfig.AttachPoints.Parent.AttachVerticalCenter() {
				targetAttachPosition.Y = parentBoundingBox.Y + parentBoundingBox.Height/2
			} else if rootFloatConfig.AttachPoints.Parent.AttachBottom() {
				targetAttachPosition.Y = parentBoundingBox.Y + parentBoundingBox.Height
			} else {
				panic("invalid attach point")
			}

			if rootFloatConfig.AttachPoints.Element.AttachTop() {
			} else if rootFloatConfig.AttachPoints.Element.AttachVerticalCenter() {
				targetAttachPosition.Y -= rootDimensions.Height / 2
			} else if rootFloatConfig.AttachPoints.Element.AttachBottom() {
				targetAttachPosition.Y -= rootDimensions.Height
			} else {
				panic("invalid attach point")
			}
			targetAttachPosition.X += rootFloatConfig.Offset.X
			targetAttachPosition.Y += rootFloatConfig.Offset.Y
			rootPosition = targetAttachPosition
		}
		if root.ClipElementID != 0 {
			clipHashMapItem := context.HashMapItem(root.ClipElementID)
			if clipHashMapItem != nil {
				// Floating elements that are attached to scrolling contents won't be correctly positioned if external scroll handling is enabled, fix here
				if context.ExternalScrollHandlingEnabled {
					scrollConfig := clipHashMapItem.LayoutElement.GetConfig(ElementConfigTypeScroll).(*ScrollElementConfig)
					for i := intn(0); i < arrlen(context.scrollContainerDatas); i++ {
						mapping := &context.scrollContainerDatas[i]
						if mapping.LayoutElement == clipHashMapItem.LayoutElement {
							root.PointerOffset = mapping.ScrollPosition
							if scrollConfig.Horizontal {
								rootPosition.X += mapping.ScrollPosition.X
							}
							if scrollConfig.Vertical {
								rootPosition.Y += mapping.ScrollPosition.Y
							}
							break
						}
					}
				}
				context.addRenderCommand(RenderCommand{
					BoundingBox: clipHashMapItem.BoundingBox,
					UserData:    nil,
					ID:          hashNumber(rootElement.ID, uintn(len(rootElement.Children())+10)).ID,
					Zindex:      root.Zindex,
					CommandType: RenderCommandTypeScissorStart,
				})
			}
		}
		dfsBuffer = arradd(dfsBuffer, layoutElementTreeNode{layoutElement: rootElement, position: rootPosition, NextChildOffset: Vector2{X: floatn(rootElement.LayoutConfig.Padding.Left), Y: floatn(rootElement.LayoutConfig.Padding.Top)}})

		context.TreeNodeVisited[0] = false
		for len(dfsBuffer) > 0 {
			currentElementTreeNode := &dfsBuffer[len(dfsBuffer)-1]
			currentElement := currentElementTreeNode.layoutElement
			children := currentElement.Children()
			layoutConfig := currentElement.LayoutConfig
			scrollOffset := Vector2{}

			// This will only be run a single time for each element in downwards DFS order
			if !context.TreeNodeVisited[len(dfsBuffer)-1] {
				context.TreeNodeVisited[len(dfsBuffer)-1] = true

				currentElementBoundingBox := BoundingBox{Vector2: currentElementTreeNode.position, Dimensions: currentElement.Dimensions}
				floatingConfig, _ := currentElement.GetConfig(ElementConfigTypeFloating).(*FloatingElementConfig)
				if floatingConfig != nil {
					expand := floatingConfig.Expand
					currentElementBoundingBox.X -= expand.Width
					currentElementBoundingBox.Width += expand.Width * 2
					currentElementBoundingBox.Y -= expand.Height
					currentElementBoundingBox.Y += expand.Height * 2
				}
				var scrollContainerData *scrollContainerDataInternal
				scrollConfig, _ := currentElement.GetConfig(ElementConfigTypeScroll).(*ScrollElementConfig)
				if scrollConfig != nil {
					for i := intn(0); i < arrlen(context.scrollContainerDatas); i++ {
						mapping := &context.scrollContainerDatas[i]
						if mapping.LayoutElement == currentElement {
							scrollContainerData = mapping
							mapping.BoundingBox = currentElementBoundingBox
							if scrollConfig.Horizontal {
								scrollOffset.X = mapping.ScrollPosition.X
							}
							if scrollConfig.Vertical {
								scrollOffset.Y = mapping.ScrollPosition.Y
							}
							if context.ExternalScrollHandlingEnabled {
								scrollOffset = Vector2{}
							}
							break
						}
					}
				}

				hashMapItem := context.HashMapItem(currentElement.ID)
				if hashMapItem != nil {
					hashMapItem.BoundingBox = currentElementBoundingBox
					if hashMapItem.IDAlias != 0 { // For non-rootcontainer element.
						hashMapItemAlias := context.HashMapItem(hashMapItem.IDAlias)
						if hashMapItemAlias != nil {
							hashMapItemAlias.BoundingBox = currentElementBoundingBox
						}
					}
				}

				var sortedConfigIndexes [20]intn
				for elementConfigIndex := intn(0); elementConfigIndex < arrlen(currentElement.ElementConfigs); elementConfigIndex++ {
					sortedConfigIndexes[elementConfigIndex] = elementConfigIndex
				}
				sortMax := arrlen(currentElement.ElementConfigs) - 1
				for sortMax > 0 {
					for i := intn(0); i < sortMax; i++ {
						current := sortedConfigIndexes[i]
						next := sortedConfigIndexes[i+1]
						currentType := currentElement.ElementConfigs[current].Type
						nextType := currentElement.ElementConfigs[next].Type
						if nextType == ElementConfigTypeScroll || currentType == ElementConfigTypeBorder {
							sortedConfigIndexes[i] = next
							sortedConfigIndexes[i+1] = current
						}
					}
					sortMax--
				}
				emitRectangle := false
				// Create the render commands for this element.
				sharedConfig, hasSharedConfig := currentElement.GetSharedConfig()
				if hasSharedConfig && sharedConfig.BackgroundColor.A > 0 {
					emitRectangle = true
				} else if !hasSharedConfig {
					emitRectangle = false
				}
				for elementConfigIndex := intn(0); elementConfigIndex < arrlen(currentElement.ElementConfigs); elementConfigIndex++ {
					elementConfig := &currentElement.ElementConfigs[sortedConfigIndexes[elementConfigIndex]]
					renderCommand := RenderCommand{
						BoundingBox: currentElementBoundingBox,
						UserData:    sharedConfig.UserData,
						ID:          currentElement.ID,
					}
					offscreen := context.IsOffscreen(&currentElementBoundingBox)
					shouldRender := !offscreen
					switch elementConfig.Type {
					case ElementConfigTypeFloating, ElementConfigTypeShared, ElementConfigTypeBorder:
						shouldRender = false
					case ElementConfigTypeScroll:
						renderCommand.CommandType = RenderCommandTypeScissorStart
						renderCommand.RenderData = &ScrollRenderData{
							Horizontal: elementConfig.Config.(*ScrollElementConfig).Horizontal,
							Vertical:   elementConfig.Config.(*ScrollElementConfig).Vertical,
						}
					case ElementConfigTypeImage:
						renderCommand.CommandType = RenderCommandTypeImage
						renderCommand.RenderData = &ImageRenderData{
							BackgroundColor:  sharedConfig.BackgroundColor,
							CornerRadius:     sharedConfig.CornerRadius,
							SourceDimensions: elementConfig.Config.(*ImageElementConfig).SourceDimensions,
							ImageData:        elementConfig.Config.(*ImageElementConfig).ImageData,
						}
					case ElementConfigTypeText:
						if !shouldRender {
							break
						}
						shouldRender = false
						// TODO bunch of stuff here.
					case ElementConfigTypeCustom:
						renderCommand.CommandType = RenderCommandTypeCustom
						renderCommand.RenderData = CustomRenderData{
							BackgroundColor: sharedConfig.BackgroundColor,
							CornerRadius:    sharedConfig.CornerRadius,
							CustomData:      elementConfig.Config,
							// CustomData: elementConfig.Config, // TODO.
						}
					default:
						println("unknown command?")
					}
					if shouldRender {
						context.addRenderCommand(renderCommand)
					}
					if offscreen {
						// NOTE: You may be tempted to try an early return / continue if an element is off screen. Why bother calculating layout for its children, right?
						// Unfortunately, a FLOATING_CONTAINER may be defined that attaches to a child or grandchild of this element, which is large enough to still
						// be on screen, even if this element isn't. That depends on this element and it's children being laid out correctly (even if they are entirely off screen)
					}
				}
				if emitRectangle {
					context.addRenderCommand(RenderCommand{
						BoundingBox: currentElementBoundingBox,
						RenderData: RectangleRenderData{
							BackgroundColor: sharedConfig.BackgroundColor,
							CornerRadius:    sharedConfig.CornerRadius,
						},
						// TODO: Render Data
						UserData:    sharedConfig.UserData,
						ID:          currentElement.ID,
						Zindex:      root.Zindex,
						CommandType: RenderCommandTypeRectangle,
					})
				}

				// Setup initial on-axis alignment.
				textconfig, _ := currentElementTreeNode.layoutElement.GetConfig(ElementConfigTypeText).(*TextElementConfig)
				if textconfig == nil {
					var contentSize Dimensions
					if layoutConfig.LayoutDirection == LeftToRight {
						for i := intn(0); i < arrlen(children); i++ {
							childElement := &context.LayoutElements[children[i]]
							contentSize.Width += childElement.Dimensions.Width
							contentSize.Height = max(contentSize.Height, childElement.Dimensions.Height)
						}
						contentSize.Width += max(floatn(len(children)-1), 0) * floatn(layoutConfig.ChildGap)
						extraSpace := currentElement.Dimensions.Width - floatn(layoutConfig.Padding.Horizontal()) - contentSize.Width
						switch layoutConfig.ChildAlignment.X {
						case AlignXLeft:
							extraSpace = 0
						case AlignXCenter:
							extraSpace /= 2
						}
						currentElementTreeNode.NextChildOffset.X += extraSpace
					} else {
						for i := intn(0); i < arrlen(children); i++ {
							childElement := context.LayoutElements[children[i]]
							contentSize.Width = max(contentSize.Width, childElement.Dimensions.Width)
							contentSize.Height += childElement.Dimensions.Height
						}
						contentSize.Height += max(floatn(len(children)-1), 0) * floatn(layoutConfig.ChildGap)
						extraSpace := currentElement.Dimensions.Height - floatn(layoutConfig.Padding.Vertical()) - contentSize.Height
						switch layoutConfig.ChildAlignment.Y {
						case AlignYTop:
							extraSpace = 0
						case AlignYCenter:
							extraSpace /= 2
						}
						currentElementTreeNode.NextChildOffset.Y += extraSpace
					}
					if scrollContainerData != nil {
						scrollContainerData.ContentSize = Dimensions{
							Width:  contentSize.Width + floatn(layoutConfig.Padding.Horizontal()),
							Height: contentSize.Height + floatn(layoutConfig.Padding.Vertical()),
						}
					}
				}
			} else {
				// DFS is returning upwards backwards.
				closeScrollElement := false
				scrollConfig, _ := currentElement.GetConfig(ElementConfigTypeScroll).(*ScrollElementConfig)
				if scrollConfig != nil {
					closeScrollElement = true
					for i := intn(0); i < arrlen(context.scrollContainerDatas); i++ {
						mapping := &context.scrollContainerDatas[i]
						if mapping.LayoutElement == currentElement {
							if scrollConfig.Horizontal {
								scrollOffset.X = mapping.ScrollPosition.X
							}
							if scrollConfig.Vertical {
								scrollOffset.Y = mapping.ScrollPosition.Y
							}
							if context.ExternalScrollHandlingEnabled {
								scrollOffset = Vector2{}
							}
							break
						}
					}
				}

				borderConfig, _ := currentElement.GetConfig(ElementConfigTypeBorder).(*BorderElementConfig)
				if borderConfig != nil {
					currentElementData := context.HashMapItem(currentElement.ID)
					currentElementBoundingBox := currentElementData.BoundingBox
					if !context.IsOffscreen(&currentElementBoundingBox) {
						// Culling - Don't bother to generate render commands for rectangles entirely outside the screen - this won't stop their children from being rendered if they overflow.
						sharedConfig, _ := currentElement.GetSharedConfig()
						renderCommand := RenderCommand{
							BoundingBox: currentElementBoundingBox,
							// RenderData: TODO,
							UserData:    sharedConfig.UserData,
							ID:          hashNumber(currentElement.ID, uintn(len(children))).ID,
							CommandType: RenderCommandTypeBorder,
						}
						context.addRenderCommand(renderCommand)
						if borderConfig.Width.BetweenChildren > 0 && borderConfig.Color.A > 0 {
							halfGap := floatn(layoutConfig.ChildGap) / 2
							borderOffset := Vector2{X: floatn(layoutConfig.Padding.Left) - halfGap, Y: floatn(layoutConfig.Padding.Top) - halfGap}
							if layoutConfig.LayoutDirection == LeftToRight {
								for i := intn(0); i < arrlen(children); i++ {
									childElement := &context.LayoutElements[children[i]]
									if i > 0 {
										context.addRenderCommand(RenderCommand{
											// BoundingBox: ,
											// RenderData: ,
											UserData:    sharedConfig.UserData,
											ID:          hashNumber(currentElement.ID, uintn(arrlen(children)+i+1)).ID,
											CommandType: RenderCommandTypeRectangle,
										})
									}
									borderOffset.X += childElement.Dimensions.Width + floatn(layoutConfig.ChildGap)
								}
							} else {
								for i := intn(0); i < arrlen(children); i++ {
									childElement := &context.LayoutElements[children[i]]
									if i > 0 {
										context.addRenderCommand(RenderCommand{
											// BoundingBox: ,
											// RenderData: ,
											UserData:    sharedConfig.UserData,
											ID:          hashNumber(currentElement.ID, uintn(arrlen(children)+i+1)).ID,
											CommandType: RenderCommandTypeRectangle,
										})
									}
									borderOffset.Y += childElement.Dimensions.Height + floatn(layoutConfig.ChildGap)
								}
							}
						}
					}
				}
				// This exists because the scissor needs to end _after_ borders between elements
				if closeScrollElement {
					context.addRenderCommand(RenderCommand{
						ID:          hashNumber(currentElement.ID, uintn(arrlen(children)+11)).ID,
						CommandType: RenderCommandTypeScissorEnd,
					})
				}

				dfsBuffer = dfsBuffer[:len(dfsBuffer)-1]
				continue
			}

			// Add children to the DFS buffer.
			textConfig := currentElement.GetConfig(ElementConfigTypeText)
			if textConfig == nil {
				dfsBuffer = dfsBuffer[:len(dfsBuffer)+len(children)]
				for i := intn(0); i < arrlen(children); i++ {
					childElement := &context.LayoutElements[children[i]]
					// Alignment along non-layout axis.
					if layoutConfig.LayoutDirection == LeftToRight {
						currentElementTreeNode.NextChildOffset.Y = floatn(currentElement.LayoutConfig.Padding.Top)
						whiteSpaceAroundChild := currentElement.Dimensions.Height - floatn(layoutConfig.Padding.Vertical()) - childElement.Dimensions.Height
						switch layoutConfig.ChildAlignment.Y {
						case AlignYTop:
						case AlignYCenter:
							currentElementTreeNode.NextChildOffset.Y += whiteSpaceAroundChild / 2
						case AlignYBottom:
							currentElementTreeNode.NextChildOffset.Y += whiteSpaceAroundChild
						default:
							panic("invalid Y alignment")
						}
					} else {
						currentElementTreeNode.NextChildOffset.X = floatn(currentElement.LayoutConfig.Padding.Left)
						whiteSpaceAroundChild := currentElement.Dimensions.Width - floatn(layoutConfig.Padding.Horizontal()) - childElement.Dimensions.Width
						switch layoutConfig.ChildAlignment.X {
						case AlignXLeft:
						case AlignXCenter:
							currentElementTreeNode.NextChildOffset.X += whiteSpaceAroundChild / 2
						case AlignXRight:
							currentElementTreeNode.NextChildOffset.X += whiteSpaceAroundChild
						default:
							panic("invalid X alignment")
						}
					}
					childPosition := Vector2{
						X: currentElementTreeNode.position.X + currentElementTreeNode.NextChildOffset.X + scrollOffset.X,
						Y: currentElementTreeNode.position.Y + currentElementTreeNode.NextChildOffset.Y + scrollOffset.Y,
					}

					// DFS buffer elements need to be added in reverse because stack traversal happens backwards
					newNodeIndex := uintn(arrlen(dfsBuffer) - 1 - i)
					dfsBuffer[newNodeIndex] = layoutElementTreeNode{
						layoutElement:   childElement,
						position:        childPosition,
						NextChildOffset: Vector2{X: floatn(childElement.LayoutConfig.Padding.Left), Y: floatn(childElement.LayoutConfig.Padding.Top)},
					}
					context.TreeNodeVisited[newNodeIndex] = false

					// Update parent offsets.
					if layoutConfig.LayoutDirection == LeftToRight {
						currentElementTreeNode.NextChildOffset.X += childElement.Dimensions.Width + floatn(layoutConfig.ChildGap)
					} else {
						currentElementTreeNode.NextChildOffset.Y += childElement.Dimensions.Height + floatn(layoutConfig.ChildGap)
					}
				}
			}
		}

		if root.ClipElementID != 0 {
			rootChildren := rootElement.Children()
			context.addRenderCommand(RenderCommand{
				ID:          hashNumber(rootElement.ID, uintn(len(rootChildren)+11)).ID,
				CommandType: RenderCommandTypeScissorEnd,
			})
		}
	}
	return nil
}

func (context *Context) addRenderCommand(cmd RenderCommand) error {
	if arrfree(context.renderCommands) > 0 {
		context.renderCommands = arradd(context.renderCommands, cmd)
	} else {
		panic("clay ran out of capacity for commands; try setMaxElementCount with higher value")
	}
	return nil
}

func (context *Context) sizeContainersAlongAxis(xaxis bool) error {
	bfs := context.LayoutElementChildrenBuffer
	resizableContainerBuffer := context.OpenLayoutElementStack
	for rootIdx := intn(0); rootIdx < arrlen(context.LayoutElementTreeRoots); rootIdx++ {
		bfs = bfs[:0]
		root := &context.LayoutElementTreeRoots[rootIdx]
		rootElement := &context.LayoutElements[root.LayoutElementIndex]
		bfs = arradd(bfs, root.LayoutElementIndex)

		if floatingCfg, _ := rootElement.GetConfig(ElementConfigTypeFloating).(*FloatingElementConfig); floatingCfg != nil {
			// Size floating containers to their parents.
			parentItem := context.HashMapItem(floatingCfg.ParentID)
			if parentItem != nil && !parentItem.isdefault() {
				parentLayoutElement := parentItem.LayoutElement
				if rootElement.LayoutConfig.Sizing.Width.Type == SizingGrow {
					rootElement.Dimensions.Width = parentLayoutElement.Dimensions.Width
				}
				if rootElement.LayoutConfig.Sizing.Height.Type == SizingGrow {
					rootElement.Dimensions.Height = parentLayoutElement.Dimensions.Height
				}
			}
		}
		rootElement.Dimensions = rootElement.LayoutConfig.Sizing.Clamp(rootElement.Dimensions)

		for i := intn(0); i < arrlen(bfs); i++ {
			parentIndex := bfs[i]
			parent := &context.LayoutElements[parentIndex]
			parentStyleConfig := parent.LayoutConfig
			var growContainerCount intn
			parentSize := parent.Dimensions.SizeAxis(xaxis)
			parentPadding := parent.LayoutConfig.Padding.SizeAxis(xaxis)
			var innerContentSize, totalPaddingAndChildGaps floatn = 0, parentPadding
			sizingAlongAxis := (xaxis && parentStyleConfig.LayoutDirection == LeftToRight) ||
				(!xaxis && parentStyleConfig.LayoutDirection == TopToBottom)
			resizableContainerBuffer := resizableContainerBuffer[:0]
			parentChildGap := floatn(parentStyleConfig.ChildGap)
			children := parent.Children()
			for childOffset := intn(0); childOffset < arrlen(children); childOffset++ {
				childElementIndex := children[childOffset]
				childElement := &context.LayoutElements[childElementIndex]
				childSizing := childElement.LayoutConfig.Sizing.SizingAxis(xaxis)
				childSize := childElement.Dimensions.SizeAxis(xaxis)
				textcfg, hasTxt := childElement.GetConfig(ElementConfigTypeText).(*TextElementConfig)
				if !hasTxt && len(childElement.Children()) > 0 {
					// Child is not text element with 1+ children.
					bfs = arradd(bfs, childElementIndex)
				}
				if childSizing.Type != SizingPercent &&
					childSizing.Type != SizingFixed &&
					(textcfg == nil || textcfg.WrapMode == TextWrapWords) &&
					(xaxis || childElement.GetConfig(ElementConfigTypeImage) == nil) {
					resizableContainerBuffer = arradd(resizableContainerBuffer, childElementIndex)
				}

				if sizingAlongAxis {
					if childSizing.Type != SizingPercent {
						innerContentSize += childSize
					}
					if childSizing.Type == SizingGrow {
						growContainerCount++
					}
					if childOffset > 0 {
						// For children after index 0, the childAxisOffset is the gap from the previous child
						innerContentSize += parentChildGap
						totalPaddingAndChildGaps += parentChildGap
					}
				} else {
					innerContentSize = max(childSize, innerContentSize)
				}
			}

			// Expand percentage containers to size.
			for childOffset := intn(0); childOffset < arrlen(children); childOffset++ {
				childElementIndex := children[childOffset]
				childElement := &context.LayoutElements[childElementIndex]
				childSizing := childElement.LayoutConfig.Sizing.SizingAxis(xaxis)
				childSize := childElement.Dimensions.SizeAxisPtr(xaxis)
				if childSizing.Type == SizingPercent {
					*childSize = (parentSize - totalPaddingAndChildGaps) * childSizing.Percent
					if sizingAlongAxis {
						innerContentSize += *childSize
					}
					childElement.UpdateAspectRatioBox()
				}
			}

			if sizingAlongAxis {
				sizeToDistribute := parentSize - parentPadding - innerContentSize
				// The content is too large, compress children as much as possible.
				if sizeToDistribute < 0 {
					scrollCfg, hasScrollConfig := parent.GetConfig(ElementConfigTypeScroll).(*ScrollElementConfig)
					if hasScrollConfig {
						if (xaxis && scrollCfg.Horizontal) || (!xaxis && scrollCfg.Vertical) {
							// If the parent can scroll in the axis direction in this direction, don't compress children, just leave them alone.
							continue
						}
					}

					// Scrolling containers preferentially compress before others.
					for sizeToDistribute < -eps && arrlen(resizableContainerBuffer) > 0 {
						var largest, secondLargest, widthToAdd floatn = 0, 0, sizeToDistribute
						for childIndex := intn(0); childIndex < arrlen(resizableContainerBuffer); childIndex++ {
							child := &context.LayoutElements[childIndex]
							childSize := child.Dimensions.SizeAxis(xaxis)
							if floatequal(childSize, largest) {
								continue
							}
							if childSize > largest {
								secondLargest = largest
								largest = childSize
							}
							if childSize < largest {
								secondLargest = max(secondLargest, childSize)
								widthToAdd = secondLargest - largest
							}
						}
						widthToAdd = max(widthToAdd, sizeToDistribute/floatn(len(resizableContainerBuffer)))

						for childIndex := intn(0); childIndex < arrlen(resizableContainerBuffer); childIndex++ {
							child := &context.LayoutElements[resizableContainerBuffer[childIndex]]
							childSize := child.Dimensions.SizeAxisPtr(xaxis)
							minSize := child.MinDimensions.SizeAxis(xaxis)
							previousWidth := *childSize
							if floatequal(*childSize, largest) {
								*childSize += widthToAdd
								if *childSize <= minSize {
									*childSize = minSize
									resizableContainerBuffer = arrremoveswapback(resizableContainerBuffer, childIndex)
									childIndex--
								}
								sizeToDistribute -= *childSize - previousWidth
							}
						}
					}
				} else if sizeToDistribute > 0 && growContainerCount > 0 {
					// Content is too small, allow SizingGrow containers to expand.
					for childIndex := intn(0); childIndex < arrlen(resizableContainerBuffer); childIndex++ {
						child := &context.LayoutElements[resizableContainerBuffer[childIndex]]
						childSizing := child.LayoutConfig.Sizing.SizingAxis(xaxis).Type
						if childSizing != SizingGrow {
							resizableContainerBuffer = arrremoveswapback(resizableContainerBuffer, childIndex)
							childIndex--
						}
					}
					for sizeToDistribute > eps && len(resizableContainerBuffer) > 0 {
						smallest := maxfloat
						secondSmallest := maxfloat
						widthToAdd := sizeToDistribute
						for childIndex := intn(0); childIndex < arrlen(resizableContainerBuffer); childIndex++ {
							child := &context.LayoutElements[resizableContainerBuffer[childIndex]]
							childSize := child.Dimensions.SizeAxis(xaxis)
							if floatequal(childSize, smallest) {
								continue
							} else if childSize < smallest {
								secondSmallest = smallest
								smallest = childSize
							}
							if childSize > smallest {
								secondSmallest = min(secondSmallest, childSize)
								widthToAdd = secondSmallest - smallest
							}
						}
						widthToAdd = min(widthToAdd, sizeToDistribute/floatn(len(resizableContainerBuffer)))
						for childIndex := intn(0); childIndex < arrlen(resizableContainerBuffer); childIndex++ {
							child := &context.LayoutElements[resizableContainerBuffer[childIndex]]
							childSize := child.Dimensions.SizeAxisPtr(xaxis)
							maxSize := child.LayoutConfig.Sizing.SizingAxis(xaxis).MinMax.Max
							previousWidth := *childSize
							if floatequal(*childSize, smallest) {
								*childSize += widthToAdd
								if *childSize >= maxSize {
									*childSize = maxSize
									resizableContainerBuffer = arrremoveswapback(resizableContainerBuffer, childIndex)
									childIndex--
								}
								sizeToDistribute -= *childSize - previousWidth
							}
						}
					}
				}

			} else {
				// Sizing along the non-layout axis ("off axis")
				for childOffset := intn(0); childOffset < arrlen(resizableContainerBuffer); childOffset++ {
					childElement := &context.LayoutElements[resizableContainerBuffer[childOffset]]
					childSizing := childElement.LayoutConfig.Sizing.SizingAxis(xaxis)
					childSize := childElement.Dimensions.SizeAxisPtr(xaxis)
					if !xaxis && childElement.GetConfig(ElementConfigTypeImage) != nil {
						continue // Currently we don't support resizing aspect ratio images on the Y axis because it would break the ratio
					}
					// If laying out the children of a scroll panel, grow containers to exapnd to the height of the inner content, not outer content.
					maxSize := parentSize - parentPadding
					scrollCfg, ok := parent.GetConfig(ElementConfigTypeScroll).(*ScrollElementConfig)
					if ok {
						if (xaxis && scrollCfg.Horizontal) || (!xaxis && scrollCfg.Vertical) {
							maxSize = max(maxSize, innerContentSize)
						}
					}
					if childSizing.Type == SizingFit {
						*childSize = max(childSizing.MinMax.Min, min(*childSize, maxSize))
					} else if childSizing.Type == SizingGrow {
						*childSize = min(maxSize, childSizing.MinMax.Max)
					}
				}
			}
		}
	}
	return nil
}

func (context *Context) IsOffscreen(boundingBox *BoundingBox) bool {
	if context.DisableCulling {
		return false
	}
	return boundingBox.X > context.LayoutDimensions.Width ||
		boundingBox.Y > context.LayoutDimensions.Height ||
		boundingBox.X+boundingBox.Width < 0 ||
		boundingBox.Y+boundingBox.Height < 0
}

func (le *LayoutElement) UpdateAspectRatioBox() {
	imageConfig, ok := le.GetConfig(ElementConfigTypeImage).(*ImageElementConfig)
	if !ok {
		return
	}
	if imageConfig.SourceDimensions.Width == 0 || imageConfig.SourceDimensions.Height == 0 {
		return
	}
	aspect := imageConfig.SourceDimensions.Aspect()
	if le.Dimensions.Width == 0 && le.Dimensions.Height != 0 {
		le.Dimensions.Width = le.Dimensions.Height * aspect
	} else if le.Dimensions.Width != 0 && le.Dimensions.Height == 0 {
		le.Dimensions.Height = le.Dimensions.Height * (1 / aspect)
	}
}

func (le *LayoutElement) GetConfig(etype ElementConfigType) any {
	for i := range le.ElementConfigs {
		if le.ElementConfigs[i].Type == etype {
			if le.ElementConfigs[i].Config == nil {
				panic("nil configuration")
			}
			return le.ElementConfigs[i].Config
		}
	}
	return nil
}

func (le *LayoutElement) GetSharedConfig() (*SharedElementConfig, bool) {
	cfg, ok := le.GetConfig(ElementConfigTypeShared).(*SharedElementConfig)
	if !ok {
		return &defaultSharedElementConfig, false
	}
	return cfg, true
}

func (le *LayoutElement) attachConfig(config any) *ElementConfig {
	Type := GetElementConfigType(config)
	if le.GetConfig(Type) != nil {
		panic("element already has type")
	}
	le.ElementConfigs = arradd(le.ElementConfigs, ElementConfig{
		Config: config,
		Type:   Type,
	})
	return &le.ElementConfigs[len(le.ElementConfigs)-1]
}

func (context *Context) attachID(id ElementID) {
	element := context.openLayoutElement()
	idAlias := element.ID
	element.ID = id.ID
	context.AddHashMapItem(id, element, idAlias)
	context.LayoutElementIDStrings = arradd(context.LayoutElementIDStrings, id.StringID)
}

func (context *Context) HashMapItem(id uintn) *LayoutElementHashMapItem {
	item, ok := context.GoHash[id]
	if !ok {
		if id != 0 {
			println("item not found in hash map")
		}
		return nil
	}
	return item
}

func (context *Context) AddHashMapItem(elementID ElementID, layoutElem *LayoutElement, idAlias uintn) *LayoutElementHashMapItem {
	id := elementID.ID
	_, existing := context.GoHash[id]
	if existing {
		slog.Debug("warning: overwriting element with id %d\n", id)
	}
	v := &LayoutElementHashMapItem{
		Generation:    context.Generation + 1,
		ElementID:     elementID,
		LayoutElement: layoutElem,
		IDAlias:       idAlias,
		NextIndex:     -1,
	}
	context.GoHash[id] = v
	return v
}

func arrlen[T any](s []T) intn {
	return intn(len(s))
}
func arrfree[T any](s []T) intn {
	return intn(cap(s) - len(s))
}
func arradd[T any](s []T, v T) []T {
	if len(s) < cap(s) {
		return append(s, v)
	}
	panic("attempted to add an element beyond capacity")
}
func arrextend[T any](s []T, extend intn) []T {
	if arrcap(s)-arrlen(s) < extend {
		panic("cannot extend array, not enough capacity")
	}
	return s[:arrlen(s)+extend]
}
func arrcap[T any](s []T) intn {
	return intn(cap(s))
}
func arrpop[T any](s []T) ([]T, T) {
	lastIdx := len(s) - 1
	return s[:lastIdx], s[lastIdx]
}
func arrmemset[T any](s []T, v T) {
	for i := range s {
		s[i] = v
	}
}
func arrlast[T any](s []T) *T {
	return &s[len(s)-1]
}

func arrremoveswapback[T any](s []T, index intn) []T {
	lastIdx := len(s) - 1
	s[index] = s[lastIdx] // Replace element at index with last element.
	return s[:lastIdx]    // Remove last element which was moved.
}

func clamp(Min, Max, v floatn) floatn {
	return min(Max, max(Min, v))
}
func floatequal(a, b floatn) bool {
	return math.Abs(float64(a-b)) < eps
}
func (d Dimensions) SizeAxis(xAxis bool) floatn {
	if xAxis {
		return d.Width
	}
	return d.Height
}

func (d *Dimensions) SizeAxisPtr(xAxis bool) *floatn {
	if xAxis {
		return &d.Width
	}
	return &d.Height
}

func (d *Dimensions) Aspect() floatn {
	return d.Width / d.Height
}

var emptyChildren []intn

func (le *LayoutElement) Children() []intn {
	switch v := le.ChildrenOrTextContent.(type) {
	case []intn:
		return v
	case nil:
		return nil
		panic("no children")
		children := make([]intn, 8)[:0]
		le.ChildrenOrTextContent = children
		return children
	default:
		panic("children must be of type []intn")
	}
}

func (le *LayoutElement) SetChildren(children []intn) {
	le.ChildrenOrTextContent = children
}

func (ap FloatingAttachPointType) AttachLeft() bool {
	return ap == AttachPointLeftTop || ap == AttachPointLeftCenter || ap == AttachPointLeftBottom
}

func (ap FloatingAttachPointType) AttachRight() bool {
	return ap == AttachPointRightTop || ap == AttachPointRightCenter || ap == AttachPointRightBottom
}

func (ap FloatingAttachPointType) AttachHorizontalCenter() bool {
	return ap == AttachPointCenterTop || ap == AttachPointCenterCenter || ap == AttachPointCenterBottom
}

func (ap FloatingAttachPointType) AttachTop() bool {
	return ap == AttachPointLeftTop || ap == AttachPointCenterTop || ap == AttachPointRightTop
}
func (ap FloatingAttachPointType) AttachBottom() bool {
	return ap == AttachPointLeftBottom || ap == AttachPointCenterBottom || ap == AttachPointRightBottom
}
func (ap FloatingAttachPointType) AttachVerticalCenter() bool {
	return ap == AttachPointLeftCenter || ap == AttachPointCenterCenter || ap == AttachPointRightCenter
}

func hashNumber(offset, seed uintn) ElementID {
	hash := seed
	hash += (offset + 48)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += (hash << 3)
	hash ^= (hash >> 11)
	hash += (hash << 15)
	return ElementID{ID: hash + 1, Offset: offset}
}

func hashString(key string, offset, seed uintn) ElementID {
	var hash uintn
	base := seed
	for i := 0; i < len(key); i++ {
		base += uintn(key[i])
		base += base << 10
		base ^= base >> 6
	}
	hash = base
	hash += offset
	hash += hash << 10
	hash ^= hash >> 6

	hash += hash << 3
	base += base << 3
	hash ^= hash >> 11
	base ^= base >> 11
	hash += hash << 15
	base += base << 15
	return ElementID{
		ID:       hash + 1,
		Offset:   offset,
		BaseID:   base + 1,
		StringID: key,
	}
}
