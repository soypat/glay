package glay

import "math"

// OpenTextElement declares a text element as a child of the open element. The text is not
// copied and must remain unmodified until the render commands are consumed.
// A measure text function must be set; see [Config.MeasureText].
func (c *Context) OpenTextElement(text string, textConfig TextElementConfig) {
	c.neededElements++
	if len(c.layoutElements) == cap(c.layoutElements)-1 || c.booleanWarnings.MaxElementsExceeded {
		c.booleanWarnings.MaxElementsExceeded = true
		return
	}
	parentElement := c.openLayoutElement()

	textElement := add(c, &c.layoutElements, layoutElement{textConfig: textConfig, isTextElement: true}, &c.def.element)
	idx := intn(len(c.layoutElements) - 1)
	c.setClipElementID(idx)

	addv(c, &c.layoutElementChildrenBuffer, idx)
	// Measure with the element's copy of the config: taking the address of the parameter
	// would make it escape through the MeasureText callback.
	textMeasured := c.measureTextCached(text, &textElement.textConfig)
	elementID := hashNumber(uintn(parentElement.childrenLength)+uintn(parentElement.floatingChildrenCount), parentElement.id)
	textElement.id = elementID.ID
	c.addHashMapItem(elementID, idx)
	addv(c, &c.layoutElementIDStrings, elementID.StringID)
	textDimensions := Dimensions{Width: textMeasured.unwrappedDimensions.Width, Height: textMeasured.unwrappedDimensions.Height}
	if textConfig.LineHeight > 0 {
		textDimensions.Height = floatn(textConfig.LineHeight)
	}
	textElement.dimensions = textDimensions
	textElement.minDimensions = Dimensions{Width: textMeasured.minWidth, Height: textDimensions.Height}
	textElement.textElementData = textElementData{text: text, preferredDimensions: textMeasured.unwrappedDimensions}
	parentElement.childrenLength++
}

// ResetMeasureTextCache clears the text measurement cache, i.e. after fonts are reloaded.
func (c *Context) ResetMeasureTextCache() {
	c.measureTextHashMapInternal = c.measureTextHashMapInternal[:0]
	c.measureTextHashMapInternalFreeList = c.measureTextHashMapInternalFreeList[:0]
	c.measuredWords = c.measuredWords[:0]
	c.measuredWordsFreeList = c.measuredWordsFreeList[:0]
	clear(c.measureTextHashMap)
	c.measureTextHashMapInternal = c.measureTextHashMapInternal[:1] // Reserve the 0 value to mean "no next element".
	c.measureTextHashMapInternal[0] = measureTextCacheItem{}
}

func (c *Context) measureTextRaw(text string, config *TextElementConfig) Dimensions {
	if c.measureText == nil {
		return Dimensions{}
	}
	return c.measureText(text, config, c.measureTextUserData)
}

func (c *Context) addMeasuredWord(word measuredWord, previousWord *measuredWord) *measuredWord {
	if len(c.measuredWordsFreeList) > 0 {
		newItemIndex := getValue(c, c.measuredWordsFreeList, intn(len(c.measuredWordsFreeList))-1)
		c.measuredWordsFreeList = c.measuredWordsFreeList[:len(c.measuredWordsFreeList)-1]
		set(c, &c.measuredWords, newItemIndex, word)
		previousWord.next = newItemIndex
		return get(c, c.measuredWords, newItemIndex, &c.def.word)
	}
	previousWord.next = intn(len(c.measuredWords))
	return add(c, &c.measuredWords, word, &c.def.word)
}

func (c *Context) measureTextCached(text string, config *TextElementConfig) *measureTextCacheItem {
	if c.measureText == nil {
		if !c.booleanWarnings.TextMeasurementFunctionNotSet {
			c.booleanWarnings.TextMeasurementFunctionNotSet = true
			c.report(ErrTextMeasurementFunctionNotProvided)
		}
		return &c.def.measure
	}
	id := hashStringContentsWithConfig(text, config)
	hashBucket := id % uintn(c.maxMeasureTextCacheWordCount/32)
	elementIndexPrevious := intn(0)
	elementIndex := c.measureTextHashMap[hashBucket]
	for elementIndex != 0 {
		hashEntry := get(c, c.measureTextHashMapInternal, elementIndex, &c.def.measure)
		if hashEntry.id == id {
			hashEntry.generation = c.generation
			return hashEntry
		}
		// This element hasn't been seen in a few frames, delete the hash map item.
		if c.generation-hashEntry.generation > 2 {
			// Add all the measured words that were included in this measurement to the freelist.
			nextWordIndex := hashEntry.measuredWordsStartIndex
			for nextWordIndex != -1 {
				measured := get(c, c.measuredWords, nextWordIndex, &c.def.word)
				addv(c, &c.measuredWordsFreeList, nextWordIndex)
				nextWordIndex = measured.next
			}
			nextIndex := hashEntry.nextIndex
			set(c, &c.measureTextHashMapInternal, elementIndex, measureTextCacheItem{measuredWordsStartIndex: -1})
			addv(c, &c.measureTextHashMapInternalFreeList, elementIndex)
			if elementIndexPrevious == 0 {
				c.measureTextHashMap[hashBucket] = nextIndex
			} else {
				get(c, c.measureTextHashMapInternal, elementIndexPrevious, &c.def.measure).nextIndex = nextIndex
			}
			elementIndex = nextIndex
		} else {
			elementIndexPrevious = elementIndex
			elementIndex = hashEntry.nextIndex
		}
	}

	var newItemIndex intn
	newCacheItem := measureTextCacheItem{measuredWordsStartIndex: -1, id: id, generation: c.generation}
	var measured *measureTextCacheItem
	if len(c.measureTextHashMapInternalFreeList) > 0 {
		newItemIndex = getValue(c, c.measureTextHashMapInternalFreeList, intn(len(c.measureTextHashMapInternalFreeList))-1)
		c.measureTextHashMapInternalFreeList = c.measureTextHashMapInternalFreeList[:len(c.measureTextHashMapInternalFreeList)-1]
		set(c, &c.measureTextHashMapInternal, newItemIndex, newCacheItem)
		measured = get(c, c.measureTextHashMapInternal, newItemIndex, &c.def.measure)
	} else {
		if len(c.measureTextHashMapInternal) == cap(c.measureTextHashMapInternal)-1 {
			if !c.booleanWarnings.MaxTextMeasureCacheExceeded {
				c.report(ErrElementsCapacityExceeded)
				c.booleanWarnings.MaxTextMeasureCacheExceeded = true
			}
			return &c.def.measure
		}
		measured = add(c, &c.measureTextHashMapInternal, newCacheItem, &c.def.measure)
		newItemIndex = intn(len(c.measureTextHashMapInternal) - 1)
	}

	var start, end intn
	var lineWidth, measuredWidth, measuredHeight floatn
	spaceWidth := c.measureTextRaw(" ", config).Width
	tempWord := measuredWord{next: -1}
	previousWord := &tempWord
	for end < intn(len(text)) {
		if len(c.measuredWords) == cap(c.measuredWords)-1 {
			if !c.booleanWarnings.MaxTextMeasureCacheExceeded {
				c.report(ErrTextMeasurementCapacityExceeded)
				c.booleanWarnings.MaxTextMeasureCacheExceeded = true
			}
			return &c.def.measure
		}
		current := text[end]
		if current == ' ' || current == '\n' {
			length := end - start
			var dimensions Dimensions
			if length > 0 {
				dimensions = c.measureTextRaw(text[start:end], config)
			}
			measured.minWidth = cmax(dimensions.Width, measured.minWidth)
			measuredHeight = cmax(measuredHeight, dimensions.Height)
			if current == ' ' {
				dimensions.Width += spaceWidth
				previousWord = c.addMeasuredWord(measuredWord{startOffset: start, length: length + 1, width: dimensions.Width, next: -1}, previousWord)
				lineWidth += dimensions.Width
			}
			if current == '\n' {
				if length > 0 {
					previousWord = c.addMeasuredWord(measuredWord{startOffset: start, length: length, width: dimensions.Width, next: -1}, previousWord)
				}
				previousWord = c.addMeasuredWord(measuredWord{startOffset: end + 1, length: 0, width: 0, next: -1}, previousWord)
				lineWidth += dimensions.Width
				measuredWidth = cmax(lineWidth, measuredWidth)
				measured.containsNewlines = true
				lineWidth = 0
			}
			start = end + 1
		}
		end++
	}
	if end-start > 0 {
		dimensions := c.measureTextRaw(text[start:end], config)
		c.addMeasuredWord(measuredWord{startOffset: start, length: end - start, width: dimensions.Width, next: -1}, previousWord)
		lineWidth += dimensions.Width
		measuredHeight = cmax(measuredHeight, dimensions.Height)
		measured.minWidth = cmax(dimensions.Width, measured.minWidth)
	}
	measuredWidth = cmax(lineWidth, measuredWidth) - floatn(config.LetterSpacing)

	measured.measuredWordsStartIndex = tempWord.next
	measured.unwrappedDimensions.Width = measuredWidth
	measured.unwrappedDimensions.Height = measuredHeight

	if elementIndexPrevious != 0 {
		get(c, c.measureTextHashMapInternal, elementIndexPrevious, &c.def.measure).nextIndex = newItemIndex
	} else {
		c.measureTextHashMap[hashBucket] = newItemIndex
	}
	return measured
}

// hashData is the portable (non-SIMD) variant of clay's Clay__HashData.
func hashData(data string) uint64 {
	var hash uint64
	for i := 0; i < len(data); i++ {
		hash += uint64(data[i])
		hash += hash << 10
		hash ^= hash >> 6
	}
	return hash
}

func hashStringContentsWithConfig(text string, config *TextElementConfig) uint32 {
	hash := uint32(hashData(text) % math.MaxUint32)

	hash += uint32(config.FontID)
	hash += hash << 10
	hash ^= hash >> 6

	hash += uint32(config.FontSize)
	hash += hash << 10
	hash ^= hash >> 6

	hash += uint32(config.LetterSpacing)
	hash += hash << 10
	hash ^= hash >> 6

	hash += hash << 3
	hash ^= hash >> 11
	hash += hash << 15
	return hash + 1 // Reserve the hash result of zero as "null id".
}
