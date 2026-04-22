package pipeline

import "github.com/bjluckow/fsvector/internal/model"

// Stage represents a discrete processing operation in the pipeline.
type Stage string

const (
	StageDownload   Stage = "download"
	StageConvert    Stage = "convert"
	StageClipEmbed  Stage = "clip_embed"
	StageCaption    Stage = "caption"
	StageOCR        Stage = "ocr"
	StageTranscribe Stage = "transcribe"
	StageTextEmbed  Stage = "text_embed"
	StageUpsert     Stage = "upsert"
)

// ModalityStages defines which processing stages apply to each modality.
// Download and convert are handled in extraction (phase 1), so these
// list only the phase 2 worker stages.
var ModalityStages = map[model.Modality][]Stage{
	model.ModalityImage: {StageClipEmbed, StageCaption, StageOCR, StageTextEmbed, StageUpsert},
	model.ModalityText:  {StageTextEmbed, StageUpsert},
	model.ModalityAudio: {StageTranscribe, StageTextEmbed, StageUpsert},
	model.ModalityVideo: {StageClipEmbed, StageCaption, StageTranscribe, StageTextEmbed, StageUpsert},
	model.ModalityEmail: {StageTextEmbed, StageUpsert},
}
