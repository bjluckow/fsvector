package source

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bjluckow/fsvector/internal/model"
	"github.com/gabriel-vasile/mimetype"
)

const (
	defaultLargeFileThreshold = 100 * 1024 * 1024      // 100MB
	maxFileSize               = 2 * 1024 * 1024 * 1024 // 2GB
)

// S3Config holds S3 connection parameters.
type S3Config struct {
	Client             *s3.Client
	Bucket             string
	Prefix             string
	LargeFileThreshold int64
	PollInterval       time.Duration
}

// S3Source implements Source for S3 buckets.
// Does not implement Watcher — use fsvector reindex for updates.
type S3Source struct {
	cfg    S3Config
	reader *S3Reader
}

func NewS3Source(cfg S3Config) *S3Source {
	if cfg.LargeFileThreshold == 0 {
		cfg.LargeFileThreshold = defaultLargeFileThreshold
	}
	return &S3Source{
		cfg:    cfg,
		reader: &S3Reader{cfg: cfg},
	}
}

func (s *S3Source) Walk(ctx context.Context) ([]model.SourceFile, error) {
	var files []model.SourceFile

	paginator := s3.NewListObjectsV2Paginator(s.cfg.Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.Bucket),
		Prefix: aws.String(s.cfg.Prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)

			// skip directory markers
			if strings.HasSuffix(key, "/") {
				continue
			}

			// skip files over max size
			if obj.Size != nil && *obj.Size > maxFileSize {
				fmt.Fprintf(os.Stderr, "    skipping %s: exceeds 2GB limit\n", key)
				continue
			}

			name := path.Base(key)
			ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))

			// use etag as content hash
			// strip quotes that AWS wraps around etag
			hash := strings.Trim(aws.ToString(obj.ETag), "\"")

			modifiedAt := time.Time{}
			if obj.LastModified != nil {
				modifiedAt = *obj.LastModified
			}

			size := int64(0)
			if obj.Size != nil {
				size = *obj.Size
			}

			files = append(files, model.SourceFile{
				Path:       s3URI(s.cfg.Bucket, key),
				Name:       name,
				Ext:        ext,
				Size:       size,
				MimeType:   mimeFromExt(ext),
				Hash:       hash,
				ModifiedAt: modifiedAt,
				CreatedAt:  modifiedAt, // S3 has no creation time
				SourceURI:  s.URI(),
			})
		}
	}

	return files, nil
}

func (s *S3Source) Reader() FileReader { return s.reader }

func (s *S3Source) URI() string {
	if s.cfg.Prefix != "" {
		return fmt.Sprintf("s3://%s/%s", s.cfg.Bucket, s.cfg.Prefix)
	}
	return fmt.Sprintf("s3://%s", s.cfg.Bucket)
}

func (s *S3Source) PollInterval() time.Duration { return s.cfg.PollInterval }

// s3URI returns the full S3 URI for a key.
func s3URI(bucket, key string) string {
	return fmt.Sprintf("s3://%s/%s", bucket, key)
}

// mimeFromExt returns a best-effort MIME type from extension.
// S3 objects don't require downloading for MIME detection.
func mimeFromExt(ext string) string {
	m := mimetype.Lookup("." + ext)
	if m != nil {
		return m.String()
	}
	return "application/octet-stream"
}
