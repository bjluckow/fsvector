package main

import (
	"fmt"
	"os"

	"github.com/bjluckow/fsvector/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("fsvectord starting\n")
	fmt.Printf("  watch path : %s\n", cfg.WatchPath)
	fmt.Printf("  embed model: %s\n", cfg.EmbedModel)
	fmt.Printf("  embed svc  : %s\n", cfg.EmbedSvcURL)
	fmt.Printf("  convert svc: %s\n", cfg.ConvertSvcURL)
	fmt.Printf("  source     : %s\n", cfg.Source)
}
