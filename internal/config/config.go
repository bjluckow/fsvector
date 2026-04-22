package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	// Database (from .env only)
	DatabaseURL string

	// Embed
	EmbedSvcURL string
	EmbedModel  string

	// Convert
	ConvertSvcURL string

	// Transcribe
	TranscribeSvcURL string

	// Vision
	VisionSvcURL  string
	CaptionFrames bool

	// Pipeline
	ChunkSize        int
	ChunkOverlap     int
	MinChunkSize     int
	MinEmbedSize     int64
	VideoFrameRate   float64
	EnableCaption    bool
	EnableOCR        bool
	EnableTranscribe bool

	// Source
	SourceType         string
	WatchPath          string
	S3Bucket           string
	S3Prefix           string
	S3Region           string
	LargeFileThreshold int64

	// Daemon
	DaemonPort  int
	WorkerCount int
}

func Load() (*Config, error) {
	// YAML config
	viper.SetConfigName("fsvector")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.fsvector")

	// env vars override yaml
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// set defaults
	viper.SetDefault("database_url", "postgres://fsvector:fsvector@postgres:5432/fsvector")
	viper.SetDefault("embed.url", "http://embedsvc:8000")
	viper.SetDefault("embed.model", "clip-ViT-B-32")
	viper.SetDefault("convert.url", "http://convertsvc:8001")
	viper.SetDefault("transcribe.url", "http://transcribesvc:8002")
	viper.SetDefault("vision.url", "http://visionsvc:8003")
	viper.SetDefault("vision.ocr_enabled", true)
	viper.SetDefault("vision.caption_frames", false)
	viper.SetDefault("pipeline.chunk_size", 1000)
	viper.SetDefault("pipeline.chunk_overlap", 100)
	viper.SetDefault("pipeline.min_chunk_size", 10)
	viper.SetDefault("pipeline.min_embed_size", 100)
	viper.SetDefault("pipeline.video_frame_rate", 1.0)
	viper.SetDefault("pipeline.enable_caption", true)
	viper.SetDefault("pipeline.enable_ocr", true)
	viper.SetDefault("pipeline.enable_transcribe", true)
	viper.SetDefault("source.type", "local")
	viper.SetDefault("source.watch_path", "/data/source")
	viper.SetDefault("source.s3_region", "us-east-1")
	viper.SetDefault("source.large_file_threshold", 104857600)
	viper.SetDefault("daemon.port", 8080)
	viper.SetDefault("daemon.worker_count", 4)

	// bind custom env vars
	viper.BindEnv("source.s3_bucket", "AWS_BUCKET_NAME")
	viper.BindEnv("source.s3_region", "AWS_REGION")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("config: %w", err)
		}
		// no config file is fine — use defaults + env vars
	}

	return &Config{
		DatabaseURL:        viper.GetString("database_url"),
		EmbedSvcURL:        viper.GetString("embed.url"),
		EmbedModel:         viper.GetString("embed.model"),
		ConvertSvcURL:      viper.GetString("convert.url"),
		TranscribeSvcURL:   viper.GetString("transcribe.url"),
		VisionSvcURL:       viper.GetString("vision.url"),
		CaptionFrames:      viper.GetBool("vision.caption_frames"),
		ChunkSize:          viper.GetInt("pipeline.chunk_size"),
		ChunkOverlap:       viper.GetInt("pipeline.chunk_overlap"),
		MinChunkSize:       viper.GetInt("pipeline.min_chunk_size"),
		MinEmbedSize:       viper.GetInt64("pipeline.min_embed_size"),
		VideoFrameRate:     viper.GetFloat64("pipeline.video_frame_rate"),
		EnableCaption:      viper.GetBool("pipeline.enable_caption"),
		EnableOCR:          viper.GetBool("pipeline.enable_ocr"),
		EnableTranscribe:   viper.GetBool("pipeline.enable_transcribe"),
		SourceType:         viper.GetString("source.type"),
		WatchPath:          viper.GetString("source.watch_path"),
		S3Bucket:           viper.GetString("source.s3_bucket"),
		S3Prefix:           viper.GetString("source.s3_prefix"),
		S3Region:           viper.GetString("source.s3_region"),
		LargeFileThreshold: viper.GetInt64("source.large_file_threshold"),
		DaemonPort:         viper.GetInt("daemon.port"),
		WorkerCount:        viper.GetInt("daemon.worker_count"),
	}, nil
}
