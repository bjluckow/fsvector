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
		if end > total {
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
func snapToWhitespace(runes []rune, pos, window int) int {
	total := len(runes)

	// search backwards
	for j := pos; j >= max(0, pos-window); j-- {
		if unicode.IsSpace(runes[j]) {
			return j
		}
	}

	// search forwards
	for j := pos; j < min(total, pos+window); j++ {
		if unicode.IsSpace(runes[j]) {
			return j
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
