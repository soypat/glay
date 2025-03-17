package glay

import (
	"unsafe"
)

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
			dimensions := context.measureTextRaw(text[start:], config)
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
	return Dimensions{}
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
	ptrAsNumber := (uintptr)(unsafe.Pointer(&text)) // First value of string header is pointer.
	if config.HashStringContents {
		maxTextLengthToHash := min(len(text), 256)
		for i := 0; i < maxTextLengthToHash; i++ {
			hash += uint32(text[i])
			hash += (hash << 10)
			hash ^= (hash >> 6)
		}
	} else {
		hash += uint32(ptrAsNumber)
		hash += (hash << 10)
		hash ^= (hash >> 6)
	}
	hash += uint32(len(text))
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += uint32(config.FontID)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += uint32(config.FontSize)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += uint32(config.LineHeight)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += uint32(config.LetterSpacing)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += uint32(config.WrapMode)
	hash += (hash << 10)
	hash ^= (hash >> 6)

	hash += (hash << 3)
	hash ^= (hash >> 11)
	hash += (hash << 15)
	return hash + 1
}
