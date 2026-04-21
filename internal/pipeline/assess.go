package pipeline

import "github.com/bjluckow/fsvector/internal/store"

// AssessNeededStages determines which processing stages still need
// to run for a file, given its modality and existing DB artifacts.
//
// If the file is new (status == nil) or its content hash has changed,
// all stages for the modality are returned.
//
// If the file is unchanged, only stages whose artifacts are missing
// are returned.
func AssessNeededStages(modality ModalityType, status *store.FileStatus, currentHash string) []Stage {
	all := ModalityStages[modality]

	// new file or content changed — need everything
	if status == nil || status.ContentHash != currentHash {
		return all
	}

	var needed []Stage
	for _, s := range all {
		if !stageComplete(s, modality, status) {
			needed = append(needed, s)
		}
	}
	return needed
}

// stageComplete checks whether a stage's artifacts already exist
// in the DB. This is the Option A approach: infer completion from
// the items and chunks that are present.
func stageComplete(s Stage, modality ModalityType, status *store.FileStatus) bool {
	switch s {
	case StageClipEmbed:
		return status.HasChunks["image:embed"] || status.HasChunks["frame:embed"]
	case StageCaption:
		return status.HasChunks["image:caption"] || status.HasChunks["frame:caption"]
	case StageOCR:
		return status.HasItems["ocr"]
	case StageTranscribe:
		return status.HasItems["transcript"] || status.HasItems["audio_track"]
	case StageTextEmbed:
		// Text embedding is complete if every text-bearing item has
		// embed chunks. This is hard to verify without counting, so
		// we're conservative: if the primary content stages (caption,
		// OCR, transcribe) are all done, we assume text embed is too.
		// A re-run will no-op via the chunk upsert's ON CONFLICT.
		switch modality {
		case ModalityText:
			return status.HasChunks["text:embed"]
		case ModalityImage:
			return status.HasChunks["ocr:embed"] || !status.HasItems["ocr"]
		case ModalityAudio:
			return status.HasChunks["transcript:embed"] || !status.HasItems["transcript"]
		case ModalityVideo:
			ocrDone := status.HasChunks["ocr:embed"] || !status.HasItems["ocr"]
			txnDone := status.HasChunks["transcript:embed"] || !status.HasItems["transcript"]
			return ocrDone && txnDone
		default:
			return false
		}
	case StageUpsert:
		// Upsert is a terminal stage — if we got here, earlier stages
		// determined there's nothing to do, so upsert is also done.
		return true
	default:
		return false
	}
}
