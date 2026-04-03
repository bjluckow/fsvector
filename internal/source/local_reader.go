package source

import (
	"context"
	"os"
)

type LocalReader struct{}

func (r *LocalReader) Read(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (r *LocalReader) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
