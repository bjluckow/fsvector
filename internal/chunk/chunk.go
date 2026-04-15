package chunk

import (
	"strings"
	"unicode"
)

// Split splits text into overlapping chunks of at most size runes,
// with overlap runes of context carried over between chunks.
// Chunks shorter than minSize runes are discarded.
// Chunk boundaries are snapped to the nearest whitespace within a
// 50-rune window to avoid cutting mid-word.
func Split(text string, size, overlap, minSize int) []string {
	if size <= 0 || overlap < 0 || overlap >= size {
		return nil
	}

	text = normalizeWhitespace(text)
	runes := []rune(text)
	total := len(runes)

	if total == 0 {
		return nil
	}

	var chunks []string
	i := 0

	for i < total {
		end := i + size
		if end >= total {
			end = total
		} else {
			end = snapToWhitespace(runes, end, 50)
		}

		chunk := strings.TrimSpace(string(runes[i:end]))
		if len([]rune(chunk)) >= minSize {
			chunks = append(chunks, chunk)
		}

		if end >= total {
			break
		}

		i += size - overlap
	}

	return chunks
}

// snapToWhitespace tries to snap pos to the nearest whitespace boundary
// within window runes, searching backwards first then forwards.
// Returns pos unchanged if no whitespace is found within the window.
func snapToWhitespace(text []rune, pos, radius int) int {
	// ensure we never go out of bounds
	if pos >= len(text) {
		return len(text)
	}
	// search backwards for whitespace
	for i := pos; i > pos-radius && i > 0; i-- {
		if unicode.IsSpace(text[i]) {
			return i
		}
	}
	// search forwards for whitespace
	for i := pos; i < pos+radius && i < len(text); i++ { // ← i < len(text) not i <= len(text)
		if unicode.IsSpace(text[i]) {
			return i
		}
	}
	return pos
}

// normalizeWhitespace collapses runs of whitespace (including newlines)
// into single spaces and trims leading/trailing whitespace.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
