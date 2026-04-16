package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/api"
	"github.com/bjluckow/fsvector/pkg/chunk"
)

func processEmail(ctx context.Context, cfg Config, fi source.FileInfo) (Result, error) {
	data, err := cfg.Reader.Read(ctx, fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	parsed, err := cfg.ConvertClient.ParseEmail(ctx, fi.Name, data)
	if err != nil {
		return Result{}, fmt.Errorf("parse email %s: %w", fi.Path, err)
	}

	var files []store.UpsertFile
	chunkOffset := 0
	emailBodyType := "email-body"

	// 1. embed body as text chunks
	if strings.TrimSpace(parsed.Body) != "" {
		bodyChunks := chunk.Split(parsed.Body, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
		for i, c := range bodyChunks {
			f, err := processTextChunk(ctx, cfg, fi, c, chunkOffset+i)
			if err != nil || f == nil {
				continue
			}
			f.Modality = "email"
			f.ChunkType = &emailBodyType
			f.Metadata = map[string]any{
				"subject": parsed.Subject,
				"from":    parsed.From,
				"to":      parsed.To,
				"date":    parsed.Date,
			}
			files = append(files, *f)
		}
		chunkOffset += len(bodyChunks)
	}

	// 2. process attachments — max depth 1, no recursion into email attachments
	for _, att := range parsed.Attachments {
		attData, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "    decode attachment %s in %s: %v\n", att.Filename, fi.Path, err)
			continue
		}

		attExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(att.Filename), "."))

		// skip nested emails — no recursion
		if attExt == "eml" || attExt == "msg" {
			fmt.Printf("    skipping nested email attachment %s\n", att.Filename)
			continue
		}

		modality, ok := Modality(attExt)
		if !ok {
			fmt.Printf("    skipping unsupported attachment %s\n", att.Filename)
			continue
		}

		// synthetic FileInfo for the attachment
		attFI := source.FileInfo{
			Path:       attachmentPath(fi.Path, att.Filename),
			Name:       att.Filename,
			Ext:        attExt,
			Size:       int64(len(attData)),
			MimeType:   att.Mime,
			Hash:       hashBytes(attData),
			ModifiedAt: fi.ModifiedAt,
			CreatedAt:  fi.CreatedAt,
			SourceURI:  fi.SourceURI,
		}

		// register attachment bytes so readFile can find them
		attCfg := cfg.withSyntheticData(attFI.Path, attData)

		attResult, err := Process(ctx, attCfg, attFI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "    attachment %s in %s: %v\n", att.Filename, fi.Path, err)
			continue
		}
		if attResult.Skipped {
			continue
		}

		// offset chunk indexes and tag with email metadata
		for _, f := range attResult.Files {
			f.ChunkIndex += chunkOffset
			if f.Metadata == nil {
				f.Metadata = map[string]any{}
			}
			f.Metadata["email_path"] = fi.Path
			f.Metadata["email_subject"] = parsed.Subject
			f.Metadata["email_from"] = parsed.From
			f.Metadata["email_date"] = parsed.Date
			f.Metadata["attachment"] = att.Filename
			f.Metadata["modality"] = modality
			files = append(files, f)
		}
		chunkOffset += len(attResult.Files)
	}

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "no embeddable content in email",
		}, nil
	}

	return Result{Files: files}, nil
}

// attachmentPath returns a synthetic path for an email attachment.
// Uses :: as separator since it won't appear in S3 or local paths.
func attachmentPath(emailPath, filename string) string {
	return emailPath + api.AttachmentSep + filename
}

// hashBytes returns a simple hash of bytes for dedup detection.
func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
