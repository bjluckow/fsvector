package fswalk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjluckow/fsvector/internal/fswalk"
)

func TestWalk(t *testing.T) {
	// build a small fixture tree
	root := t.TempDir()
	files := map[string]string{
		"hello.txt":     "hello world",
		"sub/notes.md":  "# notes",
		"sub/image.jpg": "\xff\xd8\xff", // minimal JPEG magic bytes
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	results, err := fswalk.Walk(root)
	if err != nil {
		t.Fatalf("Walk error: %v", err)
	}

	if len(results) != len(files) {
		t.Fatalf("expected %d files, got %d", len(files), len(results))
	}

	// verify each result has required fields populated
	for _, f := range results {
		if f.Path == "" {
			t.Error("empty path")
		}
		if f.Hash == "" {
			t.Error("empty hash", f.Path)
		}
		if f.MimeType == "" {
			t.Error("empty mime type", f.Path)
		}
		if f.Size == 0 && f.Name != ".keep" {
			t.Error("zero size", f.Path)
		}
	}

	// verify hashes are deterministic
	results2, err := fswalk.Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	hashMap := map[string]string{}
	for _, f := range results {
		hashMap[f.Path] = f.Hash
	}
	for _, f := range results2 {
		if hashMap[f.Path] != f.Hash {
			t.Errorf("hash mismatch on second walk for %s", f.Path)
		}
	}
}
