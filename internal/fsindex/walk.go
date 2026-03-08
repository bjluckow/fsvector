package fsindex

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
)

// FileInfo holds the metadata collected for a single file during a walk.
type FileInfo struct {
	Path       string
	Name       string
	Ext        string
	Size       int64
	MimeType   string
	Hash       string
	ModifiedAt time.Time
	CreatedAt  time.Time // best-effort; may equal ModifiedAt on some systems
}

// Walk recursively walks root and returns a FileInfo for every regular file.
// Symlinks and directories are skipped.
func Walk(root string) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// skip directories and symlinks
		if !d.Type().IsRegular() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		hash, err := hashFile(path)
		if err != nil {
			return err
		}

		mime, err := detectMime(path)
		if err != nil {
			mime = "application/octet-stream"
		}

		ext := strings.TrimPrefix(filepath.Ext(d.Name()), ".")

		files = append(files, FileInfo{
			Path:       path,
			Name:       d.Name(),
			Ext:        strings.ToLower(ext),
			Size:       info.Size(),
			MimeType:   mime,
			Hash:       hash,
			ModifiedAt: info.ModTime(),
			CreatedAt:  createdAt(info), // platform-specific, see below
		})

		return nil
	})

	return files, err
}

// hashFile returns the hex-encoded SHA-256 of the file at path.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// detectMime sniffs the mime type from the file's contents.
func detectMime(path string) (string, error) {
	mime, err := mimetype.DetectFile(path)
	if err != nil {
		return "", err
	}
	return mime.String(), nil
}
