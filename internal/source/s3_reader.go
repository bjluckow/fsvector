package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Reader implements FileReader for S3 objects.
type S3Reader struct {
	cfg S3Config
}

func (r *S3Reader) Read(ctx context.Context, path string) ([]byte, error) {
	bucket, key, err := parseS3URI(path)
	if err != nil {
		return nil, err
	}

	// check size first via HeadObject
	head, err := r.cfg.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 head %s: %w", path, err)
	}

	size := int64(0)
	if head.ContentLength != nil {
		size = *head.ContentLength
	}

	if size > r.cfg.LargeFileThreshold {
		return r.readToTemp(ctx, bucket, key)
	}
	return r.readToMemory(ctx, bucket, key)
}

func (r *S3Reader) Exists(ctx context.Context, path string) (bool, error) {
	bucket, key, err := parseS3URI(path)
	if err != nil {
		return false, err
	}
	_, err = r.cfg.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// check for 404
		return false, nil
	}
	return true, nil
}

func (r *S3Reader) readToMemory(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := r.cfg.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s/%s: %w", bucket, key, err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (r *S3Reader) readToTemp(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := r.cfg.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s/%s: %w", bucket, key, err)
	}
	defer out.Body.Close()

	tmp, err := os.CreateTemp("", "fsvector-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, out.Body); err != nil {
		return nil, fmt.Errorf("stream to temp: %w", err)
	}

	if _, err := tmp.Seek(0, 0); err != nil {
		return nil, err
	}
	return io.ReadAll(tmp)
}

// parseS3URI parses s3://bucket/key into its components.
func parseS3URI(uri string) (bucket, key string, err error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URI: %s", uri)
	}
	uri = strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(uri, "/", 2)
	if len(parts) < 2 {
		return parts[0], "", nil
	}
	return parts[0], parts[1], nil
}
