package pipeline

var fileTypes = map[string]string{
	// text
	"txt": "text", "md": "text", "go": "text", "py": "text",
	"js": "text", "ts": "text", "css": "text", "json": "text",
	"yaml": "text", "yml": "text", "toml": "text", "sh": "text",
	"rs": "text", "c": "text", "cpp": "text", "h": "text",
	"java": "text", "rb": "text", "html": "text", "htm": "text",
	"pdf": "text", "docx": "text", "doc": "text", "odt": "text", "rtf": "text",
	// image
	"jpg": "image", "jpeg": "image", "png": "image", "gif": "image",
	"webp": "image", "bmp": "image", "tiff": "image", "tif": "image",
	"heic": "image", "heif": "image",
	// audio
	"mp3": "audio", "m4a": "audio", "wav": "audio", "aac": "audio", "ogg": "audio",
	// video
	"mp4": "video", "mov": "video", "avi": "video", "mkv": "video",
	// email
	"eml": "email", "msg": "email",
}

func Modality(ext string) (string, bool) {
	m, ok := fileTypes[ext]
	return m, ok
}
