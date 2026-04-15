package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bjluckow/fsvector/internal/cluster"
	"github.com/bjluckow/fsvector/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	categoriesFile string
	outputDir      string
	download       bool
	filterModality string
	daemonHost     string
)

var rootCmd = &cobra.Command{
	Use:   "fscluster",
	Short: "Zero-shot file clustering using semantic similarity",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		cf, err := cluster.LoadCategories(categoriesFile)
		if err != nil {
			return fmt.Errorf("load categories: %w", err)
		}
		fmt.Printf("loaded %d categories\n", len(cf.Categories))

		client := api.NewClient(viper.GetString("daemon.host"))
		if _, err := client.Health(ctx); err != nil {
			return fmt.Errorf("daemon unreachable at %s: %w",
				viper.GetString("daemon.host"), err)
		}

		fmt.Println("clustering...")
		results, err := cluster.Run(ctx, client, cf)
		if err != nil {
			return fmt.Errorf("cluster: %w", err)
		}
		fmt.Printf("clustered %d files\n", len(results))

		if err := cluster.Write(results, cf, cluster.OutputConfig{
			Dir:      outputDir,
			Download: download,
			Modality: filterModality,
		}); err != nil {
			return fmt.Errorf("write output: %w", err)
		}

		printSummary(results, cf)
		return nil
	},
}

func init() {
	rootCmd.Flags().StringVarP(&categoriesFile, "categories", "c",
		"categories.yaml", "path to categories YAML file")
	rootCmd.Flags().StringVarP(&outputDir, "output", "o",
		"sorted", "output directory")
	rootCmd.Flags().BoolVar(&download, "download", false,
		"download S3 files locally")
	rootCmd.Flags().StringVarP(&filterModality, "modality", "m", "",
		"filter by modality")
	rootCmd.Flags().StringVarP(&daemonHost, "host", "H",
		"http://localhost:8080", "fsvectord daemon address")

	viper.BindPFlag("daemon.host", rootCmd.Flags().Lookup("host"))
	viper.BindEnv("daemon.host", "DAEMON_HOST")
	viper.SetDefault("daemon.host", "http://localhost:8080")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func printSummary(results []cluster.FileResult, cf *cluster.CategoriesFile) {
	counts := make(map[string]int)
	uncategorized := 0
	for _, r := range results {
		if len(r.Matches) == 0 {
			uncategorized++
			continue
		}
		for _, m := range r.Matches {
			counts[m.Name]++
		}
	}

	fmt.Println("\n── summary ──────────────────────────────")
	for _, cat := range cf.Categories {
		fmt.Printf("  %-25s %d files\n", cat.Name, counts[cat.Name])
	}
	if cf.Global.Uncategorized {
		fmt.Printf("  %-25s %d files\n", "uncategorized", uncategorized)
	}
	fmt.Printf("  %-25s %d files\n", "total", len(results))
}
