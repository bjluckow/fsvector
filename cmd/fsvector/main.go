package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/bjluckow/fsvector/internal/config"
	"github.com/bjluckow/fsvector/internal/embed"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fsvector",
	Short: "Query and inspect a fsvector index",
	Long: `fsvector is the query interface for a fsvectord index.

It connects directly to Postgres and supports semantic search,
file listing, metadata inspection, and index statistics.`,
}

func main() {
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(daemonCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustConnect() (*pgx.Conn, *config.Config) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvector: config error: %v\n", err)
		os.Exit(1)
	}
	conn, err := pgx.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvector: db connect: %v\n", err)
		os.Exit(1)
	}
	return conn, cfg
}

func fmtSize(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// ── search ────────────────────────────────────────────────────────────────────

var searchLimit int

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search over indexed files",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		conn, cfg := mustConnect()
		defer conn.Close(ctx)

		embedClient := embed.NewClient(cfg.EmbedSvcURL)
		vectors, err := embedClient.EmbedTexts(ctx, []string{args[0]})
		if err != nil {
			return fmt.Errorf("embed query: %w", err)
		}

		results, err := store.Search(ctx, conn, vectors[0], searchLimit)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("no results")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SCORE\tMODALITY\tSIZE\tPATH")
		for _, r := range results {
			fmt.Fprintf(w, "%.4f\t%s\t%s\t%s\n",
				r.Score, r.Modality, fmtSize(r.Size), r.Path)
		}
		w.Flush()
		return nil
	},
}

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "number of results")
}

// ── ls ────────────────────────────────────────────────────────────────────────

var lsDeleted bool

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List indexed files",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		conn, _ := mustConnect()
		defer conn.Close(ctx)

		files, err := store.Ls(ctx, conn, lsDeleted)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			fmt.Println("no files indexed")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "MODALITY\tEXT\tSIZE\tMODIFIED\tPATH")
		for _, f := range files {
			deleted := ""
			if f.DeletedAt != nil {
				deleted = " [deleted]"
			}
			dupe := ""
			if f.IsDuplicate {
				dupe = " [dupe]"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s%s%s\n",
				f.Modality, f.FileExt, fmtSize(f.Size),
				fmtTime(f.ModifiedAt), f.Path, deleted, dupe)
		}
		w.Flush()
		return nil
	},
}

func init() {
	lsCmd.Flags().BoolVar(&lsDeleted, "deleted", false, "include soft-deleted files")
}

// ── show ──────────────────────────────────────────────────────────────────────

var showCmd = &cobra.Command{
	Use:   "show <path>",
	Short: "Show metadata for an indexed file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		conn, _ := mustConnect()
		defer conn.Close(ctx)

		f, err := store.Show(ctx, conn, args[0])
		if err != nil {
			return err
		}

		fmt.Printf("path         %s\n", f.Path)
		fmt.Printf("source       %s\n", f.Source)
		fmt.Printf("modality     %s\n", f.Modality)
		fmt.Printf("mime         %s\n", f.MimeType)
		fmt.Printf("ext          %s\n", f.FileExt)
		fmt.Printf("size         %s\n", fmtSize(f.Size))
		fmt.Printf("hash         %s\n", f.ContentHash)
		fmt.Printf("model        %s\n", f.EmbedModel)
		fmt.Printf("chunks       %d\n", f.ChunkCount)
		fmt.Printf("indexed at   %s\n", f.IndexedAt.Local().Format("2006-01-02 15:04:05"))
		fmt.Printf("modified at  %s\n", fmtTime(f.ModifiedAt))
		if f.DeletedAt != nil {
			fmt.Printf("deleted at   %s\n", f.DeletedAt.Local().Format("2006-01-02 15:04:05"))
		}
		if f.CanonicalPath != nil {
			fmt.Printf("duplicate of %s\n", *f.CanonicalPath)
		}
		return nil
	},
}

// ── stats ─────────────────────────────────────────────────────────────────────

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		conn, _ := mustConnect()
		defer conn.Close(ctx)

		s, err := store.GetStats(ctx, conn)
		if err != nil {
			return err
		}

		fmt.Printf("model        %s\n", s.EmbedModel)
		fmt.Printf("total        %d\n", s.TotalFiles)
		fmt.Printf("text         %d\n", s.TextFiles)
		fmt.Printf("image        %d\n", s.ImageFiles)
		fmt.Printf("deleted      %d\n", s.DeletedFiles)
		fmt.Printf("duplicates   %d\n", s.Duplicates)
		return nil
	},
}

// ── daemon ────────────────────────────────────────────────────────────────────

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the fsvectord daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the fsvectord stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("daemon start: not yet implemented")
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the fsvectord stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("daemon stop: not yet implemented")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("daemon status: not yet implemented")
		return nil
	},
}

var daemonLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail daemon logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("daemon logs: not yet implemented")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
}
