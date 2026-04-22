// model/modality.go

package model

type Modality string

const (
	ModalityImage Modality = "image"
	ModalityText  Modality = "text"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
	ModalityEmail Modality = "email"
)

// ModalityPriority defines processing order. Text first (fast), video last (heavy).
var ModalityPriority = []Modality{
	ModalityText,
	ModalityImage,
	ModalityAudio,
	ModalityVideo,
	ModalityEmail,
}

var fileTypes = map[string]Modality{
	// text — plain
	"txt": ModalityText, "md": ModalityText,
	// text — code
	"go": ModalityText, "py": ModalityText, "js": ModalityText, "ts": ModalityText,
	"css": ModalityText, "rs": ModalityText, "c": ModalityText, "cpp": ModalityText,
	"h": ModalityText, "java": ModalityText, "rb": ModalityText, "sh": ModalityText,
	// text — config
	"json": ModalityText, "yaml": ModalityText, "yml": ModalityText, "toml": ModalityText,
	// text — documents
	"html": ModalityText, "htm": ModalityText,
	"pdf": ModalityText, "docx": ModalityText, "doc": ModalityText,
	"odt": ModalityText, "rtf": ModalityText,
	// image
	"jpg": ModalityImage, "jpeg": ModalityImage, "png": ModalityImage,
	"gif": ModalityImage, "webp": ModalityImage, "bmp": ModalityImage,
	"tiff": ModalityImage, "tif": ModalityImage, "heic": ModalityImage, "heif": ModalityImage,
	// audio
	"mp3": ModalityAudio, "m4a": ModalityAudio, "wav": ModalityAudio,
	"aac": ModalityAudio, "ogg": ModalityAudio,
	// video
	"mp4": ModalityVideo, "mov": ModalityVideo, "avi": ModalityVideo, "mkv": ModalityVideo,
	// email
	"eml": ModalityEmail, "msg": ModalityEmail,
}

func FileModality(ext string) (Modality, bool) {
	m, ok := fileTypes[ext]
	return m, ok
}

func GroupByModality(files []SourceFile) map[Modality][]SourceFile {
	groups := make(map[Modality][]SourceFile)
	for _, f := range files {
		mod, ok := FileModality(f.Ext)
		if !ok {
			continue
		}
		groups[mod] = append(groups[mod], f)
	}
	return groups
}
