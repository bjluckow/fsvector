package pipeline

// fileType describes how a file extension should be processed.
type fileType struct {
	modality  string
	convertTo string // empty = no conversion needed
}

var fileTypes = map[string]fileType{
	// text — no conversion
	"txt":  {"text", ""},
	"md":   {"text", ""},
	"go":   {"text", ""},
	"py":   {"text", ""},
	"js":   {"text", ""},
	"ts":   {"text", ""},
	"css":  {"text", ""},
	"json": {"text", ""},
	"yaml": {"text", ""},
	"yml":  {"text", ""},
	"toml": {"text", ""},
	"sh":   {"text", ""},
	"rs":   {"text", ""},
	"c":    {"text", ""},
	"cpp":  {"text", ""},
	"h":    {"text", ""},
	"java": {"text", ""},
	"rb":   {"text", ""},
	"html": {"text", ""},
	"htm":  {"text", ""},
	// text — needs conversion
	"pdf":  {"text", "txt"},
	"docx": {"text", "txt"},
	"doc":  {"text", "txt"},
	"odt":  {"text", "txt"},
	"rtf":  {"text", "txt"},
	// image — no conversion
	"jpg":  {"image", ""},
	"jpeg": {"image", ""},
	// image — needs conversion
	"png":  {"image", "jpeg"},
	"gif":  {"image", "jpeg"},
	"webp": {"image", "jpeg"},
	"bmp":  {"image", "jpeg"},
	"tiff": {"image", "jpeg"},
	"tif":  {"image", "jpeg"},
	"heic": {"image", "jpeg"},
	"heif": {"image", "jpeg"},
	// audio — needs normalization
	"mp3": {"audio", ""},
	"m4a": {"audio", ""},
	"wav": {"audio", ""},
	"aac": {"audio", ""},
	"ogg": {"audio", ""},
}

// Modality returns the modality for a given file extension
// and whether the extension is supported at all.
func Modality(ext string) (string, bool) {
	if ft, ok := fileTypes[ext]; ok {
		return ft.modality, true
	}
	return "", false
}

// ConvertTarget returns the target format for conversion,
// or empty string if no conversion is needed.
func ConvertTarget(ext string) string {
	return fileTypes[ext].convertTo
}
