package glay

func (context *Context) openTextElement(text string, config *TextElementConfig) error {
	if arrlen(context.LayoutElements) == arrcap(context.LayoutElements)-1 || context.warnMaxElementsExceeded() {
		return ErrElementsCapacityExceeded
	}
	parentElement := context.openLayoutElement()

	context.LayoutElements = arradd(context.LayoutElements, LayoutElement{})
	textElement := arrlast(context.LayoutElements)
	if arrlen(context.openClipElementStack) > 0 {
		context.LayoutElementClipElementIDs[len(context.LayoutElements)-1] = context.openClipElementStack[arrlen(context.openClipElementStack)-1]
	} else {
		context.LayoutElementClipElementIDs[len(context.LayoutElements)-1] = 0
	}
	context.LayoutElementChildrenBuffer = arradd(context.LayoutElementChildrenBuffer, arrlen(context.LayoutElements)-1)
	textMeasured := context.measureTextCached(text, config)

	elementID := hashNumber(uintn(len(parentElement.Children())), parentElement.ID)
	textElement.ID = elementID.ID
	context.AddHashMapItem(elementID, textElement, 0)
	context.LayoutElementIDStrings = arradd(context.LayoutElementIDStrings, elementID.StringID)

	var textHeight floatn
	if config.LineHeight > 0 {
		textHeight = floatn(config.LineHeight)
	} else {
		textHeight = textMeasured.unwrappedDimensions.Height
	}
	textElement.Dimensions = Dimensions{Width: textMeasured.unwrappedDimensions.Width, Height: textHeight}
	textElement.MinDimensions = Dimensions{Width: textMeasured.minWidth, Height: textHeight}

	context.TextElementData = arradd(context.TextElementData, TextElementData{
		Text:                text,
		PreferredDimensions: textMeasured.unwrappedDimensions,
		ElementIndex:        arrlen(context.LayoutElements) - 1,
	})
	textElement.ChildrenOrTextContent = arrlast(context.TextElementData)

	context.ElementConfigs = arradd(context.ElementConfigs, ElementConfig{
		Type:   ElementConfigTypeText,
		Config: config,
	})
	textElement.ElementConfigs = context.ElementConfigs[len(context.ElementConfigs)-1:]
	textElement.LayoutConfig = &defaultLayoutConfig

	children := parentElement.Children()
	parentElement.ChildrenOrTextContent = append(children, -1)
	return nil
}

func (context *Context) measureTextCached(text string, config *TextElementConfig) *measureTextCacheItem {
	id := hashTextWithConfig(text, config)
	hashbucket := id % (uint32(context.MaxMeasureTextCacheWordCount) / 32)
	var elementIndexPrevious intn
	elementIndex := context.measureTextHashMap[hashbucket]
	for elementIndex != 0 {
		hashEntry := &context.measureTextHashMapInternal[elementIndex]
		if hashEntry.ID == uintn(id) {
			hashEntry.generation = context.Generation
			return hashEntry
		}
		// This element hasn't been seen in a few frames, delete the hash map item.
		if context.Generation-hashEntry.generation > 2 {
			// Add all the measured words that were included in this measurement to the freelist
			nextWordIndex := hashEntry.measureWordsStartIndex
			for nextWordIndex != -1 {
				measured := &context.measuredWords[nextWordIndex]
				context.measuredWordsFreeList = arradd(context.measuredWordsFreeList, nextWordIndex)
				nextWordIndex = measured.Next
			}

			nextIndex := hashEntry.nextIndex
			context.measureTextHashMapInternal[elementIndex] = measureTextCacheItem{measureWordsStartIndex: -1}
			context.MeasureTextHashMapInternalFreelist = arradd(context.MeasureTextHashMapInternalFreelist, elementIndex)
			if elementIndexPrevious == 0 {
				context.measureTextHashMap[hashbucket] = nextIndex
			} else {
				previousHashEntry := &context.measureTextHashMapInternal[elementIndexPrevious]
				previousHashEntry.nextIndex = nextIndex
			}
			elementIndex = nextIndex
		} else {
			elementIndexPrevious = elementIndex
			elementIndex = hashEntry.nextIndex
		}
	}

	var newItemIndex intn
	newCacheItem := measureTextCacheItem{measureWordsStartIndex: -1, ID: id, generation: context.Generation}
	var measured *measureTextCacheItem
	if len(context.MeasureTextHashMapInternalFreelist) > 0 {
		context.MeasureTextHashMapInternalFreelist, newItemIndex = arrpop(context.MeasureTextHashMapInternalFreelist)
		context.measureTextHashMapInternal[newItemIndex] = newCacheItem
		measured = &context.measureTextHashMapInternal[newItemIndex]
	} else {
		if arrfree(context.measureTextHashMapInternal) == 0 {
			panic("clay ran out of capacity during text element measurement")
		}
		context.measureTextHashMapInternal = arradd(context.measureTextHashMapInternal, newCacheItem)
		newItemIndex = arrlen(context.measureTextHashMapInternal) - 1
		measured = &context.measureTextHashMapInternal[newItemIndex]
	}
	var start, end intn
	var lineWidth, measuredWidth, measuredHeight floatn
	spaceWidth := context.measureSpaceWidth(config)
	tempWord := measuredWord{Next: -1}
	prevWord := &tempWord
	for end < intn(len(text)) {
		if arrfree(context.measuredWords) == 0 {
			panic("clay run out of space in internal text measurement cache")
		}
		current := text[end]
		if current == ' ' || current == '\n' {
			length := end - start
			var dimensions Dimensions
			if length > 0 {
				dimensions = context.measureTextRaw(text[start:], config)
			}
			measured.minWidth = max(dimensions.Width, measured.minWidth)
			measuredHeight = max(measuredHeight, dimensions.Height)
			if current == ' ' {
				dimensions.Width += spaceWidth
				word := measuredWord{StartOffset: start, Length: length + 1, Width: dimensions.Width, Next: -1}
				prevWord = context.addMeasuredWord(word, prevWord)
				lineWidth += dimensions.Width
			}
			if current == '\n' {
				if length > 0 {
					word := measuredWord{StartOffset: start, Length: length, Width: dimensions.Width, Next: -1}
					prevWord = context.addMeasuredWord(word, prevWord)
				}
				word := measuredWord{StartOffset: end + 1, Next: -1}
				prevWord = context.addMeasuredWord(word, prevWord)
				lineWidth += dimensions.Width
				measuredWidth = max(lineWidth, measuredWidth)
				measured.containsNewlines = true
				lineWidth = 0
			}
			start = end + 1
		}
		end++
	}
	if end-start > 0 {
		dimensions := context.measureTextRaw(text[start:end], config)
		word := measuredWord{StartOffset: start, Length: end - start, Width: dimensions.Width, Next: -1}
		_ = context.addMeasuredWord(word, prevWord)
		lineWidth += dimensions.Width
		measuredHeight = max(measuredHeight, dimensions.Height)
		measured.minWidth = max(dimensions.Width, measured.minWidth)
	}
	measuredWidth = max(lineWidth, measuredWidth)

	measured.measureWordsStartIndex = tempWord.Next
	measured.unwrappedDimensions.Width = measuredWidth
	measured.unwrappedDimensions.Height = measuredHeight

	if elementIndexPrevious != 0 {
		context.measureTextHashMapInternal[elementIndexPrevious].nextIndex = newItemIndex
	} else {
		context.measureTextHashMap[hashbucket] = newItemIndex
	}
	return measured
}

func (context *Context) measureSpaceWidth(textconfig *TextElementConfig) floatn {
	return context.measureTextRaw(" ", textconfig).Width
}

func (context *Context) measureTextRaw(text string, textconfig *TextElementConfig) Dimensions {
	if context.MeasureTextFunction == nil {
		context.logerr("measuretextfunc==nil")
		return Dimensions{}
	}
	return context.MeasureTextFunction(text, textconfig, context.MeasureTextUserData)
}

func (context *Context) addMeasuredWord(word measuredWord, previousWord *measuredWord) *measuredWord {
	if len(context.measuredWordsFreeList) > 0 {
		var newItemIndex intn
		context.measuredWordsFreeList, newItemIndex = arrpop(context.measuredWordsFreeList)
		context.measuredWords[newItemIndex] = word
		previousWord.Next = newItemIndex
		return &context.measuredWords[newItemIndex]
	}
	previousWord.Next = arrlen(context.measuredWords)
	context.measuredWords = arradd(context.measuredWords, word)
	return &context.measuredWords[len(context.measuredWords)-1]
}

func hashTextWithConfig(text string, config *TextElementConfig) (hash uint32) {
	for i := 0; i < len(text); i++ {
		hash += uint32(text[i])
		hash += (hash << 10)
		hash ^= (hash >> 6)
	}

	hash += uint32(config.FontID)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += uint32(config.FontSize)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += uint32(config.LetterSpacing)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += (hash << 3)
	hash ^= (hash >> 11)
	hash += (hash << 15)
	return hash + 1
}
