package pipeline

import (
	"sync/atomic"

	"github.com/bjluckow/fsvector/internal/model"
	pgvector "github.com/pgvector/pgvector-go"
)

type fileData struct {
	FilePath string
	Data     []byte
	pending  atomic.Int32
}

func newFileData(path string, data []byte, refcount int) *fileData {
	fd := &fileData{FilePath: path, Data: data}
	fd.pending.Store(int32(refcount))
	return fd
}

// addRef increments the refcount. Called when a worker produces
// downstream work items that still need the file data (e.g., OCR
// producing text chunks that need text embedding).
func (fd *fileData) addRef() {
	fd.pending.Add(1)
}

// release decrements the refcount. When it hits zero, Data is
// nilled to allow garbage collection.
func (fd *fileData) release() {
	if fd.pending.Add(-1) <= 0 {
		fd.Data = nil
	}
}

// refs returns the current refcount (for testing/debugging).
func (fd *fileData) refs() int {
	return int(fd.pending.Load())
}

// job represents a single unit of work flowing through a
// worker queue. Each item targets a specific Stage and carries
// the data needed for that stage's processing function.
type job struct {
	fileData  *fileData
	modality  model.Modality
	stage     Stage
	itemType  string // finer grain: "image", "frame", "ocr_chunk", "transcript_chunk", "body"
	itemID    int64  // set by extractor after UpsertItemRow
	itemIndex int

	// Payload fields — written by the producing stage, read by the consuming stage.
	text      string          // caption text, OCR text, transcript text, chunk text
	embedding pgvector.Vector // populated by embed stages
	newChunks []string        // populated by chunking (OCR/transcript text split into pieces)
}

// ForStage creates a shallow copy of the WorkItem with a different
// target stage. Used when routing results downstream (e.g., caption
// worker produces text that needs text embedding).
func (w *job) ForStage(s Stage) *job {
	return &job{
		fileData:  w.fileData,
		modality:  w.modality,
		stage:     s,
		itemType:  w.itemType,
		itemID:    w.itemID,
		itemIndex: w.itemIndex,
		text:      w.text,
		embedding: w.embedding,
		newChunks: w.newChunks,
	}
}

func jobsForItem(item model.Item, fd *fileData, enabled map[Stage]bool) []*job {
	base := job{
		fileData:  fd,
		modality:  model.Modality(item.Modality),
		itemType:  item.ItemType,
		itemID:    item.ID,
		itemIndex: item.ItemIndex,
	}

	switch item.ItemType {
	case "image", "frame":
		var jobs []*job
		if enabled[StageClipEmbed] {
			j := base
			j.stage = StageClipEmbed
			jobs = append(jobs, &j)
		}
		if enabled[StageCaption] {
			j := base
			j.stage = StageCaption
			jobs = append(jobs, &j)
		}
		if enabled[StageOCR] {
			j := base
			j.stage = StageOCR
			jobs = append(jobs, &j)
		}
		return jobs

	case "text", "body":
		j := base
		j.stage = StageTextEmbed
		j.text = item.Text
		return []*job{&j}

	case "audio", "audio_track":
		j := base
		j.stage = StageTranscribe
		return []*job{&j}

	default:
		return nil
	}
}
