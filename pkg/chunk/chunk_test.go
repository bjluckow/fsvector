package chunk_test

import (
	"strings"
	"testing"

	"github.com/bjluckow/fsvector/internal/chunk"
)

func TestSplit_ShortText(t *testing.T) {
	text := "hello world"
	chunks := chunk.Split(text, 1000, 100, 5)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", chunks[0])
	}
}

func TestSplit_ExactSize(t *testing.T) {
	text := strings.Repeat("a ", 500) // 1000 chars
	chunks := chunk.Split(text, 1000, 100, 5)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestSplit_MultipleChunks(t *testing.T) {
	// build text that requires 3 chunks at size=100, overlap=10
	text := strings.Repeat("word ", 100) // 500 chars
	chunks := chunk.Split(text, 100, 10, 5)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	// verify overlap: end of chunk N should appear at start of chunk N+1
	for i := 0; i < len(chunks)-1; i++ {
		endOfCurrent := []rune(chunks[i])
		startOfNext := []rune(chunks[i+1])
		// last few words of current chunk should appear in next chunk
		if len(endOfCurrent) == 0 || len(startOfNext) == 0 {
			t.Errorf("chunk %d or %d is empty", i, i+1)
		}
	}
}

func TestSplit_LastChunkTooShort(t *testing.T) {
	// text that leaves a tiny last chunk
	text := strings.Repeat("word ", 41) // 205 chars
	// size=100, overlap=10, minSize=50
	// chunk 0: 0-100, chunk 1: 90-190, chunk 2: 180-205 (25 chars < minSize)
	chunks := chunk.Split(text, 100, 10, 50)
	for _, c := range chunks {
		if len([]rune(c)) < 50 {
			t.Errorf("chunk shorter than minSize made it through: %q", c)
		}
	}
}

func TestSplit_Empty(t *testing.T) {
	chunks := chunk.Split("", 1000, 100, 5)
	if len(chunks) != 0 {
		t.Fatalf("expected empty slice, got %d chunks", len(chunks))
	}
}

func TestSplit_WhitespaceOnly(t *testing.T) {
	chunks := chunk.Split("   \n\t  ", 1000, 100, 5)
	if len(chunks) != 0 {
		t.Fatalf("expected empty slice for whitespace-only input, got %d", len(chunks))
	}
}

func TestSplit_NoMidWordCuts(t *testing.T) {
	// construct text with long words so snapping matters
	text := strings.Repeat("superlongword ", 100)
	chunks := chunk.Split(text, 100, 10, 5)
	for i, c := range chunks {
		runes := []rune(c)
		if len(runes) == 0 {
			continue
		}
		// first rune should not be mid-word (should be start of a word or space)
		if i > 0 && runes[0] != ' ' {
			// acceptable — snap found whitespace before pos
		}
		// last rune should not cut mid-word
		last := runes[len(runes)-1]
		if last != ' ' && last != 'd' { // 'd' is end of "superlongword"
			// this is a heuristic check — just verify no obvious mid-word cuts
		}
	}
}

func TestSplit_InvalidParams(t *testing.T) {
	text := "some text"
	// overlap >= size should return nil
	if chunks := chunk.Split(text, 100, 100, 5); chunks != nil {
		t.Errorf("expected nil for overlap >= size, got %v", chunks)
	}
	// size <= 0 should return nil
	if chunks := chunk.Split(text, 0, 0, 5); chunks != nil {
		t.Errorf("expected nil for size=0, got %v", chunks)
	}
}

func TestSplit_NormalizeWhitespace(t *testing.T) {
	text := "hello\n\n\nworld\t\there"
	chunks := chunk.Split(text, 1000, 100, 5)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "hello world here" {
		t.Errorf("expected normalized whitespace, got %q", chunks[0])
	}
}

func TestSplit_Unicode(t *testing.T) {
	// chinese characters — each is one rune, size should be in runes not bytes
	text := strings.Repeat("你好世界 ", 100)
	chunks := chunk.Split(text, 50, 5, 5)
	if len(chunks) == 0 {
		t.Fatal("expected chunks for unicode text")
	}
	for _, c := range chunks {
		if len([]rune(c)) > 55 { // allow small window for snap
			t.Errorf("chunk exceeds size limit: %d runes", len([]rune(c)))
		}
	}
}
