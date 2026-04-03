package source

import "context"

// FileReader abstracts file reading so pipeline functions
// work identically for local and S3 sources.
type FileReader interface {
	// Read returns the full contents of a file at path.
	Read(ctx context.Context, path string) ([]byte, error)

	// Exists returns true if the file exists at path.
	Exists(ctx context.Context, path string) (bool, error)
}
