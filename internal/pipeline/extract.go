// internal/pipeline/extract.go

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/bjluckow/fsvector/internal/clients"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/chunk"
)

// Extractor handles phase 1 for a specific modality: download,
// convert, create DB rows (UpsertFile + UpsertItem), assess which
// stages are needed, and return WorkItems for phase 2 workers.
type Extractor interface {
	Extract(ctx context.Context, fi source.FileInfo) ([]*WorkItem, error)
}

// ImageExtractor handles image files (jpg, png, heic, webp, etc.)
type ImageExtractor struct {
	Reader  source.FileReader
	Convert *clients.ConvertClient
}

func (e *ImageExtractor) Extract(ctx context.Context, fi source.FileInfo) ([]*WorkItem, error) {
	data, err := e.Reader.Read(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	// convert non-JPEG/PNG to JPEG for model consumption
	ext := strings.ToLower(fi.Ext)
	if ext != "jpg" && ext != "jpeg" && ext != "png" {
		converted, err := ConvertToImage(ctx, e.Convert, fi.Name, data)
		if err != nil {
			return nil, err
		}
		data = converted
	}

	// phase 1 DB writes
	fileID, err := store.UpsertFileRow(ctx, store.FileRow{
		Path:        fi.Path,
		Source:      fi.SourceURI,
		Modality:    string(ModalityImage),
		FileName:    fi.Name,
		FileExt:     fi.Ext,
		MimeType:    fi.MimeType,
		Size:        fi.Size,
		ContentHash: fi.Hash,
		CreatedAt:   fi.CreatedAt,
		ModifiedAt:  fi.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", fi.Path, err)
	}

	imageItemID, err := store.UpsertItemRow(ctx, fileID, store.ItemRow{
		ItemType:    "image",
		ItemName:    fi.Name,
		MimeType:    fi.MimeType,
		Size:        fi.Size,
		ContentHash: fi.Hash,
		ItemIndex:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert image item %s: %w", fi.Path, err)
	}

	// assess what stages are already done
	status, err := store.GetFileStatus(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", fi.Path, err)
	}
	needed := AssessNeededStages(ModalityImage, status, fi.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	// build work items for needed stages
	needSet := make(map[Stage]bool, len(needed))
	for _, s := range needed {
		needSet[s] = true
	}

	var workerCount int
	if needSet[StageClipEmbed] {
		workerCount++
	}
	if needSet[StageCaption] {
		workerCount++
	}
	if needSet[StageOCR] {
		workerCount++
	}
	if workerCount == 0 {
		return nil, nil
	}

	fd := NewFileData(fi, data, workerCount)
	fd.FileID = fileID

	var items []*WorkItem
	if needSet[StageClipEmbed] {
		items = append(items, &WorkItem{
			FileData: fd, Modality: ModalityImage,
			Stage: StageClipEmbed, ItemType: "image",
			ItemID: imageItemID, ItemIndex: 0,
		})
	}
	if needSet[StageCaption] {
		items = append(items, &WorkItem{
			FileData: fd, Modality: ModalityImage,
			Stage: StageCaption, ItemType: "image",
			ItemID: imageItemID, ItemIndex: 0,
		})
	}
	if needSet[StageOCR] {
		items = append(items, &WorkItem{
			FileData: fd, Modality: ModalityImage,
			Stage: StageOCR, ItemType: "image",
			ItemID: imageItemID, ItemIndex: 0,
		})
	}

	return items, nil
}

// TextExtractor handles text files (txt, md, docx, pdf, etc.)
type TextExtractor struct {
	Reader       source.FileReader
	Convert      *clients.ConvertClient
	ChunkSize    int
	ChunkOverlap int
	MinChunkSize int
}

func (e *TextExtractor) Extract(ctx context.Context, fi source.FileInfo) ([]*WorkItem, error) {
	data, err := e.Reader.Read(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	// convert non-plaintext formats
	ext := strings.ToLower(fi.Ext)
	if ext != "txt" && ext != "md" {
		converted, err := ConvertToText(ctx, e.Convert, fi.Name, data)
		if err != nil {
			return nil, err
		}
		data = converted
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, nil
	}

	// phase 1 DB writes
	fileID, err := store.UpsertFileRow(ctx, store.FileRow{
		Path:        fi.Path,
		Source:      fi.SourceURI,
		Modality:    string(ModalityText),
		FileName:    fi.Name,
		FileExt:     fi.Ext,
		MimeType:    fi.MimeType,
		Size:        fi.Size,
		ContentHash: fi.Hash,
		CreatedAt:   fi.CreatedAt,
		ModifiedAt:  fi.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", fi.Path, err)
	}

	textItemID, err := store.UpsertItemRow(ctx, fileID, store.ItemRow{
		ItemType:    "text",
		ItemName:    fi.Name,
		MimeType:    "text/plain",
		Size:        int64(len(text)),
		ContentHash: fi.Hash,
		ItemIndex:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert text item %s: %w", fi.Path, err)
	}

	// assess
	status, err := store.GetFileStatus(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", fi.Path, err)
	}
	needed := AssessNeededStages(ModalityText, status, fi.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	// chunk the text — each chunk becomes a TextEmbed work item
	chunks := chunkText(text, e.ChunkSize, e.ChunkOverlap, e.MinChunkSize)
	if len(chunks) == 0 {
		return nil, nil
	}

	fd := NewFileData(fi, data, len(chunks))
	fd.FileID = fileID

	var items []*WorkItem
	for i, c := range chunks {
		items = append(items, &WorkItem{
			FileData: fd, Modality: ModalityText,
			Stage: StageTextEmbed, ItemType: "text",
			ItemID: textItemID, ItemIndex: i,
			Text: c,
		})
	}
	return items, nil
}

// AudioExtractor handles audio files (mp3, wav, m4a, etc.)
type AudioExtractor struct {
	Reader  source.FileReader
	Convert *clients.ConvertClient
}

func (e *AudioExtractor) Extract(ctx context.Context, fi source.FileInfo) ([]*WorkItem, error) {
	data, err := e.Reader.Read(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	// normalize to 16kHz mono WAV for Whisper
	ext := strings.ToLower(fi.Ext)
	if ext != "wav" {
		converted, err := e.Convert.NormalizeAudio(ctx, fi.Name, data)
		if err != nil {
			return nil, fmt.Errorf("normalize audio %s: %w", fi.Path, err)
		}
		data = converted
	}

	// phase 1 DB writes
	fileID, err := store.UpsertFileRow(ctx, store.FileRow{
		Path:        fi.Path,
		Source:      fi.SourceURI,
		Modality:    string(ModalityAudio),
		FileName:    fi.Name,
		FileExt:     fi.Ext,
		MimeType:    fi.MimeType,
		Size:        fi.Size,
		ContentHash: fi.Hash,
		CreatedAt:   fi.CreatedAt,
		ModifiedAt:  fi.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", fi.Path, err)
	}

	audioItemID, err := store.UpsertItemRow(ctx, fileID, store.ItemRow{
		ItemType:    "audio",
		ItemName:    fi.Name,
		MimeType:    "audio/wav",
		Size:        int64(len(data)),
		ContentHash: fi.Hash,
		ItemIndex:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert audio item %s: %w", fi.Path, err)
	}

	// assess
	status, err := store.GetFileStatus(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", fi.Path, err)
	}
	needed := AssessNeededStages(ModalityAudio, status, fi.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	fd := NewFileData(fi, data, 1)
	fd.FileID = fileID

	return []*WorkItem{{
		FileData: fd, Modality: ModalityAudio,
		Stage: StageTranscribe, ItemType: "audio",
		ItemID: audioItemID, ItemIndex: 0,
	}}, nil
}

// VideoExtractor handles video files (mp4, mov, avi, etc.)
type VideoExtractor struct {
	Reader    source.FileReader
	Convert   *clients.ConvertClient
	FrameRate float64
}

func (e *VideoExtractor) Extract(ctx context.Context, fi source.FileInfo) ([]*WorkItem, error) {
	data, err := e.Reader.Read(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	// phase 1 DB writes for the file
	fileID, err := store.UpsertFileRow(ctx, store.FileRow{
		Path:        fi.Path,
		Source:      fi.SourceURI,
		Modality:    string(ModalityVideo),
		FileName:    fi.Name,
		FileExt:     fi.Ext,
		MimeType:    fi.MimeType,
		Size:        fi.Size,
		ContentHash: fi.Hash,
		CreatedAt:   fi.CreatedAt,
		ModifiedAt:  fi.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", fi.Path, err)
	}

	// assess before doing expensive frame extraction
	status, err := store.GetFileStatus(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", fi.Path, err)
	}
	needed := AssessNeededStages(ModalityVideo, status, fi.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	needSet := make(map[Stage]bool, len(needed))
	for _, s := range needed {
		needSet[s] = true
	}

	var items []*WorkItem

	// extract frames
	if needSet[StageClipEmbed] || needSet[StageCaption] {
		frames, err := e.Convert.ExtractVideoFrames(ctx, fi.Name, data, e.FrameRate)
		if err != nil {
			fmt.Printf("      extract frames %s: %v\n", fi.Path, err)
		} else {
			for _, frame := range frames {
				frameItemID, err := store.UpsertItemRow(ctx, fileID, store.ItemRow{
					ItemType:  "frame",
					ItemName:  fmt.Sprintf("frame_%06d.jpg", frame.Index),
					MimeType:  "image/jpeg",
					Size:      int64(len(frame.Data)),
					ItemIndex: frame.Index,
					Metadata: mustJSON(map[string]any{
						"timestamp_ms": frame.TimestampMs,
						"fps":          e.FrameRate,
					}),
				})
				if err != nil {
					fmt.Printf("      upsert frame item %s[%d]: %v\n", fi.Path, frame.Index, err)
					continue
				}

				frameFD := NewFileData(fi, frame.Data, 0)
				frameFD.FileID = fileID

				var refcount int
				if needSet[StageClipEmbed] {
					refcount++
				}
				if needSet[StageCaption] {
					refcount++
				}
				frameFD.pending.Store(int32(refcount))

				if needSet[StageClipEmbed] {
					items = append(items, &WorkItem{
						FileData: frameFD, Modality: ModalityVideo,
						Stage: StageClipEmbed, ItemType: "frame",
						ItemID: frameItemID, ItemIndex: frame.Index,
					})
				}
				if needSet[StageCaption] {
					items = append(items, &WorkItem{
						FileData: frameFD, Modality: ModalityVideo,
						Stage: StageCaption, ItemType: "frame",
						ItemID: frameItemID, ItemIndex: frame.Index,
					})
				}
			}
		}
	}

	// extract audio track
	if needSet[StageTranscribe] {
		audioData, err := e.Convert.ExtractVideoAudio(ctx, fi.Name, data)
		if err != nil {
			fmt.Printf("      extract audio %s: %v\n", fi.Path, err)
		} else {
			audioItemID, err := store.UpsertItemRow(ctx, fileID, store.ItemRow{
				ItemType:  "audio_track",
				ItemName:  fi.Name,
				MimeType:  "audio/wav",
				Size:      int64(len(audioData)),
				ItemIndex: 0,
			})
			if err != nil {
				fmt.Printf("      upsert audio item %s: %v\n", fi.Path, err)
			} else {
				audioFD := NewFileData(fi, audioData, 1)
				audioFD.FileID = fileID
				items = append(items, &WorkItem{
					FileData: audioFD, Modality: ModalityVideo,
					Stage: StageTranscribe, ItemType: "audio_track",
					ItemID: audioItemID, ItemIndex: 0,
				})
			}
		}
	}

	return items, nil
}

// EmailExtractor handles email files (eml, msg)
type EmailExtractor struct {
	Reader       source.FileReader
	Convert      *clients.ConvertClient
	ChunkSize    int
	ChunkOverlap int
	MinChunkSize int
}

func (e *EmailExtractor) Extract(ctx context.Context, fi source.FileInfo) ([]*WorkItem, error) {
	data, err := e.Reader.Read(ctx, fi.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	parsed, err := e.Convert.ParseEmail(ctx, fi.Name, data)
	if err != nil {
		return nil, fmt.Errorf("parse email %s: %w", fi.Path, err)
	}

	// phase 1 DB writes
	fileID, err := store.UpsertFileRow(ctx, store.FileRow{
		Path:        fi.Path,
		Source:      fi.SourceURI,
		Modality:    string(ModalityEmail),
		FileName:    fi.Name,
		FileExt:     fi.Ext,
		MimeType:    fi.MimeType,
		Size:        fi.Size,
		ContentHash: fi.Hash,
		CreatedAt:   fi.CreatedAt,
		ModifiedAt:  fi.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", fi.Path, err)
	}

	var items []*WorkItem

	// body text
	body := strings.TrimSpace(parsed.Body)
	if body != "" {
		bodyItemID, err := store.UpsertItemRow(ctx, fileID, store.ItemRow{
			ItemType:  "body",
			ItemName:  parsed.Subject,
			MimeType:  "text/plain",
			Size:      int64(len(body)),
			ItemIndex: 0,
		})
		if err != nil {
			return nil, fmt.Errorf("upsert body item %s: %w", fi.Path, err)
		}

		chunks := chunkText(body, e.ChunkSize, e.ChunkOverlap, e.MinChunkSize)
		if len(chunks) > 0 {
			fd := NewFileData(fi, []byte(body), len(chunks))
			fd.FileID = fileID

			for i, c := range chunks {
				items = append(items, &WorkItem{
					FileData: fd, Modality: ModalityEmail,
					Stage: StageTextEmbed, ItemType: "body",
					ItemID: bodyItemID, ItemIndex: i,
					Text: c,
				})
			}
		}
	}

	// TODO: attachments — decode base64, route through appropriate
	// sub-extractor (ImageExtractor for images, TextExtractor for docs).
	// For M0.9.5.

	return items, nil
}

// chunkText is a placeholder that defers to the existing chunk package.
// Replace with actual import of your chunk.Split function.
func chunkText(text string, size, overlap, minSize int) []string {
	return chunk.Split(text, size, overlap, minSize)
}
