package pipeline

// Modality represents the type of content a file contains.
type ModalityType string

const (
	ModalityImage ModalityType = "image"
	ModalityText  ModalityType = "text"
	ModalityAudio ModalityType = "audio"
	ModalityVideo ModalityType = "video"
	ModalityEmail ModalityType = "email"
)

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
var ModalityStages = map[ModalityType][]Stage{
	ModalityImage: {StageClipEmbed, StageCaption, StageOCR, StageTextEmbed, StageUpsert},
	ModalityText:  {StageTextEmbed, StageUpsert},
	ModalityAudio: {StageTranscribe, StageTextEmbed, StageUpsert},
	ModalityVideo: {StageClipEmbed, StageCaption, StageTranscribe, StageTextEmbed, StageUpsert},
	ModalityEmail: {StageTextEmbed, StageUpsert},
}

// ModalityOrder defines the processing order for extraction.
// Text first (fast/lightweight), video last (heavy).
var ModalityOrder = []ModalityType{
	ModalityText,
	ModalityImage,
	ModalityAudio,
	ModalityVideo,
	ModalityEmail,
}
