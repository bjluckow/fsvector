package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bjluckow/fsvector/internal/clients/embed"
	"github.com/bjluckow/fsvector/internal/query"
	query_ "github.com/bjluckow/fsvector/internal/query"
	"github.com/bjluckow/fsvector/internal/search"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/api"
	"github.com/bjluckow/fsvector/pkg/parse"
)

type Server struct {
	pool        store.Querier
	embedClient *embed.Client
	searchCfg   search.SearchConfig
	progress    *Progress
	trigger     chan bool
	started     time.Time
	sourceURI   string
}

func newServer(pool store.Querier, embedClient *embed.Client, progress *Progress, trigger chan bool, sourceURI string, searchCfg search.SearchConfig) *Server {
	return &Server{
		pool:        pool,
		embedClient: embedClient,
		searchCfg:   searchCfg,
		progress:    progress,
		trigger:     trigger,
		started:     time.Now(),
		sourceURI:   sourceURI,
	}
}

func (s *Server) Serve(ctx context.Context, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/reindex", s.handleReindex)
	mux.HandleFunc("/search/text", s.handleSearch)
	mux.HandleFunc("/search/image", s.handleSearchImage)
	mux.HandleFunc("/files", s.handleFiles)
	mux.HandleFunc("/files/", s.handleFileDetail)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/embed/text", s.handleEmbedText)
	mux.HandleFunc("/embed/image", s.handleEmbedImage)
	mux.HandleFunc("/export/files", s.handleExportFiles)
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	fmt.Printf("  daemon API listening on 127.0.0.1:%d\n", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.progress.snapshot()
	json.NewEncoder(w).Encode(map[string]any{
		"status":     statusString(snap.Running),
		"source":     s.sourceURI,
		"started_at": s.started,
		"reindex":    snap,
	})
}

func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	purge := r.URL.Query().Get("purge") == "true"

	select {
	case s.trigger <- purge:
		json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
	default:
		json.NewEncoder(w).Encode(map[string]string{"status": "already running"})
	}
}

func statusString(running bool) string {
	if running {
		return "reconciling"
	}
	return "idle"
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// set defaults
	if req.Limit == 0 {
		req.Limit = 10
	}
	if req.Page == 0 {
		req.Page = 1
	}

	// embed query
	vectors, err := s.embedClient.EmbedTexts(r.Context(), []string{req.Query})
	if err != nil {
		http.Error(w, fmt.Sprintf("embed: %v", err), http.StatusInternalServerError)
		return
	}

	cfg := s.searchCfg

	// override search config
	if req.FTSWeight > 0 {
		cfg.FTSWeight = req.FTSWeight
	}

	mode := search.SearchMode(req.Mode)
	if mode == "" {
		mode = cfg.DefaultMode
	}

	// build search query
	q := search.SearchQuery{
		Query:  req.Query,
		Mode:   mode,
		Config: cfg,
		Vector: vectors[0],
		Limit:  req.Limit,
		Offset: (req.Page - 1) * req.Limit,
	}

	if req.Modality != "" {
		q.Modality = req.Modality
	}
	if req.Ext != "" {
		q.Ext = req.Ext
	}
	if req.Source != "" {
		q.Source = req.Source
	}
	if req.Since != "" {
		t, err := parse.Since(req.Since)
		if err != nil {
			http.Error(w, fmt.Sprintf("since: %v", err), http.StatusBadRequest)
			return
		}
		q.Since = &t
	}
	if req.Before != "" {
		t, err := parse.Since(req.Before)
		if err != nil {
			http.Error(w, fmt.Sprintf("before: %v", err), http.StatusBadRequest)
			return
		}
		q.Before = &t
	}
	if req.MinSize != "" {
		n, err := parse.Size(req.MinSize)
		if err != nil {
			http.Error(w, fmt.Sprintf("min_size: %v", err), http.StatusBadRequest)
			return
		}
		q.MinSize = &n
	}
	if req.MaxSize != "" {
		n, err := parse.Size(req.MaxSize)
		if err != nil {
			http.Error(w, fmt.Sprintf("max_size: %v", err), http.StatusBadRequest)
			return
		}
		q.MaxSize = &n
	}
	if req.MinScore != 0 {
		q.MinScore = &req.MinScore
	}

	results, err := search.Search(r.Context(), s.pool, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	results = search.Normalize(results)

	// convert to api types
	apiResults := make([]api.SearchResult, len(results))
	for i, r := range results {
		apiResults[i] = api.SearchResult{
			Path:       r.Path,
			Modality:   r.Modality,
			Ext:        r.FileExt,
			Size:       r.Size,
			Score:      r.Score,
			NormScore:  r.NormScore,
			IndexedAt:  r.IndexedAt,
			ModifiedAt: r.ModifiedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.SearchResponse{Results: apiResults})
}

func (s *Server) handleSearchImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	limit := 10
	if v := r.FormValue("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	page := 1
	if v := r.FormValue("page"); v != "" {
		fmt.Sscanf(v, "%d", &page)
	}

	q := search.SearchQuery{
		Config:   s.searchCfg,
		Limit:    limit,
		Offset:   (page - 1) * limit,
		Modality: r.FormValue("modality"),
		Ext:      r.FormValue("ext"),
		Source:   r.FormValue("source"),
	}

	results, err := search.SearchByImage(r.Context(), s.pool, s.embedClient, header.Filename, data, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	results = search.Normalize(results)

	apiResults := make([]api.SearchResult, len(results))
	for i, r := range results {
		apiResults[i] = api.SearchResult{
			Path:       r.Path,
			Modality:   r.Modality,
			Ext:        r.FileExt,
			Size:       r.Size,
			Score:      r.Score,
			NormScore:  r.NormScore,
			IndexedAt:  r.IndexedAt,
			ModifiedAt: r.ModifiedAt,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.SearchResponse{Results: apiResults})
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	limit := 100
	page := 1
	if v := query.Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := query.Get("page"); v != "" {
		fmt.Sscanf(v, "%d", &page)
	}

	q := query_.ListQuery{
		Limit:          limit,
		Offset:         (page - 1) * limit,
		IncludeDeleted: query.Get("deleted") == "true",
		Modality:       query.Get("modality"),
		Ext:            query.Get("ext"),
		Source:         query.Get("source"),
	}

	if v := query.Get("since"); v != "" {
		t, err := parse.Since(v)
		if err == nil {
			q.Since = &t
		}
	}
	if v := query.Get("before"); v != "" {
		t, err := parse.Since(v)
		if err == nil {
			q.Before = &t
		}
	}

	files, err := query_.List(r.Context(), s.pool, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apiFiles := make([]api.FileItem, len(files))
	for i, f := range files {
		apiFiles[i] = api.FileItem{
			Path:        f.Path,
			Modality:    f.Modality,
			Ext:         f.FileExt,
			Size:        f.Size,
			IndexedAt:   f.IndexedAt,
			ModifiedAt:  f.ModifiedAt,
			DeletedAt:   f.DeletedAt,
			IsDuplicate: f.IsDuplicate,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.ListResponse{Files: apiFiles})
}

func (s *Server) handleFileDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/files/")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	includeDeleted := r.URL.Query().Get("include_deleted") == "true"

	f, err := query.Show(r.Context(), s.pool, path, includeDeleted)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.FileDetail{
		Path:          f.Path,
		Source:        f.Source,
		CanonicalPath: f.CanonicalPath,
		ContentHash:   f.ContentHash,
		Size:          f.Size,
		MimeType:      f.MimeType,
		Modality:      f.Modality,
		Ext:           f.FileExt,
		EmbedModel:    f.EmbedModel,
		ChunkCount:    f.ChunkCount,
		IndexedAt:     f.IndexedAt,
		ModifiedAt:    f.ModifiedAt,
		DeletedAt:     f.DeletedAt,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := query_.GetStats(r.Context(), s.pool)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.StatsResponse{
		Model:      stats.EmbedModel,
		Total:      stats.TotalFiles,
		Text:       stats.TextFiles,
		Image:      stats.ImageFiles,
		Audio:      stats.AudioFiles,
		Video:      stats.VideoFiles,
		Deleted:    stats.DeletedFiles,
		Duplicates: stats.Duplicates,
	})
}

func (s *Server) handleEmbedText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.EmbedTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	embeddings, err := s.embedClient.EmbedTexts(r.Context(), req.Texts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.EmbedTextResponse{Embeddings: embeddings})
}

func (s *Server) handleEmbedImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	embedding, err := s.embedClient.EmbedImage(r.Context(), header.Filename, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.EmbedImageResponse{Embedding: embedding})
}

func (s *Server) handleExportFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	q := query_.ListQuery{
		IncludeDeleted: query.Get("deleted") == "true",
		Modality:       query.Get("modality"),
		Ext:            query.Get("ext"),
		Source:         query.Get("source"),
	}

	if v := query.Get("since"); v != "" {
		t, err := parse.Since(v)
		if err == nil {
			q.Since = &t
		}
	}
	if v := query.Get("before"); v != "" {
		t, err := parse.Since(v)
		if err == nil {
			q.Before = &t
		}
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	enc := json.NewEncoder(w)
	flusher, canFlush := w.(http.Flusher)

	err := query_.ExportStream(r.Context(), s.pool, q, func(row api.ExportRow) error {
		if err := enc.Encode(row); err != nil {
			return err
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	})
	if err != nil {
		// headers already sent — just log
		fmt.Fprintf(os.Stderr, "export stream: %v\n", err)
	}
}
