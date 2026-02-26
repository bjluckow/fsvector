package main

import (
	"fmt"
	"os"

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

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search over indexed files",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("search: %q (not yet implemented)\n", args[0])
		return nil
	},
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List indexed files",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("ls: not yet implemented")
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:   "show <path>",
	Short: "Show metadata for an indexed file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("show: %q (not yet implemented)\n", args[0])
		return nil
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("stats: not yet implemented")
		return nil
	},
}

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
