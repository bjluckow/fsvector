package pipeline

import (
	"sync/atomic"

	"github.com/bjluckow/fsvector/internal/source"
	pgvector "github.com/pgvector/pgvector-go"
)

// FileData holds the downloaded/converted bytes for a file, shared
// across multiple WorkItems via refcounting. When the last worker
// calls Release(), Data is nilled to allow GC.
type FileData struct {
	FileInfo source.FileInfo
	Data     []byte
	FileID   int64 // set by extractor after UpsertFileRow
	pending  atomic.Int32
}

// NewFileData creates a FileData with the given initial refcount.
// The refcount should equal the number of WorkItems that will
// reference this data.
func NewFileData(fi source.FileInfo, data []byte, refcount int) *FileData {
	fd := &FileData{
		FileInfo: fi,
		Data:     data,
	}
	fd.pending.Store(int32(refcount))
	return fd
}

// AddRef increments the refcount. Called when a worker produces
// downstream work items that still need the file data (e.g., OCR
// producing text chunks that need text embedding).
func (fd *FileData) AddRef() {
	fd.pending.Add(1)
}

// Release decrements the refcount. When it hits zero, Data is
// nilled to allow garbage collection.
func (fd *FileData) Release() {
	if fd.pending.Add(-1) <= 0 {
		fd.Data = nil
	}
}

// Refs returns the current refcount (for testing/debugging).
func (fd *FileData) Refs() int {
	return int(fd.pending.Load())
}

// WorkItem represents a single unit of work flowing through a
// worker queue. Each item targets a specific Stage and carries
// the data needed for that stage's processing function.
type WorkItem struct {
	FileData  *FileData
	Modality  ModalityType
	Stage     Stage
	ItemType  string // finer grain: "image", "frame", "ocr_chunk", "transcript_chunk", "body"
	ItemID    int64  // set by extractor after UpsertItemRow
	ItemIndex int

	// Payload fields — written by the producing stage, read by the consuming stage.
	Text      string          // caption text, OCR text, transcript text, chunk text
	Embedding pgvector.Vector // populated by embed stages
	Chunks    []string        // populated by chunking (OCR/transcript text split into pieces)
}

// ForStage creates a shallow copy of the WorkItem with a different
// target stage. Used when routing results downstream (e.g., caption
// worker produces text that needs text embedding).
func (w *WorkItem) ForStage(s Stage) *WorkItem {
	return &WorkItem{
		FileData:  w.FileData,
		Modality:  w.Modality,
		Stage:     s,
		ItemType:  w.ItemType,
		ItemID:    w.ItemID,
		ItemIndex: w.ItemIndex,
		Text:      w.Text,
		Embedding: w.Embedding,
		Chunks:    w.Chunks,
	}
}
