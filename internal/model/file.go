package model

import (
	"encoding/json"
	"time"

	pgvector "github.com/pgvector/pgvector-go"
)

// SourceFile is a file discovered by walking a source (S3, local, etc).
type SourceFile struct {
	Path       string
	Name       string
	Ext        string
	Size       int64
	MimeType   string
	Hash       string
	CreatedAt  time.Time
	ModifiedAt time.Time
	SourceURI  string
}

// File is a row in the files table.
type File struct {
	ID            int64
	Path          string
	Source        string
	CanonicalPath *string
	Modality      string
	Name          string
	Ext           string
	MimeType      string
	Size          int64
	ContentHash   string
	CreatedAt     time.Time
	ModifiedAt    time.Time
	Metadata      json.RawMessage
}

// Item is a row in the items table, plus optional payload fields
// for the indexer→pipeline handoff.
type Item struct {
	ID          int64
	FileID      int64
	ItemType    string
	ItemName    string
	MimeType    string
	Size        int64
	ContentHash string
	ItemIndex   int
	Metadata    json.RawMessage

	// Handoff fields — set by indexer, consumed by pipeline, not persisted.
	Modality string
	Data     []byte
	Text     string
	FilePath string // for logging
}

// Chunk is a row in the chunks table.
type Chunk struct {
	ID          int64
	ItemID      int64
	ChunkIndex  int
	ChunkType   string
	EmbedModel  string
	Embedding   *pgvector.Vector
	TextContent *string
	Metadata    json.RawMessage
}

// FileStatus reports what processing artifacts exist for a file.
type FileStatus struct {
	FileID      int64
	ContentHash string
	HasItems    map[string]bool
	HasChunks   map[string]bool
}
