package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/bjluckow/fsvector/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "fsvector",
	Short: "fsvector — semantic file search",
}

var daemonHost string

func init() {
	rootCmd.PersistentFlags().StringVarP(
		&daemonHost, "host", "H",
		"http://localhost:8080",
		"fsvectord daemon address",
	)
	viper.BindPFlag("daemon.host", rootCmd.PersistentFlags().Lookup("host"))
	viper.BindEnv("daemon.host", "DAEMON_HOST")
	viper.SetDefault("daemon.host", "http://localhost:8080")

	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(statusCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func client() *api.Client {
	return api.NewClient(viper.GetString("daemon.host"))
}

// ── search ───────────────────────────────────────────────────────────────────

var (
	searchModality string
	searchExt      string
	searchSource   string
	searchSince    string
	searchBefore   string
	searchMinSize  string
	searchMaxSize  string
	searchMinScore float64
	searchLimit    int
	searchPage     int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search across indexed files",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		resp, err := client().Search(ctx, api.SearchRequest{
			Query:    args[0],
			Modality: searchModality,
			Ext:      searchExt,
			Source:   searchSource,
			Since:    searchSince,
			Before:   searchBefore,
			MinSize:  searchMinSize,
			MaxSize:  searchMaxSize,
			MinScore: searchMinScore,
			Limit:    searchLimit,
			Page:     searchPage,
		})
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SCORE\tNORM\tMODALITY\tEXT\tSIZE\tPATH")
		for _, r := range resp.Results {
			fmt.Fprintf(w, "%.4f\t%.4f\t%s\t%s\t%s\t%s\n",
				r.Score, r.NormScore, r.Modality, r.Ext,
				fmtSize(r.Size), r.Path)
		}
		w.Flush()
		return nil
	},
}

func init() {
	searchCmd.Flags().StringVarP(&searchModality, "modality", "m", "", "filter by modality (text, image, audio, video)")
	searchCmd.Flags().StringVar(&searchExt, "ext", "", "filter by file extension")
	searchCmd.Flags().StringVar(&searchSource, "source", "", "filter by source")
	searchCmd.Flags().StringVar(&searchSince, "since", "", "filter by modified date (e.g. 7d, 2024-01-01)")
	searchCmd.Flags().StringVar(&searchBefore, "before", "", "filter by modified date")
	searchCmd.Flags().StringVar(&searchMinSize, "min-size", "", "minimum file size (e.g. 10kb)")
	searchCmd.Flags().StringVar(&searchMaxSize, "max-size", "", "maximum file size (e.g. 100mb)")
	searchCmd.Flags().Float64Var(&searchMinScore, "min-score", 0, "minimum similarity score")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "maximum number of results")
	searchCmd.Flags().IntVar(&searchPage, "page", 1, "page number")
}

// ── ls ───────────────────────────────────────────────────────────────────────

var (
	lsModality string
	lsExt      string
	lsSource   string
	lsSince    string
	lsBefore   string
	lsDeleted  bool
	lsLimit    int
	lsPage     int
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List indexed files",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		resp, err := client().ListFiles(ctx, api.ListRequest{
			Modality:       lsModality,
			Ext:            lsExt,
			Source:         lsSource,
			Since:          lsSince,
			Before:         lsBefore,
			IncludeDeleted: lsDeleted,
			Limit:          lsLimit,
			Page:           lsPage,
		})
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "MODALITY\tEXT\tSIZE\tMODIFIED\tPATH")
		for _, f := range resp.Files {
			modified := ""
			if f.ModifiedAt != nil {
				modified = f.ModifiedAt.Format("2006-01-02")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				f.Modality, f.Ext, fmtSize(f.Size), modified, f.Path)
		}
		w.Flush()
		return nil
	},
}

func init() {
	lsCmd.Flags().StringVarP(&lsModality, "modality", "m", "", "filter by modality")
	lsCmd.Flags().StringVar(&lsExt, "ext", "", "filter by extension")
	lsCmd.Flags().StringVar(&lsSource, "source", "", "filter by source")
	lsCmd.Flags().StringVar(&lsSince, "since", "", "filter by modified date")
	lsCmd.Flags().StringVar(&lsBefore, "before", "", "filter by modified date")
	lsCmd.Flags().BoolVar(&lsDeleted, "deleted", false, "include soft-deleted files")
	lsCmd.Flags().IntVarP(&lsLimit, "limit", "n", 100, "maximum number of results")
	lsCmd.Flags().IntVar(&lsPage, "page", 1, "page number")
}

// ── show ─────────────────────────────────────────────────────────────────────

var showCmd = &cobra.Command{
	Use:   "show <path>",
	Short: "Show metadata for a specific file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		f, err := client().ShowFile(ctx, args[0])
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

// ── stats ────────────────────────────────────────────────────────────────────

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		s, err := client().Stats(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("model        %s\n", s.Model)
		fmt.Printf("total        %d\n", s.Total)
		fmt.Printf("text         %d\n", s.Text)
		fmt.Printf("image        %d\n", s.Image)
		fmt.Printf("audio        %d\n", s.Audio)
		fmt.Printf("video        %d\n", s.Video)
		fmt.Printf("deleted      %d\n", s.Deleted)
		fmt.Printf("duplicates   %d\n", s.Duplicates)
		return nil
	},
}

// ── reindex ──────────────────────────────────────────────────────────────────

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Trigger a full reindex on the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		resp, err := client().Reindex(ctx)
		if err != nil {
			return err
		}
		fmt.Println(resp.Status)
		return nil
	},
}

// ── status ───────────────────────────────────────────────────────────────────

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		s, err := client().Status(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("status       %s\n", s.Status)
		fmt.Printf("source       %s\n", s.Source)
		fmt.Printf("started      %s\n", s.StartedAt.Format("2006-01-02 15:04:05"))
		if s.Reindex.Running {
			fmt.Printf("reindex      running (%d/%d indexed)\n",
				s.Reindex.Indexed, s.Reindex.Total)
		} else if s.Reindex.FinishedAt != nil {
			fmt.Printf("reindex      completed at %s\n",
				s.Reindex.FinishedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("             %d indexed, %d deleted, %d skipped\n",
				s.Reindex.Indexed, s.Reindex.Deleted, s.Reindex.Skipped)
		}
		if len(s.Reindex.Errors) > 0 {
			fmt.Printf("errors       %d\n", len(s.Reindex.Errors))
			for _, e := range s.Reindex.Errors {
				fmt.Printf("             %s\n", e)
			}
		}
		return nil
	},
}

// ── helpers ──────────────────────────────────────────────────────────────────

func fmtSize(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1fGB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
